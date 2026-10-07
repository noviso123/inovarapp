package webapp

import (
	"net/http"
	"strings"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/domain"
)

type teamStartServiceForm struct {
	ServiceType string
	ServiceID   string
	ClientID    string
	ApplianceID string
	Search      string
	Message     string
	Step        int
}

func (p *serviceCatalogPage) openTeamStartServiceForm(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.caller == nil || p.session == nil || (p.caller.Role != domain.RoleAdmin && p.caller.Role != domain.RoleTechnician) {
		return
	}
	p.teamStartServiceForm = &teamStartServiceForm{ServiceType: string(domain.ServiceCleaning), Step: 1}
	p.teamActionMessage = ""
	ctx.Update()
}

func (p *serviceCatalogPage) openTeamStartServiceForAppliance(clientID, applianceID string) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		if p.caller == nil || p.session == nil || (p.caller.Role != domain.RoleAdmin && p.caller.Role != domain.RoleTechnician) {
			return
		}
		p.teamStartServiceForm = newTeamStartServiceFormForAppliance(clientID, applianceID)
		p.teamActionMessage = ""
		ctx.Update()
	}
}

func newTeamStartServiceFormForAppliance(clientID, applianceID string) *teamStartServiceForm {
	return &teamStartServiceForm{ServiceType: string(domain.ServiceCleaning), ClientID: clientID, ApplianceID: applianceID, Step: 3}
}

func newTeamStartServiceFormForRequest(service map[string]any) *teamStartServiceForm {
	serviceType := firstNonEmptyBudget(portalText(service["descricao"]), portalText(service["tipo"]), string(domain.ServiceCleaning))
	return &teamStartServiceForm{
		ServiceType: serviceType,
		ServiceID:   portalText(service["id"]),
		ClientID:    portalText(service["cliente_id"]),
		ApplianceID: portalText(service["aparelho_id"]),
		Step:        3,
	}
}

func (p *serviceCatalogPage) selectTeamStartClient(ctx app.Context, event app.Event) {
	if f := p.teamStartServiceForm; f != nil {
		f.ClientID = event.Get("target").Get("value").String()
		f.ApplianceID = ""
		if customer := findTeamCustomer(p.teamCustomers, f.ClientID); customer != nil {
			if appliances := applianceMaps(customer); len(appliances) == 1 {
				f.ApplianceID = portalText(appliances[0]["id"])
				p.beginTeamStartService(ctx)
				return
			} else if len(appliances) > 1 {
				f.Step = 3
			} else {
				f.Message = "Este cliente ainda não tem aparelhos cadastrados. Cadastre um aparelho antes de iniciar o serviço."
			}
		}
		ctx.Update()
	}
}

func (p *serviceCatalogPage) chooseTeamStartService(serviceType string) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		if f := p.teamStartServiceForm; f != nil {
			f.ServiceType, f.Step, f.Message = serviceType, 2, ""
			ctx.Update()
		}
	}
}

