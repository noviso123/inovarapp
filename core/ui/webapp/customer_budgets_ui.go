package webapp

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

func (p *serviceCatalogPage) customerBudgetsSection() app.UI {
	budgets := p.portalData.Budgets
	content := []app.UI{
		app.Div().Class("portal-budgets__heading").Body(
			app.Div().Body(app.H3().Body(app.Text("Meus Orçamentos")), app.P().Class("portal-section__intro").Body(app.Text("Propostas enviadas pela Inovar — aprove com sua assinatura"))),
		),
	}
	if p.budgetFeedback != "" && p.budgetToSign == nil && p.budgetDecline == nil {
		content = append(content, app.P().Class("portal-budget__feedback").Attr("role", "status").Body(
			app.Text(p.budgetFeedback),
			app.Button().Class("auth-link").Type("button").OnClick(func(ctx app.Context, event app.Event) {
				event.PreventDefault()
				p.budgetFeedback = ""
			}).Body(app.Text("Fechar")),
		))
	}
	if len(budgets) == 0 {
		content = append(content, app.P().Class("portal-section__empty").Body(app.Text("Nenhum orçamento recebido ainda. Quando o técnico enviar uma proposta, ela aparece aqui para você aprovar.")))
	}
	for _, budget := range budgets {
		meta := decodeBudgetMetadata(portalText(budget["descricao"]))
		status := strings.ToUpper(portalText(budget["status"]))
		label, tone := portalBudgetStatus(status, meta)
		number := portalText(budget["numero"])
		itemCount := 0
		if items, ok := meta["items"].([]any); ok {
			itemCount = len(items)
		}
		lines := []app.UI{
			app.Div().Class("portal-service__heading").Body(app.Strong().Body(app.Text("Proposta #"+number)), app.Span().Class("portal-service__status portal-service__status--"+tone).Body(app.Text(label))),
			app.P().Class("portal-service__date").Body(app.Text(portalBudgetDates(budget, itemCount))),
		}
		if kind := portalText(budget["tipo_servico"]); kind != "" {
			lines = append(lines, app.P().Class("portal-service__date").Body(app.Text(kind)))
		}
		if signed := portalText(meta["assinatura_em"]); signed != "" {
			lines = append(lines, app.P().Class("portal-service__date").Body(app.Text("Assinado digitalmente em "+portalDateTime(signed))))
		}
		if items, ok := meta["items"].([]any); ok && len(items) > 0 {
			for _, raw := range items {
				item, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				description := portalText(item["description"])
				if description == "" {
					continue
				}
				lines = append(lines, app.P().Class("portal-budget__item").Body(app.Text(description+portalBudgetItemTotal(item))))
			}
		}
		lines = append(lines, app.Div().Class("portal-budget__total").Body(
			app.Span().Body(app.Text("Valor total")), app.Strong().Body(app.Text(portalLegacyBudgetMoney(budget["valor_total"]))),
		))
		if conditions := portalText(budget["condicoes"]); conditions != "" {
			lines = append(lines, app.P().Class("portal-service__date").Body(app.Text("Pagamento: "+conditions)))
		}
		if status == "ENVIADO" || status == "RASCUNHO" {
			lines = append(lines, app.Div().Class("portal-budget__actions").Body(
				app.Button().Class("auth-submit").Type("button").OnClick(p.openBudgetApproval(budget)).Body(app.Text("Aprovar e Assinar")),
				app.Button().Class("auth-link").Type("button").OnClick(p.openBudgetDecline(budget)).Body(app.Text("Recusar")),
			))
		}
		content = append(content, app.Article().Class("portal-service portal-budget").Body(lines...))
	}
	return app.Div().Class("portal-budget-list").Body(content...)
}

func decodeBudgetMetadata(raw string) map[string]any {
	metadata := map[string]any{}
	_ = json.Unmarshal([]byte(raw), &metadata)
	return metadata
}

func portalBudgetStatus(status string, metadata map[string]any) (string, string) {
	switch status {
	case "APROVADO":
		if portalText(metadata["assinatura_em"]) != "" {
			return "Aprovado • Assinado", "complete"
		}
		return "Aprovado", "complete"
	case "RECUSADO":
		return "Recusado", "cancelled"
	case "CANCELADO":
		return "Cancelado", "cancelled"
	case "EXPIRADO":
		return "Expirado", "unknown"
	default:
		return "Aguardando sua resposta", "pending"
	}
}

func portalBudgetDates(budget map[string]any, itemCounts ...int) string {
	date := portalDate(budget["data"])
	if date == "" {
		date = "—"
	}
	result := "Emitida em " + date
	if validUntil := portalDate(budget["validade"]); validUntil != "" {
		result += " • válida até " + validUntil
	}
	if len(itemCounts) > 0 && itemCounts[0] > 0 {
		result += " • " + strconv.Itoa(itemCounts[0]) + " item(ns)"
	}
	return result
}

