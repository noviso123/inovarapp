package whatsappqueue

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"inovarapp/core/adapter/supabase"
)

func TestQueueListExplainsMissingSupabaseMigrationWithoutLeakingResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != tablePath {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"PGRST205","details":"internal schema diagnostic"}`))
	}))
	defer server.Close()

	response := httptest.NewRecorder()
	queueHandlerWithServiceRole(t, server).list(response, context.Background())
	var problem struct {
		Error      string `json:"error"`
		Stage      string `json:"etapa"`
		HTTPStatus int    `json:"http_status"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusServiceUnavailable || problem.Stage != "mensagens_pendentes" || problem.HTTPStatus != http.StatusNotFound {
		t.Fatalf("status=%d problem=%+v", response.Code, problem)
	}
	if !strings.Contains(problem.Error, "migration") && !strings.Contains(problem.Error, "migrações") {
		t.Fatalf("missing-migration guidance not shown: %q", problem.Error)
	}
	if strings.Contains(response.Body.String(), "internal schema diagnostic") || strings.Contains(response.Body.String(), "PGRST205") {
		t.Fatal("raw Supabase response leaked into the authenticated UI error")
	}
}

func TestQueueListIdentifiesSupabaseCredentialMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == tablePath {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		if r.URL.Path == "/rest/v1/rpc/whatsapp_message_queue_counts" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"private database error"}`))
			return
		}
		t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
		http.NotFound(w, r)
	}))
	defer server.Close()

	response := httptest.NewRecorder()
	queueHandlerWithServiceRole(t, server).list(response, context.Background())
	var problem map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusServiceUnavailable || problem["etapa"] != "contagens_fila" {
		t.Fatalf("status=%d problem=%#v", response.Code, problem)
	}
	if !strings.Contains(problem["error"].(string), "SUPABASE_SERVICE_ROLE_KEY") {
		t.Fatalf("credential guidance not shown: %#v", problem)
	}
	if strings.Contains(response.Body.String(), "private database error") {
		t.Fatal("raw Supabase response leaked into the authenticated UI error")
	}
}

func TestQueueListDoesNotSilentlyHideAttemptHistoryFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == tablePath && r.URL.Query().Get("status") == "in.(pendente,processando)":
			_, _ = w.Write([]byte(`[{"id":"00000000-0000-4000-8000-000000000001","event_type":"boas_vindas_autocadastro","recipient_phone":"5527999991234","message_text":"Olá","scheduled_at":"2026-10-07T12:00:00Z","next_attempt_at":"2026-10-07T12:00:00Z","status":"pendente","created_at":"2026-10-07T12:00:00Z"}]`))
		case r.Method == http.MethodGet && r.URL.Path == tablePath:
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/rest/v1/rpc/whatsapp_message_queue_counts":
			_, _ = w.Write([]byte(`{"pendente":1}`))
		case r.Method == http.MethodGet && r.URL.Path == "/rest/v1/whatsapp_message_attempts":
			w.WriteHeader(http.StatusNotFound)
		case r.URL.Path == "/rest/v1/whatsapp_message_attempts":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	response := httptest.NewRecorder()
	queueHandlerWithServiceRole(t, server).list(response, context.Background())
	var problem map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusServiceUnavailable || problem["etapa"] != "tentativas" {
		t.Fatalf("status=%d problem=%#v", response.Code, problem)
	}
}

func queueHandlerWithServiceRole(t *testing.T, server *httptest.Server) HTTPHandler {
	t.Helper()
	db, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "test-anon-key", ServiceRoleKey: "test-service-role", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	return HTTPHandler{Supabase: db}
}

