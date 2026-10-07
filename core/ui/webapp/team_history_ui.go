package webapp

import (
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/domain"
)

var (
	teamHistoryReturnMarker       = regexp.MustCompile(`\[PROXIMO_RETORNO:(\d{4}-\d{2}-\d{2})\]`)
	teamHistoryContactMarker      = regexp.MustCompile(`\[Contato feito ([^\]]+)\]`)
	teamHistoryWarrantyTextMarker = regexp.MustCompile(`(?i)Garantia de (\d+) dias?\.?`)
)

const (
	teamDeleteLegacyHistoryAction  = "delete-history-legacy:"
	teamDeleteHistoryServiceAction = "delete-history-service:"
)

type teamHistoryDisplay struct {
	Notes, Payment, Warranty string
	Contacts                 []string
}

func formatTeamHistoryDisplay(observations string) teamHistoryDisplay {
	display := teamHistoryDisplay{}
	seenContacts := map[string]struct{}{}
	for _, match := range teamHistoryContactMarker.FindAllStringSubmatch(observations, -1) {
		if len(match) != 2 {
			continue
		}
		contact := strings.TrimSpace(match[1])
		if contact == "" {
			continue
		}
		if _, exists := seenContacts[contact]; exists {
			continue
		}
		seenContacts[contact] = struct{}{}
		display.Contacts = append(display.Contacts, contact)
	}

	display.Payment = markerValue(teamServicePaymentMarker, observations)
	if warranty := teamServiceWarrantyMarker.FindStringSubmatch(observations); len(warranty) == 2 {
		display.Warranty = warranty[1] + " dias"
	} else if warranty := teamHistoryWarrantyTextMarker.FindStringSubmatch(observations); len(warranty) == 2 {
		display.Warranty = warranty[1] + " dias"
	}

	clean := teamHistoryContactMarker.ReplaceAllString(observations, "")
	clean = teamServiceInternalMarkers.ReplaceAllString(clean, "")
	clean = stripTeamServiceChecklistJSON(clean)
	clean = teamServicePaymentMarker.ReplaceAllString(clean, "")
	clean = teamHistoryWarrantyTextMarker.ReplaceAllString(clean, "")
	clean = strings.TrimSpace(clean)
	clean = strings.Trim(clean, "| ")
	if strings.HasPrefix(strings.ToLower(clean), "serviço de campo concluído") {
		if end := strings.Index(clean, "."); end >= 0 {
			clean = strings.TrimSpace(clean[end+1:])
		}
	}
	parts := strings.Split(clean, "|")
	cleanParts := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" && len(teamServiceChecklistData(part)) == 0 {
			cleanParts = append(cleanParts, part)
		}
	}
	display.Notes = strings.Join(cleanParts, " · ")
	return display
}

type teamHistoryForm struct {
	ApplianceID string
	CustomerID  string
	Type        string
	Date        string
	Price       string
	Notes       string
	UpdateCycle bool
	Saving      bool
	Message     string
}

func (p *serviceCatalogPage) toggleApplianceHistory(applianceID string) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		if p.teamHistoryApplianceID == applianceID {
			p.teamHistoryApplianceID = ""
			p.teamHistoryNotice = ""
			if p.teamQueueSection == teamQueueReturns {
				for _, item := range p.teamDashboard.ReturnEvents {
					if item.ApplianceID == applianceID && p.teamQueueSelectedKey == teamReturnQueueKey(item) {
						p.teamQueueSelectedKey = ""
						break
					}
				}
			}
			ctx.Update()
			return
		}
		p.teamHistoryApplianceID = applianceID
		if p.teamQueueSection == teamQueueReturns {
			for _, item := range p.teamDashboard.ReturnEvents {
				if item.ApplianceID == applianceID {
					p.teamQueueSelectedKey = teamReturnQueueKey(item)
					break
				}
			}
		}
		p.teamHistoryNotice = "Carregando histórico..."
		p.teamHistoryRows = nil
		p.teamHistoryLegacyRows = nil
		ctx.Update()
		if p.session == nil {
			p.teamHistoryNotice = "Entre novamente para carregar o histórico."
			ctx.Update()
			return
		}
		pageURL := app.Window().URL()
		if pageURL == nil || pageURL.Scheme == "" || pageURL.Host == "" {
			p.teamHistoryNotice = "Não foi possível conectar ao servidor."
			ctx.Update()
			return
		}
		base, token := apiBaseURL(), p.session.AccessToken
		go func() {
			rows, legacyRows, err := fetchTeamApplianceHistory(ctx, base, token)
			if err != nil {
				if p.offlineMode {
					p.teamHistoryRows, p.teamHistoryLegacyRows = p.teamServices, p.teamReturnHistory
					p.teamHistoryNotice = "Sem conexão: mostrando os registros salvos neste aparelho."
					ctx.Update()
					return
				}
				p.teamHistoryNotice = "Não foi possível carregar o histórico."
				ctx.Update()
				return
			}
			p.teamHistoryRows = rows
			p.teamHistoryLegacyRows = legacyRows
			p.teamHistoryNotice = ""
			ctx.Update()
		}()
	}
}

