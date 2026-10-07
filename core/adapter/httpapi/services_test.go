package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServicesHandlerUsesCallerJWTAndPreservesLegacyResourceContract(t *testing.T) {
	var method, rawQuery, prefer string
	var authHeader, apiKey string
	var saved map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"user-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"TECNICO"}]`)
		case "/rest/v1/services":
			method, rawQuery, prefer = r.Method, r.URL.RawQuery, r.Header.Get("Prefer")
			authHeader, apiKey = r.Header.Get("Authorization"), r.Header.Get("apikey")
			if err := json.NewDecoder(r.Body).Decode(&saved); err != nil {
				t.Fatal(err)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"service-1","tipo":"LIMPEZA"}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	handler := ServicesHandler{Supabase: accountTestClient(t, server)}
	request := httptest.NewRequest(http.MethodPost, "/api/servicos", strings.NewReader(`{"id":"550e8400-e29b-41d4-a716-446655440000","cliente_id":"client-1","tipo":"LIMPEZA","status":"PENDENTE","script":"ignored"}`))
	request.Header.Set("Authorization", "Bearer user-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if method != http.MethodPost || prefer != "return=representation" || authHeader != "Bearer user-token" || apiKey != "public-anon" {
		t.Fatalf("request method=%s prefer=%q auth=%q apikey=%q", method, prefer, authHeader, apiKey)
	}
	if saved["id"] != "550e8400-e29b-41d4-a716-446655440000" || saved["cliente_id"] != "client-1" || saved["tipo"] != "LIMPEZA" || saved["script"] != nil {
		t.Fatalf("saved payload=%#v", saved)
	}
	if rawQuery != "" {
		t.Fatalf("insert query=%s", rawQuery)
	}
}

