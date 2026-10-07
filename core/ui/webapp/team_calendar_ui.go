package webapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/adapter/supabase"
)

func (p *serviceCatalogPage) loadTeamCalendar(ctx app.Context) {
	if p.session == nil {
		return
	}
	pageURL := app.Window().URL()
	if pageURL == nil || pageURL.Scheme == "" || pageURL.Host == "" {
		return
	}
	base, token, userID := apiBaseURL(), p.session.AccessToken, p.caller.UserID
	go func() {
		result, err := teamCalendarRequest(ctx, base+"/api/google-calendar", token, http.MethodGet, nil)
		if p.session == nil || p.caller == nil || p.caller.UserID != userID {
			return
		}
		if err != nil {
			// Keep the internal agenda fully usable when the optional Google
			// integration cannot be reached; do not turn a background status
			// check into an alarming page-level error.
			p.teamCalendarConnected = false
			p.teamCalendarNotice = "Google Agenda indisponível no momento. A agenda interna continua funcionando."
		} else {
			p.teamCalendarConnected, _ = result["connected"].(bool)
			p.teamCalendarLastSync = portalText(result["lastSync"])
			p.teamCalendarNotice = ""
		}
		ctx.Update()
	}()
}

func teamCalendarLastSyncLabel(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return " — última atualização: " + value
	}
	return " — última atualização: " + parsed.Local().Format("02/01 15:04")
}

func (p *serviceCatalogPage) connectOrSyncGoogleCalendar(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.teamCalendarBusy || p.session == nil {
		return
	}
	if p.teamCalendarConnected {
		p.syncGoogleCalendar(ctx)
		return
	}
	client, err := publicSupabaseClient()
	if err != nil {
		p.teamCalendarNotice = "A conexão com Google não está configurada."
		return
	}
	pageURL := app.Window().URL()
	if pageURL == nil {
		return
	}
	redirect := oauthRedirectURLFor(pageURL, url.Values{"googleCalendar": {"1"}}, nativeOAuthAvailable())
	params := url.Values{"access_type": {"offline"}, "prompt": {"consent"}}
	if p.session.User != nil && p.session.User.Email != "" {
		params.Set("login_hint", p.session.User.Email)
	}
	const scopes = "email profile https://www.googleapis.com/auth/calendar.events"
	var providerLinked bool
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
				providerLinked = true
				break
			}
		}
	}
	var authURL string
	if providerLinked {
		authURL, err = client.SignInWithOAuthURLAndScopes("google", redirect, scopes, params)
	} else {
		authURL, err = client.LinkIdentityOAuthURL(ctx, p.session.AccessToken, "google", redirect, scopes, params)
	}
	if err != nil {
		p.teamCalendarNotice = "Não foi possível iniciar a autorização do Google Agenda."
		return
	}
	p.launchOAuth(ctx, authURL)
}

func (p *serviceCatalogPage) syncGoogleCalendar(ctx app.Context) {
	p.performTeamCalendarSync(ctx, true)
}

func (p *serviceCatalogPage) performTeamCalendarSync(ctx app.Context, manual bool) {
	if p.session == nil || p.teamCalendarBusy || (!manual && !p.teamCalendarConnected) {
		return
	}
	pageURL := app.Window().URL()
	if pageURL == nil || pageURL.Scheme == "" || pageURL.Host == "" {
		return
	}
	base, token := apiBaseURL(), p.session.AccessToken
	userID := ""
	if p.caller != nil {
		userID = p.caller.UserID
	}
	p.teamCalendarBusy = true
	if manual {
		p.teamCalendarNotice = "Sincronizando agendamentos..."
	}
	go func() {
		result, err := teamCalendarRequest(ctx, base+"/api/google-calendar", token, http.MethodPost, map[string]any{"action": "sync"})
		if p.session == nil || p.caller == nil || p.caller.UserID != userID {
			return
		}
		p.teamCalendarBusy = false
		if err != nil {
			p.teamCalendarNotice = "Não foi possível sincronizar. Reconecte a agenda e tente novamente."
		} else {
			p.teamCalendarConnected, _ = result["connected"].(bool)
			p.teamCalendarLastSync = portalText(result["lastSync"])
			if manual {
				p.teamCalendarNotice = fmt.Sprintf("Sincronização concluída: %v alteração(ões).", result["changed"])
			}
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) resumeGoogleCalendarConnection(ctx app.Context, session supabase.AuthSession) {
	if session.ProviderToken == "" {
		p.teamCalendarNotice = "O Google não retornou autorização para a agenda. Conecte novamente."
		return
	}
	pageURL := app.Window().URL()
	if pageURL == nil || pageURL.Scheme == "" || pageURL.Host == "" {
		return
	}
	base, token := apiBaseURL(), session.AccessToken
	userID := ""
	if p.caller != nil {
		userID = p.caller.UserID
	}
	p.teamCalendarBusy = true
	p.teamCalendarNotice = "Conectando ao Google Agenda..."
	go func() {
		_, err := teamCalendarRequest(ctx, base+"/api/google-calendar", token, http.MethodPost, map[string]any{"action": "connect", "providerToken": session.ProviderToken, "providerRefreshToken": session.ProviderRefreshToken})
		if p.session == nil || p.caller == nil || p.caller.UserID != userID {
			return
		}
		p.teamCalendarBusy = false
		if err != nil {
			p.teamCalendarNotice = "Não foi possível conectar o Google Agenda. Verifique a conta e as permissões."
		} else {
			p.teamCalendarConnected = true
			p.teamCalendarNotice = "Google Agenda conectado. Use “Sincronizar agora” para enviar os agendamentos."
		}
		ctx.Update()
	}()
}

func teamCalendarRequest(ctx context.Context, endpoint, token, method string, payload any) (map[string]any, error) {
	var body *strings.Reader
	if payload == nil {
		body = strings.NewReader("")
	} else {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = strings.NewReader(string(encoded))
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("calendar request returned %d", response.StatusCode)
	}
	var result map[string]any
	if response.ContentLength != 0 {
		err = json.NewDecoder(response.Body).Decode(&result)
	}
	return result, err
}
