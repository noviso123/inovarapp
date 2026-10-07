package webapp

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/domain"
)

type teamBudgetForm struct {
	ID, Number, ClientID, ClientName, ClientPhone, ApplianceID, Date, ValidUntil string
	Equipment, Description, ExecutionTime                                        string
	Payment, Warranty, Notes                                                     string
	Discount                                                                     string
	Items                                                                        []domain.BudgetItem
	Signature                                                                    *string
	SignedAt                                                                     *string
	Paid                                                                         *bool
	PaidAt                                                                       *string
	AmountReceived                                                               *float64
	ServiceID                                                                    *string
	Message                                                                      string
	QuickItemSearch                                                              string
	ClientSearch                                                                 string
	WizardStep                                                                   string
	Wizard                                                                       bool
	Saving                                                                       bool
}

type teamBudgetPrompt struct {
	Budget   domain.BudgetEstimate
	ClientID string
}

func (p *serviceCatalogPage) openTeamBudgetForm() app.EventHandler {
	return p.openNewTeamBudgetForm("")
}

func (p *serviceCatalogPage) openTeamBudgetFormForService(service map[string]any) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		customerID := portalText(service["cliente_id"])
		if budget, ok := findPendingTeamBudgetForClient(p.teamBudgets, customerID); ok {
			p.teamBudgetPrompt = &teamBudgetPrompt{Budget: budget, ClientID: customerID}
			ctx.Update()
			return
		}
		p.openNewTeamBudgetForm(customerID)(ctx, event)
		form := p.teamBudgetForm
		if form == nil {
			return
		}
		prefillTeamBudgetFromService(form, service, p.teamCustomers)
		ctx.Update()
	}
}

func prefillTeamBudgetFromService(form *teamBudgetForm, service map[string]any, customers []map[string]any) {
	if form == nil || service == nil {
		return
	}
	if serviceID := portalText(service["id"]); serviceID != "" {
		form.ServiceID = optionalBudgetString(serviceID)
	}
	serviceName := teamRequestServiceName(service)
	if serviceName != "" {
		if len(form.Items) == 0 {
			form.Items = append(form.Items, domain.BudgetItem{ID: uuid.NewString(), Quantity: 1, Category: domain.BudgetItemService})
		}
		if strings.TrimSpace(form.Items[0].Description) == "" {
			form.Items[0].Description = serviceName
		}
	}
	customerID, applianceID := portalText(service["cliente_id"]), portalText(service["aparelho_id"])
	if customer := findTeamCustomer(customers, customerID); customer != nil {
		form.ClientID = customerID
		form.ClientName, form.ClientPhone = portalText(customer["nome"]), portalText(customer["whatsapp"])
		setTeamBudgetAppliance(form, nil)
		for _, appliance := range applianceMaps(customer) {
			if portalText(appliance["id"]) == applianceID {
				setTeamBudgetAppliance(form, appliance)
				return
			}
		}
		return
	}
	linkedCustomer, _ := service["customers"].(map[string]any)
	form.ClientID = customerID
	form.ClientName = portalText(linkedCustomer["nome"])
	form.ClientPhone = portalText(linkedCustomer["whatsapp"])
	if appliance, ok := service["air_conditioners"].(map[string]any); ok {
		setTeamBudgetAppliance(form, appliance)
	}
}

func findPendingTeamBudgetForClient(budgets []domain.BudgetEstimate, clientID string) (domain.BudgetEstimate, bool) {
	if clientID == "" {
		return domain.BudgetEstimate{}, false
	}
	for _, budget := range budgets {
		if budget.ClientID == clientID && budget.Status == domain.BudgetPending {
			return budget, true
		}
	}
	return domain.BudgetEstimate{}, false
}

func resolveTeamBudgetCustomer(customers []map[string]any, clientID, name string, allowCreate bool) (customer map[string]any, resolvedID string, create bool) {
	if clientID != "" {
		if customer = findTeamCustomer(customers, clientID); customer != nil {
			return customer, clientID, false
		}
		if !allowCreate {
			return nil, clientID, false
		}
		if uuid.Validate(clientID) == nil {
			return nil, clientID, true
		}
	}
	name = strings.TrimSpace(name)
	if name != "" {
		for _, candidate := range customers {
			if teamBudgetCustomerNameMatch(portalText(candidate["nome"]), name) {
				return candidate, portalText(candidate["id"]), false
			}
		}
	}
	if !allowCreate || name == "" {
		return nil, "", false
	}
	return nil, uuid.NewString(), true
}

func teamBudgetCustomerNameMatch(left, right string) bool {
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}

func newTeamBudgetCustomerPayload(id, name, phone string) map[string]any {
	return map[string]any{"id": id, "nome": strings.TrimSpace(name), "whatsapp": strings.TrimSpace(phone), "ativo": true, "appliances": []any{}}
}

func (p *serviceCatalogPage) openNewTeamBudgetForm(customerID string) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		today := dashboardCivilToday(time.Now()).Format("2006-01-02")
		valid := dashboardCivilToday(time.Now()).AddDate(0, 0, 7).Format("2006-01-02")
		form := &teamBudgetForm{Date: today, ValidUntil: valid, ExecutionTime: "2 a 3 horas", Payment: "À vista no PIX ou cartão a consultar o valor à parte com taxa", Warranty: "90 dias para mão de obra / 1 ano para instalação", Discount: "0", Items: newTeamBudgetItems(), Wizard: true, WizardStep: "client-choice"}
		p.teamCustomerReturnToBudget = false
		customer := findTeamCustomer(p.teamCustomers, customerID)
		if customer != nil {
			p.setBudgetClient(form, customer)
			form.WizardStep = "items"
		} else if customerID != "" {
			form.ClientID = customerID
			form.WizardStep = "items"
		}
		p.teamBudgetPrompt = nil
		p.teamBudgetForm = form
		ctx.Update()
	}
}

