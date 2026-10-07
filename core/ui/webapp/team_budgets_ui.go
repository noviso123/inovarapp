package webapp

import (
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/domain"
)

func detailedTeamBudgetStatus(status domain.BudgetStatus) (string, string) {
	switch status {
	case domain.BudgetApproved:
		return "Aprovado", "complete"
	case domain.BudgetDeclined:
		return "Recusado", "cancelled"
	case domain.BudgetCancelled:
		return "Cancelado", "cancelled"
	default:
		return "Pendente", "pending"
	}
}

func teamBudgetExpired(validUntil string, now time.Time) bool {
	value := strings.TrimSpace(validUntil)
	if value == "" {
		return false
	}
	date, err := time.ParseInLocation("2006-01-02", value, now.Location())
	if err != nil {
		return false
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return date.Before(today)
}

func teamBudgetCardReference(id string) string {
	if len(id) > 8 {
		id = id[:8]
	}
	return strings.ToUpper(id)
}

func budgetPDFDownloadFilename(contentDisposition, fallback string) string {
	_, params, err := mime.ParseMediaType(contentDisposition)
	if err != nil {
		return fallback
	}
	name := filepath.Base(strings.TrimSpace(params["filename"]))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return fallback
	}
	return name
}

func (p *serviceCatalogPage) filteredTeamBudgets() []domain.BudgetEstimate {
	result := make([]domain.BudgetEstimate, 0, len(p.teamBudgets))
	query := strings.TrimSpace(p.teamBudgetSearch)
	for _, budget := range p.teamBudgets {
		if p.teamBudgetFilter != "" && string(budget.Status) != p.teamBudgetFilter {
			continue
		}
		if !searchMatches(query, budget.ClientName, budget.ClientPhone, budget.ApplianceDescription) {
			continue
		}
		result = append(result, budget)
	}
	return result
}

type teamBudgetMonthlySummary struct {
	Received float64
	Issued   float64
	Clients  []teamBudgetClientReceipt
}

type teamBudgetClientReceipt struct {
	Name  string
	Value float64
}

func summarizeTeamBudgetMonth(budgets []domain.BudgetEstimate, month string) teamBudgetMonthlySummary {
	result := teamBudgetMonthlySummary{}
	clientIndex := make(map[string]int)
	for _, budget := range budgets {
		if strings.HasPrefix(budget.Date, month) {
			result.Issued += budget.FinalValue
		}
		if budget.PaidAt == nil || !strings.HasPrefix(*budget.PaidAt, month) {
			continue
		}
		// Match OrcamentosTab: the monthly ledger uses finalValue, not the
		// optional valorRecebido metadata, and preserves first-seen client order.
		result.Received += budget.FinalValue
		index, exists := clientIndex[budget.ClientName]
		if !exists {
			index = len(result.Clients)
			clientIndex[budget.ClientName] = index
			result.Clients = append(result.Clients, teamBudgetClientReceipt{Name: budget.ClientName})
		}
		result.Clients[index].Value += budget.FinalValue
	}
	return result
}

