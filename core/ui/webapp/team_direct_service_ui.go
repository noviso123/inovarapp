package webapp

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/domain"
)

type teamDirectServiceForm struct {
	ClientID, ApplianceID                   string
	ServiceType, Date, Price, Status, Notes string
	ServiceID                               string
	Message                                 string
}

func newTeamDirectServiceForm(customers []map[string]any, now time.Time) *teamDirectServiceForm {
	form := &teamDirectServiceForm{
		ServiceType: string(domain.ServiceCleaning),
		Date:        now.UTC().Add(24 * time.Hour).Format("2006-01-02"),
		Price:       "250", Status: "AGENDADO",
	}
	if len(customers) > 0 {
		form.ClientID = portalText(customers[0]["id"])
		appliances, _ := customers[0]["appliances"].([]any)
		if len(appliances) > 0 {
			if appliance, ok := appliances[0].(map[string]any); ok {
				form.ApplianceID = portalText(appliance["id"])
			}
		}
	}
	return form
}

func teamDirectServicePayload(form teamDirectServiceForm, price float64) map[string]any {
	serviceType := strings.TrimSpace(form.ServiceType)
	return map[string]any{
		"cliente_id": form.ClientID, "aparelho_id": form.ApplianceID,
		"tipo": supabase.MapServiceTypeToSupabase(serviceType), "descricao": serviceType,
		"status": form.Status, "valor": price, "data_agendamento": form.Date,
		"observacoes": strings.TrimSpace(form.Notes),
	}
}

func teamDirectAppointmentPayload(form teamDirectServiceForm, serviceID string) map[string]any {
	return map[string]any{
		"service_id": serviceID, "cliente_id": form.ClientID, "data": form.Date,
		"status": "AGENDADO", "observacoes": strings.TrimSpace(form.Notes),
	}
}

func teamDirectAppointmentMethod(existingService, appointmentExists bool) string {
	if existingService && appointmentExists {
		return http.MethodPatch
	}
	return http.MethodPost
}

func teamDirectServiceMethod(form teamDirectServiceForm) string {
	if strings.TrimSpace(form.ServiceID) != "" {
		return http.MethodPatch
	}
	return http.MethodPost
}

func (p *serviceCatalogPage) openTeamDirectServiceForm(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.caller == nil || p.session == nil || (p.caller.Role != domain.RoleAdmin && p.caller.Role != domain.RoleTechnician) {
		return
	}
	p.teamDirectServiceForm = newTeamDirectServiceForm(p.teamCustomers, time.Now())
	p.teamActionMessage = ""
	ctx.Update()
}