func (p *serviceCatalogPage) teamBudgetWizardDialog(f *teamBudgetForm) app.UI {
	content := []app.UI{app.P().Class("portal-section__intro team-budget-wizard-intro").Body(app.Text("Crie uma proposta completa em poucos passos. Os valores são definidos por você."))}
	switch f.WizardStep {
	case "client-choice":
		content = append(content,
			app.H3().Class("team-budget-wizard-heading").Body(app.Text("O cliente já está cadastrado?")),
			app.Button().Class("team-budget-wizard-option").Type("button").OnClick(func(ctx app.Context, event app.Event) {
				event.PreventDefault()
				f.WizardStep = "select-client"
				ctx.Update()
			}).Body(app.Strong().Body(app.Text("Sim, já é cliente")), app.Span().Body(app.Text("Buscar na lista e preencher os dados automaticamente"))),
			app.Button().Class("team-budget-wizard-option").Type("button").OnClick(func(ctx app.Context, event app.Event) {
				event.PreventDefault()
				f.WizardStep = "new-client"
				p.teamCustomerForm, p.teamCustomerNotice = newTeamCustomerForm(), ""
				p.teamCustomerReturnToBudget = true
				ctx.Update()
			}).Body(app.Strong().Body(app.Text("Não, cadastrar cliente")), app.Span().Body(app.Text("Abrir o cadastro completo e voltar ao orçamento ao salvar"))),
		)
	case "select-client":
		query := strings.TrimSpace(f.ClientSearch)
		content = append(content,
			app.H3().Class("team-budget-wizard-heading").Body(app.Text("Buscar cliente cadastrado")),
			app.Label().Class("auth-field team-budget-field").Body(app.Text("Nome ou telefone"), app.Input().Type("search").Value(f.ClientSearch).Placeholder("Digite para buscar").OnInput(p.ValueTo(&f.ClientSearch))),
		)
		matches := 0
		if query == "" {
			content = append(content, app.P().Class("portal-section__empty").Body(app.Text("Digite o nome ou telefone para localizar o cliente.")))
		} else {
			for _, customer := range p.teamCustomers {
				name, phone := portalText(customer["nome"]), portalText(customer["whatsapp"])
				if !searchMatches(query, name, phone, portalText(customer["email"]), portalText(customer["bairro"])) {
					continue
				}
				matches++
				selectedCustomer := customer
				content = append(content, app.Button().Class("team-budget-wizard-customer").Type("button").OnClick(func(ctx app.Context, event app.Event) {
					event.PreventDefault()
					p.setBudgetClient(f, selectedCustomer)
					f.ClientSearch, f.WizardStep = "", "items"
					ctx.Update()
				}).Body(app.Strong().Body(app.Text(name)), app.Span().Body(app.Text(phone))))
			}
		}
		if query != "" && matches == 0 {
			content = append(content, app.P().Class("portal-section__empty").Body(app.Text("Nenhum cliente encontrado. Cadastre um novo cliente para continuar.")))
		}
		content = append(content, app.Button().Class("auth-submit").Type("button").OnClick(func(ctx app.Context, event app.Event) {
			event.PreventDefault()
			f.WizardStep = "new-client"
			p.teamCustomerForm, p.teamCustomerNotice = newTeamCustomerForm(), ""
			p.teamCustomerReturnToBudget = true
			ctx.Update()
		}).Body(app.Text("Cadastrar novo cliente")))
	case "new-client":
		content = append(content, app.P().Class("portal-section__empty").Body(app.Text("Conclua o cadastro do cliente para continuar com os itens do orçamento.")))
	}
	back := app.Button().Class("auth-link").Type("button").OnClick(func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		if f.WizardStep == "select-client" {
			f.WizardStep = "client-choice"
		} else {
			p.teamBudgetForm = nil
		}
		ctx.Update()
	}).Body(app.Text(func() string {
		if f.WizardStep == "select-client" {
			return "Voltar"
		}
		return "Cancelar"
	}()))
	content = append(content, app.Div().Class("portal-budget__actions team-budget-wizard-actions").Body(back))
	return app.Div().Class("auth-backdrop team-budget-backdrop").Body(app.Div().Class("auth-dialog team-budget-dialog team-budget-wizard").Role("dialog").Body(
		app.H2().Class("auth-dialog__title").Body(app.Text("Novo orçamento")),
		app.Div().Class("team-budget-wizard-content").Body(content...),
	))
}

func (p *serviceCatalogPage) resumeBudgetAfterCustomer(form *teamCustomerForm, customerID, applianceID string) {
	if !p.teamCustomerReturnToBudget || p.teamBudgetForm == nil || form == nil {
		return
	}
	capacity, _ := strconv.Atoi(form.Capacity)
	customer, appliance := offlineCustomerPayload(customerID, applianceID, form, capacity)
	p.setBudgetClient(p.teamBudgetForm, customer)
	setTeamBudgetAppliance(p.teamBudgetForm, appliance)
	p.teamBudgetForm.WizardStep = "items"
	p.teamCustomerReturnToBudget = false
}

