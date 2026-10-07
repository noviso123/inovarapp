package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"inovarapp/core/adapter/supabase"
)

func TestDecodeMutableBudgetRowAcceptsPostgRESTArrayAndSingleObject(t *testing.T) {
	for _, body := range []string{
		`[{"id":"11111111-1111-4111-8111-111111111111","status":"APROVADO"}]`,
		`{"id":"11111111-1111-4111-8111-111111111111","status":"APROVADO"}`,
	} {
		row, ok := decodeMutableBudgetRow([]byte(body))
		if !ok || row.Status != "APROVADO" {
			t.Errorf("decodeMutableBudgetRow(%s) = %#v, %v", body, row, ok)
		}
	}
	for _, body := range []string{`[]`, `[{"id":"one"},{"id":"two"}]`, `{}`} {
		if _, ok := decodeMutableBudgetRow([]byte(body)); ok {
			t.Errorf("decodeMutableBudgetRow(%s) unexpectedly succeeded", body)
		}
	}
}

func TestTeamBudgetEditPreservesSignaturePaymentAndStatus(t *testing.T) {
	var saved map[string]any
	var metadata map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("request %s %s", r.Method, r.URL.Path)
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"staff-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"staff-1","tipo":"TECNICO"}]`)
		case "/rest/v1/budgets":
			if r.Method == http.MethodGet {
				_, _ = io.WriteString(w, `[{"id":"11111111-1111-4111-8111-111111111111","numero":"2026-7","cliente_id":"c1","data":"2026-10-01","validade":"2026-11-01","valor_total":250,"status":"APROVADO","descricao":"{\"clientName\":\"Ana\",\"assinatura\":\"signed-png\",\"assinatura_em\":\"2026-10-01T10:00:00Z\",\"pago\":true,\"pago_em\":\"2026-10-02T10:00:00Z\",\"valor_recebido\":250,\"resposta_cliente\":\"APROVAR\",\"customLegacy\":\"keep\"}"}]`)
				return
			}
			if r.Method != http.MethodPatch {
				t.Errorf("method=%s", r.Method)
			}
			if err := json.NewDecoder(r.Body).Decode(&saved); err != nil {
				t.Fatal(err)
			}
			_ = json.Unmarshal([]byte(saved["descricao"].(string)), &metadata)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "service", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"acao":"EDIT","orcamento":{"id":"11111111-1111-4111-8111-111111111111","clientId":"c1","clientName":"Ana editada","date":"2026-10-03","validUntil":"2026-11-03","items":[{"id":"i1","description":"Limpeza de Ar","quantity":2,"unitPrice":150,"totalPrice":1,"category":"servico"}],"discount":10,"status":"recusado"}}`
	req := httptest.NewRequest(http.MethodPatch, "/api/orcamento-equipe?id=11111111-1111-4111-8111-111111111111", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer staff-token")
	res := httptest.NewRecorder()
	TeamBudgetMutationHandler{Supabase: client}.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if saved["status"] != nil || saved["cliente_id"] != nil {
		t.Fatalf("edit changed immutable columns: %#v", saved)
	}
	for key, want := range map[string]any{"assinatura": "signed-png", "assinatura_em": "2026-10-01T10:00:00Z", "pago": true, "pago_em": "2026-10-02T10:00:00Z", "valor_recebido": float64(250), "resposta_cliente": "APROVAR", "customLegacy": "keep"} {
		if metadata[key] != want {
			t.Errorf("metadata[%s]=%#v want %#v", key, metadata[key], want)
		}
	}
	if saved["valor_total"] != float64(290) || saved["status"] != nil {
		t.Fatalf("recomputed write=%#v", saved)
	}
}
