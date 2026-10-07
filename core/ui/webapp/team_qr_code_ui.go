package webapp

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/url"
	"strings"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	qrcode "github.com/skip2/go-qrcode"
	"inovarapp/core/domain"
)

const (
	teamQRCodeContact  = "contato"
	teamQRCodeWhatsApp = "whatsapp"
	teamQRCodeSite     = "site"
	teamQRCodeCustom   = "personalizado"
	teamQRCodeSiteURL  = "https://inovarapp.vercel.app/"
)

func (p *serviceCatalogPage) canUseTeamQRCode() bool {
	return p != nil && p.caller != nil && (p.caller.Role == domain.RoleAdmin || p.caller.Role == domain.RoleTechnician)
}

func (p *serviceCatalogPage) teamQRCodePanel() app.UI {
	if !p.canUseTeamQRCode() {
		return app.Div()
	}
	if p.teamQRCodeMode == "" {
		p.teamQRCodeMode = teamQRCodeWhatsApp
		p.teamQRCodePhone = p.teamProfile.Phone
		p.teamQRCodeMessage = "Olá! Gostaria de agendar um atendimento com a Inovar Refrigeração."
	}
	if p.teamQRCodeCustom == "" {
		p.teamQRCodeCustom = teamQRCodeSiteURL
	}
	if p.teamQRCodePNG == "" && p.teamQRCodeError == "" {
		p.generateTeamQRCode()
	}

	modeOptions := []app.UI{
		app.Option().Value(teamQRCodeWhatsApp).Selected(p.teamQRCodeMode == teamQRCodeWhatsApp).Body(app.Text("Abrir WhatsApp comercial")),
		app.Option().Value(teamQRCodeSite).Selected(p.teamQRCodeMode == teamQRCodeSite).Body(app.Text("Abrir o site do InovarApp")),
		app.Option().Value(teamQRCodeCustom).Selected(p.teamQRCodeMode == teamQRCodeCustom).Body(app.Text("Link ou texto personalizado")),
	}

	configuration := []app.UI{
		app.Label().Class("auth-field").Body(
			app.Text("O que o QR Code vai abrir"),
			app.Select().Attr("value", p.teamQRCodeMode).OnChange(p.changeTeamQRCodeMode).Body(modeOptions...),
		),
	}
	if p.teamQRCodeMode == teamQRCodeWhatsApp {
		configuration = append(configuration,
			app.Label().Class("auth-field").Body(
				app.Text("Número do WhatsApp da empresa"),
				app.Input().Type("tel").Value(p.teamQRCodePhone).Placeholder("Ex.: (27) 99999-9999").OnChange(p.changeTeamQRCodePhone),
			),
			app.Label().Class("auth-field").Body(
				app.Text("Mensagem inicial (opcional)"),
				app.Textarea().Rows(4).Text(p.teamQRCodeMessage).Placeholder("Olá! Gostaria de agendar uma manutenção.").OnChange(p.changeTeamQRCodeMessage),
			),
			app.P().Class("team-qr__hint").Body(app.Text("Informe o número com DDD. Para outro país, inclua o código do país. Ao ler o QR, a conversa abre com a mensagem preenchida; o cliente confirma o envio no WhatsApp.")),
		)
	} else if p.teamQRCodeMode == teamQRCodeCustom {
		configuration = append(configuration,
			app.Label().Class("auth-field").Body(
				app.Text("Endereço ou texto"),
				app.Input().Type("text").Value(p.teamQRCodeCustom).Placeholder("https://seusite.com/ ou uma mensagem").OnChange(p.changeTeamQRCodeCustom),
			),
			app.P().Class("team-qr__hint").Body(app.Text("Para cartões, prefira um link curto e público. Textos longos deixam o QR mais denso e podem ser mais difíceis de ler.")),
		)
	}
	configuration = append(configuration,
		app.Button().Class("auth-submit team-qr__generate").Type("button").OnClick(p.generateTeamQRCodeClick).Body(app.Text("Gerar / atualizar QR Code")),
	)

	previewContent := []app.UI{
		app.P().Class("team-qr__preview-label").Body(app.Text(teamQRCodeModeLabel(p.teamQRCodeMode))),
	}
	if p.teamQRCodeError != "" {
		previewContent = append(previewContent, app.Div().Class("portal-section__error").Role("alert").Body(app.Text(p.teamQRCodeError)))
	} else if p.teamQRCodePNG != "" {
		previewContent = append(previewContent,
			app.Img().Class("team-qr__image").Src(p.teamQRCodePNG).Alt("QR Code de alta resolução da Inovar Refrigeração"),
			app.P().Class("team-qr__scan-note").Body(app.Text("Alto contraste, margem de leitura preservada e sem elementos sobre o código.")),
			app.Div().Class("team-qr__downloads").Body(
				app.A().Class("auth-submit team-qr__download").Href(teamQRCodeDataURL("image/svg+xml", p.teamQRCodeSVG)).Attr("download", teamQRCodeFilename(p.teamQRCodeMode, "svg")).Body(app.Text("Baixar SVG para impressão")),
				app.A().Class("auth-link team-qr__download").Href(p.teamQRCodePNG).Attr("download", teamQRCodeFilename(p.teamQRCodeMode, "png")).Body(app.Text("Baixar PNG em alta resolução")),
			),
			app.P().Class("team-qr__hint").Body(app.Text("SVG é vetorial e mantém nitidez em qualquer tamanho. O PNG é gerado com cerca de 2.200 px para uso digital e impressão.")),
		)
	}

	return app.Section().ID("team-qr-code").Class("team-qr").Body(
		app.Div().Class("team-view-heading").Body(
			app.Div().Body(
				app.P().Class("catalog__eyebrow").Body(app.Text("MATERIAL DE DIVULGAÇÃO")),
				app.H2().Class("catalog__title").Body(app.Text("QR Code da empresa")),
				app.P().Class("portal-section__intro").Body(app.Text("Gere um código nítido para cartões, adesivos e materiais da empresa. Escolha se a leitura abre o WhatsApp, o site ou um endereço personalizado.")),
			),
		),
		app.Div().Class("team-qr__layout").Body(
			app.Div().Class("team-qr__configuration").Body(
				app.H3().Body(app.Text("Configurar destino")),
				app.Div().Class("team-qr__fields").Body(configuration...),
			),
			app.Div().Class("team-qr__preview").Body(previewContent...),
		),
	)
}

