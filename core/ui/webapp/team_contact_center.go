package webapp

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/domain"
)

const (
	teamQueueReturns  = "fila"
	teamQueueRequests = "chamados"
	teamQueueHistory  = "historico"
)

// ariaBoolean serializes true with an uppercase first letter because go-app's
// HTML encoder treats the exact string "true" as a valueless boolean attribute.
// ARIA boolean tokens are case-insensitive, and this keeps the attribute value
// explicit for assistive technology.
func ariaBoolean(value bool) string {
	if value {
		return "True"
	}
	return "false"
}

func (p *serviceCatalogPage) teamContactCenter() app.UI {
	if p.teamQueueSection == "" {
		p.teamQueueSection = teamQueueReturns
	}
	if p.teamQueueFilter == "" {
		p.teamQueueFilter = "todos"
	}
	counts := map[string]int{"todos": 0, "chamados": 0, "atrasado": 0, "esta_semana": 0, "em_breve": 0, "sem_historico": 0}
	for _, item := range p.teamDashboard.ReturnEvents {
		counts[item.Status]++
		if item.Status != string(domain.ReturnNoHistory) || len(item.PendingServiceIDs) > 0 || len(item.InProgressServiceIDs) > 0 {
			counts["todos"]++
		}
	}
	pendingRequestCount := 0
	pendingServices := make([]map[string]any, 0)
	for _, service := range p.teamServices {
		if strings.EqualFold(portalText(service["status"]), "PENDENTE") {
			pendingRequestCount++
			pendingServices = append(pendingServices, service)
		}
	}
	counts["chamados"] = 0
	for _, item := range p.teamDashboard.ReturnEvents {
		if len(item.PendingServiceIDs) > 0 {
			counts["chamados"]++
		}
	}
	// Retain a safe fallback for pending requests when dashboard cards are stale.
	if counts["chamados"] == 0 {
		counts["chamados"] = len(groupTeamRequestServices(pendingServices))
	}
	seenQueueKeys := map[string]bool{}
	for _, item := range p.teamDashboard.ReturnEvents {
		if item.Status != string(domain.ReturnNoHistory) || len(item.PendingServiceIDs) > 0 || len(item.InProgressServiceIDs) > 0 {
			seenQueueKeys[teamReturnQueueKey(item)] = true
		}
	}
	for _, group := range groupTeamRequestServices(pendingServices) {
		key := group.CustomerID + ":" + group.ApplianceID
		if group.CustomerID == "" && group.ApplianceID == "" && len(group.Services) > 0 {
			key = "service:" + portalText(group.Services[0]["id"])
		}
		if !seenQueueKeys[key] {
			counts["todos"]++
			seenQueueKeys[key] = true
		}
	}
	sections := []struct{ id, title string }{{teamQueueReturns, "Retornos"}, {teamQueueRequests, "Chamados"}, {teamQueueHistory, "Ordens de Serviço"}}
	tabs := make([]app.UI, 0, len(sections))
	for _, section := range sections {
		class := "auth-link"
		if p.teamQueueSection == section.id {
			class = "auth-submit"
		}
		count := 0
		switch section.id {
		case teamQueueReturns:
			count = len(p.teamDashboard.ReturnEvents)
		case teamQueueRequests:
			count = pendingRequestCount
		case teamQueueHistory:
			count = len(p.teamServices)
		}
		id := section.id
		tabs = append(tabs, app.Button().Class(class).Type("button").Attr("role", "tab").Attr("aria-selected", ariaBoolean(p.teamQueueSection == id)).OnClick(func(ctx app.Context, event app.Event) {
			event.PreventDefault()
			p.teamQueueSection = id
			if id == teamQueueRequests {
				p.teamRequestFilter, p.teamRequestSearch = "TODOS", ""
			}
			ctx.Update()
		}).Body(app.Text(section.title+" "), app.Span().Body(app.Text(count))))
	}
	content := []app.UI{
		app.Div().Class("catalog__heading").Body(app.Div().Body(app.P().Class("catalog__eyebrow").Body(app.Text("ATENDIMENTO")), app.H2().Class("catalog__title").Body(app.Text("Central de Atendimento"))), app.Span().Class("catalog__count").Body(app.Text("Retornos, chamados e ordens de serviço"))),
		app.Div().Class("portal-budget__actions").Attr("role", "tablist").Body(tabs...),
	}
	if p.teamQueueSection == teamQueueReturns {
		filterLabels := []struct{ id, title string }{{"todos", "Todos"}, {"chamados", "Chamados"}, {string(domain.ReturnOverdue), "Atrasados"}, {string(domain.ReturnThisWeek), "Esta semana"}, {string(domain.ReturnSoon), "Em breve"}, {string(domain.ReturnNoHistory), "Sem histórico"}}
		filterButtons := make([]app.UI, 0, len(filterLabels))
		for _, filter := range filterLabels {
			id := filter.id
			className := "auth-link team-return-filter"
			if p.teamQueueFilter == id {
				className += " team-return-filter--selected"
			}
			filterButtons = append(filterButtons, app.Button().Class(className).Type("button").Attr("aria-pressed", ariaBoolean(p.teamQueueFilter == id)).OnClick(func(ctx app.Context, event app.Event) {
				event.PreventDefault()
				p.teamQueueFilter = id
				ctx.Update()
			}).Body(app.Text(filter.title+" ("+strconv.Itoa(counts[id])+")")))
		}
		content = append(content,
			app.Label().Class("auth-field team-search-field").Body(app.Text("Buscar por cliente, telefone, aparelho ou cômodo"), app.Input().Type("search").Class("team-search-input").Value(p.teamQueueSearch).Placeholder("Digite para buscar...").OnInput(p.ValueTo(&p.teamQueueSearch))),
			app.Div().Class("portal-budget__actions").Body(filterButtons...),
		)
		items := filterTeamContactReturns(p.teamDashboard.ReturnEvents, p.teamQueueFilter, p.teamQueueSearch)
		cards := make([]app.UI, 0, len(items))
		for _, item := range items {
			facts := []app.UI{}
			if item.Appliance != "" {
				facts = append(facts, teamReturnFact("Equipamento", item.Appliance))
			}
			if item.Capacity != "" {
				facts = append(facts, teamReturnFact("Capacidade", portalBTUs(item.Capacity)))
			}
			if item.Room != "" {
				facts = append(facts, teamReturnFact("Ambiente", item.Room))
			}
			if item.LastMaintenanceDate != "" {
				lastMaintenance := portalDate(item.LastMaintenanceDate)
				if item.LastMaintenanceService != "" {
					lastMaintenance += " · " + item.LastMaintenanceService
				}
				facts = append(facts, teamReturnFact("Última manutenção", lastMaintenance))
			} else {
				facts = append(facts, teamReturnFact("Última manutenção", "Sem histórico registrado"))
			}
			nextLabel := "Próxima manutenção prevista"
			if item.ScheduledServiceID != "" {
				nextLabel = "Agendamento confirmado"
			}
			if item.Date != "" {
				nextDate := portalDate(item.Date)
				if item.ScheduledServiceID != "" && item.ScheduledTime != "" {
					nextDate += " às " + item.ScheduledTime
				}
				facts = append(facts, teamReturnFact(nextLabel, nextDate))
			} else {
				facts = append(facts, teamReturnFact("Próxima manutenção", "Sem previsão"))
			}
			if len(item.PendingServiceIDs) > 0 {
				facts = append(facts, teamReturnFact("Chamado", "Aguardando atendimento"))
			}
			if len(item.InProgressServiceIDs) > 0 {
				facts = append(facts, teamReturnFact("Serviço", "Em andamento"))
			}
			actions := []app.UI{}
			if phone := cleanTeamContactPhone(item.Phone); phone != "" {
				actions = append(actions,
					app.A().Class("auth-link").Href("tel:"+phone).Target("_blank").Rel("noopener noreferrer").OnClick(p.markTeamContacted(item)).Body(app.Text("Ligar")),
					app.A().Class("auth-submit").Href(returnWhatsAppURL(item, p.teamProfile)).Target("_blank").Rel("noopener noreferrer").OnClick(p.markTeamContacted(item)).Body(app.Text("Chamar no WhatsApp")),
				)
			}
			actions = append(actions, p.teamReturnQuickActions(item)...)
			selected := p.teamQueueSelectedKey == teamReturnQueueKey(item)
			cards = append(cards, app.Article().Class("portal-service team-return-card").Body(
				app.Button().Class("team-return-queue__select").Type("button").Attr("aria-expanded", ariaBoolean(selected)).OnClick(p.toggleTeamReturnItem(item)).Body(app.Strong().Body(app.Text(item.Customer)), app.Span().Class("portal-service__status").Body(app.Text(teamReturnStatusLabel(item.Status)))),
				app.Div().Class("team-return-card__facts").Body(facts...),
				app.Div().Class("portal-budget__actions team-return-card__actions").Body(actions...),
				func() app.UI {
					if selected {
						return p.teamReturnDrawer(item)
					}
					return app.Span()
				}(),
			))
		}
		if len(cards) == 0 {
			cards = append(cards, app.P().Class("portal-section__empty").Body(app.Text("Nenhum cliente corresponde a este filtro.")))
		}
		content = append(content, app.Div().Class("service-grid").Body(cards...))
	} else if p.teamQueueSection == teamQueueRequests {
		if p.teamRequestFilter == "" {
			p.teamRequestFilter = "TODOS"
		}
		requestFilters := []struct{ id, label string }{{"TODOS", "Todas"}, {"PENDENTE", "Pendentes"}, {"AGENDADO", "Agendadas"}, {"CONCLUIDO", "Concluídas"}}
		filterButtons := make([]app.UI, 0, len(requestFilters))
		for _, filter := range requestFilters {
			id := filter.id
			class := "auth-link"
			if p.teamRequestFilter == id {
				class = "auth-submit"
			}
			filterButtons = append(filterButtons, app.Button().Class(class).Type("button").Attr("aria-pressed", ariaBoolean(p.teamRequestFilter == id)).OnClick(func(ctx app.Context, event app.Event) {
				event.PreventDefault()
				p.teamRequestFilter = id
				ctx.Update()
			}).Body(app.Text(filter.label)))
		}
		today := dashboardCivilToday(time.Now()).Format("2006-01-02")
		filtered := filterTeamRequestServices(p.teamServices, p.teamRequestFilter, p.teamRequestSearch, today)
		overdueCount := countOverdueTeamRequests(p.teamServices, today)
		cards := make([]app.UI, 0, len(filtered))
		for _, service := range filtered {
			cards = append(cards, p.teamRequestCard(service, today))
		}
		if len(cards) == 0 {
			cards = append(cards, app.P().Class("portal-section__empty").Body(app.Text("Nenhuma solicitação encontrada. Quando os clientes solicitarem atendimentos pelo app ou pelo site da Inovar, as ordens aparecerão aqui para você confirmar e atender.")))
		}
		content = append(content,
			app.Div().Class("portal-budget__actions").Attr("role", "tablist").Body(filterButtons...),
			app.Label().Class("auth-field team-search-field").Body(app.Text("Buscar chamados"), app.Input().Type("search").Class("team-search-input").Value(p.teamRequestSearch).Placeholder("Cliente, telefone ou descrição...").OnInput(p.ValueTo(&p.teamRequestSearch))),
			func() app.UI {
				if overdueCount == 0 {
					return app.Span()
				}
				return app.P().Class("portal-section__error").Body(app.Text(strconv.Itoa(overdueCount) + " chamado(s) atrasado(s)"))
			}(),
			app.Div().Class("service-grid").Body(cards...),
		)
	} else {
		cards := make([]app.UI, 0)
		for _, service := range p.teamServices {
			status := strings.ToUpper(portalText(service["status"]))
			if status != "CONCLUIDO" && status != "CANCELADO" || !teamServiceMatchesSearch(service, p.teamQueueSearch) {
				continue
			}
			customer := "Cliente"
			if row, ok := service["customers"].(map[string]any); ok {
				customer = firstNonEmptyBudget(portalText(row["nome"]), customer)
			}
			label, _ := portalServiceStatus(status)
			cards = append(cards, app.Article().Class("portal-service team-history-card").Body(
				app.Div().Class("portal-service__heading").Body(app.Strong().Body(app.Text(customer)), app.Span().Class("portal-service__status").Body(app.Text(label))),
				app.P().Class("portal-section__intro").Body(app.Text(firstNonEmptyBudget(portalText(service["descricao"]), portalText(service["tipo"]), "Ordem de serviço"))),
				app.P().Class("portal-service__date").Body(app.Text("Data: "+portalDate(teamServiceLifecycleDate(service)))),
				app.Button().Class("auth-link team-history-card__open").Type("button").OnClick(p.openTeamServiceDetail(service)).Body(app.Text("Abrir detalhes e comprovante da OS")),
			))
		}
		if len(cards) == 0 {
			cards = append(cards, app.P().Class("portal-section__empty").Body(app.Text("Nenhuma ordem de serviço no histórico encontrado.")))
		}
		content = append(content, app.Label().Class("auth-field team-search-field").Body(app.Text("Buscar ordens de serviço"), app.Input().Type("search").Class("team-search-input").Value(p.teamQueueSearch).Placeholder("Cliente ou descrição...").OnInput(p.ValueTo(&p.teamQueueSearch))), app.Div().Class("service-grid").Body(cards...))
	}
	return app.Section().ID("team-contact-center").Class("team-contact-center").Body(content...)
}

