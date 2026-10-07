package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"inovarapp/core/adapter/supabase"
	"inovarapp/core/domain"
)

func TestCustomersListUsesCallerJWTAndJoinsAppliances(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"staff-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"staff-1","tipo":"ADMIN"}]`)
		case "/rest/v1/customers":
			calls++
			if r.Header.Get("Authorization") != "Bearer staff-token" || r.URL.Query().Get("order") != "nome.asc" {
				t.Errorf("customer request auth=%q query=%s", r.Header.Get("Authorization"), r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `[{"id":"c1","nome":"João"},{"id":"c2","nome":"Ana"}]`)
		case "/rest/v1/air_conditioners":
			calls++
			if r.Header.Get("Authorization") != "Bearer staff-token" {
				t.Errorf("appliance request did not use caller JWT")
			}
			_, _ = io.WriteString(w, `[{"id":"a1","cliente_id":"c1","marca":"LG"}]`)
		default:
			t.Errorf("unexpected request %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "service", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/clientes", nil)
	req.Header.Set("Authorization", "Bearer staff-token")
	res := httptest.NewRecorder()
	(CustomersHandler{Supabase: client}).ServeHTTP(res, req)
	if res.Code != http.StatusOK || calls != 2 {
		t.Fatalf("status=%d calls=%d body=%s", res.Code, calls, res.Body.String())
	}
	var customers []map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &customers); err != nil {
		t.Fatal(err)
	}
	appliances := customers[0]["appliances"].([]any)
	if customers[0]["nome"] != "João" || len(appliances) != 1 || customers[1]["appliances"] != nil {
		t.Fatalf("joined customers=%#v", customers)
	}
}

func TestCustomersWritesAreAllowlistedAndDeleteIsAdminOnly(t *testing.T) {
	for _, tc := range []struct {
		role, method, body string
		want               int
	}{
		{"TECNICO", http.MethodPost, `{"id":"550e8400-e29b-41d4-a716-446655440000","nome":"Bia","profile_id":"another-user","admin":true}`, http.StatusCreated},
		{"TECNICO", http.MethodPost, `{"id":"local-customer","nome":"Bia"}`, http.StatusBadRequest},
		{"TECNICO", http.MethodPatch, `{"id":"c1","fields":{"id":"550e8400-e29b-41d4-a716-446655440000"}}`, http.StatusBadRequest},
		{"TECNICO", http.MethodDelete, `{"id":"c1"}`, http.StatusForbidden},
		{"ADMIN", http.MethodDelete, `{"id":"c1"}`, http.StatusNoContent},
	} {
		var saved map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/auth/v1/user":
				_, _ = io.WriteString(w, `{"id":"staff-1"}`)
			case "/rest/v1/profiles":
				_, _ = io.WriteString(w, `[{"id":"staff-1","tipo":"`+tc.role+`"}]`)
			case "/rest/v1/customers":
				if r.Method == http.MethodPost {
					_ = json.NewDecoder(r.Body).Decode(&saved)
					w.WriteHeader(http.StatusCreated)
					_, _ = io.WriteString(w, `[{"id":"c1"}]`)
					return
				}
				if r.Method == http.MethodDelete {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				t.Errorf("unexpected customers method %s", r.Method)
			default:
				t.Errorf("unexpected request %s", r.URL.String())
				http.NotFound(w, r)
			}
		}))
		client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "service", HTTPClient: server.Client()})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(tc.method, "/api/clientes", strings.NewReader(tc.body))
		req.Header.Set("Authorization", "Bearer staff-token")
		res := httptest.NewRecorder()
		(CustomersHandler{Supabase: client}).ServeHTTP(res, req)
		server.Close()
		if res.Code != tc.want {
			t.Fatalf("%s %s status=%d want=%d body=%s", tc.role, tc.method, res.Code, tc.want, res.Body.String())
		}
		if tc.method == http.MethodPost && tc.want == http.StatusCreated && (saved["id"] != "550e8400-e29b-41d4-a716-446655440000" || saved["nome"] != "Bia" || saved["profile_id"] != nil || saved["admin"] != nil) {
			t.Fatalf("unsafe customer payload %#v", saved)
		}
	}
}

