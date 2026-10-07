package webapp

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/adapter/whatsapp"
	"inovarapp/core/domain"
)

type teamSettingsForm struct {
	Name, BusinessName, CNPJ, Address, Phone string
	PIXKey, PIXType                          string
	DefaultPrice, ReturnMonths, WarrantyDays string
}

func newTeamSettingsForm(profile domain.TechnicianProfile) *teamSettingsForm {
	return &teamSettingsForm{
		Name: profile.Name, BusinessName: profile.BusinessName, CNPJ: profile.CNPJ, Address: profile.Address, Phone: profile.Phone,
		PIXKey: profile.PIXKey, PIXType: string(profile.PIXType), DefaultPrice: strconv.FormatFloat(profile.DefaultPrice, 'f', -1, 64),
		ReturnMonths: strconv.Itoa(profile.DefaultReturnMonths), WarrantyDays: strconv.Itoa(profile.DefaultWarrantyDays),
	}
}

func (p *serviceCatalogPage) openTeamSettings(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.caller == nil || (p.caller.Role != domain.RoleAdmin && p.caller.Role != domain.RoleTechnician) {
		return
	}
	p.teamSettingsForm = newTeamSettingsForm(p.teamProfile)
	p.teamSettingsNotice = ""
	p.teamEmailUsername, p.teamEmailPassword = "", ""
	p.teamEmailPasswordSaved, p.teamEmailBusy, p.teamEmailNotice = false, false, ""
	p.teamEmailGoogleConnected, p.teamEmailGoogleReady, p.teamEmailGoogleAccount = false, false, ""
	p.teamMessageKey, p.teamMessageNotice = "", ""
	p.teamMessageInterval = strconv.Itoa(p.teamProfile.ReminderIntervalDays)
	if p.teamProfile.ReminderIntervalDays <= 0 {
		p.teamMessageInterval = "7"
	}
	// Keep pairing actions hidden until the server confirms that the Go service
	// is configured. This prevents a slow status request from exposing dead buttons.
	p.teamWhatsAppStatus = map[string]any{"configurado": false, "conectado": false}
	p.teamWhatsAppQR, p.teamWhatsAppPairing, p.teamWhatsAppSaved = "", "", false
	p.teamSettingsOpen = true
	p.refreshTeamWhatsApp(ctx)
	p.refreshTeamEmailSettings(ctx)
	ctx.Update()
}

func (p *serviceCatalogPage) closeTeamSettings(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if !p.teamSettingsSaving {
		p.teamSettingsOpen = false
		p.teamSettingsForm = nil
		p.teamEmailPassword = ""
		p.teamWhatsAppGeneration++
	}
	ctx.Update()
}

