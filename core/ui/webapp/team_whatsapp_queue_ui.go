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

type whatsAppQueueItem struct {
	ID            string                 `json:"id"`
	EventType     string                 `json:"event_type"`
	Recipient     string                 `json:"recipient"`
	Preview       string                 `json:"preview"`
	Sensitive     bool                   `json:"sensitive"`
	ScheduledAt   string                 `json:"scheduled_at"`
	NextAttemptAt string                 `json:"next_attempt_at"`
	ExpiresAt     string                 `json:"expires_at"`
	Status        string                 `json:"status"`
	AttemptCount  int                    `json:"attempt_count"`
	LastError     string                 `json:"last_error"`
	CreatedAt     string                 `json:"created_at"`
	SentAt        string                 `json:"sent_at"`
	SourceEntity  string                 `json:"source_entity"`
	Context       map[string]any         `json:"context"`
	History       []whatsAppQueueAttempt `json:"history"`
}

type whatsAppQueueAttempt struct {
	Number     int    `json:"attempt_number"`
	Status     string `json:"status"`
	Error      string `json:"error_message"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`
}

type whatsAppQueueResponse struct {
	Items  []whatsAppQueueItem `json:"items"`
	Counts map[string]int      `json:"counts"`
}

func (p *serviceCatalogPage) loadWhatsAppQueue(ctx app.Context) {
	if p.session == nil || p.caller == nil || p.teamWhatsAppQueueLoading || p.caller.Role != "ADMIN" && p.caller.Role != "TECNICO" {
		return
	}
	token := p.session.AccessToken
	base := apiBaseURL()
	p.teamWhatsAppQueueLoading, p.teamWhatsAppQueueError = true, ""
	go func() {
		request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, base+"/api/whatsapp/fila", nil)
		if err == nil {
			request.Header.Set("Authorization", "Bearer "+token)
			var response *http.Response
			response, err = http.DefaultClient.Do(request)
			if err == nil {
				defer response.Body.Close()
				if response.StatusCode != http.StatusOK {
					var problem struct {
						Error string `json:"error"`
					}
					_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&problem)
					err = fmt.Errorf("%s", strings.TrimSpace(problem.Error))
					if err.Error() == "" {
						err = fmt.Errorf("servidor respondeu HTTP %d", response.StatusCode)
					}
				} else {
					var payload whatsAppQueueResponse
					err = json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&payload)
					if err == nil {
						p.teamWhatsAppQueue, p.teamWhatsAppQueueCounts = payload.Items, payload.Counts
						p.teamWhatsAppQueueLoaded = true
						p.teamWhatsAppQueueUpdatedAt = time.Now().Local().Format("02/01/2006 às 15:04")
					}
				}
			}
		}
		if p.session == nil || p.session.AccessToken != token {
			return
		}
		p.teamWhatsAppQueueLoading = false
		if err != nil {
			p.teamWhatsAppQueueError = "Não foi possível consultar a fila de mensagens. " + err.Error()
		}
		ctx.Update()
		p.scheduleWhatsAppQueueRefresh(ctx)
	}()
}

func (p *serviceCatalogPage) scheduleWhatsAppQueueRefresh(ctx app.Context) {
	if p.teamWhatsAppQueueRefreshScheduled {
		return
	}
	p.teamWhatsAppQueueRefreshScheduled = true
	ctx.After(time.Minute, func(next app.Context) {
		p.teamWhatsAppQueueRefreshScheduled = false
		if p.teamActiveSection == "whatsapp" && p.session != nil && p.caller != nil {
			p.loadWhatsAppQueue(next)
		}
	})
}

func (p *serviceCatalogPage) refreshWhatsAppQueue(ctx app.Context, event app.Event) {
	event.PreventDefault()
	p.loadWhatsAppQueue(ctx)
}