func (p *serviceCatalogPage) selectDirectServiceClient(ctx app.Context, event app.Event) {
	f := p.teamDirectServiceForm
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

func (p *serviceCatalogPage) selectDirectServiceType(ctx app.Context, event app.Event) {
	f := p.teamDirectServiceForm
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

func (p *serviceCatalogPage) saveTeamDirectService(ctx app.Context, event app.Event) {
	event.PreventDefault()
	f := p.teamDirectServiceForm
	if f == nil || p.session == nil || p.teamDirectServiceSaving {
		return
	}
	price, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(f.Price), ",", "."), 64)
	if f.ClientID == "" || f.ApplianceID == "" || !appointmentApplianceBelongsToClient(p.teamCustomers, f.ClientID, f.ApplianceID) {
		f.Message = "Este cliente não tem aparelhos. Cadastre o aparelho dele primeiro."
		ctx.Update()
		return
	}
	if _, dateErr := time.Parse("2006-01-02", f.Date); dateErr != nil {
		f.Message = "Informe uma data válida."
		ctx.Update()
		return
	}
	if err != nil || math.IsNaN(price) || math.IsInf(price, 0) || price < 0 {
		f.Message = "Informe um valor válido."
		ctx.Update()
		return
	}
	if f.ServiceType == "" || (f.Status != "AGENDADO" && f.Status != "CONCLUIDO" && f.Status != "PENDENTE") {
		f.Message = "Informe um tipo de serviço e status válidos."
		ctx.Update()
		return
	}
	client := appointmentClient(p.teamCustomers, f.ClientID)
	if client == nil {
		f.Message = "Não foi possível identificar o cliente selecionado."
		ctx.Update()
		return
	}
	snapshot, profile := *f, p.teamProfile
	token, baseURL := p.session.AccessToken, apiBaseURL()
	serviceID := snapshot.ServiceID
	if serviceID == "" {
		serviceID = uuid.NewString()
	}
	servicePayload := teamDirectServicePayload(snapshot, price)
	servicePayload["id"] = serviceID
	var appointmentPayload map[string]any
	if snapshot.Status == "AGENDADO" {
		appointmentPayload = teamDirectAppointmentPayload(snapshot, serviceID)
	}
	p.teamDirectServiceSaving, f.Message = true, "Salvando serviço..."
	ctx.Update()
	go func() {
		var saveErr error
		if p.offlineMode && teamDirectServiceMethod(snapshot) == http.MethodPost {
			saveErr = enqueueOfflineTeamService(p.caller.UserID, p.caller.Role, serviceID, servicePayload, appointmentPayload)
			if saveErr == nil {
				p.teamServices, p.teamAppointments = mergePendingOfflineServices(p.teamServices, p.teamAppointments, []offlineTeamService{{ServiceID: serviceID, Service: servicePayload, Appointment: appointmentPayload}})
				p.teamDashboard = buildTeamDashboardSummary(p.teamCustomers, p.teamServices, p.teamAppointments, p.teamBudgets, p.teamReturnHistory, p.teamProfile.DefaultReturnMonths, dashboardCivilToday(time.Now()))
				p.offlineMode = true
				p.teamDirectServiceSaving, p.teamDirectServiceForm = false, nil
				p.teamActionMessage = "Serviço e agendamento salvos neste dispositivo. Serão sincronizados quando a conexão voltar."
				p.retryPendingOfflineMutations(ctx)
				ctx.Update()
				return
			}
		} else if teamDirectServiceMethod(snapshot) == http.MethodPost {
			var service map[string]any
			service, saveErr = sendTeamJSONResult(ctx, baseURL+"/api/servicos", token, http.MethodPost, servicePayload)
			if saveErr == nil {
				serviceID = portalText(service["id"])
				if serviceID == "" {
					saveErr = fmt.Errorf("resposta da OS sem identificador")
				}
			}
		} else {
			fields := teamDirectServicePayload(snapshot, price)
			delete(fields, "cliente_id")
			delete(fields, "aparelho_id")
			saveErr = sendTeamJSON(ctx, baseURL+"/api/servicos", token, http.MethodPatch, map[string]any{"id": serviceID, "fields": fields})
		}
		if p.session == nil || p.session.AccessToken != token {
			return
		}
		if saveErr != nil {
			if isOfflineNetworkError(saveErr) && teamDirectServiceMethod(snapshot) == http.MethodPost {
				if queueErr := enqueueOfflineTeamService(p.caller.UserID, p.caller.Role, serviceID, servicePayload, appointmentPayload); queueErr == nil {
					p.teamServices, p.teamAppointments = mergePendingOfflineServices(p.teamServices, p.teamAppointments, []offlineTeamService{{ServiceID: serviceID, Service: servicePayload, Appointment: appointmentPayload}})
					p.teamDirectServiceSaving, p.teamDirectServiceForm = false, nil
					p.offlineMode = true
					p.teamActionMessage = "Serviço salvo neste dispositivo e aguardando sincronização."
					p.teamLoadedFor = ""
					p.teamDashboard = buildTeamDashboardSummary(p.teamCustomers, p.teamServices, p.teamAppointments, p.teamBudgets, p.teamReturnHistory, p.teamProfile.DefaultReturnMonths, dashboardCivilToday(time.Now()))
					p.retryPendingOfflineMutations(ctx)
					ctx.Update()
					return
				}
			}
			p.teamDirectServiceSaving = false
			if p.teamDirectServiceForm != nil {
				p.teamDirectServiceForm.ServiceID = serviceID
				p.teamDirectServiceForm.Message = "Erro ao cadastrar serviço. O formulário foi mantido para tentar novamente."
			}
			ctx.Update()
			return
		}
		appointmentErr := error(nil)
		if snapshot.Status == "AGENDADO" && serviceID != "" {
			appointments, loadErr := getTeamRows(ctx, baseURL+"/api/agendamentos", token)
			if loadErr != nil {
				appointmentErr = loadErr
			} else {
				appointmentErr = saveTeamDirectAppointment(ctx, baseURL, token, snapshot, serviceID, appointments)
			}
			if appointmentErr != nil {
				if isOfflineNetworkError(appointmentErr) {
					servicePayload["id"] = serviceID
					if queueErr := enqueueOfflineTeamService(p.caller.UserID, p.caller.Role, serviceID, servicePayload, appointmentPayload); queueErr == nil {
						p.teamServices, p.teamAppointments = mergePendingOfflineServices(p.teamServices, p.teamAppointments, []offlineTeamService{{ServiceID: serviceID, Service: servicePayload, Appointment: appointmentPayload}})
						p.teamDirectServiceSaving, p.teamDirectServiceForm = false, nil
						p.offlineMode = true
						p.teamActionMessage = "Serviço salvo. O agendamento ficou pendente e será sincronizado quando a conexão voltar."
						p.teamLoadedFor = ""
						p.teamDashboard = buildTeamDashboardSummary(p.teamCustomers, p.teamServices, p.teamAppointments, p.teamBudgets, p.teamReturnHistory, p.teamProfile.DefaultReturnMonths, dashboardCivilToday(time.Now()))
						p.retryPendingOfflineMutations(ctx)
						ctx.Update()
						return
					}
				}
				p.teamDirectServiceSaving = false
				if p.teamDirectServiceForm != nil {
					p.teamDirectServiceForm.ServiceID = serviceID
					p.teamDirectServiceForm.Message = "Serviço cadastrado, mas o agendamento não foi salvo. Tente novamente para completar a agenda."
				}
				p.teamActionMessage = ""
				p.teamLoadedFor = ""
				p.loadTeamOperations(ctx)
				ctx.Update()
				return
			}
		}
		whatsSent := false
		if snapshot.Status != "CANCELADO" && portalText(client["whatsapp"]) != "" {
			message := registeredServiceMessage(profile, portalText(client["nome"]), strings.TrimSpace(snapshot.ServiceType), snapshot.Date)
			waResult, waErr := sendTeamJSONResult(ctx, baseURL+"/api/whatsapp", token, http.MethodPost, map[string]any{
				"acao": "enviar", "evento": "servico_cadastrado", "origem_tipo": "servico", "origem_id": serviceID, "telefone": client["whatsapp"], "texto": message,
			})
			if waErr == nil {
				whatsSent, _ = waResult["ok"].(bool)
			}
		}
		p.teamDirectServiceSaving = false
		p.teamDirectServiceForm = nil
		p.registerLocalNotification(ctx, "Serviço cadastrado", "O serviço foi salvo no banco e atualizado na agenda quando aplicável.")
		if snapshot.Status == "AGENDADO" && (appointmentErr != nil || serviceID == "") {
			p.teamActionMessage = "Serviço cadastrado, mas não foi possível incluir o compromisso na agenda."
		} else {
			p.teamActionMessage = "Serviço cadastrado e atualizado na agenda quando aplicável."
		}
		if whatsSent {
			p.teamActionMessage += " Notificação registrada na fila do WhatsApp."
		} else if snapshot.Status != "CANCELADO" && portalText(client["whatsapp"]) != "" {
			p.teamActionMessage += " Não foi possível enviar a notificação pelo WhatsApp."
		}
		p.teamLoadedFor = ""
		p.loadTeamOperations(ctx)
		ctx.Update()
	}()
}