func TestServiceCreateRejectsInvalidStableID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"user-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"TECNICO"}]`)
		default:
			t.Errorf("unexpected upstream request %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/servicos", strings.NewReader(`{"id":"not-a-uuid","status":"AGENDADO"}`))
	req.Header.Set("Authorization", "Bearer user-token")
	res := httptest.NewRecorder()
	(ServicesHandler{Supabase: accountTestClient(t, server)}).ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestServiceAndAppointmentMutationsFilterFieldsAndUseResourceKeys(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"user-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"TECNICO"}]`)
		case "/rest/v1/services", "/rest/v1/appointments":
			calls++
			if r.URL.Path == "/rest/v1/services" && r.URL.Query().Get("id") != "eq.service/1" {
				t.Errorf("service query=%s", r.URL.RawQuery)
			}
			if r.URL.Path == "/rest/v1/appointments" && r.URL.Query().Get("service_id") != "eq.service-1" {
				t.Errorf("appointment query=%s", r.URL.RawQuery)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if _, ok := body["is_admin"]; ok {
				t.Errorf("unapproved field forwarded: %#v", body)
			}
			if r.URL.Path == "/rest/v1/services" && body["status"] != "AGENDADO" {
				t.Errorf("service body=%#v", body)
			}
			if r.URL.Path == "/rest/v1/appointments" && body["hora"] != "10:30" {
				t.Errorf("appointment body=%#v", body)
			}
			_, _ = io.WriteString(w, `[{"ok":true}]`)
		default:
			t.Errorf("unexpected request %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := accountTestClient(t, server)
	for _, tc := range []struct {
		h    http.Handler
		body string
	}{
		{ServicesHandler{Supabase: client}, `{"id":"service/1","fields":{"status":"AGENDADO","is_admin":true}}`},
		{AppointmentsHandler{Supabase: client}, `{"service_id":"service-1","fields":{"data":"2026-10-05","hora":"10:30","is_admin":true}}`},
	} {
		req := httptest.NewRequest(http.MethodPatch, "/api/resource", strings.NewReader(tc.body))
		req.Header.Set("Authorization", "Bearer user-token")
		res := httptest.NewRecorder()
		tc.h.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
		}
	}
	if calls != 2 {
		t.Fatalf("mutation calls=%d", calls)
	}
}

func TestServiceCancellationForwardsLifecycleColumnsUnderUserJWT(t *testing.T) {
	var forwarded map[string]any
	var queued map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveQueueTestRequest(t, w, r, &queued) {
			return
		}
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"tech-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"tech-1","tipo":"TECNICO"}]`)
		case "/rest/v1/services":
			if r.Method != http.MethodPatch || r.URL.Query().Get("id") != "eq.service-1" || r.Header.Get("Authorization") != "Bearer tech-token" {
				t.Errorf("unexpected request: %s %s auth=%q", r.Method, r.URL.String(), r.Header.Get("Authorization"))
			}
			if err := json.NewDecoder(r.Body).Decode(&forwarded); err != nil {
				t.Fatal(err)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	req := httptest.NewRequest(http.MethodPatch, "/api/servicos", strings.NewReader(`{"id":"service-1","fields":{"status":"CANCELADO","data_cancelamento":"2026-10-03","motivo_cancelamento":"Cancelado pelo técnico","admin":true}}`))
	req.Header.Set("Authorization", "Bearer tech-token")
	res := httptest.NewRecorder()
	(ServicesHandler{Supabase: accountTestClient(t, server)}).ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if forwarded["status"] != "CANCELADO" || forwarded["data_cancelamento"] != "2026-10-03" || forwarded["motivo_cancelamento"] != "Cancelado pelo técnico" || forwarded["admin"] != nil {
		t.Fatalf("forwarded cancellation=%#v", forwarded)
	}
}

func TestResourceHandlersRejectUnauthenticatedRequestsAndUnknownOnlyFields(t *testing.T) {
	unauthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/v1/user" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		t.Errorf("unexpected unauthenticated request %s", r.URL.String())
	}))
	unauthHandler := ServicesHandler{Supabase: accountTestClient(t, unauthServer)}
	unauthRequest := httptest.NewRequest(http.MethodGet, "/api/servicos", nil)
	unauthResponse := httptest.NewRecorder()
	unauthHandler.ServeHTTP(unauthResponse, unauthRequest)
	unauthServer.Close()
	if unauthResponse.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d", unauthResponse.Code)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"user-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"TECNICO"}]`)
		default:
			t.Errorf("unexpected request %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	h := ServicesHandler{Supabase: accountTestClient(t, server)}
	req := httptest.NewRequest(http.MethodPost, "/api/servicos", strings.NewReader(`{"script":"ignored"}`))
	req.Header.Set("Authorization", "Bearer user-token")
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("unknown fields status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestCustomerServiceRequestUsesLinkedCustomerAndPendingStatus(t *testing.T) {
	var inserted map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"profile-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"profile-1","tipo":"CLIENTE"}]`)
		case "/rest/v1/customers":
			if r.URL.Query().Get("profile_id") != "eq.profile-1" {
				t.Errorf("customer profile query=%s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `[{"id":"customer-owned"}]`)
		case "/rest/v1/air_conditioners":
			if r.URL.Query().Get("cliente_id") != "eq.customer-owned" || r.URL.Query().Get("id") != "eq.appliance-owned" {
				t.Errorf("appliance ownership query=%s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `[{"id":"appliance-owned"}]`)
		case "/rest/v1/services":
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer customer-token" {
				t.Errorf("service insert headers %s %q", r.Method, r.Header.Get("Authorization"))
			}
			if err := json.NewDecoder(r.Body).Decode(&inserted); err != nil {
				t.Fatal(err)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"service-new"}`)
		default:
			t.Errorf("unexpected request %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	notifier := &recordingTeamNotifier{}
	handler := CustomerServicesHandler{Supabase: accountTestClient(t, server), Notifier: notifier}
	request := httptest.NewRequest(http.MethodPost, "/api/cliente/servicos", strings.NewReader(`{"cliente_id":"attacker-customer","aparelho_id":"appliance-owned","tipo":"LIMPEZA","status":"CONCLUIDO","problema":"Ar sem gelar"}`))
	request.Header.Set("Authorization", "Bearer customer-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if inserted["cliente_id"] != "customer-owned" || inserted["status"] != "PENDENTE" || inserted["tipo"] != "LIMPEZA" || inserted["status"] == "CONCLUIDO" {
		t.Fatalf("inserted request=%#v", inserted)
	}
	if len(notifier.notices) != 1 || notifier.notices[0] != (recordedPushNotice{title: "Novo chamado de cliente", body: "Um cliente solicitou atendimento — confira a Central de Atendimento."}) {
		t.Fatalf("team notices=%+v", notifier.notices)
	}
}

func TestCustomerServiceTypeAndDateValidation(t *testing.T) {
	for _, value := range []string{"LIMPEZA", "INSTALACAO", "MANUTENCAO_PREVENTIVA", "MANUTENCAO_CORRETIVA", "RECARGA_GAS", "AVALIACAO", "OUTRO"} {
		if !validCustomerServiceType(value) {
			t.Errorf("valid service type %q rejected", value)
		}
	}
	for _, value := range []string{"Limpeza de Ar", "HIGIENIZACAO", "", "CONCLUIDO"} {
		if validCustomerServiceType(value) {
			t.Errorf("invalid service type %q accepted", value)
		}
	}
	for _, value := range []string{"2026-10-03", "2024-02-29"} {
		if !validCustomerServiceDate(value) {
			t.Errorf("valid date %q rejected", value)
		}
	}
	for _, value := range []string{"2026-02-30", "03/10/2026", "2026-10-03T00:00:00Z"} {
		if validCustomerServiceDate(value) {
			t.Errorf("invalid date %q accepted", value)
		}
	}
}

func TestCustomerServiceRequestRejectsApplianceOwnedBySomeoneElse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"profile-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"profile-1","tipo":"CLIENTE"}]`)
		case "/rest/v1/customers":
			_, _ = io.WriteString(w, `[{"id":"customer-owned"}]`)
		case "/rest/v1/air_conditioners":
			_, _ = io.WriteString(w, " [ ]\n")
		default:
			t.Errorf("unexpected request %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	handler := CustomerServicesHandler{Supabase: accountTestClient(t, server)}
	request := httptest.NewRequest(http.MethodPost, "/api/cliente/servicos", strings.NewReader(`{"aparelho_id":"other-appliance","tipo":"LIMPEZA"}`))
	request.Header.Set("Authorization", "Bearer customer-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "não pertence") {
		t.Fatalf("response=%d %s", response.Code, response.Body.String())
	}
}

func TestCustomersCannotUseTeamServiceOrAppointmentMutations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"customer-profile"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"customer-profile","tipo":"CLIENTE"}]`)
		default:
			t.Errorf("mutation unexpectedly reached Supabase: %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := accountTestClient(t, server)
	for _, test := range []struct {
		handler      http.Handler
		method, body string
	}{
		{ServicesHandler{Supabase: client}, http.MethodPatch, `{"id":"service-1","fields":{"status":"CONCLUIDO"}}`},
		{AppointmentsHandler{Supabase: client}, http.MethodPost, `{"service_id":"service-1","cliente_id":"customer-1","data":"2026-10-05"}`},
	} {
		request := httptest.NewRequest(test.method, "/api/resource", strings.NewReader(test.body))
		request.Header.Set("Authorization", "Bearer customer-token")
		response := httptest.NewRecorder()
		test.handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s role restriction response=%d %s", test.method, response.Code, response.Body.String())
		}
	}
}

func TestTeamCanCreateAppointmentWithLegacyFields(t *testing.T) {
	var inserted map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"tech-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"tech-1","tipo":"TECNICO"}]`)
		case "/rest/v1/appointments":
			if r.Method != http.MethodPost || r.Header.Get("Prefer") != "return=representation" {
				t.Errorf("request=%s prefer=%q", r.Method, r.Header.Get("Prefer"))
			}
			if err := json.NewDecoder(r.Body).Decode(&inserted); err != nil {
				t.Fatal(err)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"appointment-1"}`)
		default:
			t.Errorf("unexpected request %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	handler := AppointmentsHandler{Supabase: accountTestClient(t, server)}
	request := httptest.NewRequest(http.MethodPost, "/api/agendamentos", strings.NewReader(`{"service_id":"service-1","cliente_id":"customer-1","data":"2026-10-05","hora":"09:30","status":"AGENDADO","observacoes":"Retorno"}`))
	request.Header.Set("Authorization", "Bearer team-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("response=%d %s", response.Code, response.Body.String())
	}
	if inserted["service_id"] != "service-1" || inserted["cliente_id"] != "customer-1" || inserted["data"] != "2026-10-05" || inserted["hora"] != "09:30" || inserted["status"] != "AGENDADO" || inserted["observacoes"] != "Retorno" {
		t.Fatalf("appointment payload=%#v", inserted)
	}
}
