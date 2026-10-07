package alertcron

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	mailadapter "inovarapp/core/adapter/email"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/adapter/webpush"
	"inovarapp/core/adapter/whatsapp"
)

func TestReadAllRowsReadsBeyondFirstPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if r.URL.Query().Get("limit") != "500" || r.URL.Query().Get("order") != "data.desc,id.asc" {
			t.Errorf("missing stable pagination: %s", r.URL.RawQuery)
		}
		rows := make([]map[string]any, 0)
		for i := offset; i < offset+500 && i < 1201; i++ {
			rows = append(rows, map[string]any{"id": fmt.Sprint(i)})
		}
		_ = json.NewEncoder(w).Encode(rows)
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "service-role"})
	if err != nil {
		t.Fatal(err)
	}
	h := Handler{Supabase: client}
	var rows []map[string]any
	if err := h.readAllRows(context.Background(), "/rest/v1/service_history?order=data.desc&limit=2000", &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1201 || rows[1200]["id"] != "1200" {
		t.Fatalf("incomplete rows: %d", len(rows))
	}
}

type mailFunc func(context.Context, mailadapter.Config, mailadapter.Message) (string, error)

func (f mailFunc) Send(ctx context.Context, c mailadapter.Config, m mailadapter.Message) (string, error) {
	return f(ctx, c, m)
}

type whatsappFunc func(context.Context, whatsapp.Config, string, string) error

func (f whatsappFunc) SendText(ctx context.Context, c whatsapp.Config, phone, text string) error {
	return f(ctx, c, phone, text)
}

type pushFunc func(context.Context, webpush.Subscription, webpush.Payload) error

func (f pushFunc) Send(ctx context.Context, s webpush.Subscription, p webpush.Payload) error {
	return f(ctx, s, p)
}