func (p *serviceCatalogPage) refreshTeamEmailSettings(ctx app.Context) {
	if p.session == nil {
		return
	}
	token := p.session.AccessToken
	go func() {
		result, err := sendTeamJSONResult(ctx, apiBaseURL()+"/api/configuracoes", token, http.MethodGet, nil)
		googleStatus, googleErr := sendTeamJSONResult(ctx, apiBaseURL()+"/api/google-email", token, http.MethodGet, nil)
		if p.session == nil || p.session.AccessToken != token {
			return
		}
		config, _ := result["config"].(map[string]any)
		if err != nil || result["ok"] != true || config == nil {
			p.teamEmailNotice = "Não foi possível carregar as configurações de e-mail."
			ctx.Update()
			return
		}
		p.teamEmailUsername = portalText(config["email_gmail_user"])
		p.teamEmailPasswordSaved = strings.HasPrefix(portalText(config["email_gmail_pass"]), "***")
		if googleErr == nil {
			p.teamEmailGoogleConnected, _ = googleStatus["connected"].(bool)
			p.teamEmailGoogleReady, _ = googleStatus["configured"].(bool)
			p.teamEmailGoogleAccount = portalText(googleStatus["email"])
		} else {
			p.teamEmailNotice = "Não foi possível verificar a conexão com o Google."
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) saveTeamEmailSettings(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.session == nil || p.teamEmailBusy {
		return
	}
	username, password := strings.TrimSpace(p.teamEmailUsername), strings.TrimSpace(p.teamEmailPassword)
	if username == "" {
		p.teamEmailNotice = "Informe o usuário Gmail. Deixe a senha vazia para manter a senha de app atual."
		ctx.Update()
		return
	}
	if password == "" && !p.teamEmailPasswordSaved {
		p.teamEmailNotice = "Informe a senha de app do Gmail para configurar o canal."
		ctx.Update()
		return
	}
	token := p.session.AccessToken
	p.teamEmailBusy, p.teamEmailNotice = true, "Salvando credenciais do Gmail..."
	ctx.Update()
	go func() {
		payload := map[string]string{"email_gmail_user": username}
		if password != "" {
			payload["email_gmail_pass"] = password
		}
		result, err := sendTeamJSONResult(ctx, apiBaseURL()+"/api/configuracoes", token, http.MethodPost, payload)
		if p.session == nil || p.session.AccessToken != token {
			return
		}
		p.teamEmailBusy = false
		if err != nil || result["ok"] != true {
			p.teamEmailNotice = "Não foi possível salvar o Gmail. Verifique sua conexão e tente novamente."
		} else {
			p.teamEmailUsername = username
			p.teamEmailPassword = ""
			p.teamEmailPasswordSaved = true
			p.teamEmailNotice = "Credenciais do Gmail salvas no servidor. A senha não é exibida nesta tela."
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) testTeamEmail(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.session == nil || p.teamEmailBusy {
		return
	}
	to, name := "", "Equipe Inovar"
	if p.session.User != nil {
		to = strings.TrimSpace(p.session.User.Email)
	}
	if to == "" {
		p.teamEmailNotice = "Não encontramos um e-mail na sessão da sua conta para receber o teste."
		ctx.Update()
		return
	}
	token := p.session.AccessToken
	p.teamEmailBusy, p.teamEmailNotice = true, "Enviando teste para "+to+"..."
	ctx.Update()
	go func() {
		result, err := sendTeamJSONResult(ctx, apiBaseURL()+"/api/email", token, http.MethodPost, map[string]string{"acao": "testar", "para": to, "nome": name})
		if p.session == nil || p.session.AccessToken != token {
			return
		}
		p.teamEmailBusy = false
		if err != nil {
			p.teamEmailNotice = teamEmailTestFailureNotice(err)
		} else if result["ok"] == true {
			p.teamEmailNotice = "E-mail de teste enviado para " + to + ". Confira também a caixa de spam."
		} else {
			p.teamEmailNotice = portalText(result["mensagem"])
			if p.teamEmailNotice == "" {
				p.teamEmailNotice = "O envio de teste falhou. Confira as credenciais do Gmail."
			}
		}
		ctx.Update()
	}()
}

func teamEmailTestFailureNotice(err error) string {
	if err != nil {
		return "Não foi possível enviar o teste. Verifique as credenciais e tente novamente."
	}
	return "O envio de teste falhou. Confira as credenciais do Gmail."
}

func (p *serviceCatalogPage) saveTeamSettings(ctx app.Context, event app.Event) {
	event.PreventDefault()
	f := p.teamSettingsForm
	if f == nil || p.teamSettingsSaving || p.session == nil {
		return
	}
	if strings.TrimSpace(f.Name) == "" {
		p.teamSettingsNotice = "Informe seu nome profissional."
		ctx.Update()
		return
	}
	if strings.TrimSpace(f.Phone) == "" {
		p.teamSettingsNotice = "Informe seu WhatsApp de contato."
		ctx.Update()
		return
	}
	price, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(f.DefaultPrice), ",", "."), 64)
	if err != nil || price < 0 {
		p.teamSettingsNotice = "Informe um preço padrão válido."
		return
	}
	months, err := strconv.Atoi(f.ReturnMonths)
	if err != nil || months <= 0 {
		p.teamSettingsNotice = "Informe um ciclo de retorno válido."
		return
	}
	warranty, err := strconv.Atoi(f.WarrantyDays)
	if err != nil || warranty < 0 {
		p.teamSettingsNotice = "Informe uma garantia válida."
		return
	}
	base := apiBaseURL()
	token := p.session.AccessToken
	profile := p.teamProfile
	profile.Name, profile.BusinessName, profile.CNPJ = strings.TrimSpace(f.Name), strings.TrimSpace(f.BusinessName), strings.TrimSpace(f.CNPJ)
	profile.Address, profile.Phone = strings.TrimSpace(f.Address), strings.TrimSpace(f.Phone)
	profile.PIXKey, profile.PIXType = strings.TrimSpace(f.PIXKey), domain.PIXKeyType(f.PIXType)
	profile.DefaultPrice, profile.DefaultReturnMonths, profile.DefaultWarrantyDays = price, months, warranty
	p.teamSettingsSaving, p.teamSettingsNotice = true, "Salvando configurações..."
	go func() {
		result, err := sendTeamJSONResult(ctx, base+"/api/configuracoes", token, http.MethodPost, profile)
		if p.session == nil || p.session.AccessToken != token {
			return
		}
		p.teamSettingsSaving = false
		if err != nil || result["ok"] != true {
			if isOfflineNetworkError(err) && p.caller != nil && enqueueOfflineTeamSettings(p.caller.UserID, p.caller.Role, profile, offlineTechnicianProfilePayload(profile)) == nil {
				p.teamProfile = profile
				p.teamSettingsForm = newTeamSettingsForm(profile)
				p.teamDashboard = buildTeamDashboardSummary(p.teamCustomers, p.teamServices, p.teamAppointments, p.teamBudgets, p.teamReturnHistory, profile.DefaultReturnMonths, dashboardCivilToday(time.Now()))
				p.teamSettingsNotice = "Configurações salvas neste dispositivo. Serão sincronizadas quando a conexão voltar."
				p.offlineMode = true
				p.teamLoadedFor = ""
				p.retryPendingOfflineMutations(ctx)
				ctx.Update()
				return
			}
			p.teamSettingsNotice = "Não foi possível salvar as configurações. Verifique sua conexão e tente novamente."
			ctx.Update()
			return
		}
		if raw, marshalErr := json.Marshal(result["config"]); marshalErr == nil {
			var saved domain.TechnicianProfile
			if json.Unmarshal(raw, &saved) == nil {
				profile = mergeTechnicianProfile(profile, saved)
			}
		}
		p.teamProfile = profile
		p.teamDashboard = buildTeamDashboardSummary(p.teamCustomers, p.teamServices, p.teamAppointments, p.teamBudgets, p.teamReturnHistory, profile.DefaultReturnMonths, dashboardCivilToday(time.Now()))
		p.teamSettingsForm = newTeamSettingsForm(profile)
		p.teamSettingsNotice = "Configurações salvas e sincronizadas entre os dispositivos."
		ctx.Update()
	}()
}

func offlineTechnicianProfilePayload(profile domain.TechnicianProfile) map[string]any {
	return map[string]any{
		"name": profile.Name, "businessName": profile.BusinessName, "cnpj": profile.CNPJ,
		"address": profile.Address, "phone": profile.Phone, "pixKey": profile.PIXKey,
		"pixType": profile.PIXType, "defaultReturnMonths": profile.DefaultReturnMonths,
		"defaultWarrantyDays": profile.DefaultWarrantyDays, "defaultPrice": profile.DefaultPrice,
		"tiposServicosCustom": profile.CustomServiceTypes,
		"tiposFixosRemovidos": profile.RemovedFixedServiceTypes, "tiposFixosEditados": profile.EditedFixedServiceTypes,
	}
}

func mergeTechnicianProfile(current, returned domain.TechnicianProfile) domain.TechnicianProfile {
	if returned.Signature == nil {
		returned.Signature = current.Signature
	}
	return returned
}

func (p *serviceCatalogPage) refreshTeamWhatsApp(ctx app.Context) {
	if p.session == nil || p.teamWhatsAppBusy {
		return
	}
	base, token := apiBaseURL(), p.session.AccessToken
	p.teamWhatsAppBusy, p.teamWhatsAppNotice = true, "Verificando conexão do WhatsApp..."
	go func() {
		result, err := sendTeamJSONResult(ctx, base+"/api/whatsapp", token, http.MethodPost, map[string]any{"acao": "status"})
		if p.session == nil || p.session.AccessToken != token {
			return
		}
		p.teamWhatsAppBusy = false
		if err != nil {
			p.teamWhatsAppNotice = whatsappConnectionError(err)
		} else {
			p.teamWhatsAppStatus = result
			p.teamWhatsAppNotice = portalText(result["mensagem"])
			if configured, exists := result["configurado"].(bool); exists && !configured {
				p.teamWhatsAppNotice = "WhatsApp ainda não configurado na produção. Falta apontar o app para a API Go persistente e cadastrar WHATSAPP_OWN_URL e WHATSAPP_OWN_TOKEN no Vercel."
				p.teamWhatsAppQR, p.teamWhatsAppPairing, p.teamWhatsAppSaved = "", "", false
				p.teamWhatsAppGeneration++
			}
			if connected, _ := result["conectado"].(bool); connected {
				p.teamWhatsAppQR, p.teamWhatsAppPairing, p.teamWhatsAppSaved = "", "", true
				p.teamWhatsAppGeneration++
			} else {
				p.teamWhatsAppSaved = false
				if whatsappPairingTerminal(portalText(result["estado"])) {
					p.teamWhatsAppQR, p.teamWhatsAppPairing = "", ""
					p.teamWhatsAppGeneration++
				}
			}
		}
		ctx.Update()
	}()
}

func whatsappConnectionError(err error) string {
	if err == nil {
		return "Não foi possível comunicar com o serviço WhatsApp."
	}
	detail := strings.TrimSpace(err.Error())
	lower := strings.ToLower(detail)
	if strings.Contains(lower, "a conexão automática do whatsapp ainda não está disponível no servidor") || strings.Contains(lower, "serviço whatsapp go ainda não foi configurado") {
		return "WhatsApp ainda não configurado na produção. Falta apontar o app para a API Go persistente e cadastrar WHATSAPP_OWN_URL e WHATSAPP_OWN_TOKEN no Vercel."
	}
	if strings.Contains(lower, "a api respondeu com status 502") || strings.Contains(lower, "a api respondeu com status 503") || strings.Contains(lower, "a api respondeu com status 504") {
		return "A API do WhatsApp não respondeu. Verifique se o serviço Go está ativo e tente novamente."
	}
	return "Falha na conexão com WhatsApp: " + detail
}

func (p *serviceCatalogPage) connectTeamWhatsApp(ctx app.Context, event app.Event) {
	event.PreventDefault()
	p.startTeamWhatsAppConnection(ctx, "")
}

func (p *serviceCatalogPage) pairTeamWhatsApp(ctx app.Context, event app.Event) {
	event.PreventDefault()
	phone := strings.TrimSpace(p.teamWhatsAppPhone)
	if _, valid := whatsapp.NormalizePhone(phone); !valid {
		p.teamWhatsAppNotice = "Informe um telefone válido com DDD e código do país para gerar o código."
		ctx.Update()
		return
	}
	p.startTeamWhatsAppConnection(ctx, phone)
}

func (p *serviceCatalogPage) startTeamWhatsAppConnection(ctx app.Context, phone string) {
	if p.teamWhatsAppBusy || p.session == nil {
		return
	}
	p.teamWhatsAppGeneration++
	generation := p.teamWhatsAppGeneration
	p.teamWhatsAppQR, p.teamWhatsAppPairing = "", ""
	base, token := apiBaseURL(), p.session.AccessToken
	p.teamWhatsAppBusy, p.teamWhatsAppNotice = true, "Solicitando conexão ao servidor..."
	go func() {
		result, err := sendTeamJSONResult(ctx, base+"/api/whatsapp", token, http.MethodPost, map[string]any{"acao": "conectar", "telefone": phone})
		if p.session == nil || p.session.AccessToken != token {
			return
		}
		p.teamWhatsAppBusy = false
		if err != nil {
			p.teamWhatsAppNotice = whatsappConnectionError(err)
		} else if connected, _ := result["conectado"].(bool); connected {
			p.teamWhatsAppStatus, p.teamWhatsAppNotice = result, portalText(result["mensagem"])
			p.teamWhatsAppQR, p.teamWhatsAppPairing, p.teamWhatsAppSaved = "", "", true
		} else {
			if configured, exists := result["configurado"].(bool); exists && !configured {
				p.teamWhatsAppStatus = result
				p.teamWhatsAppNotice = "WhatsApp ainda não configurado na produção. Falta apontar o app para a API Go persistente e cadastrar WHATSAPP_OWN_URL e WHATSAPP_OWN_TOKEN no Vercel."
				p.teamWhatsAppQR, p.teamWhatsAppPairing, p.teamWhatsAppSaved = "", "", false
				p.teamWhatsAppGeneration++
				ctx.Update()
				return
			}
			p.teamWhatsAppQR, p.teamWhatsAppPairing = portalText(result["qr"]), portalText(result["pairingCode"])
			p.teamWhatsAppStatus = map[string]any{"conectado": false, "estado": result["estado"], "configurado": true}
			if p.teamWhatsAppPairing != "" {
				p.teamWhatsAppNotice = "No celular, abra Aparelhos conectados → Conectar aparelho → Conectar com número de telefone e informe o código."
			} else if p.teamWhatsAppQR != "" {
				p.teamWhatsAppNotice = "Escaneie o QR com o WhatsApp. A conexão será detectada automaticamente."
			} else {
				p.teamWhatsAppNotice = portalText(result["mensagem"])
				if p.teamWhatsAppNotice == "" {
					p.teamWhatsAppNotice = "A conexão foi iniciada. Aguardando o QR Code ou o código de pareamento..."
				}
			}
		}
		ctx.Update()
		if err == nil && !p.teamWhatsAppSaved {
			p.pollTeamWhatsApp(ctx, generation, 0)
		}
	}()
}

func (p *serviceCatalogPage) pollTeamWhatsApp(ctx app.Context, generation uint64, attempts int) {
	ctx.After(4*time.Second, func(next app.Context) {
		if generation != p.teamWhatsAppGeneration || p.teamWhatsAppBusy || p.session == nil {
			return
		}
		if attempts >= 150 {
			p.teamWhatsAppNotice = "O pareamento não foi confirmado após 10 minutos. Atualize o estado e, se necessário, gere um novo QR ou código."
			next.Update()
			return
		}
		base, token := apiBaseURL(), p.session.AccessToken
		p.teamWhatsAppBusy = true
		go func() {
			result, err := sendTeamJSONResult(next, base+"/api/whatsapp", token, http.MethodPost, map[string]any{"acao": "status"})
			if p.session == nil || p.session.AccessToken != token || generation != p.teamWhatsAppGeneration {
				return
			}
			p.teamWhatsAppBusy = false
			if err == nil {
				p.teamWhatsAppStatus = result
				if qr := portalText(result["qr"]); qr != "" {
					p.teamWhatsAppQR = qr
				}
				if code := portalText(result["pairingCode"]); code != "" {
					p.teamWhatsAppPairing = code
				}
				if connected, _ := result["conectado"].(bool); connected {
					p.teamWhatsAppQR, p.teamWhatsAppPairing, p.teamWhatsAppSaved = "", "", true
					p.teamWhatsAppNotice = "Conta conectada e salva! Os disparos automáticos estão ativos."
					p.teamWhatsAppGeneration++
					notifyWhatsAppConnected()
				} else if whatsappPairingTerminal(portalText(result["estado"])) {
					p.teamWhatsAppQR, p.teamWhatsAppPairing = "", ""
					p.teamWhatsAppNotice = portalText(result["mensagem"])
				} else {
					if p.teamWhatsAppPairing != "" {
						p.teamWhatsAppNotice = "No celular, abra Aparelhos conectados → Conectar aparelho → Conectar com número de telefone e digite o código acima."
					} else {
						p.teamWhatsAppNotice = portalText(result["mensagem"])
					}
					p.pollTeamWhatsApp(next, generation, attempts+1)
				}
			} else {
				p.teamWhatsAppNotice = "Falha ao verificar conexão. Aguardando para tentar novamente..."
				p.pollTeamWhatsApp(next, generation, attempts+1)
			}
			next.Update()
		}()
	})
}

func whatsappPairingTerminal(state string) bool {
	switch state {
	case "logged_out", "temporarily_banned", "client_outdated", "stream_replaced", "connection_failed", "pairing_failed", "passkey_required", "passkey_processing", "passkey_confirmation", "credenciais_invalidas", "nao_encontrada":
		return true
	default:
		return false
	}
}

func (p *serviceCatalogPage) requestTeamWhatsAppDisconnect(mode string) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		p.teamWhatsAppConfirm = mode
		ctx.Update()
	}
}

