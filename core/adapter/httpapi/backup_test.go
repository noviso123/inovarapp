package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"inovarapp/core/adapter/supabase"
)

func TestBackupExportsCompleteAccountScopedArchiveWithoutSecrets(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/rest/v1/") || r.URL.Path == "/auth/v1/user" {
			if r.Header.Get("Authorization") != "Bearer staff-token" {
				t.Errorf("caller JWT missing for %s", r.URL.Path)
			}
		} else if r.URL.Path == technicianSettingsPath && r.Header.Get("Authorization") != "Bearer service" {
			t.Errorf("settings read should use server storage credential")
		}
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"staff-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"staff-1","tipo":"ADMIN"}]`)
		case "/rest/v1/customers", "/rest/v1/air_conditioners", "/rest/v1/services", "/rest/v1/appointments", "/rest/v1/budgets", "/rest/v1/service_history":
			if r.URL.Query().Get("limit") != "500" || r.URL.Query().Get("offset") != "0" {
				t.Errorf("backup request not paginated: %s", r.URL)
			}
			paths = append(paths, r.URL.Path)
			if r.URL.Path == "/rest/v1/customers" {
				_, _ = io.WriteString(w, `[{"id":"customer-1","nome":"Ana","profile_id":"linked-account-1"}]`)
			} else {
				_, _ = io.WriteString(w, `[]`)
			}
		case technicianSettingsPath:
			_, _ = io.WriteString(w, `{"name":"Inovar","email_gmail_pass":"must-not-export","whatsapp_proprio_token":"must-not-export","calendario_token":"must-not-export","mensagensWhats":{"teste":"Token secret-x"},"defaultPrice":250}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "service", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/backup", nil)
	request.Header.Set("Authorization", "Bearer staff-token")
	response := httptest.NewRecorder()
	BackupHandler{Supabase: client}.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Content-Disposition") == "" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d headers=%v body=%s", response.Code, response.Header(), response.Body)
	}
	body := response.Body.String()
	for _, field := range []string{`"format":"inovarapp-go-backup"`, `"version":1`, `"customers":[{"id":"customer-1","nome":"Ana"}]`, `"appointments":[]`, `"budgets":[]`, `"serviceHistory":[]`, `"defaultPrice":250`} {
		if !strings.Contains(body, field) {
			t.Errorf("archive missing %s: %s", field, body)
		}
	}
	for _, secret := range []string{"must-not-export", "email_gmail_pass", "whatsapp_proprio_token", "calendario_token", "secret-x", "staff-1", "linked-account-1", "profile_id"} {
		if strings.Contains(body, secret) {
			t.Errorf("secret included in archive: %s", secret)
		}
	}
	if len(paths) != 6 {
		t.Fatalf("expected all six datasets in RLS scope; got %v", paths)
	}
}

func TestLegacyReactBackupMapsRelatedRowsWithStableIDs(t *testing.T) {
	raw := json.RawMessage(`{"version":"1.0","clients":[{"id":"c_local_7","name":"Ana","phone":"27999999999","city":"Serra","createdAt":"2025-01-02T00:00:00.000Z","appliances":[{"id":"a_local_9","brand":"LG","model":"Dual","type":"Split Hi-Wall","capacityBtu":"12.000","room":"Sala","gasType":"R-410A","voltage":"220V"}]}],"maintenances":[{"id":"m_local_3","clientId":"c_local_7","applianceId":"a_local_9","date":"2025-02-03","returnDate":"2025-08-03","serviceType":"Limpeza de Ar","price":250,"warrantyDays":90,"status":"concluido","paymentMethod":"PIX","checklist":{"filtrosLavados":true}}],"profile":{"name":"Técnico"}}`)
	one, recognized, err := decodeLegacyLocalBackup(raw)
	if err != nil || !recognized {
		t.Fatalf("decode legacy archive recognized=%v err=%v", recognized, err)
	}
	two, _, err := decodeLegacyLocalBackup(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(one.Customers) != 1 || len(one.Appliances) != 1 || len(one.Services) != 1 || len(one.History) != 1 || len(one.Appointments) != 0 {
		t.Fatalf("wrong converted counts: %+v", one)
	}
	if one.Customers[0]["id"] != two.Customers[0]["id"] || one.Appliances[0]["id"] != two.Appliances[0]["id"] || one.Services[0]["id"] != two.Services[0]["id"] {
		t.Fatal("legacy IDs must be stable across retries")
	}
	if one.Appliances[0]["cliente_id"] != one.Customers[0]["id"] || one.Services[0]["cliente_id"] != one.Customers[0]["id"] || one.Services[0]["aparelho_id"] != one.Appliances[0]["id"] || one.History[0]["service_id"] != one.Services[0]["id"] {
		t.Fatalf("converted relationships diverged: %+v", one)
	}
	if one.Services[0]["status"] != "CONCLUIDO" || !strings.Contains(one.Services[0]["observacoes"].(string), "[PROXIMO_RETORNO:2025-08-03]") || !strings.Contains(one.Services[0]["observacoes"].(string), "[CHECKLIST:{\"filtrosLavados\":true}]") || one.Appliances[0]["btus"] != 12000 {
		t.Fatalf("fields lost in conversion: %+v", one)
	}
}

func TestLegacyReactBackupRejectsBrokenRelationsBeforeWrite(t *testing.T) {
	_, recognized, err := decodeLegacyLocalBackup(json.RawMessage(`{"version":"1.0","clients":[],"maintenances":[{"id":"m1","clientId":"missing"}]}`))
	if !recognized || err == nil {
		t.Fatalf("broken client reference accepted: recognized=%v err=%v", recognized, err)
	}
}

func TestBackupRestoreAcceptsReactLocalStorageExport(t *testing.T) {
	var written map[string]map[string]any = map[string]map[string]any{}
	settingsSaved := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"staff-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"staff-1","tipo":"ADMIN"}]`)
		case "/rest/v1/customers", "/rest/v1/air_conditioners", "/rest/v1/services", "/rest/v1/service_history":
			if r.Method != http.MethodPost {
				t.Errorf("unexpected method %s for %s", r.Method, r.URL)
			}
			var rows []map[string]any
			if err := json.NewDecoder(r.Body).Decode(&rows); err != nil {
				t.Errorf("decode write: %v", err)
			} else if len(rows) > 0 {
				written[r.URL.Path] = rows[0]
			}
			w.WriteHeader(http.StatusNoContent)
		case technicianSettingsPath:
			if r.Method == http.MethodGet {
				_, _ = io.WriteString(w, `{"whatsapp_proprio_token":"preserve-current-secret"}`)
				return
			}
			var config map[string]json.RawMessage
			if json.NewDecoder(r.Body).Decode(&config) != nil || len(config["name"]) == 0 {
				t.Errorf("invalid settings import: %v", config)
			}
			if string(config["whatsapp_proprio_token"]) != `"preserve-current-secret"` {
				t.Errorf("settings import overwrote existing credentials: %s", config["whatsapp_proprio_token"])
			}
			settingsSaved = true
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "service", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	legacy := `{"version":"1.0","clients":[{"id":"local-c1","name":"Ana","phone":"2799","appliances":[{"id":"local-a1","brand":"LG","type":"Split Hi-Wall","capacityBtu":"9.000","room":"Quarto"}]}],"maintenances":[{"id":"local-m1","clientId":"local-c1","applianceId":"local-a1","date":"2025-01-02","serviceType":"Limpeza de Ar","status":"concluido","price":220}],"profile":{"name":"Técnico","whatsapp_proprio_token":"must-not-overwrite"}}`
	request := httptest.NewRequest(http.MethodPost, "/api/backup", strings.NewReader(legacy))
	request.Header.Set("Authorization", "Bearer staff-token")
	response := httptest.NewRecorder()
	BackupHandler{Supabase: client}.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"ok":true`) {
		t.Fatalf("legacy restore status=%d body=%s", response.Code, response.Body)
	}
	if len(written) != 4 || !settingsSaved || written["/rest/v1/air_conditioners"]["cliente_id"] != written["/rest/v1/customers"]["id"] || written["/rest/v1/services"]["aparelho_id"] != written["/rest/v1/air_conditioners"]["id"] || written["/rest/v1/service_history"]["service_id"] != written["/rest/v1/services"]["id"] {
		t.Fatalf("legacy archive relationships not preserved: %#v", written)
	}
}

func TestBackupAbortsOnAnyIncompleteRead(t *testing.T) {
	dataReads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"staff-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"staff-1","tipo":"TECNICO"}]`)
		case "/rest/v1/customers":
			dataReads++
			_, _ = io.WriteString(w, `[]`)
		case "/rest/v1/air_conditioners":
			dataReads++
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/backup", nil)
	request.Header.Set("Authorization", "Bearer staff-token")
	response := httptest.NewRecorder()
	BackupHandler{Supabase: client}.ServeHTTP(response, request)
	if response.Code != http.StatusBadGateway || response.Header().Get("Content-Disposition") != "" || strings.Contains(response.Body.String(), `"customers"`) || dataReads != 2 {
		t.Fatalf("partial backup leaked: status=%d disposition=%q body=%s reads=%d", response.Code, response.Header().Get("Content-Disposition"), response.Body, dataReads)
	}
}

