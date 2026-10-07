package webapp

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/domain"
)

func (p *serviceCatalogPage) openTeamBudgetSchedule(budget domain.BudgetEstimate) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		if budget.Status != domain.BudgetApproved || p.teamBudgetBusy || p.session == nil {
			return
		}
		p.teamBudgetScheduleID = budget.ID
		p.teamBudgetScheduleDate = teamCallScheduleDefaultDate(time.Now())
		p.teamBudgetScheduleTime = "09:00"
		p.teamBudgetScheduleMessage = ""
		ctx.Update()
	}
}

func (p *serviceCatalogPage) closeTeamBudgetSchedule(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if !p.teamBudgetBusy {
		p.teamBudgetScheduleID = ""
		p.teamBudgetScheduleMessage = ""
	}
	ctx.Update()
}

func (p *serviceCatalogPage) saveTeamBudgetSchedule(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.teamBudgetBusy || p.session == nil || p.teamBudgetScheduleID == "" {
		return
	}
	var budget *domain.BudgetEstimate
	for i := range p.teamBudgets {
		if p.teamBudgets[i].ID == p.teamBudgetScheduleID {
			budget = &p.teamBudgets[i]
			break
		}
	}
	if budget == nil || budget.Status != domain.BudgetApproved {
		p.teamBudgetScheduleMessage = "Este orçamento não está mais aprovado. Atualize a tela e tente novamente."
		ctx.Update()
		return
	}
	date, hour := p.teamBudgetScheduleDate, p.teamBudgetScheduleTime
	if _, err := time.Parse("2006-01-02", date); err != nil {
		p.teamBudgetScheduleMessage = "Informe uma data válida."
		ctx.Update()
		return
	}
	if _, err := time.Parse("15:04", hour); err != nil {
		p.teamBudgetScheduleMessage = "Informe um horário válido."
		ctx.Update()
		return
	}
	customer := findTeamCustomer(p.teamCustomers, budget.ClientID)
	if customer == nil {
		p.teamBudgetScheduleMessage = "Cliente do orçamento não encontrado no banco."
		ctx.Update()
		return
	}
	budgetSnapshot := *budget
	if _, err := uuid.Parse(budgetSnapshot.ID); err != nil {
		p.teamBudgetScheduleMessage = "Identificador do orçamento inválido. Atualize a tela e tente novamente."
		ctx.Update()
		return
	}
	if budgetSnapshot.ApplianceID != nil && *budgetSnapshot.ApplianceID != "" && !teamCustomerHasAppliance(customer, *budgetSnapshot.ApplianceID) {
		p.teamBudgetScheduleMessage = "O aparelho deste orçamento não pertence mais ao cliente. Atualize os dados antes de agendar."
		ctx.Update()
		return
	}
	var linkedService map[string]any
	if budgetSnapshot.ServiceID != nil && *budgetSnapshot.ServiceID != "" {
		for _, service := range p.teamServices {
			if portalText(service["id"]) == *budgetSnapshot.ServiceID && portalText(service["cliente_id"]) == budgetSnapshot.ClientID {
				linkedService = service
				break
			}
		}
	} else if budgetSnapshot.ApplianceID != nil && *budgetSnapshot.ApplianceID != "" {
		for _, service := range p.teamServices {
			if portalText(service["cliente_id"]) == budgetSnapshot.ClientID && portalText(service["aparelho_id"]) == *budgetSnapshot.ApplianceID && strings.EqualFold(portalText(service["status"]), "PENDENTE") {
				linkedService = service
				break
			}
		}
	}
	if linkedService != nil && (strings.EqualFold(portalText(linkedService["status"]), "CONCLUIDO") || strings.EqualFold(portalText(linkedService["status"]), "CANCELADO")) {
		p.teamBudgetScheduleMessage = "A OS deste orçamento já foi encerrada."
		ctx.Update()
		return
	}
	serviceDescription := budgetScheduledServiceDescription(budgetSnapshot)
	serviceType := budgetScheduledServiceType(budgetSnapshot)
	serviceID := ""
	var appointment map[string]any
	if linkedService != nil {
		serviceID = portalText(linkedService["id"])
		if _, err := uuid.Parse(serviceID); err != nil {
			p.teamBudgetScheduleMessage = "A OS vinculada tem um identificador inválido. Atualize os dados antes de agendar."
			ctx.Update()
			return
		}
		for _, row := range p.teamAppointments {
			if portalText(row["service_id"]) == serviceID {
				appointment = row
				break
			}
		}
	}
	token, baseURL := p.session.AccessToken, apiBaseURL()
	profile := p.teamProfile
	p.teamBudgetBusy, p.teamBudgetScheduleMessage = true, "Agendando serviço do orçamento..."
	ctx.Update()
	go func() {
		var err error
		if linkedService != nil {
			fields := map[string]any{"status": "AGENDADO", "data_agendamento": date, "hora_agendamento": hour, "valor": budgetSnapshot.FinalValue, "descricao": serviceDescription}
			err = sendTeamJSON(ctx, baseURL+"/api/servicos", token, http.MethodPatch, map[string]any{"id": serviceID, "fields": fields})
		} else {
			applianceID := budgetScheduledApplianceID(budgetSnapshot, customer)
			serviceFields := map[string]any{
				"cliente_id": budgetSnapshot.ClientID, "tipo": supabase.MapServiceTypeToSupabase(serviceType),
				"status": "AGENDADO", "valor": budgetSnapshot.FinalValue, "data_agendamento": date,
				"hora_agendamento": hour, "descricao": serviceDescription,
				"observacoes": fmt.Sprintf("Gerado do orçamento aprovado — valor R$ %.2f", budgetSnapshot.FinalValue),
			}
			if applianceID != "" {
				serviceFields["aparelho_id"] = applianceID
			}
			var created map[string]any
			created, err = sendTeamJSONResult(ctx, baseURL+"/api/servicos", token, http.MethodPost, serviceFields)
			if err == nil {
				serviceID = portalText(created["id"])
				if serviceID == "" {
					err = fmt.Errorf("resposta da OS sem identificador")
				}
			}
		}
		if err == nil {
			if budgetScheduleAppointmentMethod(linkedService != nil, appointment != nil) == http.MethodPatch {
				err = sendTeamJSON(ctx, baseURL+"/api/agendamentos", token, http.MethodPatch, map[string]any{
					"service_id": serviceID, "fields": map[string]any{"data": date, "hora": hour},
				})
			} else if serviceID != "" {
				appointmentFields := map[string]any{"service_id": serviceID, "cliente_id": budgetSnapshot.ClientID, "data": date, "hora": hour, "status": "AGENDADO"}
				err = sendTeamJSON(ctx, baseURL+"/api/agendamentos", token, http.MethodPost, appointmentFields)
				err = budgetScheduleAppointmentError(serviceID, err)
			} else {
				err = budgetScheduleAppointmentError(serviceID, nil)
			}
		}
		if p.session == nil || p.session.AccessToken != token {
			return
		}
		linkWarning := ""
		linkErr := error(nil)
		if (budgetSnapshot.ServiceID == nil || *budgetSnapshot.ServiceID == "") && serviceID != "" {
			_, linkErr = sendTeamJSONResult(ctx, baseURL+"/api/orcamento-equipe?id="+budgetSnapshot.ID, token, http.MethodPatch, map[string]any{
				"id": budgetSnapshot.ID, "acao": "LINK_SERVICE", "service_id": serviceID,
			})
			if linkErr != nil {
				linkWarning = " A OS foi criada, mas o orçamento ainda não foi vinculado. Atualize os dados antes de tentar novamente."
			}
		}
		if err == nil {
			err = linkErr
		}
		if err != nil {
			p.teamBudgetBusy = false
			p.teamBudgetScheduleMessage = "Não foi possível salvar o compromisso do orçamento na agenda. A OS pode já ter sido atualizada; sincronize antes de tentar novamente." + linkWarning
			p.teamBudgetScheduleID = ""
			p.teamLoadedFor = ""
			p.refreshTeamBudgetAndOperations(ctx, baseURL, token)
			ctx.Update()
			return
		}
		if err = budgetSchedulePersistenceError(serviceID, true, err); err != nil {
			p.teamBudgetBusy = false
			p.teamBudgetScheduleMessage = "Não foi possível salvar o compromisso do orçamento na agenda. A OS pode já ter sido atualizada; sincronize antes de tentar novamente." + linkWarning
			p.teamBudgetScheduleID = ""
			p.teamLoadedFor = ""
			p.refreshTeamBudgetAndOperations(ctx, baseURL, token)
			ctx.Update()
			return
		}
		waSent := false
		customerName, customerPhone := budgetContactValues(&teamBudgetForm{}, customer)
		if customerPhone != "" {
			message := budgetScheduledWhatsAppMessage(profile, budgetSnapshot, customerName, date, hour)
			waResult, waErr := sendTeamJSONResult(ctx, baseURL+"/api/whatsapp", token, http.MethodPost, map[string]any{
				"acao": "enviar", "evento": "servico_agendado_orcamento", "origem_tipo": "servico", "origem_id": budgetSnapshot.ServiceID, "telefone": customerPhone, "texto": message,
			})
			if waErr == nil {
				waSent, _ = waResult["ok"].(bool)
			}
		}
		p.teamBudgetBusy, p.teamBudgetScheduleID = false, ""
		p.teamBudgetScheduleMessage = ""
		p.teamBudgetMessage = "Serviço agendado para " + appointmentDateBR(date) + " às " + hour + "." + linkWarning
		p.registerLocalNotification(ctx, "Serviço agendado", appointmentDateBR(date)+" às "+hour)
		if waSent {
			p.teamBudgetMessage += " Aviso registrado na fila do WhatsApp."
		} else if customerPhone != "" {
			p.teamBudgetMessage += " Não foi possível enviar a confirmação pelo WhatsApp."
		}
		p.teamLoadedFor = ""
		p.refreshTeamBudgetAndOperations(ctx, baseURL, token)
		ctx.Update()
	}()
}