func saveTeamDirectAppointment(ctx context.Context, baseURL, token string, form teamDirectServiceForm, serviceID string, appointments []map[string]any) error {
	if serviceID == "" {
		return fmt.Errorf("OS do serviço não identificada")
	}
	method := teamDirectAppointmentMethod(strings.TrimSpace(form.ServiceID) != "", teamDirectAppointmentExists(appointments, serviceID))
	return sendTeamJSON(ctx, baseURL+"/api/agendamentos", token, method, teamDirectAppointmentPayload(form, serviceID))
}

func teamDirectAppointmentExists(appointments []map[string]any, serviceID string) bool {
	for _, appointment := range appointments {
		if portalText(appointment["service_id"]) == serviceID {
			return true
		}
	}
	return false
}

func registeredServiceMessage(profile domain.TechnicianProfile, clientName, serviceType, date string) string {
	name := strings.TrimSpace(clientName)
	if parts := strings.Fields(name); len(parts) > 0 {
		name = parts[0]
	}
	return domain.ApplyWhatsAppPlaceholders(domain.ResolveWhatsAppTemplate(profile.WhatsAppMessages, "servico_registrado"), map[string]string{
		"cliente": name, "servico": serviceType, "data": appointmentDateBR(date),
		"empresa": firstNonEmptyBudget(profile.BusinessName, "Inovar Refrigeração"),
	})
}