func (p *serviceCatalogPage) changeTeamQRCodeMode(ctx app.Context, event app.Event) {
	if !p.canUseTeamQRCode() {
		return
	}
	p.teamQRCodeMode = event.Get("target").Get("value").String()
	p.generateTeamQRCode()
	ctx.Update()
}

func (p *serviceCatalogPage) changeTeamQRCodeCustom(ctx app.Context, event app.Event) {
	if !p.canUseTeamQRCode() {
		return
	}
	p.teamQRCodeCustom = event.Get("target").Get("value").String()
	p.generateTeamQRCode()
	ctx.Update()
}

func (p *serviceCatalogPage) generateTeamQRCodeClick(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if !p.canUseTeamQRCode() {
		return
	}
	p.generateTeamQRCode()
	ctx.Update()
}

func (p *serviceCatalogPage) generateTeamQRCode() {
	content, err := teamQRCodePayload(p.teamQRCodeMode, p.teamQRCodeCustom, p.teamProfile)
	if p.teamQRCodeMode == teamQRCodeWhatsApp {
		content, err = teamQRCodeWhatsAppPayload(p.teamQRCodePhone, p.teamQRCodeMessage)
	}
	if err != nil {
		p.teamQRCodePNG, p.teamQRCodeSVG, p.teamQRCodePayload = "", "", ""
		p.teamQRCodeError = err.Error()
		return
	}
	code, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		p.teamQRCodePNG, p.teamQRCodeSVG, p.teamQRCodePayload = "", "", ""
		p.teamQRCodeError = "Não foi possível gerar este QR Code. Use um link ou texto mais curto."
		return
	}
	bitmap := code.Bitmap()
	if len(bitmap) == 0 {
		p.teamQRCodePNG, p.teamQRCodeSVG, p.teamQRCodePayload = "", "", ""
		p.teamQRCodeError = "Não foi possível gerar o QR Code."
		return
	}

	svg := teamQRCodeSVG(bitmap)
	pngBytes, err := teamQRCodePNG(bitmap)
	if err != nil {
		p.teamQRCodePNG, p.teamQRCodeSVG, p.teamQRCodePayload = "", "", ""
		p.teamQRCodeError = "Não foi possível preparar o arquivo PNG. Tente novamente."
		return
	}
	p.teamQRCodePayload = content
	p.teamQRCodeSVG = svg
	p.teamQRCodePNG = "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)
	p.teamQRCodeError = ""
}

func (p *serviceCatalogPage) changeTeamQRCodePhone(ctx app.Context, event app.Event) {
	if !p.canUseTeamQRCode() {
		return
	}
	p.teamQRCodePhone = event.Get("target").Get("value").String()
	p.generateTeamQRCode()
	ctx.Update()
}

func (p *serviceCatalogPage) changeTeamQRCodeMessage(ctx app.Context, event app.Event) {
	if !p.canUseTeamQRCode() {
		return
	}
	p.teamQRCodeMessage = event.Get("target").Get("value").String()
	p.generateTeamQRCode()
	ctx.Update()
}

