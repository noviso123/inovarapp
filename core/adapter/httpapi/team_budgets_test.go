package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"inovarapp/core/adapter/supabase"
	"inovarapp/core/domain"
)

func TestMergeReceivedBudgetMetadataUsesDatabaseAndSnapshotFallbacks(t *testing.T) {
	const budgetID = "22222222-2222-4222-8222-222222222222"
	const databaseServiceID = "33333333-3333-4333-8333-333333333333"
	const staleServiceID = "44444444-4444-4444-8444-444444444444"
	applianceID := "55555555-5555-4555-8555-555555555555"
	serviceID := staleServiceID
	signature, signedAt := "data:image/png;base64,snapshot", "2026-10-03T12:34:56Z"
	row := mutableBudgetRow{
		ID: budgetID, Number: "database-number", Description: `{"custom":"keep","assinatura":"","assinatura_em":null,"applianceId":""}`,
		ServiceID: databaseServiceID, Status: "APROVADO", TotalValue: 250,
	}
	snapshot := &domain.BudgetEstimate{
		ID: budgetID, Number: "snapshot-number", FinalValue: 999,
		ApplianceID: &applianceID, ServiceID: &serviceID, Signature: &signature, SignedAt: &signedAt,
	}
	instant := time.Date(2026, time.October, 4, 22, 0, 0, 0, time.FixedZone("BRT", -3*60*60))
	encoded, err := mergeReceivedBudgetMetadata(row, snapshot, instant)
	if err != nil {
		t.Fatal(err)
	}
	metadata := map[string]any{}
	if err := json.Unmarshal([]byte(encoded), &metadata); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{
		"custom": "keep", "numero": "snapshot-number", "applianceId": applianceID,
		"service_id": databaseServiceID, "assinatura": signature, "assinatura_em": signedAt,
		"pago": true, "pago_em": "2026-10-05T01:00:00Z", "valor_recebido": float64(999),
	} {
		if metadata[key] != want {
			t.Errorf("metadata[%s]=%#v want %#v; full metadata=%#v", key, metadata[key], want, metadata)
		}
	}
}

func TestMergeReceivedBudgetMetadataUsesEmptyOrNullFallbacks(t *testing.T) {
	encoded, err := mergeReceivedBudgetMetadata(mutableBudgetRow{Description: `{"custom":"keep"}`, TotalValue: 0}, nil, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	metadata := map[string]any{}
	if err := json.Unmarshal([]byte(encoded), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["numero"] != "" || metadata["service_id"] != nil || metadata["applianceId"] != nil || metadata["assinatura"] != nil || metadata["assinatura_em"] != nil {
		t.Fatalf("empty snapshot added invalid metadata: %#v", metadata)
	}
	if metadata["custom"] != "keep" {
		t.Fatalf("custom metadata lost: %#v", metadata)
	}
}

func TestTeamBudgetMutationUsesCallerJWTAndPreservesMetadata(t *testing.T) {
	const userID = "11111111-1111-4111-8111-111111111111"
	const budgetID = "22222222-2222-4222-8222-222222222222"
	var patch map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/v1/user" {
			if r.Header.Get("Authorization") != "Bearer caller-token" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"id":"` + userID + `"}`))
			return
		}
		if r.URL.Path == "/rest/v1/profiles" {
			_, _ = w.Write([]byte(`[{"id":"` + userID + `","tipo":"ADMIN"}]`))
			return
		}
		if r.URL.Path != "/rest/v1/budgets" || r.Header.Get("Authorization") != "Bearer caller-token" {
			t.Errorf("unexpected request %s %s auth=%q", r.Method, r.URL.String(), r.Header.Get("Authorization"))
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[{"id":"` + budgetID + `","numero":"12","cliente_id":"client-1","data":"2026-10-03","validade":"2026-11-02","descricao":"{\"custom\":\"keep-me\",\"totalValue\":250}","tipo_servico":"LIMPEZA","valor_mao_obra":250,"valor_material":0,"valor_total":250,"condicoes":"PIX","status":"PENDENTE"}]`))
			return
		}
		if r.Method == http.MethodPatch {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &patch)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		t.Errorf("unexpected budget method %s", r.Method)
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "service-role", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	handler := TeamBudgetMutationHandler{Supabase: client}
	request := httptest.NewRequest(http.MethodPatch, "/api/orcamento-equipe?id="+budgetID, strings.NewReader(`{"id":"`+budgetID+`","acao":"STATUS","status":"APROVAR"}`))
	request.Header.Set("Authorization", "Bearer caller-token")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if patch["status"] != "APROVADO" {
		t.Fatalf("patch=%v", patch)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(patch["descricao"].(string)), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["custom"] != "keep-me" || metadata["resposta_cliente"] != "APROVAR" {
		t.Fatalf("metadata=%v", metadata)
	}
}

