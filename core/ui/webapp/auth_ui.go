package webapp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/adapter/cep"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/domain"
)

func (p *serviceCatalogPage) togglePasswordVisibility(visible *bool) func(app.Context, app.Event) {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		*visible = !*visible
		ctx.Update()
	}
}

func (p *serviceCatalogPage) passwordField(label, autocomplete, placeholder string, value *string, visible *bool) app.UI {
	inputType := "password"
	buttonLabel := "Mostrar"
	ariaLabel := "Mostrar senha"
	if *visible {
		inputType, buttonLabel, ariaLabel = "text", "Ocultar", "Ocultar senha"
	}
	return app.Label().Class("auth-field").Body(app.Text(label), app.Div().Class("auth-password-control").Body(
		app.Input().Type(inputType).Required(true).Attr("autocomplete", autocomplete).Placeholder(placeholder).Value(*value).OnChange(p.ValueTo(value)),
		app.Button().Class("auth-password-toggle").Type("button").Attr("aria-label", ariaLabel).Attr("aria-pressed", ariaBoolean(*visible)).OnClick(p.togglePasswordVisibility(visible)).Body(app.Text(buttonLabel)),
	))
}

const (
	legacySupabaseSessionStorageKey = "supabase.auth.token"
	sessionRefreshLead              = 90 * time.Second
	sessionRefreshRetry             = 30 * time.Second
)

func supabaseSessionStorageKeyForURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Hostname() == "" {
		return legacySupabaseSessionStorageKey
	}
	projectRef := strings.SplitN(parsed.Hostname(), ".", 2)[0]
	if projectRef == "" {
		return legacySupabaseSessionStorageKey
	}
	return "sb-" + projectRef + "-auth-token"
}

func supabaseSessionStorageKeys() (string, string) {
	return supabaseSessionStorageKeyForURL(app.Getenv("SUPABASE_URL")), legacySupabaseSessionStorageKey
}

func saveSupabaseSession(ctx app.Context, session supabase.AuthSession) error {
	currentKey, legacyKey := supabaseSessionStorageKeys()
	if err := ctx.LocalStorage().Set(currentKey, session); err != nil {
		return err
	}
	if legacyKey != currentKey {
		ctx.LocalStorage().Del(legacyKey)
	}
	return nil
}

func deleteSupabaseSession(ctx app.Context) {
	currentKey, legacyKey := supabaseSessionStorageKeys()
	ctx.LocalStorage().Del(currentKey)
	if legacyKey != currentKey {
		ctx.LocalStorage().Del(legacyKey)
	}
}

func publicSupabaseClient() (*supabase.Client, error) {
	return supabase.New(supabase.Config{
		URL:     app.Getenv("SUPABASE_URL"),
		AnonKey: app.Getenv("SUPABASE_ANON_KEY"),
	})
}

func (p *serviceCatalogPage) OnMount(ctx app.Context) {
	if !app.IsClient {
		return
	}
	p.loadLocalNotifications(ctx)
	p.restoreTeamSectionFromURL()
	if p.city == "" {
		p.city = "Vitória"
	}
	p.restoreSession(ctx)
	p.loadWhatsAppQueueIfSelected(ctx)
	if p.authLifecycleStop == nil {
		currentKey, legacyKey := supabaseSessionStorageKeys()
		p.authLifecycleStop = watchAuthLifecycle(currentKey, legacyKey, func() {
			ctx.Dispatch(func(next app.Context) {
				storage := next.LocalStorage()
				currentKey, legacyKey := supabaseSessionStorageKeys()
				if !storage.Contains(currentKey) && !storage.Contains(legacyKey) {
					if p.session != nil {
						p.clearSession(next)
						p.authNotice = "Sua sessão foi encerrada em outra janela."
						next.Update()
					}
					return
				}
				p.restoreSession(next)
			})
		})
	}
	if p.notificationLifecycleStop == nil {
		p.notificationLifecycleStop = watchNotificationStorage(func() {
			ctx.Dispatch(func(next app.Context) {
				p.loadLocalNotifications(next)
				next.Update()
			})
		})
	}
	registerOfflineOnlineCallback(func() {
		ctx.Dispatch(func(next app.Context) {
			if p.session == nil || p.caller == nil || !hasPendingOfflineTeamMutations(p.caller.UserID) {
				return
			}
			p.retryPendingOfflineMutations(next)
		})
	})
	p.processAuthRedirect(ctx)
	registerNativeOAuthCallbacks(func(callbackURL string) {
		p.processNativeAuthRedirect(ctx, callbackURL)
		ctx.Update()
	}, func() {
		p.authOpen, p.authMode = true, "login"
		p.authError = "Não foi possível abrir a autenticação do Google. Tente novamente."
		ctx.Update()
	})
}

func (p *serviceCatalogPage) OnDismount() {
	if p.authLifecycleStop != nil {
		p.authLifecycleStop()
		p.authLifecycleStop = nil
	}
	if p.notificationLifecycleStop != nil {
		p.notificationLifecycleStop()
		p.notificationLifecycleStop = nil
	}
}

