package webapp

import (
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/domain"
)

func TestTeamFinanceRendersEachPaidBudgetWithReceivedAmountAndDate(t *testing.T) {
	paid := true
	paidAt := "2026-10-02T14:30:00Z"
	received := 275.5
	page := &serviceCatalogPage{
		teamFinanceMonth: "2026-10",
		teamCustomers:    []map[string]any{{"id": "c1", "nome": "Ana Souza"}, {"id": "c2", "nome": "Bia Costa"}},
		teamServices: []map[string]any{
			{"id": "current", "cliente_id": "c1", "descricao": "Limpeza", "status": "CONCLUIDO", "data_conclusao": "2026-10-01", "data_agendamento": "2026-09-17", "valor": 275.5},
			{"id": "current-bia", "cliente_id": "c2", "descricao": "Instalação", "status": "CONCLUIDO", "data_conclusao": "2026-10-02", "data_agendamento": "2026-10-03", "valor": 80.0},
			{"id": "prior", "cliente_id": "c2", "descricao": "Reparo fora do mês", "status": "CONCLUIDO", "data_conclusao": "2026-09-30", "data_agendamento": "2026-09-28", "valor": 900.0},
		},
		teamBudgets: []domain.BudgetEstimate{{
			ID: "budget-paid", ClientName: "Ana Souza", Status: domain.BudgetApproved,
			Paid: &paid, PaidAt: &paidAt, AmountReceived: &received, FinalValue: 300,
		}},
	}
	markup := app.HTMLString(page.teamFinancePanel())
	for _, want := range []string{"Financeiro", "Tudo que entrou, o que falta receber e o que atrasou", "Recebido em 10/2026", "Serviços executados em 10/2026", "Recebimentos registrados (orçamentos)", "Ana Souza", "02/10/2026", "275,50"} {
		if !strings.Contains(markup, want) {
			t.Errorf("finance panel missing %q: %s", want, markup)
		}
	}
	if !strings.Contains(markup, "17/09/2026") || strings.Contains(markup, "Reparo fora do mês") || strings.Contains(markup, "R$ 900,00") {
		t.Errorf("completed services do not respect selected month/display date: %s", markup)
	}
	if !strings.Contains(markup, "R$ 80,00") || strings.Index(markup, "Ana Souza") > strings.Index(markup, "Bia Costa") {
		t.Errorf("customer totals are not grouped/sorted using only the selected period: %s", markup)
	}
}

func TestTeamFinanceLabelsFullHistory(t *testing.T) {
	page := &serviceCatalogPage{teamFinanceMonth: "2026-10", teamFinanceAll: true}
	markup := app.HTMLString(page.teamFinancePanel())
	for _, want := range []string{"Recebido (histórico)", "Serviços executados (histórico completo)", "Histórico completo ✓"} {
		if !strings.Contains(markup, want) {
			t.Errorf("full-history label missing %q", want)
		}
	}
}

func TestTeamFinanceKeepsDisabledPIXButtonWhenNoKeyIsConfigured(t *testing.T) {
	markup := app.HTMLString((&serviceCatalogPage{}).teamFinancePanel())
	if !strings.Contains(markup, "Copiar chave PIX") || !strings.Contains(markup, "disabled") {
		t.Fatalf("missing disabled PIX action without a configured key: %s", markup)
	}
	for _, want := range []string{"Nenhum serviço agendado pendente de recebimento.", "Nenhum orçamento aprovado aguardando recebimento."} {
		if !strings.Contains(markup, want) {
			t.Errorf("finance panel missing separate empty state %q", want)
		}
	}
}

func TestTeamFinanceSortsReceivableServicesAndLabelsOnlyOverdueOrInProgress(t *testing.T) {
	page := &serviceCatalogPage{
		teamCustomers: []map[string]any{{"id": "c1", "nome": "Ana"}},
		teamServices: []map[string]any{
			{"id": "future", "cliente_id": "c1", "descricao": "OS futura", "status": "AGENDADO", "data_agendamento": "2099-10-25", "valor": 30.0},
			{"id": "progress", "cliente_id": "c1", "descricao": "OS em andamento", "status": "EM_ANDAMENTO", "data_agendamento": "2099-10-04", "valor": 20.0},
			{"id": "late", "cliente_id": "c1", "descricao": "OS atrasada", "status": "AGENDADO", "data_agendamento": "2000-10-01", "valor": 10.0},
		},
	}
	markup := app.HTMLString(page.teamFinancePanel())
	lateAt, progressAt, futureAt := strings.Index(markup, "OS atrasada"), strings.Index(markup, "OS em andamento"), strings.Index(markup, "OS futura")
	if lateAt < 0 || progressAt < 0 || futureAt < 0 || !(lateAt < progressAt && progressAt < futureAt) {
		t.Fatalf("receivables are not sorted by scheduled date: %s", markup)
	}
	if !strings.Contains(markup, "• atrasado") || !strings.Contains(markup, "• em andamento") {
		t.Fatalf("missing overdue/in-progress labels: %s", markup)
	}
	rowStart := strings.LastIndex(markup[:futureAt], "<article")
	rowEndRelative := strings.Index(markup[futureAt:], "</article>")
	if rowStart < 0 || rowEndRelative < 0 {
		t.Fatalf("could not isolate future service row: %s", markup)
	}
	futureRow := markup[rowStart : futureAt+rowEndRelative]
	if strings.Contains(futureRow, "em andamento") || strings.Contains(futureRow, "atrasado") {
		t.Fatalf("future scheduled service has an incorrect status tag: %s", futureRow)
	}
}
