package webapp

import (
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/domain"
)

type teamAppointmentForm struct {
	ClientID, ApplianceID     string
	Date, Time, ReturnDate    string
	ServiceType, Price, Notes string
	Message                   string
}

func newTeamAppointmentForm(profile domain.TechnicianProfile, customers []map[string]any, now time.Time) *teamAppointmentForm {
	today := now.Format("2006-01-02")
	price := profile.DefaultPrice
	if price <= 0 {
		price = 250
	}
	f := &teamAppointmentForm{
		Date: today, Time: "09:00", ReturnDate: domain.AddMonthsClamped(now, 6).Format("2006-01-02"),
		ServiceType: string(domain.ServiceCleaning), Price: strconv.FormatFloat(price, 'f', -1, 64),
	}
	for _, customer := range customers {
		appliances, _ := customer["appliances"].([]any)
		if f.ClientID == "" {
			f.ClientID = portalText(customer["id"])
		}
		for _, raw := range appliances {
			appliance, _ := raw.(map[string]any)
			if appliance == nil {
				continue
			}
			if f.ClientID == portalText(customer["id"]) {
				f.ApplianceID = portalText(appliance["id"])
				return f
			}
		}
	}
	return f
}

func newTeamAppointmentFormForAppliance(profile domain.TechnicianProfile, customers []map[string]any, clientID, applianceID string, now time.Time) *teamAppointmentForm {
	form := newTeamAppointmentForm(profile, customers, now)
	form.ClientID, form.ApplianceID = clientID, applianceID
	return form
}

func (p *serviceCatalogPage) openTeamAppointmentForAppliance(item teamReturnItem) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		if p.caller == nil || p.session == nil {
			return
		}
		p.teamNewAppointment = newTeamAppointmentFormForAppliance(p.teamProfile, p.teamCustomers, item.CustomerID, item.ApplianceID, time.Now())
		p.teamNewAppointmentNotice = ""
		ctx.Update()
	}
}

func (p *serviceCatalogPage) openTeamAppointmentForm(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.caller == nil || p.session == nil || (p.caller.Role != domain.RoleAdmin && p.caller.Role != domain.RoleTechnician) {
		return
	}
	p.teamNewAppointment = newTeamAppointmentForm(p.teamProfile, p.teamCustomers, time.Now())
	p.teamNewAppointmentNotice = ""
	ctx.Update()
}

func (p *serviceCatalogPage) selectAppointmentClient(ctx app.Context, event app.Event) {
	f := p.teamNewAppointment
	if f == nil {
		return
	}
	f.ClientID = event.Get("target").Get("value").String()
	f.ApplianceID = ""
	for _, customer := range p.teamCustomers {
		if portalText(customer["id"]) != f.ClientID {
			continue
		}
		appliances, _ := customer["appliances"].([]any)
		for _, raw := range appliances {
			if appliance, ok := raw.(map[string]any); ok {
				f.ApplianceID = portalText(appliance["id"])
				if f.ApplianceID != "" {
					break
				}
			}
		}
		break
	}
	ctx.Update()
}

func (p *serviceCatalogPage) selectAppointmentType(ctx app.Context, event app.Event) {
	f := p.teamNewAppointment
	if f == nil {
		return
	}
	f.ServiceType = event.Get("target").Get("value").String()
	for _, entry := range domain.BuildServiceCatalog(&p.teamProfile) {
		if entry.Name == f.ServiceType && entry.Price > 0 {
			f.Price = strconv.FormatFloat(entry.Price, 'f', -1, 64)
			break
		}
	}
	ctx.Update()
}