func (p *serviceCatalogPage) cancelTeamWhatsAppDisconnect(ctx app.Context, event app.Event) {
	event.PreventDefault()
	p.teamWhatsAppConfirm = ""
	ctx.Update()
}

func (p *serviceCatalogPage) toggleTeamWhatsAppMeta(ctx app.Context, event app.Event) {
	event.PreventDefault()
	p.teamWhatsAppMetaExpanded = !p.teamWhatsAppMetaExpanded
	ctx.Update()
}

func (p *serviceCatalogPage) disconnectTeamWhatsApp(ctx app.Context, event app.Event) {
	event.PreventDefault()
	mode := p.teamWhatsAppConfirm
	if mode != "logout" && mode != "apagar" || p.session == nil || p.teamWhatsAppBusy {
		return
	}
	base, token := apiBaseURL(), p.session.AccessToken
	p.teamWhatsAppBusy = true
	go func() {
		result, err := sendTeamJSONResult(ctx, base+"/api/whatsapp", token, http.MethodPost, map[string]any{"acao": "desconectar", "modo": mode})
		if p.session == nil || p.session.AccessToken != token {
			return
		}
		p.teamWhatsAppBusy, p.teamWhatsAppConfirm = false, ""
		p.teamWhatsAppQR, p.teamWhatsAppPairing, p.teamWhatsAppSaved = "", "", false
		p.teamWhatsAppGeneration++
		if err != nil {
			p.teamWhatsAppNotice = "Falha ao desconectar o WhatsApp."
		} else {
			p.teamWhatsAppNotice = portalText(result["mensagem"])
			p.teamWhatsAppStatus = map[string]any{"conectado": false, "configurado": true}
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) teamSettingsDialog() app.UI {
	f := p.teamSettingsForm
	if f == nil {
		return app.Div()
	}
	field := func(label, value, placeholder string, target *string, inputType string) app.UI {
		return app.Label().Class("auth-field").Body(app.Text(label), app.Input().Type(inputType).Value(value).Placeholder(placeholder).OnChange(p.ValueTo(target)))
	}
	requiredField := func(label, value, placeholder string, target *string, inputType string) app.UI {
		return app.Label().Class("auth-field").Body(app.Text(label), app.Input().Type(inputType).Required(true).Value(value).Placeholder(placeholder).OnChange(p.ValueTo(target)))
	}
	pixTypes := []struct{ value, label string }{{"cpf", "CPF"}, {"cnpj", "CNPJ"}, {"email", "E-mail"}, {"telefone", "Telefone"}, {"aleatoria", "Aleatória"}}
	options := make([]app.UI, 0, len(pixTypes))
	for _, item := range pixTypes {
		options = append(options, app.Option().Value(item.value).Body(app.Text(item.label)))
	}
	return app.Div().Class("auth-backdrop").Body(app.Div().Class("auth-dialog").Body(
		app.Div().Class("auth-dialog__heading").Body(
			app.H2().Class("auth-dialog__title").Body(app.Text("Configurações do app")),
			app.Button().Class("auth-close").Type("button").Disabled(p.teamSettingsSaving).OnClick(p.closeTeamSettings).Body(app.Text("Fechar")),
		),
		app.P().Class("auth-dialog__intro").Body(app.Text("Perfil, preços, cobrança, mensagens e integrações do InovarApp.")),
		app.P().Class("team-calendar__hint").Body(app.Text("Perfil e mensagens podem ser alterados offline e sincronizam automaticamente. Senhas, tokens e conexão de provedores são enviados diretamente ao servidor e nunca ficam na fila local.")),
		func() app.UI {
			if p.teamSettingsNotice == "" {
				return app.Div()
			}
			return app.P().Class("portal-section__intro").Body(app.Text(p.teamSettingsNotice))
		}(),
		app.Form().Class("auth-form team-settings-form").OnSubmit(p.saveTeamSettings).Body(
			requiredField("Seu Nome Profissional *", f.Name, "Nome do técnico", &f.Name, "text"),
			field("Nome da Empresa / Fantasia", f.BusinessName, "Inovar Refrigeração", &f.BusinessName, "text"),
			field("CNPJ da empresa", f.CNPJ, "00.000.000/0000-00", &f.CNPJ, "text"),
			field("Endereço da empresa", f.Address, "Cidade / endereço", &f.Address, "text"),
			requiredField("WhatsApp de Contato", f.Phone, "DDD + telefone", &f.Phone, "tel"),
			app.H3().Class("team-section__heading-title").Body(app.Text("Dados de Cobrança & PIX")),
			field("Chave PIX", f.PIXKey, "CPF, CNPJ, e-mail, telefone ou chave aleatória", &f.PIXKey, "text"),
			app.Label().Class("auth-field").Body(app.Text("Tipo de Chave"), app.Select().Attr("value", f.PIXType).OnChange(p.ValueTo(&f.PIXType)).Body(options...)),
			app.Div().Class("team-settings-form__numbers").Body(
				field("Preço Médio Limpeza (R$)", f.DefaultPrice, "250", &f.DefaultPrice, "number"),
				field("Ciclo Padrão (meses)", f.ReturnMonths, "6", &f.ReturnMonths, "number"),
				field("Garantia padrão (dias)", f.WarrantyDays, "90", &f.WarrantyDays, "number"),
			),
			p.teamEmailSettings(),
			p.teamBackupSettings(),
			app.H3().Class("team-section__heading-title").Body(app.Text("Assinatura profissional")),
			app.Div().Class("team-settings-signature").Body(
				func() app.UI {
					if fSignature := p.teamProfile.Signature; fSignature != nil && *fSignature != "" {
						return app.Img().Src(*fSignature).Alt("Assinatura cadastrada")
					}
					return app.P().Body(app.Text("Assinatura profissional padrão"))
				}(),
				app.P().Body(app.Text("Aplicada automaticamente nas Ordens de Serviço e orçamentos.")),
			),
			p.teamWhatsAppSettings(),
			p.teamMessageCenter(),
			app.Div().Class("portal-budget__actions").Body(
				app.Button().Class("auth-link").Type("button").Disabled(p.teamSettingsSaving).OnClick(p.closeTeamSettings).Body(app.Text("Cancelar")),
				app.Button().Class("auth-submit").Type("submit").Disabled(p.teamSettingsSaving).Body(app.Text(func() string {
					if p.teamSettingsSaving {
						return "Salvando..."
					}
					return "Salvar e sincronizar"
				}())),
			),
		),
	))
}

func (p *serviceCatalogPage) teamEmailSettings() app.UI {
	statusClass := "team-settings-email__status team-settings-email__status--unconnected"
	googleStatus := "Conta Google ainda não conectada."
	if p.teamEmailGoogleConnected {
		statusClass = "team-settings-email__status team-settings-email__status--connected"
		googleStatus = "Gmail conectado: " + p.teamEmailGoogleAccount + ". Os envios automáticos usam esta conta."
	} else if !p.teamEmailGoogleReady {
		googleStatus = "OAuth do Google não está configurado no servidor."
	}
	if p.teamEmailNotice != "" {
		googleStatus += " " + p.teamEmailNotice
	}
	passwordStatus := "Senha de app ainda não configurada."
	if p.teamEmailPasswordSaved {
		passwordStatus = "Senha de app cadastrada e protegida no servidor."
	}
	return app.Div().Class("team-settings-email").Body(
		app.H3().Class("team-section__heading-title").Body(app.Text("E-mail do sistema · Google")),
		app.P().Class("portal-section__intro").Body(app.Text("Conecte o Gmail uma vez. Orçamentos, ordens de serviço e lembretes automáticos passam a enviar pela conta autorizada, sem senha de app.")),
		app.P().Class(statusClass).Role("status").Body(app.Text(googleStatus)),
		app.Div().Class("portal-budget__actions").Body(
			app.Button().Class("auth-submit").Type("button").Disabled(p.teamEmailBusy || !p.teamEmailGoogleReady).OnClick(p.connectTeamGoogleEmail).Body(app.Text(func() string {
				if p.teamEmailBusy {
					return "Aguarde..."
				}
				if p.teamEmailGoogleConnected {
					return "Trocar conta Google"
				}
				return "Conectar com Google"
			}())),
			app.Button().Class("auth-link").Type("button").Disabled(p.teamEmailBusy || !p.teamEmailGoogleConnected).OnClick(p.disconnectTeamGoogleEmail).Body(app.Text("Desconectar")),
			app.Button().Class("auth-link").Type("button").Disabled(p.teamEmailBusy || !(p.teamEmailGoogleConnected || p.teamEmailPasswordSaved)).OnClick(p.testTeamEmail).Body(app.Text(func() string {
				if p.teamEmailBusy {
					return "Aguarde..."
				}
				return "Enviar e-mail de teste"
			}())),
		),
		app.Details().Class("team-settings-email__manual").Body(
			app.Summary().Body(app.Text("Configurar SMTP manualmente (senha de app)")),
			app.P().Class("portal-section__intro").Body(app.Text(passwordStatus)),
			app.Label().Class("auth-field").Body(app.Text("Usuário Gmail"), app.Input().Type("email").Value(p.teamEmailUsername).Placeholder("voce@gmail.com").OnChange(p.ValueTo(&p.teamEmailUsername))),
			app.Label().Class("auth-field").Body(app.Text("Senha de app do Gmail"), app.Input().Type("password").Value(p.teamEmailPassword).Placeholder("Deixe vazio para manter a senha salva").AutoComplete(false).OnChange(p.ValueTo(&p.teamEmailPassword))),
			app.Div().Class("portal-budget__actions").Body(app.Button().Class("auth-link").Type("button").Disabled(p.teamEmailBusy).OnClick(p.saveTeamEmailSettings).Body(app.Text("Salvar credenciais SMTP"))),
		),
	)
}

func (p *serviceCatalogPage) teamBackupSettings() app.UI {
	return app.Div().Class("team-settings-email").Body(
		app.H3().Class("team-section__heading-title").Body(app.Text("Cópia de Segurança")),
		app.P().Class("portal-section__intro").Body(app.Text("Exporte os dados operacionais sincronizados em JSON. Também é possível restaurar o backup local antigo do React; clientes, aparelhos e histórico são convertidos e mesclados sem apagar dados ausentes.")),
		app.Div().Class("portal-budget__actions").Body(
			app.Button().Class("auth-submit").Type("button").Disabled(p.teamEmailBusy || p.session == nil).OnClick(p.exportTeamBackup).Body(app.Text("Exportar backup (JSON)")),
			app.Button().Class("auth-link").Type("button").Disabled(p.teamEmailBusy || p.session == nil).OnClick(p.restoreTeamBackup).Body(app.Text("Restaurar backup (JSON)")),
		),
	)
}

func (p *serviceCatalogPage) exportTeamBackup(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.session == nil || p.teamEmailBusy {
		return
	}
	token := p.session.AccessToken
	p.teamEmailBusy, p.teamEmailNotice = true, "Preparando backup..."
	ctx.Update()
	go func() {
		err := downloadTeamBackup(ctx, apiBaseURL()+"/api/backup", token)
		if p.session == nil || p.session.AccessToken != token {
			return
		}
		p.teamEmailBusy = false
		if err != nil {
			p.teamEmailNotice = "Não foi possível gerar o backup completo. Verifique sua conexão e tente novamente."
		} else {
			p.teamEmailNotice = "Backup exportado com sucesso. Guarde o arquivo em local seguro."
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) restoreTeamBackup(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.session == nil || p.teamEmailBusy {
		return
	}
	token := p.session.AccessToken
	p.teamEmailBusy, p.teamEmailNotice = true, "Aguardando seleção do arquivo de backup..."
	ctx.Update()
	go func() {
		message, err := selectAndRestoreTeamBackup(ctx, apiBaseURL()+"/api/backup", token)
		if p.session == nil || p.session.AccessToken != token {
			return
		}
		p.teamEmailBusy = false
		if err != nil {
			p.teamEmailNotice = err.Error()
		} else {
			p.teamEmailNotice = message
			p.loadTeamOperations(ctx)
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) teamWhatsAppSettings() app.UI {
	connected, _ := p.teamWhatsAppStatus["conectado"].(bool)
	configured, hasConfiguration := p.teamWhatsAppStatus["configurado"].(bool)
	canConnect := !hasConfiguration || configured
	state := "Conectar WhatsApp"
	if p.teamWhatsAppBusy {
		state = "Verificando conexão..."
	} else if connected {
		state = "WhatsApp conectado"
	} else if current := whatsappStateLabel(portalText(p.teamWhatsAppStatus["estado"])); current != "" {
		state = current
	}
	content := []app.UI{
		app.H3().Class("team-section__heading-title").Body(app.Text("Conexão com WhatsApp")),
		app.Div().Class("team-settings-whatsapp__heading").Body(app.Strong().Body(app.Text(state)), app.Button().Class("auth-link").Type("button").Disabled(p.teamWhatsAppBusy).OnClick(func(ctx app.Context, event app.Event) { event.PreventDefault(); p.refreshTeamWhatsApp(ctx) }).Body(app.Text("Atualizar"))),
		app.P().Class("team-settings-whatsapp__queue-policy").Body(app.Text("Desconectar não apaga a fila. Mensagens válidas ficam pendentes no Supabase e voltam a ser processadas quando uma conta for conectada; avisos com horário ou credenciais temporárias expiram para não serem enviados atrasados.")),
	}
	if p.teamWhatsAppNotice != "" {
		content = append(content, app.P().Class("portal-section__intro").Body(app.Text(p.teamWhatsAppNotice)))
	}
	if p.teamWhatsAppSaved {
		content = append(content, app.P().Class("team-settings-whatsapp__saved").Body(app.Text("Conta conectada e salva! Você pode desconectar esta sessão ou gerar um novo QR para outra conta.")))
		if account := portalText(p.teamWhatsAppStatus["conta"]); account != "" {
			content = append(content, app.P().Class("portal-section__intro").Body(app.Text("Conta WhatsApp vinculada: "+account)))
		}
	}
	if !connected && canConnect {
		content = append(content, app.Button().Class("auth-submit").Type("button").Disabled(p.teamWhatsAppBusy).OnClick(p.connectTeamWhatsApp).Body(app.Text("Conectar pelo QR Code")))
		content = append(content, app.Div().Class("team-settings-whatsapp__pairing-method").Body(
			app.Strong().Body(app.Text("Conectar sem QR Code")),
			app.P().Class("portal-section__intro").Body(app.Text("Gere um código e vincule este app pelo número de telefone no WhatsApp do celular.")),
			app.Label().Class("auth-field").Body(app.Text("Número da conta WhatsApp"), app.Input().Type("tel").Value(p.teamWhatsAppPhone).Placeholder("55 + DDD + telefone").OnChange(p.ValueTo(&p.teamWhatsAppPhone))),
			app.Button().Class("auth-submit team-settings-whatsapp__pair-button").Type("button").Disabled(p.teamWhatsAppBusy).OnClick(p.pairTeamWhatsApp).Body(app.Text(func() string {
				if p.teamWhatsAppBusy {
					return "Gerando código..."
				}
				return "Conectar por código"
			}())),
		))
	} else if !connected && !p.teamWhatsAppBusy {
		content = append(content, app.P().Class("team-settings-whatsapp__saved").Body(app.Text("Conexão indisponível até o serviço WhatsApp Go ser configurado no servidor.")))
	}
	if p.teamWhatsAppQR != "" || p.teamWhatsAppPairing != "" {
		if p.teamWhatsAppQR != "" {
			content = append(content, app.Img().Class("team-settings-whatsapp__qr").Src(p.teamWhatsAppQR).Alt("QR Code WhatsApp"))
		}
		if p.teamWhatsAppQR != "" {
			content = append(content, app.Ol().Class("team-settings-whatsapp__steps").Body(app.Li().Body(app.Text("Abra o WhatsApp no celular")), app.Li().Body(app.Text("Toque em Aparelhos conectados → Conectar aparelho")), app.Li().Body(app.Text("Aponte a câmera para este QR"))))
		}
		if p.teamWhatsAppPairing != "" {
			content = append(content,
				app.P().Class("team-settings-whatsapp__pairing-label").Body(app.Text("Digite este código no WhatsApp do celular:")),
				app.P().Class("team-settings-whatsapp__pairing").Body(app.Text(p.teamWhatsAppPairing)),
				app.Button().Class("auth-link").Type("button").OnClick(copyTextToClipboardNotice(p.teamWhatsAppPairing, func(ctx app.Context, copied bool) {
					if copied {
						p.teamWhatsAppNotice = "Código copiado. Digite-o em Aparelhos conectados → Conectar aparelho → Conectar com número de telefone no WhatsApp do celular."
					} else {
						p.teamWhatsAppNotice = "Não foi possível copiar. Digite o código exibido no WhatsApp do celular."
					}
					ctx.Update()
				})).Body(app.Text("Copiar código")),
				app.Ol().Class("team-settings-whatsapp__steps").Body(
					app.Li().Body(app.Text("Abra o WhatsApp no celular")),
					app.Li().Body(app.Text("Aparelhos conectados → Conectar aparelho")),
					app.Li().Body(app.Text("Escolha Conectar com número de telefone e informe o código acima")),
				),
			)
		}
		content = append(content, app.P().Class("portal-section__intro").Body(app.Text("Aguardando a confirmação do pareamento...")))
	}
	if connected {
		content = append(content, app.Div().Class("portal-budget__actions").Body(
			app.Button().Class("auth-link").Type("button").OnClick(p.requestTeamWhatsAppDisconnect("logout")).Body(app.Text("Desconectar")),
			app.Button().Class("auth-link").Type("button").OnClick(p.requestTeamWhatsAppDisconnect("apagar")).Body(app.Text("Conectar outra conta")),
		))
	}
	metaDetails := []app.UI{
		app.H3().Class("team-section__heading-title").Body(app.Text("Integração oficial da Meta · opção futura")),
		app.P().Class("portal-section__intro").Body(app.Text("A Meta Cloud API é uma alternativa futura ao provedor Go. Ela usa uma conta WhatsApp Business e o fluxo de cadastro da Meta, sem pareamento por QR ou código.")),
		app.Button().Class("auth-link").Type("button").Attr("aria-expanded", ariaBoolean(p.teamWhatsAppMetaExpanded)).OnClick(p.toggleTeamWhatsAppMeta).Body(app.Text(func() string {
			if p.teamWhatsAppMetaExpanded {
				return "Fechar preparação da Meta"
			}
			return "Preparar ativação da Meta"
		}())),
	}
	if p.teamWhatsAppMetaExpanded {
		metaDetails = append(metaDetails,
			app.P().Class("team-settings-whatsapp__saved").Body(app.Text("A Meta não está ativa. O WhatsApp Go continua selecionado.")),
			app.Ul().Class("team-settings-whatsapp__steps").Body(
				app.Li().Body(app.Text("Conectar a conta Business, WABA e número telefônico.")),
				app.Li().Body(app.Text("Configurar app, permissões e credenciais protegidas no servidor.")),
				app.Li().Body(app.Text("Ativar webhooks para mensagens recebidas e estados de envio, entrega e leitura.")),
				app.Li().Body(app.Text("Validar textos, documentos/PDF e modelos aprovados nos envios automáticos e manuais.")),
			),
			app.P().Class("portal-section__intro").Body(app.Text("A troca de provedor só deve ocorrer depois que todos esses fluxos estiverem configurados e validados.")),
		)
	}
	content = append(content, app.Div().Class("team-settings-email").Body(
		metaDetails...,
	))
	if p.teamWhatsAppConfirm != "" {
		message := "Desconectar o WhatsApp? A instância continua salva — você pode conectar o mesmo ou outro número depois."
		if p.teamWhatsAppConfirm == "apagar" {
			message = "Trocar o número conectado? O app usa uma única instância. A conta atual será desconectada e removida; depois, você poderá parear outro número no mesmo lugar."
		}
		content = append(content, app.Div().Class("auth-notice").Body(app.P().Body(app.Text(message)), app.Div().Class("portal-budget__actions").Body(app.Button().Class("auth-link").Type("button").Disabled(p.teamWhatsAppBusy).OnClick(p.cancelTeamWhatsAppDisconnect).Body(app.Text("Cancelar")), app.Button().Class("auth-submit").Type("button").Disabled(p.teamWhatsAppBusy).OnClick(p.disconnectTeamWhatsApp).Body(app.Text("Confirmar")))))
	}
	return app.Div().Class("team-settings-whatsapp").Body(content...)
}

func whatsappStateLabel(state string) string {
	switch state {
	case "connecting":
		return "Conectando WhatsApp..."
	case "nao_encontrada":
		return "Aguardando primeiro pareamento"
	case "indisponivel":
		return "Serviço WhatsApp indisponível"
	case "credenciais_invalidas":
		return "Credencial do serviço inválida"
	case "logged_out":
		return "Conta desconectada"
	case "temporarily_banned":
		return "Conta temporariamente bloqueada"
	case "client_outdated":
		return "Serviço precisa de atualização"
	case "stream_replaced":
		return "Outra conexão substituiu esta sessão"
	case "connection_failed", "pairing_failed":
		return "Falha na conexão"
	case "passkey_required", "passkey_processing", "passkey_confirmation":
		return "Confirmação de chave de acesso necessária"
	default:
		return ""
	}
}
