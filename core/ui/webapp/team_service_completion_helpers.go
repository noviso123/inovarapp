package webapp

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"inovarapp/core/adapter/pdf"
	"inovarapp/core/domain"
)

type serviceOrderPDFResult struct {
	status   int
	whatsapp string
	url      string
	err      error
}

func serviceOrderData(form *teamCompletionForm, completion domain.ServiceCompletion, profile domain.TechnicianProfile) (pdf.ServiceOrderData, error) {
	if form == nil {
		return pdf.ServiceOrderData{}, fmt.Errorf("formulário de conclusão ausente")
	}
	clientRow, ok := form.service["customers"].(map[string]any)
	if !ok {
		return pdf.ServiceOrderData{}, fmt.Errorf("cliente não vinculado")
	}
	applianceRow, ok := form.service["air_conditioners"].(map[string]any)
	if !ok {
		return pdf.ServiceOrderData{}, fmt.Errorf("aparelho não vinculado")
	}
	client := domain.Client{ID: portalText(form.service["cliente_id"]), Name: portalText(clientRow["nome"]), Phone: portalText(clientRow["whatsapp"])}
	client.Address, client.Neighborhood, client.City = completionStringPointer(clientRow, "endereco"), completionStringPointer(clientRow, "bairro"), completionStringPointer(clientRow, "cidade")
	appliance := domain.Appliance{ID: portalText(form.service["aparelho_id"]), Brand: portalText(applianceRow["marca"]), Type: domain.ApplianceType(portalText(applianceRow["tipo"])), CapacityBTU: completionNumber(applianceRow["btus"]), Room: portalText(applianceRow["ambiente"])}
	appliance.Model = completionStringPointer(applianceRow, "modelo")
	checklistJSON, err := json.Marshal(teamCompletionChecklist(form))
	if err != nil {
		return pdf.ServiceOrderData{}, err
	}
	var checklist domain.ChecklistData
	if err := json.Unmarshal(checklistJSON, &checklist); err != nil {
		return pdf.ServiceOrderData{}, err
	}
	date := completion.Date
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	serviceType := domain.ServiceType(form.serviceType)
	notes := strings.TrimSpace(form.notes)
	record := domain.MaintenanceRecord{ID: portalText(form.service["id"]), ClientID: client.ID, ApplianceID: appliance.ID, Date: date, CompletionDate: date, ReturnDate: completion.ReturnDate, ServiceType: serviceType, Price: completion.Total, PartsUsed: form.partsUsed, PaymentMethod: domain.PaymentMethod(form.payment), WarrantyDays: completion.WarrantyDays, Checklist: &checklist, Status: domain.MaintenanceCompleted}
	if notes != "" {
		record.Notes = &notes
	}
	labor, parts := form.laborPrice, form.partsPrice
	record.LaborPrice, record.PartsPrice = &labor, &parts
	return pdf.ServiceOrderData{Client: client, Appliance: appliance, Record: record, Profile: profile}, nil
}

func teamCompletionChecklist(form *teamCompletionForm) map[string]any {
	checklist := make(map[string]any)
	if form != nil {
		for key, value := range form.checklist {
			checklist[key] = value
		}
		if partsUsed := strings.TrimSpace(form.partsUsed); partsUsed != "" {
			checklist["pecasSubstituidas"] = partsUsed
		}
	}
	return checklist
}

func serviceOrderMessage(profile domain.TechnicianProfile, form *teamCompletionForm, completion domain.ServiceCompletion, client map[string]any) string {
	template := domain.ResolveWhatsAppTemplate(profile.WhatsAppMessages, "os_concluida")
	name := portalText(client["nome"])
	if fields := strings.Fields(name); len(fields) > 0 {
		name = fields[0]
	}
	osID := strings.ToUpper(portalText(form.service["id"]))
	if len(osID) > 6 {
		osID = osID[:6]
	}
	date := completion.Date
	if parsed, err := time.Parse("2006-01-02", date); err == nil {
		date = parsed.Format("02/01/2006")
	}
	business := profile.BusinessName
	if business == "" {
		business = "Inovar Refrigeração"
	}
	values := map[string]string{"cliente": name, "os": "OS#" + osID, "servico": form.serviceType, "data": date, "garantia": fmt.Sprintf("%d dias", completion.WarrantyDays), "empresa": business, "app": domain.AppPublicURL}
	return domain.ApplyWhatsAppPlaceholders(template, values)
}

func completionStringPointer(row map[string]any, key string) *string {
	value := strings.TrimSpace(portalText(row[key]))
	if value == "" {
		return nil
	}
	return &value
}

func completionNumber(value any) string {
	switch number := value.(type) {
	case float64:
		return fmt.Sprintf("%.0f", number)
	case string:
		return strings.TrimSpace(number)
	default:
		return ""
	}
}
