package webapp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/adapter/cep"
	"inovarapp/core/adapter/googlecontacts"
)

type teamCustomerForm struct {
	CustomerID                                             string
	Name, Phone, Address, Neighborhood, City, State, Notes string
	PostalCode, PostalMessage                              string
	PostalGeneration                                       uint64
	PostalSuggestion                                       *cep.Address
	Email, Password                                        string
	Brand, Model, Capacity, ApplianceType, Room            string
	GoogleQuery, GoogleNotice                              string
	GoogleContacts                                         []googlecontacts.Contact
	DeviceContacts                                         []deviceContact
	GoogleSearching                                        bool
	GoogleSearchGeneration                                 uint64
	DeviceContactsBusy                                     bool
	Editing, ApplianceSaved                                bool
	Saving, PostalLoading                                  bool
}

func newTeamCustomerForm() *teamCustomerForm {
	return &teamCustomerForm{Name: "", Brand: "LG", Capacity: "12000", ApplianceType: "Split Hi-Wall", Room: "Sala"}
}

func teamCustomerOption(value, label, current string) app.UI {
	option := app.Option().Value(value)
	if value == current {
		option = option.Selected(true)
	}
	return option.Body(app.Text(label))
}

func (p *serviceCatalogPage) toggleTeamCustomerForm(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.teamCustomerForm != nil && p.teamCustomerForm.Saving {
		return
	}
	if p.teamCustomerForm != nil {
		p.teamCustomerForm = nil
		if p.teamCustomerReturnToBudget && p.teamBudgetForm != nil {
			p.teamBudgetForm.WizardStep = "client-choice"
		}
		p.teamCustomerReturnToBudget = false
		ctx.Update()
		return
	}
	p.teamCustomerForm, p.teamCustomerNotice = newTeamCustomerForm(), ""
	ctx.Update()
}

func (p *serviceCatalogPage) openTeamCustomerForm(customer map[string]any) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		form := newTeamCustomerForm()
		if customer != nil {
			form.CustomerID = portalText(customer["id"])
			form.Editing = true
			form.Name, form.Phone = portalText(customer["nome"]), portalText(customer["whatsapp"])
			form.Address, form.Neighborhood, form.City = portalText(customer["endereco"]), portalText(customer["bairro"]), portalText(customer["cidade"])
			form.State = portalText(customer["estado"])
			form.Notes = portalText(customer["observacoes"])
		}
		p.teamCustomerForm, p.teamCustomerNotice = form, ""
		p.teamCustomerReturnToBudget = false
	}
}

func (p *serviceCatalogPage) teamCustomerDialog() app.UI {
	title := "Cadastro de Cliente"
	editing := p.teamCustomerForm != nil && p.teamCustomerForm.Editing
	if editing {
		title = "Editar Cliente"
	}
	heading := []app.UI{app.H2().Class("auth-dialog__title").Body(app.Text(title))}
	if editing {
		heading = append(heading, app.P().Class("auth-dialog__intro").Body(app.Text("Salva no banco — visível em todas as plataformas")))
	}
	return app.Div().Class("auth-backdrop team-customer-backdrop").Body(app.Div().Class("auth-dialog team-customer-dialog").Body(
		app.Div().Class("auth-dialog__heading").Body(app.Div().Body(heading...), app.Button().Class("auth-close").Type("button").OnClick(p.toggleTeamCustomerForm).Body(app.Text("Fechar"))),
		p.teamCustomerCreateForm(),
	))
}

