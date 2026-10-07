package webapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/adapter/cep"
	"inovarapp/core/domain"
)

type customerPortalEnvelope struct {
	OK    bool                       `json:"ok"`
	Data  *domain.CustomerPortalData `json:"data"`
	Error string                     `json:"error"`
}

type customerPortalServiceOption struct {
	Value string
	Label string
}

type customerProfileForm struct {
	WhatsApp, Address, Number, Neighborhood, City, PostalCode string
}

type customerMaintenanceReminder struct {
	Brand, Room, Date string
	Days              int
}

type customerPortalStatusNotification struct{ Title, Body string }

func customerPortalServiceStatusChanges(services []map[string]any, previous map[string]string) (map[string]string, []customerPortalStatusNotification) {
	current := make(map[string]string, len(services))
	notifications := make([]customerPortalStatusNotification, 0)
	labels := map[string]string{
		"AGENDADO":     "✅ Serviço AGENDADO pela Inovar",
		"EM_ANDAMENTO": "🔧 Serviço EM EXECUÇÃO hoje",
		"CONCLUIDO":    "🏁 Serviço CONCLUÍDO — garantia ativa!",
		"CANCELADO":    "⚠️ Serviço cancelado",
	}
	for _, service := range services {
		id, status := portalText(service["id"]), portalText(service["status"])
		if id == "" {
			continue
		}
		key := "svc:" + id
		current[key] = status
		if before, exists := previous[key]; exists && before != status {
			if title := labels[status]; title != "" {
				serviceType := customerPortalServiceTypeLabel(portalText(service["tipo"]))
				if serviceType == "" {
					serviceType = "Serviço"
				}
				notifications = append(notifications, customerPortalStatusNotification{Title: title, Body: serviceType})
			}
		}
	}
	return current, notifications
}

func (p *serviceCatalogPage) updateCustomerPortalStatuses(ctx app.Context, services []map[string]any) {
	current, notifications := customerPortalServiceStatusChanges(services, p.portalServiceStatuses)
	for _, notification := range notifications {
		p.loadLocalNotifications(ctx)
		p.notifications = addLocalNotification(p.notifications, notification.Title, notification.Body, time.Now())
		_ = ctx.LocalStorage().Set(notificationStorageKey, p.notifications)
		p.notificationToastTitle, p.notificationToastText = notification.Title, notification.Body
		notifyDevice(notification.Title, notification.Body)
	}
	p.portalServiceStatuses = current
}

func customerMaintenanceReminders(appliances []map[string]any, now time.Time, location *time.Location) []customerMaintenanceReminder {
	if location == nil {
		location = time.Local
	}
	today := time.Date(now.In(location).Year(), now.In(location).Month(), now.In(location).Day(), 0, 0, 0, 0, location)
	reminders := make([]customerMaintenanceReminder, 0)
	for _, appliance := range appliances {
		last := portalText(appliance["ultima_manutencao"])
		if len(last) != len("2006-01-02") {
			continue
		}
		maintained, err := time.ParseInLocation("2006-01-02", last, location)
		if err != nil || maintained.Format("2006-01-02") != last {
			continue
		}
		due := addCalendarMonthsClamped(maintained, 6)
		days := calendarDayDifference(today, due)
		if days > 7 {
			continue
		}
		reminders = append(reminders, customerMaintenanceReminder{
			Brand: portalText(appliance["marca"]), Room: portalText(appliance["ambiente"]),
			Date: due.Format("02/01"), Days: days,
		})
	}
	return reminders
}

func addCalendarMonthsClamped(date time.Time, months int) time.Time {
	year, month, day := date.Date()
	first := time.Date(year, month+time.Month(months), 1, 0, 0, 0, 0, date.Location())
	lastDay := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, date.Location()).Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(first.Year(), first.Month(), day, 0, 0, 0, 0, date.Location())
}

func calendarDayDifference(from, to time.Time) int {
	fromUTC := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	toUTC := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	return int(toUTC.Sub(fromUTC) / (24 * time.Hour))
}

func customerMaintenanceReminderText(reminders []customerMaintenanceReminder) string {
	if len(reminders) == 0 {
		return ""
	}
	if len(reminders) > 1 {
		return fmt.Sprintf("Os ciclos de limpeza de ar de %d aparelhos estão no prazo de limpeza de ar (ciclo de 6 meses).", len(reminders))
	}
	reminder := reminders[0]
	brand := reminder.Brand
	if brand == "" {
		brand = "ar-condicionado"
	}
	room := reminder.Room
	if room == "" {
		room = "aparelho"
	}
	when := fmt.Sprintf("vence em %d dia(s) (%s)", reminder.Days, reminder.Date)
	if reminder.Days < 0 {
		when = fmt.Sprintf("venceu há %d dia(s) (retornava em %s)", -reminder.Days, reminder.Date)
	}
	return fmt.Sprintf("O ciclo de limpeza de ar do seu %s (%s) %s. Manter o ciclo evita fungos, mau cheiro e maior consumo de energia.", brand, room, when)
}

func customerBusinessWhatsAppURL(phone string) string {
	digits := customerBusinessWhatsAppPhone(phone)
	return "https://wa.me/" + digits + "?text=Ol%C3%A1%20sou%20cliente%20da%20Inovar%20e%20gostaria%20de%20tirar%20uma%20d%C3%BAvida"
}

func customerBusinessWhatsAppPhone(phone string) string {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, phone)
	if len(digits) < 10 {
		digits = "5527999999999"
	} else if !strings.HasPrefix(digits, "55") {
		digits = "55" + digits
	}
	return digits
}

func customerServiceRequestPayload(kind, applianceID, date, problem, notes string) map[string]any {
	payload := map[string]any{"tipo": strings.TrimSpace(kind)}
	if strings.TrimSpace(applianceID) != "" {
		payload["aparelho_id"] = applianceID
	}
	if strings.TrimSpace(date) != "" {
		payload["data_agendamento"] = date
	}
	if strings.TrimSpace(problem) != "" {
		payload["problema"] = strings.TrimSpace(problem)
	}
	if strings.TrimSpace(notes) != "" {
		payload["observacoes"] = strings.TrimSpace(notes)
	}
	return payload
}

func selectedCustomerAppliance(currentID string, appliances []map[string]any) string {
	for _, appliance := range appliances {
		id := portalText(appliance["id"])
		if id == currentID {
			return currentID
		}
	}
	if currentID == "" && len(appliances) > 0 {
		return portalText(appliances[0]["id"])
	}
	return ""
}

func customerPortalServiceOptions() []customerPortalServiceOption {
	return []customerPortalServiceOption{
		{Value: "LIMPEZA", Label: "Limpeza de Ar"},
		{Value: "MANUTENCAO_PREVENTIVA", Label: "Manutenção Preventiva"},
		{Value: "MANUTENCAO_CORRETIVA", Label: "Conserto / Reparo (Não está gelando / Barulho)"},
		{Value: "RECARGA_GAS", Label: "Carga ou Teste de Gás Refrigerante"},
		{Value: "INSTALACAO", Label: "Instalação / Desinstalação"},
		{Value: "AVALIACAO", Label: "Visita Técnica de Avaliação"},
	}
}

