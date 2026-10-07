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

func TestBudgetApprovalCustomerMessageIncludesAppLink(t *testing.T) {
	message := expandAccountMessage(nil, "orcamento_aprovado_cliente", "", map[string]string{
		"cliente": "Ana", "numero": "42", "valor": "R$ 280,00", "empresa": "Inovar Refrigeração", "app": domain.AppPublicURL,
	})
	if !strings.Contains(message, domain.AppPublicURL) || strings.Contains(message, "{{app}}") {
		t.Fatalf("approval message does not contain the app link: %q", message)
	}
}

func TestBudgetResponseApprovalStoresSignatureAndPreservesMetadata(t *testing.T) {
	t.Setenv("EVOLUTION_URL", "")
	t.Setenv("EVOLUTION_API_KEY", "")
	t.Setenv("WHATSAPP_TOKEN", "")
	t.Setenv("WHATSAPP_PHONE_ID", "")
	var patched map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"user-1"}`)
		case r.URL.Path == "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"CLIENTE"}]`)
		case r.URL.Path == "/rest/v1/budgets" && r.Method == http.MethodGet && r.Header.Get("Authorization") == "Bearer user-access":
			_, _ = io.WriteString(w, `[{"id":"budget-1","cliente_id":"customer-1","descricao":"{\"clientName\":\"João\",\"totalValue\":125.5,\"campo_legado\":{\"preservar\":true}}","status":"ENVIADO"}]`)
		case r.URL.Path == "/rest/v1/customers":
			_, _ = io.WriteString(w, `[{"id":"customer-1"}]`)
		case r.URL.Path == "/rest/v1/budgets" && r.Method == http.MethodPatch:
			if r.Header.Get("Authorization") != "Bearer service-role" || r.Header.Get("Prefer") != "return=minimal" {
				t.Errorf("unexpected patch authorization/prefer: %q %q", r.Header.Get("Authorization"), r.Header.Get("Prefer"))
			}
			if err := json.NewDecoder(r.Body).Decode(&patched); err != nil {
				t.Fatal(err)
			}
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == budgetResponseConfigPath:
			http.NotFound(w, r)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon-key", ServiceRoleKey: "service-role", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	notifier := &recordingTeamNotifier{}
	handler := BudgetResponseHandler{Supabase: client, Notifier: notifier, Now: func() time.Time {
		return time.Date(2026, 10, 3, 12, 13, 14, 123000000, time.FixedZone("BRT", -3*60*60))
	}}
	req := httptest.NewRequest(http.MethodPost, "/api/orcamento-resposta", strings.NewReader(`{"orcamento_id":"budget-1","acao":"APROVAR","assinatura":"data:image/png;base64,abc"}`))
	req.Header.Set("Authorization", "Bearer user-access")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body)
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["status"] != "APROVADO" || result["assinado_em"] != "2026-10-03T15:13:14.123Z" || result["whatsapp_enfileirado"] != false || result["mensagem"] != "Orçamento aprovado e assinado! A Inovar foi notificada. O status foi salvo, mas a notificação do WhatsApp não entrou na fila." {
		t.Fatalf("response = %#v", result)
	}
	if len(notifier.notices) != 1 || notifier.notices[0] != (recordedPushNotice{title: "Orçamento aprovado pelo cliente!", body: "Assinatura registrada — agende o serviço."}) {
		t.Fatalf("team notices=%+v", notifier.notices)
	}
	if patched["status"] != "APROVADO" {
		t.Fatalf("patch = %#v", patched)
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal([]byte(patched["descricao"]), &metadata); err != nil {
		t.Fatal(err)
	}
	if string(metadata["assinatura"]) != `"data:image/png;base64,abc"` || string(metadata["assinatura_em"]) != `"2026-10-03T15:13:14.123Z"` || string(metadata["resposta_cliente"]) != `"APROVAR"` {
		t.Fatalf("response metadata = %s", patched["descricao"])
	}
	var legacy map[string]bool
	if err := json.Unmarshal(metadata["campo_legado"], &legacy); err != nil || !legacy["preservar"] {
		t.Fatalf("legacy metadata lost: %s", patched["descricao"])
	}
}