func fetchTeamApplianceHistory(ctx app.Context, baseURL, token string) ([]map[string]any, []map[string]any, error) {
	var services, legacy []map[string]any
	var servicesErr, legacyErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		services, servicesErr = getTeamRows(ctx, baseURL+"/api/historico?origem=timeline-services", token)
	}()
	go func() {
		defer wg.Done()
		legacy, legacyErr = getTeamRows(ctx, baseURL+"/api/historico?origem=retornos-avulsos", token)
	}()
	wg.Wait()
	if servicesErr != nil {
		return nil, nil, servicesErr
	}
	if legacyErr != nil {
		return nil, nil, legacyErr
	}
	return services, legacy, nil
}

func (p *serviceCatalogPage) openTeamHistoryForm(applianceID string) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		var customerID string
		for _, customer := range p.teamCustomers {
			appliances, _ := customer["appliances"].([]any)
			for _, raw := range appliances {
				appliance, _ := raw.(map[string]any)
				if portalText(appliance["id"]) == applianceID {
					customerID = portalText(customer["id"])
				}
			}
		}
		months := p.teamProfile.DefaultReturnMonths
		if months <= 0 {
			months = 3
		}
		serviceType, price := teamHistoryInitialSelection(p.teamProfile)
		p.teamHistoryForm = &teamHistoryForm{ApplianceID: applianceID, CustomerID: customerID, Type: serviceType, Date: time.Now().AddDate(0, -months, 0).Format("2006-01-02"), Price: strconv.FormatFloat(price, 'f', 2, 64), UpdateCycle: true}
		ctx.Update()
	}
}

func teamHistoryServiceCatalog(profile domain.TechnicianProfile) []domain.CatalogEntry {
	return domain.BuildServiceCatalog(&profile)
}

func teamHistoryInitialSelection(profile domain.TechnicianProfile) (string, float64) {
	price := profile.DefaultPrice
	if price <= 0 {
		price = 250
	}
	serviceType := string(domain.ServiceCleaning)
	if catalog := teamHistoryServiceCatalog(profile); len(catalog) > 0 {
		serviceType = catalog[0].Name
		if catalog[0].Price > 0 {
			price = catalog[0].Price
		}
	}
	return serviceType, price
}

func updateTeamHistoryServiceType(form *teamHistoryForm, profile domain.TechnicianProfile, serviceType string) {
	if form == nil {
		return
	}
	form.Type = serviceType
	for _, entry := range teamHistoryServiceCatalog(profile) {
		if entry.Name == serviceType && entry.Price > 0 {
			form.Price = strconv.FormatFloat(entry.Price, 'f', -1, 64)
			return
		}
	}
}