func (p *serviceCatalogPage) loadCustomerPortal(ctx app.Context) {
	if p.caller != nil && p.session != nil && (p.caller.Role == domain.RoleAdmin || p.caller.Role == domain.RoleTechnician) {
		p.loadTeamProfilePhoto(ctx)
	}
	if p.caller != nil && p.session != nil && p.caller.Role == domain.RoleCustomer {
		p.loadCustomerProfilePhoto(ctx)
		p.loadCustomerBusinessContact(ctx)
	}
	if p.caller == nil || p.session == nil || p.caller.Role != domain.RoleCustomer || p.portalLoading || p.portalLoadedFor == p.caller.UserID {
		return
	}
	userID, token := p.caller.UserID, p.session.AccessToken
	role := p.caller.Role
	p.portalLoading = true
	p.portalError = ""
	go func() {
		pageURL := app.Window().URL()
		if pageURL == nil || pageURL.Scheme == "" || pageURL.Host == "" {
			p.portalError = "Não foi possível carregar seus atendimentos."
			p.portalLoading = false
			ctx.Update()
			p.scheduleCustomerPortalRefresh(ctx, userID)
			return
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBaseURL()+"/api/portal/cliente", nil)
		if err != nil {
			p.portalError = "Não foi possível carregar seus atendimentos."
			p.portalLoading = false
			ctx.Update()
			p.scheduleCustomerPortalRefresh(ctx, userID)
			return
		}
		request.Header.Set("Authorization", "Bearer "+token)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			if snapshot, cacheErr := loadOfflineSnapshot(userID); cacheErr == nil && snapshot.Role == role && snapshot.PortalData != nil {
				p.portalData, p.portalLoadedFor = snapshot.PortalData, userID
				p.portalError, p.offlineMode = "", true
			} else {
				p.portalError = "Não foi possível carregar seus atendimentos. Verifique sua conexão."
			}
			p.portalLoading = false
			ctx.Update()
			p.scheduleCustomerPortalRefresh(ctx, userID)
			return
		}
		defer response.Body.Close()
		var result customerPortalEnvelope
		if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&result); err != nil || response.StatusCode != http.StatusOK || !result.OK || result.Data == nil {
			p.portalError = result.Error
			if p.portalError == "" {
				p.portalError = "Não foi possível carregar seus atendimentos."
			}
			p.portalLoading = false
			ctx.Update()
			p.scheduleCustomerPortalRefresh(ctx, userID)
			return
		}
		p.portalData = result.Data
		p.updateCustomerPortalStatuses(ctx, result.Data.Services)
		p.portalLoadedFor = userID
		p.offlineMode = false
		cachePortalSnapshot(userID, role, result.Data)
		p.portalLoading = false
		ctx.Update()
		p.scheduleCustomerPortalRefresh(ctx, userID)
	}()
}

func (p *serviceCatalogPage) loadCustomerBusinessContact(ctx app.Context) {
	if p.caller == nil || p.session == nil || p.caller.Role != domain.RoleCustomer || p.portalBusinessWhatsLoading || p.portalBusinessWhatsLoaded == p.caller.UserID {
		return
	}
	userID, token := p.caller.UserID, p.session.AccessToken
	p.portalBusinessWhatsLoading = true
	go func() {
		endpoint := apiBaseURL() + "/api/configuracoes"
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err == nil {
			request.Header.Set("Authorization", "Bearer "+token)
			var response *http.Response
			response, err = http.DefaultClient.Do(request)
			if err == nil {
				defer response.Body.Close()
				var result struct {
					Config map[string]json.RawMessage `json:"config"`
				}
				if response.StatusCode == http.StatusOK && json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result) == nil {
					var phone string
					_ = json.Unmarshal(result.Config["phone"], &phone)
					p.portalBusinessWhats = customerBusinessWhatsAppPhone(phone)
				}
			}
		}
		p.portalBusinessWhatsLoading = false
		p.portalBusinessWhatsLoaded = userID
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) loadCustomerProfilePhoto(ctx app.Context) {
	if p.caller == nil || p.session == nil || p.caller.Role != domain.RoleCustomer || p.portalProfilePhotoBusy || p.portalProfilePhotoLoaded == p.caller.UserID {
		return
	}
	userID, token := p.caller.UserID, p.session.AccessToken
	p.portalProfilePhotoBusy = true
	go func() {
		result, err := requestCustomerProfilePhoto(ctx, token, "fotoperfil-url", "")
		p.portalProfilePhotoBusy = false
		if err == nil {
			p.portalProfilePhotoURL = cacheBustCustomerProfilePhoto(portalText(result["url"]))
			p.portalProfilePhotoLoaded = userID
		}
		ctx.Update()
	}()
}

func requestCustomerProfilePhoto(ctx app.Context, token, action, dataURL string) (map[string]any, error) {
	endpoint := apiBaseURL() + "/api/documentos"
	if endpoint == "/api/documentos" {
		return nil, fmt.Errorf("API de documentos indisponível")
	}
	payload := map[string]string{"acao": action}
	if dataURL != "" {
		payload["base64"] = dataURL
	}
	body, _ := json.Marshal(payload)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	result := map[string]any{}
	_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if message := portalText(result["error"]); message != "" {
			return result, fmt.Errorf("%s", message)
		}
		return result, fmt.Errorf("Não foi possível atualizar a foto de perfil.")
	}
	return result, nil
}

func cacheBustCustomerProfilePhoto(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	separator := "?"
	if strings.Contains(rawURL, "?") {
		separator = "&"
	}
	return rawURL + separator + "v=" + strconv.FormatInt(time.Now().UnixMilli(), 10)
}