func (p *serviceCatalogPage) restoreSession(ctx app.Context) {
	storage := ctx.LocalStorage()
	currentKey, legacyKey := supabaseSessionStorageKeys()
	storageKey := currentKey
	if !storage.Contains(storageKey) {
		if !storage.Contains(legacyKey) {
			return
		}
		storageKey = legacyKey
	}
	if !storage.Contains(storageKey) {
		return
	}

	var session supabase.AuthSession
	if err := storage.Get(storageKey, &session); err != nil {
		p.authNotice = "Não foi possível ler a sessão salva neste navegador. Entre novamente."
		return
	}
	if session.AccessToken == "" || session.RefreshToken == "" {
		storage.Del(storageKey)
		return
	}

	client, err := publicSupabaseClient()
	if err != nil {
		p.authNotice = "A autenticação ainda não está configurada neste ambiente."
		return
	}

	caller, err := p.validateSession(ctx, client, &session)
	if err != nil {
		if errors.Is(err, supabase.ErrUnauthorized) || isRejectedAuthSession(err) {
			storage.Del(storageKey)
			p.authNotice = "Sua sessão expirou. Entre novamente para continuar."
			return
		}
		p.session = &session
		if session.User != nil && session.User.ID != "" {
			p.restoreOfflineAccount(ctx, session.User.ID, session.AccessToken)
			return
		}
		p.authNotice = "Não foi possível validar a sessão. Verifique a conexão e tente novamente."
		ctx.After(sessionRefreshRetry, func(next app.Context) { p.restoreSession(next) })
		return
	}
	wasOffline := p.offlineMode
	p.session = &session
	p.offlineMode = false
	if storageKey != currentKey {
		if err := storage.Set(currentKey, session); err == nil {
			storage.Del(storageKey)
		}
	}
	p.caller = &caller
	if wasOffline {
		p.teamLoadedFor, p.portalLoadedFor = "", ""
	}
	p.restoreBrowserPushState(ctx)
	p.recoveryRequired = metadataMustChange(session.User)
	p.recoveryOpen = p.recoveryRequired
	p.authNotice = ""
	p.loadCustomerPortal(ctx)
	p.loadTeamOperations(ctx)
	p.loadWhatsAppQueueIfSelected(ctx)
	p.scheduleSessionRefresh(ctx)
}

