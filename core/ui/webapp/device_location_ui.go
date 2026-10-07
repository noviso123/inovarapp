package webapp

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

func (p *serviceCatalogPage) deviceLocationPanel() app.UI {
	content := []app.UI{
		app.Strong().Body(app.Text("Localização neste dispositivo")),
		app.P().Body(app.Text("O dispositivo só será consultado quando você tocar no botão. A localização não é acompanhada em segundo plano, salva ou enviada ao servidor.")),
		app.Button().Class("auth-link device-permission__action").Type("button").Disabled(p.locationBusy).OnClick(p.requestDeviceLocation).Body(app.Text(func() string {
			if p.locationBusy {
				return "Consultando localização…"
			}
			if p.locationCoordinates != "" {
				return "Atualizar localização"
			}
			return "Usar minha localização"
		}())),
	}
	if p.locationCoordinates != "" {
		content = append(content,
			app.P().Class("device-permission__result").Body(app.Text("Coordenadas: "+p.locationCoordinates+" • precisão aproximada "+p.locationAccuracy)),
			app.A().Class("device-permission__map-link").Href("https://www.google.com/maps/search/?api=1&query="+url.QueryEscape(p.locationCoordinates)).Target("_blank").Rel("noopener noreferrer").Body(app.Text("Abrir no mapa")),
		)
	}
	if p.locationNotice != "" {
		content = append(content, app.P().Class("device-permission__notice").Role("status").Body(app.Text(p.locationNotice)))
	}
	return app.Section().Class("portal-section device-permission").Body(content...)
}

func (p *serviceCatalogPage) requestDeviceLocation(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.locationBusy {
		return
	}
	userID := ""
	if p.caller != nil {
		userID = p.caller.UserID
	}
	p.locationBusy = true
	p.locationNotice = "O sistema solicitará acesso à localização aproximada."
	locationResult := requestDeviceLocation()
	ctx.Update()
	go func() {
		result := <-locationResult
		if p.caller == nil || p.caller.UserID != userID {
			return
		}
		p.locationBusy = false
		if result.err != nil {
			p.locationNotice = result.err.Error()
		} else {
			p.locationCoordinates = fmt.Sprintf("%s, %s", strconv.FormatFloat(result.location.latitude, 'f', 5, 64), strconv.FormatFloat(result.location.longitude, 'f', 5, 64))
			p.locationAccuracy = fmt.Sprintf("%d m", int(result.location.accuracy+0.5))
			p.locationNotice = "Localização consultada. Ela permanece nesta tela e não é enviada ao servidor."
		}
		ctx.Update()
	}()
}