func budgetScheduleAppointmentError(serviceID string, saveErr error) error {
	if strings.TrimSpace(serviceID) == "" {
		return fmt.Errorf("OS do orçamento não foi identificada")
	}
	return saveErr
}

func budgetScheduleAppointmentMethod(serviceLinked, appointmentExists bool) string {
	if serviceLinked && appointmentExists {
		return http.MethodPatch
	}
	return http.MethodPost
}

func budgetSchedulePersistenceError(serviceID string, appointmentExists bool, saveErr error) error {
	if saveErr != nil {
		return saveErr
	}
	if strings.TrimSpace(serviceID) == "" {
		return fmt.Errorf("OS do orçamento não foi identificada")
	}
	if !appointmentExists {
		return fmt.Errorf("compromisso não foi salvo")
	}
	return nil
}

func (p *serviceCatalogPage) refreshTeamBudgetAndOperations(ctx app.Context, baseURL, token string) {
	go func() {
		budgets, budgetErr := getTeamBudgets(ctx, baseURL+"/api/orcamentos", token)
		services, serviceErr := getTeamRows(ctx, baseURL+"/api/servicos", token)
		if p.session == nil || p.session.AccessToken != token {
			return
		}
		if budgetErr == nil {
			p.teamBudgets = budgets
		}
		if serviceErr == nil {
			p.teamServices = services
		}
		p.teamLoadedFor = ""
		p.loadTeamOperations(ctx)
		ctx.Update()
	}()
}

