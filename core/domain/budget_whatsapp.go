package domain

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// WhatsAppBudgetMessage mirrors the commercial proposal copy used by the
// existing client, keeping message generation shared in the Go domain.
func WhatsAppBudgetMessage(budget BudgetEstimate, profile TechnicianProfile) string {
	firstName := "Cliente"
	if names := strings.Fields(strings.TrimSpace(budget.ClientName)); len(names) > 0 {
		firstName = names[0]
	}
	technician := strings.TrimSpace(profile.Name)
	if technician == "" {
		technician = "Técnico Responsável"
	}
	business := strings.TrimSpace(profile.BusinessName)
	if business == "" {
		business = "Inovar Refrigeração"
	}
	items := make([]string, 0, len(budget.Items))
	for _, item := range budget.Items {
		description := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(item.Description, "\r", " "), "\n", " "))
		items = append(items, fmt.Sprintf("• *%s* (%sx) = R$ %s", description, budgetQuantity(item.Quantity), budgetNumber(item.TotalPrice)))
	}
	if len(items) == 0 {
		items = append(items, "• Nenhum item informado")
	}
	discount := ""
	if budget.Discount > 0 {
		discount = fmt.Sprintf("🎁 *Desconto especial:* R$ %s\n", budgetNumber(budget.Discount))
	}
	notes := ""
	if strings.TrimSpace(budget.Notes) != "" {
		notes = "📝 *Observações:* " + strings.TrimSpace(budget.Notes)
	}
	return ApplyWhatsAppPlaceholders(ResolveWhatsAppTemplate(profile.WhatsAppMessages, "orcamento_criado"), map[string]string{
		"cliente":     firstName,
		"tecnico":     technician,
		"empresa":     business,
		"numero":      budgetReference(budget),
		"valor":       "R$ " + budgetNumber(budget.FinalValue),
		"itens":       strings.Join(items, "\n"),
		"equipamento": firstBudgetValue(budget.ApplianceDescription, "Ar-Condicionado"),
		"data":        budgetDate(budget.Date),
		"validade":    budgetDate(budget.ValidUntil),
		"desconto":    discount,
		"pagamento":   firstBudgetValue(budget.PaymentConditions, "A combinar"),
		"prazo":       firstBudgetValue(budget.ExecutionTime, "A combinar"),
		"garantia":    firstBudgetValue(budget.WarrantyTerms, "Conforme serviço"),
		"observacoes": notes,
		"app":         AppPublicURL,
	})
}

func budgetReference(budget BudgetEstimate) string {
	if strings.TrimSpace(budget.Number) != "" {
		return strings.ToUpper(strings.TrimSpace(budget.Number))
	}
	if strings.TrimSpace(budget.ID) == "" {
		return strings.ToUpper(strings.TrimSpace(budget.Number))
	}
	value := strings.ToUpper(strings.TrimSpace(budget.ID))
	if len(value) > 8 {
		return value[:8]
	}
	return value
}

func budgetNumber(value float64) string { return strconv.FormatFloat(value, 'f', 2, 64) }

func budgetQuantity(value float64) string { return strconv.FormatFloat(value, 'f', -1, 64) }

func budgetDate(value string) string {
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(value)[:min(len(strings.TrimSpace(value)), 10)])
	if err != nil {
		return "Não informado"
	}
	return parsed.Format("02/01/2006")
}

func firstBudgetValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
