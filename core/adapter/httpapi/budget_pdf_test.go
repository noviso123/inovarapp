package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"inovarapp/core/adapter/supabase"
)

func TestBudgetPDFRequiresTeamAndUsesBudgetFromRLS(t *testing.T) {
	var budgetReadToken string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"staff-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"staff-1","tipo":"TECNICO"}]`)
		case "/rest/v1/budgets":
			budgetReadToken = r.Header.Get("Authorization")
			_, _ = io.WriteString(w, `[{"id":"11111111-1111-4111-8111-111111111111","numero":"2026-0042","cliente_id":"customer-1","data":"2026-10-03","validade":"2026-10-18","tipo_servico":"Limpeza de Ar","valor_mao_obra":250,"valor_material":20,"valor_total":270,"status":"ENVIADO","descricao":"{\"clientName\":\"João Silva\",\"clientPhone\":\"5527999999999\",\"clientAddress\":\"Vitória\",\"equipmentName\":\"LG Inverter\",\"items\":[{\"id\":\"i1\",\"description\":\"Limpeza de serpentina\",\"quantity\":1,\"unitPrice\":270,\"totalPrice\":270,\"category\":\"servico\"}],\"warrantyTerms\":\"90 dias\"}"}]`)
		case technicianSettingsPath:
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "service", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/orcamento-pdf", strings.NewReader(`{"budget_id":"11111111-1111-4111-8111-111111111111"}`))
	request.Header.Set("Authorization", "Bearer staff-token")
	response := httptest.NewRecorder()
	BudgetPDFHandler{Supabase: client}.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/pdf" || !strings.HasPrefix(response.Body.String(), "%PDF-") {
		t.Fatalf("status=%d headers=%v body=%q", response.Code, response.Header(), response.Body.String()[:min(len(response.Body.String()), 40)])
	}
	if budgetReadToken != "Bearer staff-token" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("budget read token=%q cache=%q", budgetReadToken, response.Header().Get("Cache-Control"))
	}
}

func TestBudgetPDFRejectsCustomer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"customer-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"customer-1","tipo":"CLIENTE"}]`)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/orcamento-pdf", strings.NewReader(`{"budget_id":"11111111-1111-4111-8111-111111111111"}`))
	request.Header.Set("Authorization", "Bearer customer-token")
	response := httptest.NewRecorder()
	BudgetPDFHandler{Supabase: client}.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