func teamReturnFact(label, value string) app.UI {
	return app.Div().Class("team-return-card__fact").Body(
		app.Span().Body(app.Text(label)),
		app.Strong().Body(app.Text(value)),
	)
}

func teamReturnQueueKey(item teamReturnItem) string { return item.CustomerID + ":" + item.ApplianceID }

func (p *serviceCatalogPage) toggleTeamReturnItem(item teamReturnItem) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		key := teamReturnQueueKey(item)
		if p.teamQueueSelectedKey == key {
			p.teamQueueSelectedKey = ""
		} else {
			p.teamQueueSelectedKey = key
		}
		ctx.Update()
	}
}

func (p *serviceCatalogPage) teamReturnDrawer(item teamReturnItem) app.UI {
	services := make([]map[string]any, 0)
	completed := make([]map[string]any, 0)
	for _, service := range p.teamServices {
		if portalText(service["cliente_id"]) != item.CustomerID || portalText(service["aparelho_id"]) != item.ApplianceID {
			continue
		}
		status := strings.ToUpper(portalText(service["status"]))
		if status == "PENDENTE" || status == "AGENDADO" || status == "EM_ANDAMENTO" {
			services = append(services, service)
		}
		if status == "CONCLUIDO" {
			completed = append(completed, service)
		}
	}
	sort.SliceStable(completed, func(i, j int) bool {
		return teamServiceLifecycleDate(completed[i]) > teamServiceLifecycleDate(completed[j])
	})
	sections := []app.UI{}
	if item.BudgetID != "" {
		for _, budget := range p.teamBudgets {
			if budget.ID != item.BudgetID {
				continue
			}
			status := "Orçamento aguardando resposta"
			if budget.Status == domain.BudgetApproved {
				status = "Orçamento aprovado — agendar serviço"
			}
			description := budget.ApplianceDescription
			if len(budget.Items) > 0 {
				description = budget.Items[0].Description
			}
			budgetMeta := []string{}
			if budget.Number != "" {
				budgetMeta = append(budgetMeta, "#"+budget.Number)
			}
			if description != "" {
				budgetMeta = append(budgetMeta, description)
			}
			budgetDetails := []app.UI{
				app.Strong().Body(app.Text(status + " — R$ " + formatPortalMoney(budget.FinalValue))),
				app.P().Class("portal-section__intro").Body(app.Text(strings.Join(budgetMeta, " • "))),
			}
			budgetActions := []app.UI{
				app.Button().Class("auth-link").Type("button").OnClick(func(ctx app.Context, event app.Event) {
					event.PreventDefault()
					p.teamBudgetOpenID = budget.ID
					ctx.Update()
				}).Body(app.Text("Ver proposta")),
			}
			if budget.Status == domain.BudgetApproved {
				budgetActions = append(budgetActions, app.Button().Class("auth-submit").Type("button").OnClick(p.openTeamBudgetSchedule(budget)).Body(app.Text("Agendar serviço")))
			}
			sections = append(sections, app.Article().Class("team-return-drawer__budget").Body(append(budgetDetails, app.Div().Class("portal-budget__actions").Body(budgetActions...))...))
			break
		}
	}
	sections = append(sections, app.H3().Class("team-agenda__heading").Body(app.Text("Atendimento em tempo real")))
	for _, service := range services {
		status := strings.ToUpper(portalText(service["status"]))
		name := firstNonEmptyBudget(portalText(service["descricao"]), portalText(service["tipo"]), "Serviço")
		statusLabel, _ := portalServiceStatus(status)
		row := []app.UI{app.Strong().Body(app.Text(name)), app.Span().Class("portal-service__date").Body(app.Text(statusLabel + " • " + portalDate(teamServiceLifecycleDate(service))))}
		actions := []app.UI{}
		switch status {
		case "PENDENTE":
			actions = append(actions, app.Button().Class("auth-link").Type("button").OnClick(p.openTeamSchedule(service)).Body(app.Text("Atender chamado")))
			actions = append(actions, app.Button().Class("auth-link").Type("button").OnClick(p.updateTeamRequestStatus(portalText(service["id"]), "EM_ANDAMENTO")).Body(app.Text("Iniciar")))
		case "AGENDADO":
			actions = append(actions, app.Button().Class("auth-link").Type("button").OnClick(p.startTeamService(portalText(service["id"]))).Body(app.Text("Iniciar")))
		case "EM_ANDAMENTO":
			actions = append(actions, app.Button().Class("auth-submit").Type("button").OnClick(p.openTeamCompletion(service)).Body(app.Text("Finalizar")))
		}
		actions = append(actions, app.Button().Class("auth-link").Type("button").OnClick(p.requestTeamCancellation(portalText(service["id"]))).Body(app.Text("Cancelar")))
		sections = append(sections, app.Div().Class("team-return-drawer__service").Body(app.Div().Body(row...), app.Div().Class("portal-budget__actions").Body(actions...)))
	}
	if len(services) == 0 {
		sections = append(sections, app.P().Class("portal-section__empty").Body(app.Text("Nenhum atendimento em andamento ou agendado para este aparelho.")))
	}
	history := []app.UI{app.H3().Class("team-agenda__heading").Body(app.Text("Histórico do aparelho")), app.P().Class("portal-section__intro").Body(app.Text(strconv.Itoa(len(completed)) + " ordens concluídas recentes"))}
	for _, service := range completed {
		history = append(history, app.Button().Class("team-return-drawer__history-item").Type("button").OnClick(p.openTeamServiceDetail(service)).Body(app.Strong().Body(app.Text(firstNonEmptyBudget(portalText(service["descricao"]), portalText(service["tipo"]), "Ordem de serviço"))), app.Span().Body(app.Text(portalDate(teamServiceLifecycleDate(service))+" • R$ "+formatPortalMoney(agendaNumber(service["valor"]))))))
	}
	if len(completed) == 0 && item.ApplianceID != "" {
		history = append(history, app.P().Class("portal-section__empty").Body(app.Text("Nenhuma Ordem de Serviço ainda — este aparelho nunca foi atendido pelo aplicativo.")), app.Button().Class("auth-submit").Type("button").OnClick(p.openTeamHistoryForm(item.ApplianceID)).Body(app.Text("Registrar histórico anterior (serviço já feito)")))
	}
	sections = append(sections, app.Div().Class("team-return-drawer__history").Body(history...))
	if item.ApplianceID != "" && p.teamHistoryApplianceID == item.ApplianceID {
		sections = append(sections, p.applianceHistoryPanel())
	}
	return app.Div().Class("team-return-drawer").Body(sections...)
}

