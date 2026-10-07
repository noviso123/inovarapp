package webapp

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/domain"
)

const teamDeleteServiceAction = "delete-service:"

type teamServiceEditForm struct {
	ID, Type, Date, OriginalDate, Customer, Appliance, Notes string
	Price, Status, WarrantyDays, Message                     string
	Saving                                                   bool
}

func (p *serviceCatalogPage) openTeamServiceEdit(service map[string]any) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		if service == nil {
			return
		}
		typeName := string(supabase.MapSupabaseServiceTypeToLocal(portalText(service["tipo"])))
		if portalText(service["tipo"]) == "OUTRO" && portalText(service["descricao"]) != "" {
			typeName = portalText(service["descricao"])
		}
		status := strings.ToUpper(portalText(service["status"]))
		if status == "" {
			status = "AGENDADO"
		}
		date := portalText(service["data_agendamento"])
		for _, appointment := range p.teamAppointments {
			if portalText(appointment["service_id"]) == portalText(service["id"]) {
				date = teamScheduleOriginalDate(service, appointment)
				break
			}
		}
		if status == "CONCLUIDO" {
			date = portalText(service["data_conclusao"])
		}
		if status == "CANCELADO" {
			date = portalText(service["data_cancelamento"])
		}
		if len(date) > 10 {
			date = date[:10]
		}
		customer, _ := service["customers"].(map[string]any)
		appliance, _ := service["air_conditioners"].(map[string]any)
		applianceName := strings.TrimSpace(portalText(appliance["marca"]) + " " + portalText(appliance["modelo"]))
		if btu := portalText(appliance["btus"]); btu != "" {
			applianceName += " • " + portalBTUs(appliance["btus"])
		}
		if room := portalText(appliance["ambiente"]); room != "" {
			applianceName += " • " + room
		}
		warrantyDays := p.teamProfile.DefaultWarrantyDays
		if warrantyDays <= 0 {
			warrantyDays = 90
		}
		if match := teamServiceWarrantyMarker.FindStringSubmatch(portalText(service["observacoes"])); len(match) == 2 {
			if parsed, err := strconv.Atoi(match[1]); err == nil && parsed > 0 {
				warrantyDays = parsed
			}
		}
		p.teamServiceEdit = &teamServiceEditForm{ID: portalText(service["id"]), Type: typeName, Date: date, OriginalDate: date, Price: strconv.FormatFloat(agendaNumber(service["valor"]), 'f', -1, 64), Customer: portalText(customer["nome"]), Appliance: applianceName, Notes: teamServiceEditableNotes(portalText(service["observacoes"])), Status: status, WarrantyDays: strconv.Itoa(warrantyDays)}
	}
}

