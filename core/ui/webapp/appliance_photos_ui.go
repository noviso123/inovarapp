package webapp

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

type appliancePhoto struct {
	Name  string `json:"nome"`
	Path  string `json:"caminho"`
	URL   string `json:"url"`
	Local bool   `json:"-"`
}

const teamPhotoDeleteAction = "delete-appliance-photo:"

type appliancePhotoInput struct {
	Name            string `json:"nome"`
	Base64          string `json:"base64"`
	OriginalDataURL string `json:"originalDataUrl"`
	Type            string `json:"type"`
	Size            int64  `json:"size"`
	Error           bool   `json:"-"`
}

func (p *serviceCatalogPage) toggleAppliancePhotos(id string) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		if p.teamPhotoApplianceID == id {
			p.teamPhotoApplianceID, p.teamAppliancePhotos = "", nil
			return
		}
		p.teamPhotoApplianceID, p.teamAppliancePhotos, p.teamPhotoNotice = id, nil, "Carregando fotos..."
		p.loadAppliancePhotos(ctx, id)
	}
}

func (p *serviceCatalogPage) loadAppliancePhotos(ctx app.Context, id string) {
	p.loadAppliancePhotosWithNotice(ctx, id, "")
}

func (p *serviceCatalogPage) loadAppliancePhotosWithNotice(ctx app.Context, id, successNotice string) {
	if p.session == nil {
		return
	}
	pageURL := app.Window().URL()
	if pageURL == nil || pageURL.Scheme == "" || pageURL.Host == "" {
		return
	}
	base, token := apiBaseURL(), p.session.AccessToken
	go func() {
		result, err := sendTeamJSONResult(ctx, base+"/api/documentos", token, http.MethodPost, map[string]any{"acao": "fotos-aparelho", "aparelhoId": id})
		if err != nil {
			p.teamPhotoNotice = "Não foi possível carregar as fotos."
		} else if raw, ok := result["fotos"].([]any); ok {
			encoded, _ := json.Marshal(raw)
			_ = json.Unmarshal(encoded, &p.teamAppliancePhotos)
			p.teamPhotoNotice = successNotice
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) chooseAppliancePhotos(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.teamPhotoBusy || p.teamPhotoApplianceID == "" {
		return
	}
	p.teamPhotoBusy, p.teamPhotoNotice = true, "Preparando imagens..."
	selected := startAppliancePhotoPickerLimit(6)
	go func() {
		files := <-selected
		files = appliancePhotoBatch(files)
		if len(files) == 0 {
			p.teamPhotoBusy, p.teamPhotoNotice = false, ""
			ctx.Update()
			return
		}
		pageURL := app.Window().URL()
		if pageURL == nil || pageURL.Scheme == "" || pageURL.Host == "" || p.session == nil {
			p.teamPhotoBusy, p.teamPhotoNotice = false, "Sessão indisponível."
			ctx.Update()
			return
		}
		base, token, id := apiBaseURL(), p.session.AccessToken, p.teamPhotoApplianceID
		uploaded, failed := 0, 0
		for _, file := range files {
			if file.Error {
				failed++
				continue
			}
			_, err := sendTeamJSONResult(ctx, base+"/api/documentos", token, http.MethodPost, map[string]any{"acao": "upload", "nome": file.Name, "base64": file.Base64, "pasta": "aparelho", "refId": id})
			if err == nil {
				uploaded++
			} else {
				failed++
			}
		}
		p.teamPhotoBusy = false
		p.teamPhotoNotice = appliancePhotoUploadNotice(uploaded, failed)
		ctx.Update()
		p.loadAppliancePhotosWithNotice(ctx, id, p.teamPhotoNotice)
	}()
}

func appliancePhotoUploadNotice(uploaded, failed int) string {
	if failed == 0 {
		return ""
	}
	if uploaded == 0 {
		return "Não foi possível enviar as fotos. Verifique a conexão e tente de novo."
	}
	return itoaPhoto(uploaded) + " salva(s), " + itoaPhoto(failed) + " falharam. Tente novamente."
}

func appliancePhotoBatch(files []appliancePhotoInput) []appliancePhotoInput {
	if len(files) > 6 {
		return files[:6]
	}
	return files
}

func itoaPhoto(v int) string {
	if v == 0 {
		return "0"
	}
	return strconv.Itoa(v)
}

func (p *serviceCatalogPage) appliancePhotosPanel() app.UI {
	id := p.teamPhotoApplianceID
	if id == "" {
		return app.Div()
	}
	photos := make([]app.UI, 0, len(p.teamAppliancePhotos))
	for _, photo := range p.teamAppliancePhotos {
		photos = append(photos, app.Div().Class("service-card__item").Body(
			app.Img().Src(photo.URL).Alt(photo.Name).Style("max-width", "180px").Style("max-height", "140px"),
			app.P().Body(app.Text(photo.Name)),
			app.Button().Class("auth-link").Type("button").OnClick(p.askDeleteAppliancePhoto(photo.Path)).Body(app.Text("Excluir foto")),
		))
	}
	notice := p.teamPhotoNotice
	if len(photos) == 0 && notice == "" {
		notice = "Nenhuma foto anexada. Adicione fotos do equipamento (instalação, serial, local) — ficam salvas na ficha."
	}
	return app.Div().Class("portal-service team-appliance-photos-panel").Body(
		app.Div().Class("team-appliance-photos__header").Body(
			app.Strong().Body(app.Text("Fotos do aparelho")),
			app.P().Class("portal-section__intro").Body(app.Text(notice)),
		),
		app.Div().Class("portal-budget__actions team-appliance-photos__actions").Body(app.Button().Class("auth-submit").Type("button").Disabled(p.teamPhotoBusy).OnClick(p.chooseAppliancePhotos).Body(app.Text("Selecionar fotos"))),
		app.Div().Class("team-appliance-photo-grid").Body(photos...),
	)
}

func (p *serviceCatalogPage) askDeleteAppliancePhoto(path string) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		p.teamCustomerConfirm = teamPhotoDeleteAction + path
	}
}