func (p *serviceCatalogPage) teamDirectServiceDialog() app.UI {
	f := p.teamDirectServiceForm
	if f == nil {
		return app.Div()
	}
	clientOptions := []app.UI{app.Option().Value("").Body(app.Text("Selecione um cliente"))}
	for _, customer := range p.teamCustomers {
		clientOptions = append(clientOptions, app.Option().Value(portalText(customer["id"])).Body(app.Text(portalText(customer["nome"]))))
	}
	selected := appointmentClient(p.teamCustomers, f.ClientID)
	applianceOptions := []app.UI{}
	if selected != nil {
		if appliances, ok := selected["appliances"].([]any); ok {
			for _, raw := range appliances {
				appliance, _ := raw.(map[string]any)
				if appliance == nil {
					continue
				}
				label := strings.TrimSpace(portalText(appliance["marca"]) + " " + portalText(appliance["modelo"]) + " " + portalText(appliance["btus"]) + " BTUs (" + portalText(appliance["ambiente"]) + ")")
				applianceOptions = append(applianceOptions, app.Option().Value(portalText(appliance["id"])).Body(app.Text(label)))
			}
		}
	}
	types := make([]app.UI, 0)
	for _, entry := range domain.BuildServiceCatalog(&p.teamProfile) {
		types = append(types, app.Option().Value(entry.Name).Body(app.Text(entry.Name)))
	}
	statuses := []app.UI{
		app.Option().Value("AGENDADO").Body(app.Text("Agendado")),
		app.Option().Value("CONCLUIDO").Body(app.Text("Concluído")),
		app.Option().Value("PENDENTE").Body(app.Text("Pendente")),
	}
	canSave := f.ApplianceID != "" && !p.teamDirectServiceSaving
	fields := []app.UI{
		app.Label().Class("auth-field").Body(app.Text("Cliente *"), app.Select().Attr("value", f.ClientID).Disabled(p.teamDirectServiceSaving).OnChange(p.selectDirectServiceClient).Body(clientOptions...)),
		app.Label().Class("auth-field").Body(app.Text("Aparelho * (obrigatório)"), app.Select().Attr("value", f.ApplianceID).Disabled(p.teamDirectServiceSaving).OnChange(p.ValueTo(&f.ApplianceID)).Body(applianceOptions...)),
		app.Label().Class("auth-field").Body(app.Text("Tipo de Serviço *"), app.Select().Attr("value", f.ServiceType).Disabled(p.teamDirectServiceSaving).OnChange(p.selectDirectServiceType).Body(types...)),
		app.Div().Class("service-grid").Body(
			app.Label().Class("auth-field").Body(app.Text("Data *"), app.Input().Type("date").Required(true).Value(f.Date).Disabled(p.teamDirectServiceSaving).OnChange(p.ValueTo(&f.Date))),
			app.Label().Class("auth-field").Body(app.Text("Valor (R$)"), app.Input().Type("number").Attr("min", "0").Attr("step", "10").Value(f.Price).Disabled(p.teamDirectServiceSaving).OnChange(p.ValueTo(&f.Price))),
			app.Label().Class("auth-field").Body(app.Text("Status *"), app.Select().Attr("value", f.Status).Disabled(p.teamDirectServiceSaving).OnChange(p.ValueTo(&f.Status)).Body(statuses...)),
		),
		app.Label().Class("auth-field").Body(app.Text("Observações"), app.Textarea().Rows(2).Text(f.Notes).Placeholder("Ex: levar escada, confirmar horário...").Disabled(p.teamDirectServiceSaving).OnChange(p.ValueTo(&f.Notes))),
	}
	if f.Message != "" {
		fields = append(fields, app.P().Class("portal-section__error").Body(app.Text(f.Message)))
	}
	if len(applianceOptions) == 0 {
		fields = append(fields, app.P().Class("auth-notice").Body(app.Text("Este cliente não tem aparelhos. Cadastre o aparelho dele primeiro.")))
	}
	fields = append(fields, app.Div().Class("portal-budget__actions").Body(
		app.Button().Class("auth-link").Type("button").Disabled(p.teamDirectServiceSaving).OnClick(func(ctx app.Context, event app.Event) {
			event.PreventDefault()
			p.teamDirectServiceForm = nil
			ctx.Update()
		}).Body(app.Text("Cancelar")),
		app.Button().Class("auth-submit").Type("submit").Disabled(!canSave).Body(app.Text(func() string {
			if f.ServiceID != "" {
				return "Concluir Cadastro do Serviço"
			}
			return "Cadastrar Serviço"
		}())),
	))
	return app.Div().Class("auth-backdrop").Body(app.Div().Class("auth-dialog team-direct-service-dialog").Body(
		app.H2().Class("auth-dialog__title").Body(app.Text("Cadastrar Serviço")),
		app.P().Class("auth-dialog__intro").Body(app.Text("Registro direto — sem checklist")),
		app.Form().Class("auth-form team-direct-service-form").OnSubmit(p.saveTeamDirectService).Body(fields...),
	))
}
