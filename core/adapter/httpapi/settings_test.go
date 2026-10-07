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

func TestSettingsGetMasksCredentialsAndHidesCalendarTokenFromCustomers(t *testing.T) {
	stored := `{"businessName":"Inovar","phone":"27998279185","assinatura":"/inovar-brand/INOVAR_SIGNATURE_GABRIEL.png","whatsapp_proprio_url":"https://evolution.example","email_gmail_user":"inovar@example.com","whatsapp_proprio_token":"evolution-secret","email_api_key":"mail-secret","email_gmail_pass":"gmail-secret","calendario_token":"calendar-secret"}`
	server := settingsTestServer(t, `CLIENTE`, stored, nil)
	defer server.Close()
	handler := settingsTestHandler(t, server)
	req := httptest.NewRequest(http.MethodGet, "/api/configuracoes", nil)
	req.Header.Set("Authorization", "Bearer settings-user-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status/body = %d %s", response.Code, response.Body)
	}
	body := response.Body.String()
	for _, secret := range []string{"evolution-secret", "mail-secret", "gmail-secret", "calendar-secret"} {
		if strings.Contains(body, secret) {
			t.Errorf("response leaked %q: %s", secret, body)
		}
	}
	var decoded settingsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.OK || len(decoded.Config) != 2 || string(decoded.Config["businessName"]) != `"Inovar"` || string(decoded.Config["phone"]) != `"27998279185"` {
		t.Fatalf("customer-safe response = %#v", decoded.Config)
	}
	for _, key := range []string{"whatsapp_proprio_token", "email_api_key", "email_gmail_pass", "calendario_token", "whatsapp_proprio_url", "email_gmail_user"} {
		if _, exists := decoded.Config[key]; exists {
			t.Errorf("customer response exposed internal setting %q", key)
		}
	}
}

func TestSettingsPostRemovesLegacyWhatsAppCredentialsAndMergesCustomTypes(t *testing.T) {
	stored := `{"name":"Antigo","whatsapp_proprio_url":"https://evolution.example","whatsapp_proprio_token":"evolution-secret","email_api_key":"mail-secret","email_gmail_pass":"gmail-secret","calendario_token":"calendar-secret","tiposServicosCustom":[{"nome":"Antigo","preco":10}]}`
	var saved map[string]json.RawMessage
	server := settingsTestServer(t, `TECNICO`, stored, &saved)
	defer server.Close()
	handler := settingsTestHandler(t, server)
	handler.EmailAPIKey = "environment-mail-secret"
	req := httptest.NewRequest(http.MethodPost, "/api/configuracoes", strings.NewReader(`{"name":"Atualizado","whatsapp_proprio_url":"","whatsapp_proprio_token":"***configurada***","email_api_key":"","email_gmail_pass":"***configurada***","tipos_servicos_custom":[{"nome":"Novo","preco":25}]}`))
	req.Header.Set("Authorization", "Bearer settings-user-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status/body = %d %s", response.Code, response.Body)
	}
	if string(saved["name"]) != `"Atualizado"` || string(saved["email_api_key"]) != `"mail-secret"` || string(saved["email_gmail_pass"]) != `"gmail-secret"` {
		t.Fatalf("settings fields were not merged safely: %s", marshalSettings(saved))
	}
	for _, key := range []string{"whatsapp_proprio_url", "whatsapp_proprio_token", "whatsapp_proprio_session", "whatsapp_evol_api_key", "whatsapp_meta_token", "whatsapp_phone_id"} {
		if _, exists := saved[key]; exists {
			t.Errorf("legacy WhatsApp setting %q persisted: %s", key, marshalSettings(saved))
		}
	}
	if string(saved["calendario_token"]) != `"calendar-secret"` {
		t.Fatalf("calendar credential was not preserved: %s", marshalSettings(saved))
	}
	var custom []map[string]any
	if err := json.Unmarshal(saved["tiposServicosCustom"], &custom); err != nil || len(custom) != 1 || custom[0]["nome"] != "Novo" {
		t.Fatalf("legacy custom types alias not merged: %s", saved["tiposServicosCustom"])
	}
	if strings.Contains(response.Body.String(), "mail-secret") || strings.Contains(response.Body.String(), "gmail-secret") {
		t.Fatalf("POST response leaked credentials: %s", response.Body)
	}
}

func TestSettingsPostRejectsCustomerBeforeStorageAccess(t *testing.T) {
	storageTouched := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"settings-user"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"settings-user","tipo":"CLIENTE"}]`)
		default:
			storageTouched = true
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	handler := settingsTestHandler(t, server)
	req := httptest.NewRequest(http.MethodPost, "/api/configuracoes", strings.NewReader(`{"phone":"attacker"}`))
	req.Header.Set("Authorization", "Bearer settings-user-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusForbidden || storageTouched {
		t.Fatalf("status=%d storageTouched=%v body=%s", response.Code, storageTouched, response.Body)
	}
}

func TestSettingsReadFailureNeverOverwritesStoredConfigWithDefaults(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		t.Run(method, func(t *testing.T) {
			storageWrites := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/auth/v1/user":
					_, _ = io.WriteString(w, `{"id":"settings-user"}`)
				case "/rest/v1/profiles":
					_, _ = io.WriteString(w, `[{"id":"settings-user","tipo":"TECNICO"}]`)
				case technicianSettingsPath:
					if r.Method == http.MethodGet {
						http.Error(w, `{"message":"temporary storage outage"}`, http.StatusServiceUnavailable)
						return
					}
					storageWrites++
					w.WriteHeader(http.StatusOK)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			handler := settingsTestHandler(t, server)
			var body io.Reader
			if method == http.MethodPost {
				body = strings.NewReader(`{"name":"Alterado"}`)
			}
			request := httptest.NewRequest(method, "/api/configuracoes", body)
			request.Header.Set("Authorization", "Bearer settings-user-token")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusInternalServerError {
				t.Fatalf("status/body = %d %s", response.Code, response.Body)
			}
			if storageWrites != 0 {
				t.Fatalf("temporary read failure caused %d writes", storageWrites)
			}
		})
	}
}

func settingsTestHandler(t *testing.T, server *httptest.Server) SettingsHandler {
	t.Helper()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon-key", ServiceRoleKey: "service-role", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	return SettingsHandler{Supabase: client}
}

func settingsTestServer(t *testing.T, role, stored string, saved *map[string]json.RawMessage) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"settings-user"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"settings-user","tipo":"`+role+`"}]`)
		case technicianSettingsPath:
			if r.Method == http.MethodGet {
				if stored == "" {
					http.NotFound(w, r)
					return
				}
				_, _ = io.WriteString(w, stored)
				return
			}
			if r.Method == http.MethodPost {
				if r.Header.Get("X-Upsert") != "true" || r.Header.Get("Authorization") != "Bearer service-role" {
					t.Errorf("storage update headers = %#v", r.Header)
				}
				if err := json.NewDecoder(r.Body).Decode(saved); err != nil {
					t.Fatal(err)
				}
				w.WriteHeader(http.StatusOK)
				return
			}
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
}

func marshalSettings(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