func (p *serviceCatalogPage) saveTeamHistory(ctx app.Context, event app.Event) {
	event.PreventDefault()
	f := p.teamHistoryForm
	if f == nil || f.Saving || p.session == nil {
		return
	}
	price, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(f.Price), ",", "."), 64)
	if err != nil || math.IsNaN(price) || math.IsInf(price, 0) || price < 0 {
		f.Message = "Informe um valor válido."
		return
	}
	if f.Date == "" || f.CustomerID == "" || f.ApplianceID == "" {
		f.Message = "Confira cliente, aparelho e data."
		return
	}
	if parsedDate, parseErr := time.Parse("2006-01-02", f.Date); parseErr != nil || parsedDate.Format("2006-01-02") != f.Date {
		f.Message = "Informe uma data válida."
		return
	}
	if f.Date > time.Now().Format("2006-01-02") {
		f.Message = "A data do histórico não pode ser futura."
		return
	}
	pageURL := app.Window().URL()
	if pageURL == nil || pageURL.Scheme == "" || pageURL.Host == "" {
		f.Message = "Servidor indisponível."
		return
	}
	base, token := apiBaseURL(), p.session.AccessToken
	f.Saving, f.Message = true, "Registrando histórico..."
	go func() {
		obs := strings.TrimSpace(f.Notes)
		if obs != "" {
			obs += " | "
		}
		obs += "Histórico anterior registrado manualmente."
		service := map[string]any{"cliente_id": f.CustomerID, "aparelho_id": f.ApplianceID, "tipo": supabase.MapServiceTypeToSupabase(strings.TrimSpace(f.Type)), "status": "CONCLUIDO", "valor": price, "data_agendamento": f.Date, "descricao": f.Type, "observacoes": obs}
		serviceRow, err := sendTeamJSONResult(ctx, base+"/api/historico?retroativo=1", token, http.MethodPost, service)
		cycleUpdateFailed := false
		if err == nil && f.UpdateCycle {
			_, cycleErr := sendTeamJSONResult(ctx, base+"/api/aparelho-manutencao", token, http.MethodPatch, map[string]any{"id": f.ApplianceID, "fields": map[string]any{"ultima_manutencao": f.Date}})
			cycleUpdateFailed = cycleErr != nil
		}
		if err == nil {
			p.registerLocalNotification(ctx, "Histórico registrado", "O ciclo de retorno já está em contagem.")
			aux := map[string]any{"cliente_id": f.CustomerID, "aparelho_id": f.ApplianceID, "data": f.Date, "descricao": f.Type, "observacoes": strings.TrimSpace(f.Notes), "valor": price}
			_, _ = sendTeamJSONResult(ctx, base+"/api/historico", token, http.MethodPost, aux)
			p.teamCustomerNotice = "Histórico registrado."
			if cycleUpdateFailed {
				p.teamCustomerNotice = "Histórico registrado, mas o ciclo de retorno não foi atualizado."
			}
			p.teamHistoryForm = nil
			p.teamLoadedFor = ""
			p.loadTeamOperations(ctx)
			if rows, legacyRows, loadErr := fetchTeamApplianceHistory(ctx, base, token); loadErr == nil {
				p.teamHistoryRows = rows
				p.teamHistoryLegacyRows = legacyRows
				p.teamHistoryNotice = ""
			}
			if portalText(serviceRow["id"]) != "" {
				serviceRow["customers"] = appointmentClient(p.teamCustomers, f.CustomerID)
				for _, customer := range p.teamCustomers {
					if portalText(customer["id"]) != f.CustomerID {
						continue
					}
					for _, raw := range applianceMaps(customer) {
						if portalText(raw["id"]) == f.ApplianceID {
							serviceRow["air_conditioners"] = raw
						}
					}
				}
				p.showTeamServiceDetail(ctx, serviceRow)
			}
		} else {
			f.Saving, f.Message = false, "Não foi possível registrar o histórico. Verifique conexão e permissões."
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) applianceHistoryPanel() app.UI {
	if p.teamHistoryApplianceID == "" {
		return app.Div()
	}
	rows := make([]app.UI, 0)
	services := make([]map[string]any, 0)
	serviceIDs := make(map[string]struct{})
	for _, service := range p.teamHistoryRows {
		if portalText(service["aparelho_id"]) == p.teamHistoryApplianceID {
			services = append(services, service)
			if id := portalText(service["id"]); id != "" {
				serviceIDs[id] = struct{}{}
			}
		}
	}
	legacyByService := make(map[string][]map[string]any)
	for _, history := range p.teamHistoryLegacyRows {
		if portalText(history["aparelho_id"]) != p.teamHistoryApplianceID {
			continue
		}
		if serviceID := portalText(history["service_id"]); serviceID != "" {
			if _, exists := serviceIDs[serviceID]; exists {
				legacyByService[serviceID] = append(legacyByService[serviceID], history)
			}
		}
	}
	sort.SliceStable(services, func(i, j int) bool { return teamHistorySortDate(services[i]) > teamHistorySortDate(services[j]) })
	if len(services) > 0 {
		rows = append(rows, app.Li().Class("team-appliance-history__group").Body(app.Strong().Body(app.Text("Atendimentos e ordens de serviço ("+itoaPhoto(len(services))+")"))))
	}
	for _, service := range services {
		rows = append(rows, p.teamApplianceServiceHistoryCard(enrichServiceHistory(service, legacyByService[portalText(service["id"])])))
	}

	budgets := make([]domain.BudgetEstimate, 0)
	for _, budget := range p.teamBudgets {
		if budget.ApplianceID != nil && *budget.ApplianceID == p.teamHistoryApplianceID {
			budgets = append(budgets, budget)
		}
	}
	sort.SliceStable(budgets, func(i, j int) bool { return budgets[i].Date > budgets[j].Date })
	if len(budgets) > 0 {
		rows = append(rows, app.Li().Class("team-appliance-history__group").Body(app.Strong().Body(app.Text("Orçamentos ("+itoaPhoto(len(budgets))+")"))))
	}
	for _, budget := range budgets {
		ref := budget.Number
		if ref == "" {
			ref = budget.ID
		}
		label := "Orçamento #" + ref + " • " + string(budget.Status)
		facts := []app.UI{teamHistoryFact("Emitido", formatTeamHistoryDate(budget.Date), ""), teamHistoryFact("Validade", formatTeamHistoryDate(budget.ValidUntil), ""), teamHistoryFact("Valor total", "R$ "+formatPortalMoney(budget.FinalValue), "team-appliance-history__fact--value")}
		item := []app.UI{app.Div().Class("team-appliance-history__item-heading").Body(app.Strong().Class("team-appliance-history__service-name").Body(app.Text(label))), app.Div().Class("team-appliance-history__facts").Body(facts...)}
		if len(budget.Items) > 0 {
			item = append(item, app.P().Class("team-appliance-history__note").Body(app.Text(teamHistoryBudgetItemsSummary(budget.Items))))
		}
		item = append(item, app.Button().Class("auth-submit team-appliance-history__action").Type("button").Disabled(p.teamBudgetBusy).OnClick(p.downloadTeamBudgetPDF(budget)).Body(app.Text("Reimprimir orçamento (PDF)")))
		rows = append(rows, app.Li().Class("team-appliance-history__item").Body(item...))
	}

	legacyRows := make([]map[string]any, 0)
	for _, row := range p.teamHistoryLegacyRows {
		if portalText(row["aparelho_id"]) != p.teamHistoryApplianceID {
			continue
		}
		if serviceID := portalText(row["service_id"]); serviceID != "" {
			if _, exists := serviceIDs[serviceID]; exists {
				continue
			}
		}
		legacyRows = append(legacyRows, row)
	}
	sort.SliceStable(legacyRows, func(i, j int) bool { return portalText(legacyRows[i]["data"]) > portalText(legacyRows[j]["data"]) })
	if len(legacyRows) > 0 {
		rows = append(rows, app.Li().Class("team-appliance-history__group").Body(app.Strong().Body(app.Text("Registros de histórico anteriores ("+itoaPhoto(len(legacyRows))+")"))))
	}
	for _, row := range legacyRows {
		details := formatTeamHistoryDisplay(portalText(row["observacoes"]))
		checklistRows := teamServiceChecklistRows(row)
		title := firstNonEmptyBudget(portalText(row["descricao"]), portalText(row["solucao"]), portalText(row["problema"]), "Atendimento registrado")
		facts := []app.UI{teamHistoryFact("Data registrada", formatTeamHistoryDate(portalText(row["data"])), ""), teamHistoryFact("Valor", "R$ "+formatPortalMoney(agendaNumber(row["valor"])), "team-appliance-history__fact--value")}
		item := []app.UI{app.Div().Class("team-appliance-history__item-heading").Body(app.Strong().Class("team-appliance-history__service-name").Body(app.Text(title)), app.Span().Class("team-appliance-history__status").Body(app.Text("Histórico anterior"))), app.Div().Class("team-appliance-history__facts").Body(facts...)}
		if details.Notes != "" {
			item = append(item, app.P().Class("team-appliance-history__note").Body(app.Text(details.Notes)))
		}
		if len(checklistRows) > 0 {
			item = append(item, teamHistoryChecklistPanel(checklistRows))
		}
		for _, detail := range []struct{ label, value string }{{"Problema", portalText(row["problema"])}, {"Diagnóstico", portalText(row["diagnostico"])}, {"Solução", portalText(row["solucao"])}, {"Peças utilizadas", portalText(row["pecas_utilizadas"])}} {
			if detail.value != "" {
				item = append(item, app.P().Class("team-appliance-history__note").Body(app.Strong().Body(app.Text(detail.label+": ")), app.Text(detail.value)))
			}
		}
		if id := portalText(row["id"]); id != "" {
			item = append(item, app.Button().Class("auth-link team-appliance-history__delete").Type("button").OnClick(p.askTeamCustomerAction(teamDeleteLegacyHistoryAction+id)).Body(app.Text("Excluir histórico")))
		}
		rows = append(rows, app.Li().Class("team-appliance-history__item").Body(item...))
	}
	message := p.teamHistoryNotice
	if message == "" {
		message = "Histórico de atendimento, ordens de serviço e orçamentos deste aparelho."
	}
	if len(rows) > 0 {
		message = "Registros do banco organizados por tipo, com status, datas, valores e detalhes."
	} else if p.teamHistoryNotice == "" {
		message = "Ainda não há atendimentos, orçamentos ou ordens registrados para este aparelho."
	}
	return app.Div().Class("portal-service team-appliance-history").Body(
		app.Div().Class("team-appliance-history__header").Body(
			app.Div().Body(app.Strong().Class("team-appliance-history__title").Body(app.Text("Histórico completo do aparelho")), app.P().Class("portal-section__intro").Body(app.Text(message))),
			app.Button().Class("auth-link team-appliance-history__add").Type("button").OnClick(p.openTeamHistoryForm(p.teamHistoryApplianceID)).Body(app.Text("＋ Registrar histórico")),
		),
		app.Ul().Class("team-appliance-history__list").Body(rows...),
	)
}

func (p *serviceCatalogPage) teamApplianceServiceHistoryCard(row map[string]any) app.UI {
	status := strings.ToUpper(portalText(row["status"]))
	statusLabel, statusTone := portalServiceStatus(status)
	switch status {
	case "PENDENTE":
		statusLabel = "Solicitado"
	case "AGENDADO":
		statusLabel = "Agendado"
	case "EM_ANDAMENTO":
		statusLabel = "Em andamento"
	case "CONCLUIDO":
		statusLabel = "Concluído"
	case "CANCELADO":
		statusLabel = "Cancelado"
	default:
		statusLabel = firstNonEmptyBudget(status, "Status não informado")
	}
	title := firstNonEmptyBudget(portalText(row["descricao"]), teamServiceDisplayName(row), "Ordem de serviço")
	observations := portalText(row["observacoes"])
	datePairs := []struct{ label, value string }{
		{"Solicitado", portalText(row["data_solicitacao"])},
		{"Agendado", portalText(row["data_agendamento"])},
		{"Início", firstNonEmptyBudget(portalText(row["data_inicio"]), markerValue(teamServiceStartMarker, observations))},
		{"Conclusão", firstNonEmptyBudget(portalText(row["data_conclusao"]), markerValue(teamServiceDoneMarker, observations))},
		{"Cancelamento", firstNonEmptyBudget(portalText(row["data_cancelamento"]), markerValue(teamServiceCancelDateMarker, observations))},
	}
	facts := make([]app.UI, 0, len(datePairs)+1)
	for _, pair := range datePairs {
		if pair.value != "" {
			facts = append(facts, teamHistoryFact(pair.label, formatTeamHistoryDate(pair.value), ""))
		}
	}
	facts = append(facts, teamHistoryFact("Valor", "R$ "+formatPortalMoney(agendaNumber(row["valor"])), "team-appliance-history__fact--value"))
	if returnDate := teamHistoryReturnDate(row, p.teamProfile.DefaultReturnMonths); returnDate != "" {
		facts = append(facts, teamHistoryFact("Próximo retorno", formatTeamHistoryDate(returnDate), ""))
	}
	item := []app.UI{app.Div().Class("team-appliance-history__item-heading").Body(app.Strong().Class("team-appliance-history__service-name").Body(app.Text(title)), app.Span().Class("team-appliance-history__status team-appliance-history__status--"+statusTone).Body(app.Text(statusLabel))), app.Div().Class("team-appliance-history__facts").Body(facts...)}
	if reason := firstNonEmptyBudget(portalText(row["motivo_cancelamento"]), markerValue(teamServiceCancelReasonMarker, observations)); reason != "" {
		item = append(item, app.P().Class("team-appliance-history__note").Body(app.Strong().Body(app.Text("Motivo do cancelamento: ")), app.Text(reason)))
	}
	if problem := portalText(row["problema"]); problem != "" {
		item = append(item, app.P().Class("team-appliance-history__note").Body(app.Strong().Body(app.Text("Problema relatado: ")), app.Text(problem)))
	}
	for _, detail := range []struct{ label, value string }{{"Diagnóstico", portalText(row["diagnostico"])}, {"Solução", portalText(row["solucao"])}, {"Peças utilizadas", portalText(row["pecas_utilizadas"])}} {
		if detail.value != "" {
			item = append(item, app.P().Class("team-appliance-history__note").Body(app.Strong().Body(app.Text(detail.label+": ")), app.Text(detail.value)))
		}
	}
	details := formatTeamHistoryDisplay(observations)
	if details.Payment != "" || details.Warranty != "" {
		meta := make([]app.UI, 0, 2)
		if details.Payment != "" {
			meta = append(meta, app.Span().Body(app.Strong().Body(app.Text("Pagamento: ")), app.Text(details.Payment)))
		}
		if details.Warranty != "" {
			meta = append(meta, app.Span().Body(app.Strong().Body(app.Text("Garantia: ")), app.Text(details.Warranty)))
		}
		item = append(item, app.Div().Class("team-appliance-history__meta").Body(meta...))
	}
	if details.Notes != "" {
		item = append(item, app.P().Class("team-appliance-history__note").Body(app.Text(details.Notes)))
	}
	if checklistRows := teamServiceChecklistRows(row); len(checklistRows) > 0 {
		item = append(item, teamHistoryChecklistPanel(checklistRows))
	}
	if len(details.Contacts) > 0 {
		contacts := make([]app.UI, 0, len(details.Contacts))
		for _, contact := range details.Contacts {
			contacts = append(contacts, app.Li().Body(app.Text(contact)))
		}
		item = append(item, app.Details().Class("team-appliance-history__contacts").Body(app.Summary().Body(app.Text("Ver contatos registrados ("+itoaPhoto(len(details.Contacts))+")")), app.Ul().Body(contacts...)))
	}
	service := row
	if id := portalText(row["id"]); id != "" {
		for _, current := range p.teamServices {
			if portalText(current["id"]) == id {
				service = current
				break
			}
		}
	}
	item = append(item, app.Button().Class("auth-link team-appliance-history__action").Type("button").OnClick(p.openTeamServiceDetail(service)).Body(app.Text("Abrir atendimento e OS completa")))
	if id := portalText(row["id"]); id != "" {
		item = append(item, app.Button().Class("auth-link team-appliance-history__delete").Type("button").OnClick(p.askTeamCustomerAction(teamDeleteHistoryServiceAction+id)).Body(app.Text("Excluir ordem de serviço")))
	}
	return app.Li().Class("team-appliance-history__item").Body(item...)
}

func teamHistoryChecklistPanel(rows []teamServiceChecklistRow) app.UI {
	fields := make([]app.UI, 0, len(rows))
	for _, row := range rows {
		fields = append(fields, app.Div().Class("team-appliance-history__checklist-row").Body(
			app.Span().Body(app.Text(row.Label)),
			app.Strong().Body(app.Text(row.Value)),
		))
	}
	return app.Section().Class("team-appliance-history__checklist").Body(
		app.Strong().Class("team-appliance-history__checklist-title").Body(app.Text("Checklist técnico")),
		app.Div().Class("team-appliance-history__checklist-grid").Body(fields...),
	)
}

func teamHistorySortDate(row map[string]any) string {
	return firstNonEmptyBudget(portalText(row["data_conclusao"]), portalText(row["data_inicio"]), portalText(row["data_agendamento"]), portalText(row["data_solicitacao"]), portalText(row["data"]))
}

func teamHistoryBudgetItemsSummary(items []domain.BudgetItem) string {
	labels := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Description) != "" {
			labels = append(labels, item.Description)
		}
	}
	return strings.Join(labels, " • ")
}