func TestBudgetResponseRejectsMissingSignatureForCustomer(t *testing.T) {
	server := budgetResponseAuthServer(t, `[{"id":"budget-1","cliente_id":"customer-1","descricao":"{}","status":"ENVIADO"}]`, `[{"id":"customer-1"}]`)
	defer server.Close()
	client, _ := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon-key", ServiceRoleKey: "service-role", HTTPClient: server.Client()})
	handler := BudgetResponseHandler{Supabase: client}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"orcamento_id":"budget-1","acao":"APROVAR"}`))
	req.Header.Set("Authorization", "Bearer user-access")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "Assinatura digital obrigatoria") {
		t.Fatalf("status/body = %d %s", response.Code, response.Body)
	}
}

func TestBudgetResponseRefusalNotifiesTheTeamAfterSuccessfulMutation(t *testing.T) {
	server := budgetResponseAuthServer(t, `[{"id":"budget-1","cliente_id":"customer-1","descricao":"{}","status":"ENVIADO"}]`, `[{"id":"customer-1"}]`)
	defer server.Close()
	client, _ := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon-key", ServiceRoleKey: "service-role", HTTPClient: server.Client()})
	notifier := &recordingTeamNotifier{}
	handler := BudgetResponseHandler{Supabase: client, Notifier: notifier}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"orcamento_id":"budget-1","acao":"RECUSAR"}`))
	req.Header.Set("Authorization", "Bearer user-access")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK || len(notifier.notices) != 1 || notifier.notices[0] != (recordedPushNotice{title: "Orçamento recusado", body: "O cliente recusou a proposta."}) {
		t.Fatalf("response=%d %s notices=%+v", response.Code, response.Body.String(), notifier.notices)
	}
}

func TestBudgetResponseRejectsBudgetOwnedByAnotherCustomer(t *testing.T) {
	server := budgetResponseAuthServer(t, `[{"id":"budget-1","cliente_id":"someone-else","descricao":"{}","status":"ENVIADO"}]`, `[{"id":"customer-1"}]`)
	defer server.Close()
	client, _ := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon-key", ServiceRoleKey: "service-role", HTTPClient: server.Client()})
	handler := BudgetResponseHandler{Supabase: client}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"orcamento_id":"budget-1","acao":"RECUSAR"}`))
	req.Header.Set("Authorization", "Bearer user-access")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status/body = %d %s", response.Code, response.Body)
	}
}

func TestBudgetResponseOnlyUsesOwnerCheckedFallbackAfterSuccessfulEmptyRLSRead(t *testing.T) {
	cases := []struct {
		name              string
		rlsStatus         int
		fallbackClientID  string
		wantStatus        int
		wantFallbackReads int
		wantMessage       string
	}{
		{"own answered budget is idempotent when RLS hides it", http.StatusOK, "customer-1", http.StatusConflict, 1, "Este orçamento já foi aprovado"},
		{"another customer's hidden budget remains not found", http.StatusOK, "someone-else", http.StatusNotFound, 1, "Orçamento não encontrado para este usuário"},
		{"RLS error never falls back to service role", http.StatusForbidden, "customer-1", http.StatusNotFound, 0, "Orçamento não encontrado para este usuário"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var fallbackReads, patches int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/auth/v1/user":
					_, _ = io.WriteString(w, `{"id":"user-1"}`)
				case "/rest/v1/profiles":
					_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"CLIENTE"}]`)
				case "/rest/v1/budgets":
					if r.Method == http.MethodPatch {
						patches++
						t.Errorf("answered/hidden budget must not be patched")
						w.WriteHeader(http.StatusNoContent)
						return
					}
					if r.Header.Get("Authorization") == "Bearer user-access" {
						w.WriteHeader(test.rlsStatus)
						if test.rlsStatus == http.StatusOK {
							_, _ = io.WriteString(w, `[]`)
						}
						return
					}
					fallbackReads++
					if r.Header.Get("Authorization") != "Bearer service-role" {
						t.Errorf("fallback authorization = %q", r.Header.Get("Authorization"))
					}
					_, _ = io.WriteString(w, `[{"id":"budget-1","cliente_id":"`+test.fallbackClientID+`","descricao":"{}","status":"APROVADO"}]`)
				case "/rest/v1/customers":
					if r.Header.Get("Authorization") != "Bearer user-access" {
						t.Errorf("customer lookup did not use caller JWT: %q", r.Header.Get("Authorization"))
					}
					_, _ = io.WriteString(w, `[{"id":"customer-1"}]`)
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon-key", ServiceRoleKey: "service-role", HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			handler := BudgetResponseHandler{Supabase: client}
			req := httptest.NewRequest(http.MethodPost, "/api/orcamento-resposta", strings.NewReader(`{"orcamento_id":"budget-1","acao":"RECUSAR"}`))
			req.Header.Set("Authorization", "Bearer user-access")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if response.Code != test.wantStatus || !strings.Contains(response.Body.String(), test.wantMessage) {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
			if fallbackReads != test.wantFallbackReads || patches != 0 {
				t.Fatalf("fallback reads=%d (want %d), patches=%d", fallbackReads, test.wantFallbackReads, patches)
			}
		})
	}
}

func budgetResponseAuthServer(t *testing.T, budgetRows, customerRows string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"user-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"CLIENTE"}]`)
		case "/rest/v1/budgets":
			_, _ = io.WriteString(w, budgetRows)
		case "/rest/v1/customers":
			_, _ = io.WriteString(w, customerRows)
		case budgetResponseConfigPath:
			http.NotFound(w, r)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
}
