package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"inovarapp/core/adapter/supabase"
	"inovarapp/core/adapter/whatsapp"
)

func TestWhatsAppInstanceStatusUsesServerConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"user-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"ADMIN"}]`)
		case "/v1/sessions/inovar":
			if r.Header.Get("Authorization") != "Bearer service-secret" {
				t.Errorf("authorization=%q", r.Header.Get("Authorization"))
			}
			_, _ = io.WriteString(w, `{"status":"connected"}`)
		default:
			t.Errorf("unexpected request: %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	response := callWhatsAppAction(t, accountTestClient(t, server), `{"acao":"status"}`, whatsapp.Config{ProprioURL: server.URL, ProprioToken: "service-secret", ProprioSession: "inovar"})
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || result["conectado"] != true || result["estado"] != "open" || result["instancia"] != "inovar" {
		t.Fatalf("status=%d body=%v", response.Code, result)
	}
}

func TestWhatsAppPhonePairingReturnsCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"user-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"ADMIN"}]`)
		case "/v1/sessions/inovar/connect":
			_, _ = io.WriteString(w, `{"status":"pairing"}`)
		case "/v1/sessions/inovar/pair":
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload["phone"] != "5527999991234" {
				t.Errorf("pair payload=%v err=%v", payload, err)
			}
			_, _ = io.WriteString(w, `{"code":"ABCD-1234"}`)
		default:
			t.Errorf("unexpected request: %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	response := callWhatsAppAction(t, accountTestClient(t, server), `{"acao":"conectar","telefone":"(27) 99999-1234"}`, whatsapp.Config{ProprioURL: server.URL, ProprioToken: "service-secret", ProprioSession: "inovar"})
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || result["pairingCode"] != "ABCD-1234" || result["estado"] != "connecting" {
		t.Fatalf("status=%d body=%v", response.Code, result)
	}
}