func portalDateTime(value string) string {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.000Z", "2006-01-02T15:04:05Z"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Local().Format("02/01/2006 15:04")
		}
	}
	return value
}

func portalBudgetItemTotal(item map[string]any) string {
	parts := []string{}
	if quantity, ok := item["quantity"].(float64); ok && quantity > 1 {
		parts = append(parts, "× "+strconv.FormatFloat(quantity, 'f', -1, 64))
	}
	if total, ok := item["totalPrice"].(float64); ok {
		parts = append(parts, portalBrazilMoney(total))
	}
	if len(parts) == 0 {
		return ""
	}
	return " • " + strings.Join(parts, " • ")
}

func portalBrazilMoney(value any) string {
	amount, ok := value.(float64)
	if !ok {
		if raw, isString := value.(string); isString {
			amount, _ = strconv.ParseFloat(strings.ReplaceAll(raw, ",", "."), 64)
		}
	}
	return "R$ " + formatPortalMoney(amount)
}

// portalLegacyBudgetMoney preserves React's Number(value).toFixed(2) output.
func portalLegacyBudgetMoney(value any) string {
	amount := 0.0
	switch v := value.(type) {
	case float64:
		amount = v
	case float32:
		amount = float64(v)
	case int:
		amount = float64(v)
	case int64:
		amount = float64(v)
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return "R$ NaN"
		}
		amount = parsed
	}
	return "R$ " + strconv.FormatFloat(amount, 'f', 2, 64)
}

func (p *serviceCatalogPage) openBudgetApproval(budget map[string]any) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		p.budgetToSign, p.budgetSignature, p.budgetFeedback = budget, "", ""
		clearBudgetSignatureCanvas()
		p.prepareBudgetCanvas(ctx, 0)
	}
}

func (p *serviceCatalogPage) prepareBudgetCanvas(ctx app.Context, attempts int) {
	ctx.After(60*time.Millisecond, func(next app.Context) {
		if p.budgetToSign == nil {
			return
		}
		if !prepareBudgetSignatureCanvas() && attempts < 20 {
			p.prepareBudgetCanvas(next, attempts+1)
			return
		}
		p.pollBudgetSignature(next)
	})
}

func (p *serviceCatalogPage) pollBudgetSignature(ctx app.Context) {
	ctx.After(120*time.Millisecond, func(next app.Context) {
		if p.budgetToSign == nil {
			return
		}
		value := currentBudgetSignature()
		if value != p.budgetSignature {
			p.budgetSignature = value
			next.Update()
		}
		// Keep watching while the dialog is open: the user can draw, clear,
		// and redraw. Stopping after the first stroke left the confirm button
		// disabled if they cleared their signature.
		p.pollBudgetSignature(next)
	})
}

func (p *serviceCatalogPage) openBudgetDecline(budget map[string]any) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		p.budgetDecline = budget
		p.budgetFeedback = ""
	}
}

func (p *serviceCatalogPage) closeBudgetDialogs(ctx app.Context, event app.Event) {
	event.PreventDefault()
	p.budgetToSign, p.budgetDecline, p.budgetSignature = nil, nil, ""
	clearBudgetSignatureCanvas()
}

func (p *serviceCatalogPage) budgetApprovalDialog() app.UI {
	budget := p.budgetToSign
	return app.Div().Class("auth-backdrop").Body(app.Div().Class("auth-dialog").Body(
		app.Div().Class("auth-dialog__heading").Body(app.Div().Body(app.H3().Class("auth-dialog__title").Body(app.Text("Aprovar Proposta #"+portalText(budget["numero"]))), app.P().Class("auth-dialog__intro").Body(app.Text("Confirme os dados e assine para aprovar."))), app.Button().Class("auth-close").Type("button").OnClick(p.closeBudgetDialogs).Body(app.Text("Fechar"))),
		app.Div().Class("auth-form").Body(
			app.Div().Class("portal-budget__summary").Body(app.Div().Body(app.Span().Body(app.Text("Valor total: ")), app.Strong().Body(app.Text(portalLegacyBudgetMoney(budget["valor_total"])))), budgetPaymentSummary(budget)),
			app.Label().Class("auth-field").Body(app.Text("Assinatura Digital do Cliente"), app.Canvas().ID("budget-signature-canvas").Class("budget-signature-canvas").Attr("width", 640).Attr("height", 180)),
			app.Button().Class("auth-link").Type("button").OnClick(func(ctx app.Context, event app.Event) {
				event.PreventDefault()
				clearBudgetSignatureCanvas()
				p.budgetSignature = ""
				ctx.Update()
			}).Body(app.Text("Limpar assinatura")),
			app.Div().Class("portal-budget__notice").Body(app.Text("Ao assinar, você aprova a proposta e a Inovar recebe a notificação imediatamente para agendar o serviço. A assinatura fica registrada no orçamento.")),
			formErrorNotice(p.budgetFeedback),
			app.Div().Class("portal-budget__actions").Body(app.Button().Class("auth-link").Type("button").OnClick(p.closeBudgetDialogs).Body(app.Text("Voltar")), app.Button().Class("auth-submit").Type("button").Disabled(p.budgetSignature == "" || p.budgetBusy).OnClick(p.respondBudget("APROVAR", budget)).Body(app.Text(budgetSubmitLabel(p.budgetBusy)))),
		),
	))
}

