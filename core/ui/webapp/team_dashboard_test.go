package webapp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"inovarapp/core/adapter/httpapi"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/domain"
)

func TestBuildTeamDashboardSummaryCountsTeamOperations(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.FixedZone("BRT", -3*60*60))
	customers := []map[string]any{{"id": "c1", "appliances": []any{map[string]any{"id": "a1", "marca": "LG"}, map[string]any{"id": "a2", "marca": "Samsung"}}}, {"id": "c2", "appliances": []any{map[string]any{"id": "a3", "marca": "Midea"}}}}
	services := []map[string]any{
		{"id": "pending", "cliente_id": "c1", "aparelho_id": "a1", "status": "PENDENTE", "data_solicitacao": "2026-10-03"},
		{"id": "late", "cliente_id": "c1", "aparelho_id": "a1", "status": "AGENDADO", "data_agendamento": "2026-10-02", "valor": 90.0},
		{"id": "scheduled", "cliente_id": "c1", "aparelho_id": "a2", "status": "AGENDADO", "data_agendamento": "2026-10-05", "valor": 50.0},
		{"id": "progress", "cliente_id": "c1", "aparelho_id": "a1", "status": "EM_ANDAMENTO", "data_agendamento": "2026-10-04"},
		{"id": "complete", "cliente_id": "c2", "aparelho_id": "a3", "status": "CONCLUIDO", "data_conclusao": "2026-10-02", "valor": 245.5},
		{"id": "cancel", "status": "CANCELADO"},
	}
	linked := "complete"
	budgets := []domain.BudgetEstimate{
		{Status: domain.BudgetApproved, FinalValue: 100, ServiceID: &linked},
		{Status: domain.BudgetApproved, FinalValue: 80},
		{Status: domain.BudgetPending, FinalValue: 30},
	}
	summary := buildTeamDashboardSummary(customers, services, nil, budgets, nil, 6, now)
	if summary.Customers != 2 || summary.Appliances != 3 || summary.TotalServices != 6 || summary.TotalBudgets != 3 || summary.PendingRequests != 1 || summary.Overdue != 0 || summary.ReturnsOverdue != 1 || summary.Scheduled != 2 || summary.InProgress != 1 || summary.Completed != 1 || summary.Cancelled != 1 || summary.Revenue != 325.5 {
		t.Fatalf("summary=%+v", summary)
	}
}

func TestDashboardScheduledCountExcludesInProgressServices(t *testing.T) {
	today := time.Date(2026, 10, 3, 12, 0, 0, 0, time.FixedZone("BRT", -3*60*60))
	services := []map[string]any{
		{"id": "scheduled", "status": "AGENDADO", "data_agendamento": "2026-10-04"},
		{"id": "in-progress", "status": "EM_ANDAMENTO", "data_agendamento": "2026-10-04"},
		{"id": "pending", "status": "PENDENTE", "data_solicitacao": "2026-10-04"},
	}
	summary := buildTeamDashboardSummary(nil, services, nil, nil, nil, 6, today)
	if summary.Scheduled != 1 || summary.InProgress != 1 || summary.PendingRequests != 1 {
		t.Fatalf("scheduled=%d in_progress=%d pending=%d", summary.Scheduled, summary.InProgress, summary.PendingRequests)
	}
}

