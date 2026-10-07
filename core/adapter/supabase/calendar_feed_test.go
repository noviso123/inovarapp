package supabase

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCalendarFeedICSLoadsOnlyAfterTokenValidation(t *testing.T) {
	var serviceQuerySeen bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer service-secret" {
			t.Errorf("request did not use server service role: %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case calendarFeedConfigPath:
			_, _ = io.WriteString(w, `{"calendario_token":"feed-secret"}`)
		case "/rest/v1/services":
			serviceQuerySeen = true
			if got := r.URL.Query().Get("status"); got != `in.("AGENDADO","EM_ANDAMENTO")` {
				t.Errorf("status filter = %q", got)
			}
			if got := r.URL.Query().Get("limit"); got != "500" {
				t.Errorf("limit = %q", got)
			}
			_, _ = io.WriteString(w, `[
{"id":"svc-1","tipo":"LIMPEZA","descricao":"Limpeza; técnica, básica\nsegunda linha","status":"AGENDADO","data_agendamento":"2026-10-03","hora_agendamento":"09:30","valor":210,"cliente_id":"cli-1"},
{"id":"svc-no-date","tipo":"OUTRO","status":"AGENDADO","data_agendamento":"","cliente_id":"cli-404"}
]`)
		case "/rest/v1/customers":
			_, _ = io.WriteString(w, `[{"id":"cli-1","nome":"João, Jr.","endereco":"Rua A; 10","bairro":"Centro","cidade":"Vitória","whatsapp":"27999990000"}]`)
		default:
			t.Errorf("unexpected Supabase path: %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := testClient(t, server.URL, "public-anon", "service-secret")

	if _, err := client.CalendarFeedICS(context.Background(), "wrong-secret", time.Now()); err != ErrCalendarFeedNotFound {
		t.Fatalf("wrong token error = %v", err)
	}
	if serviceQuerySeen {
		t.Fatal("service rows were queried before feed token validation")
	}

	generatedAt := time.Date(2026, 10, 3, 13, 0, 0, 0, time.FixedZone("BRT", -3*60*60))
	ics, err := client.CalendarFeedICS(context.Background(), "feed-secret", generatedAt)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"DTSTAMP:20261003T160000Z",
		"DTSTART:20261003T123000Z",
		"DTEND:20261003T143000Z",
		"SUMMARY:Limpeza\\; técnica\\, básica\\nsegunda linha — João\\, Jr.",
		"DESCRIPTION:Serviço Inovar Refrigeração (AGENDADO). Valor: R$ 210.00. Contato: 27999990000",
		"LOCATION:Rua A\\; 10\\, Centro\\, Vitória",
	} {
		if !strings.Contains(ics, want) {
			t.Errorf("feed missing %q in:\n%s", want, ics)
		}
	}
	if strings.Contains(ics, "svc-no-date") {
		t.Fatal("service without a scheduled date was included")
	}
}

func TestServeCalendarFeedPreservesMethodAndNotFoundResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"calendario_token":"feed-secret"}`)
	}))
	defer server.Close()
	client := testClient(t, server.URL, "public-anon", "service-secret")

	methodRecorder := httptest.NewRecorder()
	client.ServeCalendarFeed(methodRecorder, httptest.NewRequest(http.MethodPost, "/api/calendario-ics", nil))
	if methodRecorder.Code != http.StatusMethodNotAllowed || !strings.Contains(methodRecorder.Body.String(), "Método não permitido") {
		t.Fatalf("method response = %d %s", methodRecorder.Code, methodRecorder.Body.String())
	}

	notFoundRecorder := httptest.NewRecorder()
	client.ServeCalendarFeed(notFoundRecorder, httptest.NewRequest(http.MethodGet, "/api/calendario-ics?token=wrong", nil))
	if notFoundRecorder.Code != http.StatusNotFound || !strings.Contains(notFoundRecorder.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("not-found response = %d %s", notFoundRecorder.Code, notFoundRecorder.Body.String())
	}
	if notFoundRecorder.Header().Get("Cache-Control") != "no-store, max-age=0" {
		t.Fatalf("cache policy = %q", notFoundRecorder.Header().Get("Cache-Control"))
	}
}
