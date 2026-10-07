package webapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

func (p *serviceCatalogPage) pushNotificationPanel() app.UI {
	if desktopWailsAvailable() {
		return p.desktopNotificationPanel()
	}
	if nativePushAvailable() {
		label := "Ativar notificações neste dispositivo"
		if p.pushChecking {
			label = "Verificando notificações…"
		} else if p.pushBusy {
			label = "Ativando notificações…"
		} else if p.pushEnabled {
			label = "Notificações ativadas neste dispositivo"
		}
		return app.Section().Class("portal-section device-permission").Body(
			app.Strong().Body(app.Text("Avisos neste dispositivo")),
			app.P().Body(app.Text("Autorize os avisos do sistema para registrar este dispositivo. A entrega remota também depende da configuração do provedor de notificações.")),
			app.Button().Class("auth-submit device-permission__action").Type("button").Disabled(p.pushBusy || p.pushChecking || p.pushEnabled).OnClick(p.enableNativePush).Body(app.Text(label)),
			p.notificationPermissionNotice(),
		)
	}
	if browserPushSupported() && strings.TrimSpace(app.Getenv("VAPID_PUBLIC_KEY")) != "" {
		return p.browserPushNotificationPanel()
	}
	if browserNotificationSupported() {
		label := "Permitir avisos neste navegador"
		if p.pushBusy {
			label = "Solicitando permissão…"
		} else if p.localNotificationsEnabled {
			label = "Enviar notificação de teste"
		}
		return app.Section().Class("portal-section device-permission").Body(
			app.Strong().Body(app.Text("Avisos neste navegador")),
			app.P().Body(app.Text("A permissão permite avisos enquanto o app está aberto. Avisos com o app fechado dependem da configuração do serviço push.")),
			app.Button().Class("auth-submit device-permission__action").Type("button").Disabled(p.pushBusy).OnClick(p.enableLocalBrowserNotifications).Body(app.Text(label)),
			p.notificationPermissionNotice(),
		)
	}
	return app.Section().Class("portal-section device-permission").Body(
		app.Strong().Body(app.Text("Avisos neste dispositivo")),
		app.P().Body(app.Text("Este navegador não oferece notificações do sistema. Você ainda verá os avisos dentro do InovarApp.")),
	)
}

func (p *serviceCatalogPage) browserPushNotificationPanel() app.UI {
	label := "Ativar notificações neste dispositivo"
	if p.pushChecking {
		label = "Verificando notificações…"
	} else if p.pushBusy {
		label = "Ativando notificações…"
	}
	if p.pushEnabled {
		label = "Desativar notificações neste dispositivo"
	}
	onClick := p.enableBrowserPush
	if p.pushEnabled {
		onClick = p.disableBrowserPush
	}
	return app.Section().Class("portal-section device-permission").Body(
		app.Strong().Body(app.Text("Avisos neste navegador")),
		app.P().Body(app.Text("Autorize o navegador para receber atualizações da sua conta Inovar neste dispositivo.")),
		app.Button().Class("auth-submit device-permission__action").Type("button").Disabled(p.pushBusy || p.pushChecking).OnClick(onClick).Body(app.Text(label)),
		p.notificationPermissionNotice(),
	)
}

func (p *serviceCatalogPage) notificationPermissionNotice() app.UI {
	if p.pushNotice == "" {
		return app.Span()
	}
	return app.P().Class("device-permission__notice").Attr("role", "status").Body(app.Text(p.pushNotice))
}

func (p *serviceCatalogPage) desktopNotificationPanel() app.UI {
	label := "Ativar avisos do sistema"
	if p.pushBusy {
		label = "Solicitando permissão…"
	} else if p.localNotificationsEnabled {
		label = "Enviar notificação de teste"
	}
	content := []app.UI{
		app.Strong().Body(app.Text("Avisos neste computador")),
		app.P().Body(app.Text("Ative os avisos do sistema para receber alertas enquanto o InovarApp estiver aberto. O Windows não exige uma permissão separada; macOS pode exibir uma confirmação.")),
		app.Button().Class("auth-submit device-permission__action").Type("button").Disabled(p.pushBusy).OnClick(p.enableDesktopNotifications).Body(app.Text(label)),
	}
	if browserPushSupported() && strings.TrimSpace(app.Getenv("VAPID_PUBLIC_KEY")) != "" {
		pushLabel := "Ativar alertas também com o app fechado"
		if p.pushChecking {
			pushLabel = "Verificando alertas remotos…"
		} else if p.pushBusy {
			pushLabel = "Ativando alertas remotos…"
		} else if p.pushEnabled {
			pushLabel = "Desativar alertas com o app fechado"
		}
		pushAction := p.enableBrowserPush
		if p.pushEnabled {
			pushAction = p.disableBrowserPush
		}
		content = append(content,
			app.P().Body(app.Text("Se este computador oferecer Web Push, você também pode ativar alertas remotos usando sua conta neste navegador.")),
			app.Button().Class("auth-link device-permission__action").Type("button").Disabled(p.pushBusy || p.pushChecking).OnClick(pushAction).Body(app.Text(pushLabel)),
		)
	}
	content = append(content, p.notificationPermissionNotice())
	return app.Section().Class("portal-section device-permission").Body(content...)
}