func (p *serviceCatalogPage) saveTeamAppointment(ctx app.Context, event app.Event) {
	event.PreventDefault()
	f := p.teamNewAppointment
	if f == nil || p.session == nil || p.teamNewAppointmentSaving {
		return
	}
	price, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(f.Price), ",", "."), 64)
	if f.ClientID == "" || f.ApplianceID == "" {
		f.Message = "Selecione um cliente que tenha aparelho cadastrado."
		ctx.Update()
		return
	}
	if _, dateErr := time.Parse("2006-01-02", f.Date); dateErr != nil {
		f.Message = "Informe uma data de serviço válida."
		ctx.Update()
		return
	}
	if _, timeErr := time.Parse("15:04", f.Time); timeErr != nil {
		f.Message = "Informe um horário válido."
		ctx.Update()
		return
	}
	if _, returnDateErr := time.Parse("2006-01-02", f.ReturnDate); returnDateErr != nil {
		f.Message = "Informe uma data de retorno válida."
		ctx.Update()
		return
	}
	if f.ServiceType == "" || err != nil || math.IsNaN(price) || math.IsInf(price, 0) || price < 0 {
		f.Message = "Informe o tipo de serviço e um valor válido."
		ctx.Update()
		return
	}
	if !appointmentApplianceBelongsToClient(p.teamCustomers, f.ClientID, f.ApplianceID) {
		f.Message = "O aparelho selecionado não pertence ao cliente. Atualize os dados e tente novamente."
		ctx.Update()
		return
	}
	client := appointmentClient(p.teamCustomers, f.ClientID)
	if client == nil {
		f.Message = "Não foi possível identificar o cliente selecionado."
		ctx.Update()
		return
	}
	pageURL := app.Window().URL()
	if pageURL == nil || pageURL.Scheme == "" || pageURL.Host == "" {
		return
	}
	formSnapshot := *f
	token, baseURL := p.session.AccessToken, apiBaseURL()
	serviceType := strings.TrimSpace(formSnapshot.ServiceType)
	servicePayload := map[string]any{
		"cliente_id": formSnapshot.ClientID, "aparelho_id": formSnapshot.ApplianceID,
		"tipo": supabase.MapServiceTypeToSupabase(serviceType), "status": "AGENDADO",
		"valor": price, "data_agendamento": formSnapshot.Date, "hora_agendamento": formSnapshot.Time,
		"descricao": serviceType, "observacoes": strings.TrimSpace(formSnapshot.Notes),
	}
	if servicePayload["observacoes"] == "" {
		servicePayload["observacoes"] = "Retorno agendado pelo painel técnico."
	}
	p.teamNewAppointmentSaving, f.Message = true, "Salvando agendamento..."
	ctx.Update()
	go func() {
		result, saveErr := sendTeamJSONResult(ctx, baseURL+"/api/servicos", token, http.MethodPost, servicePayload)
		if p.session == nil || p.session.AccessToken != token {
			return
		}
		if saveErr != nil {
			p.teamNewAppointmentSaving = false
			if p.teamNewAppointment != nil {
				p.teamNewAppointment.Message = "Não foi possível salvar o agendamento. O formulário foi mantido para tentar novamente."
			}
			ctx.Update()
			return
		}
		serviceID := portalText(result["id"])
		appointmentPayload := map[string]any{"service_id": serviceID, "cliente_id": formSnapshot.ClientID, "data": formSnapshot.Date, "hora": formSnapshot.Time, "status": "AGENDADO"}
		if notes := strings.TrimSpace(formSnapshot.Notes); notes != "" {
			appointmentPayload["observacoes"] = notes
		}
		appointmentErr := error(nil)
		if serviceID != "" {
			appointmentErr = sendTeamJSON(ctx, baseURL+"/api/agendamentos", token, http.MethodPost, appointmentPayload)
		}
		whatsSent := false
		if appointmentConfirmationCanBeSent(serviceID, portalText(client["whatsapp"]), appointmentErr) {
			message := domain.ApplyWhatsAppPlaceholders(domain.ResolveWhatsAppTemplate(p.teamProfile.WhatsAppMessages, "agendamento_criado"), map[string]string{
				"cliente": strings.Split(strings.TrimSpace(portalText(client["nome"])), " ")[0],
				"data":    appointmentDateBR(formSnapshot.Date), "servico": serviceType,
				"empresa": firstNonEmptyBudget(p.teamProfile.BusinessName, "Inovar Refrigeração"),
			})
			waResult, waErr := sendTeamJSONResult(ctx, baseURL+"/api/whatsapp", token, http.MethodPost, map[string]any{"acao": "enviar", "evento": "agendamento_criado", "origem_tipo": "servico", "origem_id": serviceID, "telefone": client["whatsapp"], "texto": message})
			if waErr == nil {
				whatsSent, _ = waResult["ok"].(bool)
			}
		}
		p.teamNewAppointmentSaving = false
		p.teamNewAppointment = nil
		if appointmentErr != nil || serviceID == "" {
			p.teamNewAppointmentNotice = "A OS foi agendada, mas não foi possível salvar o compromisso da agenda. Tente sincronizar novamente."
		} else {
			p.teamNewAppointmentNotice = "Agendamento confirmado."
			p.registerLocalNotification(ctx, "Agendamento confirmado", serviceType+" em "+appointmentDateBR(formSnapshot.Date)+" — consulte a Agenda.")
			if whatsSent {
				p.teamNewAppointmentNotice += " Confirmação registrada na fila do WhatsApp."
			} else if portalText(client["whatsapp"]) != "" {
				p.teamNewAppointmentNotice += " Não foi possível enviar a confirmação pelo WhatsApp."
			}
		}
		p.teamLoadedFor = ""
		p.loadTeamOperations(ctx)
		ctx.Update()
	}()
}

