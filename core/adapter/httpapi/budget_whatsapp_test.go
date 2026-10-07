package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"inovarapp/core/adapter/supabase"
	"inovarapp/core/adapter/whatsapp"
)

type captureWhatsAppDoer struct {
	url             string
	body            map[string]any
	multipart       bool
	multipartStatus int
	form            map[string]string
	fileName        string
	fileBytes       []byte
}

func (d *captureWhatsAppDoer) Do(request *http.Request) (*http.Response, error) {
	d.url = request.URL.String()
	if request.Method == http.MethodGet {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("%PDF-1.7 documento de teste")), Header: http.Header{"Content-Type": []string{"application/pdf"}}}, nil
	}
	if request.Body != nil {
		defer request.Body.Close()
		if strings.HasPrefix(request.Header.Get("Content-Type"), "multipart/") {
			d.multipart = true
			if err := request.ParseMultipartForm(10 << 20); err != nil {
				return nil, err
			}
			d.form = map[string]string{"to": request.FormValue("to"), "caption": request.FormValue("caption")}
			file, header, err := request.FormFile("file")
			if err != nil {
				return nil, err
			}
			d.fileName = header.Filename
			d.fileBytes, err = io.ReadAll(file)
			_ = file.Close()
			if err != nil {
				return nil, err
			}
			status := d.multipartStatus
			if status == 0 {
				status = http.StatusOK
			}
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
		}
		_ = json.NewDecoder(request.Body).Decode(&d.body)
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"ok":true}`)), Header: make(http.Header)}, nil
}

func TestBudgetWhatsAppGeneratesPrivatePDFAndSendsSignedLink(t *testing.T) {
	var budgetToken, storagePath string
	var server *httptest.Server
	var queued map[string]any
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveQueueTestRequest(t, w, r, &queued) {
			return
		}
		switch {
		case r.URL.Path == "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"staff-1"}`)
		case r.URL.Path == "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"staff-1","tipo":"TECNICO"}]`)
		case r.URL.Path == "/rest/v1/budgets":
			budgetToken = r.Header.Get("Authorization")
			_, _ = io.WriteString(w, `[{"id":"11111111-1111-4111-8111-111111111111","numero":"2026-0042","cliente_id":"customer-1","data":"2026-10-03","validade":"2026-10-18","tipo_servico":"Limpeza de Ar","valor_mao_obra":250,"valor_material":20,"valor_total":270,"status":"ENVIADO","descricao":"{\"clientName\":\"João Silva\",\"clientPhone\":\"(27) 99999-0000\",\"clientAddress\":\"Vitória\",\"applianceDesc\":\"LG Inverter 12.000 BTUs\",\"items\":[{\"id\":\"i1\",\"description\":\"Higienização de serpentina\",\"quantity\":1,\"unitPrice\":270,\"totalPrice\":270,\"category\":\"servico\"}],\"discount\":10,\"totalValue\":280,\"executionTime\":\"2 horas\",\"warrantyTerms\":\"90 dias\",\"paymentConditions\":\"PIX\"}"}]`)
		case r.URL.Path == technicianSettingsPath:
			_, _ = io.WriteString(w, `{"name":"Gabriel","businessName":"Inovar Refrigeração"}`)
		case strings.HasPrefix(r.URL.Path, "/storage/v1/object/documentos-inovar/"):
			storagePath = strings.TrimPrefix(r.URL.Path, "/storage/v1/object/documentos-inovar/")
			if r.Header.Get("Authorization") != "Bearer service-role" || r.Header.Get("Content-Type") != "application/pdf" {
				t.Errorf("storage headers=%v", r.Header)
			}
			pdfBytes, _ := io.ReadAll(r.Body)
			if !strings.HasPrefix(string(pdfBytes), "%PDF-") {
				t.Errorf("uploaded object is not PDF: %q", pdfBytes[:min(len(pdfBytes), 12)])
			}
			w.WriteHeader(http.StatusOK)
		case strings.HasPrefix(r.URL.Path, "/storage/v1/object/sign/documentos-inovar/"):
			objectPath := strings.TrimPrefix(r.URL.Path, "/storage/v1/object/sign/documentos-inovar/")
			_ = json.NewEncoder(w).Encode(map[string]string{"signedURL": server.URL + "/storage/v1/object/sign/documentos-inovar/" + objectPath + "?token=private"})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "service-role", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	provider := &captureWhatsAppDoer{}
	request := httptest.NewRequest(http.MethodPost, "/api/orcamento-whatsapp", strings.NewReader(`{"budget_id":"11111111-1111-4111-8111-111111111111"}`))
	request.Header.Set("Authorization", "Bearer staff-token")
	response := httptest.NewRecorder()
	BudgetWhatsAppHandler{Supabase: client, Sender: whatsapp.Sender{HTTPClient: provider}, Defaults: whatsapp.Config{ProprioURL: "https://evolution.example", ProprioToken: "test-key", ProprioSession: "inovar"}}.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["ok"] != true || budgetToken != "Bearer staff-token" || !strings.HasPrefix(storagePath, "os-orcamentos/") {
		t.Fatalf("result=%v budgetToken=%q storagePath=%q", result, budgetToken, storagePath)
	}
	if result["enfileirado"] != true || result["status"] != "pendente" || queued["recipient_phone"] != "5527999990000" || queued["document_storage_path"] != storagePath {
		t.Fatalf("queued proposal=%#v result=%#v", queued, result)
	}
	caption, _ := queued["message_text"].(string)
	if !strings.Contains(caption, "Higienização de serpentina") || !strings.Contains(caption, "R$ 270.00") {
		t.Fatalf("proposal text=%q", caption)
	}
	if queued["document_url"] != nil || queued["expires_at"] == nil {
		t.Fatal("queue must retain a storage path and expiry, not a temporary signed URL")
	}
	if provider.multipart || provider.body != nil {
		t.Fatal("proposal must wait for the worker")
	}
}

func TestBudgetManualWhatsAppURLIncludesPDFLinkAndNormalizedPhone(t *testing.T) {
	link := budgetManualWhatsAppURL("(27) 99999-0000", "Proposta e PDF")
	if !strings.HasPrefix(link, "https://wa.me/5527999990000?") || !strings.Contains(link, "text=Proposta") {
		t.Fatalf("manual WhatsApp link=%q", link)
	}
}