func (p *serviceCatalogPage) teamReturnQuickActions(item teamReturnItem) []app.UI {
	if item.CustomerID == "" || item.ApplianceID == "" {
		return nil
	}
	customer := findTeamCustomer(p.teamCustomers, item.CustomerID)
	if customer == nil {
		return nil
	}
	var appliance map[string]any
	for _, candidate := range applianceMaps(customer) {
		if portalText(candidate["id"]) == item.ApplianceID {
			appliance = candidate
			break
		}
	}
	if appliance == nil {
		return nil
	}
	actions := []app.UI{}
	for _, id := range item.PendingServiceIDs {
		if service := findOfflineService(p.teamServices, id); service != nil {
			actions = append(actions, app.Button().Class("auth-submit").Type("button").OnClick(p.openTeamSchedule(service)).Body(app.Text("Atender chamado")))
			break
		}
	}
	for _, id := range item.InProgressServiceIDs {
		if service := findOfflineService(p.teamServices, id); service != nil {
			actions = append(actions,
				app.Button().Class("auth-submit").Type("button").OnClick(p.openTeamCompletion(service)).Body(app.Text("Finalizar serviço")),
				app.Button().Class("auth-link").Type("button").OnClick(p.requestTeamCancellation(id)).Body(app.Text("Cancelar OS")),
			)
			break
		}
	}
	if item.BudgetID != "" {
		for _, budget := range p.teamBudgets {
			if budget.ID == item.BudgetID && budget.Status == domain.BudgetApproved {
				actions = append(actions, app.Button().Class("auth-submit").Type("button").OnClick(p.openTeamBudgetSchedule(budget)).Body(app.Text("Agendar serviço")))
				break
			}
		}
	}
	return append(actions, []app.UI{
		app.Button().Class("auth-link").Type("button").OnClick(p.openTeamApplianceForm(customer, appliance)).Body(app.Text("Ficha do Aparelho")),
		app.Button().Class("auth-link").Type("button").OnClick(p.openTeamAppointmentForAppliance(item)).Body(app.Text("Agendar Retorno")),
		app.Button().Class("auth-submit").Type("button").OnClick(p.openTeamStartServiceForAppliance(item.CustomerID, item.ApplianceID)).Body(app.Text("Ordem de Serviço")),
		app.Button().Class("auth-link").Type("button").OnClick(p.toggleApplianceHistory(item.ApplianceID)).Body(app.Text("Histórico completo")),
		app.Button().Class("auth-link").Type("button").OnClick(p.openTeamBudgetFormForService(map[string]any{"cliente_id": item.CustomerID, "aparelho_id": item.ApplianceID})).Body(app.Text("Orçamento")),
	}...)
}