func enrichServiceHistory(service map[string]any, histories []map[string]any) map[string]any {
	if len(histories) == 0 {
		return service
	}
	enriched := make(map[string]any, len(service)+4)
	for key, value := range service {
		enriched[key] = value
	}
	for _, history := range histories {
		for _, key := range []string{"data", "problema", "diagnostico", "solucao", "pecas_utilizadas", "valor"} {
			if portalText(enriched[key]) == "" && portalText(history[key]) != "" {
				enriched[key] = history[key]
			}
		}
		if notes := portalText(history["observacoes"]); notes != "" {
			current := portalText(enriched["observacoes"])
			if current == "" {
				enriched["observacoes"] = notes
			} else if !strings.Contains(current, notes) {
				enriched["observacoes"] = current + " | " + notes
			}
		}
	}
	return enriched
}

func teamHistoryFact(label, value, className string) app.UI {
	class := "team-appliance-history__fact"
	if className != "" {
		class += " " + className
	}
	return app.Div().Class(class).Body(app.Span().Body(app.Text(label)), app.Strong().Body(app.Text(value)))
}

func formatTeamHistoryDate(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 10 {
		value = value[:10]
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return value
	}
	return parsed.Format("02/01/2006")
}

func teamHistoryReturnDate(row map[string]any, defaultMonths int) string {
	if portalText(row["status"]) != "CONCLUIDO" {
		return ""
	}
	observations := portalText(row["observacoes"])
	if match := teamHistoryReturnMarker.FindStringSubmatch(observations); len(match) > 1 {
		if _, err := time.Parse("2006-01-02", match[1]); err == nil {
			return match[1]
		}
	}
	base := portalText(row["data_conclusao"])
	if base == "" {
		base = portalText(row["data_agendamento"])
	}
	if len(base) > 10 {
		base = base[:10]
	}
	parsed, err := time.Parse("2006-01-02", base)
	if err != nil {
		return ""
	}
	if defaultMonths <= 0 {
		defaultMonths = 3
	}
	return domain.AddMonthsClamped(parsed, defaultMonths).Format("2006-01-02")
}