func (p *serviceCatalogPage) teamStartServiceDialog() app.UI {
	f := p.teamStartServiceForm
	if f == nil {
		return app.Div()
	}
	clientOptions := []app.UI{app.Option().Value("").Body(app.Text("Selecione um cliente"))}
	query := strings.TrimSpace(f.Search)
	for _, customer := range p.teamCustomers {
		if portalText(customer["id"]) != f.ClientID && !searchMatches(query, portalText(customer["nome"]), portalText(customer["whatsapp"]), portalText(customer["bairro"]), portalText(customer["endereco"]), portalText(customer["cidade"])) {
			continue
		}
		clientOptions = append(clientOptions, app.Option().Value(portalText(customer["id"])).Body(app.Text(portalText(customer["nome"]))))
	}
	applianceOptions := []app.UI{app.Option().Value("").Body(app.Text("Selecione um aparelho"))}
	if customer := findTeamCustomer(p.teamCustomers, f.ClientID); customer != nil {
		for _, appliance := range applianceMaps(customer) {
			label := strings.TrimSpace(strings.Join([]string{portalText(appliance["marca"]), portalText(appliance["modelo"]), portalText(appliance["btus"]) + " BTUs"}, " "))
			if room := portalText(appliance["ambiente"]); room != "" {
				label += " • " + room
			}
			applianceOptions = append(applianceOptions, app.Option().Value(portalText(appliance["id"])).Body(app.Text(label)))
		}
	}
	title := "Selecione o Tipo de Serviço"
	if f.Step == 2 {
		title = "Selecione o Cliente & Aparelho"
	} else if f.Step == 3 {
		title = "Selecione o Aparelho"
	}
	stepContent := []app.UI{}
	switch f.Step {
	case 1:
		stepContent = append(stepContent, app.P().Class("auth-dialog__intro").Body(app.Text("Escolha qual procedimento técnico será realizado para abrir o checklist adequado:")))
		for _, entry := range domain.BuildServiceCatalog(&p.teamProfile) {
			stepContent = append(stepContent, app.Button().Class("auth-link").Type("button").OnClick(p.chooseTeamStartService(teamCompletionCatalogValue(entry))).Body(app.Strong().Body(app.Text(entry.Name)), app.Span().Body(app.Text("  ›"))))
		}
	case 2:
		stepContent = append(stepContent,
			app.Button().Class("auth-link").Type("button").OnClick(func(ctx app.Context, event app.Event) {
				event.PreventDefault()
				f.Step = 1
				f.Message = ""
				ctx.Update()
			}).Body(app.Text("← Voltar para tipos de serviço")),
			app.P().Class("auth-dialog__intro").Body(app.Text("Serviço: "+f.ServiceType)),
			app.Label().Class("auth-field").Body(app.Text("Buscar cliente por nome, telefone ou bairro"), app.Input().Type("search").Value(f.Search).Placeholder("Buscar cliente...").OnInput(p.ValueTo(&f.Search))),
			app.Label().Class("auth-field").Body(app.Text("Cliente"), app.Select().Attr("value", f.ClientID).OnChange(p.selectTeamStartClient).Body(clientOptions...)),
		)
	case 3:
		stepContent = append(stepContent,
			app.Button().Class("auth-link").Type("button").OnClick(func(ctx app.Context, event app.Event) {
				event.PreventDefault()
				f.Step = 2
				f.Message = ""
				ctx.Update()
			}).Body(app.Text("← Voltar para clientes")),
			app.P().Class("auth-dialog__intro").Body(app.Text("Selecione o aparelho para iniciar "+f.ServiceType+":")),
			app.Label().Class("auth-field").Body(app.Text("Aparelho"), app.Select().Attr("value", f.ApplianceID).Required(true).OnChange(p.ValueTo(&f.ApplianceID)).Body(applianceOptions...)),
		)
	}
	if f.Message != "" {
		stepContent = append(stepContent, app.P().Class("portal-section__error").Body(app.Text(f.Message)))
	}
	if f.Step == 3 {
		stepContent = append(stepContent, app.Div().Class("portal-budget__actions").Body(
			app.Button().Class("auth-link").Type("button").Disabled(p.teamStartServiceSaving).OnClick(func(ctx app.Context, event app.Event) {
				event.PreventDefault()
				p.teamStartServiceForm = nil
				ctx.Update()
			}).Body(app.Text("Cancelar")),
			app.Button().Class("auth-submit").Type("submit").Disabled(p.teamStartServiceSaving || f.ApplianceID == "").Body(app.Text("Abrir checklist")),
		))
	} else if f.Step == 2 {
		stepContent = append(stepContent, app.Div().Class("portal-budget__actions").Body(app.Button().Class("auth-link").Type("button").Disabled(p.teamStartServiceSaving).OnClick(func(ctx app.Context, event app.Event) {
			event.PreventDefault()
			p.teamStartServiceForm = nil
			ctx.Update()
		}).Body(app.Text("Cancelar"))))
	}
	return app.Div().Class("auth-backdrop").Body(app.Div().Class("auth-dialog").Role("dialog").Body(
		app.Div().Class("auth-dialog__heading").Body(app.H2().Class("auth-dialog__title").Body(app.Text(title)), app.Button().Class("auth-close").Type("button").Disabled(p.teamStartServiceSaving).OnClick(func(ctx app.Context, event app.Event) {
			event.PreventDefault()
			p.teamStartServiceForm = nil
			ctx.Update()
		}).Body(app.Text("Fechar"))),
		app.Form().Class("auth-form team-start-service-form").OnSubmit(p.submitTeamStartService).Body(stepContent...),
	))
}

func findOpenTeamService(services []map[string]any, clientID, applianceID string) (map[string]any, bool) {
	var match map[string]any
	for _, service := range services {
		status := strings.ToUpper(portalText(service["status"]))
		if portalText(service["cliente_id"]) != clientID || (status != "PENDENTE" && status != "AGENDADO" && status != "EM_ANDAMENTO") {
			continue
		}
		serviceAppliance := portalText(service["aparelho_id"])
		if serviceAppliance != "" && serviceAppliance != applianceID {
			continue
		}
		if match != nil {
			return nil, true
		}
		match = service
	}
	return match, false
}

func (p *serviceCatalogPage) submitTeamStartService(ctx app.Context, event app.Event) {
	event.PreventDefault()
	p.beginTeamStartService(ctx)
}