func (p *serviceCatalogPage) teamRequestCard(service map[string]any, today string) app.UI {
	id := portalText(service["id"])
	status := strings.ToUpper(portalText(service["status"]))
	serviceName := teamRequestServiceName(service)
	customer, _ := service["customers"].(map[string]any)
	appliance, _ := service["air_conditioners"].(map[string]any)
	name := firstNonEmptyBudget(portalText(customer["nome"]), "Cliente não identificado")
	phone := portalText(customer["whatsapp"])
	date := agendaDate(service["data_agendamento"])
	overdue := isOverdueTeamRequest(service, today)
	label, tone := portalServiceStatus(status)
	heading := []app.UI{app.Strong().Body(app.Text(name))}
	if overdue {
		heading = append(heading, app.Span().Class("portal-service__status portal-service__status--cancelled").Body(app.Text("ATRASADO")))
	}
	heading = append(heading, app.Span().Class("portal-service__status portal-service__status--"+tone).Body(app.Text(label)))

	parts := []app.UI{
		app.Div().Class("portal-service__heading").Body(heading...),
		app.P().Class("portal-section__intro").Body(app.Text("Serviço: " + serviceName)),
	}
	if phone != "" || portalText(customer["bairro"]) != "" {
		contact := []string{}
		if phone != "" {
			contact = append(contact, "Tel: "+phone)
		}
		if bairro := portalText(customer["bairro"]); bairro != "" {
			contact = append(contact, bairro+", "+firstNonEmptyBudget(portalText(customer["cidade"]), "ES"))
		}
		parts = append(parts, app.P().Class("portal-section__intro").Body(app.Text(strings.Join(contact, " • "))))
	}
	if len(appliance) > 0 {
		btus := "Split"
		if value := portalText(appliance["btus"]); value != "" {
			btus = value + " BTUs"
		}
		applianceName := strings.TrimSpace(portalText(appliance["marca"]) + " " + portalText(appliance["modelo"]))
		applianceName = firstNonEmptyBudget(applianceName, "Aparelho") + " (" + btus + ") - " + firstNonEmptyBudget(portalText(appliance["ambiente"]), "Ambiente")
		parts = append(parts, app.P().Class("portal-section__intro").Body(app.Text("Aparelho: "+applianceName)))
	}
	if problem := portalText(service["problema"]); problem != "" {
		parts = append(parts, app.P().Class("portal-section__intro").Body(app.Text("Observações / Sintomas: \""+problem+"\"")))
	}
	if date != "" {
		parts = append(parts, app.P().Class("portal-service__date").Body(app.Text("Data Preferencial: "+portalDate(date))))
	}
	for _, field := range []struct{ key, label string }{{"data_inicio", "Iniciado em: "}, {"data_conclusao", "Concluído em: "}, {"data_cancelamento", "Cancelado em: "}} {
		if value := portalText(service[field.key]); value != "" {
			parts = append(parts, app.P().Class("portal-service__date").Body(app.Text(field.label+portalDate(value))))
		}
	}

	actions := []app.UI{}
	if phone != "" {
		actions = append(actions, app.A().Class("auth-submit").Href(teamRequestWhatsAppURL(service)).Target("_blank").Rel("noopener noreferrer").Body(app.Text("WhatsApp")))
		actions = append(actions, app.Button().Class("auth-link").Type("button").OnClick(p.askTeamCustomerAction(teamDeleteServiceAction+id)).Body(app.Text("Excluir serviço")))
	}
	if status == "PENDENTE" || status == "AGENDADO" {
		if portalText(appliance["id"]) != "" {
			actions = append(actions, app.Button().Class("auth-submit").Type("button").OnClick(p.startTeamRequestAndOpenChecklist(service)).Body(app.Text("Iniciar Serviço")))
		}
		if status == "PENDENTE" {
			actions = append(actions, app.Button().Class("auth-submit").Type("button").OnClick(p.openTeamSchedule(service)).Body(app.Text("Agendar Serviço")))
		}
		actions = append(actions, app.Button().Class("auth-submit").Type("button").OnClick(p.openTeamBudgetFormForService(service)).Body(app.Text("Gerar Orçamento")))
	}
	if status == "AGENDADO" || status == "EM_ANDAMENTO" {
		actions = append(actions, app.Button().Class("auth-submit").Type("button").OnClick(p.updateTeamRequestStatus(id, "CONCLUIDO")).Body(app.Text("Concluir Serviço")))
	}
	if status == "PENDENTE" || status == "AGENDADO" || status == "EM_ANDAMENTO" {
		actions = append(actions, app.Button().Class("auth-link").Type("button").OnClick(p.requestTeamCancellation(id)).Body(app.Text("Cancelar Serviço")))
	}
	parts = append(parts, app.Div().Class("portal-budget__actions").Body(actions...))
	return app.Article().Class("portal-service team-request-card").Body(parts...)
}

