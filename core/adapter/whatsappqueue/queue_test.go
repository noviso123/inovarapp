package whatsappqueue

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"inovarapp/core/adapter/supabase"
	"inovarapp/core/adapter/whatsapp"
)

func TestProcessDueWaitsForConnectionWithoutClaimingMessages(t *testing.T) {
	claimed := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == tablePath:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions/inovar":
			_, _ = w.Write([]byte(`{"status":"disconnected","connected":false}`))
		case r.Method == http.MethodPost && r.URL.Path == "/rest/v1/rpc/claim_whatsapp_message_queue":
			claimed = true
			t.Error("worker claimed messages while WhatsApp was disconnected")
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions/inovar/messages":
			t.Error("worker sent a message while WhatsApp was disconnected")
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	db := testQueueClient(t, server)
	result, err := ProcessDue(context.Background(), db, whatsapp.Sender{HTTPClient: server.Client()}, whatsapp.Config{
		ProprioURL: server.URL, ProprioToken: "test-token", ProprioSession: "inovar",
	}, time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC), 1)
	if err != nil {
		t.Fatal(err)
	}
	if !result.WaitingForConnection || result.Claimed != 0 || result.Sent != 0 || claimed {
		t.Fatalf("disconnected result=%+v claimed=%t", result, claimed)
	}
}

func TestProcessDueRecordsProviderAcceptedMessageAsSent(t *testing.T) {
	var finish map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == tablePath:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions/inovar":
			_, _ = w.Write([]byte(`{"status":"connected","connected":true}`))
		case r.Method == http.MethodPost && r.URL.Path == "/rest/v1/rpc/claim_whatsapp_message_queue":
			_, _ = w.Write([]byte(`[{"id":"00000000-0000-4000-8000-000000000001","idempotency_key":"welcome:customer-1","event_type":"boas_vindas_autocadastro","recipient_phone":"5527999991234","message_text":"Olá, João!","scheduled_at":"2026-10-07T12:00:00Z","next_attempt_at":"2026-10-07T12:00:00Z","status":"processando","attempt_count":1,"created_at":"2026-10-07T12:00:00Z"}]`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions/inovar/messages":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["to"] != "5527999991234@s.whatsapp.net" || body["text"] != "Olá, João!" {
				t.Errorf("provider payload=%#v", body)
			}
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodPost && r.URL.Path == "/rest/v1/rpc/finish_whatsapp_message_attempt":
			if err := json.NewDecoder(r.Body).Decode(&finish); err != nil {
				t.Fatal(err)
			}
			_, _ = w.Write([]byte(`"enviado"`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	db := testQueueClient(t, server)
	result, err := ProcessDue(context.Background(), db, whatsapp.Sender{HTTPClient: server.Client()}, whatsapp.Config{
		ProprioURL: server.URL, ProprioToken: "test-token", ProprioSession: "inovar",
	}, time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC), 1)
	if err != nil {
		t.Fatal(err)
	}
	if result.Claimed != 1 || result.Sent != 1 || result.Retried != 0 {
		t.Fatalf("unexpected processing result: %+v", result)
	}
	if finish["p_attempt_status"] != "enviado" || finish["p_attempt_number"] != float64(1) || finish["p_message_id"] != "00000000-0000-4000-8000-000000000001" {
		t.Fatalf("finish RPC payload=%#v", finish)
	}
}

func TestProcessDueSchedulesRetryAfterProviderFailure(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	var finish map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/rest/v1/rpc/validate_whatsapp_reminder":
			_, _ = w.Write([]byte(`true`))
		case r.Method == http.MethodPatch && r.URL.Path == tablePath:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions/inovar":
			_, _ = w.Write([]byte(`{"status":"connected","connected":true}`))
		case r.Method == http.MethodPost && r.URL.Path == "/rest/v1/rpc/claim_whatsapp_message_queue":
			_, _ = w.Write([]byte(`[{"id":"00000000-0000-4000-8000-000000000002","idempotency_key":"reminder:customer-1","event_type":"lembrete_manutencao_recorrente","recipient_phone":"5527999991234","message_text":"Agende sua manutenção","scheduled_at":"2026-10-07T12:00:00Z","next_attempt_at":"2026-10-07T12:00:00Z","status":"processando","attempt_count":2,"created_at":"2026-10-07T12:00:00Z"}]`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions/inovar/messages":
			http.Error(w, "temporary provider failure", http.StatusServiceUnavailable)
		case r.Method == http.MethodPost && r.URL.Path == "/rest/v1/rpc/finish_whatsapp_message_attempt":
			if err := json.NewDecoder(r.Body).Decode(&finish); err != nil {
				t.Fatal(err)
			}
			_, _ = w.Write([]byte(`"pendente"`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	db := testQueueClient(t, server)
	result, err := ProcessDue(context.Background(), db, whatsapp.Sender{HTTPClient: server.Client()}, whatsapp.Config{
		ProprioURL: server.URL, ProprioToken: "test-token", ProprioSession: "inovar",
	}, now, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result.Claimed != 1 || result.Retried != 1 || result.Sent != 0 {
		t.Fatalf("unexpected processing result: %+v", result)
	}
	if finish["p_attempt_status"] != "falha" || finish["p_retry_at"] != now.Add(2*time.Minute).Format(time.RFC3339) {
		t.Fatalf("finish RPC payload=%#v, expected second retry at %s", finish, now.Add(2*time.Minute).Format(time.RFC3339))
	}
	if strings.Contains(finish["p_error_message"].(string), "temporary provider failure") {
		t.Fatal("provider response body should not be persisted into queue errors")
	}
}

func testQueueClient(t *testing.T, server *httptest.Server) *supabase.Client {
	t.Helper()
	client, err := supabase.New(supabase.Config{
		URL: server.URL, AnonKey: "test-anon-key", ServiceRoleKey: "test-service-role", HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}