func TestTeamBudgetEditPreservesLifecycleAndPaymentMetadata(t *testing.T) {
	const userID = "11111111-1111-4111-8111-111111111111"
	const budgetID = "22222222-2222-4222-8222-222222222222"
	const serviceID = "33333333-3333-4333-8333-333333333333"
	var patch map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = w.Write([]byte(`{"id":"` + userID + `"}`))
		case "/rest/v1/profiles":
			_, _ = w.Write([]byte(`[{"id":"` + userID + `","tipo":"ADMIN"}]`))
		case "/rest/v1/budgets":
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`[{"id":"` + budgetID + `","numero":"12","cliente_id":"client-1","data":"2026-10-03","validade":"2026-11-02","descricao":"{\"assinatura\":\"data:image/png;base64,signature\",\"assinatura_em\":\"2026-10-03T12:00:00Z\",\"pago\":true,\"pago_em\":\"2026-10-03T13:00:00Z\",\"valor_recebido\":120,\"service_id\":\"` + serviceID + `\",\"resposta_cliente\":\"APROVAR\",\"custom\":\"keep\"}","tipo_servico":"LIMPEZA","valor_mao_obra":250,"valor_material":0,"valor_total":250,"condicoes":"PIX","status":"APROVADO","service_id":"` + serviceID + `"}]`))
				return
			}
			if r.Method == http.MethodPatch {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &patch)
				w.WriteHeader(http.StatusNoContent)
				return
			}
			t.Errorf("unexpected budget method %s", r.Method)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "service-role", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	budget := domain.BudgetEstimate{
		ID: budgetID, ClientID: "client-1", ClientName: "Ana", Date: "2026-10-03", ValidUntil: "2026-11-12",
		Items:             []domain.BudgetItem{{ID: "item-1", Description: "Manutenção", Quantity: 1, UnitPrice: 300, Category: domain.BudgetItemService}},
		PaymentConditions: "PIX", Status: domain.BudgetPending,
	}
	encoded, err := json.Marshal(map[string]any{"id": budgetID, "acao": "EDIT", "orcamento": budget})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPatch, "/api/orcamento-equipe?id="+budgetID, strings.NewReader(string(encoded)))
	request.Header.Set("Authorization", "Bearer caller-token")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	TeamBudgetMutationHandler{Supabase: client}.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if patch["status"] != nil {
		t.Fatalf("edit must not change the current approved state, patch=%v", patch)
	}
	if patch["service_id"] != nil {
		t.Fatalf("edit must not move or clear the linked service, patch=%v", patch)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(patch["descricao"].(string)), &metadata); err != nil {
		t.Fatal(err)
	}
	for key, expected := range map[string]any{
		"assinatura": "data:image/png;base64,signature", "assinatura_em": "2026-10-03T12:00:00Z",
		"pago": true, "pago_em": "2026-10-03T13:00:00Z", "valor_recebido": float64(120),
		"service_id": serviceID, "resposta_cliente": "APROVAR", "custom": "keep",
	} {
		if metadata[key] != expected {
			t.Errorf("metadata[%s]=%v, want %v; all metadata=%v", key, metadata[key], expected, metadata)
		}
	}
}