func (p *serviceCatalogPage) detailedTeamBudgetsPanel() app.UI {
	budgets := p.teamBudgets
	filtered := p.filteredTeamBudgets()
	pending, approved, cancelled, approvedValue := 0, 0, 0, 0.0
	month := p.teamBudgetMonth
	if month == "" {
		month = time.Now().Format("2006-01")
		p.teamBudgetMonth = month
	}
	receivable := 0.0
	for _, budget := range budgets {
		if budget.Status == domain.BudgetPending {
			pending++
		}
		if budget.Status == domain.BudgetApproved {
			approved++
			approvedValue += budget.FinalValue
			if budget.Paid == nil || !*budget.Paid {
				receivable += budget.FinalValue
			}
		}
		if budget.Status == domain.BudgetCancelled {
			cancelled++
		}
	}
	monthly := summarizeTeamBudgetMonth(budgets, month)
	cards := []app.UI{
		app.Article().Class("portal-service").Body(app.P().Class("catalog__eyebrow").Body(app.Text("Propostas emitidas")), app.Strong().Class("catalog__title").Body(app.Text(fmt.Sprintf("%d propostas", len(budgets))))),
		app.Article().Class("portal-service").Body(app.P().Class("catalog__eyebrow").Body(app.Text("Aguardando aprovação")), app.Strong().Class("catalog__title").Body(app.Text(fmt.Sprintf("%d pendentes", pending)))),
		app.Article().Class("portal-service").Body(app.P().Class("catalog__eyebrow").Body(app.Text("Orçamentos aprovados")), app.Strong().Class("catalog__title").Body(app.Text(fmt.Sprintf("%d • R$ %.2f", approved, approvedValue)))),
		app.Article().Class("portal-service").Body(app.P().Class("catalog__eyebrow").Body(app.Text("Faturamento aprovado")), app.Strong().Class("catalog__title").Body(app.Text(fmt.Sprintf("R$ %.2f", approvedValue)))),
	}
	finance := []app.UI{
		app.Label().Class("auth-field").Body(app.Text("Financeiro do mês"), app.Input().Type("month").Value(month).OnChange(p.ValueTo(&p.teamBudgetMonth))),
		app.Article().Class("portal-service").Body(app.P().Class("catalog__eyebrow").Body(app.Text("Recebido no mês")), app.Strong().Body(app.Text(fmt.Sprintf("R$ %.2f", monthly.Received)))),
		app.Article().Class("portal-service").Body(app.P().Class("catalog__eyebrow").Body(app.Text("A receber (aprovados)")), app.Strong().Body(app.Text(fmt.Sprintf("R$ %.2f", receivable)))),
		app.Article().Class("portal-service").Body(app.P().Class("catalog__eyebrow").Body(app.Text("Emitido no mês")), app.Strong().Body(app.Text(fmt.Sprintf("R$ %.2f", monthly.Issued)))),
	}
	for _, customer := range monthly.Clients {
		finance = append(finance, app.P().Class("portal-section__intro").Body(app.Text(fmt.Sprintf("%s • R$ %.2f recebidos", customer.Name, customer.Value))))
	}
	filters := []app.UI{
		app.Input().Type("search").Class("team-search-input").Placeholder("Buscar por cliente, aparelho ou telefone...").Value(p.teamBudgetSearch).OnInput(p.ValueTo(&p.teamBudgetSearch)),
		app.Select().Attr("value", p.teamBudgetFilter).OnChange(p.ValueTo(&p.teamBudgetFilter)).Body(
			app.Option().Value("").Body(app.Text(fmt.Sprintf("Todos (%d)", len(budgets)))),
			app.Option().Value(string(domain.BudgetPending)).Body(app.Text(fmt.Sprintf("Pendentes (%d)", pending))),
			app.Option().Value(string(domain.BudgetApproved)).Body(app.Text(fmt.Sprintf("Aprovados (%d)", approved))),
			app.Option().Value(string(domain.BudgetDeclined)).Body(app.Text("Recusados")),
			app.Option().Value(string(domain.BudgetCancelled)).Body(app.Text(fmt.Sprintf("Cancelados (%d)", cancelled))),
		),
	}
	list := []app.UI{}
	if p.teamBudgetMessage != "" {
		list = append(list, app.P().Class("portal-section__intro").Body(app.Text(p.teamBudgetMessage)))
	}
	if p.teamBudgetWhatsAppURL != "" {
		list = append(list, app.A().Class("auth-link").Href(p.teamBudgetWhatsAppURL).Target("_blank").Rel("noopener noreferrer").Body(app.Text("Abrir WhatsApp com proposta e PDF")))
	}
	if len(filtered) == 0 {
		list = append(list, app.P().Class("portal-section__empty").Body(app.Text("Nenhum orçamento encontrado com os filtros selecionados.")))
	}
	for _, budget := range filtered {
		list = append(list, p.detailedTeamBudgetCard(budget))
	}
	return app.Section().ID("team-budgets").Class("portal-section").Body(
		app.Div().Class("team-budget-page-heading").Body(
			app.Div().Body(
				app.P().Class("catalog__eyebrow").Body(app.Text("PROPOSTAS & ORÇAMENTOS • INOVAR REFRIGERAÇÃO")),
				app.H2().Class("catalog__title").Body(app.Text("Propostas comerciais")),
				app.P().Class("portal-section__intro").Body(app.Text("Crie, envie, acompanhe e converta propostas em ordens de serviço.")),
			),
			app.Button().Class("auth-submit team-budget-create-action").Type("button").OnClick(p.openTeamBudgetForm()).Body(app.Text("＋ Novo orçamento")),
		),
		app.Div().Class("service-grid team-budget-kpis").Body(cards...),
		app.Div().Class("service-grid team-budget-finance").Body(finance...),
		app.Div().Class("portal-budget__actions").Body(filters...),
		app.Div().Class("service-grid team-budget-list").Body(list...),
	)
}