func teamCustomerHasAppliance(customer map[string]any, applianceID string) bool {
	for _, appliance := range applianceMaps(customer) {
		if portalText(appliance["id"]) == applianceID {
			return true
		}
	}
	return false
}

func isValidBudgetApplianceSelection(customer map[string]any, applianceID string) bool {
	if lenTeamCustomerAppliances(customer) == 0 {
		return applianceID == ""
	}
	return applianceID != "" && teamCustomerHasAppliance(customer, applianceID)
}

func budgetScheduledServiceDescription(budget domain.BudgetEstimate) string {
	if len(budget.Items) > 0 && strings.TrimSpace(budget.Items[0].Description) != "" {
		return budget.Items[0].Description
	}
	if strings.TrimSpace(budget.ApplianceDescription) != "" {
		return budget.ApplianceDescription
	}
	return "Serviço aprovado (orçamento)"
}

func budgetScheduledServiceType(budget domain.BudgetEstimate) string {
	if len(budget.Items) > 0 && strings.TrimSpace(budget.Items[0].Description) != "" {
		return budget.Items[0].Description
	}
	return "OUTRO"
}

func budgetScheduledApplianceID(budget domain.BudgetEstimate, customer map[string]any) string {
	if budget.ApplianceID != nil && *budget.ApplianceID != "" {
		return *budget.ApplianceID
	}
	appliances := applianceMaps(customer)
	if len(appliances) == 1 {
		return portalText(appliances[0]["id"])
	}
	return ""
}