func (p *serviceCatalogPage) restoreOfflineAccount(ctx app.Context, userID, token string) {
	go func() {
		snapshot, err := loadOfflineSnapshot(userID)
		if p.session == nil || p.session.AccessToken != token {
			return
		}
		if err != nil {
			p.authNotice = "Não foi possível validar a sessão. Verifique a conexão; ainda não há dados offline salvos para esta conta."
			p.offlineMode = false
			ctx.After(sessionRefreshRetry, func(next app.Context) { p.restoreSession(next) })
			ctx.Update()
			return
		}
		p.offlineMode = true
		p.caller = &supabase.Caller{UserID: userID, Role: snapshot.Role, Token: token}
		p.authNotice = "A sessão não pôde ser conferida sem conexão. Dados locais estão em modo somente leitura; reconecte para validar a conta."
		if snapshot.Role == domain.RoleCustomer {
			p.portalData, p.portalLoadedFor = snapshot.PortalData, userID
		} else {
			p.teamServices, p.teamAppointments, p.teamCustomers = snapshot.TeamServices, snapshot.TeamAppointments, snapshot.TeamCustomers
			p.teamBudgets, p.teamReturnHistory, p.teamLoadedFor = snapshot.TeamBudgets, snapshot.TeamHistory, userID
			if snapshot.TeamProfile != nil {
				p.teamProfile = *snapshot.TeamProfile
			}
			p.teamDashboard = buildTeamDashboardSummary(p.teamCustomers, p.teamServices, p.teamAppointments, p.teamBudgets, p.teamReturnHistory, p.teamProfile.DefaultReturnMonths, dashboardCivilToday(time.Now()))
		}
		ctx.After(sessionRefreshRetry, func(next app.Context) { p.restoreSession(next) })
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) validateSession(ctx app.Context, client *supabase.Client, session *supabase.AuthSession) (supabase.Caller, error) {
	refreshed := false
	if session.ExpiresAt > 0 && time.Until(time.Unix(session.ExpiresAt, 0)) <= sessionRefreshLead {
		if err := p.refreshSession(ctx, client, session); err != nil {
			return supabase.Caller{}, err
		}
		refreshed = true
	}

	caller, err := client.AuthenticateCaller(ctx, session.AccessToken)
	if err == nil || !errors.Is(err, supabase.ErrUnauthorized) || refreshed {
		return caller, err
	}
	if err := p.refreshSession(ctx, client, session); err != nil {
		return supabase.Caller{}, err
	}
	return client.AuthenticateCaller(ctx, session.AccessToken)
}

func (p *serviceCatalogPage) refreshSession(ctx app.Context, client *supabase.Client, session *supabase.AuthSession) error {
	refreshed, err := client.RefreshSession(ctx, *session)
	if err != nil {
		return err
	}
	*session = refreshed
	if err := saveSupabaseSession(ctx, *session); err != nil {
		return fmt.Errorf("save refreshed Supabase session: %w", err)
	}
	return nil
}

func (p *serviceCatalogPage) scheduleSessionRefresh(ctx app.Context) {
	if p.session == nil || p.offlineMode {
		return
	}
	delay := 45 * time.Minute
	if p.session.ExpiresAt > 0 {
		delay = time.Until(time.Unix(p.session.ExpiresAt, 0)) - sessionRefreshLead
	}
	if delay < time.Second {
		delay = time.Second
	}
	ctx.After(delay, func(next app.Context) { p.refreshStoredSession(next) })
}

func (p *serviceCatalogPage) refreshStoredSession(ctx app.Context) {
	if p.session == nil || p.refreshingSession {
		return
	}
	if p.offlineMode {
		ctx.After(sessionRefreshRetry, func(next app.Context) { p.restoreSession(next) })
		return
	}
	p.refreshingSession = true
	client, err := publicSupabaseClient()
	if err != nil {
		p.refreshingSession = false
		p.authNotice = "A autenticação ainda não está configurada neste ambiente."
		ctx.After(sessionRefreshRetry, func(next app.Context) { p.refreshStoredSession(next) })
		return
	}

	if err := p.refreshSession(ctx, client, p.session); err != nil {
		p.refreshingSession = false
		if isRejectedAuthSession(err) {
			p.clearSession(ctx)
			p.authNotice = "Sua sessão expirou. Entre novamente para continuar."
			return
		}
		p.authNotice = "A conexão foi interrompida. A sessão será renovada novamente em instantes."
		ctx.After(sessionRefreshRetry, func(next app.Context) { p.refreshStoredSession(next) })
		return
	}

	caller, err := client.AuthenticateCaller(ctx, p.session.AccessToken)
	p.refreshingSession = false
	if errors.Is(err, supabase.ErrUnauthorized) {
		p.clearSession(ctx)
		p.authNotice = "Sua sessão expirou. Entre novamente para continuar."
		return
	}
	if err != nil {
		p.authNotice = "Não foi possível validar a sessão renovada. Tentaremos novamente."
		ctx.After(sessionRefreshRetry, func(next app.Context) { p.retryCallerValidation(next) })
		return
	}
	p.caller = &caller
	p.offlineMode = false
	p.teamLoadedFor, p.portalLoadedFor = "", ""
	p.restoreBrowserPushState(ctx)
	p.authNotice = ""
	p.loadCustomerPortal(ctx)
	p.loadTeamOperations(ctx)
	p.loadWhatsAppQueueIfSelected(ctx)
	p.scheduleSessionRefresh(ctx)
}

// retryCallerValidation rechecks the refreshed access token without rotating the
// refresh token again when only the profile request had a temporary failure.
func (p *serviceCatalogPage) retryCallerValidation(ctx app.Context) {
	if p.session == nil || p.refreshingSession {
		return
	}
	p.refreshingSession = true
	client, err := publicSupabaseClient()
	if err == nil {
		var caller supabase.Caller
		caller, err = client.AuthenticateCaller(ctx, p.session.AccessToken)
		if err == nil {
			p.caller = &caller
			p.offlineMode = false
			p.teamLoadedFor, p.portalLoadedFor = "", ""
			p.restoreBrowserPushState(ctx)
			p.authNotice = ""
			p.loadCustomerPortal(ctx)
			p.loadTeamOperations(ctx)
			p.loadWhatsAppQueueIfSelected(ctx)
			p.refreshingSession = false
			p.scheduleSessionRefresh(ctx)
			return
		}
	}
	p.refreshingSession = false
	if errors.Is(err, supabase.ErrUnauthorized) || isRejectedAuthSession(err) {
		p.clearSession(ctx)
		p.authNotice = "Sua sessão expirou. Entre novamente para continuar."
		return
	}
	p.authNotice = "Não foi possível validar a sessão. Tentaremos novamente."
	ctx.After(sessionRefreshRetry, func(next app.Context) { p.retryCallerValidation(next) })
}

func isRejectedAuthSession(err error) bool {
	var authErr *supabase.AuthError
	return errors.As(err, &authErr) && (authErr.StatusCode == 400 || authErr.StatusCode == 401 || authErr.StatusCode == 403)
}

func (p *serviceCatalogPage) openAuth(ctx app.Context, event app.Event) {
	event.PreventDefault()
	p.authMode = "login"
	p.authOpen = true
	p.authError = ""
	p.authSuccess = ""
}

func (p *serviceCatalogPage) openSignup(ctx app.Context, event app.Event) {
	event.PreventDefault()
	p.authMode = "signup"
	p.authError = ""
	p.authSuccess = ""
	p.authOpen = true
}

func (p *serviceCatalogPage) closeAuth(ctx app.Context, event app.Event) {
	event.PreventDefault()
	p.authOpen = false
	p.authError = ""
	p.authSuccess = ""
	p.password = ""
}

func (p *serviceCatalogPage) toggleRecovery(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.authMode == "forgot" {
		p.authMode = "login"
	} else {
		p.authMode = "forgot"
	}
	p.authError = ""
	p.authSuccess = ""
}

func (p *serviceCatalogPage) processAuthRedirect(ctx app.Context) {
	pageURL := app.Window().URL()
	if pageURL == nil {
		return
	}
	if stripCredentialQuery(pageURL) {
		ctx.Page().ReplaceURL(pageURL)
	}
	p.processAuthURL(ctx, pageURL)
}

func (p *serviceCatalogPage) processNativeAuthRedirect(ctx app.Context, rawURL string) {
	if !isNativeOAuthCallbackURL(rawURL) {
		return
	}
	pageURL, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	p.processAuthURL(ctx, pageURL)
}

func (p *serviceCatalogPage) processAuthURL(ctx app.Context, pageURL *url.URL) {
	fragment, _ := url.ParseQuery(strings.TrimPrefix(pageURL.Fragment, "#"))
	accessToken := fragment.Get("access_token")
	refreshToken := fragment.Get("refresh_token")
	flowType := fragment.Get("type")
	if accessToken == "" || refreshToken == "" {
		if message := authRedirectError(pageURL); message != "" {
			p.authOpen, p.authMode = true, "login"
			p.authError = message
			cleanAuthURL(ctx)
		}
		return
	}
	client, err := publicSupabaseClient()
	if err != nil {
		p.authNotice = "A autenticação ainda não está configurada neste ambiente."
		return
	}
	session, err := client.SessionFromAccessToken(ctx, accessToken)
	if err != nil {
		p.authOpen, p.authMode = true, "login"
		p.authError = "Não foi possível validar o link de acesso. Solicite um novo link e tente novamente."
		cleanAuthURL(ctx)
		return
	}
	session.RefreshToken = refreshToken
	if seconds, parseErr := time.ParseDuration(fragment.Get("expires_in") + "s"); parseErr == nil {
		session.ExpiresIn = int64(seconds / time.Second)
		session.ExpiresAt = time.Now().Add(seconds).Unix()
	}
	session.ProviderToken = fragment.Get("provider_token")
	session.ProviderRefreshToken = fragment.Get("provider_refresh_token")
	caller, err := client.AuthenticateCaller(ctx, session.AccessToken)
	if err != nil {
		p.authOpen, p.authMode = true, "login"
		p.authError = "Não foi possível validar o perfil desta conta."
		cleanAuthURL(ctx)
		return
	}
	if err := saveSupabaseSession(ctx, session); err != nil {
		p.authNotice = "Não foi possível salvar a sessão neste navegador."
		return
	}
	p.session, p.caller = &session, &caller
	p.recoveryRequired = metadataMustChange(session.User)
	p.loadCustomerPortal(ctx)
	p.loadTeamOperations(ctx)
	p.loadWhatsAppQueueIfSelected(ctx)
	if pageURL.Query().Get("googleCalendar") == "1" && (caller.Role == domain.RoleAdmin || caller.Role == domain.RoleTechnician) {
		p.resumeGoogleCalendarConnection(ctx, session)
	}
	if pageURL.Query().Get("googleEmail") == "1" && (caller.Role == domain.RoleAdmin || caller.Role == domain.RoleTechnician) {
		p.resumeGoogleEmailConnection(ctx, session)
	}
	if pageURL.Query().Get("googleContacts") == "1" && (caller.Role == domain.RoleAdmin || caller.Role == domain.RoleTechnician) && session.ProviderToken != "" {
		p.authNotice = "Contatos Google autorizados. Abra o cadastro de cliente e pesquise um contato."
	}
	p.recoveryOpen = flowType == "recovery" || pageURL.Query().Get("recovery") == "1" || p.recoveryRequired
	cleanAuthURL(ctx)
	p.scheduleSessionRefresh(ctx)
}

func authRedirectError(pageURL *url.URL) string {
	if pageURL == nil {
		return ""
	}
	fragment, _ := url.ParseQuery(strings.TrimPrefix(pageURL.Fragment, "#"))
	for _, values := range []url.Values{fragment, pageURL.Query()} {
		if message := strings.TrimSpace(values.Get("error_description")); message != "" {
			return message
		}
		if message := strings.TrimSpace(values.Get("error")); message != "" {
			return message
		}
	}
	return ""
}

func cleanAuthURL(ctx app.Context) {
	pageURL := app.Window().URL()
	if pageURL == nil {
		return
	}
	pageURL.RawQuery = ""
	pageURL.Fragment = ""
	ctx.Page().ReplaceURL(pageURL)
}

func stripCredentialQuery(pageURL *url.URL) bool {
	if pageURL == nil {
		return false
	}
	query := pageURL.Query()
	changed := false
	for _, key := range []string{"email", "password", "senha"} {
		for existing := range query {
			if strings.EqualFold(existing, key) {
				query.Del(existing)
				changed = true
			}
		}
	}
	if changed {
		pageURL.RawQuery = query.Encode()
	}
	return changed
}

func metadataMustChange(user *supabase.AuthUser) bool {
	if user == nil || user.UserMetadata == nil {
		return false
	}
	return string(user.UserMetadata["must_change_password"]) == "true"
}

func (p *serviceCatalogPage) submitAuth(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.authBusy {
		return
	}
	p.authBusy = true
	p.authError = ""
	p.authSuccess = ""
	client, err := publicSupabaseClient()
	if err != nil {
		p.authBusy = false
		p.authError = "A autenticação ainda não está configurada neste ambiente."
		return
	}

	if p.authMode == "forgot" {
		err = client.RequestPasswordReset(ctx, p.email, recoveryRedirectURL())
		p.authBusy = false
		if err != nil {
			p.authError = "Não foi possível enviar o link agora. Aguarde alguns minutos e tente novamente."
			return
		}
		p.authSuccess = "Se houver uma conta com este e-mail, você receberá um link para criar uma nova senha. Confira também o spam."
		return
	}
	if p.authMode == "signup" {
		p.submitSignup(ctx, client)
		return
	}

	session, err := client.SignInWithPassword(ctx, p.email, p.password)
	if err != nil {
		p.authBusy = false
		p.authError = authMessage(err, "Erro ao realizar login. Verifique seu e-mail e senha.")
		return
	}
	caller, err := client.AuthenticateCaller(ctx, session.AccessToken)
	if err != nil {
		p.authBusy = false
		p.authError = authMessage(err, "Não foi possível validar o perfil desta conta.")
		return
	}
	if err := saveSupabaseSession(ctx, session); err != nil {
		p.authBusy = false
		p.authError = "Não foi possível salvar a sessão neste navegador. Verifique as permissões de armazenamento."
		return
	}

	p.session = &session
	p.caller = &caller
	p.restoreBrowserPushState(ctx)
	p.recoveryRequired = metadataMustChange(session.User)
	p.loadCustomerPortal(ctx)
	p.loadTeamOperations(ctx)
	p.loadWhatsAppQueueIfSelected(ctx)
	p.recoveryOpen = p.recoveryRequired
	p.authBusy = false
	p.password = ""
	p.authSuccess = "Login realizado com sucesso!"
	p.authNotice = ""
	p.scheduleSessionRefresh(ctx)
	ctx.After(700*time.Millisecond, func(next app.Context) {
		p.authOpen = false
		next.Update()
	})
}

func (p *serviceCatalogPage) submitSignup(ctx app.Context, client *supabase.Client) {
	if len(p.password) < 6 {
		p.authBusy = false
		p.authError = "A senha deve ter no mínimo 6 caracteres."
		return
	}
	if strings.TrimSpace(p.fullName) == "" {
		p.authBusy = false
		p.authError = "Informe seu nome completo."
		return
	}
	body, _ := json.Marshal(map[string]string{
		"acao": "minha_conta", "email": strings.TrimSpace(p.email), "senha": p.password,
		"nome": strings.TrimSpace(p.fullName), "whatsapp": strings.TrimSpace(p.whatsapp),
		"endereco": strings.TrimSpace(p.address), "bairro": strings.TrimSpace(p.neighborhood), "cidade": strings.TrimSpace(p.city),
	})
	whatsappNotice := ""
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/api/contas", bytes.NewReader(body))
	if err == nil {
		req.Header.Set("Content-Type", "application/json")
		var response *http.Response
		response, err = http.DefaultClient.Do(req)
		if err == nil {
			defer response.Body.Close()
			var result struct {
				Error          string `json:"error"`
				WhatsAppNotice string `json:"whatsapp_aviso"`
			}
			_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result)
			if response.StatusCode < 200 || response.StatusCode >= 300 {
				p.authBusy = false
				p.authError = result.Error
				if p.authError == "" {
					p.authError = "Erro ao criar sua conta. Tente novamente."
				}
				return
			}
			whatsappNotice = result.WhatsAppNotice
		}
	}
	if err != nil {
		p.authBusy = false
		p.authError = authMessage(err, "Erro ao criar sua conta. Tente novamente.")
		return
	}
	session, err := client.SignInWithPassword(ctx, p.email, p.password)
	if err != nil {
		p.authBusy = false
		p.authError = "A conta foi criada. Entre com seu e-mail e senha para continuar."
		p.authMode = "login"
		return
	}
	message := "Conta criada com sucesso! Entrando no seu portal..."
	if whatsappNotice != "" {
		message += " " + whatsappNotice
	}
	p.finishSignIn(ctx, session, message)
}