func (p *serviceCatalogPage) detailedTeamBudgetCard(budget domain.BudgetEstimate) app.UI {
	label, tone := detailedTeamBudgetStatus(budget.Status)
	date, valid := portalDate(budget.Date), portalDate(budget.ValidUntil)
	if budget.Status == domain.BudgetPending && teamBudgetExpired(budget.ValidUntil, time.Now()) {
		label, tone = "Proposta vencida", "cancelled"
	}
	lines := []app.UI{
		app.Div().Class("portal-service__heading").Body(app.Strong().Body(app.Text(budget.ClientName)), app.Span().Class("portal-service__status portal-service__status--"+tone).Body(app.Text(label)), app.Span().Class("portal-service__date").Body(app.Text("#"+teamBudgetCardReference(budget.ID)))),
		app.P().Class("portal-service__date").Body(app.Text(fmt.Sprintf("%s • WhatsApp: %s • %d item(ns) • Emitido: %s • Validade: %s", firstNonEmptyBudget(budget.ApplianceDescription, "Ar-Condicionado"), budget.ClientPhone, len(budget.Items), date, valid))),
	}
	if budget.Status == domain.BudgetPending && teamBudgetExpired(budget.ValidUntil, time.Now()) {
		lines = append(lines, app.P().Class("portal-section__intro").Body(app.Text("Esta proposta venceu. Atualize a validade ou emita um novo orçamento antes de continuar.")))
	}
	lines = append(lines,
		app.P().Class("portal-service__date").Body(app.Text(fmt.Sprintf("Subtotal: R$ %.2f", budget.TotalValue))),
	)
	if budget.Discount > 0 {
		lines = append(lines, app.P().Class("portal-service__date").Body(app.Text(fmt.Sprintf("Desconto: − R$ %.2f", budget.Discount))))
	}
	lines = append(lines,
		app.P().Class("portal-budget__total").Body(app.Text(fmt.Sprintf("Valor total: R$ %.2f", budget.FinalValue))),
	)
	for _, item := range budget.Items {
		lines = append(lines, app.P().Class("portal-budget__item").Body(app.Text(fmt.Sprintf("%s • %.2f × R$ %.2f = R$ %.2f", item.Description, item.Quantity, item.UnitPrice, item.TotalPrice))))
	}
	if strings.TrimSpace(budget.PaymentConditions) != "" {
		lines = append(lines, app.P().Class("portal-service__date").Body(app.Text("Condições de pagamento: "+budget.PaymentConditions)))
	}
	if strings.TrimSpace(budget.ExecutionTime) != "" {
		lines = append(lines, app.P().Class("portal-service__date").Body(app.Text("Prazo de execução: "+budget.ExecutionTime)))
	}
	if strings.TrimSpace(budget.WarrantyTerms) != "" {
		lines = append(lines, app.P().Class("portal-service__date").Body(app.Text("Garantia: "+budget.WarrantyTerms)))
	}
	if strings.TrimSpace(budget.Notes) != "" {
		lines = append(lines, app.P().Class("portal-service__date").Body(app.Text("Observações: "+budget.Notes)))
	}
	if budget.Signature != nil && *budget.Signature != "" {
		signed := "Assinatura digital registrada"
		if budget.SignedAt != nil {
			signed += " em " + portalDateTime(*budget.SignedAt)
		}
		lines = append(lines, app.P().Class("portal-section__intro").Body(app.Text(signed)))
	}
	if budget.Paid != nil && *budget.Paid {
		lines = append(lines, app.P().Class("portal-section__intro").Body(app.Text("Recebido")))
	}
	if budget.CancelledAt != nil && strings.TrimSpace(*budget.CancelledAt) != "" {
		lines = append(lines, app.P().Class("portal-section__intro").Body(app.Text("Cancelado em "+portalDateTime(*budget.CancelledAt))))
	}
	actions := []app.UI{}
	if budget.Status == domain.BudgetPending {
		actions = append(actions,
			app.Button().Class("auth-submit").Type("button").Disabled(p.teamBudgetBusy).OnClick(p.mutateTeamBudget(budget, "STATUS", "APROVAR")).Body(app.Text("✓ Aprovar")),
			app.Button().Class("auth-link").Type("button").Disabled(p.teamBudgetBusy).OnClick(p.confirmTeamBudget(budget, "RECUSAR")).Body(app.Text("✕ Recusar")),
		)
	}
	if budget.Status == domain.BudgetApproved && (budget.Paid == nil || !*budget.Paid) {
		actions = append(actions, app.Button().Class("auth-submit").Type("button").Disabled(p.teamBudgetBusy).OnClick(p.confirmTeamBudget(budget, "RECEBIDO")).Body(app.Text("💰 Recebido")))
	}
	if budget.Status == domain.BudgetApproved {
		actions = append(actions, app.Button().Class("auth-submit").Type("button").Disabled(p.teamBudgetBusy).OnClick(p.openTeamBudgetSchedule(budget)).Body(app.Text("Agendar Atendimento")))
	}
	if budget.ClientPhone != "" && budget.Status != domain.BudgetCancelled {
		actions = append(actions, app.Button().Class("auth-link").Type("button").OnClick(p.sendTeamBudgetWhatsApp(budget)).Body(app.Text("Enviar no WhatsApp")))
	}
	actions = append(actions, app.Button().Class("auth-link team-budget-reprint").Type("button").Disabled(p.teamBudgetBusy).OnClick(p.downloadTeamBudgetPDF(budget)).Body(app.Text("Reimprimir orçamento (PDF)")))
	if budget.Status == domain.BudgetPending || budget.Status == domain.BudgetApproved {
		actions = append(actions, app.Button().Class("auth-link").Type("button").OnClick(p.openEditTeamBudget(budget)).Body(app.Text("Editar")))
	}
	if budget.Status == domain.BudgetApproved && (budget.ServiceID == nil || *budget.ServiceID == "") {
		actions = append(actions, app.Button().Class("auth-link").Type("button").Disabled(budget.ApplianceID == nil || *budget.ApplianceID == "").OnClick(p.startBudgetService(budget)).Body(app.Text("Iniciar ordem de serviço")))
	}
	canCancel := budget.Status == domain.BudgetPending || budget.Status == domain.BudgetApproved
	if budget.Paid != nil && *budget.Paid || budget.ServiceID != nil && strings.TrimSpace(*budget.ServiceID) != "" {
		canCancel = false
	}
	if canCancel {
		actions = append(actions, app.Button().Class("auth-link team-budget-cancel-action").Type("button").Disabled(p.teamBudgetBusy).OnClick(p.confirmTeamBudget(budget, "CANCELAR")).Body(app.Text("Cancelar orçamento")))
	}
	actions = append(actions, app.Button().Class("auth-link team-budget-delete-action").Type("button").Disabled(p.teamBudgetBusy).OnClick(p.confirmTeamBudget(budget, "DELETE")).Body(app.Text("Excluir orçamento")))
	lines = append(lines, app.Div().Class("portal-budget__actions team-budget-card-actions").Body(actions...))
	return app.Article().Class("portal-service").Body(lines...)
}

