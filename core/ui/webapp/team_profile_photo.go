package webapp

import "github.com/maxence-charriere/go-app/v11/pkg/app"

func (p *serviceCatalogPage) loadTeamProfilePhoto(ctx app.Context) {
	if p.caller == nil || p.session == nil || (p.caller.Role != "ADMIN" && p.caller.Role != "TECNICO") || p.teamProfilePhotoBusy || p.teamProfilePhotoLoaded == p.caller.UserID {
		return
	}
	userID, token := p.caller.UserID, p.session.AccessToken
	p.teamProfilePhotoBusy = true
	go func() {
		result, err := requestCustomerProfilePhoto(ctx, token, "fotoperfil-url", "")
		if p.caller == nil || p.session == nil || p.caller.UserID != userID || p.session.AccessToken != token {
			return
		}
		p.teamProfilePhotoBusy = false
		if err == nil {
			p.teamProfilePhotoURL = cacheBustCustomerProfilePhoto(portalText(result["url"]))
			p.teamProfilePhotoLoaded = userID
		} else {
			p.teamProfilePhotoError = "Não foi possível carregar a foto de perfil."
			p.teamProfilePhotoLoaded = userID
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) changeTeamProfilePhoto(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.teamProfilePhotoBusy || p.session == nil || p.caller == nil {
		return
	}
	selected := startCustomerProfilePhotoPicker()
	p.teamProfilePhotoBusy = true
	p.teamProfilePhotoError = ""
	ctx.Update()
	userID, token := p.caller.UserID, p.session.AccessToken
	go func() {
		photo := <-selected
		if p.caller == nil || p.session == nil || p.caller.UserID != userID || p.session.AccessToken != token {
			return
		}
		if photo == nil {
			p.teamProfilePhotoBusy = false
			ctx.Update()
			return
		}
		if photo.Error != "" {
			p.teamProfilePhotoBusy = false
			p.teamProfilePhotoError = photo.Error
			ctx.Update()
			return
		}
		previous := p.teamProfilePhotoURL
		p.teamProfilePhotoURL = photo.DataURL
		ctx.Update()
		result, err := requestCustomerProfilePhoto(ctx, token, "fotoperfil", photo.DataURL)
		if err == nil {
			photoResult, photoErr := requestCustomerProfilePhoto(ctx, token, "fotoperfil-url", "")
			if photoErr == nil {
				p.teamProfilePhotoURL = cacheBustCustomerProfilePhoto(portalText(photoResult["url"]))
			}
			p.teamProfilePhotoLoaded = userID
		} else {
			p.teamProfilePhotoURL = previous
			p.teamProfilePhotoError = "Não foi possível enviar a foto de perfil."
			if result == nil {
				p.teamProfilePhotoError = "Sem conexão para enviar a foto."
			} else if message := portalText(result["error"]); message != "" {
				p.teamProfilePhotoError = message
			}
		}
		p.teamProfilePhotoBusy = false
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) removeTeamProfilePhoto(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if !p.teamProfilePhotoBusy {
		p.teamProfilePhotoConfirm = true
		p.teamProfilePhotoError = ""
	}
	ctx.Update()
}

func (p *serviceCatalogPage) confirmRemoveTeamProfilePhoto(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.teamProfilePhotoBusy || p.session == nil || p.caller == nil {
		return
	}
	p.teamProfilePhotoBusy = true
	token, userID := p.session.AccessToken, p.caller.UserID
	ctx.Update()
	go func() {
		_, err := requestCustomerProfilePhoto(ctx, token, "fotoperfil-remover", "")
		p.teamProfilePhotoBusy = false
		p.teamProfilePhotoConfirm = false
		if err == nil && p.caller != nil && p.caller.UserID == userID {
			p.teamProfilePhotoURL = ""
			p.teamProfilePhotoLoaded = userID
		} else if err != nil {
			p.teamProfilePhotoError = err.Error()
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) teamProfilePhotoRemovalDialog() app.UI {
	return app.Div().Class("auth-backdrop").Body(
		app.Section().Class("auth-dialog").Attr("role", "dialog").Attr("aria-modal", "true").Body(
			app.H2().Class("auth-dialog__title").Body(app.Text("Remover foto de perfil?")),
			app.P().Class("auth-dialog__intro").Body(app.Text("Sua foto será removida do perfil.")),
			app.Div().Class("auth-dialog__actions").Body(
				app.Button().Class("auth-link").Type("button").Disabled(p.teamProfilePhotoBusy).OnClick(func(ctx app.Context, event app.Event) {
					event.PreventDefault()
					p.teamProfilePhotoConfirm = false
					ctx.Update()
				}).Body(app.Text("Cancelar")),
				app.Button().Class("auth-submit").Type("button").Disabled(p.teamProfilePhotoBusy).OnClick(p.confirmRemoveTeamProfilePhoto).Body(app.Text("Remover foto")),
			),
		),
	)
}