func budgetScheduledWhatsAppMessage(profile domain.TechnicianProfile, budget domain.BudgetEstimate, customerName, date, hour string) string {
	name := strings.TrimSpace(customerName)
	if parts := strings.Fields(name); len(parts) > 0 {
		name = parts[0]
	}
	message := domain.ResolveWhatsAppTemplate(profile.WhatsAppMessages, "orcamento_agendado")
	return domain.ApplyWhatsAppPlaceholders(message, map[string]string{
		"cliente": name, "data": appointmentDateBR(date), "hora": hour,
		"valor":   "R$ " + fmt.Sprintf("%.2f", budget.FinalValue),
		"empresa": firstNonEmptyBudget(profile.BusinessName, "Inovar Refrigeração"),
		"app":     "https://inovarapp.vercel.app",
	})
}

func (p *serviceCatalogPage) teamBudgetScheduleDialog() app.UI {
	var budget *domain.BudgetEstimate
	for i := range p.teamBudgets {
		if p.teamBudgets[i].ID == p.teamBudgetScheduleID {
			budget = &p.teamBudgets[i]
			break
		}
	}
	if budget == nil {
		return app.Div()
	}
	service := "Serviço aprovado"
	if len(budget.Items) > 0 {
		service = budget.Items[0].Description
	} else if budget.ApplianceDescription != "" {
		service = budget.ApplianceDescription
	}
	return app.Div().Class("auth-backdrop").Body(app.Div().Class("auth-dialog").Body(
		app.H2().Class("auth-dialog__title").Body(app.Text("Agendar Atendimento")),
		app.P().Class("auth-dialog__intro").Body(app.Text(budget.ClientName+" • "+service)),
		app.Form().Class("auth-form").OnSubmit(p.saveTeamBudgetSchedule).Body(
			app.Div().Class("service-grid").Body(
				app.Label().Class("auth-field").Body(app.Text("Data do Atendimento *"), app.Input().Type("date").Required(true).Value(p.teamBudgetScheduleDate).Disabled(p.teamBudgetBusy).OnChange(p.ValueTo(&p.teamBudgetScheduleDate))),
				app.Label().Class("auth-field").Body(app.Text("Horário *"), app.Input().Type("time").Required(true).Value(p.teamBudgetScheduleTime).Disabled(p.teamBudgetBusy).OnChange(p.ValueTo(&p.teamBudgetScheduleTime))),
			),
			app.P().Class("portal-section__intro").Body(app.Text("Ao salvar, o serviço entra na Agenda e o cliente recebe uma confirmação pelo WhatsApp se a integração estiver disponível.")),
			formErrorNotice(p.teamBudgetScheduleMessage),
			app.Div().Class("portal-budget__actions").Body(
				app.Button().Class("auth-link").Type("button").Disabled(p.teamBudgetBusy).OnClick(p.closeTeamBudgetSchedule).Body(app.Text("Cancelar")),
				app.Button().Class("auth-submit").Type("submit").Disabled(p.teamBudgetBusy).Body(app.Text("Confirmar Agendamento")),
			),
		),
	))
}
