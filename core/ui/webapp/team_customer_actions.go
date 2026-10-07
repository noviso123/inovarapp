package webapp

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

type teamCustomerTemporaryAccess struct {
	Email, Password string
}

type teamApplianceForm struct {
	ID, ClientID, Brand, Model, Capacity, Type, Room string
	Serial, InstallSite, InstallDate, Notes          string
	GasType, Voltage                                 string
	Saving                                           bool
	Message                                          string
}

func (p *serviceCatalogPage) openTeamApplianceForm(client map[string]any, appliance map[string]any) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		if client == nil || portalText(client["id"]) == "" {
			p.teamCustomerNotice = "Não foi possível identificar o cliente deste aparelho."
			ctx.Update()
			return
		}
		form := &teamApplianceForm{ClientID: portalText(client["id"]), Brand: "LG", Capacity: "12000", Type: "Split Hi-Wall", GasType: "R-410A", Voltage: "220V"}
		if appliance != nil {
			form.ID, form.ClientID = portalText(appliance["id"]), portalText(appliance["cliente_id"])
			form.Brand, form.Model, form.Capacity = portalText(appliance["marca"]), portalText(appliance["modelo"]), completionNumber(appliance["btus"])
			form.Type, form.Room = portalText(appliance["tipo"]), portalText(appliance["ambiente"])
			form.Serial, form.InstallSite, form.InstallDate, form.Notes = portalText(appliance["numero_serie"]), portalText(appliance["local_instalacao"]), portalText(appliance["data_instalacao"]), portalText(appliance["observacoes"])
			form.GasType, form.Voltage = firstNonEmptyBudget(portalText(appliance["gas_tipo"]), "R-410A"), firstNonEmptyBudget(portalText(appliance["tensao"]), "220V")
		}
		p.teamApplianceForm, p.teamCustomerNotice = form, ""
		ctx.Update()
	}
}

func (p *serviceCatalogPage) closeTeamApplianceForm(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.teamApplianceForm != nil && !p.teamApplianceForm.Saving {
		p.teamApplianceForm = nil
		ctx.Update()
	}
}

func (p *serviceCatalogPage) saveTeamAppliance(ctx app.Context, event app.Event) {
	event.PreventDefault()
	f := p.teamApplianceForm
	if f == nil || f.Saving || p.session == nil {
		return
	}
	capacity, err := strconv.Atoi(strings.TrimSpace(f.Capacity))
	if f.ClientID == "" || strings.TrimSpace(f.Brand) == "" || strings.TrimSpace(f.Type) == "" || strings.TrimSpace(f.Room) == "" || err != nil {
		f.Message = "Informe marca, capacidade, tipo e ambiente do aparelho."
		return
	}
	pageURL := app.Window().URL()
	if pageURL == nil || pageURL.Scheme == "" || pageURL.Host == "" {
		f.Message = "Não foi possível conectar ao servidor."
		return
	}
	endpoint, token := apiBaseURL()+"/api/aparelhos", p.session.AccessToken
	fields := teamAppliancePayload(*f, capacity, time.Now().UTC().Format("2006-01-02"))
	method, payload := http.MethodPost, any(fields)
	if f.ID != "" {
		method, payload = http.MethodPatch, map[string]any{"id": f.ID, "fields": teamApplianceEditPayload(fields, f.ID)}
	}
	f.Saving = true
	f.Message = ""
	go func() {
		_, err := sendTeamJSONResult(ctx, endpoint, token, method, payload)
		f.Saving = false
		if err != nil {
			if method == http.MethodPatch && p.deferOfflineTeamMutation(endpoint, method, payload, err) {
				p.teamApplianceForm = nil
				p.teamCustomerNotice = "Edição do aparelho salva neste dispositivo. Será sincronizada quando a conexão voltar."
				p.teamLoadedFor = ""
				p.retryPendingOfflineMutations(ctx)
			} else {
				f.Message = "Não foi possível salvar os dados do aparelho."
			}
			ctx.Update()
			return
		}
		p.teamApplianceForm = nil
		p.teamCustomerNotice = "Aparelho salvo."
		p.teamLoadedFor = ""
		p.loadTeamOperations(ctx)
		ctx.Update()
	}()
}

