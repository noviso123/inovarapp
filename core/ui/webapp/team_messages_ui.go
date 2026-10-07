package webapp

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/domain"
)

var teamMessagePreviewValues = map[string]string{
	"cliente": "Maria", "tecnico": "Gabriel", "empresa": "Inovar Refrigeração", "email": "maria@email.com", "senha": "123456",
	"numero": "2026-0001", "os": "#A1B2C3", "servico": "Limpeza de Ar", "data": "10/09/2026", "hora": "09:00",
	"valor": "R$ 250,00", "garantia": "90 dias", "equipamento": "LG 12.000 BTUs (Sala)", "data_ultima": "10/03/2026",
	"validade": "25/09/2026", "itens": "• Higienização de serpentina (1x) = R$ 250,00", "desconto": "🎁 Desconto especial: R$ 15,00\n",
	"pagamento": "PIX ou cartão", "prazo": "2 horas", "observacoes": "📝 Observações: verificar tensão 220 V",
	"situacao": "está em atraso há 5 dias", "meses": "6", "status": "CONCLUÍDO", "app": domain.AppPublicURL,
}

func (p *serviceCatalogPage) openTeamMessageCenter(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.caller == nil || p.caller.Role != domain.RoleAdmin {
		return
	}
	p.teamMessageKey = ""
	p.teamMessageDraft = ""
	p.teamMessageInterval = strconv.Itoa(p.teamProfile.ReminderIntervalDays)
	if p.teamProfile.ReminderIntervalDays <= 0 {
		p.teamMessageInterval = "7"
	}
	p.teamMessageNotice = ""
	ctx.Update()
}

func (p *serviceCatalogPage) editTeamMessage(key string) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		if p.caller == nil || p.caller.Role != domain.RoleAdmin {
			return
		}
		p.teamMessageKey = key
		p.teamMessageDraft = domain.ResolveWhatsAppTemplate(p.teamProfile.WhatsAppMessages, key)
		p.teamMessageNotice = ""
		ctx.Update()
	}
}

func (p *serviceCatalogPage) backToTeamMessages(ctx app.Context, event app.Event) {
	event.PreventDefault()
	p.teamMessageKey, p.teamMessageDraft, p.teamMessageNotice = "", "", ""
	ctx.Update()
}

func (p *serviceCatalogPage) appendTeamMessagePlaceholder(value string) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		if p.teamMessageDraft == "" {
			p.teamMessageDraft = value
		} else {
			p.teamMessageDraft += " " + value
		}
		ctx.Update()
	}
}

func (p *serviceCatalogPage) saveTeamMessage(ctx app.Context, event app.Event) {
	event.PreventDefault()
	model, ok := domain.WhatsAppTemplateByKey(p.teamMessageKey)
	if !ok || p.caller == nil || p.caller.Role != domain.RoleAdmin || p.session == nil || p.teamMessageSaving {
		return
	}
	messages := make(map[string]string, len(p.teamProfile.WhatsAppMessages)+1)
	for key, value := range p.teamProfile.WhatsAppMessages {
		messages[key] = value
	}
	if strings.TrimSpace(p.teamMessageDraft) == "" || p.teamMessageDraft == model.Default {
		delete(messages, model.Key)
	} else {
		messages[model.Key] = p.teamMessageDraft
	}
	p.persistTeamMessageSettings(ctx, map[string]any{"mensagensWhats": messages}, func() {
		p.teamProfile.WhatsAppMessages = messages
		p.teamMessageDraft = domain.ResolveWhatsAppTemplate(messages, model.Key)
		p.teamMessageNotice = "Mensagem salva no banco e sincronizada entre os dispositivos."
	})
}

func (p *serviceCatalogPage) restoreTeamMessage(ctx app.Context, event app.Event) {
	event.PreventDefault()
	model, ok := domain.WhatsAppTemplateByKey(p.teamMessageKey)
	if !ok || p.caller == nil || p.caller.Role != domain.RoleAdmin || p.session == nil || p.teamMessageSaving {
		return
	}
	messages := make(map[string]string, len(p.teamProfile.WhatsAppMessages))
	for key, value := range p.teamProfile.WhatsAppMessages {
		messages[key] = value
	}
	delete(messages, model.Key)
	p.persistTeamMessageSettings(ctx, map[string]any{"mensagensWhats": messages}, func() {
		p.teamProfile.WhatsAppMessages = messages
		p.teamMessageDraft = model.Default
		p.teamMessageNotice = "Mensagem padrão restaurada e sincronizada."
	})
}

func (p *serviceCatalogPage) saveTeamReminderInterval(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.caller == nil || p.caller.Role != domain.RoleAdmin || p.session == nil || p.teamMessageSaving {
		return
	}
	days, err := strconv.Atoi(strings.TrimSpace(p.teamMessageInterval))
	if err != nil || days < 1 || days > 180 {
		p.teamMessageNotice = "Informe um intervalo entre 1 e 180 dias."
		return
	}
	p.persistTeamMessageSettings(ctx, map[string]any{"lembrete_intervalo_dias": days}, func() {
		p.teamProfile.ReminderIntervalDays = days
		p.teamMessageNotice = "Intervalo do lembrete salvo e sincronizado."
	})
}