func (p *serviceCatalogPage) downloadTeamBudgetPDF(budget domain.BudgetEstimate) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		if p.teamBudgetBusy || p.session == nil {
			return
		}
		pageURL := app.Window().URL()
		if pageURL == nil || pageURL.Scheme == "" || pageURL.Host == "" {
			p.teamBudgetMessage = "Servidor indisponível."
			ctx.Update()
			return
		}
		p.teamBudgetBusy, p.teamBudgetMessage = true, "Gerando PDF da proposta..."
		endpoint, token := apiBaseURL()+"/api/orcamento-pdf", p.session.AccessToken
		go func() {
			err := requestBudgetPDF(ctx, endpoint, token, budget.ID)
			p.teamBudgetBusy = false
			if err != nil {
				p.teamBudgetMessage = "Não foi possível gerar o PDF. Verifique a conexão e tente novamente."
			} else {
				p.teamBudgetMessage = "A proposta foi baixada em PDF."
			}
			ctx.Update()
		}()
	}
}

func (p *serviceCatalogPage) confirmTeamBudget(budget domain.BudgetEstimate, action string) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		p.teamBudgetConfirm = action + ":" + budget.ID
		ctx.Update()
	}
}

func (p *serviceCatalogPage) teamBudgetConfirmDialog() app.UI {
	parts := strings.SplitN(p.teamBudgetConfirm, ":", 2)
	if len(parts) != 2 {
		return app.Div()
	}
	budgetID := parts[1]
	budget := domain.BudgetEstimate{ID: budgetID}
	for _, candidate := range p.teamBudgets {
		if candidate.ID == budgetID {
			budget = candidate
			break
		}
	}
	message := teamBudgetConfirmationMessage(parts[0], budget)
	action, status := teamBudgetConfirmationMutation(parts[0])
	return app.Div().Class("auth-backdrop team-budget-confirm-backdrop").Body(app.Div().Class("auth-dialog team-budget-confirm-dialog").Role("dialog").Attr("aria-modal", "true").Attr("aria-label", "Confirmar ação").Body(
		app.H2().Class("auth-dialog__title").Body(app.Text("Confirmar ação")), app.P().Class("auth-dialog__intro team-budget-confirm-message").Body(app.Text(message)),
		app.Div().Class("portal-budget__actions team-budget-confirm-actions").Body(app.Button().Class("auth-link team-budget-confirm-cancel").Type("button").OnClick(func(ctx app.Context, event app.Event) { event.PreventDefault(); p.teamBudgetConfirm = ""; ctx.Update() }).Body(app.Text("Voltar")), app.Button().Class(teamBudgetConfirmButtonClass(parts[0])).Type("button").Disabled(p.teamBudgetBusy).OnClick(p.mutateTeamBudget(budget, action, status)).Body(app.Text(teamBudgetConfirmButtonLabel(parts[0])))),
	))
}