func teamAppliancePayload(form teamApplianceForm, capacity int, lastMaintenanceDate string) map[string]any {
	return map[string]any{
		"cliente_id": form.ClientID, "marca": strings.TrimSpace(form.Brand), "modelo": strings.TrimSpace(form.Model),
		"btus": capacity, "tipo": strings.TrimSpace(form.Type), "ambiente": strings.TrimSpace(form.Room),
		"gas_tipo": strings.TrimSpace(form.GasType), "tensao": strings.TrimSpace(form.Voltage),
		"numero_serie": strings.TrimSpace(form.Serial), "local_instalacao": strings.TrimSpace(form.InstallSite),
		"data_instalacao": strings.TrimSpace(form.InstallDate), "observacoes": strings.TrimSpace(form.Notes),
		"ultima_manutencao": lastMaintenanceDate,
	}
}

func teamApplianceEditPayload(createFields map[string]any, applianceID string) map[string]any {
	fields := map[string]any{}
	for _, key := range []string{"marca", "modelo", "btus", "tipo", "ambiente", "gas_tipo", "tensao"} {
		if value, exists := createFields[key]; exists {
			fields[key] = value
		}
	}
	// Preserve registration and maintenance metadata not edited by the ficha.
	return map[string]any{"id": applianceID, "fields": fields}
}

func teamApplianceBrands() []string {
	return []string{"LG", "Gree", "Midea", "Samsung", "Daikin", "Carrier", "Elgin", "Fujitsu", "Consul", "Electrolux", "Springer", "TCL", "Outra"}
}

func teamApplianceCapacities() []string {
	return []string{"7000", "9000", "12000", "18000", "24000", "30000", "36000", "48000", "60000"}
}

func teamApplianceCapacityLabel(value string) string {
	if len(value) > 3 {
		return value[:len(value)-3] + "." + value[len(value)-3:]
	}
	return value
}

func teamApplianceEditTypes() []string {
	return []string{"Split Hi-Wall", "Inverter", "Cassete", "Piso Teto", "Janela", "Multi Split", "Portátil"}
}

func teamApplianceCreateTypes() []string {
	return []string{"Split Hi-Wall", "Inverter", "Cassete", "Piso Teto", "Janela"}
}

func teamApplianceOptions(values []string) []app.UI {
	options := make([]app.UI, 0, len(values))
	for _, value := range values {
		options = append(options, app.Option().Value(value).Body(app.Text(value)))
	}
	return options
}

func teamApplianceDialogTitle(form teamApplianceForm, customers []map[string]any) string {
	if form.ID != "" {
		return "Editar aparelho"
	}
	if customer := appointmentClient(customers, form.ClientID); customer != nil {
		if name := strings.TrimSpace(portalText(customer["nome"])); name != "" {
			return "Adicionar Aparelho para " + name
		}
	}
	return "Adicionar aparelho"
}

func (p *serviceCatalogPage) askTeamCustomerAction(action string) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		p.teamCustomerConfirm = action
		ctx.Update()
	}
}
func (p *serviceCatalogPage) cancelTeamCustomerAction(ctx app.Context, event app.Event) {
	event.PreventDefault()
	p.teamCustomerConfirm = ""
	ctx.Update()
}