func (p *serviceCatalogPage) finishSignIn(ctx app.Context, session supabase.AuthSession, message string) {
	client, err := publicSupabaseClient()
	if err != nil {
		p.authBusy = false
		p.authError = "A autenticação ainda não está configurada neste ambiente."
		return
	}
	caller, err := client.AuthenticateCaller(ctx, session.AccessToken)
	if err != nil {
		p.authBusy = false
		p.authError = "Não foi possível validar o perfil desta conta."
		return
	}
	if err := saveSupabaseSession(ctx, session); err != nil {
		p.authBusy = false
		p.authError = "Não foi possível salvar a sessão neste navegador."
		return
	}
	p.session, p.caller, p.authBusy = &session, &caller, false
	p.recoveryRequired = metadataMustChange(session.User)
	p.loadCustomerPortal(ctx)
	p.recoveryOpen = p.recoveryRequired
	p.password, p.authSuccess, p.authNotice = "", message, ""
	p.scheduleSessionRefresh(ctx)
	ctx.After(700*time.Millisecond, func(next app.Context) { p.authOpen = false; next.Update() })
}

func (p *serviceCatalogPage) loginGoogle(ctx app.Context, event app.Event) {
	event.PreventDefault()
	client, err := publicSupabaseClient()
	if err != nil {
		p.authError = "A autenticação ainda não está configurada neste ambiente."
		return
	}
	redirect, err := client.SignInWithOAuthURL("google", oauthRedirectURL())
	if err != nil {
		p.authError = "Não foi possível iniciar o login com Google."
		return
	}
	p.launchOAuth(ctx, redirect)
}

