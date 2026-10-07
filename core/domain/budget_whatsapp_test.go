package domain

import (
	"strings"
	"testing"
)

func TestWhatsAppBudgetMessagePreservesCommercialDetailsAndUTF8(t *testing.T) {
	message := WhatsAppBudgetMessage(BudgetEstimate{
		ID: "abcdef12-3456-7890", ClientName: "João Silva", ApplianceDescription: "LG Inverter 12.000 BTUs (Sala)",
		Date: "2026-10-03", ValidUntil: "2026-10-18", Discount: 15, FinalValue: 235,
		PaymentConditions: "PIX ou cartão", ExecutionTime: "2 horas", WarrantyTerms: "90 dias", Notes: "Verificar tensão 220 V",
		Items: []BudgetItem{{Description: "Higienização de serpentina", Quantity: 1, TotalPrice: 250}},
	}, TechnicianProfile{Name: "Gabriel", BusinessName: "Inovar Refrigeração"})
	for _, want := range []string{"Olá, *João*!", "Gabriel", "Inovar Refrigeração", "#ABCDEF12", "03/10/2026", "18/10/2026", "Higienização de serpentina", "(1x)", "R$ 15.00", "R$ 235.00", "PIX ou cartão", "2 horas", "90 dias", "220 V"} {
		if !strings.Contains(message, want) {
			t.Errorf("message missing %q: %s", want, message)
		}
	}
}

func TestWhatsAppBudgetMessageUsesFallbacks(t *testing.T) {
	message := WhatsAppBudgetMessage(BudgetEstimate{ID: "12345678-aaaa", ClientName: "", Date: "bad", ValidUntil: "", Items: nil}, TechnicianProfile{})
	for _, want := range []string{"Olá, *Cliente*!", "Técnico Responsável", "Inovar Refrigeração", "Ar-Condicionado", "Não informado", "Nenhum item informado"} {
		if !strings.Contains(message, want) {
			t.Errorf("message missing fallback %q", want)
		}
	}
}