type teamRequestQueueGroup struct {
	CustomerID, ApplianceID string
	Customer, Appliance     string
	Phone                   string
	Services                []map[string]any
}

func groupTeamRequestServices(services []map[string]any) []teamRequestQueueGroup {
	groups := make([]teamRequestQueueGroup, 0, len(services))
	indexes := make(map[string]int, len(services))
	for _, service := range services {
		customer, _ := service["customers"].(map[string]any)
		appliance, _ := service["air_conditioners"].(map[string]any)
		clientID, applianceID := portalText(service["cliente_id"]), portalText(service["aparelho_id"])
		key := clientID + ":" + applianceID
		if clientID == "" && applianceID == "" {
			key = "service:" + portalText(service["id"])
		}
		index, exists := indexes[key]
		if !exists {
			index = len(groups)
			indexes[key] = index
			applianceName := strings.TrimSpace(portalText(appliance["marca"]) + " " + portalText(appliance["modelo"]))
			if btus := portalText(appliance["btus"]); btus != "" {
				applianceName = strings.TrimSpace(applianceName + " • " + portalBTUs(btus))
			}
			if room := portalText(appliance["ambiente"]); room != "" {
				applianceName = strings.TrimSpace(applianceName + " • " + room)
			}
			groups = append(groups, teamRequestQueueGroup{
				CustomerID: clientID, ApplianceID: applianceID,
				Customer:  firstNonEmptyBudget(portalText(customer["nome"]), "Cliente não identificado"),
				Appliance: firstNonEmptyBudget(applianceName, "Aparelho"),
				Phone:     portalText(customer["whatsapp"]),
			})
		}
		groups[index].Services = append(groups[index].Services, service)
	}
	return groups
}