func TestBuildReturnQueueUsesScheduledThenLatestCompletionAndLegacyMarker(t *testing.T) {
	today := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	customers := []map[string]any{
		{"id": "c1", "nome": "Ana", "appliances": []any{
			map[string]any{"id": "a1", "marca": "LG", "modelo": "S1"},
			map[string]any{"id": "a2", "marca": "Samsung"},
			map[string]any{"id": "a3", "marca": "Midea"},
			map[string]any{"id": "a4", "marca": "Carrier"},
		}},
	}
	services := []map[string]any{
		{"id": "done-old", "aparelho_id": "a1", "status": "CONCLUIDO", "data_conclusao": "2026-01-01", "observacoes": "[PROXIMO_RETORNO:2026-07-01]"},
		{"id": "done-new", "aparelho_id": "a1", "status": "CONCLUIDO", "data_conclusao": "2026-04-01", "observacoes": "[PROXIMO_RETORNO:2026-10-04]"},
		{"id": "scheduled", "aparelho_id": "a1", "status": "AGENDADO", "data_agendamento": "2026-10-10"},
		{"id": "done-week", "aparelho_id": "a2", "status": "CONCLUIDO", "data_conclusao": "2026-03-28", "observacoes": "texto"},
		{"id": "done-soon", "aparelho_id": "a3", "status": "CONCLUIDO", "data_conclusao": "2026-08-20", "observacoes": "texto"},
	}
	budgets := []domain.BudgetEstimate{{ClientID: "c1", Status: domain.BudgetApproved, FinalValue: 50, ApplianceID: stringPtrDashboard("a4")}}
	items, overdue, week, soon, noHistory := buildReturnQueue(customers, services, []map[string]any{{"service_id": "scheduled", "data": "2026-10-10", "hora": "14:30"}}, budgets, nil, 6, today)
	if len(items) != 4 || overdue != 1 || week != 2 || soon != 0 || noHistory != 0 {
		t.Fatalf("items=%+v counts=%d/%d/%d/%d", items, overdue, week, soon, noHistory)
	}
	want := map[string]string{"LG S1": "2026-10-10", "Samsung": "2026-09-28", "Midea": "2027-02-20", "Carrier": "2026-10-03"}
	for _, item := range items {
		if item.Date != want[item.Appliance] {
			t.Errorf("%s return=%s want=%s", item.Appliance, item.Date, want[item.Appliance])
		}
		if item.Appliance == "LG S1" && (item.ScheduledServiceID != "scheduled" || item.ScheduledTime != "14:30") {
			t.Errorf("scheduled service context not retained: %+v", item)
		}
	}
}

func TestBuildReturnQueueIncludesStandaloneHistoryAndDeduplicatesMirrors(t *testing.T) {
	today := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	customers := []map[string]any{{"id": "c1", "nome": "Ana", "appliances": []any{map[string]any{"id": "a1", "marca": "LG"}}}}
	services := []map[string]any{
		{"id": "s1", "aparelho_id": "a1", "status": "CONCLUIDO", "data_conclusao": "2026-04-01", "observacoes": "[PROXIMO_RETORNO:2026-10-04]"},
	}
	history := []map[string]any{
		{"id": "mirror", "service_id": "s1", "aparelho_id": "a1", "data": "2026-04-01", "descricao": "espelho"},
		{"id": "duplicate", "aparelho_id": "a1", "data": "2026-04-01", "descricao": "sem service_id"},
		{"id": "standalone", "aparelho_id": "a1", "data": "2026-06-01", "descricao": "Histórico manual", "observacoes": "[PROXIMO_RETORNO:2026-12-01]"},
	}
	items, overdue, week, soon, noHistory := buildReturnQueue(customers, services, nil, nil, history, 6, today)
	if len(items) != 1 || items[0].Date != "2026-12-01" || overdue != 0 || week != 0 || soon != 0 || noHistory != 0 {
		t.Fatalf("items=%+v counts=%d/%d/%d/%d", items, overdue, week, soon, noHistory)
	}
	withoutService := []map[string]any{{"id": "manual", "aparelho_id": "a1", "data": "2026-06-01", "descricao": "Histórico manual", "observacoes": "[PROXIMO_RETORNO:2026-12-01]"}}
	items, _, _, soon, _ = buildReturnQueue(customers, nil, nil, nil, withoutService, 6, today)
	if len(items) != 1 || items[0].Date != "2026-12-01" || soon != 0 {
		t.Fatalf("standalone history not used: items=%+v soon=%d", items, soon)
	}
}