func TestWhatsAppPhonePairingTimeoutReturnsActionableMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"user-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"ADMIN"}]`)
		case "/v1/sessions/inovar/connect":
			_, _ = io.WriteString(w, `{"status":"pairing"}`)
		case "/v1/sessions/inovar/pair":
			w.WriteHeader(http.StatusGatewayTimeout)
			_, _ = io.WriteString(w, `{"error":{"code":"pairing_timeout","message":"not ready"}}`)
		default:
			t.Errorf("unexpected request: %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	response := callWhatsAppAction(t, accountTestClient(t, server), `{"acao":"conectar","telefone":"27999991234"}`, whatsapp.Config{ProprioURL: server.URL, ProprioToken: "service-secret", ProprioSession: "inovar"})
	if response.Code != http.StatusGatewayTimeout || !strings.Contains(response.Body.String(), "preparando o pareamento") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestWhatsAppStatusRefreshesPhoneCodeWithoutReturningFakeQR(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"user-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"TECNICO"}]`)
		case "/v1/sessions/inovar":
			_, _ = io.WriteString(w, `{"status":"pairing","pairing_method":"phone"}`)
		case "/v1/sessions/inovar/pairing":
			if r.URL.Query().Has("format") {
				t.Errorf("phone code must not be requested as an image: %s", r.URL.String())
			}
			_, _ = io.WriteString(w, `{"code":"ABCD-1234","method":"phone"}`)
		default:
			t.Errorf("unexpected request: %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	response := callWhatsAppAction(t, accountTestClient(t, server), `{"acao":"status"}`, whatsapp.Config{ProprioURL: server.URL, ProprioToken: "service-secret", ProprioSession: "inovar"})
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || result["pairingCode"] != "ABCD-1234" || result["qr"] != nil || response.Header().Get("Cache-Control") == "" {
		t.Fatalf("status=%d body=%v headers=%v", response.Code, result, response.Header())
	}
}

func TestWhatsAppConnectReturnsQRAndSessionState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"user-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"TECNICO"}]`)
		case "/v1/sessions/inovar/connect":
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer service-secret" {
				t.Errorf("connect method=%s authorization=%q", r.Method, r.Header.Get("Authorization"))
			}
			_, _ = io.WriteString(w, `{"status":"connecting"}`)
		case "/v1/sessions/inovar/pairing":
			if r.URL.Query().Get("format") != "png" {
				t.Errorf("pairing format = %q", r.URL.Query().Get("format"))
			}
			_, _ = w.Write([]byte("FAKEPNG"))
		default:
			t.Errorf("unexpected request: %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	response := callWhatsAppAction(t, accountTestClient(t, server), `{"acao":"conectar"}`, whatsapp.Config{ProprioURL: server.URL, ProprioToken: "service-secret", ProprioSession: "inovar"})
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	esperado := base64.StdEncoding.EncodeToString([]byte("FAKEPNG"))
	if response.Code != http.StatusOK || result["qr"] != "data:image/png;base64,"+esperado || result["estado"] != "connecting" {
		t.Fatalf("status=%d body=%v", response.Code, result)
	}
}

func TestWhatsAppConnectWithoutReadyQRReturnsWaitState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"user-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"ADMIN"}]`)
		case "/v1/sessions/inovar/connect":
			_, _ = io.WriteString(w, `{"status":"connecting"}`)
		case "/v1/sessions/inovar/pairing":
			http.NotFound(w, r)
		default:
			t.Errorf("unexpected request: %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	response := callWhatsAppAction(t, accountTestClient(t, server), `{"acao":"conectar"}`, whatsapp.Config{ProprioURL: server.URL, ProprioToken: "service-secret", ProprioSession: "inovar"})
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || result["qr"] != nil || result["configurado"] != true || !strings.Contains(stringValue(result["mensagem"]), "aguardando o QR Code") {
		t.Fatalf("status=%d body=%v", response.Code, result)
	}
}

func TestWhatsAppConnectCreatesMissingPersistentSession(t *testing.T) {
	created := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"user-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"ADMIN"}]`)
		case "/v1/sessions/inovar/connect":
			if !created {
				http.NotFound(w, r)
				return
			}
			_, _ = io.WriteString(w, `{"status":"pairing"}`)
		case "/v1/sessions":
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload["id"] != "inovar" {
				t.Errorf("create payload=%v err=%v", payload, err)
			}
			created = true
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"inovar","status":"disconnected"}`)
		case "/v1/sessions/inovar/pairing":
			_, _ = w.Write([]byte("QR"))
		default:
			t.Errorf("unexpected request: %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	response := callWhatsAppAction(t, accountTestClient(t, server), `{"acao":"conectar"}`, whatsapp.Config{ProprioURL: server.URL, ProprioToken: "service-secret", ProprioSession: "inovar"})
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || !created || result["qr"] != "data:image/png;base64,UVI=" {
		t.Fatalf("status=%d created=%v body=%v", response.Code, created, result)
	}
	if response.Header().Get("Cache-Control") != "no-store, max-age=0" {
		t.Fatalf("QR response cache policy = %q", response.Header().Get("Cache-Control"))
	}
}

func TestWhatsAppDisconnectSupportsLogoutAndDelete(t *testing.T) {
	for _, mode := range []string{"logout", "apagar"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/auth/v1/user":
					_, _ = io.WriteString(w, `{"id":"user-1"}`)
				case "/rest/v1/profiles":
					_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"ADMIN"}]`)
				case "/v1/sessions/inovar/disconnect":
					if mode != "logout" || r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer service-secret" {
						t.Errorf("disconnect method=%s authorization=%q", r.Method, r.Header.Get("Authorization"))
					}
					_, _ = io.WriteString(w, `{"status":"ok"}`)
				case "/v1/sessions/inovar":
					if mode != "apagar" || r.Method != http.MethodDelete || r.Header.Get("Authorization") != "Bearer service-secret" {
						t.Errorf("delete method=%s authorization=%q", r.Method, r.Header.Get("Authorization"))
					}
					_, _ = io.WriteString(w, `{"status":"ok"}`)
				default:
					t.Errorf("unexpected request: %s", r.URL.String())
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			body, _ := json.Marshal(map[string]string{"acao": "desconectar", "modo": mode})
			response := callWhatsAppAction(t, accountTestClient(t, server), string(body), whatsapp.Config{ProprioURL: server.URL, ProprioToken: "service-secret", ProprioSession: "inovar"})
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"ok":true`) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestWhatsAppInstanceActionsRejectCustomer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"user-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"CLIENTE"}]`)
		default:
			t.Errorf("unexpected downstream request: %s", r.URL.String())
		}
	}))
	defer server.Close()
	response := callWhatsAppAction(t, accountTestClient(t, server), `{"acao":"status"}`, whatsapp.Config{ProprioURL: server.URL, ProprioToken: "service-secret", ProprioSession: "inovar"})
	if response.Code != http.StatusForbidden {
		t.Fatalf("customer status=%d", response.Code)
	}
}

func TestWhatsAppDisconnectRejectsUnknownMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"user-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"ADMIN"}]`)
		default:
			t.Errorf("invalid mode must not reach the WhatsApp system: %s", r.URL.String())
		}
	}))
	defer server.Close()
	response := callWhatsAppAction(t, accountTestClient(t, server), `{"acao":"desconectar","modo":"reset"}`, whatsapp.Config{})
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "logout") {
		t.Fatalf("invalid mode status=%d body=%s", response.Code, response.Body.String())
	}
}

func callWhatsAppAction(t *testing.T, client *supabase.Client, body string, config whatsapp.Config) *httptest.ResponseRecorder {
	t.Helper()
	handler := WhatsAppHandler{Supabase: client, Defaults: config}
	request := httptest.NewRequest(http.MethodPost, "/api/whatsapp", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer user-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
