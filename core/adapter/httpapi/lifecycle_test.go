package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServiceHistoryInsertKeepsTeamJWTAndFiltersPayload(t *testing.T) {
	var saved map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"user-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"TECNICO"}]`)
		case "/rest/v1/service_history":
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer user-token" || r.Header.Get("Prefer") != "return=representation" {
				t.Errorf("unexpected history request: %s auth=%q prefer=%q", r.Method, r.Header.Get("Authorization"), r.Header.Get("Prefer"))
			}
			if err := json.NewDecoder(r.Body).Decode(&saved); err != nil {
				t.Fatal(err)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"history-1"}`)
		default:
			t.Errorf("unexpected request %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	handler := ServiceHistoryHandler{Supabase: accountTestClient(t, server)}
	request := httptest.NewRequest(http.MethodPost, "/api/historico", strings.NewReader(`{"service_id":"svc-1","cliente_id":"client-1","data":"2026-10-03","observacoes":"{\"filtrosLavados\":true}","service_role":true}`))
	request.Header.Set("Authorization", "Bearer user-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if saved["service_id"] != "svc-1" || saved["cliente_id"] != "client-1" || saved["service_role"] != nil {
		t.Fatalf("history payload=%#v", saved)
	}
}

func TestRetroactiveHistoryCreatesCompletedServiceWithoutExtraFields(t *testing.T) {
	var saved map[string]any
	var queued map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveQueueTestRequest(t, w, r, &queued) {
			return
		}
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"user-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"TECNICO"}]`)
		case "/rest/v1/services":
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer user-token" || r.Header.Get("Prefer") != "return=representation" {
				t.Errorf("unexpected retroactive service request: method=%s auth=%q prefer=%q", r.Method, r.Header.Get("Authorization"), r.Header.Get("Prefer"))
			}
			if err := json.NewDecoder(r.Body).Decode(&saved); err != nil {
				t.Fatal(err)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"service-1"}`)
		default:
			t.Errorf("unexpected request %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	handler := ServiceHistoryHandler{Supabase: accountTestClient(t, server)}
	request := httptest.NewRequest(http.MethodPost, "/api/historico?retroativo=1", strings.NewReader(`{"cliente_id":"client-1","aparelho_id":"device-1","tipo":"LIMPEZA","status":"CONCLUIDO","valor":250,"data_agendamento":"2026-04-03","descricao":"Limpeza de Ar","observacoes":"Histórico anterior registrado manualmente.","is_admin":true}`))
	request.Header.Set("Authorization", "Bearer user-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || saved["status"] != "CONCLUIDO" || saved["tipo"] != "LIMPEZA" || saved["is_admin"] != nil {
		t.Fatalf("status=%d body=%s payload=%#v", response.Code, response.Body.String(), saved)
	}
}

func TestCompletedServiceHistoryIncludesStatusForReturnCycleDisplay(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"user-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"TECNICO"}]`)
		case "/rest/v1/services":
			if r.URL.Query().Get("status") != "eq.CONCLUIDO" || !strings.Contains(r.URL.Query().Get("select"), "status") {
				t.Errorf("completed history query omits status: %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `[]`)
		default:
			t.Errorf("unexpected request %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	request := httptest.NewRequest(http.MethodGet, "/api/historico?origem=services", nil)
	request.Header.Set("Authorization", "Bearer user-token")
	response := httptest.NewRecorder()
	(ServiceHistoryHandler{Supabase: accountTestClient(t, server)}).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestApplianceMaintenanceOnlyUpdatesAllowedFieldForTeam(t *testing.T) {
	var saved map[string]any
	var gotPath string
	var queued map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveQueueTestRequest(t, w, r, &queued) {
			return
		}
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"user-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"ADMIN"}]`)
		case "/rest/v1/air_conditioners":
			gotPath = r.URL.RawQuery
			if r.Method != http.MethodPatch {
				t.Errorf("method=%s", r.Method)
			}
			if err := json.NewDecoder(r.Body).Decode(&saved); err != nil {
				t.Fatal(err)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	handler := ApplianceMaintenanceHandler{Supabase: accountTestClient(t, server)}
	request := httptest.NewRequest(http.MethodPatch, "/api/aparelho-manutencao", strings.NewReader(`{"id":"appliance-1","fields":{"ultima_manutencao":"2026-10-03","marca":"não alterar"}}`))
	request.Header.Set("Authorization", "Bearer user-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || gotPath != "id=eq.appliance-1" || saved["ultima_manutencao"] != "2026-10-03" || saved["marca"] != nil {
		t.Fatalf("status=%d path=%q payload=%#v", response.Code, gotPath, saved)
	}
}
