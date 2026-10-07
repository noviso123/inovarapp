package googlecalendar

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"inovarapp/core/adapter/supabase"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

func TestEventPayloadKeepsLegacySummaryAndBrasiliaTime(t *testing.T) {
	got, err := EventPayload(serviceRow{ID: "os-1", Description: "Limpeza de Ar", Status: "AGENDADO", ScheduledDate: "2026-10-03", ScheduledTime: "14:30"}, customerRow{Name: "João", WhatsApp: "2799", Address: "Rua A", Neighborhood: "Centro", City: "Vitória"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Summary != "Inovar: Limpeza de Ar — João" || got.Location != "Rua A, Centro, Vitória" || got.Start.DateTime != "2026-10-03T17:30:00Z" || got.End.DateTime != "2026-10-03T19:30:00Z" || got.ExtendedProperties.Private.ServiceID != "os-1" {
		t.Fatalf("event payload=%#v", got)
	}
	if EventID("os-1") != EventID("os-1") || !strings.HasPrefix(EventID("os-1"), "inovar") {
		t.Fatalf("event ID=%s", EventID("os-1"))
	}
}

func TestCalendarHandlerSyncsThroughCallerJWTAndStoresConnection(t *testing.T) {
	const userID = "11111111-1111-4111-8111-111111111111"
	const serviceRole = "server-role"
	var storageEnvelope []byte
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			if r.Header.Get("Authorization") != "Bearer user-token" {
				t.Errorf("auth user token=%q", r.Header.Get("Authorization"))
			}
			_, _ = io.WriteString(w, `{"id":"`+userID+`"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"`+userID+`","tipo":"TECNICO"}]`)
		case "/rest/v1/services":
			if r.Header.Get("Authorization") != "Bearer user-token" {
				t.Errorf("services must use caller JWT: %q", r.Header.Get("Authorization"))
			}
			_, _ = io.WriteString(w, `[{"id":"service-1","cliente_id":"client-1","tipo":"LIMPEZA","descricao":"Limpeza de Ar","status":"AGENDADO","data_agendamento":"2026-10-05","hora_agendamento":"10:00"}]`)
		case "/rest/v1/customers":
			if r.Header.Get("Authorization") != "Bearer user-token" {
				t.Errorf("customers must use caller JWT: %q", r.Header.Get("Authorization"))
			}
			_, _ = io.WriteString(w, `[{"id":"client-1","nome":"João","whatsapp":"27999","endereco":"Rua A","bairro":"Centro","cidade":"Vitória"}]`)
		case "/storage/v1/object/documentos-inovar/config/google/" + userID + ".json":
			if r.Header.Get("Authorization") != "Bearer "+serviceRole {
				t.Errorf("storage must use server role")
			}
			mu.Lock()
			defer mu.Unlock()
			if r.Method == http.MethodGet {
				if len(storageEnvelope) == 0 {
					http.NotFound(w, r)
					return
				}
				_, _ = w.Write(storageEnvelope)
				return
			}
			if r.Method == http.MethodPost {
				storageEnvelope, _ = io.ReadAll(r.Body)
				w.WriteHeader(http.StatusOK)
				return
			}
			t.Errorf("unexpected Storage method %s", r.Method)
		default:
			t.Errorf("unexpected Supabase request %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon-key", ServiceRoleKey: serviceRole})
	if err != nil {
		t.Fatal(err)
	}
	connection := Connection{AccessToken: "google-token", ExpiresAt: time.Now().Add(time.Hour).UnixMilli(), Events: map[string]string{}}
	if err := client.StoreGoogleConnection(t.Context(), userID, connection); err != nil {
		t.Fatal(err)
	}
	var googleCalls []string
	google := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		googleCalls = append(googleCalls, r.Method+" "+r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer google-token" {
			t.Errorf("google bearer=%q", r.Header.Get("Authorization"))
		}
		if r.Method == http.MethodPut {
			return &http.Response{StatusCode: http.StatusNotFound, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
		}
		if r.Method == http.MethodPost {
			var event map[string]any
			if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
				t.Fatal(err)
			}
			if event["id"] != EventID("service-1") || event["summary"] != "Inovar: Limpeza de Ar — João" {
				t.Errorf("Google event=%#v", event)
			}
			return &http.Response{StatusCode: http.StatusCreated, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
		}
		t.Errorf("unexpected Google method %s", r.Method)
		return &http.Response{StatusCode: http.StatusBadRequest, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})
	handler := Handler{Supabase: client, HTTP: google}
	request := httptest.NewRequest(http.MethodPost, "/api/google-calendar", strings.NewReader(`{"action":"sync"}`))
	request.Header.Set("Authorization", "Bearer user-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var result SyncResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Connected || result.Changed != 1 || result.LastSync == "" {
		t.Fatalf("sync result=%#v", result)
	}
	if strings.Join(googleCalls, ",") != "PUT /calendar/v3/calendars/primary/events/"+EventID("service-1")+",POST /calendar/v3/calendars/primary/events" {
		t.Fatalf("Google calls=%v", googleCalls)
	}
	mu.Lock()
	saved := append([]byte(nil), storageEnvelope...)
	mu.Unlock()
	var envelope struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(saved, &envelope); err != nil || envelope.Data == "" {
		t.Fatalf("updated encrypted connection missing: %s %v", saved, err)
	}
}

func TestCalendarConnectConfirmsLinkedGoogleIdentityBeforeStoring(t *testing.T) {
	const userID = "11111111-1111-4111-8111-111111111111"
	const serviceRole = "server-role"
	var saved []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"`+userID+`","identities":[{"provider":"google","identity_data":{"sub":"google-42"}}]}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"`+userID+`","tipo":"ADMIN"}]`)
		case "/storage/v1/object/documentos-inovar/config/google/" + userID + ".json":
			if r.Method == http.MethodGet {
				http.NotFound(w, r)
				return
			}
			saved, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected Supabase request %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon-key", ServiceRoleKey: serviceRole})
	if err != nil {
		t.Fatal(err)
	}
	google := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer provider-token" {
			t.Errorf("Google bearer token=%q", r.Header.Get("Authorization"))
		}
		if r.URL.Path == "/userinfo" {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"sub":"google-42"}`))}, nil
		}
		if r.URL.Path == "/calendar/v3/calendars/primary/events" && r.URL.Query().Get("maxResults") == "1" {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"items":[]}`))}, nil
		}
		t.Errorf("unexpected Google request %s", r.URL.String())
		return &http.Response{StatusCode: http.StatusNotFound, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})
	handler := Handler{Supabase: client, HTTP: google, IdentityAPI: "https://accounts.test/userinfo", GoogleAPI: "https://accounts.test/calendar/v3"}
	request := httptest.NewRequest(http.MethodPost, "/api/google-calendar", strings.NewReader(`{"action":"connect","providerToken":"provider-token","providerRefreshToken":"provider-refresh"}`))
	request.Header.Set("Authorization", "Bearer user-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || len(saved) == 0 || strings.Contains(string(saved), "provider-token") || strings.Contains(string(saved), "provider-refresh") {
		t.Fatalf("connect status=%d response=%s encryptedStorage=%s", response.Code, response.Body.String(), saved)
	}
}