func (p *serviceCatalogPage) persistTeamMessageSettings(ctx app.Context, payload map[string]any, success func()) {
	token, base := p.session.AccessToken, apiBaseURL()
	p.teamMessageSaving, p.teamMessageNotice = true, "Salvando no banco..."
	go func() {
		result, err := sendTeamJSONResult(ctx, base+"/api/configuracoes", token, http.MethodPost, payload)
		if p.session == nil || p.session.AccessToken != token {
			return
		}
		p.teamMessageSaving = false
		if err != nil || result["ok"] != true {
			if isOfflineNetworkError(err) && p.caller != nil && enqueueOfflineTeamSettings(p.caller.UserID, p.caller.Role, p.teamProfile, payload) == nil {
				success()
				p.teamMessageNotice = "Salvo neste dispositivo. Será sincronizado quando a conexão voltar."
				p.offlineMode = true
				p.teamLoadedFor = ""
				p.retryPendingOfflineMutations(ctx)
			} else {
				p.teamMessageNotice = "Não foi possível salvar no banco. Verifique a conexão e tente novamente."
			}
		} else {
			success()
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) teamMessageCenter() app.UI {
	if p.caller == nil || p.caller.Role != domain.RoleAdmin {
		return app.Div()
	}
	models := domain.WhatsAppTemplates()
	customized := 0
	for _, model := range models {
		if value := strings.TrimSpace(p.teamProfile.WhatsAppMessages[model.Key]); value != "" && value != model.Default {
			customized++
		}
	}
	content := []app.UI{
		app.Div().Class("team-messages__heading").Body(app.H3().Class("team-section__heading-title").Body(app.Text("Central de Mensagens do WhatsApp")), app.Span().Body(app.Text(fmt.Sprintf("%d personalizada(s) de %d", customized, len(models))))),
		app.P().Class("team-calendar__hint").Body(app.Text("Edite os textos enviados automaticamente. As alterações ficam salvas no banco.")),
	}
	if p.teamMessageNotice != "" {
		content = append(content, app.P().Class("portal-section__intro").Body(app.Text(p.teamMessageNotice)))
	}
	if p.teamMessageKey == "" {
		interval := p.teamProfile.ReminderIntervalDays
		if interval <= 0 {
			interval = 7
		}
		content = append(content, app.Div().Class("team-messages__interval").Body(
			app.Span().Body(app.Text("⏰ Lembrete de ciclo vencido: reenviar a cada")),
			app.Input().Type("number").Attr("min", "1").Attr("max", "180").Value(p.teamMessageInterval).Placeholder(fmt.Sprint(interval)).OnChange(p.ValueTo(&p.teamMessageInterval)),
			app.Span().Body(app.Text("dias")),
			app.Button().Class("auth-submit").Type("button").Disabled(p.teamMessageSaving).OnClick(p.saveTeamReminderInterval).Body(app.Text("Salvar")),
		))
		for _, model := range models {
			personalized := strings.TrimSpace(p.teamProfile.WhatsAppMessages[model.Key]) != "" && p.teamProfile.WhatsAppMessages[model.Key] != model.Default
			badge := app.Span()
			if personalized {
				badge = app.Span().Class("team-messages__badge").Body(app.Text("PERSONALIZADA"))
			}
			content = append(content, app.Button().Class("team-messages__row").Type("button").OnClick(p.editTeamMessage(model.Key)).Body(app.Div().Body(app.Strong().Body(app.Text(model.Title), badge), app.P().Body(app.Text(model.When))), app.Span().Body(app.Text("›"))))
		}
	} else if model, ok := domain.WhatsAppTemplateByKey(p.teamMessageKey); ok {
		content = append(content,
			app.Button().Class("team-messages__back").Type("button").OnClick(p.backToTeamMessages).Body(app.Text("← Todas as mensagens")),
			app.H4().Class("team-messages__title").Body(app.Text(model.Title)),
			app.P().Class("team-calendar__hint").Body(app.Text(model.When)),
			app.Span().Class("team-messages__label").Body(app.Text("Variáveis (toque para adicionar):")),
		)
		buttons := make([]app.UI, 0, len(model.Placeholders))
		for _, placeholder := range model.Placeholders {
			buttons = append(buttons, app.Button().Class("team-messages__variable").Type("button").OnClick(p.appendTeamMessagePlaceholder(placeholder)).Body(app.Text(placeholder)))
		}
		content = append(content, app.Div().Class("team-messages__variables").Body(buttons...))
		content = append(content,
			app.Label().Class("team-messages__label").Body(app.Text("Texto da mensagem:"), app.Textarea().Rows(9).Text(p.teamMessageDraft).OnChange(p.ValueTo(&p.teamMessageDraft))),
			app.Span().Class("team-messages__label").Body(app.Text("Pré-visualização (exemplo):")),
			app.Div().Class("team-messages__preview").Body(app.Text(domain.ApplyWhatsAppPlaceholders(p.teamMessageDraft, teamMessagePreviewValues))),
			app.Div().Class("portal-budget__actions").Body(
				app.Button().Class("auth-link").Type("button").Disabled(p.teamMessageSaving).OnClick(p.restoreTeamMessage).Body(app.Text("Restaurar padrão")),
				app.Button().Class("auth-submit").Type("button").Disabled(p.teamMessageSaving).OnClick(p.saveTeamMessage).Body(app.Text("Salvar no banco")),
			),
		)
	}
	return app.Section().Class("team-messages").Body(content...)
}