func oauthRedirectURL() string {
	if desktopWailsAvailable() {
		return desktopOAuthRedirectURL(nil)
	}
	return oauthRedirectURLFor(app.Window().URL(), nil, nativeOAuthAvailable())
}

func (p *serviceCatalogPage) launchOAuth(ctx app.Context, authorizeURL string) {
	if !launchNativeOAuth(authorizeURL) {
		ctx.Navigate(authorizeURL)
	}
}

func (p *serviceCatalogPage) submitPasswordRecovery(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.authBusy {
		return
	}
	p.authError = ""
	if len(p.password) < 8 {
		p.authError = "Use pelo menos 8 caracteres."
		return
	}
	if p.password != p.confirmation {
		p.authError = "As senhas não são iguais."
		return
	}
	if p.session == nil {
		p.authError = "Este link expirou. Solicite outro em Esqueci minha senha."
		return
	}
	password := p.password
	p.authBusy = true
	ctx.Update()
	go func() {
		client, err := publicSupabaseClient()
		if err == nil {
			err = client.UpdatePasswordWithMetadata(ctx, p.session.AccessToken, password, map[string]any{"must_change_password": false})
		}
		p.authBusy = false
		if err != nil {
			p.authError = authMessage(err, "Não foi possível salvar a senha. Verifique sua conexão e tente novamente.")
			ctx.Update()
			return
		}
		p.password, p.confirmation = "", ""
		p.recoveryDone, p.recoveryRequired = true, false
		if p.session.User != nil {
			if p.session.User.UserMetadata == nil {
				p.session.User.UserMetadata = map[string]json.RawMessage{}
			}
			p.session.User.UserMetadata["must_change_password"], _ = json.Marshal(false)
			_ = saveSupabaseSession(ctx, *p.session)
		}
		cleanAuthURL(ctx)
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) closePasswordRecovery(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.recoveryRequired && !p.recoveryDone {
		return
	}
	p.recoveryOpen, p.recoveryDone = false, false
	p.authError = ""
}

func (p *serviceCatalogPage) passwordRecoveryModal() app.UI {
	title, intro := "Criar nova senha", "Escolha uma senha de pelo menos 8 caracteres."
	if p.recoveryRequired {
		title, intro = "Troca de senha obrigatória", "Por segurança, crie uma senha pessoal antes de acessar o painel."
	}
	if p.recoveryDone {
		title, intro = "Senha atualizada", "Sua nova senha já pode ser usada para acessar o app."
	}
	items := []app.UI{app.P().Class("auth-dialog__intro").Body(app.Text(intro))}
	if !p.recoveryDone {
		items = append(items,
			p.passwordField("Nova senha", "new-password", "Pelo menos 8 caracteres", &p.password, &p.passwordVisible),
			p.passwordField("Confirmar nova senha", "new-password", "Repita a nova senha", &p.confirmation, &p.confirmationVisible),
		)
	}
	if p.authError != "" {
		items = append(items, app.Div().Class("auth-message auth-message--error").Role("alert").Body(app.Text(p.authError)))
	}
	if p.recoveryDone {
		items = append(items, app.Button().Class("auth-submit").Type("button").OnClick(p.closePasswordRecovery).Body(app.Text("Continuar no app")))
	} else {
		submitLabel := recoverySubmitLabel(p.authBusy)
		items = append(items, app.Form().Class("auth-form").OnSubmit(p.submitPasswordRecovery).Body(
			app.Button().Class("auth-submit").Type("submit").Disabled(p.authBusy).Body(app.Text(submitLabel)),
			app.Button().Class("auth-link").Type("button").Disabled(p.recoveryRequired || p.authBusy).OnClick(p.closePasswordRecovery).Body(app.Text("Cancelar")),
		))
	}
	modalItems := []app.UI{app.Div().Class("auth-dialog__heading").Body(app.H2().Class("auth-dialog__title").Body(app.Text(title)))}
	modalItems = append(modalItems, items...)
	return app.Div().Class("auth-backdrop auth-backdrop--recovery").Body(app.Div().Class("auth-dialog").Role("dialog").Body(modalItems...))
}

func recoverySubmitLabel(busy bool) string {
	if busy {
		return "Salvando..."
	}
	return "Salvar nova senha"
}

func (p *serviceCatalogPage) setSignupPostalCode(ctx app.Context, event app.Event) {
	p.postalCode = cep.Mask(event.Get("target").Get("value").String())
	p.signupPostalGeneration++
	generation := p.signupPostalGeneration
	p.signupPostalSuggestion = nil
	p.signupPostalLoading = false
	digits := cep.Digits(p.postalCode)
	switch {
	case digits == "":
		p.signupPostalMessage = ""
	case len(digits) < 8:
		p.signupPostalMessage = fmt.Sprintf("Digite mais %d dígitos para localizar o endereço.", 8-len(digits))
	default:
		p.signupPostalMessage = "Buscando endereço pelo CEP..."
	}
	ctx.Update()
	if len(digits) == 8 {
		p.lookupPostalCode(ctx, digits, generation)
	}
}

func (p *serviceCatalogPage) lookupPostalCode(ctx app.Context, digits string, generation uint64) {
	if len(cep.Digits(digits)) != 8 || generation != p.signupPostalGeneration {
		return
	}
	p.signupPostalLoading = true
	p.signupPostalMessage = "Buscando endereço pelo CEP..."
	ctx.Update()
	go func() {
		address, err := cep.Lookup(ctx, digits, nil, "")
		if generation != p.signupPostalGeneration || cep.Digits(p.postalCode) != digits {
			return
		}
		p.signupPostalLoading = false
		if err != nil {
			p.signupPostalMessage = "Não encontrei esse CEP. Confira os números ou preencha o endereço manualmente."
			ctx.Update()
			return
		}
		p.signupPostalSuggestion = &address
		if address.Street != "" {
			p.address = address.Street
		}
		if address.Neighborhood != "" {
			p.neighborhood = address.Neighborhood
		}
		if address.City != "" {
			p.city = address.City
		}
		p.signupPostalMessage = "Endereço localizado. Você também pode selecionar a sugestão abaixo."
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) selectSignupPostalSuggestion(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if address := p.signupPostalSuggestion; address != nil {
		if address.Street != "" {
			p.address = address.Street
		}
		if address.Neighborhood != "" {
			p.neighborhood = address.Neighborhood
		}
		if address.City != "" {
			p.city = address.City
		}
		p.signupPostalMessage = "Sugestão de endereço selecionada; os campos continuam editáveis."
		ctx.Update()
	}
}

func (p *serviceCatalogPage) signOut(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.authBusy {
		return
	}
	if p.pushBusy {
		p.authNotice = "Aguarde o término da atualização das notificações antes de sair."
		ctx.Update()
		return
	}
	p.authBusy = true
	var signOutErr error
	userID := ""
	if p.caller != nil {
		userID = p.caller.UserID
	}
	if p.session != nil && browserPushSupported() {
		signOutErr = p.removeBrowserPushForAccount(ctx, p.session.AccessToken)
	}
	if p.session != nil {
		if client, err := publicSupabaseClient(); err != nil {
			if signOutErr == nil {
				signOutErr = err
			}
		} else {
			if err := client.SignOut(ctx, p.session.AccessToken); err != nil && signOutErr == nil {
				signOutErr = err
			}
		}
	}
	deleteSupabaseSession(ctx)
	if userID != "" {
		go func() { _ = deleteOfflineSnapshot(userID) }()
	}
	p.session = nil
	p.caller = nil
	p.offlineMode = false
	p.pushBusy, p.pushChecking, p.pushEnabled, p.pushNotice = false, false, false, ""
	p.portalData, p.portalLoadedFor = nil, ""
	p.portalServiceStatuses = nil
	p.portalBusinessWhats, p.portalBusinessWhatsLoaded, p.portalBusinessWhatsLoading = "", "", false
	p.portalProfilePhotoURL, p.portalProfilePhotoError, p.portalProfilePhotoNotice, p.portalProfilePhotoLoaded = "", "", "", ""
	p.portalProfilePhotoBusy, p.portalProfilePhotoConfirm = false, false
	p.teamProfilePhotoURL, p.teamProfilePhotoLoaded, p.teamProfilePhotoError = "", "", ""
	p.teamProfilePhotoBusy, p.teamProfilePhotoConfirm = false, false
	p.teamServices, p.teamAppointments, p.teamLoadedFor = nil, nil, ""
	p.teamActiveSection = "dashboard"
	p.teamCompletion, p.teamProfile = nil, domain.TechnicianProfile{}
	p.teamNewAppointment, p.teamNewAppointmentSaving, p.teamNewAppointmentNotice = nil, false, ""
	p.teamCatalogForm, p.teamCatalogConfirmKey, p.teamCatalogSaving = nil, "", false
	p.teamDirectServiceForm, p.teamDirectServiceSaving = nil, false
	p.teamCallScheduleServiceID, p.teamCallScheduleMessage = "", ""
	p.teamBudgetScheduleID, p.teamBudgetScheduleMessage = "", ""
	p.teamReceiptURL = ""
	p.password = ""
	p.authBusy = false
	if signOutErr != nil {
		p.authNotice = "A sessão foi removida deste dispositivo, mas não foi possível confirmar o encerramento remoto."
	}
}

func (p *serviceCatalogPage) clearSession(ctx app.Context) {
	if browserPushSupported() {
		_ = unsubscribeBrowserPush()
	}
	userID := ""
	if p.caller != nil {
		userID = p.caller.UserID
	} else if p.session != nil && p.session.User != nil {
		userID = p.session.User.ID
	}
	deleteSupabaseSession(ctx)
	if userID != "" {
		go func() { _ = deleteOfflineSnapshot(userID) }()
	}
	p.session = nil
	p.caller = nil
	p.offlineMode = false
	p.portalData, p.portalLoadedFor = nil, ""
	p.portalServiceStatuses = nil
	p.portalBusinessWhats, p.portalBusinessWhatsLoaded, p.portalBusinessWhatsLoading = "", "", false
	p.portalProfilePhotoURL, p.portalProfilePhotoError, p.portalProfilePhotoNotice, p.portalProfilePhotoLoaded = "", "", "", ""
	p.portalProfilePhotoBusy, p.portalProfilePhotoConfirm = false, false
	p.teamProfilePhotoURL, p.teamProfilePhotoLoaded, p.teamProfilePhotoError = "", "", ""
	p.teamProfilePhotoBusy, p.teamProfilePhotoConfirm = false, false
	p.teamServices, p.teamAppointments, p.teamLoadedFor = nil, nil, ""
	p.teamActiveSection = "dashboard"
	p.teamCompletion, p.teamProfile = nil, domain.TechnicianProfile{}
	p.teamNewAppointment, p.teamNewAppointmentSaving, p.teamNewAppointmentNotice = nil, false, ""
	p.teamCatalogForm, p.teamCatalogConfirmKey, p.teamCatalogSaving = nil, "", false
	p.teamDirectServiceForm, p.teamDirectServiceSaving = nil, false
	p.teamCallScheduleServiceID, p.teamCallScheduleMessage = "", ""
	p.teamBudgetScheduleID, p.teamBudgetScheduleMessage = "", ""
	p.teamReceiptURL = ""
	p.caller = nil
	p.pushBusy, p.pushChecking, p.pushEnabled, p.pushNotice = false, false, false, ""
	p.localNotificationsEnabled = false
	p.locationBusy, p.locationCoordinates, p.locationAccuracy, p.locationNotice = false, "", "", ""
	p.password = ""
}

func authMessage(err error, fallback string) string {
	var authErr *supabase.AuthError
	if errors.As(err, &authErr) && authErr.Message != "" {
		return authErr.Message
	}
	if errors.Is(err, supabase.ErrUnauthorized) {
		return "Não foi possível validar o perfil desta conta."
	}
	if strings.TrimSpace(err.Error()) == "" {
		return fallback
	}
	return fallback
}

func recoveryRedirectURL() string {
	pageURL := app.Window().URL()
	if pageURL == nil || pageURL.Scheme == "" || pageURL.Host == "" {
		return ""
	}
	return (&url.URL{Scheme: pageURL.Scheme, Host: pageURL.Host, Path: "/", RawQuery: "recovery=1"}).String()
}

func (p *serviceCatalogPage) accountControl() app.UI {
	if p.session == nil || p.caller == nil {
		return app.Span()
	}
	identity := string(p.caller.Role)
	if p.session.User != nil && p.session.User.Email != "" {
		identity = p.session.User.Email
	}
	items := []app.UI{}
	if p.caller.Role == domain.RoleAdmin || p.caller.Role == domain.RoleTechnician {
		items = append(items, app.Button().Class("topbar__settings").Type("button").Attr("title", "Configurações do app").Attr("aria-label", "Abrir configurações").OnClick(p.openTeamSettings).Body(app.Text("⚙")))
		photo := app.UI(app.Span().Body(app.Text("📷")))
		if p.teamProfilePhotoURL != "" {
			photo = app.Img().Class("topbar__profile-image").Src(p.teamProfilePhotoURL).Alt("Foto de perfil")
		}
		if p.teamProfilePhotoBusy {
			photo = app.Span().Class("topbar__profile-loading").Body(app.Text("…"))
		}
		if p.teamProfilePhotoError != "" {
			photo = app.Span().Attr("title", p.teamProfilePhotoError).Body(app.Text("!"))
		}
		items = append(items, app.Button().Class("topbar__profile").Type("button").Disabled(p.teamProfilePhotoBusy).Attr("title", "Trocar minha foto de perfil").OnClick(p.changeTeamProfilePhoto).Body(photo))
		if p.teamProfilePhotoURL != "" {
			items = append(items, app.Button().Class("topbar__profile-remove").Type("button").Disabled(p.teamProfilePhotoBusy).Attr("title", "Remover foto").OnClick(p.removeTeamProfilePhoto).Body(app.Text("×")))
		}
	}
	items = append(items,
		app.Span().Class("topbar__identity").Body(app.Text(identity)),
		app.Button().Class("topbar__logout").Type("button").OnClick(p.signOut).Body(app.Text("Sair")),
	)
	return app.Div().Class("topbar__account").Body(items...)
}

func (p *serviceCatalogPage) authModal() app.UI {
	if p.authMode == "" {
		p.authMode = "login"
	}
	title := "Entrar no App"
	intro := "Acesse sua conta Inovar para continuar."
	if p.authMode == "forgot" {
		title = "Recuperar minha senha"
		intro = "Informe o e-mail da sua conta para receber o link de recuperação."
	}
	if p.authMode == "signup" {
		title = "Novo Cliente"
		intro = "Crie sua conta para acompanhar seus atendimentos e orçamentos."
	}
	formItems := []app.UI{
		app.P().Class("auth-dialog__intro").Body(app.Text(intro)),
		app.Label().Class("auth-field").Body(
			app.Text("E-mail"),
			app.Input().Type("email").Name("email").Required(true).Value(p.email).Placeholder("seu@email.com").OnChange(p.ValueTo(&p.email)),
		),
	}
	if p.authMode == "login" {
		formItems = append(formItems, app.Label().Class("auth-field").Body(app.Text("Senha"), app.Div().Class("auth-password-control").Body(
			app.Input().Type(map[bool]string{true: "text", false: "password"}[p.passwordVisible]).Name("password").Required(true).Value(p.password).Placeholder("Sua senha").OnChange(p.ValueTo(&p.password)),
			app.Button().Class("auth-password-toggle").Type("button").Attr("aria-label", map[bool]string{true: "Ocultar senha", false: "Mostrar senha"}[p.passwordVisible]).Attr("aria-pressed", ariaBoolean(p.passwordVisible)).OnClick(p.togglePasswordVisibility(&p.passwordVisible)).Body(app.Text(map[bool]string{true: "Ocultar", false: "Mostrar"}[p.passwordVisible])),
		)))
	}
	if p.authMode == "signup" {
		cepInput := []app.UI{app.Input().Type("text").Attr("inputmode", "numeric").Value(p.postalCode).Placeholder("00000-000").OnInput(p.setSignupPostalCode)}
		if p.signupPostalLoading {
			cepInput = append(cepInput, app.Span().Class("auth-cep-spinner").Attr("role", "status").Attr("aria-label", "Buscando CEP").Body(app.Text(" ")))
		}
		formItems = append(formItems,
			p.passwordField("Senha", "new-password", "Mínimo de 6 caracteres", &p.password, &p.passwordVisible),
			app.Label().Class("auth-field").Body(app.Text("Nome completo"), app.Input().Type("text").Required(true).Value(p.fullName).Placeholder("Seu nome").OnChange(p.ValueTo(&p.fullName))),
			app.Label().Class("auth-field").Body(app.Text("WhatsApp"), app.Input().Type("tel").Required(true).Value(p.whatsapp).Placeholder("(27) 99999-9999").OnChange(p.ValueTo(&p.whatsapp))),
			app.Label().Class("auth-field").Body(app.Text("CEP"), app.Div().Class("auth-cep-input-wrap").Body(cepInput...)),
			cepSearchFeedback(p.signupPostalMessage),
			cepAddressSuggestion(p.signupPostalSuggestion, p.selectSignupPostalSuggestion),
			app.Label().Class("auth-field").Body(app.Text("Endereço"), app.Input().Type("text").Value(p.address).Placeholder("Rua e número").OnChange(p.ValueTo(&p.address))),
			app.Label().Class("auth-field").Body(app.Text("Bairro"), app.Input().Type("text").Value(p.neighborhood).OnChange(p.ValueTo(&p.neighborhood))),
			app.Label().Class("auth-field").Body(app.Text("Cidade"), app.Input().Type("text").Value(p.city).OnChange(p.ValueTo(&p.city))),
		)
	}
	if p.authError != "" {
		formItems = append(formItems, app.Div().Class("auth-message auth-message--error").Body(app.Text(p.authError)))
	}
	if p.authSuccess != "" {
		formItems = append(formItems, app.Div().Class("auth-message auth-message--success").Body(app.Text(p.authSuccess)))
	}
	buttonLabel := "Entrar"
	if p.authBusy {
		buttonLabel = "Aguarde..."
	} else if p.authMode == "forgot" {
		buttonLabel = "Enviar link de recuperação"
	} else if p.authMode == "signup" {
		buttonLabel = "Criar minha conta"
	}
	formItems = append(formItems, app.Button().Class("auth-submit").Type("submit").Disabled(p.authBusy).Body(app.Text(buttonLabel)))
	if p.authMode == "login" {
		formItems = append(formItems,
			app.Button().Class("auth-link").Type("button").OnClick(p.toggleRecovery).Body(app.Text("Esqueci minha senha")),
			app.Button().Class("auth-link").Type("button").OnClick(p.openSignup).Body(app.Text("Novo Cliente")),
			app.Button().Class("auth-link auth-link--google").Type("button").OnClick(p.loginGoogle).Body(app.Text("Continuar com Google")),
		)
	} else if p.authMode == "signup" {
		formItems = append(formItems, app.Button().Class("auth-link").Type("button").OnClick(p.openAuth).Body(app.Text("Já tenho conta")))
	} else {
		formItems = append(formItems, app.Button().Class("auth-link").Type("button").OnClick(p.toggleRecovery).Body(app.Text("Voltar ao login")))
	}
	return app.Div().Class("auth-backdrop").Body(
		app.Div().Class("auth-dialog").Role("dialog").Body(
			app.Div().Class("auth-dialog__heading").Body(
				app.Div().Body(
					app.P().Class("brand__name").Body(app.Text("INOVAR APP")),
					app.H2().Class("auth-dialog__title").Body(app.Text(title)),
				),
				func() app.UI {
					if p.caller == nil {
						return app.Div()
					}
					return app.Button().Class("auth-close").Type("button").OnClick(p.closeAuth).Body(app.Text("Fechar"))
				}(),
			),
			app.Form().Class("auth-form").OnSubmit(p.submitAuth).Body(formItems...),
		),
	)
}

func recoveryToggleLabel(mode string) string {
	if mode == "forgot" {
		return "Voltar ao login"
	}
	return "Esqueci minha senha"
}