func (p *serviceCatalogPage) updateWhatsAppQueueItem(id, action string) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		if p.session == nil || id == "" || p.teamWhatsAppQueueLoading {
			return
		}
		token, base := p.session.AccessToken, apiBaseURL()
		p.teamWhatsAppQueueLoading = true
		p.teamWhatsAppQueueError = ""
		ctx.Update()
		go func() {
			body, _ := json.Marshal(map[string]string{"acao": action, "id": id})
			request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, base+"/api/whatsapp/fila", bytes.NewReader(body))
			if err == nil {
				request.Header.Set("Authorization", "Bearer "+token)
				request.Header.Set("Content-Type", "application/json")
				var response *http.Response
				response, err = http.DefaultClient.Do(request)
				if err == nil {
					defer response.Body.Close()
					if response.StatusCode < 200 || response.StatusCode >= 300 {
						var problem struct {
							Error string `json:"error"`
						}
						_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&problem)
						err = fmt.Errorf("%s", problem.Error)
					}
				}
			}
			if p.session == nil || p.session.AccessToken != token {
				return
			}
			p.teamWhatsAppQueueLoading = false
			if err != nil {
				p.teamWhatsAppQueueError = "Não foi possível atualizar o item da fila. " + err.Error()
			} else {
				p.teamWhatsAppQueueError = "Estado atualizado."
			}
			ctx.Update()
			p.loadWhatsAppQueue(ctx)
		}()
	}
}

func (p *serviceCatalogPage) teamWhatsAppQueuePanel() app.UI {
	counts := p.teamWhatsAppQueueCounts
	if counts == nil {
		counts = map[string]int{}
	}
	content := []app.UI{
		app.Div().Class("team-whatsapp-queue__heading").Body(
			app.Div().Body(app.P().Class("catalog__eyebrow").Body(app.Text("AUTOMAÇÕES E ACOMPANHAMENTO")), app.H2().Class("catalog__title").Body(app.Text("Agendamentos e fila do WhatsApp")), app.P().Class("portal-section__intro").Body(app.Text("Veja mensagens futuras, pendentes, processando e o histórico recente de envios. Com as migrações e o worker ativos, a fila fica salva no Supabase enquanto a conta está desconectada e retoma os envios quando ela voltar. Lembretes ligados a horários, validade e credenciais expiram para evitar envios fora de contexto."))),
			app.Button().Class("auth-link team-whatsapp-queue__refresh").Type("button").Disabled(p.teamWhatsAppQueueLoading).OnClick(p.refreshWhatsAppQueue).Body(app.Text(map[bool]string{true: "Atualizando…", false: "Atualizar fila"}[p.teamWhatsAppQueueLoading])),
		),
	}
	if p.teamWhatsAppQueueLoaded {
		content = append(content, app.Div().Class("team-whatsapp-queue__metrics").Body(
			queueMetric("Pendentes", counts["pendente"]),
			queueMetric("Processando", counts["processando"]),
			queueMetric("Enviadas", counts["enviado"]),
			queueMetric("Falhas", counts["falha"]),
			queueMetric("Expiradas / canceladas", counts["expirado"]+counts["cancelado"]),
		))
	}
	content = append(content,
		app.P().Class("team-whatsapp-queue__delivery-note").Body(app.Text("“Enviada” significa que o serviço WhatsApp aceitou o envio. Esta integração não recebe confirmação de leitura/entrega no aparelho do cliente.")),
		app.P().Class("team-whatsapp-queue__delivery-note").Body(app.Text("O processador tenta uma mensagem por minuto. Falhas transitórias voltam para a fila com novas tentativas; o prazo de expiração de cada mensagem aparece no cartão.")),
		app.P().Class("team-whatsapp-queue__delivery-note").Body(app.Text("Retenção: depois de aplicada a migração do Supabase, mensagens enviadas ficam na fila por 3 dias e são removidas automaticamente junto com as tentativas. Pendentes e falhas não são apagadas por essa limpeza.")),
	)
	if p.teamWhatsAppQueueError != "" {
		content = append(content, app.Div().Class("team-whatsapp-queue__notice").Role("alert").Body(
			app.Strong().Body(app.Text("Não foi possível confirmar o estado atual da fila.")),
			app.P().Body(app.Text(p.teamWhatsAppQueueError)),
		))
		if p.teamWhatsAppQueueLoaded {
			content = append(content, app.P().Class("team-whatsapp-queue__stale").Body(app.Text("Os dados abaixo são da última consulta bem-sucedida, em "+p.teamWhatsAppQueueUpdatedAt+". Atualize a fila para consultar o estado atual.")))
		}
		if p.teamWhatsAppQueueLoaded && len(p.teamWhatsAppQueue) > 0 {
			cards := make([]app.UI, 0, len(p.teamWhatsAppQueue))
			for _, item := range p.teamWhatsAppQueue {
				cards = append(cards, p.teamWhatsAppQueueCard(item))
			}
			content = append(content, app.Div().Class("team-whatsapp-queue__list").Body(cards...))
		}
	} else if p.teamWhatsAppQueueLoading && !p.teamWhatsAppQueueLoaded {
		content = append(content, app.P().Class("portal-section__empty").Body(app.Text("Carregando mensagens agendadas…")))
	} else if !p.teamWhatsAppQueueLoaded {
		content = append(content, app.Div().Class("team-whatsapp-queue__empty").Body(app.Strong().Body(app.Text("A fila ainda não foi consultada")), app.P().Body(app.Text("Use “Atualizar fila” para carregar os agendamentos e o histórico."))))
	} else if len(p.teamWhatsAppQueue) == 0 {
		content = append(content, app.Div().Class("team-whatsapp-queue__empty").Body(app.Strong().Body(app.Text("Consulta concluída: fila vazia")), app.P().Body(app.Text("Não há mensagens registradas neste momento. Novos lembretes aparecerão quando forem gerados pelos agendamentos e eventos do sistema."))))
	} else {
		cards := make([]app.UI, 0, len(p.teamWhatsAppQueue))
		for _, item := range p.teamWhatsAppQueue {
			cards = append(cards, p.teamWhatsAppQueueCard(item))
		}
		content = append(content, app.Div().Class("team-whatsapp-queue__list").Body(cards...))
	}
	return app.Section().ID("team-whatsapp-queue").Class("portal-section team-whatsapp-queue").Body(content...)
}