func (p *serviceCatalogPage) teamBudgetPromptDialog() app.UI {
	prompt := p.teamBudgetPrompt
	if prompt == nil {
		return app.Div()
	}
	number := firstNonEmptyBudget(prompt.Budget.Number, "em aberto")
	return app.Div().Class("auth-backdrop").Style("z-index", "95").Body(app.Div().Class("auth-dialog team-budget-prompt-dialog").Role("dialog").Body(
		app.H2().Class("auth-dialog__title").Body(app.Text("Orçamento pendente encontrado")),
		app.P().Class("auth-dialog__intro").Body(app.Text(fmt.Sprintf("Este cliente já tem o orçamento %s (R$ %.2f).", number, prompt.Budget.FinalValue))),
		app.P().Class("auth-dialog__intro").Body(app.Text("Edite a proposta existente ou escolha gerar um novo orçamento.")),
		app.Div().Class("portal-budget__actions").Body(
			app.Button().Class("auth-submit").Type("button").OnClick(func(ctx app.Context, event app.Event) {
				event.PreventDefault()
				budget := prompt.Budget
				p.teamBudgetPrompt = nil
				p.openEditTeamBudget(budget)(ctx, event)
			}).Body(app.Text("Editar orçamento existente")),
			app.Button().Class("auth-link").Type("button").OnClick(p.openNewTeamBudgetForm(prompt.ClientID)).Body(app.Text("Gerar um novo orçamento")),
			app.Button().Class("auth-link").Type("button").OnClick(func(ctx app.Context, event app.Event) { event.PreventDefault(); p.teamBudgetPrompt = nil; ctx.Update() }).Body(app.Text("Fechar")),
		),
	))
}

func (p *serviceCatalogPage) openEditTeamBudget(budget domain.BudgetEstimate) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		form := &teamBudgetForm{
			ID: budget.ID, Number: budget.Number, ClientID: budget.ClientID, ClientName: budget.ClientName, ClientPhone: budget.ClientPhone,
			Date: budget.Date, ValidUntil: budget.ValidUntil,
			Equipment: budget.EquipmentName, Description: budget.ApplianceDescription,
			ExecutionTime: budget.ExecutionTime, Payment: budget.PaymentConditions,
			Warranty: budget.WarrantyTerms, Notes: budget.Notes,
			Discount: fmt.Sprintf("%.2f", budget.Discount), Items: append([]domain.BudgetItem(nil), budget.Items...),
			Signature: cloneBudgetString(budget.Signature), SignedAt: cloneBudgetString(budget.SignedAt),
			Paid: cloneBudgetBool(budget.Paid), PaidAt: cloneBudgetString(budget.PaidAt),
			AmountReceived: cloneBudgetFloat(budget.AmountReceived), ServiceID: cloneBudgetString(budget.ServiceID),
		}
		if budget.ApplianceID != nil {
			form.ApplianceID = *budget.ApplianceID
		}
		p.teamBudgetForm = form
		ctx.Update()
	}
}

func (p *serviceCatalogPage) setBudgetClient(form *teamBudgetForm, customer map[string]any) {
	if customer == nil {
		form.ClientID, form.ClientName, form.ClientPhone = "", "", ""
		setTeamBudgetAppliance(form, nil)
		return
	}
	form.ClientID = portalText(customer["id"])
	form.ClientName = portalText(customer["nome"])
	form.ClientPhone = portalText(customer["whatsapp"])
	appliances := applianceMaps(customer)
	if len(appliances) == 0 {
		setTeamBudgetAppliance(form, nil)
		return
	}
	setTeamBudgetAppliance(form, appliances[0])
}

func setTeamBudgetAppliance(form *teamBudgetForm, appliance map[string]any) {
	if form == nil {
		return
	}
	if appliance == nil {
		form.Equipment, form.Description, form.ApplianceID = "", "", ""
		return
	}
	form.ApplianceID = portalText(appliance["id"])
	form.Equipment = strings.TrimSpace(portalText(appliance["marca"]) + " " + portalText(appliance["modelo"]))
	form.Description = strings.TrimSpace(form.Equipment + " " + portalText(appliance["btus"]) + " BTUs (" + portalText(appliance["ambiente"]) + ")")
}

func maxTeamBudgetValidityDate(issueDate string) string {
	issued, err := time.Parse("2006-01-02", strings.TrimSpace(issueDate))
	if err != nil {
		return ""
	}
	return issued.AddDate(0, 0, 7).Format("2006-01-02")
}

func teamBudgetValidityExceedsSevenDays(issueDate, validUntil string) bool {
	issued, issueErr := time.Parse("2006-01-02", strings.TrimSpace(issueDate))
	valid, validErr := time.Parse("2006-01-02", strings.TrimSpace(validUntil))
	if issueErr != nil || validErr != nil || valid.Before(issued) {
		return true
	}
	return valid.After(issued.AddDate(0, 0, 7))
}

