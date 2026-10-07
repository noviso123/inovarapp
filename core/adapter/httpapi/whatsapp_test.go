package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"inovarapp/core/adapter/whatsapp"
)

func TestWhatsAppHandlerQueuesPrivatePDFForWorker(t *testing.T) {
	var queued map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveQueueTestRequest(t, w, r, &queued) {
			return
		}
		switch r.URL.Path {
		case "/auth/v1/user":
			io.WriteString(w, `{"id":"user-1"}`)
		case "/rest/v1/profiles":
			io.WriteString(w, `[{"id":"user-1","tipo":"TECNICO"}]`)
		default:
			t.Errorf("unexpected request: %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	provider := &captureWhatsAppDoer{}
	handler := WhatsAppHandler{Supabase: accountTestClient(t, server), Sender: whatsapp.Sender{HTTPClient: provider}, Defaults: whatsapp.Config{ProprioURL: server.URL, ProprioToken: "service-key", ProprioSession: "inovar"}}
	input, _ := json.Marshal(map[string]string{"acao": "enviar", "telefone": "(27) 99999-1234", "texto": "Olá, João!", "documento_url": server.URL + "/storage/v1/object/sign/documentos-inovar/os-orcamentos/os.pdf?token=abc", "documento_nome": "OS_João.pdf"})
	request := httptest.NewRequest(http.MethodPost, "/api/whatsapp", strings.NewReader(string(input)))
	request.Header.Set("Authorization", "Bearer user-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var result map[string]any
	json.Unmarshal(response.Body.Bytes(), &result)
	if response.Code != http.StatusAccepted || result["enfileirado"] != true || result["status"] != "pendente" {
		t.Fatalf("status=%d result=%#v", response.Code, result)
	}
	if queued["recipient_phone"] != "5527999991234" || queued["message_text"] != "Olá, João!" || queued["document_name"] != "OS_João.pdf" || queued["document_storage_path"] != "os-orcamentos/os.pdf" || queued["document_url"] != nil {
		t.Fatalf("queued private PDF=%#v", queued)
	}
	if provider.multipart || provider.body != nil {
		t.Fatal("delivery must be deferred to the worker")
	}
}

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(request *http.Request) (*http.Response, error) { return f(request) }

// roundTripperFunc adapta uma função a http.RoundTripper para redirecionar
// hosts externos nos testes.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestWhatsAppHandlerRejectsCustomer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"user-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"CLIENTE"}]`)
		default:
			t.Errorf("unexpected request: %s", r.URL.String())
		}
	}))
	defer server.Close()
	handler := WhatsAppHandler{Supabase: accountTestClient(t, server)}
	request := httptest.NewRequest(http.MethodPost, "/api/whatsapp", strings.NewReader(`{"acao":"enviar","telefone":"27999991234","documento_url":"https://storage.example/os.pdf"}`))
	request.Header.Set("Authorization", "Bearer user-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