func queueMetric(label string, value int) app.UI {
	return app.Article().Class("team-whatsapp-queue__metric").Body(app.Span().Body(app.Text(label)), app.Strong().Body(app.Text(value)))
}

func (p *serviceCatalogPage) teamWhatsAppQueueCard(item whatsAppQueueItem) app.UI {
	statusClass := "team-whatsapp-queue__status team-whatsapp-queue__status--" + item.Status
	status := queueStatusLabel(item.Status)
	main := []app.UI{
		app.Div().Class("team-whatsapp-queue__card-head").Body(
			app.Div().Body(app.Strong().Body(app.Text(queueEventLabel(item.EventType))), app.Span().Class("team-whatsapp-queue__recipient").Body(app.Text(item.Recipient))),
			app.Span().Class(statusClass).Body(app.Text(status)),
		),
		app.P().Class("team-whatsapp-queue__preview").Body(app.Text(func() string {
			if item.Sensitive {
				return "Conteúdo sensível oculto; será apagado do banco após o envio ou expiração."
			}
			return item.Preview
		}())),
		app.Div().Class("team-whatsapp-queue__details").Body(
			app.Span().Body(app.Strong().Body(app.Text("Programada: ")), app.Text(queueDateTime(item.ScheduledAt))),
			app.Span().Body(app.Strong().Body(app.Text("Próxima tentativa: ")), app.Text(queueDateTime(item.NextAttemptAt))),
			app.Span().Body(app.Strong().Body(app.Text("Tentativas: ")), app.Text(item.AttemptCount)),
		),
	}
	if item.ExpiresAt != "" {
		main = append(main, app.P().Class("team-whatsapp-queue__expires").Body(app.Text("Prazo para envio: até "+queueDateTime(item.ExpiresAt))))
	} else if item.Status == "pendente" || item.Status == "processando" {
		main = append(main, app.P().Class("team-whatsapp-queue__expires").Body(app.Text("Sem prazo de expiração; permanece na fila até envio ou cancelamento.")))
	}
	if item.SentAt != "" {
		main = append(main, app.P().Class("team-whatsapp-queue__sent-at").Body(app.Text("Aceita pelo WhatsApp em "+queueDateTime(item.SentAt))))
	}
	if context := queueContextLines(item); len(context) > 0 {
		lines := make([]app.UI, 0, len(context))
		for _, line := range context {
			lines = append(lines, app.Span().Body(app.Text(line)))
		}
		main = append(main, app.Div().Class("team-whatsapp-queue__context").Body(lines...))
	}
	if item.LastError != "" {
		main = append(main, app.P().Class("team-whatsapp-queue__error").Role("status").Body(app.Text("Último erro: "+item.LastError)))
	}
	if len(item.History) > 0 {
		history := make([]app.UI, 0, len(item.History))
		for _, attempt := range item.History {
			line := fmt.Sprintf("Tentativa %d · %s · %s", attempt.Number, queueStatusLabel(attempt.Status), queueDateTime(attempt.FinishedAt))
			if attempt.Error != "" {
				line += " · " + attempt.Error
			}
			history = append(history, app.Li().Body(app.Text(line)))
		}
		main = append(main, app.Details().Class("team-whatsapp-queue__history").Body(app.Summary().Body(app.Text(fmt.Sprintf("Histórico de tentativas (%d)", len(item.History)))), app.Ul().Body(history...)))
	}
	actions := []app.UI{}
	if item.Status == "pendente" {
		actions = append(actions, app.Button().Class("auth-link team-whatsapp-queue__cancel").Type("button").Disabled(p.teamWhatsAppQueueLoading).OnClick(p.updateWhatsAppQueueItem(item.ID, "cancelar")).Body(app.Text("Cancelar envio")))
	}
	if item.Status == "falha" || item.Status == "pendente" && item.LastError != "" {
		actions = append(actions, app.Button().Class("auth-submit").Type("button").Disabled(p.teamWhatsAppQueueLoading).OnClick(p.updateWhatsAppQueueItem(item.ID, "tentar_novamente")).Body(app.Text("Tentar novamente")))
	}
	if len(actions) > 0 {
		main = append(main, app.Div().Class("team-whatsapp-queue__actions").Body(actions...))
	}
	return app.Article().Class("team-whatsapp-queue__card").Body(main...)
}

