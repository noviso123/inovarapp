package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"inovarapp/core/adapter/pdf"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/adapter/whatsapp"
	"inovarapp/core/adapter/whatsappqueue"
	"inovarapp/core/domain"
)

type ServiceOrderPDFHandler struct {
	Supabase         *supabase.Client
	WhatsApp         whatsapp.Sender
	WhatsAppDefaults whatsapp.Config
}

type serviceOrderPDFRequest struct {
	ServiceID    string `json:"service_id"`
	SendWhatsApp bool   `json:"send_whatsapp"`
}

var (
	serviceOrderPaymentPattern = regexp.MustCompile(`Pagamento: (PIX|Cartão Crédito|Cartão Débito|Dinheiro|A Faturar)\.`)
	unsafeFileNamePattern      = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)
)

func (h ServiceOrderPDFHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeResourceJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Método não permitido"})
		return
	}
	if h.Supabase == nil {
		writeResourceJSON(w, http.StatusInternalServerError, map[string]string{"error": "Backend não configurado"})
		return
	}
	caller, err := h.Supabase.AuthenticateCaller(r.Context(), supabase.BearerToken(r.Header.Get("Authorization")))
	if err != nil {
		writeResourceJSON(w, http.StatusUnauthorized, map[string]string{"error": "Não autenticado"})
		return
	}
	if !isTeamRole(caller.Role) {
		writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Somente a equipe Inovar pode gerar a ordem de serviço"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var request serviceOrderPDFRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || strings.TrimSpace(request.ServiceID) == "" {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Informe uma ordem de serviço válida"})
		return
	}
	data, phone, profile, err := h.loadServiceOrder(r.Context(), caller, request.ServiceID)
	if err != nil {
		writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível carregar os dados da ordem de serviço"})
		return
	}
	output, err := pdf.BuildServiceOrderPDF(data.Client, data.Appliance, data.Record, profile)
	if err != nil {
		writeResourceJSON(w, http.StatusInternalServerError, map[string]string{"error": "Não foi possível gerar o PDF da ordem de serviço"})
		return
	}
	fileBase := unsafeFileNamePattern.ReplaceAllString(data.Client.Name, "_")
	if fileBase == "" {
		fileBase = "Cliente"
	}
	fileName := fmt.Sprintf("OS_INOVAR_%s_%s.pdf", fileBase, unsafeFileNamePattern.ReplaceAllString(data.Record.ID, "_"))
	if request.SendWhatsApp {
		fileID := uuid.NewString()
		objectPath := "os-orcamentos/" + time.Now().UTC().Format("2006-01-02") + "/" + fileID + ".pdf"
		if err := h.Supabase.UploadPrivateObject(r.Context(), objectPath, "application/pdf", output); err != nil {
			writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível salvar o PDF da ordem de serviço"})
			return
		}
		signedURL, err := h.Supabase.CreatePrivateSignedURL(r.Context(), objectPath, 30*24*60*60)
		if err != nil {
			writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível criar o link privado do PDF"})
			return
		}
		w.Header().Set("X-Service-Order-URL", signedURL)
		w.Header().Set("X-Service-Order-WhatsApp", "not-configured")
		message := serviceOrderWhatsAppMessage(data, profile)
		if strings.TrimSpace(phone) != "" {
			queueErr := whatsappqueue.Enqueue(r.Context(), h.Supabase, whatsappqueue.EnqueueInput{
				IdempotencyKey: "os-comprovante:" + data.Record.ID + ":" + time.Now().UTC().Format("200601021504"),
				EventType:      "ordem_servico_concluida", RecipientPhone: phone, MessageText: message,
				DocumentStoragePath: objectPath, DocumentName: fileName,
				SourceEntityType: "servico", SourceEntityID: data.Record.ID,
			})
			if queueErr != nil {
				w.Header().Set("X-Service-Order-WhatsApp", "queue-error")
			} else {
				w.Header().Set("X-Service-Order-WhatsApp", "queued")
			}
		}
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("X-Service-Order-File", fileName)
	w.Header().Set("Content-Disposition", `attachment; filename="`+fileName+`"`)
	w.Header().Set("Cache-Control", "no-store")
	if request.SendWhatsApp {
		w.Header().Set("X-Service-Order-PDF-Generated", "true")
	}
	_, _ = w.Write(output)
}