func (p *serviceCatalogPage) saveTeamBudget(ctx app.Context, event app.Event) {
	event.PreventDefault()
	f := p.teamBudgetForm
	if f == nil || f.Saving || p.session == nil {
		return
	}
	if teamBudgetValidityExceedsSevenDays(f.Date, f.ValidUntil) {
		legacyValidityUnchanged := false
		if f.ID != "" {
			for _, existing := range p.teamBudgets {
				if existing.ID == f.ID && existing.Date == f.Date && existing.ValidUntil == f.ValidUntil {
					legacyValidityUnchanged = true
					break
				}
			}
		}
		if !legacyValidityUnchanged {
			f.Message = "A validade do orçamento deve ficar entre a data de emissão e, no máximo, 7 dias depois."
			return
		}
	}
	selectedClientID := f.ClientID
	name, phone := budgetContactValues(f, findTeamCustomer(p.teamCustomers, f.ClientID))
	if strings.TrimSpace(name) == "" || strings.TrimSpace(phone) == "" {
		f.Message = "Informe o nome completo e o telefone do cliente para emitir o orçamento."
		return
	}
	customer, resolvedClientID, createCustomer := resolveTeamBudgetCustomer(p.teamCustomers, f.ClientID, name, f.ID == "")
	if resolvedClientID == "" {
		f.Message = "Selecione um cliente cadastrado ou informe os dados do novo cliente."
		return
	}
	if _, err := uuid.Parse(resolvedClientID); err != nil {
		f.Message = "Selecione um cliente cadastrado válido para emitir o orçamento."
		return
	}
	if createCustomer {
		f.ClientID = resolvedClientID // keep the same identity if a network retry is needed
	}
	if len(f.Items) == 0 {
		f.Message = "Adicione pelo menos um item ao orçamento."
		return
	}
	discount, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(f.Discount), ",", "."), 64)
	if err != nil || discount < 0 {
		f.Message = "Informe um desconto válido."
		return
	}
	if customer == nil && !createCustomer {
		f.Message = "O cliente selecionado não está mais disponível."
		return
	}
	if customer != nil && lenTeamCustomerAppliances(customer) > 0 && f.ApplianceID == "" {
		f.Message = "Selecione um aparelho para vincular ao orçamento."
		return
	}
	if customer != nil && !isValidBudgetApplianceSelection(customer, f.ApplianceID) {
		f.Message = "O aparelho selecionado não pertence ao cliente. Atualize os dados e tente novamente."
		return
	}
	if selectedClientID != "" && customer != nil && lenTeamCustomerAppliances(customer) == 0 && strings.TrimSpace(f.Description) == "" {
		f.Message = "Informe a descrição do equipamento para este cliente."
		return
	}
	pageURL := app.Window().URL()
	if pageURL == nil || pageURL.Scheme == "" || pageURL.Host == "" {
		f.Message = "Servidor indisponível."
		return
	}
	address := ""
	if customer != nil {
		address = strings.Join(nonEmptyAgendaParts(portalText(customer["endereco"]), portalText(customer["bairro"]), portalText(customer["cidade"])), ", ")
	}
	var document string
	if customer != nil {
		for _, key := range []string{"cpf_cnpj", "documento"} {
			if document = portalText(customer[key]); document != "" {
				break
			}
		}
	}
	for i := range f.Items {
		if strings.TrimSpace(f.Items[i].Description) == "" || f.Items[i].Quantity <= 0 || f.Items[i].UnitPrice <= 0 {
			f.Message = "Informe a descrição, quantidade e o preço individual de cada item do orçamento."
			return
		}
		f.Items[i].TotalPrice = domain.BudgetLineTotal(f.Items[i].Quantity, f.Items[i].UnitPrice)
	}
	final := domain.CalculateBudgetTotals(f.Items, discount)
	budget := domain.BudgetEstimate{
		ID: f.ID, Number: f.Number,
		ClientID: resolvedClientID, ClientName: name, ClientPhone: phone, ClientAddress: address,
		ClientDocument: document, EquipmentName: f.Equipment, ApplianceDescription: f.Description,
		Date: f.Date, ValidUntil: f.ValidUntil, Items: f.Items, TotalValue: final.Subtotal,
		Discount: discount, FinalValue: final.FinalValue, PaymentConditions: f.Payment,
		ExecutionTime: f.ExecutionTime, WarrantyTerms: f.Warranty, Status: domain.BudgetPending,
		Notes: f.Notes, ApplianceID: optionalBudgetString(f.ApplianceID), Signature: f.Signature,
		SignedAt: f.SignedAt, Paid: f.Paid, PaidAt: f.PaidAt, AmountReceived: f.AmountReceived,
		ServiceID: f.ServiceID,
	}
	if budget.ID != "" {
		for _, existing := range p.teamBudgets {
			if existing.ID == budget.ID {
				budget.Status = existing.Status
				break
			}
		}
	}
	base, token := apiBaseURL(), p.session.AccessToken
	isNewBudget := budget.ID == ""
	if isNewBudget {
		budget.ID = uuid.NewString()
	}
	var newCustomer map[string]any
	if createCustomer {
		newCustomer = newTeamBudgetCustomerPayload(budget.ClientID, name, phone)
	}
	f.Saving, f.Message = true, "Salvando orçamento..."
	go func() {
		var err error
		var created domain.BudgetEstimate
		if p.offlineMode && isNewBudget {
			err = enqueueTeamBudgetOffline(p.caller.UserID, p.caller.Role, budget, newCustomer)
			if err == nil {
				if newCustomer != nil {
					p.teamCustomers = mergePendingOfflineCustomers(p.teamCustomers, []offlineTeamCustomer{{CustomerID: budget.ClientID, Customer: newCustomer}})
				}
				p.teamBudgets = mergePendingOfflineBudgets(p.teamBudgets, []domain.BudgetEstimate{budget})
				p.teamDashboard = buildTeamDashboardSummary(p.teamCustomers, p.teamServices, p.teamAppointments, p.teamBudgets, p.teamReturnHistory, p.teamProfile.DefaultReturnMonths, dashboardCivilToday(time.Now()))
				p.teamBudgetForm = nil
				p.teamBudgetMessage = "Orçamento salvo neste dispositivo. Será sincronizado quando a conexão voltar."
				p.offlineMode = true
				p.retryPendingOfflineMutations(ctx)
				ctx.Update()
				return
			}
		} else if createCustomer {
			var result map[string]any
			customerPayload := cloneAnyMap(newCustomer)
			delete(customerPayload, "appliances")
			result, err = sendTeamJSONResult(ctx, base+"/api/clientes", token, http.MethodPost, customerPayload)
			if err == nil {
				if createdID := portalText(result["id"]); createdID != budget.ClientID {
					err = fmt.Errorf("o servidor retornou outro identificador de cliente")
				} else {
					p.teamCustomers = append(p.teamCustomers, cloneAnyMap(newCustomer))
				}
			}
		}
		if err == nil && isNewBudget {
			var result map[string]any
			result, err = sendTeamJSONResult(ctx, base+"/api/orcamentos", token, http.MethodPost, budget)
			if err == nil {
				created, err = decodeCreatedTeamBudget(result)
			}
		} else if err == nil {
			_, err = sendTeamJSONResult(ctx, base+"/api/orcamento-equipe?id="+budget.ID, token, http.MethodPatch, map[string]any{"id": budget.ID, "acao": "EDIT", "orcamento": budget})
		}
		if err != nil {
			if isOfflineNetworkError(err) && isNewBudget {
				if queueErr := enqueueTeamBudgetOffline(p.caller.UserID, p.caller.Role, budget, newCustomer); queueErr == nil {
					p.teamBudgets = mergePendingOfflineBudgets(p.teamBudgets, []domain.BudgetEstimate{budget})
					p.teamDashboard = buildTeamDashboardSummary(p.teamCustomers, p.teamServices, p.teamAppointments, p.teamBudgets, p.teamReturnHistory, p.teamProfile.DefaultReturnMonths, dashboardCivilToday(time.Now()))
					p.teamBudgetForm = nil
					p.teamBudgetMessage = "Orçamento salvo neste dispositivo. Será sincronizado quando a conexão voltar."
					p.offlineMode = true
					p.teamLoadedFor = ""
					p.retryPendingOfflineMutations(ctx)
					ctx.Update()
					return
				}
			}
			f.Saving, f.Message = false, "Não foi possível salvar o orçamento. Verifique conexão e permissões."
			ctx.Update()
			return
		}
		p.teamBudgetForm = nil
		p.teamBudgetWhatsAppURL = ""
		p.teamBudgetMessage = "Orçamento salvo."
		if !isNewBudget {
			p.teamBudgetMessage = "Orçamento atualizado."
			p.registerLocalNotification(ctx, "Orçamento atualizado", "A proposta foi sincronizada no banco.")
		} else {
			p.registerLocalNotification(ctx, "Orçamento gerado", fmt.Sprintf("Proposta nº %s criada para %s.", created.Number, created.ClientName))
		}
		if isNewBudget && created.ClientPhone != "" {
			delivery, deliveryErr := sendTeamJSONResult(ctx, base+"/api/orcamento-whatsapp", token, http.MethodPost, map[string]any{"budget_id": created.ID})
			if deliveryErr == nil {
				if queued, _ := delivery["enfileirado"].(bool); queued {
					p.teamBudgetMessage = fmt.Sprintf("Orçamento #%s salvo. Proposta e PDF registrados na fila do WhatsApp para %s.", created.Number, created.ClientName)
					p.registerLocalNotification(ctx, "Proposta na fila do WhatsApp", fmt.Sprintf("O orçamento nº %s foi registrado para envio com PDF a %s.", created.Number, created.ClientName))
				} else {
					p.teamBudgetWhatsAppURL = portalText(delivery["whatsapp_url"])
					p.teamBudgetMessage = fmt.Sprintf("Orçamento #%s salvo. Não foi possível enviar automaticamente; você pode abrir o WhatsApp para enviar a proposta.", created.Number)
				}
			} else {
				p.teamBudgetMessage = fmt.Sprintf("Orçamento #%s salvo. Não foi possível enviar automaticamente; use ‘Enviar no WhatsApp’ na proposta.", created.Number)
			}
		}
		p.teamLoadedFor = ""
		p.loadTeamOperations(ctx)
		ctx.Update()
	}()
}

