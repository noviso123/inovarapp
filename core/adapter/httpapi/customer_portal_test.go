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

func TestCustomerPortalReadsOnlyOwnedRowsThroughCallerRLS(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer portal-user-token" {
			t.Errorf("request did not use the caller token: %s %s", r.Method, r.URL)
		}
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"portal-user"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"portal-user","tipo":"CLIENTE"}]`)
		case "/rest/v1/customers":
			if r.URL.Query().Get("profile_id") != "eq.portal-user" {
				t.Errorf("customer lookup was not bound to the authenticated profile: %s", r.URL)
			}
			_, _ = io.WriteString(w, `[{"id":"customer-1","nome":"João"}]`)
		case "/rest/v1/air_conditioners", "/rest/v1/services", "/rest/v1/budgets":
			if r.URL.Query().Get("cliente_id") != "eq.customer-1" {
				t.Errorf("portal data query not scoped to linked customer: %s", r.URL)
			}
			paths = append(paths, r.URL.Path)
			if r.URL.Path == "/rest/v1/services" && r.URL.Query().Get("order") != "data_solicitacao.desc" {
				t.Errorf("service ordering differs from portal contract: %s", r.URL)
			}
			_, _ = io.WriteString(w, `[{"id":"row-1"}]`)
		default:
			t.Errorf("unexpected Supabase request: %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon-key", ServiceRoleKey: "service-role", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	handler := CustomerPortalHandler{Supabase: client}
	request := httptest.NewRequest(http.MethodGet, "/api/portal/cliente", nil)
	request.Header.Set("Authorization", "Bearer portal-user-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status/body = %d %s", response.Code, response.Body)
	}
	if len(paths) != 3 || !strings.Contains(response.Body.String(), `"nome":"João"`) || !strings.Contains(response.Body.String(), `"services":[{"id":"row-1"}]`) {
		t.Fatalf("incomplete portal response: paths=%v body=%s", paths, response.Body)
	}
}

func TestCustomerPortalRejectsTeamAndWrongMethodBeforeDataReads(t *testing.T) {
	dataRead := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"team-user"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"team-user","tipo":"TECNICO"}]`)
		default:
			dataRead = true
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon-key", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	handler := CustomerPortalHandler{Supabase: client}
	request := httptest.NewRequest(http.MethodGet, "/api/portal/cliente", nil)
	request.Header.Set("Authorization", "Bearer team-user-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || dataRead {
		t.Fatalf("team access status=%d dataRead=%v", response.Code, dataRead)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/portal/cliente", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d", response.Code)
	}
}

func TestCustomerApplianceCreationDerivesOwnerAndUsesCallerJWT(t *testing.T) {
	var inserted map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer appliance-token" {
			t.Errorf("request did not use caller token: %s %s", r.Method, r.URL)
		}
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"appliance-profile"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"appliance-profile","tipo":"CLIENTE"}]`)
		case "/rest/v1/customers":
			if r.URL.Query().Get("profile_id") != "eq.appliance-profile" {
				t.Errorf("customer query was not profile-scoped: %s", r.URL)
			}
			_, _ = io.WriteString(w, `[{"id":"appliance-customer"}]`)
		case "/rest/v1/air_conditioners":
			if err := json.NewDecoder(r.Body).Decode(&inserted); err != nil {
				t.Fatal(err)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"appliance-new"}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	handler := CustomerAppliancesHandler{Supabase: accountTestClient(t, server)}
	request := httptest.NewRequest(http.MethodPost, "/api/cliente/aparelhos", strings.NewReader(`{"cliente_id":"other","marca":" LG ","modelo":"Dual Inverter","btus":12000,"tipo":"Inverter","ambiente":"Sala","ultima_manutencao":"2026-10-03","is_admin":true}`))
	request.Header.Set("Authorization", "Bearer appliance-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status/body = %d %s", response.Code, response.Body)
	}
	if inserted["cliente_id"] != "appliance-customer" || inserted["marca"] != "LG" || inserted["status"] != nil || inserted["is_admin"] != nil {
		t.Fatalf("unsafe or incomplete appliance payload: %#v", inserted)
	}
}

func TestCustomerApplianceValidationRestrictsLegacyChoices(t *testing.T) {
	for _, value := range []string{"Split Hi-Wall", "Inverter", "Cassete", "Piso Teto", "Janela", "Multi Split", "Portátil"} {
		if !validCustomerApplianceType(value) {
			t.Errorf("valid appliance type %q rejected", value)
		}
	}
	for _, value := range []string{"split", "", "Outro tipo"} {
		if validCustomerApplianceType(value) {
			t.Errorf("invalid appliance type %q accepted", value)
		}
	}
	for _, value := range []int{7000, 9000, 12000, 18000, 24000, 30000, 36000, 48000, 60000} {
		if !validCustomerApplianceBTUs(value) {
			t.Errorf("valid capacity %d rejected", value)
		}
	}
	for _, value := range []int{0, 10000, 99999} {
		if validCustomerApplianceBTUs(value) {
			t.Errorf("invalid capacity %d accepted", value)
		}
	}
}

func TestCustomerProfileUpdateDerivesOwnedRowAndFiltersFields(t *testing.T) {
	var patched map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer profile-token" {
			t.Errorf("request did not use caller JWT: %s", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"profile-user"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"profile-user","tipo":"CLIENTE"}]`)
		case "/rest/v1/customers":
			if r.Method == http.MethodGet {
				if r.URL.Query().Get("profile_id") != "eq.profile-user" || r.URL.Query().Get("limit") != "2" {
					t.Errorf("customer lookup not identity-scoped: %s", r.URL)
				}
				_, _ = io.WriteString(w, `[{"id":"owned-customer"}]`)
				return
			}
			if r.Method != http.MethodPatch || r.URL.Query().Get("id") != "eq.owned-customer" {
				t.Errorf("unexpected mutation target: %s %s", r.Method, r.URL)
			}
			if err := json.NewDecoder(r.Body).Decode(&patched); err != nil {
				t.Fatal(err)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected Supabase request: %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	handler := CustomerProfileHandler{Supabase: accountTestClient(t, server)}
	request := httptest.NewRequest(http.MethodPatch, "/api/cliente/perfil", strings.NewReader(`{"whatsapp":" 27999999999 ","endereco":" Rua A ","numero":"12","bairro":"Centro","cidade":"Vitória","profile_id":"other","ativo":false,"id":"other"}`))
	request.Header.Set("Authorization", "Bearer profile-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status/body=%d %s", response.Code, response.Body)
	}
	if len(patched) != 5 || patched["whatsapp"] != "27999999999" || patched["endereco"] != "Rua A" || patched["cidade"] != "Vitória" || patched["ativo"] != nil || patched["profile_id"] != nil || patched["id"] != nil {
		t.Fatalf("unexpected/unsafe profile patch: %#v", patched)
	}
}

func TestCustomerProfileRejectsTeamAndWrongMethod(t *testing.T) {
	reads := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"team-user"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"team-user","tipo":"TECNICO"}]`)
		default:
			reads = true
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	handler := CustomerProfileHandler{Supabase: accountTestClient(t, server)}
	request := httptest.NewRequest(http.MethodPatch, "/api/cliente/perfil", strings.NewReader(`{"cidade":"Vitória"}`))
	request.Header.Set("Authorization", "Bearer team-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || reads {
		t.Fatalf("team update status=%d extraReads=%v", response.Code, reads)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/cliente/perfil", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status=%d", response.Code)
	}
}