func teamBudgetConfirmationMessage(action string, budget domain.BudgetEstimate) string {
	switch action {
	case "RECUSAR":
		return "Recusar este orçamento?"
	case "RECEBIDO":
		return fmt.Sprintf("Confirmar recebimento de R$ %.2f de %s?", budget.FinalValue, budget.ClientName)
	case "DELETE":
		return "Excluir este orçamento? Esta ação não pode ser desfeita."
	case "CANCELAR":
		return "Cancelar esta proposta? Ela será mantida no histórico com o status Cancelado."
	default:
		return "Confirma esta ação no orçamento?"
	}
}

func teamBudgetConfirmationMutation(action string) (string, string) {
	if action == "RECUSAR" {
		return "STATUS", "RECUSAR"
	}
	return action, ""
}

func teamBudgetConfirmButtonLabel(action string) string {
	switch action {
	case "CANCELAR":
		return "Confirmar cancelamento"
	case "DELETE":
		return "Excluir orçamento"
	default:
		return "Confirmar"
	}
}

func teamBudgetConfirmButtonClass(action string) string {
	if action == "DELETE" {
		return "auth-submit team-budget-confirm-submit team-budget-confirm-submit--danger team-budget-delete-confirm"
	}
	class := "auth-submit team-budget-confirm-submit"
	if action == "CANCELAR" || action == "RECUSAR" {
		class += " team-budget-confirm-submit--danger"
	}
	return class
}