func (p *serviceCatalogPage) teamHistoryDialog() app.UI {
	f := p.teamHistoryForm
	if f == nil {
		return app.Div()
	}
	field := func(label string, target *string, kind string) app.UI {
		return app.Label().Class("auth-field").Body(app.Text(label), app.Input().Type(kind).Value(*target).OnChange(p.ValueTo(target)))
	}
	catalog := teamHistoryServiceCatalog(p.teamProfile)
	options := make([]app.UI, 0, len(catalog))
	for _, entry := range catalog {
		options = append(options, app.Option().Value(entry.Name).Body(app.Text(entry.Name)))
	}
	if len(options) == 0 {
		options = append(options, app.Option().Value(f.Type).Body(app.Text(f.Type)))
	}
	cycleToggle := func(ctx app.Context, event app.Event) { f.UpdateCycle = event.Get("target").Get("checked").Bool() }
	return app.Div().Class("auth-backdrop team-long-form-backdrop").Body(app.Div().Class("auth-dialog team-history-dialog").Body(app.H2().Class("auth-dialog__title").Body(app.Text("Registrar histórico anterior")), app.P().Class("auth-dialog__intro").Body(app.Text("O serviço será registrado como concluído e não enviará WhatsApp ao cliente.")), app.Label().Class("auth-field").Body(app.Text("Tipo de serviço"), app.Select().Attr("value", f.Type).OnChange(p.selectTeamHistoryType).Body(options...)), field("Data em que foi realizado", &f.Date, "date"), field("Valor (R$)", &f.Price, "number"), app.Label().Class("auth-field").Body(app.Text("Observações (opcional)"), app.Textarea().Rows(2).Text(f.Notes).OnChange(p.ValueTo(&f.Notes))), app.Label().Class("auth-field").Body(app.Input().Type("checkbox").Checked(f.UpdateCycle).OnChange(cycleToggle), app.Text("Iniciar ciclo de retorno a partir desta data")), formErrorNotice(f.Message), app.Div().Class("portal-budget__actions").Body(app.Button().Class("auth-link").Type("button").Disabled(f.Saving).OnClick(func(ctx app.Context, event app.Event) { event.PreventDefault(); p.teamHistoryForm = nil }).Body(app.Text("Cancelar")), app.Button().Class("auth-submit").Type("button").Disabled(f.Saving).OnClick(p.saveTeamHistory).Body(app.Text("Registrar histórico")))))
}

func (p *serviceCatalogPage) selectTeamHistoryType(ctx app.Context, event app.Event) {
	updateTeamHistoryServiceType(p.teamHistoryForm, p.teamProfile, event.Get("target").Get("value").String())
	ctx.Update()
}