func (h ServiceOrderPDFHandler) loadServiceOrder(ctx context.Context, caller supabase.Caller, serviceID string) (pdf.ServiceOrderData, string, domain.TechnicianProfile, error) {
	query := url.Values{}
	query.Set("id", "eq."+serviceID)
	query.Set("select", "*,customers:cliente_id(*),air_conditioners:aparelho_id(*)")
	result, err := h.Supabase.UserRequest(ctx, caller.Token, "/rest/v1/services?"+query.Encode(), supabase.RequestOptions{Method: http.MethodGet})
	if err != nil || result.StatusCode != http.StatusOK {
		return pdf.ServiceOrderData{}, "", domain.TechnicianProfile{}, fmt.Errorf("read service order: status=%d err=%v", result.StatusCode, err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(result.Body, &rows); err != nil || len(rows) != 1 {
		return pdf.ServiceOrderData{}, "", domain.TechnicianProfile{}, fmt.Errorf("service order is not visible: %w", err)
	}
	row := rows[0]
	clientRow, ok := nestedRelation(row["customers"])
	if !ok {
		return pdf.ServiceOrderData{}, "", domain.TechnicianProfile{}, fmt.Errorf("customer is not linked")
	}
	applianceRow, ok := nestedRelation(row["air_conditioners"])
	if !ok {
		return pdf.ServiceOrderData{}, "", domain.TechnicianProfile{}, fmt.Errorf("appliance is not linked")
	}
	profile := h.loadDocumentProfile(ctx)
	client := serviceOrderClient(clientRow)
	appliance := serviceOrderAppliance(applianceRow)
	encoded, err := json.Marshal(row)
	if err != nil {
		return pdf.ServiceOrderData{}, "", domain.TechnicianProfile{}, err
	}
	var service supabase.SupabaseServiceRow
	if err := json.Unmarshal(encoded, &service); err != nil {
		return pdf.ServiceOrderData{}, "", domain.TechnicianProfile{}, err
	}
	record := supabase.MapSupabaseServiceToLocal(service, profile, time.Now())
	if matches := serviceOrderPaymentPattern.FindStringSubmatch(service.Notes); len(matches) > 1 {
		record.PaymentMethod = domain.PaymentMethod(matches[1])
	}
	return pdf.ServiceOrderData{Client: client, Appliance: appliance, Record: record, Profile: profile}, client.Phone, profile, nil
}

func (h ServiceOrderPDFHandler) loadDocumentProfile(ctx context.Context) domain.TechnicianProfile {
	profile := domain.TechnicianProfile{}
	config, err := (SettingsHandler{Supabase: h.Supabase}).loadSettings(ctx)
	if err != nil {
		config = defaultTechnicianSettings()
	}
	profileRaw, _ := json.Marshal(config)
	_ = json.Unmarshal(profileRaw, &profile)
	if profile.Name == "" {
		profile.Name = "Gabriel"
	}
	return profile
}

func (h ServiceOrderPDFHandler) sendOrderByWhatsApp(ctx context.Context, data pdf.ServiceOrderData, phone string, profile domain.TechnicianProfile, message string, output []byte, signedURL, fileName string) (string, bool) {
	if strings.TrimSpace(phone) == "" {
		return "not-configured", false
	}
	config := h.WhatsAppDefaults
	configured := strings.TrimSpace(config.ProprioURL) != "" && strings.TrimSpace(config.ProprioToken) != "" && strings.TrimSpace(config.ProprioSession) != ""
	if !configured {
		return "not-configured", false
	}
	deadline, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	delivery, err := h.WhatsApp.SendDocumentWithResult(deadline, config, phone, message, signedURL, fileName)
	if err != nil {
		return "failed", true
	}
	if delivery.Attached {
		return "sent", true
	}
	if delivery.LinkSent {
		return "sent-link", true
	}
	return "failed", true
}

func serviceOrderWhatsAppMessage(data pdf.ServiceOrderData, profile domain.TechnicianProfile) string {
	firstName := data.Client.Name
	if fields := strings.Fields(firstName); len(fields) > 0 {
		firstName = fields[0]
	}
	osID := strings.ToUpper(strings.TrimSpace(data.Record.ID))
	if len(osID) > 6 {
		osID = osID[:6]
	}
	date := data.Record.Date
	if parsed, err := time.Parse("2006-01-02", date); err == nil {
		date = parsed.Format("02/01/2006")
	}
	warranty := ""
	if data.Record.WarrantyDays != 0 {
		warranty = fmt.Sprintf("%d dias", data.Record.WarrantyDays)
	}
	business := strings.TrimSpace(profile.BusinessName)
	if business == "" {
		business = "Inovar Refrigeração"
	}
	return domain.ApplyWhatsAppPlaceholders(domain.ResolveWhatsAppTemplate(profile.WhatsAppMessages, "os_concluida"), map[string]string{
		"cliente": firstName, "os": "OS#" + osID, "servico": serviceOrderTypeLabel(data.Record.ServiceType),
		"data": date, "garantia": warranty, "empresa": business, "app": domain.AppPublicURL,
	})
}

func serviceOrderTypeLabel(serviceType domain.ServiceType) string {
	switch serviceType {
	case domain.ServiceCleaning:
		return "Limpeza de Ar"
	case domain.ServiceCorrective:
		return "Manutenção Corretiva"
	case domain.ServiceInstallation:
		return "Instalação"
	case domain.ServiceRefrigerant:
		return "Carga de Gás"
	default:
		return string(serviceType)
	}
}

func nestedRelation(value any) (map[string]any, bool) {
	if record, ok := value.(map[string]any); ok {
		return record, true
	}
	if records, ok := value.([]any); ok && len(records) > 0 {
		record, ok := records[0].(map[string]any)
		return record, ok
	}
	return nil, false
}

func serviceOrderClient(row map[string]any) domain.Client {
	client := domain.Client{ID: stringField(row, "id"), Name: stringField(row, "nome"), Phone: stringField(row, "whatsapp"), CreatedAt: stringField(row, "created_at")}
	client.Document = stringPointer(row, "cpf_cnpj")
	client.Address = stringPointer(row, "endereco")
	client.Neighborhood = stringPointer(row, "bairro")
	client.City = stringPointer(row, "cidade")
	return client
}

func serviceOrderAppliance(row map[string]any) domain.Appliance {
	return domain.Appliance{
		ID: stringField(row, "id"), Brand: stringField(row, "marca"), Model: stringPointer(row, "modelo"),
		Type: domain.ApplianceType(stringField(row, "tipo")), CapacityBTU: numberString(row["btus"]), Room: stringField(row, "ambiente"),
	}
}

func stringField(row map[string]any, key string) string {
	value, _ := row[key].(string)
	return strings.TrimSpace(value)
}

func stringPointer(row map[string]any, key string) *string {
	value := stringField(row, key)
	if value == "" {
		return nil
	}
	return &value
}

func numberString(value any) string {
	switch number := value.(type) {
	case float64:
		return strconv.FormatInt(int64(number), 10)
	case string:
		return strings.TrimSpace(number)
	default:
		return ""
	}
}

func lastServiceOrderChars(value string, count int) string {
	if len(value) <= count {
		return value
	}
	return value[len(value)-count:]
}