func decodeCreatedTeamBudget(result map[string]any) (domain.BudgetEstimate, error) {
	encoded, err := json.Marshal(result)
	if err != nil {
		return domain.BudgetEstimate{}, err
	}
	var budget domain.BudgetEstimate
	if err := json.Unmarshal(encoded, &budget); err != nil {
		return domain.BudgetEstimate{}, err
	}
	if budget.ID == "" {
		return domain.BudgetEstimate{}, fmt.Errorf("resposta do orçamento sem identificador")
	}
	return budget, nil
}

func cloneBudgetString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneBudgetBool(value *bool) *bool {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneBudgetFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func optionalBudgetString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	copy := value
	return &copy
}

func budgetContactValues(form *teamBudgetForm, customer map[string]any) (string, string) {
	name, phone := strings.TrimSpace(form.ClientName), strings.TrimSpace(form.ClientPhone)
	if name == "" {
		name = portalText(customer["nome"])
	}
	if phone == "" {
		phone = portalText(customer["whatsapp"])
	}
	return name, phone
}

func findTeamCustomer(customers []map[string]any, id string) map[string]any {
	for _, customer := range customers {
		if portalText(customer["id"]) == id {
			return customer
		}
	}
	return nil
}

func lenTeamCustomerAppliances(customer map[string]any) int {
	appliances, _ := customer["appliances"].([]any)
	return len(appliances)
}

