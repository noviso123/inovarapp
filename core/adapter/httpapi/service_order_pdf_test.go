package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"inovarapp/core/adapter/pdf"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/adapter/whatsapp"
	"inovarapp/core/domain"
)

func TestServiceOrderPDFRequiresTeam(t *testing.T) {
	for _, test := range []struct {
		name string
		role string
		want int
	}{
		{"customer forbidden", "CLIENTE", http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/auth/v1/user":
					_, _ = io.WriteString(w, `{"id":"user-1"}`)
				case "/rest/v1/profiles":
					_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"`+test.role+`"}]`)
				default:
					t.Errorf("unexpected request %s", r.URL.String())
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "service", HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			h := ServiceOrderPDFHandler{Supabase: client}
			req := httptest.NewRequest(http.MethodPost, "/api/os-pdf", strings.NewReader(`{"service_id":"os-1"}`))
			req.Header.Set("Authorization", "Bearer test-token")
			res := httptest.NewRecorder()
			h.ServeHTTP(res, req)
			if res.Code != test.want {
				t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
			}
		})
	}
}

func TestServiceOrderPDFCanBeGeneratedForScheduledService(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"user-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"TECNICO"}]`)
		case "/rest/v1/services":
			_, _ = io.WriteString(w, `[{"id":"os-scheduled","cliente_id":"client-1","aparelho_id":"device-1","tipo":"LIMPEZA","status":"AGENDADO","data_agendamento":"2026-10-05","valor":250,"observacoes":"[GARANTIA_DIAS:90]","customers":{"id":"client-1","nome":"Ana Silva","whatsapp":"27999991234"},"air_conditioners":{"id":"device-1","marca":"LG","btus":12000,"ambiente":"Sala"}}]`)
		case technicianSettingsPath:
			_, _ = io.WriteString(w, `{"name":"Gabriel","businessName":"Inovar Refrigeração","defaultWarrantyDays":90}`)
		default:
			t.Errorf("unexpected request %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "service", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/os-pdf", strings.NewReader(`{"service_id":"os-scheduled"}`))
	req.Header.Set("Authorization", "Bearer user-token")
	res := httptest.NewRecorder()
	(ServiceOrderPDFHandler{Supabase: client}).ServeHTTP(res, req)
	if res.Code != http.StatusOK || !strings.HasPrefix(res.Body.String(), "%PDF-") {
		t.Fatalf("scheduled order PDF status=%d body=%q", res.Code, res.Body.String()[:min(24, res.Body.Len())])
	}
}

func TestServiceOrderPDFBuildsFromCallerRLSData(t *testing.T) {
	var query string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"user-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"TECNICO"}]`)
		case "/rest/v1/services":
			query = r.URL.RawQuery
			if r.Header.Get("Authorization") != "Bearer user-token" {
				t.Errorf("service read auth=%q", r.Header.Get("Authorization"))
			}
			_, _ = io.WriteString(w, `[{"id":"os-12345678","cliente_id":"client-1","aparelho_id":"device-1","tipo":"LIMPEZA","status":"CONCLUIDO","data_conclusao":"2026-10-03","valor":235,"observacoes":"Serviço de campo concluído. [GARANTIA_DIAS:90] Pagamento: PIX. [PROXIMO_RETORNO:2027-04-03] [CHECKLIST:{\"filtrosLavados\":true}] [MAO_OBRA:200] [PECAS:35]","customers":{"id":"client-1","nome":"João Silva","whatsapp":"27999991234","endereco":"Rua A","bairro":"Centro","cidade":"Vitória"},"air_conditioners":{"id":"device-1","marca":"Consul","modelo":"Inverter","btus":12000,"ambiente":"Quarto"}}]`)
		case technicianSettingsPath:
			_, _ = io.WriteString(w, `{"businessName":"Inovar Refrigeração","defaultWarrantyDays":90,"defaultReturnMonths":6,"defaultPrice":250,"mensagensWhats":{}}`)
		default:
			t.Errorf("unexpected request %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "service", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/os-pdf", strings.NewReader(`{"service_id":"os-12345678"}`))
	req.Header.Set("Authorization", "Bearer user-token")
	res := httptest.NewRecorder()
	(ServiceOrderPDFHandler{Supabase: client}).ServeHTTP(res, req)
	if res.Code != http.StatusOK || res.Header().Get("Content-Type") != "application/pdf" || !strings.HasPrefix(string(res.Body.Bytes()), "%PDF-") {
		t.Fatalf("status=%d headers=%v body=%q", res.Code, res.Header(), res.Body.String()[:min(24, res.Body.Len())])
	}
	fileName := res.Header().Get("X-Service-Order-File")
	if fileName == "" || !strings.Contains(res.Header().Get("Content-Disposition"), `filename="`+fileName+`"`) {
		t.Fatalf("download filename header=%q disposition=%q", fileName, res.Header().Get("Content-Disposition"))
	}
	if !strings.Contains(query, "customers") || !strings.Contains(query, "air_conditioners") || !strings.Contains(query, "id=eq.os-12345678") {
		t.Fatalf("RLS service query=%s", query)
	}
	if !json.Valid([]byte(`{"service_id":"ok"}`)) {
		t.Fatal("invalid JSON fixture")
	}
}