func (p *serviceCatalogPage) budgetDeclineDialog() app.UI {
	budget := p.budgetDecline
	return app.Div().Class("auth-backdrop").Body(app.Div().Class("auth-dialog").Body(
		app.Div().Class("auth-dialog__heading").Body(app.Div().Body(app.H3().Class("auth-dialog__title").Body(app.Text("Recusar proposta #"+portalText(budget["numero"]))), app.P().Class("auth-dialog__intro").Body(app.Text("Recusar a proposta #"+portalText(budget["numero"])+"? A Inovar será notificada.")))),
		app.Div().Class("auth-form").Body(budgetDeclineError(p.budgetFeedback), app.Div().Class("portal-budget__actions").Body(app.Button().Class("auth-link").Type("button").OnClick(p.closeBudgetDialogs).Body(app.Text("Voltar")), app.Button().Class("auth-submit").Type("button").Disabled(p.budgetBusy).OnClick(p.respondBudget("RECUSAR", budget)).Body(app.Text(budgetDeclineSubmitLabel(p.budgetBusy))))),
	))
}

func (p *serviceCatalogPage) respondBudget(action string, budget map[string]any) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		signature := p.budgetSignature
		if action == "APROVAR" {
			signature = currentBudgetSignature()
			if signature == "" {
				p.budgetFeedback = "Desenhe sua assinatura antes de confirmar."
				return
			}
		}
		if p.session == nil || p.budgetBusy {
			return
		}
		pageURL := app.Window().URL()
		if pageURL == nil {
			return
		}
		endpoint, token := apiBaseURL()+"/api/orcamento-resposta", p.session.AccessToken
		p.budgetBusy, p.budgetFeedback = true, ""
		payload := map[string]any{"orcamento_id": portalText(budget["id"]), "acao": action, "assinatura": signature}
		body, _ := json.Marshal(payload)
		go func() {
			request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
			if err == nil {
				request.Header.Set("Authorization", "Bearer "+token)
				request.Header.Set("Content-Type", "application/json")
				var response *http.Response
				response, err = http.DefaultClient.Do(request)
				if err == nil {
					defer response.Body.Close()
					if response.StatusCode < 200 || response.StatusCode >= 300 {
						var failure map[string]any
						_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&failure)
						p.budgetFeedback = portalText(failure["error"])
						if p.budgetFeedback == "" {
							p.budgetFeedback = "Não foi possível responder ao orçamento."
						}
						err = io.ErrUnexpectedEOF
					} else {
						var result struct {
							Message string `json:"mensagem"`
						}
						_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result)
						p.budgetFeedback = strings.TrimSpace(result.Message)
					}
				}
			}
			if err != nil && p.budgetFeedback == "" {
				p.budgetFeedback = "Erro ao responder orçamento. Verifique sua conexão e tente novamente."
			}
			p.budgetBusy = false
			if err == nil {
				if p.budgetFeedback == "" {
					if action == "APROVAR" {
						p.budgetFeedback = "Orçamento aprovado e assinado! A Inovar foi notificada."
					} else {
						p.budgetFeedback = "Orçamento recusado. A Inovar foi notificada."
					}
				}
				p.budgetToSign, p.budgetDecline = nil, nil
				p.portalData, p.portalLoadedFor = nil, ""
				p.loadCustomerPortal(ctx)
			}
			ctx.Update()
		}()
	}
}

func budgetSubmitLabel(busy bool) string {
	if busy {
		return "Enviando..."
	}
	return "Confirmar e Assinar"
}

func budgetDeclineSubmitLabel(busy bool) string {
	if busy {
		return "Enviando..."
	}
	return "Confirmar recusa"
}

func budgetDeclineError(feedback string) app.UI {
	if feedback == "" {
		return nil
	}
	return app.P().Class("portal-section__error").Attr("role", "alert").Body(app.Text(feedback))
}

func budgetPaymentSummary(budget map[string]any) app.UI {
	conditions := portalText(budget["condicoes"])
	if conditions == "" {
		return nil
	}
	return app.Div().Body(app.Span().Body(app.Text("Pagamento: ")), app.Span().Body(app.Text(conditions)))
}