func (p *serviceCatalogPage) teamBudgetDialog() app.UI {
	f := p.teamBudgetForm
	if f == nil {
		return app.Div()
	}
	if f.Wizard && f.WizardStep != "items" {
		return p.teamBudgetWizardDialog(f)
	}
	clientOptions := []app.UI{app.Option().Value("").Body(app.Text("Cliente Avulso / Novo"))}
	for _, customer := range p.teamCustomers {
		id, name := portalText(customer["id"]), portalText(customer["nome"])
		clientOptions = append(clientOptions, app.Option().Value(id).Body(app.Text(name)))
	}
	selected := findTeamCustomer(p.teamCustomers, f.ClientID)
	selectClient := app.Select().Attr("value", f.ClientID).Disabled(f.ID != "").OnChange(func(ctx app.Context, event app.Event) {
		f.ClientID = event.Get("target").Get("value").String()
		p.setBudgetClient(f, findTeamCustomer(p.teamCustomers, f.ClientID))
		ctx.Update()
	}).Body(clientOptions...)
	clientLabel := "Cliente"
	if f.ID != "" {
		clientLabel = "Cliente (não pode ser alterado após a emissão)"
	}
	fields := []app.UI{}
	if f.Wizard {
		fields = append(fields, app.Div().Class("team-budget-wizard-selected-client").Body(
			app.Div().Body(app.Span().Body(app.Text("CLIENTE DO ORÇAMENTO")), app.Strong().Body(app.Text(firstNonEmptyBudget(f.ClientName, "Cliente selecionado"))), app.P().Body(app.Text(firstNonEmptyBudget(f.ClientPhone, "Telefone não informado")))),
			app.Button().Class("auth-link").Type("button").OnClick(func(ctx app.Context, event app.Event) {
				event.PreventDefault()
				f.WizardStep, f.ClientSearch = "select-client", ""
				ctx.Update()
			}).Body(app.Text("Trocar cliente")),
		))
	} else {
		fields = append(fields,
			app.Div().Class("team-budget-form-section").Body(app.Span().Class("team-budget-step").Body(app.Text("1")), app.Div().Body(app.H3().Body(app.Text("Cliente e equipamento")), app.P().Body(app.Text("Escolha um cadastro para preencher os dados automaticamente.")))),
			app.Label().Class("auth-field team-budget-field").Body(app.Text(clientLabel), selectClient),
			app.Div().Class("team-budget-form-grid").Body(
				app.Label().Class("auth-field team-budget-field").Body(app.Text("Nome no orçamento"), app.Input().Value(f.ClientName).OnChange(p.ValueTo(&f.ClientName))),
				app.Label().Class("auth-field team-budget-field").Body(app.Text("WhatsApp / telefone para contato"), app.Input().Type("tel").Value(f.ClientPhone).OnChange(p.ValueTo(&f.ClientPhone))),
			),
			app.Div().Class("team-budget-form-grid").Body(
				app.Label().Class("auth-field team-budget-field").Body(app.Text("Equipamento"), app.Input().Value(f.Equipment).Disabled(f.ID != "").OnChange(p.ValueTo(&f.Equipment))),
				app.Label().Class("auth-field team-budget-field").Body(app.Text("Descrição do equipamento"), app.Input().Value(f.Description).Disabled(f.ID != "").OnChange(p.ValueTo(&f.Description))),
			),
		)
	}
	if selected != nil {
		appliances, _ := selected["appliances"].([]any)
		if len(appliances) > 0 {
			options := []app.UI{}
			for _, raw := range appliances {
				appliance, _ := raw.(map[string]any)
				if appliance == nil {
					continue
				}
				id := portalText(appliance["id"])
				label := strings.TrimSpace(portalText(appliance["marca"]) + " " + portalText(appliance["modelo"]) + " • " + portalText(appliance["btus"]) + " BTUs • " + portalText(appliance["ambiente"]))
				options = append(options, app.Option().Value(id).Body(app.Text(label)))
			}
			fields = append(fields, app.Label().Class("auth-field team-budget-field").Body(app.Text("Aparelho vinculado"), app.Select().Attr("value", f.ApplianceID).Disabled(f.ID != "").OnChange(func(ctx app.Context, event app.Event) {
				f.ApplianceID = event.Get("target").Get("value").String()
				for _, appliance := range applianceMaps(selected) {
					if portalText(appliance["id"]) == f.ApplianceID {
						setTeamBudgetAppliance(f, appliance)
						break
					}
				}
				if f.ApplianceID == "" {
					setTeamBudgetAppliance(f, nil)
				}
				ctx.Update()
			}).Body(options...)))
		}
	}
	fields = append(fields,
		app.Div().Class("team-budget-form-section").Body(app.Span().Class("team-budget-step").Body(app.Text(func() string {
			if f.Wizard {
				return "3"
			}
			return "2"
		}())), app.Div().Body(app.H3().Body(app.Text(func() string {
			if f.Wizard {
				return "Itens e valores"
			}
			return "Detalhes do orçamento"
		}())), app.P().Body(app.Text("Defina os serviços e preços para este cliente.")))),
	)
	if !f.Wizard {
		fields = append(fields, app.Div().Class("team-budget-form-grid").Body(app.Label().Class("auth-field team-budget-field").Body(app.Text("Data de emissão"), app.Input().Type("date").Value(f.Date).OnChange(func(ctx app.Context, event app.Event) {
			f.Date = event.Get("target").Get("value").String()
			if max := maxTeamBudgetValidityDate(f.Date); max != "" && f.ValidUntil > max {
				f.ValidUntil = max
			}
			ctx.Update()
		})), app.Label().Class("auth-field team-budget-field").Body(app.Text("Validade (até 7 dias)"), app.Input().Type("date").Attr("min", f.Date).Attr("max", maxTeamBudgetValidityDate(f.Date)).Value(f.ValidUntil).OnChange(p.ValueTo(&f.ValidUntil)))))
	}
	itemRows := make([]app.UI, 0, len(f.Items))
	quickItems := budgetQuickItems(p.teamProfile)
	quickButtons := make([]app.UI, 0, len(quickItems))
	search := strings.TrimSpace(f.QuickItemSearch)
	for _, quick := range quickItems {
		if !searchMatches(search, quick.Description, string(quick.Category)) {
			continue
		}
		item := quick
		quickButtons = append(quickButtons, app.Button().Class("auth-link").Type("button").OnClick(func(ctx app.Context, event app.Event) {
			event.PreventDefault()
			f.Items = append(f.Items, item)
			if item.Category == domain.BudgetItemService {
				if itemAverage, itemWarranty := budgetQuickItemDefaults(item.Description, p.teamProfile); itemAverage != "" {
					f.ExecutionTime = itemAverage
					f.Warranty = itemWarranty
				}
			}
			ctx.Update()
		}).Body(app.Strong().Body(app.Text("+ "+quick.Description)), app.Span().Body(app.Text("Valor sugerido: R$ "+formatPortalMoney(quick.UnitPrice)))))
	}
	for i := range f.Items {
		index := i
		item := &f.Items[i]
		categoryOptions := []app.UI{
			app.Option().Value("").Body(app.Text("Selecione uma categoria")),
			app.Option().Value(string(domain.BudgetItemService)).Body(app.Text("Serviço")),
			app.Option().Value(string(domain.BudgetItemPart)).Body(app.Text("Peça")),
			app.Option().Value(string(domain.BudgetItemMaterial)).Body(app.Text("Material")),
		}
		lineTotal := domain.BudgetLineTotal(item.Quantity, item.UnitPrice)
		itemRows = append(itemRows, app.Article().Class("portal-service").Body(
			app.Label().Class("auth-field team-budget-field").Body(app.Text("Descrição"), app.Input().Value(item.Description).OnChange(func(ctx app.Context, event app.Event) {
				f.Items[index].Description = event.Get("target").Get("value").String()
				ctx.Update()
			})),
			app.Label().Class("auth-field team-budget-field").Body(app.Text("Categoria"), app.Select().Attr("value", string(item.Category)).OnChange(func(ctx app.Context, event app.Event) {
				f.Items[index].Category = domain.BudgetItemCategory(event.Get("target").Get("value").String())
				ctx.Update()
			}).Body(categoryOptions...)),
			app.Label().Class("auth-field team-budget-field").Body(app.Text("Quantidade"), app.Input().Type("number").Attr("min", "0.01").Attr("step", "0.01").Value(budgetQuantityInputValue(item.Quantity)).Placeholder("Informe a quantidade").OnChange(func(ctx app.Context, event app.Event) {
				f.Items[index].Quantity = positiveFloat(event.Get("target").Get("value").String())
				f.Items[index].TotalPrice = domain.BudgetLineTotal(f.Items[index].Quantity, f.Items[index].UnitPrice)
				ctx.Update()
			})),
			app.Label().Class("auth-field team-budget-field").Body(app.Text("Preço unitário (R$)"), app.Input().Type("number").Attr("min", "0.01").Attr("step", "0.01").Value(budgetUnitPriceInputValue(item.UnitPrice)).Placeholder("Valor sugerido — ajuste se necessário").OnChange(func(ctx app.Context, event app.Event) {
				f.Items[index].UnitPrice = nonNegativeFloat(event.Get("target").Get("value").String())
				f.Items[index].TotalPrice = domain.BudgetLineTotal(f.Items[index].Quantity, f.Items[index].UnitPrice)
				ctx.Update()
			})),
			app.P().Class("portal-section__intro").Body(app.Text(fmt.Sprintf("Total da linha: R$ %.2f", lineTotal))),
			app.Button().Class("auth-link").Type("button").OnClick(func(ctx app.Context, event app.Event) {
				event.PreventDefault()
				f.Items = append(f.Items[:index], f.Items[index+1:]...)
				ctx.Update()
			}).Body(app.Text("Remover item")),
		))
	}
	addItem := app.Button().Class("auth-link").Type("button").OnClick(func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		f.Items = append(f.Items, newTeamBudgetItem())
		ctx.Update()
	}).Body(app.Text("Adicionar item"))
	totals := domain.CalculateBudgetTotals(f.Items, parseBudgetNumber(f.Discount))
	itemsContent := []app.UI{app.H3().Class("team-agenda__heading").Body(app.Text("Itens do orçamento")), app.P().Class("portal-section__intro team-budget-items-hint").Body(app.Text("Toque em um serviço para adicioná-lo com quantidade e valor sugeridos. Você pode ajustar o preço antes de salvar."))}
	if len(itemRows) == 0 {
		itemsContent = append(itemsContent, app.P().Class("portal-section__empty team-budget-items__empty").Body(app.Text("Adicione os serviços e materiais deste orçamento.")))
	}
	itemsContent = append(itemsContent,
		app.Label().Class("auth-field team-budget-field team-budget-quick-search").Body(app.Text("Buscar serviço do catálogo"), app.Input().Type("search").Value(f.QuickItemSearch).Placeholder("Ex.: instalação, limpeza, gás").OnInput(p.ValueTo(&f.QuickItemSearch))),
		app.Div().Class("portal-budget__actions team-budget-quick-items").Body(quickButtons...),
		app.Div().Class("service-grid").Body(itemRows...),
		addItem,
	)
	fields = append(fields,
		app.Div().Class("team-budget-items").Body(itemsContent...),
		app.Label().Class("auth-field team-budget-field").Body(app.Text("Desconto (R$)"), app.Input().Type("number").Attr("min", "0").Attr("step", "0.01").Value(f.Discount).OnChange(p.ValueTo(&f.Discount))),
		app.Div().Class("team-budget-total").Body(app.Span().Body(app.Text("Total do orçamento")), app.Strong().Body(app.Text("R$ "+formatPortalMoney(totals.FinalValue)))),
		app.Details().Class("team-budget-more-details").Body(app.Summary().Body(app.Text("Mais detalhes (prazo, pagamento, garantia e observações)")),
			app.Div().Class("team-budget-form-grid").Body(app.Label().Class("auth-field team-budget-field").Body(app.Text("Data de emissão"), app.Input().Type("date").Value(f.Date).OnChange(func(ctx app.Context, event app.Event) {
				f.Date = event.Get("target").Get("value").String()
				if max := maxTeamBudgetValidityDate(f.Date); max != "" && f.ValidUntil > max {
					f.ValidUntil = max
				}
				ctx.Update()
			})), app.Label().Class("auth-field team-budget-field").Body(app.Text("Validade (até 7 dias)"), app.Input().Type("date").Attr("min", f.Date).Attr("max", maxTeamBudgetValidityDate(f.Date)).Value(f.ValidUntil).OnChange(p.ValueTo(&f.ValidUntil)))),
			app.Label().Class("auth-field team-budget-field").Body(app.Text("Prazo de execução"), app.Input().Value(f.ExecutionTime).OnChange(p.ValueTo(&f.ExecutionTime))),
			app.Label().Class("auth-field team-budget-field").Body(app.Text("Condições de pagamento"), app.Textarea().Rows(2).Text(f.Payment).OnChange(p.ValueTo(&f.Payment))),
			app.Label().Class("auth-field team-budget-field").Body(app.Text("Garantia"), app.Input().Value(f.Warranty).OnChange(p.ValueTo(&f.Warranty))),
			app.Label().Class("auth-field team-budget-field").Body(app.Text("Observações"), app.Textarea().Rows(3).Text(f.Notes).OnChange(p.ValueTo(&f.Notes))),
		),
	)
	formBody := append([]app.UI{}, fields...)
	formBody = append(formBody, formErrorNotice(f.Message))
	formBody = append(formBody, app.Div().Class("portal-budget__actions").Body(app.Button().Class("auth-link").Type("button").Disabled(f.Saving).OnClick(func(ctx app.Context, event app.Event) { event.PreventDefault(); p.teamBudgetForm = nil }).Body(app.Text("Cancelar")), app.Button().Class("auth-submit").Type("submit").Disabled(f.Saving).Body(app.Text("Salvar orçamento"))))
	return app.Div().Class("auth-backdrop team-budget-backdrop").Body(app.Div().Class("auth-dialog team-budget-dialog").Body(
		app.H2().Class("auth-dialog__title").Body(app.Text(func() string {
			if f.ID != "" {
				return "Editar orçamento #" + f.Number
			}
			return "Criar novo orçamento"
		}())),
		app.Form().Class("auth-form team-budget-form").OnSubmit(p.saveTeamBudget).Body(formBody...),
	))
}