func TestTeamBudgetsUsesCallerJWTAndRestrictsRole(t *testing.T) {
	for _, tc := range []struct {
		role string
		want int
	}{
		{"ADMIN", http.StatusOK},
		{"TECNICO", http.StatusOK},
		{"CLIENTE", http.StatusForbidden},
	} {
		budgetCalls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/auth/v1/user":
				_, _ = io.WriteString(w, `{"id":"staff-1"}`)
			case "/rest/v1/profiles":
				_, _ = io.WriteString(w, `[{"id":"staff-1","tipo":"`+tc.role+`"}]`)
			case "/rest/v1/budgets":
				budgetCalls++
				if r.Header.Get("Authorization") != "Bearer staff-token" || r.URL.Query().Get("order") != "created_at.desc" {
					t.Errorf("budget request auth=%q query=%s", r.Header.Get("Authorization"), r.URL.RawQuery)
				}
				_, _ = io.WriteString(w, `[{"id":"b1","cliente_id":"c1","data":"2026-10-01","validade":"2026-10-15","valor_total":125,"status":"APROVADO","descricao":"{\"items\":[{\"id\":\"1\",\"description\":\"Limpeza de Ar\",\"quantity\":1,\"unitPrice\":125,\"totalPrice\":125,\"category\":\"servico\"}],\"service_id\":\"s1\"}"}]`)
			default:
				t.Errorf("unexpected request %s", r.URL.String())
				http.NotFound(w, r)
			}
		}))
		client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "service", HTTPClient: server.Client()})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodGet, "/api/orcamentos", nil)
		req.Header.Set("Authorization", "Bearer staff-token")
		res := httptest.NewRecorder()
		(TeamBudgetsHandler{Supabase: client}).ServeHTTP(res, req)
		server.Close()
		if res.Code != tc.want {
			t.Fatalf("role=%s status=%d want=%d body=%s", tc.role, res.Code, tc.want, res.Body.String())
		}
		if tc.role == "CLIENTE" {
			if budgetCalls != 0 {
				t.Fatalf("customer role made %d budget request(s)", budgetCalls)
			}
			continue
		}
		var budgets []domain.BudgetEstimate
		if err := json.Unmarshal(res.Body.Bytes(), &budgets); err != nil {
			t.Fatal(err)
		}
		if budgetCalls != 1 || len(budgets) != 1 || budgets[0].FinalValue != 125 || budgets[0].Status != domain.BudgetApproved || budgets[0].ServiceID == nil || *budgets[0].ServiceID != "s1" {
			t.Fatalf("calls=%d budgets=%#v", budgetCalls, budgets)
		}
	}
}

func TestTeamBudgetsCreateRecalculatesAndUsesCallerJWT(t *testing.T) {
	budgetCalls, rpcCalls := 0, 0
	var saved map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"staff-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"staff-1","tipo":"ADMIN"}]`)
		case "/rest/v1/rpc/proximo_numero_orcamento":
			rpcCalls++
			if r.Header.Get("Authorization") != "Bearer staff-token" {
				t.Errorf("RPC auth=%q", r.Header.Get("Authorization"))
			}
			_, _ = io.WriteString(w, `"2026-0042"`)
		case "/rest/v1/budgets":
			budgetCalls++
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer staff-token" {
				t.Errorf("budget method/auth=%s %q", r.Method, r.Header.Get("Authorization"))
			}
			if err := json.NewDecoder(r.Body).Decode(&saved); err != nil {
				t.Errorf("decode write: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `[{"id":"550e8400-e29b-41d4-a716-446655440000","numero":"2026-0042","cliente_id":"c1","data":"2026-10-03","validade":"2026-11-02","valor_mao_obra":200,"valor_material":50,"valor_total":240,"condicoes":"PIX","status":"ENVIADO","descricao":"{\"numero\":\"2026-0042\",\"clientName\":\"Bia\",\"items\":[{\"id\":\"i1\",\"description\":\"Limpeza de Ar\",\"quantity\":1,\"unitPrice\":250,\"totalPrice\":250,\"category\":\"servico\"}],\"discount\":10,\"totalValue\":250}"}]`)
		default:
			t.Errorf("unexpected request %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "service", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"id":"550e8400-e29b-41d4-a716-446655440000","clientId":"c1","clientName":"Bia","date":"2026-10-03","validUntil":"2026-11-02","items":[{"id":"i1","description":"Limpeza de Ar","quantity":1,"unitPrice":250,"totalPrice":1,"category":"servico"}],"discount":10,"totalValue":1,"finalValue":1,"status":"APROVADO"}`
	req := httptest.NewRequest(http.MethodPost, "/api/orcamentos", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer staff-token")
	res := httptest.NewRecorder()
	(TeamBudgetsHandler{Supabase: client}).ServeHTTP(res, req)
	server.Close()
	if res.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if budgetCalls != 1 || rpcCalls != 1 || saved["id"] != "550e8400-e29b-41d4-a716-446655440000" || saved["numero"] != "2026-0042" || saved["valor_mao_obra"] != float64(250) || saved["valor_total"] != float64(240) || saved["status"] != "ENVIADO" {
		t.Fatalf("rpc=%d calls=%d write=%#v", rpcCalls, budgetCalls, saved)
	}
	var returned domain.BudgetEstimate
	if err := json.Unmarshal(res.Body.Bytes(), &returned); err != nil {
		t.Fatal(err)
	}
	if returned.ID != "550e8400-e29b-41d4-a716-446655440000" || returned.ClientName != "Bia" || returned.Status != domain.BudgetPending || returned.FinalValue != 240 {
		t.Fatalf("returned=%#v", returned)
	}
}