func TestBuildReturnQueueIncludesCustomerOnlyBudgetsAndUnlinkedCalls(t *testing.T) {
	today := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	customers := []map[string]any{
		{"id": "c1", "nome": "Ana", "appliances": []any{}},
		{"id": "c2", "nome": "Bia", "appliances": []any{map[string]any{"id": "a1", "marca": "LG"}}},
		{"id": "c3", "nome": "Caio", "appliances": []any{map[string]any{"id": "a2", "marca": "Samsung"}}},
	}
	services := []map[string]any{{"id": "active-with-device", "cliente_id": "c3", "aparelho_id": "a2", "status": "EM_ANDAMENTO"}}
	budgets := []domain.BudgetEstimate{
		{ID: "b1", ClientID: "c1", Status: domain.BudgetPending, ValidUntil: "2026-10-08", ApplianceDescription: "Portátil"},
		{ID: "b2", ClientID: "c1", Status: domain.BudgetApproved, FinalValue: 200, ApplianceDescription: "Split aprovado"},
		{ID: "b3", ClientID: "c2", Status: domain.BudgetPending, ValidUntil: "2026-10-08", ApplianceID: stringPtrDashboard("a1"), ApplianceDescription: "LG"},
	}
	items, overdue, week, soon, noHistory := buildReturnQueue(customers, services, nil, budgets, nil, 6, today)
	if len(items) != 4 || overdue != 0 || week != 3 || soon != 0 || noHistory != 1 {
		t.Fatalf("items=%+v counts=%d/%d/%d/%d", items, overdue, week, soon, noHistory)
	}
	labels := map[string]string{}
	for _, item := range items {
		labels[item.Appliance] = item.Date
	}
	if labels["Portátil"] != "2026-10-08" || labels["Split aprovado"] != "2026-10-03" || labels["LG"] != "2026-10-08" || labels["Samsung"] != "" {
		t.Fatalf("return items=%+v", items)
	}
}

func TestBuildReturnQueueRetainsRelatedBudgetIDForExpandedCard(t *testing.T) {
	today := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	customers := []map[string]any{
		{"id": "c1", "nome": "Ana", "whatsapp": "27999990000", "appliances": []any{map[string]any{"id": "a1", "marca": "LG"}}},
		{"id": "c2", "nome": "Bia", "appliances": []any{}},
	}
	budgets := []domain.BudgetEstimate{
		{ID: "linked", ClientID: "c1", Status: domain.BudgetPending, ValidUntil: "2026-10-08", ApplianceID: stringPtrDashboard("a1")},
		{ID: "customer-only", ClientID: "c2", Status: domain.BudgetApproved},
	}
	items, _, _, _, _ := buildReturnQueue(customers, nil, nil, budgets, nil, 6, today)
	ids := map[string]string{}
	for _, item := range items {
		ids[item.CustomerID] = item.BudgetID
	}
	if ids["c1"] != "linked" || ids["c2"] != "customer-only" {
		t.Fatalf("queue lost budget references: %v", ids)
	}
}

func TestReturnCountersIncludeOnlyAppliancesWithoutExistingOpenWork(t *testing.T) {
	today := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	customers := []map[string]any{{"id": "c1", "nome": "Ana", "appliances": []any{
		map[string]any{"id": "a1", "marca": "LG"},
		map[string]any{"id": "a2", "marca": "Samsung"},
	}}}
	services := []map[string]any{{"id": "open", "cliente_id": "c1", "aparelho_id": "a2", "status": "EM_ANDAMENTO"}}
	budgets := []domain.BudgetEstimate{{ID: "b1", ClientID: "c1", Status: domain.BudgetPending, ValidUntil: "2026-10-08", ApplianceID: stringPtrDashboard("a1")}}
	items, overdue, week, _, noHistory := buildReturnQueue(customers, services, nil, budgets, nil, 1, today)
	if len(items) != 2 || overdue != 0 || week != 1 || noHistory != 1 || items[0].Appliance != "LG" && items[1].Appliance != "LG" {
		t.Fatalf("items=%+v counts overdue=%d week=%d noHistory=%d", items, overdue, week, noHistory)
	}
}

func stringPtrDashboard(value string) *string { return &value }