func queueStatusLabel(status string) string {
	switch status {
	case "pendente":
		return "Pendente"
	case "processando":
		return "Processando"
	case "enviado":
		return "Aceita pelo WhatsApp"
	case "falha":
		return "Falha"
	case "cancelado":
		return "Cancelada"
	case "expirado":
		return "Expirada"
	default:
		return status
	}
}

func queueEventLabel(event string) string {
	labels := map[string]string{
		"lembrete_manutencao_recorrente": "Manutenção preventiva recorrente",
		"lembrete_agendamento_vespera":   "Lembrete de agendamento · véspera",
		"lembrete_agendamento_uma_hora":  "Lembrete de agendamento · 1 hora",
		"orcamento_gerado":               "Orçamento com PDF",
		"ordem_servico_concluida":        "Ordem de serviço com PDF",
		"boas_vindas_autocadastro":       "Boas-vindas ao cliente",
		"boas_vindas_tecnico":            "Acesso criado pela equipe",
		"redefinicao_senha":              "Redefinição de senha",
		"mensagem_equipe":                "Mensagem da equipe",
	}
	if label := labels[event]; label != "" {
		return label
	}
	return strings.ReplaceAll(strings.ReplaceAll(event, "_", " "), "-", " ")
}

func queueDateTime(raw string) string {
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return raw
	}
	return parsed.Local().Format("02/01/2006 15:04")
}

func queueContextLines(item whatsAppQueueItem) []string {
	value := func(key string) string {
		return strings.TrimSpace(fmt.Sprint(item.Context[key]))
	}
	lines := make([]string, 0, 3)
	if client := value("cliente"); client != "" && client != "<nil>" {
		lines = append(lines, "Cliente: "+client)
	}
	if equipment := value("equipamento"); equipment != "" && equipment != "<nil>" {
		lines = append(lines, "Equipamento: "+equipment)
	}
	if item.EventType == "lembrete_manutencao_recorrente" {
		if date := queueCivilDate(value("ultima_manutencao")); date != "" {
			lines = append(lines, "Última manutenção: "+date)
		}
		if date := queueCivilDate(value("proxima_manutencao")); date != "" {
			lines = append(lines, "Próxima manutenção: "+date)
		}
		if cycle := value("intervalo_meses"); cycle != "" && cycle != "<nil>" {
			lines = append(lines, "Ciclo: "+cycle+" meses")
		}
	}
	if strings.HasPrefix(item.EventType, "lembrete_agendamento_") {
		date := queueCivilDate(value("data_agendamento"))
		hour := value("hora_agendamento")
		when := strings.TrimSpace(date + " " + hour)
		if when != "" {
			lines = append(lines, "Agendamento: "+when)
		}
		if service := value("servico"); service != "" && service != "<nil>" {
			lines = append(lines, "Serviço: "+service)
		}
	}
	return lines
}

func queueCivilDate(raw string) string {
	if len(raw) >= 10 {
		if parsed, err := time.Parse("2006-01-02", raw[:10]); err == nil {
			return parsed.Format("02/01/2006")
		}
	}
	return ""
}