func (p *serviceCatalogPage) enableDesktopNotifications(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.pushBusy {
		return
	}
	p.pushBusy, p.pushNotice = true, ""
	authorization := beginDesktopNotificationAuthorization()
	ctx.Update()
	go func() {
		result := <-authorization
		allowed, err := result.allowed, result.err
		p.pushBusy = false
		if err != nil {
			p.pushNotice = "Não foi possível ativar os avisos do sistema: " + err.Error()
		} else if !allowed {
			p.pushNotice = "A permissão foi negada. Confira as notificações do InovarApp nas configurações do sistema."
		} else {
			p.localNotificationsEnabled = true
			p.pushNotice = "Avisos do sistema ativados neste computador."
			notifyDevice("InovarApp", "As notificações estão prontas neste computador.")
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) enableLocalBrowserNotifications(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.pushBusy {
		return
	}
	p.pushBusy, p.pushNotice = true, ""
	permissionRequest := beginBrowserNotificationPermissionRequest()
	ctx.Update()
	go func() {
		result := <-permissionRequest
		permission, err := result.permission, result.err
		p.pushBusy = false
		if err != nil {
			p.pushNotice = err.Error()
		} else if permission != "granted" {
			p.pushNotice = "A permissão não foi concedida. Ative as notificações do InovarApp nas configurações do navegador ou do sistema."
		} else {
			p.localNotificationsEnabled = true
			p.pushNotice = "Avisos ativados neste navegador."
			notifyDevice("InovarApp", "As notificações estão prontas neste navegador.")
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) enableNativePush(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.pushBusy || p.pushEnabled || p.session == nil || p.caller == nil {
		return
	}
	userID, accessToken := p.caller.UserID, p.session.AccessToken
	p.pushBusy = true
	p.pushNotice = ""
	pushRequest := beginNativePushRequest()
	ctx.Update()
	go func() {
		result := <-pushRequest
		platform, token, err := result.platform, result.token, result.err
		if err == nil {
			err = p.storeNativePushToken(ctx, accessToken, platform, token)
		}
		if p.caller == nil || p.caller.UserID != userID {
			return
		}
		p.pushBusy = false
		if err != nil {
			p.pushNotice = "Não foi possível registrar notificações neste dispositivo. Verifique as permissões e a configuração nativa."
		} else {
			p.pushEnabled = true
			p.pushNotice = "Dispositivo registrado. A entrega requer configuração do provedor de notificações."
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) storeNativePushToken(ctx context.Context, accessToken, platform, token string) error {
	pageURL := app.Window().URL()
	if pageURL == nil || pageURL.Scheme == "" || pageURL.Host == "" {
		return fmt.Errorf("URL da aplicação indisponível")
	}
	body, err := json.Marshal(struct {
		Action   string `json:"acao"`
		Platform string `json:"platform"`
		Token    string `json:"token"`
	}{"inscrever-nativo", platform, token})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, apiEndpoint("/api/notificacoes"), bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 12 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("native push registration returned status %d", response.StatusCode)
	}
	return nil
}

func (p *serviceCatalogPage) restoreBrowserPushState(ctx app.Context) {
	p.restoreNativePushState(ctx)
	if !browserPushSupported() || p.session == nil || p.caller == nil || p.pushChecking {
		return
	}
	userID, accessToken := p.caller.UserID, p.session.AccessToken
	p.pushChecking = true
	go func() {
		subscription, exists, err := currentBrowserPushSubscription()
		active := false
		if err == nil && exists {
			active, err = p.browserPushSubscriptionActive(ctx, accessToken, subscription)
		}
		if p.caller == nil || p.caller.UserID != userID {
			return
		}
		p.pushChecking = false
		p.pushEnabled = active
		if err != nil {
			p.pushNotice = "Não foi possível consultar o estado das notificações neste dispositivo."
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) restoreNativePushState(ctx app.Context) {
	if !nativePushAvailable() || p.session == nil || p.caller == nil {
		return
	}
	userID, accessToken := p.caller.UserID, p.session.AccessToken
	listenForNativePushToken(func(platform, token string) {
		if p.caller == nil || p.caller.UserID != userID {
			return
		}
		p.pushEnabled = true
		go func() {
			err := p.storeNativePushToken(ctx, accessToken, platform, token)
			if p.caller != nil && p.caller.UserID == userID {
				if err != nil {
					p.pushNotice = "Não foi possível atualizar o registro de notificações nativas."
				}
				ctx.Update()
			}
		}()
		ctx.Update()
	})
}

func (p *serviceCatalogPage) disableBrowserPush(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.pushBusy || !p.pushEnabled || p.session == nil {
		return
	}
	userID, accessToken := "", p.session.AccessToken
	if p.caller != nil {
		userID = p.caller.UserID
	}
	p.pushBusy = true
	p.pushNotice = ""
	ctx.Update()
	go func() {
		subscription, exists, err := currentBrowserPushSubscription()
		if err == nil && exists {
			_, err = p.browserPushAction(ctx, accessToken, "remover", subscription)
		}
		if unsubscribeErr := unsubscribeBrowserPush(); err == nil {
			err = unsubscribeErr
		}
		if p.caller == nil || p.caller.UserID != userID {
			return
		}
		p.pushBusy, p.pushEnabled = false, false
		if err != nil {
			p.pushNotice = "A inscrição foi removida deste navegador, mas o servidor não confirmou a atualização."
		} else {
			p.pushNotice = "Notificações desativadas neste dispositivo."
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) removeBrowserPushForAccount(ctx app.Context, accessToken string) error {
	subscription, exists, err := currentBrowserPushSubscription()
	if err == nil && exists {
		_, err = p.browserPushAction(ctx, accessToken, "remover", subscription)
	}
	if unsubscribeErr := unsubscribeBrowserPush(); err == nil {
		err = unsubscribeErr
	}
	return err
}

func (p *serviceCatalogPage) enableBrowserPush(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.pushBusy || p.pushEnabled || p.session == nil {
		return
	}
	userID := ""
	if p.caller != nil {
		userID = p.caller.UserID
	}
	accessToken := p.session.AccessToken
	subscription, err := subscribeBrowserPush(app.Getenv("VAPID_PUBLIC_KEY"))
	if err != nil {
		p.pushNotice = err.Error()
		ctx.Update()
		return
	}
	p.pushBusy = true
	p.pushNotice = ""
	ctx.Update()
	go func() {
		err := p.storeBrowserPushSubscription(ctx, accessToken, subscription)
		if p.caller == nil || p.caller.UserID != userID {
			return
		}
		p.pushBusy = false
		if err != nil {
			p.pushNotice = "Não foi possível ativar notificações neste dispositivo. Verifique a conexão e tente novamente."
		} else {
			p.pushEnabled = true
			p.pushNotice = "Notificações ativadas neste dispositivo."
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) storeBrowserPushSubscription(ctx context.Context, accessToken, subscription string) error {
	_, err := p.browserPushAction(ctx, accessToken, "inscrever", subscription)
	return err
}

func (p *serviceCatalogPage) browserPushSubscriptionActive(ctx context.Context, accessToken, subscription string) (bool, error) {
	return p.browserPushAction(ctx, accessToken, "estado", subscription)
}

func (p *serviceCatalogPage) browserPushAction(ctx context.Context, accessToken, action, subscription string) (bool, error) {
	pageURL := app.Window().URL()
	if pageURL == nil || pageURL.Scheme == "" || pageURL.Host == "" {
		return false, fmt.Errorf("URL da aplicação indisponível")
	}
	body, err := json.Marshal(struct {
		Action       string          `json:"acao"`
		Subscription json.RawMessage `json:"subscription"`
	}{Action: action, Subscription: json.RawMessage(subscription)})
	if err != nil {
		return false, err
	}
	endpoint := apiEndpoint("/api/notificacoes")
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 12 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var result struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(responseBody, &result)
		if result.Error != "" {
			return false, fmt.Errorf("%s", result.Error)
		}
		return false, fmt.Errorf("subscription action returned status %d", response.StatusCode)
	}
	var result struct {
		Active bool `json:"active"`
	}
	if action == "estado" {
		if err := json.Unmarshal(responseBody, &result); err != nil {
			return false, err
		}
		return result.Active, nil
	}
	return true, nil
}