func TestDashboardCivilTodayUsesSaoPauloDate(t *testing.T) {
	utc := time.Date(2026, 10, 4, 1, 30, 0, 0, time.UTC)
	got := dashboardCivilToday(utc)
	if got.Format("2006-01-02") != "2026-10-03" {
		t.Fatalf("civil today=%s", got.Format("2006-01-02"))
	}
}

func TestDashboardCardsNavigateToMatchingTeamViews(t *testing.T) {
	cases := []struct {
		label, anchor, section, filter string
	}{
		{"Clientes", "team-customers", "", ""},
		{"Aparelhos", "team-customers", "", ""},
		{"Chamados abertos", "team-contact-center", teamQueueRequests, ""},
		{"Ordens de serviço", "team-contact-center", teamQueueHistory, ""},
		{"Concluídos", "team-contact-center", teamQueueHistory, ""},
		{"Cancelados", "team-contact-center", teamQueueHistory, ""},
		{"Agendamentos", "team-calendar", "calendar", "AGENDADO"},
		{"Em andamento", "team-calendar", "calendar", "EM_ANDAMENTO"},
		{"Retornos atrasados", "team-contact-center", "returns", string(domain.ReturnOverdue)},
		{"Vencendo esta semana", "team-contact-center", "returns", string(domain.ReturnThisWeek)},
		{"Sem histórico", "team-contact-center", "returns", string(domain.ReturnNoHistory)},
		{"OS atrasadas", "team-agenda-overdue", "", ""},
		{"Orçamentos", "team-budgets", "", ""},
		{"Faturamento", "team-finance", "", ""},
	}
	for _, tc := range cases {
		got := dashboardNavigationFor(tc.label)
		if got.Anchor != tc.anchor || got.Section != tc.section || got.Filter != tc.filter {
			t.Errorf("%q navigation=%+v, want anchor=%q section=%q filter=%q", tc.label, got, tc.anchor, tc.section, tc.filter)
		}
	}
}

func TestReturnHistoryIncludesCompletedLegacyLifecycleFields(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = w.Write([]byte(`{"id":"staff-1"}`))
		case "/rest/v1/profiles":
			_, _ = w.Write([]byte(`[{"id":"staff-1","tipo":"ADMIN"}]`))
		case "/rest/v1/services":
			if r.Header.Get("Authorization") != "Bearer staff-token" || r.URL.Query().Get("status") != "eq.CONCLUIDO" || !strings.Contains(r.URL.Query().Get("select"), "observacoes") {
				t.Errorf("history request auth=%q query=%s", r.Header.Get("Authorization"), r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`[{"id":"s1","cliente_id":"c1","aparelho_id":"a1","data_conclusao":"2026-04-01","observacoes":"[PROXIMO_RETORNO:2026-10-04]"}]`))
		case "/rest/v1/service_history":
			calls++
			selectFields := r.URL.Query().Get("select")
			if r.Header.Get("Authorization") != "Bearer staff-token" || !strings.Contains(selectFields, "id,service_id,cliente_id,aparelho_id,data,descricao") || !strings.Contains(selectFields, "problema,diagnostico,solucao,pecas_utilizadas,observacoes,valor") {
				t.Errorf("standalone history request auth=%q query=%s", r.Header.Get("Authorization"), r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`[{"id":"h1","aparelho_id":"a1","data":"2026-06-01","descricao":"Histórico manual"}]`))
		default:
			t.Errorf("unexpected request %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "service", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/historico?origem=retornos", nil)
	request.Header.Set("Authorization", "Bearer staff-token")
	response := httptest.NewRecorder()
	(httpapi.ServiceHistoryHandler{Supabase: client}).ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "PROXIMO_RETORNO") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/historico?origem=retornos-avulsos", nil)
	request.Header.Set("Authorization", "Bearer staff-token")
	response = httptest.NewRecorder()
	(httpapi.ServiceHistoryHandler{Supabase: client}).ServeHTTP(response, request)
	if response.Code != http.StatusOK || calls != 1 || !strings.Contains(response.Body.String(), "Histórico manual") {
		t.Fatalf("standalone status=%d calls=%d body=%s", response.Code, calls, response.Body.String())
	}
}