func (p *serviceCatalogPage) mutateTeamBudget(budget domain.BudgetEstimate, action, status string) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		if action == "RECEBIDO" {
			if _, err := uuid.Parse(budget.ID); err != nil {
				p.teamBudgetMessage = "Orçamento local — salve-o no banco primeiro."
				ctx.Update()
				return
			}
		}
		if p.teamBudgetBusy || p.session == nil {
			return
		}
		pageURL := app.Window().URL()
		if pageURL == nil {
			return
		}
		p.teamBudgetBusy, p.teamBudgetMessage = true, ""
		endpoint := apiBaseURL() + "/api/orcamento-equipe?id=" + budget.ID
		method := http.MethodPatch
		if action == "DELETE" {
			method = http.MethodDelete
		}
		payload := map[string]any{"id": budget.ID, "acao": action, "status": status}
		if action == "RECEBIDO" {
			payload["orcamento"] = budget
		}
		token := p.session.AccessToken
		go func() {
			var err error
			if method == http.MethodPatch {
				_, err = sendTeamJSONResult(ctx, endpoint, token, method, payload)
			} else {
				err = sendTeamEmptyRequest(ctx, endpoint, token, method)
			}
			p.teamBudgetBusy, p.teamBudgetConfirm = false, ""
			if err != nil {
				if p.deferOfflineTeamMutation(endpoint, method, payload, err) {
					p.teamBudgetMessage = "Alteração salva neste dispositivo e será sincronizada quando a conexão voltar."
					p.teamLoadedFor = ""
					p.retryPendingOfflineMutations(ctx)
				} else {
					p.teamBudgetMessage = "Não foi possível atualizar este orçamento. Verifique a conexão e suas permissões."
				}
			} else {
				p.teamBudgetMessage = teamBudgetMutationSuccessMessage(action, budget)
				p.teamLoadedFor = ""
				p.loadTeamOperations(ctx)
			}
			ctx.Update()
		}()
	}
}

func teamBudgetMutationSuccessMessage(action string, budget domain.BudgetEstimate) string {
	if action == "RECEBIDO" {
		return fmt.Sprintf("💰 R$ %.2f marcado como recebido!", budget.FinalValue)
	}
	if action == "CANCELAR" {
		return fmt.Sprintf("Orçamento #%s cancelado e mantido no histórico.", firstNonEmptyBudget(budget.Number, teamBudgetCardReference(budget.ID)))
	}
	if action == "DELETE" {
		return "Orçamento excluído."
	}
	return ""
}