func (p *serviceCatalogPage) submitTeamCustomer(ctx app.Context, event app.Event) {
	event.PreventDefault()
	f := p.teamCustomerForm
	if f == nil || f.Saving || p.session == nil {
		return
	}
	if strings.TrimSpace(f.Name) == "" || strings.TrimSpace(f.Phone) == "" || !f.Editing && !f.ApplianceSaved && strings.TrimSpace(f.Room) == "" {
		p.teamCustomerNotice = "Preencha nome e telefone do cliente e o ambiente do primeiro aparelho."
		return
	}
	capacity, err := strconv.Atoi(f.Capacity)
	if err != nil {
		p.teamCustomerNotice = "Selecione uma capacidade de aparelho válida."
		return
	}
	pageURL := app.Window().URL()
	if pageURL == nil || pageURL.Scheme == "" || pageURL.Host == "" {
		p.teamCustomerNotice = "Não foi possível conectar ao servidor."
		return
	}
	customerID, applianceID := "", ""
	var offlineCustomer, offlineAppliance map[string]any
	if !f.Editing {
		customerID, applianceID = newOfflineTeamCustomerIDs()
		offlineCustomer, offlineAppliance = offlineCustomerPayload(customerID, applianceID, f, capacity)
	}
	if p.offlineMode && !f.Editing {
		if err := enqueueOfflineTeamCustomer(p.caller.UserID, p.caller.Role, offlineCustomer, offlineAppliance); err != nil {
			p.teamCustomerNotice = "Não foi possível guardar o cadastro offline neste dispositivo."
			ctx.Update()
			return
		}
		p.teamCustomers = append([]map[string]any{offlineCustomer}, p.teamCustomers...)
		p.teamDashboard = buildTeamDashboardSummary(p.teamCustomers, p.teamServices, p.teamAppointments, p.teamBudgets, p.teamReturnHistory, p.teamProfile.DefaultReturnMonths, dashboardCivilToday(time.Now()))
		p.resumeBudgetAfterCustomer(f, customerID, applianceID)
		p.teamCustomerForm = nil
		p.teamActionMessage = "Cliente e aparelho salvos neste dispositivo e serão sincronizados quando a conexão voltar."
		if strings.TrimSpace(f.Email) != "" {
			p.teamActionMessage += " A conta de acesso não foi criada sem conexão."
		}
		p.retryPendingOfflineMutations(ctx)
		ctx.Update()
		return
	}
	baseURL, token := apiBaseURL(), p.session.AccessToken
	form := *f
	f.Saving, p.teamCustomerNotice = true, ""
	go func() {
		if form.Editing {
			payload := map[string]any{
				"id": form.CustomerID, "fields": map[string]any{"nome": strings.TrimSpace(form.Name), "whatsapp": strings.TrimSpace(form.Phone), "endereco": strings.TrimSpace(form.Address), "bairro": strings.TrimSpace(form.Neighborhood), "cidade": strings.TrimSpace(form.City), "estado": strings.TrimSpace(form.State), "observacoes": strings.TrimSpace(form.Notes)},
			}
			_, err := sendTeamJSONResult(ctx, baseURL+"/api/clientes", token, http.MethodPatch, payload)
			f.Saving = false
			if err != nil {
				if p.deferOfflineTeamMutation(baseURL+"/api/clientes", http.MethodPatch, payload, err) {
					p.teamCustomerForm = nil
					p.teamCustomerNotice = "Edição salva neste dispositivo. Será sincronizada quando a conexão voltar."
					p.teamLoadedFor = ""
					p.retryPendingOfflineMutations(ctx)
				} else {
					p.teamCustomerNotice = "Não foi possível atualizar o cadastro do cliente."
				}
			} else {
				p.teamCustomerNotice = "Cadastro do cliente atualizado."
				p.registerLocalNotification(ctx, "Cliente atualizado", "Os dados do cliente foram sincronizados no banco.")
				p.teamCustomerForm, p.teamLoadedFor = nil, ""
				p.loadTeamOperations(ctx)
			}
			ctx.Update()
			return
		}
		if form.CustomerID == "" {
			createPayload := cloneAnyMap(offlineCustomer)
			delete(createPayload, "appliances")
			delete(createPayload, "created_at")
			customer, createErr := sendTeamJSONResult(ctx, baseURL+"/api/clientes", token, http.MethodPost, createPayload)
			if createErr != nil {
				if isOfflineNetworkError(createErr) && enqueueOfflineTeamCustomer(p.caller.UserID, p.caller.Role, offlineCustomer, offlineAppliance) == nil {
					p.teamCustomers = append([]map[string]any{offlineCustomer}, p.teamCustomers...)
					p.teamDashboard = buildTeamDashboardSummary(p.teamCustomers, p.teamServices, p.teamAppointments, p.teamBudgets, p.teamReturnHistory, p.teamProfile.DefaultReturnMonths, dashboardCivilToday(time.Now()))
					p.resumeBudgetAfterCustomer(&form, customerID, applianceID)
					p.teamCustomerForm = nil
					p.teamActionMessage = "Cliente e aparelho salvos neste dispositivo e serão sincronizados quando a conexão voltar."
					if strings.TrimSpace(form.Email) != "" {
						p.teamActionMessage += " A conta de acesso não foi criada sem conexão."
					}
					f.Saving = false
					p.retryPendingOfflineMutations(ctx)
					ctx.Update()
					return
				}
				f.Saving = false
				p.teamCustomerNotice = "Não foi possível cadastrar o cliente. Confira os dados e tente novamente."
				ctx.Update()
				return
			}
			f.CustomerID = portalText(customer["id"])
			if f.CustomerID == "" {
				f.CustomerID = customerID
			}
			if f.CustomerID == "" {
				f.Saving = false
				p.teamCustomerNotice = "Cliente salvo, mas o servidor não retornou o identificador para cadastrar o aparelho."
				ctx.Update()
				return
			}
		}
		var applianceErr error
		if !f.ApplianceSaved {
			_, applianceErr = sendTeamJSONResult(ctx, baseURL+"/api/aparelhos", token, http.MethodPost, map[string]any{
				"id": applianceID, "cliente_id": f.CustomerID, "marca": form.Brand, "modelo": strings.TrimSpace(form.Model),
				"btus": capacity, "tipo": form.ApplianceType, "ambiente": strings.TrimSpace(form.Room),
			})
			if applianceErr == nil {
				f.ApplianceSaved = true
			}
		}
		accountErr := error(nil)
		accountWhatsAppNotice := ""
		if email := strings.TrimSpace(form.Email); email != "" {
			password := strings.TrimSpace(form.Password)
			if password == "" {
				password = "123456"
			}
			var result map[string]any
			result, accountErr = sendTeamJSONResult(ctx, baseURL+"/api/contas", token, http.MethodPost, map[string]any{
				"acao": "do_tecnico", "email": email, "senha": password,
				"nome": strings.TrimSpace(form.Name), "telefone": strings.TrimSpace(form.Phone), "customer_id": f.CustomerID,
			})
			if accountErr == nil && result["error"] != nil {
				accountErr = fmt.Errorf("criação da conta recusada")
			}
			if accountErr == nil {
				accountWhatsAppNotice = portalText(result["whatsapp_aviso"])
			}
		}
		if applianceErr != nil {
			if isOfflineNetworkError(applianceErr) && enqueueOfflineTeamCustomer(p.caller.UserID, p.caller.Role, offlineCustomer, offlineAppliance) == nil {
				p.teamCustomers = append([]map[string]any{offlineCustomer}, p.teamCustomers...)
				p.teamDashboard = buildTeamDashboardSummary(p.teamCustomers, p.teamServices, p.teamAppointments, p.teamBudgets, p.teamReturnHistory, p.teamProfile.DefaultReturnMonths, dashboardCivilToday(time.Now()))
				p.resumeBudgetAfterCustomer(&form, f.CustomerID, applianceID)
				p.teamCustomerForm = nil
				p.teamActionMessage = "Cliente e aparelho salvos neste dispositivo e serão sincronizados quando a conexão voltar."
				if strings.TrimSpace(form.Email) != "" {
					p.teamActionMessage += " A conta de acesso não foi criada sem conexão."
				}
				f.Saving = false
				p.retryPendingOfflineMutations(ctx)
				ctx.Update()
				return
			}
			f.Saving = false
			p.teamCustomerNotice = "Cliente salvo. Não foi possível cadastrar o primeiro aparelho; corrija os dados e salve novamente."
			ctx.Update()
			return
		}
		switch {
		case accountErr != nil:
			p.teamCustomerNotice = "Cliente e aparelho salvos. A conta de acesso não foi criada; tente novamente pelo cadastro de clientes."
		default:
			p.teamCustomerNotice = "Cliente cadastrado com sucesso!"
			if strings.TrimSpace(form.Email) != "" {
				p.teamCustomerNotice += " Conta de acesso criada."
			}
			if accountWhatsAppNotice != "" {
				p.teamCustomerNotice += " " + accountWhatsAppNotice
			}
		}
		p.resumeBudgetAfterCustomer(&form, f.CustomerID, applianceID)
		p.teamCustomerForm, p.teamLoadedFor = nil, ""
		p.loadTeamOperations(ctx)
		ctx.Update()
	}()
}