func (p *serviceCatalogPage) changeCustomerProfilePhoto(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.portalProfilePhotoBusy || p.session == nil || p.caller == nil {
		return
	}
	selected := startCustomerProfilePhotoPicker()
	p.portalProfilePhotoBusy = true
	p.portalProfilePhotoError = ""
	p.portalProfilePhotoNotice = ""
	ctx.Update()
	userID, token := p.caller.UserID, p.session.AccessToken
	go func() {
		photo := <-selected
		if photo == nil {
			p.portalProfilePhotoBusy = false
			ctx.Update()
			return
		}
		if photo.Error != "" {
			p.portalProfilePhotoBusy = false
			p.portalProfilePhotoError = photo.Error
			ctx.Update()
			return
		}
		previous := p.portalProfilePhotoURL
		p.portalProfilePhotoURL = photo.DataURL
		ctx.Update()
		result, err := requestCustomerProfilePhoto(ctx, token, "fotoperfil", photo.DataURL)
		if err == nil {
			photoResult, photoErr := requestCustomerProfilePhoto(ctx, token, "fotoperfil-url", "")
			if photoErr == nil {
				p.portalProfilePhotoURL = cacheBustCustomerProfilePhoto(portalText(photoResult["url"]))
			}
			p.portalProfilePhotoLoaded = userID
			p.portalProfilePhotoNotice = "Foto de perfil atualizada!"
		} else {
			p.portalProfilePhotoURL = previous
			p.portalProfilePhotoError = err.Error()
			if result == nil {
				p.portalProfilePhotoError = "Sem conexão para enviar a foto."
			} else if message := portalText(result["error"]); message != "" {
				p.portalProfilePhotoError = message
			}
		}
		p.portalProfilePhotoBusy = false
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) confirmRemoveCustomerProfilePhoto(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.portalProfilePhotoBusy || p.session == nil {
		return
	}
	p.portalProfilePhotoBusy = true
	p.portalProfilePhotoError = ""
	p.portalProfilePhotoNotice = ""
	token := p.session.AccessToken
	go func() {
		_, err := requestCustomerProfilePhoto(ctx, token, "fotoperfil-remover", "")
		p.portalProfilePhotoBusy = false
		p.portalProfilePhotoConfirm = false
		if err != nil {
			p.portalProfilePhotoError = err.Error()
		} else {
			p.portalProfilePhotoURL = ""
			p.portalProfilePhotoLoaded = p.caller.UserID
			p.portalProfilePhotoNotice = "Foto de perfil removida."
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) customerPortalProfileHeader() app.UI {
	name := "Cliente"
	if p.portalData != nil && p.portalData.Customer != nil {
		if stored := strings.TrimSpace(portalText(p.portalData.Customer["nome"])); stored != "" {
			name = stored
		}
	} else if p.session != nil && p.session.User != nil && p.session.User.Email != "" {
		name = strings.SplitN(p.session.User.Email, "@", 2)[0]
	}
	firstName := strings.Fields(name)[0]
	initial := "?"
	if runes := []rune(firstName); len(runes) > 0 {
		initial = strings.ToUpper(string(runes[0]))
	}
	var photoContent app.UI = app.Span().Class("portal-profile__initial").Body(app.Text(initial))
	if p.portalProfilePhotoBusy {
		photoContent = app.Span().Class("portal-profile__spinner").Body(app.Text("…"))
	} else if p.portalProfilePhotoURL != "" {
		photoContent = app.Img().Class("portal-profile__image").Src(p.portalProfilePhotoURL).Alt("Foto de perfil")
	}
	items := []app.UI{
		app.Div().Class("portal-profile__photo-wrap").Body(
			app.Button().Class("portal-profile__photo").Type("button").Disabled(p.portalProfilePhotoBusy).Attr("title", "Trocar foto de perfil").OnClick(p.changeCustomerProfilePhoto).Body(photoContent),
		),
		app.Div().Class("portal-profile__identity").Body(
			app.Span().Class("portal-profile__eyebrow").Body(app.Text("PORTAL DO CLIENTE • INOVAR REFRIGERAÇÃO")),
			app.H2().Class("portal-profile__title").Body(app.Text("Olá, "+firstName+"!")),
			app.P().Class("portal-profile__intro").Body(app.Text("Acompanhe a saúde dos seus aparelhos de ar-condicionado, solicite limpeza de ar e consulte suas garantias.")),
		),
	}
	if p.portalProfilePhotoURL != "" {
		items[0] = app.Div().Class("portal-profile__photo-wrap").Body(
			app.Button().Class("portal-profile__photo").Type("button").Disabled(p.portalProfilePhotoBusy).Attr("title", "Trocar foto de perfil").OnClick(p.changeCustomerProfilePhoto).Body(photoContent),
			app.Button().Class("portal-profile__remove").Type("button").Disabled(p.portalProfilePhotoBusy).Attr("title", "Remover foto").OnClick(func(ctx app.Context, event app.Event) { event.PreventDefault(); p.portalProfilePhotoConfirm = true }).Body(app.Text("×")),
		)
	}
	if p.portalProfilePhotoError != "" {
		items = append(items, app.P().Class("portal-profile__error").Role("alert").Body(app.Text(p.portalProfilePhotoError)))
	}
	if p.portalProfilePhotoNotice != "" {
		items = append(items, app.P().Class("portal-profile__notice").Attr("role", "status").Body(app.Text(p.portalProfilePhotoNotice)))
	}
	if p.portalProfileSaved {
		items = append(items, app.P().Class("portal-profile__notice").Role("status").Body(app.Text("Cadastro atualizado com sucesso!")))
	}
	if p.portalPasswordNotice != "" && !p.portalPasswordOpen {
		items = append(items, app.P().Class("portal-profile__notice").Role("status").Body(app.Text(p.portalPasswordNotice)))
	}
	if p.portalProfilePhotoConfirm {
		items = append(items, app.Div().Class("auth-backdrop").Body(app.Div().Class("auth-dialog").Role("dialog").Body(
			app.H3().Class("auth-dialog__title").Body(app.Text("Remover foto de perfil?")),
			app.P().Class("auth-dialog__intro").Body(app.Text("Sua foto será removida do perfil.")),
			app.Div().Class("portal-budget__actions").Body(
				app.Button().Class("auth-link").Type("button").OnClick(func(ctx app.Context, event app.Event) { event.PreventDefault(); p.portalProfilePhotoConfirm = false }).Body(app.Text("Cancelar")),
				app.Button().Class("auth-submit").Type("button").Disabled(p.portalProfilePhotoBusy).OnClick(p.confirmRemoveCustomerProfilePhoto).Body(app.Text("Remover foto")),
			),
		)))
	}
	items = append(items,
		app.Div().Class("portal-budget__actions").Body(
			app.Button().Class("auth-submit").Type("button").OnClick(p.openCustomerRequest).Body(app.Text("Solicitar Atendimento")),
			app.A().Class("auth-submit").Href(customerBusinessWhatsAppURL(p.portalBusinessWhats)).Target("_blank").Rel("noopener noreferrer").Body(app.Text("WhatsApp Inovar")),
			app.Button().Class("auth-link").Type("button").OnClick(p.openCustomerProfile).Body(app.Text("Meus Dados")),
			app.Button().Class("auth-link").Type("button").OnClick(p.openCustomerPassword).Body(app.Text("Alterar Senha")),
		),
	)
	if p.portalApplianceSaved {
		items = append(items, app.P().Class("portal-profile__notice").Role("status").Body(app.Text("Aparelho cadastrado com sucesso!")))
	}
	if p.portalRequestFeedback != "" {
		tone := "portal-profile__notice"
		if strings.HasPrefix(p.portalRequestFeedback, "Não foi possível") || strings.HasPrefix(p.portalRequestFeedback, "Informe ") {
			tone = "portal-profile__error"
		}
		items = append(items, app.P().Class(tone).Role("status").Body(app.Text(p.portalRequestFeedback)))
	}
	return app.Div().Class("portal-profile").Body(items...)
}

func (p *serviceCatalogPage) openCustomerPassword(ctx app.Context, event app.Event) {
	event.PreventDefault()
	p.portalPasswordOpen, p.portalPasswordError, p.portalPasswordNotice = true, "", ""
	p.portalPassword, p.portalPasswordConfirm = "", ""
}

func (p *serviceCatalogPage) closeCustomerPassword(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if !p.portalPasswordBusy {
		p.portalPasswordOpen = false
	}
}

func (p *serviceCatalogPage) customerPasswordDialog() app.UI {
	items := []app.UI{app.Div().Class("auth-dialog__heading").Body(app.H3().Class("auth-dialog__title").Body(app.Text("Alterar Minha Senha")), app.Button().Class("auth-close").Type("button").OnClick(p.closeCustomerPassword).Body(app.Text("✕")))}
	if p.portalPasswordNotice != "" {
		items = append(items, app.P().Class("auth-message auth-message--success").Body(app.Text(p.portalPasswordNotice)))
	}
	if p.portalPasswordError != "" {
		items = append(items, app.P().Class("auth-message auth-message--error").Role("alert").Body(app.Text(p.portalPasswordError)))
	}
	items = append(items,
		p.passwordField("Nova senha (mín. 6 caracteres)", "new-password", "••••••••", &p.portalPassword, &p.portalPasswordVisible),
		p.passwordField("Confirmar nova senha", "new-password", "••••••••", &p.portalPasswordConfirm, &p.portalPasswordConfirmVisible),
	)
	if p.portalPassword != "" && p.portalPasswordConfirm != "" && p.portalPassword != p.portalPasswordConfirm {
		items = append(items, app.P().Class("auth-message auth-message--error").Body(app.Text("As senhas não coincidem.")))
	}
	items = append(items, app.Div().Class("portal-budget__actions").Body(
		app.Button().Class("auth-link").Type("button").Disabled(p.portalPasswordBusy).OnClick(p.closeCustomerPassword).Body(app.Text("Cancelar")),
		app.Button().Class("auth-submit").Type("button").Disabled(p.portalPasswordBusy || len([]rune(p.portalPassword)) < 6 || p.portalPassword != p.portalPasswordConfirm).OnClick(p.saveCustomerPassword).Body(app.Text(func() string {
			if p.portalPasswordBusy {
				return "Salvando..."
			}
			return "Salvar Senha"
		}())),
	))
	return app.Div().Class("auth-backdrop").Body(app.Div().Class("auth-dialog").Role("dialog").Body(items...))
}

func (p *serviceCatalogPage) saveCustomerPassword(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.portalPasswordBusy || p.session == nil {
		return
	}
	if len([]rune(p.portalPassword)) < 6 {
		p.portalPasswordError = "A senha deve ter pelo menos 6 caracteres."
		return
	}
	if p.portalPassword != p.portalPasswordConfirm {
		p.portalPasswordError = "As senhas não coincidem."
		return
	}
	p.portalPasswordBusy, p.portalPasswordError = true, ""
	password := p.portalPassword
	go func() {
		client, err := publicSupabaseClient()
		if err == nil {
			err = client.UpdatePassword(ctx, p.session.AccessToken, password)
		}
		p.portalPasswordBusy = false
		if err != nil {
			p.portalPasswordError = err.Error()
			if p.portalPasswordError == "" {
				p.portalPasswordError = "Não foi possível alterar a senha."
			}
		} else {
			p.portalPasswordNotice = "Senha alterada com sucesso! Use-a no próximo login."
			p.portalPassword = ""
			p.portalPasswordConfirm = ""
			p.portalPasswordOpen = false
			p.portalRequestFeedback = ""
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) openCustomerProfile(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.portalData == nil || p.portalData.Customer == nil {
		return
	}
	c := p.portalData.Customer
	p.portalProfileForm = customerProfileForm{
		WhatsApp: portalText(c["whatsapp"]), Address: portalText(c["endereco"]), Number: portalText(c["numero"]), PostalCode: portalText(c["cep"]),
		Neighborhood: portalText(c["bairro"]), City: portalText(c["cidade"]),
	}
	p.portalProfileOpen, p.portalProfileNotice = true, ""
	p.portalProfileSaved = false
	p.portalProfileCEPLooking = false
	p.portalProfileCEPMessage = ""
	p.portalProfileCEPSuggestion = nil
	p.portalProfileCEPGeneration++
}

func (p *serviceCatalogPage) customerProfileDialog() app.UI {
	f := &p.portalProfileForm
	field := func(label, value string, target *string, placeholder string) app.UI {
		return app.Label().Class("auth-field").Body(app.Text(label), app.Input().Type("text").Value(value).Placeholder(placeholder).OnChange(p.ValueTo(target)))
	}
	return app.Div().Class("auth-backdrop").Body(app.Div().Class("auth-dialog").Role("dialog").Body(
		app.Div().Class("auth-dialog__heading").Body(app.H3().Class("auth-dialog__title").Body(app.Text("Meus Dados de Cadastro")), app.Button().Class("auth-close").Type("button").Disabled(p.portalProfileBusy).OnClick(p.closeCustomerProfile).Body(app.Text("Fechar"))),
		app.Div().Class("auth-form customer-profile-form").Body(
			field("WhatsApp", f.WhatsApp, &f.WhatsApp, "(27) 99999-9999"),
			customerProfilePostalCodeField(p),
			app.Div().Class("customer-profile__address-row").Body(field("Endereço", f.Address, &f.Address, "Rua / Av."), field("Número", f.Number, &f.Number, "123")),
			app.Div().Class("customer-profile__location-row").Body(field("Bairro", f.Neighborhood, &f.Neighborhood, "Bairro"), field("Cidade", f.City, &f.City, "Cidade")),
			func() app.UI {
				if p.portalProfileNotice == "" {
					return app.Div()
				}
				tone := "portal-section__error"
				if p.portalProfileSaved {
					tone = "portal-profile__notice"
				}
				return app.P().Class(tone).Attr("role", "alert").Body(app.Text(p.portalProfileNotice))
			}(),
			app.Div().Class("portal-budget__actions").Body(
				app.Button().Class("auth-link").Type("button").Disabled(p.portalProfileBusy).OnClick(p.closeCustomerProfile).Body(app.Text("Cancelar")),
				app.Button().Class("auth-submit").Type("button").Disabled(p.portalProfileBusy).OnClick(p.saveCustomerProfile).Body(app.Text(func() string {
					if p.portalProfileBusy {
						return "Salvando..."
					}
					return "Salvar Dados"
				}())),
			),
		),
	))
}

func maskCustomerProfileCEP(value string) string { return cep.Mask(value) }

func (p *serviceCatalogPage) maskCustomerProfilePostalCode(ctx app.Context, event app.Event) {
	p.portalProfileForm.PostalCode = maskCustomerProfileCEP(event.Value.Get("target").Get("value").String())
	p.portalProfileCEPGeneration++
	p.portalProfileCEPSuggestion = nil
	p.portalProfileCEPLooking = false
	digits := cep.Digits(p.portalProfileForm.PostalCode)
	switch {
	case digits == "":
		p.portalProfileCEPMessage = ""
	case len(digits) < 8:
		p.portalProfileCEPMessage = fmt.Sprintf("Digite mais %d dígitos para localizar o endereço.", 8-len(digits))
	default:
		p.portalProfileCEPMessage = "Buscando endereço pelo CEP..."
	}
	ctx.Update()
	if len(digits) == 8 {
		p.lookupCustomerProfilePostalCode(ctx, app.Event{})
	}
}

func (p *serviceCatalogPage) lookupCustomerProfilePostalCode(ctx app.Context, event app.Event) {
	digits := cep.Digits(p.portalProfileForm.PostalCode)
	p.portalProfileCEPGeneration++
	generation := p.portalProfileCEPGeneration
	if len(digits) != 8 {
		p.portalProfileCEPLooking = false
		return
	}
	p.portalProfileCEPLooking = true
	p.portalProfileCEPMessage = "Buscando endereço pelo CEP..."
	ctx.Update()
	go func() {
		address, err := cep.Lookup(ctx, digits, nil, "")
		if generation != p.portalProfileCEPGeneration || cep.Digits(p.portalProfileForm.PostalCode) != digits {
			return
		}
		p.portalProfileCEPLooking = false
		if err != nil {
			p.portalProfileCEPSuggestion = nil
			p.portalProfileCEPMessage = "Não encontrei esse CEP. Confira os números ou preencha o endereço manualmente."
			ctx.Update()
			return
		}
		p.portalProfileCEPSuggestion = &address
		if address.Street != "" {
			p.portalProfileForm.Address = address.Street
		}
		if address.Neighborhood != "" {
			p.portalProfileForm.Neighborhood = address.Neighborhood
		}
		if address.City != "" {
			p.portalProfileForm.City = address.City
		}
		p.portalProfileCEPMessage = "Endereço localizado. Toque na sugestão abaixo para selecionar."
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) selectCustomerProfilePostalSuggestion(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if address := p.portalProfileCEPSuggestion; address != nil {
		if address.Street != "" {
			p.portalProfileForm.Address = address.Street
		}
		if address.Neighborhood != "" {
			p.portalProfileForm.Neighborhood = address.Neighborhood
		}
		if address.City != "" {
			p.portalProfileForm.City = address.City
		}
		p.portalProfileCEPMessage = "Sugestão de endereço selecionada; os campos continuam editáveis."
		ctx.Update()
	}
}

func customerProfileCEPLoading(loading bool) []app.UI {
	if !loading {
		return nil
	}
	return []app.UI{app.Span().Class("auth-cep-spinner").Attr("role", "status").Attr("aria-label", "Buscando CEP").Body(app.Text(" "))}
}

func customerProfilePostalCodeField(p *serviceCatalogPage) app.UI {
	children := []app.UI{
		app.Text("CEP (preenche o endereço automático)"),
		app.Div().Class("auth-cep-input-wrap").Body(append(
			[]app.UI{app.Input().Type("text").Attr("inputmode", "numeric").Value(p.portalProfileForm.PostalCode).Placeholder("Ex: 29060-270").OnInput(p.maskCustomerProfilePostalCode)},
			customerProfileCEPLoading(p.portalProfileCEPLooking)...,
		)...),
	}
	return app.Div().Class("auth-field customer-profile__cep-field").Body(append(children,
		cepSearchFeedback(p.portalProfileCEPMessage),
		cepAddressSuggestion(p.portalProfileCEPSuggestion, p.selectCustomerProfilePostalSuggestion),
	)...)
}

func TestMaskCustomerProfileCEPMatchesLegacyMaskAndLimit(t *testing.T) {
	for input, want := range map[string]string{"29060270": "29060-270", "29.a060-27099": "29060-270", "12345": "12345"} {
		if got := maskCustomerProfileCEP(input); got != want {
			t.Errorf("maskCustomerProfileCEP(%q)=%q want %q", input, got, want)
		}
	}
}

func (p *serviceCatalogPage) closeCustomerProfile(ctx app.Context, event app.Event) {
	event.PreventDefault()
	p.portalProfileCEPGeneration++
	if !p.portalProfileBusy {
		p.portalProfileOpen = false
	}
}

func (p *serviceCatalogPage) saveCustomerProfile(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.portalProfileBusy || p.session == nil {
		return
	}
	f := p.portalProfileForm
	body, _ := json.Marshal(map[string]string{"whatsapp": f.WhatsApp, "endereco": f.Address, "numero": f.Number, "bairro": f.Neighborhood, "cidade": f.City})
	p.portalProfileBusy, p.portalProfileNotice = true, ""
	go func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, apiBaseURL()+"/api/cliente/perfil", bytes.NewReader(body))
		if err == nil {
			req.Header.Set("Authorization", "Bearer "+p.session.AccessToken)
			req.Header.Set("Content-Type", "application/json")
			var response *http.Response
			response, err = http.DefaultClient.Do(req)
			if err == nil {
				defer response.Body.Close()
				if response.StatusCode < 200 || response.StatusCode >= 300 {
					var failure map[string]string
					_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&failure)
					p.portalProfileNotice = failure["error"]
					if p.portalProfileNotice == "" {
						p.portalProfileNotice = "Erro ao salvar."
					}
					err = fmt.Errorf("profile update rejected")
				}
			}
		}
		p.portalProfileBusy = false
		if err == nil && p.portalData != nil && p.portalData.Customer != nil {
			p.portalData.Customer["whatsapp"], p.portalData.Customer["endereco"], p.portalData.Customer["numero"], p.portalData.Customer["bairro"], p.portalData.Customer["cidade"] = strings.TrimSpace(f.WhatsApp), strings.TrimSpace(f.Address), strings.TrimSpace(f.Number), strings.TrimSpace(f.Neighborhood), strings.TrimSpace(f.City)
			p.portalProfileOpen = false
			p.portalProfileSaved = true
			p.portalProfileNotice = "Cadastro atualizado com sucesso!"
		}
		if err != nil && p.portalProfileNotice == "" {
			p.portalProfileNotice = "Não foi possível salvar. Verifique sua conexão."
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) submitPortalRequest(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.portalRequestBusy || p.session == nil {
		return
	}
	typeName := strings.TrimSpace(p.portalRequestType)
	if typeName == "" {
		p.portalRequestFeedback = "Informe o tipo de serviço."
		return
	}
	pageURL := app.Window().URL()
	if pageURL == nil || pageURL.Scheme == "" || pageURL.Host == "" {
		return
	}
	p.portalRequestBusy, p.portalRequestFeedback = true, ""
	payload := customerServiceRequestPayload(typeName, p.portalRequestAppliance, p.portalRequestDate, p.portalRequestProblem, p.portalRequestNotes)
	body, _ := json.Marshal(payload)
	endpoint, token := apiBaseURL()+"/api/cliente/servicos", p.session.AccessToken
	go func() {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err == nil {
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set("Content-Type", "application/json")
			var response *http.Response
			response, err = http.DefaultClient.Do(request)
			if err == nil {
				defer response.Body.Close()
				if response.StatusCode < 200 || response.StatusCode >= 300 {
					var failure map[string]string
					_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&failure)
					if failure["error"] != "" {
						p.portalRequestFeedback = failure["error"]
					} else {
						p.portalRequestFeedback = "Não foi possível enviar sua solicitação."
					}
					err = io.ErrUnexpectedEOF
				}
			}
		}
		if err == nil {
			p.portalRequestFeedback = "Solicitação de serviço enviada com sucesso! Nossa equipe entrará em contato para confirmar."
			p.portalRequestType, p.portalRequestProblem, p.portalRequestNotes, p.portalRequestDate, p.portalRequestAppliance = "LIMPEZA", "", "", "", ""
			p.portalRequestOpen = false
			p.portalLoadedFor = ""
		}
		p.portalRequestBusy = false
		if err == nil {
			p.portalData = nil
			p.loadCustomerPortal(ctx)
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) submitPortalAppliance(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.portalApplianceBusy || p.session == nil {
		return
	}
	brand, model := strings.TrimSpace(p.portalApplianceBrand), strings.TrimSpace(p.portalApplianceModel)
	if brand == "" {
		brand = "LG"
	}
	btus := p.portalApplianceBTUs
	if btus == "" {
		btus = "12000"
	}
	applianceType := p.portalApplianceType
	if applianceType == "" {
		applianceType = "Split Hi-Wall"
	}
	room := strings.TrimSpace(p.portalApplianceRoom)
	if room == "" {
		room = "Sala"
	}
	capacity, err := strconv.Atoi(btus)
	if err != nil {
		p.portalApplianceFeedback = "Selecione uma capacidade válida."
		return
	}
	pageURL := app.Window().URL()
	if pageURL == nil || pageURL.Scheme == "" || pageURL.Host == "" {
		p.portalApplianceFeedback = "Não foi possível salvar o aparelho."
		return
	}
	payload := map[string]any{
		"marca": brand, "modelo": model, "btus": capacity, "tipo": applianceType,
		"ambiente": room, "ultima_manutencao": customerPortalMaintenanceDate(time.Now()),
	}
	body, _ := json.Marshal(payload)
	endpoint, token := apiBaseURL()+"/api/cliente/aparelhos", p.session.AccessToken
	p.portalApplianceBusy, p.portalApplianceFeedback = true, ""
	go func() {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err == nil {
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set("Content-Type", "application/json")
			var response *http.Response
			response, err = http.DefaultClient.Do(request)
			if err == nil {
				defer response.Body.Close()
				if response.StatusCode < 200 || response.StatusCode >= 300 {
					var failure map[string]string
					_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&failure)
					p.portalApplianceFeedback = failure["error"]
					if p.portalApplianceFeedback == "" {
						p.portalApplianceFeedback = "Não foi possível salvar o aparelho."
					}
					err = io.ErrUnexpectedEOF
				}
			}
		}
		p.portalApplianceBusy = false
		if err == nil {
			p.portalApplianceFeedback = "Aparelho cadastrado com sucesso!"
			p.portalApplianceSaved = true
			p.portalApplianceModel = ""
			p.portalApplianceOpen = false
			p.portalData = nil
			p.portalLoadedFor = ""
			p.loadCustomerPortal(ctx)
		} else if p.portalApplianceFeedback == "" {
			p.portalApplianceFeedback = "Não foi possível salvar o aparelho. Verifique sua conexão e tente novamente."
		}
		ctx.Update()
	}()
}

func customerPortalMaintenanceDate(now time.Time) string {
	return now.UTC().Format("2006-01-02")
}

func (p *serviceCatalogPage) scheduleCustomerPortalRefresh(ctx app.Context, userID string) {
	ctx.After(45*time.Second, func(next app.Context) {
		if p.caller == nil || p.session == nil || p.caller.Role != domain.RoleCustomer || p.caller.UserID != userID {
			return
		}
		p.portalLoadedFor = ""
		p.loadCustomerPortal(next)
		p.scheduleCustomerPortalRefresh(next, userID)
	})
}

func (p *serviceCatalogPage) customerPortalSection() app.UI {
	content := []app.UI{}
	if p.portalData != nil {
		reminders := customerMaintenanceReminders(p.portalData.Appliances, time.Now(), time.Local)
		if len(reminders) > 0 {
			content = append(content, app.Div().Class("portal-maintenance-alert").Body(
				app.Div().Class("portal-maintenance-alert__copy").Body(
					app.H4().Body(app.Text("Hora da manutenção preventiva! ❄️")),
					app.P().Body(app.Text(customerMaintenanceReminderText(reminders))),
				),
				app.Button().Class("auth-submit").Type("button").OnClick(p.openCustomerRequest).Body(app.Text("Agendar Agora")),
			))
		}
	}
	content = append(content,
		p.customerPortalProfileHeader(),
		app.H2().Class("catalog__title").Body(app.Text("Minhas Ordens de Serviço & Agendamentos")),
		app.P().Class("portal-section__intro").Body(app.Text("Status em tempo real das manutenções realizadas")),
	)
	if p.portalProfileOpen {
		content = append(content, p.customerProfileDialog())
	}
	if p.portalPasswordOpen {
		content = append(content, p.customerPasswordDialog())
	}
	if p.portalData != nil {
		content = append(content, app.Div().Class("portal-appliances").Body(
			app.Div().Class("portal-appliances__heading").Body(
				app.Div().Body(
					app.H3().Class("team-section__heading").Body(app.Text("Meus Aparelhos")),
					app.P().Class("portal-appliances__subtitle").Body(app.Text("Seus equipamentos monitorados pela Inovar")),
				),
				app.Button().Class("auth-submit portal-appliances__add").Type("button").OnClick(p.togglePortalApplianceForm).Body(app.Text("Adicionar Aparelho")),
			),
		))
		if len(p.portalData.Appliances) == 0 {
			content = append(content, app.Div().Class("portal-section__empty").Body(
				app.Span().Body(app.Text("❄")),
				app.H3().Body(app.Text("Nenhum aparelho cadastrado")),
				app.P().Body(app.Text("Cadastre seu ar-condicionado para ter histórico de manutenções, alerta de limpeza de ar e controle de garantia.")),
				app.Button().Class("auth-submit").Type("button").OnClick(p.togglePortalApplianceForm).Body(app.Text("Cadastrar Meu Primeiro Ar-Condicionado")),
			))
		}
		for _, appliance := range p.portalData.Appliances {
			label := portalText(appliance["marca"])
			if model := portalText(appliance["modelo"]); model != "" {
				label += " " + model
			}
			if label == "" {
				label = "Ar-Condicionado"
			}
			room := portalText(appliance["ambiente"])
			if room == "" {
				room = "Não informado"
			}
			details := "Ambiente: " + room
			if kind := portalText(appliance["tipo"]); kind != "" {
				details += " • " + kind
			}
			lastMaintenance := portalDate(appliance["ultima_manutencao"])
			if lastMaintenance == "" {
				lastMaintenance = "Pendente"
			}
			content = append(content, app.Article().Class("portal-appliance").Body(
				app.Div().Class("portal-appliance__main").Body(
					app.Strong().Body(app.Text(label)),
					app.P().Body(app.Text(details)),
					app.P().Body(app.Text("Última Manutenção: "+lastMaintenance)),
					app.Span().Class("portal-service__warranty").Body(app.Text("Monitorado")),
				),
				app.Span().Class("portal-service__warranty").Body(app.Text(portalBTUs(appliance["btus"]))),
				app.Div().Class("portal-budget__actions").Body(app.Button().Class("auth-link").Type("button").OnClick(p.requestServiceForAppliance(portalText(appliance["id"]))).Body(app.Text("Pedir Limpeza de Ar"))),
			))
		}
		if p.portalApplianceOpen {
			content = append(content, p.portalApplianceDialog())
		}
	}
	if p.portalLoading {
		content = append(content, app.P().Class("portal-section__empty").Body(app.Text("Carregando seus atendimentos...")))
	} else if p.portalError != "" {
		content = append(content, app.P().Class("portal-section__error").Body(app.Text(p.portalError)))
	} else if p.portalData == nil || len(p.portalData.Services) == 0 {
		content = append(content, app.P().Class("portal-section__empty").Body(app.Text("Você ainda não possui atendimentos registrados. Quando uma limpeza de ar ou conserto for agendado ou concluído, ele aparecerá aqui com comprovante e termo de garantia.")))
	} else {
		rows := make([]app.UI, 0, len(p.portalData.Services))
		for _, service := range p.portalData.Services {
			rows = append(rows, p.customerServiceCard(service))
		}
		content = append(content, app.Div().Class("portal-service-list").Body(rows...))
	}
	if p.portalData != nil {
		if p.portalRequestType == "" {
			p.portalRequestType = "LIMPEZA"
		}
		serviceOptions := customerPortalServiceOptions()
		serviceTypeOptions := make([]app.UI, 0, len(serviceOptions))
		for _, option := range serviceOptions {
			serviceTypeOptions = append(serviceTypeOptions, app.Option().Value(option.Value).Body(app.Text(option.Label)))
		}
		requestFields := []app.UI{}
		if len(p.portalData.Appliances) > 0 {
			options := []app.UI{}
			for i, appliance := range p.portalData.Appliances {
				label := strings.TrimSpace(portalText(appliance["marca"]) + " " + portalText(appliance["modelo"]))
				room := strings.TrimSpace(portalText(appliance["ambiente"]))
				if label == "" {
					label = "Aparelho"
				}
				if room != "" {
					label += " (" + room + ")"
				}
				selected := false
				if p.portalRequestAppliance == "" && i == 0 {
					selected = true
				}
				if p.portalRequestAppliance == portalText(appliance["id"]) {
					selected = true
				}
				option := app.Option().Value(portalText(appliance["id"])).Body(app.Text(label))
				if selected {
					option = option.Attr("selected", "selected")
				}
				options = append(options, option)
			}
			requestFields = append(requestFields, app.Label().Class("auth-field").Body(app.Text("Qual Aparelho?"), app.Select().Attr("value", p.portalRequestAppliance).OnChange(p.ValueTo(&p.portalRequestAppliance)).Body(options...)))
		}
		requestFields = append(requestFields,
			app.Label().Class("auth-field").Body(app.Text("Tipo de Serviço"), app.Select().Attr("value", p.portalRequestType).OnChange(p.ValueTo(&p.portalRequestType)).Body(serviceTypeOptions...)),
			app.Label().Class("auth-field").Body(app.Text("Data Preferida para Visita"), app.Input().Type("date").Value(p.portalRequestDate).OnChange(p.ValueTo(&p.portalRequestDate))),
			app.Label().Class("auth-field").Body(app.Text("Observações ou Sintomas"), app.Textarea().Rows(2).Text(p.portalRequestProblem).Placeholder("Ex: Pingando água na sala, cheiro ruim ao ligar, etc.").OnChange(p.ValueTo(&p.portalRequestProblem))),
			app.Label().Class("auth-field").Body(app.Text("Observações adicionais"), app.Textarea().Rows(2).Text(p.portalRequestNotes).OnChange(p.ValueTo(&p.portalRequestNotes))),
			app.Div().Class("portal-request__notice").Body(app.Span().Body(app.Text("✓ ")), app.Span().Body(app.Text("Garantia de 90 dias com certificado e produtos antibacterianos homologados."))),
		)
		if p.portalRequestOpen {
			content = append(content, app.Div().Class("auth-backdrop").Body(app.Div().Class("auth-dialog").Role("dialog").Body(
				app.Div().Class("auth-dialog__heading").Body(app.H3().Class("auth-dialog__title").Body(app.Text("Solicitar Atendimento da Inovar")), app.Button().Class("auth-close").Type("button").Disabled(p.portalRequestBusy).OnClick(p.closeCustomerRequest).Body(app.Text("✕"))),
				app.Form().Class("auth-form").OnSubmit(p.submitPortalRequest).Body(append(requestFields,
					app.Button().Class("auth-submit").Type("submit").Disabled(p.portalRequestBusy).Body(app.Text(func() string {
						if p.portalRequestBusy {
							return "Enviando..."
						}
						return "Confirmar Solicitação"
					}())),
				)...),
			)))
		}
		content = append(content, p.customerBudgetsSection())
	}
	if p.budgetToSign != nil {
		content = append(content, p.budgetApprovalDialog())
	}
	if p.budgetDecline != nil {
		content = append(content, p.budgetDeclineDialog())
	}
	if p.budgetFeedback != "" {
		content = append(content, app.P().Class("portal-section__intro").Body(app.Text(p.budgetFeedback)))
	}
	return app.Section().Class("portal-section").Body(content...)
}

func (p *serviceCatalogPage) requestServiceForAppliance(id string) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		if p.portalData == nil {
			return
		}
		p.portalRequestAppliance = selectedCustomerAppliance(id, p.portalData.Appliances)
		p.portalRequestOpen = true
	}
}

func (p *serviceCatalogPage) openCustomerRequest(ctx app.Context, event app.Event) {
	event.PreventDefault()
	p.portalRequestOpen = true
	p.portalRequestAppliance = selectedCustomerAppliance(p.portalRequestAppliance, p.portalData.Appliances)
}
func (p *serviceCatalogPage) closeCustomerRequest(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if !p.portalRequestBusy {
		p.portalRequestOpen = false
	}
}

func (p *serviceCatalogPage) togglePortalApplianceForm(ctx app.Context, event app.Event) {
	event.PreventDefault()
	p.portalApplianceOpen = !p.portalApplianceOpen
	p.portalApplianceFeedback = ""
	p.portalApplianceSaved = false
}

func (p *serviceCatalogPage) portalApplianceForm() app.UI {
	brand := p.portalApplianceBrand
	if brand == "" {
		brand = "LG"
	}
	capacity := p.portalApplianceBTUs
	if capacity == "" {
		capacity = "12000"
	}
	applianceType := p.portalApplianceType
	if applianceType == "" {
		applianceType = "Split Hi-Wall"
	}
	room := p.portalApplianceRoom
	if room == "" {
		room = "Sala"
	}
	brands := []string{"LG", "Gree", "Midea", "Samsung", "Daikin", "Carrier", "Elgin", "Fujitsu", "Consul", "Electrolux", "Philco", "Outro"}
	brandOptions := make([]app.UI, 0, len(brands))
	for _, value := range brands {
		brandOptions = append(brandOptions, app.Option().Value(value).Body(app.Text(value)))
	}
	capacities := []string{"9000", "12000", "18000", "24000", "30000", "36000", "48000", "60000"}
	capacityOptions := make([]app.UI, 0, len(capacities))
	for _, value := range capacities {
		capacityOptions = append(capacityOptions, app.Option().Value(value).Body(app.Text(formatPortalCapacity(value)+" BTUs")))
	}
	kinds := []string{"Split Hi-Wall", "Inverter", "Cassete", "Piso Teto", "Janela", "Multi Split", "Portátil"}
	kindOptions := make([]app.UI, 0, len(kinds))
	for _, value := range kinds {
		kindOptions = append(kindOptions, app.Option().Value(value).Body(app.Text(value)))
	}
	fields := []app.UI{
		app.P().Class("catalog__eyebrow").Body(app.Text("CADASTRAR AR-CONDICIONADO")),
		app.Label().Class("auth-field").Body(app.Text("Marca"), app.Select().Attr("value", brand).OnChange(p.ValueTo(&p.portalApplianceBrand)).Body(brandOptions...)),
		app.Label().Class("auth-field").Body(app.Text("Modelo (Opcional)"), app.Input().Type("text").Value(p.portalApplianceModel).Placeholder("Ex.: Dual Inverter Voice").OnChange(p.ValueTo(&p.portalApplianceModel))),
		app.Div().Class("portal-appliance-form__row").Body(
			app.Label().Class("auth-field").Body(app.Text("Capacidade (BTUs)"), app.Select().Attr("value", capacity).OnChange(p.ValueTo(&p.portalApplianceBTUs)).Body(capacityOptions...)),
			app.Label().Class("auth-field").Body(app.Text("Tipo"), app.Select().Attr("value", applianceType).OnChange(p.ValueTo(&p.portalApplianceType)).Body(kindOptions...)),
		),
		app.Label().Class("auth-field").Body(app.Text("Ambiente Instalado"), app.Input().Type("text").Value(room).Placeholder("Ex.: Quarto Casal, Sala de Estar, Escritório").OnChange(p.ValueTo(&p.portalApplianceRoom))),
	}
	if p.portalApplianceFeedback != "" {
		fields = append(fields, app.P().Class("portal-section__error").Body(app.Text(p.portalApplianceFeedback)))
	}
	fields = append(fields, app.Div().Class("portal-budget__actions").Body(
		app.Button().Class("auth-link").Type("button").Disabled(p.portalApplianceBusy).OnClick(p.togglePortalApplianceForm).Body(app.Text("Cancelar")),
		app.Button().Class("auth-submit").Type("submit").Disabled(p.portalApplianceBusy).Body(app.Text(func() string {
			if p.portalApplianceBusy {
				return "Salvando..."
			}
			return "Salvar Aparelho"
		}())),
	))
	return app.Form().Class("auth-form").OnSubmit(p.submitPortalAppliance).Body(fields...)
}

func (p *serviceCatalogPage) portalApplianceDialog() app.UI {
	return app.Div().Class("auth-backdrop").Body(app.Div().Class("auth-dialog").Role("dialog").Body(
		app.Div().Class("auth-dialog__heading").Body(
			app.H3().Class("auth-dialog__title").Body(app.Text("Cadastrar Ar-Condicionado")),
			app.Button().Class("auth-close").Type("button").Disabled(p.portalApplianceBusy).OnClick(p.togglePortalApplianceForm).Body(app.Text("✕")),
		),
		p.portalApplianceForm(),
	))
}

func (p *serviceCatalogPage) customerServiceCard(service map[string]any) app.UI {
	status := strings.ToUpper(portalText(service["status"]))
	label, tone := portalServiceStatus(status)
	serviceType := customerPortalServiceTypeLabel(portalText(service["tipo"]))
	date := customerServiceCardDate(service["data_agendamento"], service["data_solicitacao"])
	if problem := portalText(service["problema"]); problem != "" {
		date += " • \"" + problem + "\""
	}
	items := []app.UI{
		app.Div().Class("portal-service__main").Body(
			app.Div().Class("portal-service__heading").Body(
				app.Strong().Body(app.Text(string(serviceType))),
				app.Span().Class("portal-service__status portal-service__status--"+tone).Body(app.Text(label)),
			),
			app.P().Class("portal-service__date").Body(app.Text(date)),
		),
	}
	if amount, show := portalLegacyServiceAmount(service["valor"]); show {
		items = append(items, app.Span().Class("portal-service__value").Body(app.Text("R$ "+amount)))
	}
	if status == "CONCLUIDO" {
		items = append(items, app.Span().Class("portal-service__warranty").Body(app.Text("✓ Garantia 90 Dias")))
	}
	return app.Article().Class("portal-service").Body(items...)
}

func customerServiceCardDate(scheduledValue, requestedValue any) string {
	return customerServiceCardDateInLocation(scheduledValue, requestedValue, time.Local)
}

func customerServiceCardDateInLocation(scheduledValue, requestedValue any, location *time.Location) string {
	if scheduled := portalText(scheduledValue); scheduled != "" {
		return "Data marcada: " + portalServiceDateInLocation(scheduled, location)
	}
	if requested := portalServiceDateInLocation(portalText(requestedValue), location); requested != "" {
		return "Solicitado em: " + requested
	}
	return "Solicitado em: Hoje"
}

// portalLegacyServiceAmount mirrors the React expression `serv.valor &&
// Number(serv.valor).toFixed(2)`: values have no thousands separator and use
// a decimal point, while zero/empty values do not render a badge.
func portalLegacyServiceAmount(value any) (string, bool) {
	if value == nil {
		return "", false
	}
	var amount float64
	truthy := true
	stringValue := false
	switch v := value.(type) {
	case float64:
		amount = v
	case float32:
		amount = float64(v)
	case int:
		amount = float64(v)
	case int64:
		amount = float64(v)
	case string:
		stringValue = true
		if v == "" {
			return "", false
		}
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return "NaN", true
		}
		amount = parsed
	default:
		return "", false
	}
	if !truthy || (!stringValue && amount == 0) || math.IsNaN(amount) {
		return "", false
	}
	return strconv.FormatFloat(amount, 'f', 2, 64), true
}

func customerPortalServiceTypeLabel(serviceType string) string {
	if index := strings.IndexByte(serviceType, '_'); index >= 0 {
		return serviceType[:index] + " " + serviceType[index+1:]
	}
	return serviceType
}

func portalServiceDate(value any) string {
	return portalServiceDateInLocation(portalText(value), time.Local)
}

func portalServiceDateInLocation(value string, location *time.Location) string {
	value = strings.TrimSpace(value)
	if len(value) < 10 {
		return ""
	}
	if location == nil {
		location = time.UTC
	}
	if len(value) == 10 {
		parsed, err := time.ParseInLocation("2006-01-02", value, location)
		if err != nil {
			return ""
		}
		return parsed.Format("02/01/2006")
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.In(location).Format("02/01/2006")
	}
	for _, layout := range []string{"2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05", "2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, value, location); err == nil {
			return parsed.Format("02/01/2006")
		}
	}
	return ""
}

func portalServiceStatus(status string) (string, string) {
	switch status {
	case "PENDENTE":
		return "Aguardando confirmação", "pending"
	case "AGENDADO":
		return "Agendado", "scheduled"
	case "EM_ANDAMENTO":
		return "Em execução", "progress"
	case "CONCLUIDO":
		return "Concluído • Garantia ativa", "complete"
	case "CANCELADO":
		return "Cancelado", "cancelled"
	default:
		return status, "unknown"
	}
}

func portalText(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func portalDate(value any) string {
	date := portalText(value)
	if len(date) < 10 {
		return ""
	}
	parsed, err := time.Parse("2006-01-02", date[:10])
	if err != nil {
		return ""
	}
	return parsed.Format("02/01/2006")
}

func formatPortalMoney(value float64) string {
	whole, fraction := int64(value), int64((value-float64(int64(value)))*100+0.5)
	if fraction >= 100 {
		whole++
		fraction -= 100
	}
	formatted := strconv.FormatInt(whole, 10)
	for index := len(formatted) - 3; index > 0; index -= 3 {
		formatted = formatted[:index] + "." + formatted[index:]
	}
	return fmt.Sprintf("%s,%02d", formatted, fraction)
}

func portalBTUs(value any) string {
	var amount int64
	switch number := value.(type) {
	case float64:
		amount = int64(number)
	case string:
		parsed, _ := strconv.ParseInt(number, 10, 64)
		amount = parsed
	}
	if amount == 0 {
		return "Capacidade não informada"
	}
	return formatPortalCapacity(strconv.FormatInt(amount, 10)) + " BTUs"
}

func formatPortalCapacity(value string) string {
	amount, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return value
	}
	formatted := strconv.FormatInt(amount, 10)
	for index := len(formatted) - 3; index > 0; index -= 3 {
		formatted = formatted[:index] + "." + formatted[index:]
	}
	return formatted
}