func (p *serviceCatalogPage) runTeamCustomerAction(ctx app.Context, event app.Event) {
	event.PreventDefault()
	action := p.teamCustomerConfirm
	if action == "" || p.session == nil || p.teamCustomerActionSaving {
		return
	}
	pageURL := app.Window().URL()
	if pageURL == nil || pageURL.Scheme == "" || pageURL.Host == "" {
		p.teamActionMessage = "Não foi possível conectar ao servidor."
		ctx.Update()
		return
	}
	base, token := apiBaseURL(), p.session.AccessToken
	p.teamCustomerActionSaving = true
	if strings.HasPrefix(action, teamDeleteServiceAction) && p.teamServiceDetail != nil && portalText(p.teamServiceDetail["id"]) == strings.TrimPrefix(action, teamDeleteServiceAction) {
		p.teamServiceDeleteNotice = ""
	}
	ctx.Update()
	go func() {
		var notice string
		switch {
		case strings.HasPrefix(action, "delete-customer:"):
			id := strings.TrimPrefix(action, "delete-customer:")
			payload := map[string]any{"id": id}
			_, err := sendTeamJSONResult(ctx, base+"/api/clientes", token, http.MethodDelete, payload)
			if err != nil {
				if p.deferOfflineTeamMutation(base+"/api/clientes", http.MethodDelete, payload, err) {
					notice = "Exclusão salva neste dispositivo e será sincronizada quando a conexão voltar."
				} else {
					notice = "Não foi possível excluir o cliente. Verifique se existem registros vinculados."
				}
			} else {
				notice = "Cliente excluído."
			}
		case strings.HasPrefix(action, "delete-appliance:"):
			id := strings.TrimPrefix(action, "delete-appliance:")
			payload := map[string]any{"id": id}
			_, err := sendTeamJSONResult(ctx, base+"/api/aparelhos", token, http.MethodDelete, payload)
			if err != nil {
				if p.deferOfflineTeamMutation(base+"/api/aparelhos", http.MethodDelete, payload, err) {
					notice = "Exclusão do aparelho salva neste dispositivo e será sincronizada ao reconectar."
				} else {
					notice = "Não foi possível excluir o aparelho."
				}
			} else {
				notice = "Aparelho excluído."
			}
		case strings.HasPrefix(action, teamDeleteLegacyHistoryAction):
			id := strings.TrimPrefix(action, teamDeleteLegacyHistoryAction)
			payload := map[string]any{"id": id}
			endpoint := base + "/api/historico?origem=retornos-avulsos"
			_, err := sendTeamJSONResult(ctx, endpoint, token, http.MethodDelete, payload)
			queued := false
			if err != nil {
				queued = p.deferOfflineTeamMutation(endpoint, http.MethodDelete, payload, err)
				if queued {
					notice = "Exclusão do histórico salva neste dispositivo e será sincronizada ao reconectar."
				} else {
					notice = "Não foi possível excluir este histórico."
				}
			} else {
				notice = "Registro de histórico excluído."
			}
			if err == nil || queued {
				p.teamHistoryLegacyRows = filterOfflineRows(p.teamHistoryLegacyRows, "id", id)
				p.teamReturnHistory = filterOfflineRows(p.teamReturnHistory, "id", id)
				p.teamHistoryNotice = notice
			}
		case strings.HasPrefix(action, teamDeleteHistoryServiceAction):
			id := strings.TrimPrefix(action, teamDeleteHistoryServiceAction)
			payload := map[string]any{"id": id}
			endpoint := base + "/api/servicos"
			_, err := sendTeamJSONResult(ctx, endpoint, token, http.MethodDelete, payload)
			queued := false
			if err != nil {
				queued = p.deferOfflineTeamMutation(endpoint, http.MethodDelete, payload, err)
				if queued {
					notice = "Exclusão da OS salva neste dispositivo e será sincronizada ao reconectar."
				} else {
					notice = "Não foi possível excluir esta OS."
				}
			} else {
				notice = "Ordem de serviço excluída."
			}
			if err == nil || queued {
				p.teamHistoryRows = filterOfflineRows(p.teamHistoryRows, "id", id)
				p.teamServices = filterOfflineRows(p.teamServices, "id", id)
				p.teamReturnHistory = filterOfflineRows(p.teamReturnHistory, "id", id)
				p.teamHistoryNotice = notice
				if p.teamServiceDetail != nil && portalText(p.teamServiceDetail["id"]) == id {
					p.teamServiceDetail = nil
				}
			}
		case strings.HasPrefix(action, "reset-password:"):
			id := strings.TrimPrefix(action, "reset-password:")
			result, err := sendTeamJSONResult(ctx, base+"/api/contas", token, http.MethodPost, map[string]any{"acao": "redefinir_cliente", "customer_id": id})
			if err != nil {
				notice = "Não foi possível redefinir o acesso. O cliente precisa ter uma conta vinculada."
			} else {
				p.teamCustomerTemporaryAccess = &teamCustomerTemporaryAccess{Email: portalText(result["email"]), Password: portalText(result["senha_temporaria"])}
				p.teamCustomerAccessCopied, p.teamCustomerAccessNotice = false, ""
				notice = "Senha temporária criada. O cliente deverá alterá-la no próximo acesso."
				if warning := portalText(result["whatsapp_aviso"]); warning != "" {
					notice += " " + warning
				}
			}
		case strings.HasPrefix(action, teamPhotoDeleteAction):
			photoPath := strings.TrimPrefix(action, teamPhotoDeleteAction)
			_, err := sendTeamJSONResult(ctx, base+"/api/documentos", token, http.MethodPost, map[string]any{"acao": "excluirfoto", "path": photoPath})
			if err != nil {
				notice = "Não foi possível excluir a foto."
			} else {
				notice = "Foto excluída."
				p.teamPhotoNotice = notice
				p.loadAppliancePhotos(ctx, p.teamPhotoApplianceID)
			}
		case strings.HasPrefix(action, teamServicePhotoDeleteAction):
			photoPath := strings.TrimPrefix(action, teamServicePhotoDeleteAction)
			var err error
			if localID, local := localServicePhotoID(photoPath); local {
				err = deleteLocalServicePhoto(localID)
			} else {
				_, err = sendTeamJSONResult(ctx, base+"/api/documentos", token, http.MethodPost, map[string]any{"acao": "excluirfoto", "path": photoPath})
			}
			if err != nil {
				notice = "Não foi possível excluir a foto da OS."
			} else {
				notice = "Foto da OS excluída."
				if p.teamServicePhotoID != "" {
					p.teamServicePhotoNotice = notice
					p.loadTeamServicePhotos(ctx, p.teamServicePhotoID)
				}
			}
		case strings.HasPrefix(action, teamDeleteServiceAction):
			serviceID := strings.TrimPrefix(action, teamDeleteServiceAction)
			payload := map[string]any{"id": serviceID}
			_, err := sendTeamJSONResult(ctx, base+"/api/servicos", token, http.MethodDelete, payload)
			if err != nil {
				if p.deferOfflineTeamMutation(base+"/api/servicos", http.MethodDelete, payload, err) {
					notice = "Exclusão da ordem salva neste dispositivo e será sincronizada ao reconectar."
					p.teamServiceDetail = nil
					p.teamServiceDeleteNotice = ""
				} else {
					notice = "Não foi possível excluir do banco — item restaurado."
					if p.teamServiceDetail != nil && portalText(p.teamServiceDetail["id"]) == serviceID {
						p.teamServiceDeleteNotice = notice
					}
				}
			} else {
				notice = "Serviço excluído do banco."
				if p.teamServiceDetail != nil && portalText(p.teamServiceDetail["id"]) == serviceID {
					p.teamServiceDetail = nil
					p.teamServiceDeleteNotice = ""
				}
			}
		}
		p.teamCustomerNotice = notice
		p.teamCustomerActionSaving = false
		p.teamCustomerConfirm = ""
		p.teamLoadedFor = ""
		if p.offlineMode {
			p.retryPendingOfflineMutations(ctx)
		} else {
			p.loadTeamOperations(ctx)
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) closeTeamCustomerTemporaryAccess(ctx app.Context, event app.Event) {
	event.PreventDefault()
	p.teamCustomerTemporaryAccess = nil
	p.teamCustomerAccessCopied, p.teamCustomerAccessNotice = false, ""
}

func (p *serviceCatalogPage) teamCustomerTemporaryAccessDialog() app.UI {
	access := p.teamCustomerTemporaryAccess
	if access == nil {
		return app.Div()
	}
	copyText := "Login: " + access.Email + "\nSenha temporária: " + access.Password
	return app.Div().Class("auth-backdrop").Style("z-index", "90").Body(app.Div().Class("auth-dialog team-customer-access-dialog").Role("dialog").Body(
		app.H2().Class("auth-dialog__title").Body(app.Text("Senha temporária criada")),
		app.P().Class("auth-dialog__intro").Body(app.Text("O cliente deverá criar uma nova senha pessoal no próximo acesso.")),
		app.Div().Class("team-customer-access-dialog__credentials").Body(
			app.P().Body(app.Span().Body(app.Text("Login")), app.Strong().Body(app.Text(access.Email))),
			app.P().Body(app.Span().Body(app.Text("Senha temporária")), app.Strong().Body(app.Text(access.Password))),
		),
		func() app.UI {
			if p.teamCustomerAccessNotice == "" {
				return app.Div()
			}
			return app.P().Class("portal-section__intro").Attr("role", "status").Body(app.Text(p.teamCustomerAccessNotice))
		}(),
		app.Div().Class("portal-budget__actions").Body(
			app.Button().Class("auth-link").Type("button").OnClick(copyTextToClipboardNotice(copyText, func(ctx app.Context, copied bool) {
				p.teamCustomerAccessCopied = copied
				if copied {
					p.teamCustomerAccessNotice = "Acesso copiado."
				} else {
					p.teamCustomerAccessNotice = "Não foi possível copiar. Selecione e copie os dados manualmente."
				}
			})).Body(app.Text(func() string {
				if p.teamCustomerAccessCopied {
					return "Copiado"
				}
				return "Copiar acesso"
			}())),
			app.Button().Class("auth-submit").Type("button").OnClick(p.closeTeamCustomerTemporaryAccess).Body(app.Text("Concluir")),
		),
	))
}

func (p *serviceCatalogPage) teamApplianceDialog() app.UI {
	f := p.teamApplianceForm
	if f == nil {
		return app.Div()
	}
	field := func(label string, target *string, kind string) app.UI {
		return app.Label().Class("auth-field").Body(app.Text(label), app.Input().Type(kind).Value(*target).OnChange(p.ValueTo(target)))
	}
	capacities := teamApplianceCapacities()
	capOptions := make([]app.UI, 0, len(capacities))
	for _, value := range capacities {
		capOptions = append(capOptions, app.Option().Value(value).Body(app.Text(teamApplianceCapacityLabel(value)+" BTUs")))
	}
	types := teamApplianceEditTypes()
	if f.ID == "" {
		types = teamApplianceCreateTypes()
	}
	typeOptions := make([]app.UI, 0, len(types))
	for _, value := range types {
		typeOptions = append(typeOptions, app.Option().Value(value).Body(app.Text(value)))
	}
	title := teamApplianceDialogTitle(*f, p.teamCustomers)
	fields := []app.UI{
		app.Label().Class("auth-field").Body(app.Text("Marca"), app.Select().Attr("value", f.Brand).OnChange(p.ValueTo(&f.Brand)).Body(teamApplianceOptions(teamApplianceBrands())...)),
		field("Modelo", &f.Model, "text"),
		app.Label().Class("auth-field").Body(app.Text("Capacidade"), app.Select().Attr("value", f.Capacity).OnChange(p.ValueTo(&f.Capacity)).Body(capOptions...)),
		app.Label().Class("auth-field").Body(app.Text("Tipo"), app.Select().Attr("value", f.Type).OnChange(p.ValueTo(&f.Type)).Body(typeOptions...)),
		field("Ambiente / cômodo", &f.Room, "text"),
	}
	if f.ID == "" {
		fields = append(fields,
			app.Label().Class("auth-field").Body(app.Text("Fluido Refrigerante"), app.Select().Attr("value", f.GasType).OnChange(p.ValueTo(&f.GasType)).Body(teamApplianceOptions([]string{"R-410A", "R-32", "R-22"})...)),
			app.Label().Class("auth-field").Body(app.Text("Tensão"), app.Select().Attr("value", f.Voltage).OnChange(p.ValueTo(&f.Voltage)).Body(teamApplianceOptions([]string{"220V", "110V"})...)),
			field("Número de série", &f.Serial, "text"),
			field("Local de instalação", &f.InstallSite, "text"),
			field("Data de instalação", &f.InstallDate, "date"),
			app.Label().Class("auth-field").Body(app.Text("Observações Técnicas"), app.Textarea().Rows(2).Text(f.Notes).Placeholder("Ex: Ponto de energia dedicado no quadro...").OnChange(p.ValueTo(&f.Notes))),
		)
	} else {
		gasTypes := []string{"R-410A", "R-32", "R-22", "Outro"}
		gasOptions := make([]app.UI, 0, len(gasTypes))
		for _, value := range gasTypes {
			gasOptions = append(gasOptions, app.Option().Value(value).Body(app.Text(value)))
		}
		fields = append(fields,
			app.Label().Class("auth-field").Body(app.Text("Fluido Refrigerante (Gás)"), app.Select().Attr("value", f.GasType).OnChange(p.ValueTo(&f.GasType)).Body(gasOptions...)),
			app.Label().Class("auth-field").Body(app.Text("Tensão"), app.Select().Attr("value", f.Voltage).OnChange(p.ValueTo(&f.Voltage)).Body(
				app.Option().Value("220V").Body(app.Text("220V")), app.Option().Value("110V").Body(app.Text("110V")), app.Option().Value("Bivolt").Body(app.Text("Bivolt")),
			)),
		)
	}
	fields = append(fields,
		formErrorNotice(f.Message),
		app.Div().Class("portal-budget__actions").Body(
			app.Button().Class("auth-link").Type("button").Disabled(f.Saving).OnClick(p.closeTeamApplianceForm).Body(app.Text("Cancelar")),
			app.Button().Class("auth-submit").Type("submit").Disabled(f.Saving).Body(app.Text("Salvar aparelho")),
		),
	)
	form := app.Form().Class("auth-form team-appliance-form").OnSubmit(p.saveTeamAppliance).Body(fields...)
	return app.Div().Class("auth-backdrop team-appliance-backdrop").Body(app.Div().Class("auth-dialog team-appliance-dialog").Role("dialog").Body(
		app.Div().Class("auth-dialog__heading").Body(
			app.H2().Class("auth-dialog__title").Body(app.Text(title)),
			app.Button().Class("auth-close").Type("button").Disabled(f.Saving).OnClick(p.closeTeamApplianceForm).Body(app.Text("Fechar")),
		),
		form,
	))
}