func offlineCustomerPayload(customerID, applianceID string, form *teamCustomerForm, capacity int) (map[string]any, map[string]any) {
	appliance := map[string]any{
		"id": applianceID, "cliente_id": customerID, "marca": strings.TrimSpace(form.Brand),
		"modelo": strings.TrimSpace(form.Model), "btus": capacity, "tipo": form.ApplianceType,
		"ambiente": strings.TrimSpace(form.Room),
	}
	customer := map[string]any{
		"id": customerID, "nome": strings.TrimSpace(form.Name), "whatsapp": strings.TrimSpace(form.Phone),
		"endereco": strings.TrimSpace(form.Address), "bairro": strings.TrimSpace(form.Neighborhood),
		"cidade": strings.TrimSpace(form.City), "estado": strings.TrimSpace(form.State), "observacoes": strings.TrimSpace(form.Notes),
		"ativo": true, "created_at": time.Now().UTC().Format(time.RFC3339Nano),
		"appliances": []any{appliance},
	}
	return customer, appliance
}

func (p *serviceCatalogPage) setTeamCustomerPostalCode(ctx app.Context, event app.Event) {
	f := p.teamCustomerForm
	if f == nil {
		return
	}
	f.PostalCode = cep.Mask(event.Get("target").Get("value").String())
	f.PostalGeneration++
	f.PostalSuggestion = nil
	f.PostalLoading = false
	digits := cep.Digits(f.PostalCode)
	switch {
	case digits == "":
		f.PostalMessage = ""
	case len(digits) < 8:
		f.PostalMessage = fmt.Sprintf("Digite mais %d dígitos para localizar o endereço.", 8-len(digits))
	default:
		f.PostalMessage = "Buscando endereço pelo CEP..."
	}
	ctx.Update()
	if len(digits) == 8 {
		p.lookupTeamCustomerPostalCode(ctx, event)
	}
}