func (p *serviceCatalogPage) teamRequestQueueCard(group teamRequestQueueGroup) app.UI {
	rows := make([]app.UI, 0, len(group.Services))
	for _, service := range group.Services {
		id := portalText(service["id"])
		label := teamRequestServiceName(service)
		if requested := agendaDate(service["data_solicitacao"]); requested != "" {
			label += " • pedido em " + portalDate(requested)
		}
		actions := []app.UI{
			app.Button().Class("auth-link").Type("button").OnClick(p.openTeamSchedule(service)).Body(app.Text("Agendar Serviço")),
			app.Button().Class("auth-link").Type("button").OnClick(p.updateTeamRequestStatus(id, "EM_ANDAMENTO")).Body(app.Text("Iniciar")),
			app.Button().Class("auth-link").Type("button").OnClick(p.requestTeamCancellation(id)).Body(app.Text("Cancelar")),
		}
		rows = append(rows, app.Div().Class("team-request-queue__row").Body(
			app.Span().Class("team-request-queue__description").Body(app.Text(label)),
			app.Div().Class("portal-budget__actions").Body(actions...),
		))
	}
	heading := []app.UI{
		app.Strong().Body(app.Text(group.Customer)),
		app.Span().Class("portal-service__status portal-service__status--pending").Body(app.Text(strconv.Itoa(len(group.Services)) + " chamado(s)")),
	}
	card := []app.UI{
		app.Div().Class("portal-service__heading").Body(heading...),
		app.P().Class("portal-section__intro").Body(app.Text(group.Appliance)),
		app.Div().Class("team-request-queue__items").Body(rows...),
	}
	if phone := cleanTeamContactPhone(group.Phone); phone != "" && len(group.Services) > 0 {
		card = append(card, app.A().Class("auth-submit").Href(teamRequestWhatsAppURL(group.Services[0])).Target("_blank").Rel("noopener noreferrer").Body(app.Text("WhatsApp")))
	}
	if len(group.Services) > 0 {
		card = append(card, app.Button().Class("auth-link").Type("button").OnClick(p.openTeamBudgetFormForService(group.Services[0])).Body(app.Text("Gerar Orçamento")))
	}
	return app.Article().Class("portal-service team-request-queue__card").Body(card...)
}

func teamRequestServiceName(service map[string]any) string {
	if description := portalText(service["descricao"]); description != "" {
		return strings.Replace(domain.NormalizeServiceName(description), "_", " ", 1)
	}
	kind := portalText(service["tipo"])
	if kind == "OUTRO" {
		return "Serviço"
	}
	return string(supabase.MapSupabaseServiceTypeToLocal(kind))
}

func teamRequestWhatsAppURL(service map[string]any) string {
	customer, _ := service["customers"].(map[string]any)
	phone := cleanTeamContactPhone(portalText(customer["whatsapp"]))
	if phone == "" {
		return "#"
	}
	name := firstNonEmptyBudget(portalText(customer["nome"]), "Cliente")
	serviceName := teamRequestServiceName(service)
	message := fmt.Sprintf("Olá %s, aqui é da Inovar Refrigeração! Vi sua solicitação no app para o serviço de %s. Vamos confirmar o horário de atendimento?", name, serviceName)
	return "https://wa.me/" + phone + "?text=" + url.QueryEscape(message)
}

func filterTeamRequestServices(services []map[string]any, filter, search, today string) []map[string]any {
	filtered := make([]map[string]any, 0, len(services))
	filter = strings.ToUpper(strings.TrimSpace(filter))
	if filter == "" {
		filter = "TODOS"
	}
	for _, service := range services {
		status := strings.ToUpper(portalText(service["status"]))
		if status == "CANCELADO" || filter == "TODOS" && status == "CONCLUIDO" || filter != "TODOS" && status != filter {
			continue
		}
		if teamServiceMatchesSearch(service, search) {
			filtered = append(filtered, service)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		leftPriority := teamRequestPriority(filtered[i], today)
		rightPriority := teamRequestPriority(filtered[j], today)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		leftDate := firstNonEmptyBudget(agendaDate(filtered[i]["data_agendamento"]), "9999-12-31")
		rightDate := firstNonEmptyBudget(agendaDate(filtered[j]["data_agendamento"]), "9999-12-31")
		return leftDate < rightDate
	})
	return filtered
}