func TestDailyAlertsPreserveReminderMessagesAndPersistThrottleLog(t *testing.T) {
	var mu sync.Mutex
	config := map[string]any{"lembrete_intervalo_dias": 7, "mensagensWhats": map[string]string{"lembrete_ciclo_vencido": "Oi {{cliente}} — {{meses}} meses", "lembrete_vespera": "Amanhã {{data}}"}}
	var queuedWhats []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/v1/whatsapp_message_queue":
			if r.Method == http.MethodPatch {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			var message map[string]any
			if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
				t.Error(err)
			}
			queuedWhats = append(queuedWhats, fmt.Sprint(message["recipient_phone"])+"|"+fmt.Sprint(message["message_text"]))
			w.WriteHeader(http.StatusCreated)
		case "/rest/v1/customers":
			_, _ = io.WriteString(w, `[{"id":"customer-1","nome":"João Silva","whatsapp":"27999991234","profile_id":"11111111-1111-4111-8111-111111111111","ativo":true}]`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"11111111-1111-4111-8111-111111111111","email":"joao@example.com"}]`)
		case "/rest/v1/air_conditioners":
			_, _ = io.WriteString(w, `[{"id":"appliance-1","cliente_id":"customer-1","marca":"LG","modelo":"Dual","btus":12000,"ambiente":"Sala","ultima_manutencao":"2026-03-01"}]`)
		case "/rest/v1/service_history":
			_, _ = io.WriteString(w, `[]`)
		case "/rest/v1/services":
			if r.URL.Query().Get("status") == "eq.CONCLUIDO" {
				_, _ = io.WriteString(w, `[]`)
			} else {
				_, _ = io.WriteString(w, `[{"id":"service-1","cliente_id":"customer-1","aparelho_id":"appliance-1","tipo":"LIMPEZA","descricao":"Limpeza completa","status":"AGENDADO","data_agendamento":"2026-10-04","hora_agendamento":"09:30:00"}]`)
			}
		case technicianConfigPath:
			mu.Lock()
			defer mu.Unlock()
			if r.Method == http.MethodGet {
				_ = json.NewEncoder(w).Encode(config)
				return
			}
			if r.Method == http.MethodPost {
				if r.Header.Get("X-Upsert") != "true" {
					t.Errorf("missing storage upsert header")
				}
				if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
					t.Error(err)
				}
				w.WriteHeader(http.StatusOK)
				return
			}
		case "/storage/v1/object/list/documentos-inovar":
			_, _ = io.WriteString(w, `[]`)
		default:
			t.Errorf("unexpected service request %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "service-role", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	var sentMail []mailadapter.Message
	handler := Handler{Supabase: client, Secret: "cron-secret", Months: 6, MaxLateDays: 180, Now: func() time.Time { return time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC) }, Mail: mailFunc(func(_ context.Context, _ mailadapter.Config, m mailadapter.Message) (string, error) {
		sentMail = append(sentMail, m)
		return "", nil
	}), WhatsApp: whatsappFunc(func(_ context.Context, _ whatsapp.Config, phone, text string) error {
		t.Error("cron must enqueue rather than send directly")
		return nil
	}), Push: pushFunc(func(context.Context, webpush.Subscription, webpush.Payload) error {
		t.Fatal("no stored subscription should be sent")
		return nil
	})}
	request := httptest.NewRequest(http.MethodPost, "/api/cron/alertas", nil)
	request.Header.Set("Authorization", "Bearer cron-secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || result["ciclosVencidos"] != float64(1) || result["lembretesAgenda"] != float64(1) {
		t.Fatalf("status=%d result=%#v body=%s", response.Code, result, response.Body.String())
	}
	if len(sentMail) != 2 || len(queuedWhats) != 2 {
		t.Fatalf("mail=%d WhatsApp=%d", len(sentMail), len(queuedWhats))
	}
	if !strings.Contains(queuedWhats[0], "Oi João — 6 meses") || !strings.Contains(queuedWhats[1], "Amanhã 04/10/2026") {
		t.Fatalf("sent WhatsApp=%#v", queuedWhats)
	}
	mu.Lock()
	defer mu.Unlock()
	saved, ok := config["lembretes_log"].(map[string]interface{})
	if !ok || saved["customer-1|appliance-1"] == nil {
		t.Fatalf("cycle throttle log was not saved: %#v", config)
	}
}

func TestCronHandlerRequiresExactBearerSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request reached backend: %s", r.URL.Path)
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "service-role", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	handler := Handler{Supabase: client, Secret: "cron-secret"}
	for _, header := range []string{"cron-secret", "Bearer wrong"} {
		request := httptest.NewRequest(http.MethodPost, "/api/cron/alertas", nil)
		request.Header.Set("Authorization", header)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("authorization %q status=%d", header, response.Code)
		}
	}
}

func TestHourlyAgendaPushUsesBrazilTimeAndPersistsDeduplication(t *testing.T) {
	var mu sync.Mutex
	config := map[string]any{}
	serviceQueries := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case technicianConfigPath:
			mu.Lock()
			defer mu.Unlock()
			if r.Method == http.MethodGet {
				_ = json.NewEncoder(w).Encode(config)
				return
			}
			if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
				t.Error(err)
			}
			w.WriteHeader(http.StatusOK)
		case "/rest/v1/services":
			serviceQueries++
			if r.URL.Query().Get("data_agendamento") != "eq.2026-10-03" {
				t.Errorf("hourly query did not use São Paulo date: %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `[{"id":"service-1","cliente_id":"customer-1","tipo":"LIMPEZA","descricao":"","data_agendamento":"2026-10-03","hora_agendamento":"13:00:00"}]`)
		case "/rest/v1/customers":
			_, _ = io.WriteString(w, `[{"id":"customer-1","nome":"João Silva","profile_id":"11111111-1111-4111-8111-111111111111","ativo":true}]`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[]`)
		case "/storage/v1/object/list/documentos-inovar":
			_, _ = io.WriteString(w, `[]`)
		default:
			t.Errorf("unexpected service request %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "service-role", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	handler := Handler{Supabase: client, Secret: "cron-secret", Now: func() time.Time { return time.Date(2026, 10, 3, 15, 0, 0, 0, time.UTC) }, Push: pushFunc(func(context.Context, webpush.Subscription, webpush.Payload) error {
		t.Fatal("empty subscription list must not send")
		return nil
	})}
	for i, wantCount := range []float64{1, 0} {
		request := httptest.NewRequest(http.MethodGet, "/api/cron/agenda-uma-hora", nil)
		request.Header.Set("Authorization", "Bearer cron-secret")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		var result map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusOK || result["lembretesUmaHora"] != wantCount {
			t.Fatalf("run %d status=%d result=%#v", i, response.Code, result)
		}
	}
	if serviceQueries != 2 {
		t.Fatalf("service requests=%d", serviceQueries)
	}
	mu.Lock()
	defer mu.Unlock()
	if config["lembretes_uma_hora_push_tentativas"] == nil {
		t.Fatal("hourly push attempt log was not persisted")
	}
}