func (p *serviceCatalogPage) lookupTeamCustomerPostalCode(ctx app.Context, event app.Event) {
	f := p.teamCustomerForm
	if f == nil || len(cep.Digits(f.PostalCode)) != 8 {
		return
	}
	digits := cep.Digits(f.PostalCode)
	f.PostalGeneration++
	generation := f.PostalGeneration
	f.PostalSuggestion = nil
	f.PostalLoading, f.PostalMessage = true, "Consultando CEP..."
	ctx.Update()
	go func() {
		address, err := cep.Lookup(ctx, digits, nil, "")
		if p.teamCustomerForm != f || generation != f.PostalGeneration || cep.Digits(f.PostalCode) != digits {
			return
		}
		f.PostalLoading = false
		if err != nil {
			f.PostalMessage = "Não encontrei esse CEP. Confira os números ou preencha o endereço manualmente."
		} else {
			f.PostalSuggestion = &address
			if address.Street != "" && strings.TrimSpace(f.Address) == "" {
				f.Address = address.Street
			}
			if address.Neighborhood != "" && strings.TrimSpace(f.Neighborhood) == "" {
				f.Neighborhood = address.Neighborhood
			}
			if address.City != "" && strings.TrimSpace(f.City) == "" {
				f.City = address.City
			}
			if address.State != "" && strings.TrimSpace(f.State) == "" {
				f.State = address.State
			}
			f.PostalMessage = "Endereço localizado. Toque na sugestão abaixo para selecionar."
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) selectTeamCustomerPostalSuggestion(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if f := p.teamCustomerForm; f != nil && f.PostalSuggestion != nil {
		address := f.PostalSuggestion
		if address.Street != "" {
			f.Address = address.Street
		}
		if address.Neighborhood != "" {
			f.Neighborhood = address.Neighborhood
		}
		if address.City != "" {
			f.City = address.City
		}
		if address.State != "" {
			f.State = address.State
		}
		f.PostalMessage = "Sugestão de endereço selecionada; os campos continuam editáveis."
		ctx.Update()
	}
}

func (p *serviceCatalogPage) teamCustomerCreateForm() app.UI {
	f := p.teamCustomerForm
	if f == nil {
		return app.Div()
	}
	textField := func(label, placeholder string, target *string, inputType string) app.UI {
		return app.Label().Class("auth-field team-customer-field").Body(app.Text(label), app.Input().Type(inputType).Value(*target).Placeholder(placeholder).OnChange(p.ValueTo(target)))
	}
	capacities := []string{"12000", "7000", "9000", "18000", "24000", "30000", "36000", "48000", "60000"}
	capacityOptions := make([]app.UI, 0, len(capacities))
	for _, value := range capacities {
		capacityOptions = append(capacityOptions, teamCustomerOption(value, value+" BTUs", f.Capacity))
	}
	types := []string{"Split Hi-Wall", "Inverter", "Cassete", "Piso Teto", "Janela", "Multi Split"}
	typeOptions := make([]app.UI, 0, len(types))
	for _, value := range types {
		typeOptions = append(typeOptions, teamCustomerOption(value, value, f.ApplianceType))
	}
	brands := []string{"LG", "Gree", "Midea", "Samsung", "Daikin", "Carrier", "Elgin", "Fujitsu", "Consul", "Electrolux", "Springer", "TCL", "Outra"}
	brandOptions := make([]app.UI, 0, len(brands))
	for _, value := range brands {
		brandOptions = append(brandOptions, teamCustomerOption(value, value, f.Brand))
	}
	var fields []app.UI
	if f.Editing {
		fields = []app.UI{
			p.teamDeviceContactsPicker(f),
			app.Div().Class("team-customer-form-section").Body(app.Span().Class("team-customer-step").Body(app.Text("1")), app.Div().Body(app.H3().Class("team-section__heading-title").Body(app.Text("Dados do cliente")), app.P().Class("team-customer-form-hint").Body(app.Text("Nome, telefone e endereço usados nos atendimentos.")))),
			app.Div().Class("team-customer-form-grid").Body(textField("Nome Completo *", "Nome do cliente", &f.Name, "text"), textField("WhatsApp / Telefone *", "11999998888", &f.Phone, "tel")),
			textField("Endereço (Rua, Número, Apto)", "Rua, número e complemento", &f.Address, "text"),
			app.Div().Class("team-customer-form-grid").Body(textField("Bairro", "Bairro", &f.Neighborhood, "text"), textField("Cidade", "Cidade", &f.City, "text"), textField("Estado", "UF", &f.State, "text")),
			app.Label().Class("auth-field team-customer-field").Body(app.Text("Observações"), app.Textarea().Rows(2).Text(f.Notes).OnChange(p.ValueTo(&f.Notes))),
		}
	} else {
		fields = []app.UI{
			p.teamDeviceContactsPicker(f),
			app.Div().Class("team-customer-form-section").Body(app.Span().Class("team-customer-step").Body(app.Text("1")), app.Div().Body(app.H3().Class("team-section__heading-title").Body(app.Text("Dados do cliente")), app.P().Class("team-customer-form-hint").Body(app.Text("Cadastro compartilhado com agenda, ordens e orçamentos.")))),
			app.Div().Class("team-customer-form-grid").Body(textField("Nome Completo *", "Nome do cliente", &f.Name, "text"), textField("WhatsApp / Telefone *", "11999998888", &f.Phone, "tel")),
			app.Label().Class("auth-field team-customer-field").Body(app.Text("CEP (preenche o endereço automático)"), app.Div().Class("auth-cep-input-wrap").Body(
				app.Input().Type("text").Attr("inputmode", "numeric").Value(f.PostalCode).Placeholder("29060-270").OnInput(p.setTeamCustomerPostalCode),
				func() app.UI {
					if !f.PostalLoading {
						return app.Span()
					}
					return app.Span().Class("auth-cep-spinner").Attr("role", "status").Attr("aria-label", "Buscando CEP").Body(app.Text(" "))
				}(),
			)),
			cepSearchFeedback(f.PostalMessage),
			cepAddressSuggestion(f.PostalSuggestion, p.selectTeamCustomerPostalSuggestion),
			textField("Endereço", "Rua, número e complemento", &f.Address, "text"),
			app.Div().Class("team-customer-form-grid").Body(textField("Bairro", "Bairro", &f.Neighborhood, "text"), textField("Cidade", "Cidade", &f.City, "text"), textField("Estado", "UF", &f.State, "text")),
			app.Label().Class("auth-field team-customer-field").Body(app.Text("Observações"), app.Textarea().Rows(2).Text(f.Notes).Placeholder("Preferências ou informações úteis no atendimento").OnChange(p.ValueTo(&f.Notes))),
		}
		fields = append(fields,
			app.Div().Class("team-customer-form-section").Body(app.Span().Class("team-customer-step").Body(app.Text("2")), app.Div().Body(app.H3().Class("team-section__heading-title").Body(app.Text("Primeiro aparelho")), app.P().Class("team-customer-form-hint").Body(app.Text("Você poderá cadastrar outros aparelhos depois.")))),
			app.Div().Class("team-customer-form-grid").Body(
				app.Label().Class("auth-field team-customer-field").Body(app.Text("Marca"), app.Select().Attr("value", f.Brand).OnChange(p.ValueTo(&f.Brand)).Body(brandOptions...)),
				textField("Modelo (opcional)", "Modelo do aparelho", &f.Model, "text"),
				app.Label().Class("auth-field team-customer-field").Body(app.Text("Capacidade"), app.Select().Attr("value", f.Capacity).OnChange(p.ValueTo(&f.Capacity)).Body(capacityOptions...)),
				app.Label().Class("auth-field team-customer-field").Body(app.Text("Tipo"), app.Select().Attr("value", f.ApplianceType).OnChange(p.ValueTo(&f.ApplianceType)).Body(typeOptions...)),
				textField("Ambiente / Cômodo *", "Sala, Quarto, Consultório", &f.Room, "text"),
			),
			app.Div().Class("team-customer-form-section team-customer-form-section--optional").Body(app.Span().Class("team-customer-step").Body(app.Text("3")), app.Div().Body(app.H3().Class("team-section__heading-title").Body(app.Text("Acesso do cliente ao app")), app.P().Class("team-customer-form-hint").Body(app.Text("Opcional. Deixe em branco para criar somente o cadastro.")))),
			textField("E-mail do Cliente", "cliente@email.com", &f.Email, "email"),
			textField("Senha inicial (opcional)", "Defina uma senha inicial", &f.Password, "password"),
		)
	}
	fields = append(fields, app.Div().Class("portal-budget__actions").Body(
		app.Button().Class("auth-link").Type("button").Disabled(f.Saving).OnClick(p.toggleTeamCustomerForm).Body(app.Text("Cancelar")),
		app.Button().Class("auth-submit").Type("submit").Disabled(f.Saving).Body(app.Text(teamCustomerSaveLabel(f))),
	))
	return app.Form().Class("auth-form team-customer-form").OnSubmit(p.submitTeamCustomer).Body(fields...)
}

func teamCustomerSaveLabel(f *teamCustomerForm) string {
	if f != nil && f.Editing {
		if f.Saving {
			return "Salvando..."
		}
		return "Salvar no Banco"
	}
	if f != nil && f.Saving {
		return "Salvando..."
	}
	return "Salvar cliente"
}

func (p *serviceCatalogPage) connectTeamGoogleContacts(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.session == nil {
		return
	}
	client, err := publicSupabaseClient()
	if err != nil {
		if p.teamCustomerForm != nil {
			p.teamCustomerForm.GoogleNotice = "A conexão com Google não está configurada."
		}
		ctx.Update()
		return
	}
	pageURL := app.Window().URL()
	if pageURL == nil {
		return
	}
	redirect := oauthRedirectURLFor(pageURL, url.Values{"googleContacts": {"1"}}, nativeOAuthAvailable())
	params := url.Values{"access_type": {"offline"}, "prompt": {"consent"}}
	if p.session.User != nil && p.session.User.Email != "" {
		params.Set("login_hint", p.session.User.Email)
	}
	linked := false
	if p.session.User != nil {
		encoded, _ := json.Marshal(p.session.User)
		var user struct {
			Identities []struct {
				Provider string `json:"provider"`
			} `json:"identities"`
		}
		_ = json.Unmarshal(encoded, &user)
		for _, identity := range user.Identities {
			if identity.Provider == "google" {
				linked = true
				break
			}
		}
	}
	const scopes = "email profile https://www.googleapis.com/auth/contacts.readonly"
	var authURL string
	if linked {
		authURL, err = client.SignInWithOAuthURLAndScopes("google", redirect, scopes, params)
	} else {
		authURL, err = client.LinkIdentityOAuthURL(ctx, p.session.AccessToken, "google", redirect, scopes, params)
	}
	if err != nil {
		if p.teamCustomerForm != nil {
			p.teamCustomerForm.GoogleNotice = "Não foi possível iniciar a autorização dos contatos Google."
		}
		ctx.Update()
		return
	}
	p.launchOAuth(ctx, authURL)
}

func (p *serviceCatalogPage) updateTeamGoogleContactQuery(ctx app.Context, event app.Event) {
	f := p.teamCustomerForm
	if f == nil {
		return
	}
	f.GoogleQuery = event.Get("target").Get("value").String()
	f.GoogleSearchGeneration++
	generation := f.GoogleSearchGeneration
	f.GoogleContacts = nil
	query := strings.TrimSpace(f.GoogleQuery)
	if query == "" {
		f.GoogleNotice = ""
		ctx.Update()
		return
	}
	f.GoogleNotice = "Os resultados aparecem automaticamente enquanto você digita."
	ctx.Update()
	p.scheduleTeamGoogleContactSearch(ctx, f, generation, query, 320*time.Millisecond)
}

func (p *serviceCatalogPage) scheduleTeamGoogleContactSearch(ctx app.Context, f *teamCustomerForm, generation uint64, query string, delay time.Duration) {
	go func() {
		if delay > 0 {
			time.Sleep(delay)
		}
		if p.teamCustomerForm != f || generation != f.GoogleSearchGeneration || strings.TrimSpace(f.GoogleQuery) != query || query == "" {
			return
		}
		if f.GoogleSearching {
			return
		}
		if p.session == nil || p.session.ProviderToken == "" {
			f.GoogleNotice = "Conecte os contatos do Google para buscar automaticamente."
			ctx.Update()
			return
		}
		p.runTeamGoogleContactSearch(ctx, f, generation, query)
	}()
}

func (p *serviceCatalogPage) runTeamGoogleContactSearch(ctx app.Context, f *teamCustomerForm, generation uint64, query string) {
	if p.teamCustomerForm != f || generation != f.GoogleSearchGeneration || f.GoogleSearching || p.session == nil || p.session.ProviderToken == "" {
		return
	}
	token, providerToken := p.session.AccessToken, p.session.ProviderToken
	f.GoogleSearching, f.GoogleNotice = true, "Buscando contatos Google..."
	ctx.Update()
	go func() {
		result, err := sendTeamJSONResult(ctx, apiBaseURL()+"/api/google-contacts", token, http.MethodPost, map[string]string{"providerToken": providerToken, "query": query})
		if p.teamCustomerForm != f || p.session == nil || p.session.AccessToken != token {
			return
		}
		f.GoogleSearching = false
		if generation != f.GoogleSearchGeneration || strings.TrimSpace(f.GoogleQuery) != query {
			latestQuery := strings.TrimSpace(f.GoogleQuery)
			latestGeneration := f.GoogleSearchGeneration
			if latestQuery != "" {
				p.scheduleTeamGoogleContactSearch(ctx, f, latestGeneration, latestQuery, 150*time.Millisecond)
			}
			ctx.Update()
			return
		}
		if err != nil {
			f.GoogleNotice = "Não foi possível buscar os contatos. Autorize novamente a permissão de contatos Google e tente outra vez."
			f.GoogleContacts = nil
		} else {
			encoded, _ := json.Marshal(result["contacts"])
			if json.Unmarshal(encoded, &f.GoogleContacts) != nil {
				f.GoogleContacts = nil
			}
			if len(f.GoogleContacts) == 0 {
				f.GoogleNotice = "Nenhum contato do Google encontrado para \"" + query + "\"."
			} else {
				f.GoogleNotice = "Selecione um contato para preencher o cadastro."
			}
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) selectTeamGoogleContact(contact googlecontacts.Contact) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		f := p.teamCustomerForm
		if f == nil {
			return
		}
		f.GoogleSearchGeneration++
		f.GoogleQuery = ""
		if contact.Name != "" {
			f.Name = contact.Name
		}
		if contact.Phone != "" {
			f.Phone = contact.Phone
		}
		if !f.Editing && contact.Email != "" && strings.TrimSpace(f.Email) == "" {
			f.Email = contact.Email
		}
		f.GoogleContacts, f.GoogleNotice = nil, "Contato selecionado; revise os dados antes de salvar."
		ctx.Update()
	}
}

func (p *serviceCatalogPage) teamGoogleContactsPicker(f *teamCustomerForm) app.UI {
	content := []app.UI{app.H3().Class("team-section__heading-title").Body(app.Text("Preencher por contato Google"))}
	if p.session == nil || p.session.ProviderToken == "" {
		content = append(content,
			app.P().Class("portal-section__intro").Body(app.Text("Conecte a conta Google e autorize a busca de contatos para preencher o cadastro.")),
			app.Button().Class("auth-link").Type("button").OnClick(p.connectTeamGoogleContacts).Body(app.Text("Conectar contatos do Google")),
		)
	} else {
		content = append(content,
			app.Label().Class("auth-field").Body(app.Text("Buscar por nome, telefone ou e-mail"), app.Input().Type("search").Value(f.GoogleQuery).Placeholder("Ex.: Maria Silva").OnInput(p.updateTeamGoogleContactQuery)),
			app.Button().Class("auth-link").Type("button").OnClick(p.connectTeamGoogleContacts).Body(app.Text("Renovar permissão Google")),
		)
	}
	if f.GoogleNotice != "" {
		content = append(content, app.P().Class("portal-section__intro").Body(app.Text(f.GoogleNotice)))
	}
	for _, contact := range f.GoogleContacts {
		contact := contact
		label := contact.Phone
		if contact.Email != "" {
			if label != "" {
				label += " · "
			}
			label += contact.Email
		}
		if contact.Name == "" {
			contact.Name = "Contato sem nome"
		}
		content = append(content, app.Button().Class("team-customer-contact").Type("button").OnClick(p.selectTeamGoogleContact(contact)).Body(app.Strong().Body(app.Text(contact.Name)), app.Span().Body(app.Text(label))))
	}
	for _, contact := range f.DeviceContacts {
		contact := contact
		label := contact.Phone
		if contact.Email != "" {
			if label != "" {
				label += " · "
			}
			label += contact.Email
		}
		name := contact.Name
		if name == "" {
			name = "Contato sem nome"
		}
		content = append(content, app.Button().Class("team-customer-contact").Type("button").OnClick(p.selectTeamDeviceContact(contact)).Body(app.Strong().Body(app.Text(name)), app.Span().Body(app.Text(label))))
	}
	return app.Div().Class("team-customer-contacts").Body(content...)
}

func (p *serviceCatalogPage) teamDeviceContactsPicker(f *teamCustomerForm) app.UI {
	if f == nil {
		return app.Div()
	}
	label := "Buscar nos contatos do celular"
	if f.DeviceContactsBusy {
		label = "Abrindo contatos..."
	}
	content := []app.UI{
		app.Button().Class("auth-submit team-device-contacts__button").Type("button").Disabled(f.DeviceContactsBusy).OnClick(p.pickTeamDeviceContacts).Body(app.Text(label)),
	}
	if deviceContactPickerAvailable() {
		content = append(content, app.P().Class("portal-section__intro").Body(app.Text("Preenche automaticamente nome, telefone, e-mail e endereço disponíveis no contato.")))
	} else {
		content = append(content, app.P().Class("portal-section__intro").Body(app.Text("No Android ou iPhone, use o app instalado para abrir a agenda do aparelho.")))
	}
	if f.GoogleNotice != "" {
		content = append(content, app.P().Class("team-device-contacts__notice").Body(app.Text(f.GoogleNotice)))
	}
	for _, contact := range f.DeviceContacts {
		contact := contact
		label := strings.TrimSpace(strings.Join([]string{contact.Phone, contact.Email}, " · "))
		if label == "" {
			label = "Contato sem telefone ou e-mail"
		}
		content = append(content, app.Button().Class("team-customer-contact").Type("button").OnClick(p.selectTeamDeviceContact(contact)).Body(app.Strong().Body(app.Text(firstNonEmptyBudget(contact.Name, "Contato sem nome"))), app.Span().Body(app.Text(label))))
	}
	return app.Div().Class("team-device-contacts").Body(content...)
}

func (p *serviceCatalogPage) pickTeamDeviceContacts(ctx app.Context, event app.Event) {
	event.PreventDefault()
	f := p.teamCustomerForm
	if f == nil || f.DeviceContactsBusy {
		return
	}
	if !deviceContactPickerAvailable() {
		f.GoogleNotice = "A agenda do aparelho está disponível pelo app instalado no Android ou iPhone."
		ctx.Update()
		return
	}
	f.DeviceContactsBusy, f.GoogleNotice = true, ""
	f.DeviceContacts = nil
	ctx.Update()
	go func() {
		result := <-pickDeviceContacts()
		if p.teamCustomerForm != f {
			return
		}
		f.DeviceContactsBusy = false
		if result.Cancelled {
			f.GoogleNotice = ""
		} else if result.Err != "" {
			f.GoogleNotice = "Não foi possível acessar os contatos do aparelho. Verifique a permissão e tente novamente."
		} else if len(result.Contacts) == 0 {
			f.GoogleNotice = "Nenhum contato selecionado."
		} else if len(result.Contacts) == 1 {
			contact := result.Contacts[0]
			applyDeviceContact(f, contact)
			if contact.PostalCode != "" {
				p.lookupTeamCustomerPostalCode(ctx, event)
			}
			f.GoogleNotice = "Contato selecionado; revise os dados antes de salvar."
		} else {
			f.DeviceContacts = result.Contacts
			f.GoogleNotice = "Selecione um contato para preencher o cadastro."
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) selectTeamDeviceContact(contact deviceContact) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		if f := p.teamCustomerForm; f != nil {
			applyDeviceContact(f, contact)
			f.DeviceContacts, f.GoogleNotice = nil, "Contato selecionado; revise os dados antes de salvar."
			if contact.PostalCode != "" {
				p.lookupTeamCustomerPostalCode(ctx, event)
			}
		}
		ctx.Update()
	}
}

func applyDeviceContact(form *teamCustomerForm, contact deviceContact) {
	if contact.Name != "" {
		form.Name = contact.Name
	}
	if contact.Phone != "" {
		form.Phone = normalizeDevicePhone(contact.Phone)
	}
	if !form.Editing && contact.Email != "" && strings.TrimSpace(form.Email) == "" {
		form.Email = contact.Email
	}
	if strings.TrimSpace(form.Address) == "" {
		form.Address = contact.Address
	}
	if strings.TrimSpace(form.PostalCode) == "" && contact.PostalCode != "" {
		form.PostalCode = cep.Mask(contact.PostalCode)
	}
	if strings.TrimSpace(form.City) == "" {
		form.City = contact.City
	}
	if strings.TrimSpace(form.State) == "" {
		form.State = contact.State
	}
}

func normalizeDevicePhone(value string) string {
	var normalized strings.Builder
	for _, char := range value {
		if char >= '0' && char <= '9' || char == '+' {
			normalized.WriteRune(char)
		}
	}
	return normalized.String()
}
