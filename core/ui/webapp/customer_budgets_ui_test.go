package webapp

import (
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/domain"
)

func TestBudgetResponseFeedbackRemainsVisibleAfterDialogCloses(t *testing.T) {
	page := &serviceCatalogPage{portalData: &domain.CustomerPortalData{}, budgetFeedback: "Proposta recusada. A Inovar foi notificada."}
	markup := app.HTMLString(page.customerBudgetsSection())
	if !strings.Contains(markup, "Proposta recusada") {
		t.Fatalf("customer budget list should show the response result: %s", markup)
	}
}

func TestBudgetDeclineShowsAPIErrorAndBusyState(t *testing.T) {
	page := &serviceCatalogPage{budgetDecline: map[string]any{"numero": "42"}, budgetFeedback: "A proposta já foi respondida.", budgetBusy: true}
	markup := app.HTMLString(page.budgetDeclineDialog())
	for _, expected := range []string{"A proposta já foi respondida.", "Enviando..."} {
		if !strings.Contains(markup, expected) {
			t.Errorf("decline dialog missing %q: %s", expected, markup)
		}
	}
}

func TestBudgetApprovalOmitsEmptyPaymentConditions(t *testing.T) {
	page := &serviceCatalogPage{budgetToSign: map[string]any{"numero": "42", "valor_total": "10.00"}}
	markup := app.HTMLString(page.budgetApprovalDialog())
	if strings.Contains(markup, "Pagamento:") {
		t.Fatalf("empty payment conditions should not be rendered: %s", markup)
	}
}

func TestPortalBudgetStatusMatchesLegacyStatuses(t *testing.T) {
	for _, test := range []struct{ status, signed, label, tone string }{
		{"ENVIADO", "", "Aguardando sua resposta", "pending"},
		{"RASCUNHO", "", "Aguardando sua resposta", "pending"},
		{"APROVADO", "2026-10-03T12:00:00Z", "Aprovado • Assinado", "complete"},
		{"APROVADO", "", "Aprovado", "complete"},
		{"RECUSADO", "", "Recusado", "cancelled"},
		{"EXPIRADO", "", "Expirado", "unknown"},
	} {
		label, tone := portalBudgetStatus(test.status, map[string]any{"assinatura_em": test.signed})
		if label != test.label || tone != test.tone {
			t.Errorf("status %s signed=%q: got %q/%q", test.status, test.signed, label, tone)
		}
	}
}

func TestPortalBudgetMoneyAndDateFormatting(t *testing.T) {
	if got := portalBrazilMoney(float64(12345.67)); got != "R$ 12.345,67" {
		t.Fatalf("money = %q", got)
	}
	if got := portalBudgetDates(map[string]any{"data": "2026-10-03", "validade": "2026-10-18"}); got != "Emitida em 03/10/2026 • válida até 18/10/2026" {
		t.Fatalf("dates = %q", got)
	}
}

func TestPortalLegacyBudgetMoneyMatchesReactToFixed(t *testing.T) {
	for value, want := range map[any]string{12345.67: "R$ 12345.67", "12345.6": "R$ 12345.60", "invalid": "R$ NaN"} {
		if got := portalLegacyBudgetMoney(value); got != want {
			t.Errorf("portalLegacyBudgetMoney(%#v)=%q want %q", value, got, want)
		}
	}
}

func TestPortalBudgetDatesIncludeLegacyItemCountWhenPresent(t *testing.T) {
	budget := map[string]any{"data": "2026-10-03", "validade": "2026-10-18"}
	if got, want := portalBudgetDates(budget, 2), "Emitida em 03/10/2026 • válida até 18/10/2026 • 2 item(ns)"; got != want {
		t.Fatalf("dates with item count=%q want %q", got, want)
	}
	if got, want := portalBudgetDates(budget, 0), "Emitida em 03/10/2026 • válida até 18/10/2026"; got != want {
		t.Fatalf("dates without item count=%q want %q", got, want)
	}
}

func TestBudgetSubmitLabelShowsPendingState(t *testing.T) {
	if got := budgetSubmitLabel(true); got != "Enviando..." {
		t.Fatalf("busy label = %q, want Enviando...", got)
	}
	if got := budgetSubmitLabel(false); got != "Confirmar e Assinar" {
		t.Fatalf("idle label = %q, want Confirmar e Assinar", got)
	}
}