func (p *serviceCatalogPage) saveTeamServiceEdit(ctx app.Context, event app.Event) {
	event.PreventDefault()
	f := p.teamServiceEdit
	if f == nil || f.Saving || f.ID == "" || p.session == nil {
		return
	}
	price, priceErr := parseTeamServicePrice(f.Price)
	warrantyDays, warrantyErr := strconv.Atoi(f.WarrantyDays)
	if priceErr != nil || price < 0 || !validCivilDate(f.Date) || warrantyErr != nil || warrantyDays < 0 {
		f.Message = "Confira o tipo, a data, o valor e a garantia do serviço."
		return
	}
	pageURL := app.Window().URL()
	if pageURL == nil || pageURL.Scheme == "" || pageURL.Host == "" {
		f.Message = "Servidor indisponível."
		return
	}
	base, token := apiBaseURL(), p.session.AccessToken
	var current map[string]any
	for _, row := range p.teamServices {
		if portalText(row["id"]) == f.ID {
			current = row
			break
		}
	}
	if current == nil {
		f.Message = "A ordem de serviço não está mais disponível."
		return
	}
	f.Saving, f.Message = true, "Salvando alterações..."
	serviceID, serviceStatus, date := f.ID, strings.ToUpper(portalText(current["status"])), f.Date
	if serviceStatus == "" {
		serviceStatus = "AGENDADO"
	}
	if strings.ToUpper(f.Status) != serviceStatus {
		f.Message = "O status desta OS mudou. Feche e abra a edição novamente para continuar."
		f.Saving = false
		return
	}
	observations := domain.MergeServiceObservationMarkers(f.Notes, portalText(current["observacoes"]))
	observations = domain.SetServiceWarrantyMarker(observations, warrantyDays)
	serviceKind := supabase.MapServiceTypeToSupabase(f.Type)
	if serviceKind == "" {
		serviceKind = "OUTRO"
	}
	fields := map[string]any{"tipo": serviceKind, "descricao": strings.TrimSpace(f.Type), "valor": price, "observacoes": observations}
	mutationPayload := map[string]any{"id": serviceID, "fields": fields}
	dateField, marker := teamServiceEditDateField(serviceStatus)
	fields[dateField] = date
	var appointment map[string]any
	for _, row := range p.teamAppointments {
		if portalText(row["service_id"]) == serviceID {
			appointment = row
			break
		}
	}
	oldScheduleDate := teamScheduleOriginalDate(current, appointment)
	oldScheduleTime := teamScheduleOriginalTime(current, appointment)
	scheduleChanged := dateField == "data_agendamento" && oldScheduleDate != date
	client, _ := current["customers"].(map[string]any)
	profile := p.teamProfile
	go func() {
		_, err := sendTeamJSONResult(ctx, base+"/api/servicos", token, http.MethodPatch, map[string]any{"id": serviceID, "fields": fields})
		if err != nil && marker != "" {
			delete(fields, dateField)
			fields["observacoes"] = domain.SetServiceDateMarker(observations, marker, date)
			_, err = sendTeamJSONResult(ctx, base+"/api/servicos", token, http.MethodPatch, map[string]any{"id": serviceID, "fields": fields})
		}
		appointmentError := false
		var failedAppointmentPayload map[string]any
		var failedAppointmentError error
		failedAppointmentMethod := http.MethodPatch
		if err == nil && scheduleChanged {
			if appointment != nil {
				failedAppointmentPayload = map[string]any{"service_id": serviceID, "fields": map[string]any{"data": date}}
				_, failedAppointmentError = sendTeamJSONResult(ctx, base+"/api/agendamentos", token, http.MethodPatch, failedAppointmentPayload)
			} else {
				failedAppointmentMethod = http.MethodPost
				failedAppointmentPayload = teamCallScheduleAppointmentPayload(serviceID, portalText(current["cliente_id"]), date, oldScheduleTime, "")
				_, failedAppointmentError = sendTeamJSONResult(ctx, base+"/api/agendamentos", token, http.MethodPost, failedAppointmentPayload)
			}
			appointmentError = failedAppointmentError != nil
		}
		if err != nil {
			if p.deferOfflineTeamMutation(base+"/api/servicos", http.MethodPatch, mutationPayload, err) {
				p.teamServiceEdit = nil
				p.teamActionMessage = "Edição da ordem salva neste dispositivo. Será sincronizada quando a conexão voltar."
				p.teamLoadedFor = ""
				p.retryPendingOfflineMutations(ctx)
			} else {
				f.Saving, f.Message = false, "Não foi possível salvar a edição do serviço. Tente novamente."
			}
			ctx.Update()
			return
		}
		p.teamServiceEdit = nil
		p.teamServiceReceiptEdit = false
		p.teamActionMessage = "OS atualizada no banco!"
		p.registerLocalNotification(ctx, "Ordem de serviço atualizada", "Os dados da OS foram sincronizados no banco.")
		if appointmentError {
			if p.deferOfflineTeamMutation(base+"/api/agendamentos", failedAppointmentMethod, failedAppointmentPayload, failedAppointmentError) {
				p.teamActionMessage = "Serviço atualizado. A data da agenda ficou na fila e será sincronizada ao reconectar."
				p.retryPendingOfflineMutations(ctx)
			} else {
				p.teamActionMessage = "Serviço atualizado, mas a data na agenda precisa ser sincronizada novamente."
			}
		}
		if scheduleChanged && !appointmentError {
			newSchedule := appointmentDateBR(date)
			if oldScheduleTime != "" {
				newSchedule += " às " + oldScheduleTime
			} else {
				newSchedule += " — horário a confirmar"
			}
			p.registerLocalNotification(ctx, "Agendamento alterado", "Novo atendimento: "+newSchedule)
			if phone := portalText(client["whatsapp"]); phone != "" {
				message := teamRescheduledServiceMessage(profile, portalText(client["nome"]), firstNonEmptyBudget(portalText(current["descricao"]), portalText(current["tipo"])), date, oldScheduleTime)
				result, sendErr := sendTeamJSONResult(ctx, base+"/api/whatsapp", token, http.MethodPost, map[string]any{"acao": "enviar", "evento": "agendamento_reagendado", "origem_tipo": "servico", "origem_id": serviceID, "telefone": phone, "texto": message})
				if sendErr != nil || result["ok"] != true {
					p.teamActionMessage += " " + teamRescheduleWhatsAppDeliveryNotice(false)
				} else {
					p.teamActionMessage += " " + teamRescheduleWhatsAppDeliveryNotice(true)
				}
			} else {
				p.teamActionMessage += " O cliente não tem WhatsApp cadastrado para receber o aviso."
			}
		}
		p.teamLoadedFor = ""
		p.loadTeamOperations(ctx)
		ctx.Update()
	}()
}