func TestCompletedServiceOrderSendsPDFAutomaticallyWhenRequested(t *testing.T) {
	var uploadedPath string
	var queued map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveQueueTestRequest(t, w, r, &queued) {
			return
		}
		switch {
		case r.URL.Path == "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"staff-1"}`)
		case r.URL.Path == "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"staff-1","tipo":"TECNICO"}]`)
		case r.URL.Path == "/rest/v1/services":
			_, _ = io.WriteString(w, `[{"id":"os-12345678","cliente_id":"client-1","aparelho_id":"device-1","tipo":"LIMPEZA","status":"CONCLUIDO","data_conclusao":"2026-10-03","valor":235,"observacoes":"[GARANTIA_DIAS:90] Pagamento: PIX.","customers":{"id":"client-1","nome":"João Silva","whatsapp":"(27) 99999-1234"},"air_conditioners":{"id":"device-1","marca":"Consul","modelo":"Inverter","btus":12000,"ambiente":"Quarto"}}]`)
		case r.URL.Path == technicianSettingsPath:
			_, _ = io.WriteString(w, `{"businessName":"Inovar Refrigeração","defaultWarrantyDays":90,"mensagensWhats":{"os_concluida":"Olá {{cliente}} — {{os}} — garantia {{garantia}}"}}`)
		case strings.HasPrefix(r.URL.Path, "/storage/v1/object/documentos-inovar/os-orcamentos/"):
			uploadedPath = r.URL.Path
			pdfBytes, err := io.ReadAll(r.Body)
			if err != nil || !strings.HasPrefix(string(pdfBytes), "%PDF-") || r.Header.Get("Content-Type") != "application/pdf" {
				t.Errorf("OS upload content type=%q error=%v body-prefix=%q", r.Header.Get("Content-Type"), err, string(pdfBytes[:min(len(pdfBytes), 8)]))
			}
			w.WriteHeader(http.StatusOK)
		case strings.HasPrefix(r.URL.Path, "/storage/v1/object/sign/documentos-inovar/os-orcamentos/"):
			objectPath := strings.TrimPrefix(r.URL.Path, "/storage/v1/object/sign/documentos-inovar/")
			_ = json.NewEncoder(w).Encode(map[string]string{"signedURL": "/object/sign/documentos-inovar/" + objectPath + "?token=private"})
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "service", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	provider := &captureWhatsAppDoer{}
	handler := ServiceOrderPDFHandler{
		Supabase:         client,
		WhatsApp:         whatsapp.Sender{HTTPClient: provider},
		WhatsAppDefaults: whatsapp.Config{ProprioURL: "https://whatsapp.invalid", ProprioToken: "test-token", ProprioSession: "inovar"},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/os-pdf", strings.NewReader(`{"service_id":"os-12345678","send_whatsapp":true}`))
	req.Header.Set("Authorization", "Bearer staff-token")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK || res.Header().Get("X-Service-Order-WhatsApp") != "queued" || !strings.HasPrefix(res.Body.String(), "%PDF-") {
		t.Fatalf("status=%d whatsapp=%q body-prefix=%q", res.Code, res.Header().Get("X-Service-Order-WhatsApp"), res.Body.String()[:min(12, res.Body.Len())])
	}
	if uploadedPath == "" || queued["recipient_phone"] != "5527999991234" || queued["document_storage_path"] == nil || queued["document_name"] == nil {
		t.Fatalf("OS must persist PDF and recipient: %#v", queued)
	}
	caption, _ := queued["message_text"].(string)
	if !strings.Contains(caption, "OS#OS-123") || !strings.Contains(caption, "garantia 90 dias") {
		t.Fatalf("queued caption=%q", caption)
	}
	if provider.multipart || provider.body != nil {
		t.Fatal("HTTP request must queue, not send synchronously")
	}
}

func TestServiceOrderPDFReportsWhenOnlySignedLinkWasSent(t *testing.T) {
	provider := &captureWhatsAppDoer{multipartStatus: http.StatusBadGateway}
	handler := ServiceOrderPDFHandler{
		WhatsApp:         whatsapp.Sender{HTTPClient: provider},
		WhatsAppDefaults: whatsapp.Config{ProprioURL: "https://whatsapp.invalid", ProprioToken: "test-token", ProprioSession: "inovar"},
	}
	status, configured := handler.sendOrderByWhatsApp(
		t.Context(), pdf.ServiceOrderData{}, "27999991234", domain.TechnicianProfile{},
		"Ordem concluída", nil, "https://storage.example/os.pdf?token=private", "OS.pdf",
	)
	if !configured || status != "sent-link" {
		t.Fatalf("configured=%v status=%q; expected sent-link", configured, status)
	}
	if provider.body["text"] == nil || !strings.Contains(provider.body["text"].(string), "Documento: https://storage.example/os.pdf") {
		t.Fatalf("fallback text=%v", provider.body)
	}
}

func TestServiceOrderWhatsAppMessageUsesConfiguredLegacyTemplate(t *testing.T) {
	data := pdf.ServiceOrderData{
		Client:  domain.Client{Name: "Ana Silva"},
		Record:  domain.MaintenanceRecord{ID: "os-12345678", Date: "2026-10-03", ServiceType: domain.ServiceCleaning, WarrantyDays: 90},
		Profile: domain.TechnicianProfile{},
	}
	profile := domain.TechnicianProfile{BusinessName: "Clima Inovar", WhatsAppMessages: map[string]string{
		"os_concluida": "{{cliente}}|{{os}}|{{servico}}|{{data}}|{{garantia}}|{{empresa}}|{{app}}",
	}}
	got := serviceOrderWhatsAppMessage(data, profile)
	want := "Ana|OS#OS-123|Limpeza de Ar|03/10/2026|90 dias|Clima Inovar|" + domain.AppPublicURL
	if got != want {
		t.Fatalf("message=%q want=%q", got, want)
	}
}
