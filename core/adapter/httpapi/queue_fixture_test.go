package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Emulate only the durable queue endpoint; provider delivery belongs to the
// worker. The HTTP flows must persist work before reporting it as queued.
func serveQueueTestRequest(t *testing.T, w http.ResponseWriter, r *http.Request, captured *map[string]any) bool {
	t.Helper()
	if r.URL.Path != "/rest/v1/whatsapp_message_queue" {
		return false
	}
	if r.Method == http.MethodPatch {
		if r.URL.Query().Get("status") != "eq.pendente" || r.URL.Query().Get("source_entity_id") == "" {
			t.Errorf("queue cancellation must target pending messages for an entity: %s", r.URL)
		}
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	if r.Method != http.MethodPost {
		t.Errorf("unexpected queue method %s", r.Method)
	}
	var message map[string]any
	if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
		t.Error(err)
	}
	phone, _ := message["recipient_phone"].(string)
	if message["idempotency_key"] == "" || message["status"] != "pendente" || !strings.HasPrefix(phone, "55") {
		t.Errorf("invalid persisted queue message: %#v", message)
	}
	if captured != nil {
		*captured = message
	}
	w.WriteHeader(http.StatusCreated)
	return true
}