func teamServiceEditDateField(status string) (field, marker string) {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "CONCLUIDO":
		return "data_conclusao", "DATA_CONCLUSAO"
	case "CANCELADO":
		return "data_cancelamento", "DATA_CANCELAMENTO"
	default:
		return "data_agendamento", ""
	}
}

func teamServiceStatusLabel(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "AGENDADO":
		return "Agendado"
	case "EM_ANDAMENTO":
		return "Em andamento"
	case "CONCLUIDO":
		return "Concluído"
	case "CANCELADO":
		return "Cancelado"
	default:
		return "Pendente"
	}
}

func validCivilDate(value string) bool {
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

func (p *serviceCatalogPage) teamServiceReceiptEditPanel(service map[string]any) app.UI {
	if !p.teamServiceReceiptEdit || p.teamServiceEdit == nil {
		return app.Div()
	}
	form := p.teamServiceEdit
	warrantyOptions := make([]app.UI, 0, 4)
	for _, days := range []string{"30", "90", "180", "365"} {
		warrantyOptions = append(warrantyOptions, app.Option().Value(days).Body(app.Text(days+" dias")))
	}
	if form.WarrantyDays != "30" && form.WarrantyDays != "90" && form.WarrantyDays != "180" && form.WarrantyDays != "365" {
		warrantyOptions = append([]app.UI{app.Option().Value(form.WarrantyDays).Body(app.Text(form.WarrantyDays + " dias"))}, warrantyOptions...)
	}
	return app.Section().Class("portal-service").Body(
		app.Strong().Body(app.Text("Editar Ordem de Serviço")),
		app.Div().Class("service-grid").Body(
			app.Label().Class("auth-field").Body(app.Text("Data do serviço"), app.Input().Type("date").Value(form.Date).OnChange(p.ValueTo(&form.Date))),
			app.Label().Class("auth-field").Body(app.Text("Valor (R$)"), app.Input().Type("number").Attr("min", 0).Attr("step", 10).Value(form.Price).OnChange(p.ValueTo(&form.Price))),
			app.Label().Class("auth-field").Body(app.Text("Garantia (dias)"), app.Select().Attr("value", form.WarrantyDays).OnChange(p.ValueTo(&form.WarrantyDays)).Body(warrantyOptions...)),
			app.Div().Class("auth-field").Body(app.Text("Status atual"), app.Strong().Body(app.Text(teamServiceStatusLabel(form.Status)))),
		),
		app.P().Class("portal-section__intro").Body(app.Text("Para iniciar, concluir ou cancelar a OS, use a ação correspondente. Assim o histórico, o comprovante e os avisos seguem o fluxo correto.")),
		formErrorNotice(form.Message),
		app.Div().Class("portal-budget__actions").Body(
			app.Button().Class("auth-link").Type("button").Disabled(form.Saving).OnClick(func(ctx app.Context, event app.Event) {
				event.PreventDefault()
				p.teamServiceReceiptEdit = false
				p.teamServiceEdit = nil
			}).Body(app.Text("Cancelar")),
			app.Button().Class("auth-submit").Type("button").Disabled(form.Saving).OnClick(p.saveTeamServiceEdit).Body(app.Text("Salvar no Banco")),
		),
	)
}

func parseTeamServicePrice(value string) (float64, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "R$", ""))
	if strings.Contains(value, ",") {
		value = strings.ReplaceAll(value, ".", "")
		value = strings.ReplaceAll(value, ",", ".")
	}
	return strconv.ParseFloat(value, 64)
}

