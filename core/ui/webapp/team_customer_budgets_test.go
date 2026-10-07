package webapp

import (
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/domain"
)

func TestBudgetsForCustomerMatchesLegacyPaidAndReceivableTotals(t *testing.T) {
	paid, unpaid := true, false
	budgets := []domain.BudgetEstimate{
		{ID: "paid", ClientID: "client-1", FinalValue: 120, Paid: &paid},
		{ID: "open", ClientID: "client-1", FinalValue: 80, Paid: &unpaid, Status: domain.BudgetApproved},
		{ID: "declined", ClientID: "client-1", FinalValue: 50, Status: domain.BudgetDeclined},
		{ID: "other", ClientID: "client-2", FinalValue: 999},
	}
	rows, received, open := budgetsForCustomer(budgets, "client-1")
	if len(rows) != 3 || received != 120 || open != 80 {
		t.Fatalf("rows=%d received=%v open=%v", len(rows), received, open)
	}
}

func TestTeamCustomerBudgetPanelRendersLegacyTextsAndFallback(t *testing.T) {
	paid, unpaid := true, false
	markup := app.HTMLString(teamCustomerBudgetPanel([]domain.BudgetEstimate{
		{ClientID: "client-1", ApplianceDescription: "Ar condicionado — sala", FinalValue: 150, Paid: &paid},
		{ClientID: "client-1", FinalValue: 40, Paid: &unpaid},
	}, "client-1"))
	for _, text := range []string{"Orçamentos", "Pagamentos", "Ar condicionado — sala", "R$ 150.00", "Serviço", "Recebido: R$ 150,00", "A receber: R$ 40,00"} {
		if !strings.Contains(markup, text) {
			t.Errorf("customer budget panel missing %q: %s", text, markup)
		}
	}
	empty := app.HTMLString(teamCustomerBudgetPanel(nil, "client-1"))
	if !strings.Contains(empty, "Nenhum orçamento para este cliente ainda.") {
		t.Fatalf("empty state missing: %s", empty)
	}
}

func TestTeamCustomerQuickContactLinksMatchReactPhoneBehavior(t *testing.T) {
	markup := app.HTMLString(teamCustomerQuickContactLinks("(27) 99999-0000"))
	for _, want := range []string{"WhatsApp", "https://api.whatsapp.com/send?phone=5527999990000", `target="_blank"`, "tel:(27) 99999-0000", "Ligar"} {
		if !strings.Contains(markup, want) {
			t.Errorf("quick contact links missing %q: %s", want, markup)
		}
	}
	if got := app.HTMLString(teamCustomerQuickContactLinks("")); got != "<div></div>" {
		t.Fatalf("empty phone should not render dead links: %s", got)
	}
}

func TestTeamCustomerApplianceTechnicalSummaryPreservesReactDefaults(t *testing.T) {
	if got := teamCustomerApplianceTechnicalSummary(map[string]any{"tipo": "Cassete", "gas_tipo": "R-32", "tensao": "Bivolt"}); got != "Cassete • R-32 • Bivolt" {
		t.Fatalf("summary=%q", got)
	}
	if got := teamCustomerApplianceTechnicalSummary(map[string]any{}); got != "Split Hi-Wall • R-410A • 220V" {
		t.Fatalf("legacy defaults=%q", got)
	}
}