func budgetQuickItems(profile domain.TechnicianProfile) []domain.BudgetItem {
	items := make([]domain.BudgetItem, 0, 12)
	for _, entry := range domain.BuildServiceCatalog(&profile) {
		price := entry.Price
		if price <= 0 {
			price = 250
		}
		items = append(items, domain.BudgetItem{ID: uuid.NewString(), Description: entry.Name, Quantity: 1, UnitPrice: price, TotalPrice: price, Category: domain.BudgetItemService})
	}
	return items
}

func newTeamBudgetItems() []domain.BudgetItem {
	return nil
}

func newTeamBudgetItem() domain.BudgetItem {
	return domain.BudgetItem{ID: uuid.NewString()}
}

func budgetUnitPriceInputValue(price float64) string {
	if price <= 0 {
		return ""
	}
	return fmt.Sprintf("%.2f", price)
}

func budgetQuantityInputValue(quantity float64) string {
	if quantity <= 0 {
		return ""
	}
	return fmt.Sprintf("%.2f", quantity)
}

func budgetQuickItemDefaults(description string, profile domain.TechnicianProfile) (string, string) {
	for _, entry := range domain.BuildServiceCatalog(&profile) {
		if entry.Name == description {
			average, warranty := entry.AverageTime, entry.DefaultWarranty
			if entry.Card != nil {
				if average == "" {
					average = entry.Card.AverageTime
				}
				if warranty == "" {
					warranty = entry.Card.DefaultWarranty
				}
			}
			return average, warranty
		}
	}
	return "", ""
}

func parseBudgetNumber(value string) float64 {
	n, _ := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(value), ",", "."), 64)
	return n
}
func positiveFloat(value string) float64 {
	n := parseBudgetNumber(value)
	if math.IsNaN(n) || n <= 0 {
		return 0
	}
	return n
}
func nonNegativeFloat(value string) float64 {
	n := parseBudgetNumber(value)
	if math.IsNaN(n) || n < 0 {
		return 0
	}
	return n
}
