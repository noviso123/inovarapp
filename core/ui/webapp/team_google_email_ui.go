package webapp

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/adapter/supabase"
)

func (p *serviceCatalogPage) connectTeamGoogleEmail(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.session == nil || p.teamEmailBusy {
		return
	}
	client, err := publicSupabaseClient()
	if err != nil {
		p.teamEmailNotice = "A conexão com Google não está configurada."
		ctx.Update()
		return
	}
	pageURL := app.Window().URL()
	if pageURL == nil {
		p.teamEmailNotice = "Não foi possível iniciar a conexão com Google."
		ctx.Update()
		return
	}
	redirect := oauthRedirectURLFor(pageURL, url.Values{"googleEmail": {"1"}}, nativeOAuthAvailable())
	params := url.Values{"access_type": {"offline"}, "prompt": {"consent"}, "include_granted_scopes": {"true"}}
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
	const scopes = "email profile https://www.googleapis.com/auth/gmail.send"
	var authURL string
	if linked {
		authURL, err = client.SignInWithOAuthURLAndScopes("google", redirect, scopes, params)
	} else {
		authURL, err = client.LinkIdentityOAuthURL(ctx, p.session.AccessToken, "google", redirect, scopes, params)
	}
	if err != nil {
		p.teamEmailNotice = "Não foi possível iniciar a autorização do Gmail."
		ctx.Update()
		return
	}
	p.launchOAuth(ctx, authURL)
}

func (p *serviceCatalogPage) resumeGoogleEmailConnection(ctx app.Context, session supabase.AuthSession) {
	if strings.TrimSpace(session.ProviderToken) == "" {
		p.authNotice = "O Google não retornou autorização do Gmail. Abra Configurações e tente conectar novamente."
		return
	}
	userID := ""
	if p.caller != nil {
		userID = p.caller.UserID
	}
	p.teamEmailBusy = true
	p.teamEmailNotice = "Concluindo a conexão segura com o Google..."
	go func() {
		result, err := sendTeamJSONResult(ctx, apiBaseURL()+"/api/google-email", session.AccessToken, http.MethodPost, map[string]string{
			"action": "connect", "providerToken": session.ProviderToken, "providerRefreshToken": session.ProviderRefreshToken,
		})
		if p.session == nil || p.caller == nil || p.caller.UserID != userID {
			return
		}
		p.teamEmailBusy = false
		if err != nil || result["connected"] != true {
			p.teamEmailGoogleConnected = false
			p.teamEmailNotice = "Não foi possível concluir a conexão. Verifique a permissão de envio do Gmail e tente novamente."
			p.authNotice = p.teamEmailNotice
		} else {
			p.teamEmailGoogleConnected = true
			p.teamEmailGoogleReady = true
			p.teamEmailGoogleAccount = portalText(result["email"])
			p.teamEmailNotice = "Google conectado. Os envios automáticos usarão esta conta."
			p.authNotice = p.teamEmailNotice
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) disconnectTeamGoogleEmail(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.session == nil || p.teamEmailBusy || !p.teamEmailGoogleConnected {
		return
	}
	token := p.session.AccessToken
	userID := ""
	if p.caller != nil {
		userID = p.caller.UserID
	}
	p.teamEmailBusy = true
	p.teamEmailNotice = "Desconectando o Gmail..."
	ctx.Update()
	go func() {
		_, err := sendTeamJSONResult(ctx, apiBaseURL()+"/api/google-email", token, http.MethodPost, map[string]string{"action": "disconnect"})
		if p.session == nil || p.session.AccessToken != token || p.caller == nil || p.caller.UserID != userID {
			return
		}
		p.teamEmailBusy = false
		if err != nil {
			p.teamEmailNotice = "Não foi possível desconectar o Gmail. Tente novamente."
		} else {
			p.teamEmailGoogleConnected = false
			p.teamEmailGoogleAccount = ""
			p.teamEmailNotice = "Gmail desconectado. Os envios automáticos voltarão ao canal SMTP manual, se configurado."
		}
		ctx.Update()
	}()
}