func appointmentDateBR(value string) string {
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return value
	}
	return date.Format("02/01/2006")
}

func appointmentConfirmationCanBeSent(serviceID, phone string, appointmentErr error) bool {
	return appointmentErr == nil && strings.TrimSpace(serviceID) != "" && strings.TrimSpace(phone) != ""
}

func appointmentClient(customers []map[string]any, clientID string) map[string]any {
	for _, customer := range customers {
		if portalText(customer["id"]) == clientID {
			return customer
		}
	}
	return nil
}

func appointmentApplianceBelongsToClient(customers []map[string]any, clientID, applianceID string) bool {
	client := appointmentClient(customers, clientID)
	if client == nil {
		return false
	}
	appliances, _ := client["appliances"].([]any)
	for _, raw := range appliances {
		if appliance, ok := raw.(map[string]any); ok && portalText(appliance["id"]) == applianceID {
			return true
		}
	}
	return false
}

func (p *serviceCatalogPage) teamAppointmentDialog() app.UI {
	f := p.teamNewAppointment
	if f == nil {
		return app.Div()
	}
	clientOptions := []app.UI{app.Option().Value("").Body(app.Text("Selecione um cliente"))}
	for _, customer := range p.teamCustomers {
		clientOptions = append(clientOptions, app.Option().Value(portalText(customer["id"])).Body(app.Text(portalText(customer["nome"]))))
	}
	selected := appointmentClient(p.teamCustomers, f.ClientID)
	applianceOptions := []app.UI{app.Option().Value("").Body(app.Text("Selecione um aparelho"))}
	if selected != nil {
		if appliances, ok := selected["appliances"].([]any); ok {
			for _, raw := range appliances {
				appliance, _ := raw.(map[string]any)
				if appliance == nil {
					continue
				}
				label := strings.TrimSpace(portalText(appliance["marca"]) + " " + portalText(appliance["btus"]) + " BTUs (" + portalText(appliance["ambiente"]) + ")")
				applianceOptions = append(applianceOptions, app.Option().Value(portalText(appliance["id"])).Body(app.Text(label)))
			}
		}
	}
	typeOptions := make([]app.UI, 0)
	for _, entry := range domain.BuildServiceCatalog(&p.teamProfile) {
		typeOptions = append(typeOptions, app.Option().Value(entry.Name).Body(app.Text(entry.Name)))
	}
	canSave := f.ClientID != "" && f.ApplianceID != "" && !p.teamNewAppointmentSaving
	fields := []app.UI{
		app.Label().Class("auth-field team-appointment-field").Body(app.Text("Cliente"), app.Select().Attr("value", f.ClientID).Required(true).Disabled(p.teamNewAppointmentSaving).OnChange(p.selectAppointmentClient).Body(clientOptions...)),
		app.Label().Class("auth-field team-appointment-field").Body(app.Text("Aparelho do Cliente"), app.Select().Attr("value", f.ApplianceID).Required(true).Disabled(p.teamNewAppointmentSaving).OnChange(p.ValueTo(&f.ApplianceID)).Body(applianceOptions...)),
		app.Div().Class("team-appointment-date-grid").Body(
			app.Label().Class("auth-field team-appointment-field").Body(app.Text("Data do Serviço"), app.Input().Type("date").Required(true).Value(f.Date).Disabled(p.teamNewAppointmentSaving).OnChange(p.ValueTo(&f.Date))),
			app.Label().Class("auth-field team-appointment-field").Body(app.Text("Horário"), app.Input().Type("time").Required(true).Value(f.Time).Disabled(p.teamNewAppointmentSaving).OnChange(p.ValueTo(&f.Time))),
			app.Label().Class("auth-field team-appointment-field").Body(app.Text("Data do Retorno"), app.Input().Type("date").Required(true).Value(f.ReturnDate).Disabled(p.teamNewAppointmentSaving).OnChange(p.ValueTo(&f.ReturnDate))),
		),
		app.Div().Class("team-appointment-service-grid").Body(
			app.Label().Class("auth-field team-appointment-field").Body(app.Text("Tipo de Serviço"), app.Select().Attr("value", f.ServiceType).Disabled(p.teamNewAppointmentSaving).OnChange(p.selectAppointmentType).Body(typeOptions...)),
			app.Label().Class("auth-field team-appointment-field").Body(app.Text("Valor Estimado (R$)"), app.Input().Type("number").Attr("min", "0").Attr("step", "0.01").Value(f.Price).Disabled(p.teamNewAppointmentSaving).OnChange(p.ValueTo(&f.Price))),
		),
		app.Label().Class("auth-field team-appointment-field").Body(app.Text("Anotações do Agendamento"), app.Textarea().Rows(3).Text(f.Notes).Placeholder("Ex: Ligar antes para confirmar se o cliente estará em casa...").Disabled(p.teamNewAppointmentSaving).OnChange(p.ValueTo(&f.Notes))),
	}
	if f.Message != "" {
		fields = append(fields, app.P().Class("portal-section__error").Body(app.Text(f.Message)))
	}
	if len(applianceOptions) <= 1 {
		fields = append(fields, app.P().Class("auth-notice").Body(app.Text("Este cliente ainda não possui aparelhos cadastrados. Cadastre um aparelho primeiro.")))
	}
	fields = append(fields, app.Div().Class("portal-budget__actions team-appointment-actions").Body(
		app.Button().Class("auth-link").Type("button").Disabled(p.teamNewAppointmentSaving).OnClick(func(ctx app.Context, event app.Event) {
			event.PreventDefault()
			p.teamNewAppointment = nil
			ctx.Update()
		}).Body(app.Text("Cancelar")),
		app.Button().Class("auth-submit").Type("submit").Disabled(!canSave).Body(app.Text("Salvar Agendamento")),
	))
	return app.Div().Class("auth-backdrop team-appointment-backdrop").Body(app.Div().Class("auth-dialog team-appointment-dialog").Body(
		app.H2().Class("auth-dialog__title").Body(app.Text("Agendar Retorno / Manutenção")),
		app.P().Class("auth-dialog__intro").Body(app.Text("Defina o cliente e a data prevista de retorno")),
		app.Form().Class("auth-form team-appointment-form").OnSubmit(p.saveTeamAppointment).Body(fields...),
	))
}