func TestHourlyAgendaWhatsAppSendsTemplateAndDeduplicates(t *testing.T) {
	var mu sync.Mutex
	config := map[string]any{
		"mensagensWhats": map[string]string{
			"lembrete_uma_hora": "Oi {{cliente}} — {{data}} às {{hora}}",
		},
	}
	var sent []string
	keys := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/v1/whatsapp_message_queue":
			var message map[string]any
			if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
				t.Error(err)
			}
			if message["idempotency_key"] == "" || message["expires_at"] == nil {
				t.Error("hourly reminder requires deduplication and expiry")
			}
			if r.Header.Get("Prefer") != "resolution=ignore-duplicates,return=minimal" {
				t.Error("repeated cron runs must use idempotent insertion")
			}
			key := fmt.Sprint(message["idempotency_key"])
			if !keys[key] {
				keys[key] = true
				sent = append(sent, fmt.Sprint(message["recipient_phone"])+"|"+fmt.Sprint(message["message_text"]))
			}
			w.WriteHeader(http.StatusCreated)
		case technicianConfigPath:
			mu.Lock()
			defer mu.Unlock()
			if r.Method == http.MethodGet {
				_ = json.NewEncoder(w).Encode(config)
				return
			}
			if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
				t.Error(err)
			}
			w.WriteHeader(http.StatusOK)
		case "/rest/v1/services":
			if r.URL.Query().Get("data_agendamento") != "eq.2026-10-03" {
				t.Errorf("hourly query did not use São Paulo date: %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `[{"id":"service-1","cliente_id":"customer-1","tipo":"LIMPEZA","descricao":"Higienização","data_agendamento":"2026-10-03","hora_agendamento":"13:00:00"}]`)
		case "/rest/v1/customers":
			_, _ = io.WriteString(w, `[{"id":"customer-1","nome":"João Silva","whatsapp":"27999991234","profile_id":"11111111-1111-4111-8111-111111111111","ativo":true}]`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[]`)
		case "/storage/v1/object/list/documentos-inovar":
			_, _ = io.WriteString(w, `[]`)
		default:
			t.Errorf("unexpected service request %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "service-role", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	handler := Handler{
		Supabase: client,
		Secret:   "cron-secret",
		Now:      func() time.Time { return time.Date(2026, 10, 3, 15, 0, 0, 0, time.UTC) },
		WhatsApp: whatsappFunc(func(_ context.Context, _ whatsapp.Config, phone, message string) error {
			t.Error("hourly cron must not send directly")
			return nil
		}),
		Push: pushFunc(func(context.Context, webpush.Subscription, webpush.Payload) error {
			t.Fatal("empty subscription list must not send")
			return nil
		}),
	}
	for i, wantCount := range []float64{1, 1} {
		request := httptest.NewRequest(http.MethodPost, "/api/cron/agenda-uma-hora", nil)
		request.Header.Set("Authorization", "Bearer cron-secret")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		var result map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusOK || result["lembretesUmaHora"] != wantCount {
			t.Fatalf("run %d status=%d result=%#v body=%s", i, response.Code, result, response.Body.String())
		}
	}
	if len(sent) != 1 || sent[0] != "5527999991234|Oi João — 03/10/2026 às 13:00" {
		t.Fatalf("WhatsApp deliveries=%#v", sent)
	}
	if len(keys) != 1 {
		t.Fatalf("same appointment must persist only one queue message: %#v", keys)
	}
}
