package webapp

import (
	"strconv"
	"strings"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/domain"
)

func budgetsForCustomer(budgets []domain.BudgetEstimate, customerID string) (rows []domain.BudgetEstimate, received, open float64) {
	for _, budget := range budgets {
		if budget.ClientID != customerID {
			continue
		}
		rows = append(rows, budget)
		paid := budget.Paid != nil && *budget.Paid
		if paid {
			received += budget.FinalValue
		} else if budget.Status != domain.BudgetDeclined && budget.Status != domain.BudgetCancelled {
			open += budget.FinalValue
		}
	}
	return rows, received, open
}

func teamCustomerBudgetPanel(budgets []domain.BudgetEstimate, customerID string) app.UI {
	rows, received, open := budgetsForCustomer(budgets, customerID)
	content := []app.UI{
		app.Span().Class("catalog__eyebrow").Body(app.Text("Orçamentos & Pagamentos")),
	}
	if len(rows) == 0 {
		content = append(content, app.P().Class("portal-section__empty").Body(app.Text("Nenhum orçamento para este cliente ainda.")))
	} else {
		items := make([]app.UI, 0, len(rows))
		for _, budget := range rows {
			description := strings.TrimSpace(budget.ApplianceDescription)
			if description == "" {
				description = "Serviço"
			} else if runes := []rune(description); len(runes) > 34 {
				description = string(runes[:34])
			}
			paid := budget.Paid != nil && *budget.Paid
			icon, color := "⏳", "portal-section__intro"
			if paid {
				icon, color = "✅", "portal-service__value"
			} else if budget.Status == domain.BudgetCancelled {
				icon, color = "✕", "portal-section__intro"
			}
			items = append(items, app.Div().Class("portal-service__heading").Body(
				app.Span().Class(color).Body(app.Text(icon+" "+description)),
				app.Strong().Class(color).Body(app.Text("R$ "+strconv.FormatFloat(budget.FinalValue, 'f', 2, 64))),
			))
		}
		content = append(content,
			app.Div().Class("team-customer-budget-list").Body(items...),
			app.Div().Class("portal-service__heading").Body(
				app.Strong().Class("portal-service__value").Body(app.Text("Recebido: R$ "+formatPortalMoney(received))),
				app.Strong().Class("portal-section__intro").Body(app.Text("A receber: R$ "+formatPortalMoney(open))),
			),
		)
	}
	return app.Div().Class("team-customer-budget-summary").Body(content...)
}

func teamCustomerQuickContactLinks(phone string) app.UI {
	clean := cleanTeamContactPhone(phone)
	if clean == "" {
		return app.Div()
	}
	return app.Div().Class("portal-budget__actions team-customer-contact-actions").Body(
		app.A().Class("auth-submit team-customer-contact-link team-customer-contact-link--whatsapp").Href("https://api.whatsapp.com/send?phone="+clean).Target("_blank").Rel("noopener noreferrer").Body(app.Text("WhatsApp")),
		app.A().Class("auth-link team-customer-contact-link team-customer-contact-link--call").Href("tel:"+phone).Body(app.Text("Ligar")),
	)
}

func teamCustomerApplianceTechnicalSummary(appliance map[string]any) string {
	typeName := firstNonEmptyBudget(portalText(appliance["tipo"]), "Split Hi-Wall")
	gas := firstNonEmptyBudget(portalText(appliance["gas_tipo"]), "R-410A")
	voltage := firstNonEmptyBudget(portalText(appliance["tensao"]), "220V")
	return strings.Join([]string{typeName, gas, voltage}, " • ")
}
