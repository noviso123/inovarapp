package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"inovarapp/core/adapter/supabase"
	"inovarapp/core/adapter/whatsapp"
)

func TestPublicAccountCreationConfirmsUserAndUpdatesTriggeredCustomer(t *testing.T) {
	var createdBody map[string]any
	var customerUpdated map[string]any
	var queued map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveQueueTestRequest(t, w, r, &queued) {
			return
		}
		if r.URL.Path == "/auth/v1/admin/users" {
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer server-secret" {
				t.Errorf("create request = %s %s auth=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
			}
			if r.Header.Get("X-Supabase-Api-Version") == "" {
				t.Error("missing auth API version")
			}
			if err := json.NewDecoder(r.Body).Decode(&createdBody); err != nil {
				t.Fatal(err)
			}
			_, _ = io.WriteString(w, `{"id":"new-user","email":"cliente@example.com"}`)
			return
		}
		if r.URL.Path == "/rest/v1/customers" {
			if r.Method == http.MethodGet {
				if r.URL.Query().Get("profile_id") != "eq.new-user" {
					t.Errorf("customer query = %s", r.URL.RawQuery)
				}
				_, _ = io.WriteString(w, `[{"id":"trigger-customer"}]`)
				return
			}
			if r.Method == http.MethodPatch {
				if r.URL.Query().Get("id") != "eq.trigger-customer" {
					t.Errorf("customer patch query = %s", r.URL.RawQuery)
				}
				if err := json.NewDecoder(r.Body).Decode(&customerUpdated); err != nil {
					t.Fatal(err)
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		if r.URL.Path == technicianConfigStoragePath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
		http.NotFound(w, r)
	}))
	defer server.Close()

	handler := AccountsHandler{Supabase: accountTestClient(t, server)}
	request := httptest.NewRequest(http.MethodPost, "/api/contas", strings.NewReader(`{
"acao":"minha_conta","email":"  Cliente@Example.com ","senha":"segura123","nome":"João Silva","whatsapp":"(27) 99999-1234","endereco":"Rua A","bairro":"Centro","cidade":"Vitória"
}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if createdBody["email"] != "cliente@example.com" || createdBody["email_confirm"] != true || createdBody["password"] != "segura123" {
		t.Fatalf("admin create body = %#v", createdBody)
	}
	metadata := createdBody["user_metadata"].(map[string]any)
	if metadata["nome"] != "João Silva" || metadata["telefone"] != "(27) 99999-1234" || metadata["must_change_password"] != false {
		t.Fatalf("user metadata = %#v", metadata)
	}
	if customerUpdated["whatsapp"] != "(27) 99999-1234" || customerUpdated["endereco"] != "Rua A" || customerUpdated["bairro"] != "Centro" || customerUpdated["cidade"] != "Vitória" {
		t.Fatalf("triggered customer update = %#v", customerUpdated)
	}
	var result accountResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.UserID != "new-user" || result.Email != "cliente@example.com" || result.Message != "Conta criada! Entrando no seu portal..." {
		t.Fatalf("account result = %#v", result)
	}
}

func TestPublicAccountCreationSendsConfiguredWelcomeMessage(t *testing.T) {
	var patched map[string]any
	var queued map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveQueueTestRequest(t, w, r, &queued) {
			return
		}
		switch r.URL.Path {
		case "/auth/v1/admin/users":
			_, _ = io.WriteString(w, `{"id":"new-user","email":"cliente@example.com"}`)
		case "/rest/v1/customers":
			if r.Method == http.MethodGet {
				_, _ = io.WriteString(w, `[{"id":"trigger-customer"}]`)
				return
			}
			if err := json.NewDecoder(r.Body).Decode(&patched); err != nil {
				t.Fatal(err)
			}
			w.WriteHeader(http.StatusNoContent)
		case technicianSettingsPath:
			_, _ = io.WriteString(w, `{"mensagensWhats":{"boas_vindas_autocadastro":"Olá {{cliente}} — acesse {{app}}"}}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	provider := &captureWhatsAppDoer{}
	handler := AccountsHandler{
		Supabase:         accountTestClient(t, server),
		WhatsApp:         whatsapp.Sender{HTTPClient: provider},
		WhatsAppDefaults: whatsapp.Config{ProprioURL: "https://whatsapp.invalid", ProprioToken: "test-token", ProprioSession: "inovar"},
	}
	request := httptest.NewRequest(http.MethodPost, "/api/contas", strings.NewReader(`{"acao":"minha_conta","email":"cliente@example.com","senha":"segura123","nome":"João Silva","whatsapp":"(27) 99999-1234"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var result accountResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || !result.OK || result.WhatsAppEnfileirado == nil || !*result.WhatsAppEnfileirado {
		t.Fatalf("signup response=%d %#v", response.Code, result)
	}
	if queued["recipient_phone"] != "5527999991234" || queued["message_text"] != "Olá João — acesse https://inovarapp.vercel.app" {
		t.Fatalf("welcome WhatsApp payload=%#v", provider.body)
	}
	if patched["whatsapp"] != "(27) 99999-1234" {
		t.Fatalf("triggered customer was not updated before messaging: %#v", patched)
	}
}

func TestTechnicianAccountRequiresValidatedTeamRoleAndLinksCustomer(t *testing.T) {
	var createdBody map[string]any
	var linked bool
	var deletedTrigger bool
	var queued map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveQueueTestRequest(t, w, r, &queued) {
			return
		}
		switch r.URL.Path {
		case "/auth/v1/user":
			if r.Header.Get("Authorization") != "Bearer team-token" {
				t.Errorf("auth header = %q", r.Header.Get("Authorization"))
			}
			_, _ = io.WriteString(w, `{"id":"tech-1"}`)
		case "/rest/v1/profiles":
			if r.URL.Query().Get("id") != "eq.tech-1" || r.Header.Get("Authorization") != "Bearer team-token" {
				t.Errorf("team profile query = %s", r.URL.String())
			}
			_, _ = io.WriteString(w, `[{"id":"tech-1","tipo":"TECNICO"}]`)
		case "/auth/v1/admin/users":
			if err := json.NewDecoder(r.Body).Decode(&createdBody); err != nil {
				t.Fatal(err)
			}
			_, _ = io.WriteString(w, `{"id":"new-client","email":"cliente@example.com"}`)
		case "/rest/v1/customers":
			if r.Method == http.MethodDelete {
				deletedTrigger = r.URL.Query().Get("profile_id") == "eq.new-client" && r.URL.Query().Get("nome") == "eq.João Silva"
				w.WriteHeader(http.StatusNoContent)
				return
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			linked = r.URL.Query().Get("id") == "eq.customer-7" && body["profile_id"] == "new-client"
			w.WriteHeader(http.StatusNoContent)
		case technicianConfigStoragePath:
			_, _ = io.WriteString(w, `{"mensagensWhats":{"boas_vindas_tecnico":"Olá {{cliente}}. Login {{email}}. Senha {{senha}}"}}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider := &captureWhatsAppDoer{}
	handler := AccountsHandler{
		Supabase:         accountTestClient(t, server),
		WhatsApp:         whatsapp.Sender{HTTPClient: provider},
		WhatsAppDefaults: whatsapp.Config{ProprioURL: "https://whatsapp.invalid", ProprioToken: "test-token", ProprioSession: "inovar"},
	}
	request := httptest.NewRequest(http.MethodPost, "/api/contas", strings.NewReader(`{"acao":"do_tecnico","email":"Cliente@Example.com","nome":"João Silva","telefone":"27999991234","customer_id":"customer-7"}`))
	request.Header.Set("Authorization", "Bearer team-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if !deletedTrigger || !linked {
		t.Fatalf("trigger cleanup=%v linked=%v", deletedTrigger, linked)
	}
	if createdBody["email"] != "cliente@example.com" || createdBody["password"] != "123456" || createdBody["email_confirm"] != true {
		t.Fatalf("admin create body = %#v", createdBody)
	}
	metadata := createdBody["user_metadata"].(map[string]any)
	if metadata["must_change_password"] != true {
		t.Fatalf("metadata = %#v", metadata)
	}
	var result accountResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Password != "123456" || result.UserID != "new-client" || result.Email != "cliente@example.com" || result.WhatsAppEnfileirado == nil || !*result.WhatsAppEnfileirado {
		t.Fatalf("response body = %#v", result)
	}
	if queued["recipient_phone"] != "5527999991234" || queued["message_text"] != "Olá João. Login cliente@example.com. Senha 123456" {
		t.Fatalf("technician-created account WhatsApp payload=%#v", provider.body)
	}
	if queued["sensitive"] != true || queued["expires_at"] == nil {
		t.Fatalf("credentials must expire: %#v", queued)
	}
}

func TestAccountTeamActionsRejectCustomersAndMissingSessions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/v1/user" {
			_, _ = io.WriteString(w, `{"id":"customer-1"}`)
			return
		}
		if r.URL.Path == "/rest/v1/profiles" {
			_, _ = io.WriteString(w, `[{"id":"customer-1","tipo":"CLIENTE"}]`)
			return
		}
		t.Errorf("unexpected request %s", r.URL.String())
		http.NotFound(w, r)
	}))
	defer server.Close()
	handler := AccountsHandler{Supabase: accountTestClient(t, server)}

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodPost, "/api/contas", strings.NewReader(`{"acao":"redefinir_cliente","customer_id":"client-1"}`)))
	if missing.Code != http.StatusUnauthorized || !strings.Contains(missing.Body.String(), "Não autenticado") {
		t.Fatalf("missing session response = %d %s", missing.Code, missing.Body.String())
	}

	customer := httptest.NewRequest(http.MethodPost, "/api/contas", strings.NewReader(`{"acao":"do_tecnico","email":"cliente@example.com"}`))
	customer.Header.Set("Authorization", "Bearer valid-customer-token")
	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, customer)
	if denied.Code != http.StatusForbidden || !strings.Contains(denied.Body.String(), "Somente técnicos e administradores") {
		t.Fatalf("customer response = %d %s", denied.Code, denied.Body.String())
	}
}