func teamServiceEditableNotes(observations string) string {
	observations = teamServiceInternalMarkers.ReplaceAllString(observations, "")
	observations = regexp.MustCompile(`(?i)Garantia de \d+ dias\.?`).ReplaceAllString(observations, "")
	return strings.TrimSpace(regexp.MustCompile(`[\t ]+`).ReplaceAllString(observations, " "))
}

func (p *serviceCatalogPage) teamServiceEditDialog() app.UI {
	f := p.teamServiceEdit
	if f == nil {
		return app.Div()
	}
	entries := domain.BuildServiceCatalog(&p.teamProfile)
	found := false
	options := make([]app.UI, 0, len(entries)+1)
	for _, entry := range entries {
		options = append(options, app.Option().Value(entry.Name).Body(app.Text(entry.Name)))
		if entry.Name == f.Type {
			found = true
		}
	}
	if !found && f.Type != "" {
		options = append([]app.UI{app.Option().Value(f.Type).Body(app.Text(f.Type))}, options...)
	}
	field := func(label string, target *string, kind string) app.UI {
		return app.Label().Class("auth-field team-service-edit-field").Body(app.Text(label), app.Input().Type(kind).Value(*target).OnChange(p.ValueTo(target)))
	}
	warrantyOptions := []app.UI{}
	for _, days := range []string{"30", "90", "180", "365"} {
		warrantyOptions = append(warrantyOptions, app.Option().Value(days).Body(app.Text(days+" dias")))
	}
	if f.WarrantyDays != "30" && f.WarrantyDays != "90" && f.WarrantyDays != "180" && f.WarrantyDays != "365" {
		warrantyOptions = append([]app.UI{app.Option().Value(f.WarrantyDays).Body(app.Text(f.WarrantyDays + " dias"))}, warrantyOptions...)
	}
	dateNotice := app.Div()
	dateField, _ := teamServiceEditDateField(f.Status)
	if f.Date != f.OriginalDate && dateField == "data_agendamento" {
		dateNotice = app.Div().Class("auth-notice").Body(app.Text("Ao salvar uma nova data, a Agenda será atualizada e o cliente será avisado pelo WhatsApp."))
	}
	return app.Div().Class("auth-backdrop team-service-edit-backdrop").Body(app.Div().Class("auth-dialog team-service-edit-dialog").Body(
		app.H2().Class("auth-dialog__title").Body(app.Text("Editar serviço")),
		app.P().Class("auth-dialog__intro team-service-edit-intro").Body(app.Text(strings.Trim(strings.Join([]string{f.Customer, f.Appliance}, " • "), " •"))),
		app.Label().Class("auth-field team-service-edit-field").Body(app.Text("Tipo de serviço"), app.Select().Attr("value", f.Type).OnChange(p.ValueTo(&f.Type)).Body(options...)),
		field("Data do serviço", &f.Date, "date"), field("Valor (R$)", &f.Price, "number"),
		app.Div().Class("team-service-edit-grid").Body(
			app.Label().Class("auth-field team-service-edit-field").Body(app.Text("Garantia (dias)"), app.Select().Attr("value", f.WarrantyDays).OnChange(p.ValueTo(&f.WarrantyDays)).Body(warrantyOptions...)),
			app.Div().Class("auth-field team-service-edit-field").Body(app.Text("Status atual"), app.Strong().Body(app.Text(teamServiceStatusLabel(f.Status)))),
		),
		app.Label().Class("auth-field team-service-edit-field").Body(app.Text("Observações"), app.Textarea().Rows(4).Text(f.Notes).OnChange(p.ValueTo(&f.Notes))),
		app.P().Class("portal-section__intro").Body(app.Text("Para iniciar, concluir ou cancelar a OS, use a ação correspondente. Assim o histórico, o comprovante e os avisos seguem o fluxo correto.")),
		dateNotice, formErrorNotice(f.Message),
		app.Div().Class("portal-budget__actions team-service-edit-actions").Body(
			app.Button().Class("auth-link").Type("button").Disabled(f.Saving).OnClick(func(ctx app.Context, event app.Event) { event.PreventDefault(); p.teamServiceEdit = nil }).Body(app.Text("Cancelar")),
			app.Button().Class("auth-submit").Type("button").Disabled(f.Saving).OnClick(p.saveTeamServiceEdit).Body(app.Text("Salvar no Banco")),
		),
	))
}