func teamQRCodeWhatsAppPayload(number, message string) (string, error) {
	phone := normalizeBusinessPhone(number)
	if len(phone) < 10 || len(phone) > 15 {
		return "", fmt.Errorf("Informe um número válido do WhatsApp com DDD e, para outro país, o código do país.")
	}
	link := "https://wa.me/" + phone
	if text := strings.TrimSpace(message); text != "" {
		link += "?" + url.Values{"text": {text}}.Encode()
	}
	return link, nil
}

func teamQRCodePayload(mode, custom string, profile domain.TechnicianProfile) (string, error) {
	business := strings.TrimSpace(profile.BusinessName)
	if business == "" {
		business = "Inovar Refrigeração"
	}
	switch mode {
	case "", teamQRCodeContact:
		phone := normalizeBusinessPhone(profile.Phone)
		if phone == "" {
			return "", fmt.Errorf("Cadastre o WhatsApp comercial em Configurações do app antes de gerar o cartão de contato.")
		}
		lines := []string{"BEGIN:VCARD", "VERSION:3.0", "FN:" + escapeVCardText(business), "ORG:" + escapeVCardText(business)}
		lines = append(lines, "TEL;TYPE=WORK,VOICE:+"+phone)
		if address := strings.TrimSpace(profile.Address); address != "" {
			lines = append(lines, "ADR;TYPE=WORK:;;"+escapeVCardText(address)+";;;;")
		}
		lines = append(lines, "URL:"+teamQRCodeSiteURL, "END:VCARD")
		return strings.Join(lines, "\r\n"), nil
	case teamQRCodeWhatsApp:
		phone := normalizeBusinessPhone(profile.Phone)
		if phone == "" {
			return "", fmt.Errorf("Cadastre o WhatsApp comercial em Configurações do app antes de gerar este QR Code.")
		}
		return "https://wa.me/" + phone, nil
	case teamQRCodeSite:
		return teamQRCodeSiteURL, nil
	case teamQRCodeCustom:
		value := strings.TrimSpace(custom)
		if value == "" {
			return "", fmt.Errorf("Informe um link ou texto para gerar o QR Code.")
		}
		return value, nil
	default:
		return "", fmt.Errorf("Escolha um destino válido para o QR Code.")
	}
}

func teamQRCodeSVG(bitmap [][]bool) string {
	var path strings.Builder
	for y, row := range bitmap {
		for x, dark := range row {
			if dark {
				fmt.Fprintf(&path, "M%d %dh1v1h-1z", x, y)
			}
		}
	}
	size := len(bitmap)
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" shape-rendering="crispEdges"><rect width="100%%" height="100%%" fill="#ffffff"/><path fill="#071326" d="%s"/></svg>`, size, size, size, size, path.String())
}

func teamQRCodePNG(bitmap [][]bool) ([]byte, error) {
	modules := len(bitmap)
	if modules == 0 {
		return nil, fmt.Errorf("empty QR bitmap")
	}
	scale := 2200 / modules
	if scale < 12 {
		scale = 12
	}
	if scale > 128 {
		scale = 128
	}
	size := modules * scale
	img := image.NewPaletted(image.Rect(0, 0, size, size), color.Palette{color.White, color.RGBA{R: 7, G: 19, B: 38, A: 255}})
	for y, row := range bitmap {
		for x, dark := range row {
			if !dark {
				continue
			}
			for py := y * scale; py < (y+1)*scale; py++ {
				start := img.PixOffset(x*scale, py)
				for px := 0; px < scale; px++ {
					img.Pix[start+px] = 1
				}
			}
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		return nil, err
	}
	return encoded.Bytes(), nil
}

func teamQRCodeDataURL(mimeType, value string) string {
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString([]byte(value))
}

func teamQRCodeFilename(mode, extension string) string {
	name := "contato"
	switch mode {
	case teamQRCodeWhatsApp:
		name = "whatsapp"
	case teamQRCodeSite:
		name = "site"
	case teamQRCodeCustom:
		name = "personalizado"
	}
	return "inovarapp-qr-" + name + "." + extension
}

func teamQRCodeModeLabel(mode string) string {
	switch mode {
	case teamQRCodeWhatsApp:
		return "Destino: conversa no WhatsApp comercial"
	case teamQRCodeSite:
		return "Destino: site do InovarApp"
	case teamQRCodeCustom:
		return "Destino: conteúdo personalizado"
	default:
		return "Destino: contato da empresa (vCard)"
	}
}

func escapeVCardText(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, ";", "\\;")
	value = strings.ReplaceAll(value, ",", "\\,")
	return value
}

func normalizeBusinessPhone(value string) string {
	var digits strings.Builder
	for _, char := range value {
		if char >= '0' && char <= '9' {
			digits.WriteRune(char)
		}
	}
	phone := strings.TrimLeft(digits.String(), "0")
	if len(phone) == 10 || len(phone) == 11 {
		phone = "55" + phone
	}
	return phone
}