func teamRequestPriority(service map[string]any, today string) int {
	status := strings.ToUpper(portalText(service["status"]))
	date := agendaDate(service["data_agendamento"])
	if status == "CONCLUIDO" {
		return 9
	}
	if status == "CANCELADO" {
		return 10
	}
	if date != "" && date < today {
		return 0
	}
	if date == today {
		return 1
	}
	if status == "AGENDADO" && date != "" {
		return 2
	}
	if status == "AGENDADO" {
		return 3
	}
	return 4
}

func isOverdueTeamRequest(service map[string]any, today string) bool {
	status := strings.ToUpper(portalText(service["status"]))
	date := agendaDate(service["data_agendamento"])
	return date != "" && date < today && (status == "PENDENTE" || status == "AGENDADO")
}

func countOverdueTeamRequests(services []map[string]any, today string) int {
	count := 0
	for _, service := range services {
		if isOverdueTeamRequest(service, today) {
			count++
		}
	}
	return count
}

func (p *serviceCatalogPage) startTeamRequestAndOpenChecklist(service map[string]any) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		status := strings.ToUpper(portalText(service["status"]))
		if status != "PENDENTE" && status != "AGENDADO" && status != "EM_ANDAMENTO" {
			return
		}
		if p.session == nil {
			return
		}
		p.teamStartServiceForm = newTeamStartServiceFormForRequest(service)
		p.beginTeamStartService(ctx)
	}
}

func teamRequestStatusPayload(status, observations, date string) (map[string]any, map[string]any, string, string, bool) {
	dateField, marker, messageKey, notice := "", "", "", ""
	switch status {
	case "EM_ANDAMENTO":
		dateField, marker, messageKey, notice = "data_inicio", "DATA_INICIO", "status_em_andamento", "Serviço iniciado."
	case "CONCLUIDO":
		dateField, marker, messageKey, notice = "data_conclusao", "DATA_CONCLUSAO", "status_concluido", "Serviço concluído."
	default:
		return nil, nil, "", "", false
	}
	primary := map[string]any{"status": status, dateField: date}
	fallback := map[string]any{"status": status, "observacoes": domain.SetServiceDateMarker(observations, marker, date)}
	return primary, fallback, messageKey, notice, true
}

func (p *serviceCatalogPage) updateTeamRequestStatus(serviceID, status string) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		if serviceID == "" || p.session == nil {
			return
		}
		pageURL := app.Window().URL()
		if pageURL == nil || pageURL.Scheme == "" || pageURL.Host == "" {
			return
		}
		var service map[string]any
		for _, row := range p.teamServices {
			if portalText(row["id"]) == serviceID {
				service = row
				break
			}
		}
		if service == nil {
			p.teamActionMessage = "Não foi possível identificar a ordem de serviço."
			ctx.Update()
			return
		}
		client, _ := service["customers"].(map[string]any)
		date := dashboardCivilToday(time.Now()).Format("2006-01-02")
		primary, fallback, templateKey, notice, valid := teamRequestStatusPayload(status, portalText(service["observacoes"]), date)
		if !valid {
			return
		}
		baseURL, token, profile := apiBaseURL(), p.session.AccessToken, p.teamProfile
		go func() {
			err := sendTeamJSON(ctx, baseURL+"/api/servicos", token, http.MethodPatch, map[string]any{"id": serviceID, "fields": primary})
			if err != nil {
				err = sendTeamJSON(ctx, baseURL+"/api/servicos", token, http.MethodPatch, map[string]any{"id": serviceID, "fields": fallback})
			}
			if err != nil {
				p.teamActionMessage = "Não foi possível atualizar a OS. Tente novamente."
				ctx.Update()
				return
			}
			if phone := portalText(client["whatsapp"]); phone != "" {
				message := teamLegacyStatusMessage(profile, service, client, templateKey)
				if message != "" {
					result, sendErr := sendTeamJSONResult(ctx, baseURL+"/api/whatsapp", token, http.MethodPost, map[string]any{"acao": "enviar", "evento": templateKey, "origem_tipo": "servico", "origem_id": serviceID, "telefone": phone, "texto": message})
					if sendErr != nil || result["ok"] != true {
						notice += " Não foi possível enviar a notificação pelo WhatsApp."
					}
				}
			}
			p.teamActionMessage = notice
			p.teamLoadedFor = ""
			p.loadTeamOperations(ctx)
			ctx.Update()
		}()
	}
}