func TestResetCustomerPasswordPreservesMetadataAndForcesChange(t *testing.T) {
	var updatedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"tech-1"}`)
		case "/rest/v1/profiles":
			if r.URL.Query().Get("id") == "eq.tech-1" {
				_, _ = io.WriteString(w, `[{"id":"tech-1","tipo":"ADMIN"}]`)
				return
			}
			if r.URL.Query().Get("id") == "eq.client-1" {
				_, _ = io.WriteString(w, `[{"id":"client-1","email":"cliente@example.com","tipo":"CLIENTE"}]`)
				return
			}
			t.Errorf("unexpected profile query %s", r.URL.String())
		case "/rest/v1/customers":
			_, _ = io.WriteString(w, `[{"id":"customer-1","nome":"Maria Souza","whatsapp":"","profile_id":"client-1"}]`)
		case "/auth/v1/admin/users/client-1":
			if r.Method == http.MethodGet {
				_, _ = io.WriteString(w, `{"id":"client-1","user_metadata":{"nome":"Maria Souza","custom":"preservado","must_change_password":false}}`)
				return
			}
			if err := json.NewDecoder(r.Body).Decode(&updatedBody); err != nil {
				t.Fatal(err)
			}
			_, _ = io.WriteString(w, `{"id":"client-1","email":"cliente@example.com"}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	handler := AccountsHandler{Supabase: accountTestClient(t, server)}
	request := httptest.NewRequest(http.MethodPost, "/api/contas", strings.NewReader(`{"acao":"redefinir_cliente","customer_id":"customer-1"}`))
	request.Header.Set("Authorization", "Bearer team-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	if updatedBody["password"] == nil {
		t.Fatalf("missing temporary password in update: %#v", updatedBody)
	}
	password := updatedBody["password"].(string)
	if len(password) != 10 || !regexp.MustCompile(`[A-Z]`).MatchString(password) || !regexp.MustCompile(`[a-z]`).MatchString(password) || !regexp.MustCompile(`[2-9]`).MatchString(password) || !regexp.MustCompile(`[!@#]`).MatchString(password) {
		t.Fatalf("temporary password does not follow the existing format: %q", password)
	}
	metadata := updatedBody["user_metadata"].(map[string]any)
	if metadata["must_change_password"] != true || metadata["custom"] != "preservado" || metadata["nome"] != "Maria Souza" {
		t.Fatalf("updated metadata = %#v", metadata)
	}
	var result accountResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.TemporaryPassword != password || result.Email != "cliente@example.com" || result.Message != "Senha temporária criada. O cliente deverá alterá-la no próximo acesso." {
		t.Fatalf("response body = %#v", result)
	}
}

func TestAccountsValidationAndDuplicateEmailContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"code":"email_exists","msg":"User already registered"}`)
	}))
	defer server.Close()
	handler := AccountsHandler{Supabase: accountTestClient(t, server)}

	for _, input := range []string{
		`{"acao":"minha_conta","email":"bad","senha":"123456","nome":"Cliente"}`,
		`{"acao":"minha_conta","email":"cliente@example.com","senha":"12345","nome":"Cliente"}`,
		`{"acao":"minha_conta","email":"cliente@example.com","senha":"123456","nome":" "}`,
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/contas", strings.NewReader(input)))
		if response.Code != http.StatusBadRequest {
			t.Errorf("validation response = %d %s", response.Code, response.Body.String())
		}
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/contas", strings.NewReader(`{"acao":"minha_conta","email":"cliente@example.com","senha":"123456","nome":"Cliente"}`)))
	var result accountResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusConflict || !result.AlreadyExists || result.Error != "Este e-mail já possui conta. Faça login normalmente." {
		t.Fatalf("duplicate response = %d %#v", response.Code, result)
	}
}

func accountTestClient(t *testing.T, server *httptest.Server) *supabase.Client {
	t.Helper()
	client, err := supabase.New(supabase.Config{
		URL: server.URL, AnonKey: "public-anon", ServiceRoleKey: "server-secret", HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}
