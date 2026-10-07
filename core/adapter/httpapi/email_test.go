package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mailadapter "inovarapp/core/adapter/email"
)

type emailSenderFunc func(context.Context, mailadapter.Config, mailadapter.Message) (string, error)

func (f emailSenderFunc) Send(ctx context.Context, cfg mailadapter.Config, msg mailadapter.Message) (string, error) {
	return f(ctx, cfg, msg)
}

func TestEmailHandlerSendsUTF8HTMLUsingStoredCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"user-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"TECNICO"}]`)
		case technicianSettingsPath:
			_, _ = io.WriteString(w, `{"email_gmail_user":"inovar@example.com","email_gmail_pass":"stored-secret"}`)
		default:
			t.Errorf("unexpected request %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	var sent mailadapter.Message
	var config mailadapter.Config
	handler := EmailHandler{Supabase: accountTestClient(t, server), Sender: emailSenderFunc(func(_ context.Context, c mailadapter.Config, m mailadapter.Message) (string, error) {
		sent = m
		config = c
		return "", nil
	})}
	req := httptest.NewRequest(http.MethodPost, "/api/email", strings.NewReader(`{"acao":"enviar","para":"cliente@example.com","assunto":"Orçamento — João","html":"<p>Olá, João! ❄️</p>"}`))
	req.Header.Set("Authorization", "Bearer user-token")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	var result map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if res.Code != http.StatusOK || result["ok"] != true || result["canal"] != "gmail" {
		t.Fatalf("status=%d result=%#v", res.Code, result)
	}
	if sent.To != "cliente@example.com" || sent.Subject != "Orçamento — João" || sent.HTML != "<p>Olá, João! ❄️</p>" || config.Username != "inovar@example.com" || config.Password != "stored-secret" {
		t.Fatalf("sent=%#v config=%#v", sent, config)
	}
}

func TestEmailHandlerRejectsCustomerAndInvalidInput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"user-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"CLIENTE"}]`)
		default:
			t.Errorf("unexpected request %s", r.URL.String())
		}
	}))
	defer server.Close()
	handler := EmailHandler{Supabase: accountTestClient(t, server), Defaults: mailadapter.Config{Username: "inovar@example.com", Password: "secret"}, Sender: emailSenderFunc(func(context.Context, mailadapter.Config, mailadapter.Message) (string, error) {
		t.Fatal("sender called")
		return "", nil
	})}
	for _, body := range []string{`{"acao":"enviar","para":"cliente@example.com","assunto":"x","html":"<p>x</p>"}`, `{"acao":"desconhecida"}`} {
		req := httptest.NewRequest(http.MethodPost, "/api/email", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer user-token")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusForbidden {
			t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
		}
	}
}

func TestRenderEmailTestEscapesCustomerName(t *testing.T) {
	body, err := renderEmailTest(`<script>alert(1)</script>`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, "<script>") || !strings.Contains(body, "&lt;script&gt;") {
		t.Fatalf("unsafe HTML template output: %s", body)
	}
}

func TestEmailHandlerTestUsesRequestedRecipientAndLegacyFallback(t *testing.T) {
	for _, test := range []struct {
		name, body, wantTo, wantGreeting string
	}{
		{name: "authenticated account recipient", body: `{"acao":"testar","para":"tecnico@example.com","nome":"João Silva"}`, wantTo: "tecnico@example.com", wantGreeting: "Olá, João!"},
		{name: "legacy default recipient", body: `{"acao":"testar"}`, wantTo: "jhonatan.satiro1@gmail.com", wantGreeting: "Olá, Jhonatan!"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/auth/v1/user":
					_, _ = io.WriteString(w, `{"id":"user-1"}`)
				case "/rest/v1/profiles":
					_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"TECNICO"}]`)
				case technicianSettingsPath:
					_, _ = io.WriteString(w, `{}`)
				default:
					t.Errorf("unexpected request %s", r.URL.String())
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)
			var sent mailadapter.Message
			handler := EmailHandler{Supabase: accountTestClient(t, server), Defaults: mailadapter.Config{Username: "inovar@example.com", Password: "secret"}, Sender: emailSenderFunc(func(_ context.Context, _ mailadapter.Config, message mailadapter.Message) (string, error) {
				sent = message
				return "", nil
			})}
			req := httptest.NewRequest(http.MethodPost, "/api/email", strings.NewReader(test.body))
			req.Header.Set("Authorization", "Bearer user-token")
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)
			if res.Code != http.StatusOK || !strings.Contains(sent.HTML, test.wantGreeting) {
				t.Fatalf("status=%d body=%s message=%#v", res.Code, res.Body.String(), sent)
			}
			if sent.To != test.wantTo {
				t.Fatalf("recipient=%q want %q", sent.To, test.wantTo)
			}
		})
	}
}

func TestEmailHandlerTestReturnsFailureMessageInClientContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"user-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"TECNICO"}]`)
		case technicianSettingsPath:
			_, _ = io.WriteString(w, `{}`)
		default:
			t.Errorf("unexpected request %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	handler := EmailHandler{Supabase: accountTestClient(t, server), Defaults: mailadapter.Config{Username: "inovar@example.com", Password: "secret"}, Sender: emailSenderFunc(func(context.Context, mailadapter.Config, mailadapter.Message) (string, error) {
		return "", context.DeadlineExceeded
	})}
	req := httptest.NewRequest(http.MethodPost, "/api/email", strings.NewReader(`{"acao":"testar","para":"tecnico@example.com","nome":"João"}`))
	req.Header.Set("Authorization", "Bearer user-token")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	var result map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if res.Code != http.StatusOK || result["ok"] != false || !strings.Contains(result["mensagem"].(string), "Falha ao enviar o teste:") {
		t.Fatalf("status=%d result=%#v", res.Code, result)
	}
	if result["error"] != nil {
		t.Fatalf("legacy error property should be normalized to mensagem: %#v", result)
	}
}