func filterTeamContactReturns(items []teamReturnItem, filter, search string) []teamReturnItem {
	query := strings.TrimSpace(search)
	filtered := make([]teamReturnItem, 0, len(items))
	for _, item := range items {
		if filter == "todos" && item.Status == string(domain.ReturnNoHistory) && len(item.PendingServiceIDs) == 0 && len(item.InProgressServiceIDs) == 0 {
			continue
		}
		if filter == "chamados" && len(item.PendingServiceIDs) == 0 {
			continue
		}
		if filter != "todos" && filter != "chamados" && filter != item.Status {
			continue
		}
		if query != "" {
			if !searchMatches(query, item.Customer, item.Phone, cleanTeamContactPhone(item.Phone), item.Appliance, item.Room, item.Brand, item.Model, item.Capacity) {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func teamServiceMatchesSearch(service map[string]any, search string) bool {
	query := strings.TrimSpace(search)
	if query == "" {
		return true
	}
	customer := ""
	if row, ok := service["customers"].(map[string]any); ok {
		customer = portalText(row["nome"]) + " " + portalText(row["whatsapp"])
	}
	return searchMatches(query, customer, portalText(service["descricao"]), portalText(service["tipo"]), portalText(service["problema"]))
}

func cleanTeamContactPhone(phone string) string {
	digits := strings.Builder{}
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	cleaned := digits.String()
	if len(cleaned) == 10 || len(cleaned) == 11 {
		return "55" + cleaned
	}
	return cleaned
}

func returnWhatsAppURL(item teamReturnItem, profile domain.TechnicianProfile) string {
	phone := cleanTeamContactPhone(item.Phone)
	firstName := strings.Fields(item.Customer)
	if len(firstName) > 0 {
		item.Customer = firstName[0]
	}
	tech := strings.TrimSpace(profile.Name)
	if tech == "" {
		tech = "seu técnico de ar-condicionado"
	}
	typeName := item.Type
	if typeName == "" {
		typeName = "Ar-condicionado"
	}
	capacity := ""
	if item.Capacity != "" {
		capacity = " " + item.Capacity + " BTUs"
	}
	room := item.Room
	if room == "" {
		room = "ambiente"
	}
	appDesc := strings.TrimSpace(typeName + capacity + " (" + room + ")")
	if item.Status == string(domain.ReturnNoHistory) {
		message := fmt.Sprintf("Olá, *%s*! Tudo bem? 😊\n\nAqui é o *%s* da *Inovar Refrigeração*.\n\nVi que temos o cadastro do seu ar-condicionado (*%s*). Para garantir ar puro, economia de energia e máximo rendimento, que tal agendarmos uma revisão ou limpeza de ar preventiva?\n\nQual dia fica melhor para você? ❄️🔧", item.Customer, tech, appDesc)
		return "https://api.whatsapp.com/send?phone=" + phone + "&text=" + url.QueryEscape(message)
	}
	months := 6
	lastDate := ""
	if item.LastMaintenanceDate != "" {
		if date, err := time.Parse("2006-01-02", item.LastMaintenanceDate); err == nil {
			now := time.Now()
			months = (now.Year()-date.Year())*12 + int(now.Month()-date.Month())
			if now.Day() < date.Day() {
				months--
			}
			if months < 1 {
				months = 1
			}
			monthNames := [...]string{"janeiro", "fevereiro", "março", "abril", "maio", "junho", "julho", "agosto", "setembro", "outubro", "novembro", "dezembro"}
			lastDate = fmt.Sprintf(" realizada em %02d de %s de %04d", date.Day(), monthNames[int(date.Month())-1], date.Year())
		}
	}
	message := fmt.Sprintf("Olá, *%s*! Tudo bem? 😊\n\nAqui é o *%s*. \n\nEstou entrando em contato pois já faz aproximadamente *%d meses* desde a última limpeza de ar preventiva do seu ar-condicionado (*%s*)%s.\n\nCom o uso contínuo, poeira, ácaros e fungos se acumulam na serpentina e turbina, o que pode:\n⚠️ Aumentar o consumo de energia em até 30%%\n⚠️ Reduzir a potência de refrigeração\n⚠️ Causar alergias e irritação respiratória\n\nPara manter o ar do seu ambiente 100%% puro e o aparelho funcionando como novo, vamos agendar a revisão preventiva para esta semana?\n\nQual dia fica melhor para você: *pela manhã* ou *à tarde*? ❄️🔧", item.Customer, tech, months, appDesc, lastDate)
	return "https://api.whatsapp.com/send?phone=" + phone + "&text=" + url.QueryEscape(message)
}

func truncateTeamObservationUTF16(value string, limit int) string {
	if limit < 0 {
		return ""
	}
	var out strings.Builder
	units := 0
	for _, r := range value {
		encoded := utf16.Encode([]rune{r})
		if units+len(encoded) > limit {
			break
		}
		out.WriteRune(r)
		units += len(encoded)
	}
	return out.String()
}

func (p *serviceCatalogPage) markTeamContacted(item teamReturnItem) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		if p.session == nil || item.CustomerID == "" || item.ApplianceID == "" {
			return
		}
		var selected map[string]any
		for _, service := range p.teamServices {
			if portalText(service["cliente_id"]) != item.CustomerID || portalText(service["aparelho_id"]) != item.ApplianceID {
				continue
			}
			if selected == nil || serviceEffectiveDate(service) > serviceEffectiveDate(selected) {
				selected = service
			}
		}
		if selected == nil {
			return
		}
		now := time.Now().Format("02/01 15:04")
		observations := portalText(selected["observacoes"])
		if observations != "" {
			observations += " | "
		}
		observations = truncateTeamObservationUTF16(observations+"[Contato feito "+now+"]", 900)
		serviceID, token := portalText(selected["id"]), p.session.AccessToken
		if serviceID == "" {
			return
		}
		go func() {
			_, err := sendTeamJSONResult(ctx, apiBaseURL()+"/api/servicos", token, http.MethodPatch, map[string]any{
				"id": serviceID, "fields": map[string]any{"observacoes": observations},
			})
			if p.session == nil || p.session.AccessToken != token {
				return
			}
			if err != nil {
				p.teamActionMessage = "Contato iniciado; não foi possível registrar no histórico da OS."
			} else {
				selected["observacoes"] = observations
				p.teamActionMessage = "Contato registrado no histórico da OS."
			}
			ctx.Update()
		}()
	}
}