func TestBackupRejectsCustomerAndWrongMethod(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"customer-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"customer-1","tipo":"CLIENTE"}]`)
		default:
			t.Errorf("unexpected read %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/backup", nil)
	request.Header.Set("Authorization", "Bearer customer-token")
	response := httptest.NewRecorder()
	BackupHandler{Supabase: client}.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("customer status=%d body=%s", response.Code, response.Body)
	}
	request = httptest.NewRequest(http.MethodPut, "/api/backup", nil)
	response = httptest.NewRecorder()
	BackupHandler{Supabase: client}.ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status=%d", response.Code)
	}
}

func TestBackupRestoreValidatesFirstAndMergesWithCallerRLS(t *testing.T) {
	var writes []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/rest/v1/") && r.Method == http.MethodPost {
			if r.Header.Get("Authorization") != "Bearer staff-token" || r.Header.Get("Prefer") != "resolution=merge-duplicates,return=minimal" {
				t.Errorf("restore did not use caller JWT/upsert: auth=%q prefer=%q", r.Header.Get("Authorization"), r.Header.Get("Prefer"))
			}
			var rows []map[string]any
			if json.NewDecoder(r.Body).Decode(&rows) != nil || len(rows) != 1 {
				t.Errorf("invalid upsert payload")
			}
			if _, exists := rows[0]["profile_id"]; exists {
				t.Errorf("restore attempted to link an auth profile")
			}
			writes = append(writes, r.URL.Path+"?conflict="+r.URL.Query().Get("on_conflict"))
			w.WriteHeader(http.StatusNoContent)
			return
		}
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"staff-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"staff-1","tipo":"ADMIN"}]`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	archive := `{"format":"inovarapp-go-backup","version":1,"customers":[{"id":"c1","nome":"Ana","profile_id":"foreign-user"}],"appliances":[{"id":"a1","cliente_id":"c1"}],"services":[{"id":"s1","cliente_id":"c1","aparelho_id":"a1"}],"appointments":[{"service_id":"s1","cliente_id":"c1"}],"budgets":[{"id":"b1","cliente_id":"c1","descricao":"{}"}],"serviceHistory":[{"id":"h1","cliente_id":"c1","service_id":"s1"}]}`
	request := httptest.NewRequest(http.MethodPost, "/api/backup", strings.NewReader(archive))
	request.Header.Set("Authorization", "Bearer staff-token")
	response := httptest.NewRecorder()
	BackupHandler{Supabase: client}.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || len(writes) != 0 {
		t.Fatalf("identity-bearing archive must fail before writes: status=%d writes=%v body=%s", response.Code, writes, response.Body)
	}
	archive = strings.Replace(archive, `,"profile_id":"foreign-user"`, "", 1)
	request = httptest.NewRequest(http.MethodPost, "/api/backup", strings.NewReader(archive))
	request.Header.Set("Authorization", "Bearer staff-token")
	response = httptest.NewRecorder()
	BackupHandler{Supabase: client}.ServeHTTP(response, request)
	if response.Code != http.StatusOK || len(writes) != 6 {
		t.Fatalf("restore status=%d writes=%v body=%s", response.Code, writes, response.Body)
	}
	if !strings.Contains(strings.Join(writes, "|"), "/appointments?conflict=service_id") || !strings.Contains(response.Body.String(), `"ok":true`) {
		t.Fatalf("wrong restore table conflict or result: writes=%v body=%s", writes, response.Body)
	}
}

func TestBackupRestoreStopsWithResumeInformationAfterPartialWrite(t *testing.T) {
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"staff-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"staff-1","tipo":"TECNICO"}]`)
		case "/rest/v1/customers":
			writes++
			w.WriteHeader(http.StatusNoContent)
		case "/rest/v1/air_conditioners":
			writes++
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			t.Errorf("unexpected request %s", r.URL)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	archive := `{"format":"inovarapp-go-backup","version":1,"customers":[{"id":"c1"}],"appliances":[{"id":"a1","cliente_id":"c1"}],"services":[],"appointments":[],"budgets":[],"serviceHistory":[]}`
	request := httptest.NewRequest(http.MethodPost, "/api/backup", strings.NewReader(archive))
	request.Header.Set("Authorization", "Bearer staff-token")
	response := httptest.NewRecorder()
	BackupHandler{Supabase: client}.ServeHTTP(response, request)
	if response.Code != http.StatusBadGateway || writes != 2 || !strings.Contains(response.Body.String(), `"customers":1`) || !strings.Contains(response.Body.String(), "backup novamente") {
		t.Fatalf("partial restore did not report resumable state: status=%d writes=%d body=%s", response.Code, writes, response.Body)
	}
}