func (p *serviceCatalogPage) startBudgetService(budget domain.BudgetEstimate) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		if budget.ApplianceID == nil || p.session == nil || budget.ServiceID != nil && *budget.ServiceID != "" || len(budget.Items) == 0 {
			return
		}
		customer := findTeamCustomer(p.teamCustomers, budget.ClientID)
		if customer == nil {
			p.teamBudgetMessage = "Cliente do orçamento não está mais disponível."
			ctx.Update()
			return
		}
		var appliance map[string]any
		for _, raw := range applianceMaps(customer) {
			if portalText(raw["id"]) == *budget.ApplianceID {
				appliance = raw
				break
			}
		}
		if appliance == nil {
			p.teamBudgetMessage = "Aparelho vinculado ao orçamento não está mais disponível."
			ctx.Update()
			return
		}
		serviceType := "OUTRO"
		serviceDescription := "Atendimento conforme orçamento"
		if len(budget.Items) > 0 {
			serviceDescription = budget.Items[0].Description
			serviceType = string(supabase.MapServiceTypeToSupabase(serviceDescription))
		}
		body := map[string]any{"cliente_id": budget.ClientID, "aparelho_id": *budget.ApplianceID, "tipo": serviceType, "descricao": fmt.Sprintf("Orçamento #%s • %s", budget.Number, serviceDescription), "status": "PENDENTE", "valor": budget.FinalValue, "data_agendamento": dashboardCivilToday(time.Now()).Format("2006-01-02"), "observacoes": budget.Notes}
		pageURL := app.Window().URL()
		if pageURL == nil {
			return
		}
		endpoint := apiBaseURL() + "/api/servicos"
		token := p.session.AccessToken
		go func() {
			created, err := sendTeamJSONResult(ctx, endpoint, token, http.MethodPost, body)
			if err == nil {
				serviceID := portalText(created["id"])
				if serviceID == "" {
					err = fmt.Errorf("resposta da OS sem identificador")
				} else {
					linkEndpoint := apiBaseURL() + "/api/orcamento-equipe?id=" + budget.ID
					_, err = sendTeamJSONResult(ctx, linkEndpoint, token, http.MethodPatch, map[string]any{"id": budget.ID, "acao": "LINK_SERVICE", "service_id": serviceID})
				}
			}
			if err != nil {
				p.teamBudgetMessage = "A OS foi criada, mas não foi possível vinculá-la ao orçamento. Confirme antes de tentar novamente."
			} else {
				p.teamBudgetMessage = "Ordem de serviço criada a partir do orçamento."
				p.teamLoadedFor = ""
				p.loadTeamOperations(ctx)
			}
			ctx.Update()
		}()
	}
}

func applianceMaps(customer map[string]any) []map[string]any {
	values, _ := customer["appliances"].([]any)
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if row, ok := value.(map[string]any); ok {
			result = append(result, row)
		}
	}
	return result
}

func (p *serviceCatalogPage) sendTeamBudgetWhatsApp(budget domain.BudgetEstimate) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		if p.session == nil || budget.ClientPhone == "" || p.teamBudgetBusy {
			return
		}
		pageURL := app.Window().URL()
		if pageURL == nil {
			return
		}
		endpoint := apiBaseURL() + "/api/orcamento-whatsapp"
		token := p.session.AccessToken
		p.teamBudgetBusy, p.teamBudgetWhatsAppURL = true, ""
		p.teamBudgetMessage = "Gerando o PDF e registrando a proposta na fila do WhatsApp..."
		go func() {
			result, err := sendTeamJSONResult(ctx, endpoint, token, http.MethodPost, map[string]any{"budget_id": budget.ID})
			p.teamBudgetBusy = false
			if err != nil {
				p.teamBudgetMessage = "Não foi possível preparar o PDF para envio. Verifique a conexão e tente novamente."
			} else if queued, _ := result["enfileirado"].(bool); queued {
				p.teamBudgetMessage = "Proposta e PDF registrados na fila do WhatsApp. O status do envio pode ser acompanhado na seção Mensagens WhatsApp."
				p.registerLocalNotification(ctx, "Proposta na fila do WhatsApp", fmt.Sprintf("O orçamento nº %s foi registrado para envio a %s.", budget.Number, budget.ClientName))
			} else {
				p.teamBudgetWhatsAppURL = portalText(result["whatsapp_url"])
				p.teamBudgetMessage = "Não foi possível enviar automaticamente. Você pode enviar a proposta e o PDF pelo WhatsApp."
			}
			ctx.Update()
		}()
	}
}

func sendTeamEmptyRequest(ctx app.Context, endpoint, token string, method string) error {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", response.StatusCode)
	}
	return nil
}

func firstNonEmptyBudget(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