func (p *serviceCatalogPage) beginTeamStartService(ctx app.Context) {
	f := p.teamStartServiceForm
	if f == nil || p.teamStartServiceSaving || p.session == nil {
		return
	}
	customer := findTeamCustomer(p.teamCustomers, f.ClientID)
	if customer == nil {
		f.Message = "Selecione um cliente válido."
		ctx.Update()
		return
	}
	var appliance map[string]any
	for _, candidate := range applianceMaps(customer) {
		if portalText(candidate["id"]) == f.ApplianceID {
			appliance = candidate
			break
		}
	}
	if appliance == nil {
		f.Message = "Selecione um aparelho válido para este cliente."
		ctx.Update()
		return
	}
	if f.ServiceType == "" {
		f.Message = "Selecione o tipo de serviço."
		ctx.Update()
		return
	}
	var open map[string]any
	if f.ServiceID != "" {
		for _, service := range p.teamServices {
			if portalText(service["id"]) == f.ServiceID && portalText(service["cliente_id"]) == f.ClientID && portalText(service["aparelho_id"]) == f.ApplianceID {
				status := strings.ToUpper(portalText(service["status"]))
				if status == "PENDENTE" || status == "AGENDADO" || status == "EM_ANDAMENTO" {
					open = service
				}
				break
			}
		}
		if open == nil {
			f.Message = "A ordem deste chamado não está mais aberta. Atualize a central e tente novamente."
			ctx.Update()
			return
		}
	} else {
		var multiple bool
		open, multiple = findOpenTeamService(p.teamServices, f.ClientID, f.ApplianceID)
		if multiple {
			f.Message = "Há mais de uma OS aberta para este aparelho. Abra a OS desejada na central antes de iniciar."
			ctx.Update()
			return
		}
	}
	pageURL := app.Window().URL()
	if pageURL == nil || pageURL.Scheme == "" || pageURL.Host == "" {
		f.Message = "Não foi possível conectar ao servidor."
		ctx.Update()
		return
	}
	today := dashboardCivilToday(time.Now()).Format("2006-01-02")
	serviceType := f.ServiceType
	serviceKind := string(supabase.MapServiceTypeToSupabase(serviceType))
	if serviceKind == "" {
		serviceKind = "OUTRO"
	}
	baseURL, token := apiBaseURL(), p.session.AccessToken
	p.teamStartServiceSaving, f.Message = true, ""
	ctx.Update()
	go func() {
		service := open
		var err error
		if service != nil {
			serviceID := portalText(service["id"])
			fields := map[string]any{"status": "EM_ANDAMENTO", "data_inicio": today}
			err = sendTeamJSON(ctx, baseURL+"/api/servicos", token, http.MethodPatch, map[string]any{"id": serviceID, "fields": fields})
			if err != nil {
				fields = map[string]any{"status": "EM_ANDAMENTO", "observacoes": domain.SetServiceDateMarker(portalText(service["observacoes"]), "DATA_INICIO", today)}
				err = sendTeamJSON(ctx, baseURL+"/api/servicos", token, http.MethodPatch, map[string]any{"id": serviceID, "fields": fields})
			}
		} else {
			body := map[string]any{
				"cliente_id": f.ClientID, "aparelho_id": f.ApplianceID, "tipo": serviceKind,
				"descricao": serviceType, "status": "EM_ANDAMENTO", "valor": 0,
				"data_agendamento": today, "observacoes": domain.SetServiceDateMarker("", "DATA_INICIO", today),
			}
			service, err = sendTeamJSONResult(ctx, baseURL+"/api/servicos", token, http.MethodPost, body)
		}
		if err != nil {
			p.teamStartServiceSaving = false
			if p.teamStartServiceForm != nil {
				p.teamStartServiceForm.Message = "Não foi possível iniciar o serviço. Verifique a conexão e tente novamente."
			}
			ctx.Update()
			return
		}
		serviceID := portalText(service["id"])
		if serviceID == "" {
			p.teamStartServiceSaving = false
			if p.teamStartServiceForm != nil {
				p.teamStartServiceForm.Message = "A resposta do servidor não identificou a ordem de serviço."
			}
			ctx.Update()
			return
		}
		service["id"], service["cliente_id"], service["aparelho_id"] = serviceID, f.ClientID, f.ApplianceID
		service["tipo"], service["descricao"], service["status"] = serviceKind, serviceType, "EM_ANDAMENTO"
		service["observacoes"], service["customers"], service["air_conditioners"] = domain.SetServiceDateMarker(portalText(service["observacoes"]), "DATA_INICIO", today), customer, appliance
		p.registerLocalNotification(ctx, "Ordem de Serviço iniciada", "A OS foi salva. Preencha o checklist para concluir; você pode continuar depois.")
		p.teamStartServiceSaving = false
		p.teamStartServiceForm = nil
		p.teamCompletion = newTeamCompletionForm(service, p.teamProfile)
		p.teamActionMessage = "Serviço iniciado. Preencha o checklist para concluir a ordem."
		ctx.Update()
	}()
}
