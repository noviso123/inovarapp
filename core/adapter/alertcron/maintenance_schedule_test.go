package alertcron

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"inovarapp/core/adapter/supabase"
)

func TestFutureMaintenanceIsPersistedAtNineInBrasilia(t *testing.T) {
	var queued map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/v1/whatsapp_message_queue" {
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		if r.Method == http.MethodPatch {
			if r.URL.Query().Get("metadata->>proxima_manutencao") != "neq.2027-01-07" {
				t.Errorf("wrong superseded cycle filter: %s", r.URL)
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Query().Get("on_conflict") != "idempotency_key" || r.Header.Get("Prefer") != "resolution=ignore-duplicates,return=minimal" {
			t.Error("missing duplicate prevention")
		}
		_ = json.NewDecoder(r.Body).Decode(&queued)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	db, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "service"})
	if err != nil {
		t.Fatal(err)
	}
	h := Handler{Supabase: db}
	now := time.Date(2026, 10, 7, 14, 0, 0, 0, time.UTC)
	err = h.syncMaintenanceRows(context.Background(), now,
		[]customerRow{{ID: "customer", Name: "João", WhatsApp: "27999991234"}},
		[]applianceRow{{ID: "appliance", CustomerID: "customer", Brand: "LG"}},
		[]historyRow{{CustomerID: "customer", ApplianceID: "appliance", Date: "2026-10-07"}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if queued["scheduled_at"] != "2027-01-07T12:00:00Z" || queued["next_attempt_at"] != queued["scheduled_at"] {
		t.Fatalf("wrong schedule: %#v", queued)
	}
	if queued["status"] != "pendente" || queued["idempotency_key"] != "manutencao:customer:appliance:2027-01-07:0" {
		t.Fatalf("wrong pending cycle: %#v", queued)
	}
	metadata := queued["metadata"].(map[string]any)
	if metadata["ultima_manutencao"] != "2026-10-07" || metadata["intervalo_meses"] != float64(3) {
		t.Fatalf("wrong maintenance context: %#v", metadata)
	}
}
