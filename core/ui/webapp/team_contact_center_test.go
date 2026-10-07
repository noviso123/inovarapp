package webapp

import (
	"net/url"
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/domain"
)

func TestTeamRequestStatusPayloadUsesLifecycleDateAndPreservesObservations(t *testing.T) {
	tests := []struct {
		name      string
		status    string
		dateField string
		marker    string
		message   string
		notice    string
	}{
		{name: "start", status: "EM_ANDAMENTO", dateField: "data_inicio", marker: "DATA_INICIO", message: "status_em_andamento", notice: "Serviço iniciado."},
		{name: "complete", status: "CONCLUIDO", dateField: "data_conclusao", marker: "DATA_CONCLUSAO", message: "status_concluido", notice: "Serviço concluído."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			primary, fallback, message, notice, ok := teamRequestStatusPayload(tt.status, "Cliente pediu cuidado", "2026-10-04")
			if !ok || message != tt.message || notice != tt.notice {
				t.Fatalf("payload metadata = (%q, %q, %v), want (%q, %q, true)", message, notice, ok, tt.message, tt.notice)
			}
			if primary["status"] != tt.status || primary[tt.dateField] != "2026-10-04" {
				t.Errorf("primary payload = %#v", primary)
			}
			if fallback["status"] != tt.status {
				t.Errorf("fallback status = %#v", fallback)
			}
			observations := portalText(fallback["observacoes"])
			if !strings.Contains(observations, "Cliente pediu cuidado") || !strings.Contains(observations, "["+tt.marker+":2026-10-04]") {
				t.Errorf("fallback observations did not preserve note and lifecycle marker: %q", observations)
			}
		})
	}
}

func TestTeamRequestStatusPayloadRejectsUnsupportedStatus(t *testing.T) {
	primary, fallback, message, notice, ok := teamRequestStatusPayload("CANCELADO", "observação", "2026-10-04")
	if ok || primary != nil || fallback != nil || message != "" || notice != "" {
		t.Fatalf("unsupported status returned (%#v, %#v, %q, %q, %v)", primary, fallback, message, notice, ok)
	}
}

func TestTeamLegacyStatusMessageMatchesReactServiceTypeFormatting(t *testing.T) {
	profile := domain.TechnicianProfile{BusinessName: "Oficina", WhatsAppMessages: map[string]string{
		"status_cancelado": "Oi {{cliente}}: {{servico}} ({{empresa}})",
	}}
	service := map[string]any{"tipo": "MANUTENCAO_CORRETIVA", "descricao": "Reparo personalizado"}
	client := map[string]any{"nome": "Ana Silva"}
	got := teamLegacyStatusMessage(profile, service, client, "status_cancelado")
	if want := "Oi Ana: MANUTENCAO CORRETIVA (Oficina)"; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestFilterTeamContactReturnsByStatusAndSearch(t *testing.T) {
	items := []teamReturnItem{
		{Customer: "Ana Silva", Phone: "27999990000", Appliance: "LG Split", Room: "Sala", Status: string(domain.ReturnOverdue)},
		{Customer: "Bruno Costa", Appliance: "Midea Inverter", Room: "Quarto", Status: string(domain.ReturnSoon)},
		{Customer: "Caio Souza", Appliance: "Samsung", Status: string(domain.ReturnNoHistory)},
		{Customer: "Dora Lima", Appliance: "LG", Status: string(domain.ReturnNoHistory), InProgressServiceIDs: []string{"running"}},
	}
	if got := filterTeamContactReturns(items, "todos", ""); len(got) != 3 {
		t.Fatalf("all filter returned %d items", len(got))
	}
	if got := filterTeamContactReturns(items, string(domain.ReturnOverdue), ""); len(got) != 1 || got[0].Customer != "Ana Silva" {
		t.Fatalf("overdue filter returned %#v", got)
	}
	if got := filterTeamContactReturns(items, "todos", "2799999"); len(got) != 1 || got[0].Customer != "Ana Silva" {
		t.Fatalf("phone search returned %#v", got)
	}
	if got := filterTeamContactReturns(items, "todos", "quarto"); len(got) != 1 || got[0].Customer != "Bruno Costa" {
		t.Fatalf("room search returned %#v", got)
	}
	if got := filterTeamContactReturns(items, string(domain.ReturnNoHistory), ""); len(got) != 2 || got[0].Customer != "Caio Souza" {
		t.Fatalf("explicit no-history filter returned %#v", got)
	}
}

func TestReturnQueueQuickActionsIncludeLinkedRequestExecutionAndApprovedBudget(t *testing.T) {
	appliance := map[string]any{"id": "appliance-1", "cliente_id": "client-1", "marca": "LG"}
	client := map[string]any{"id": "client-1", "appliances": []any{appliance}}
	budgetApplianceID := "appliance-1"
	p := &serviceCatalogPage{
		teamCustomers: []map[string]any{client},
		teamServices: []map[string]any{
			{"id": "pending", "status": "PENDENTE"},
			{"id": "running", "status": "EM_ANDAMENTO"},
		},
		teamBudgets: []domain.BudgetEstimate{{ID: "approved", Status: domain.BudgetApproved, ApplianceID: &budgetApplianceID}},
	}
	item := teamReturnItem{CustomerID: "client-1", ApplianceID: "appliance-1", PendingServiceIDs: []string{"pending"}, InProgressServiceIDs: []string{"running"}, BudgetID: "approved"}
	markup := app.HTMLString(app.Div().Body(p.teamReturnQuickActions(item)...))
	for _, want := range []string{"Atender chamado", "Finalizar serviço", "Cancelar OS", "Agendar serviço"} {
		if !strings.Contains(markup, want) {
			t.Errorf("contextual action missing %q: %s", want, markup)
		}
	}
}

func TestReturnWhatsAppURLKeepsPortugueseMessageAndPhoneNormalization(t *testing.T) {
	item := teamReturnItem{Customer: "Ana Silva", Phone: "(27) 99999-0000", Type: "Split Hi-Wall", Capacity: "12000", Room: "Sala", Status: string(domain.ReturnNoHistory)}
	address, err := url.Parse(returnWhatsAppURL(item, domain.TechnicianProfile{Name: "Gabriel"}))
	if err != nil {
		t.Fatal(err)
	}
	if address.Query().Get("phone") != "5527999990000" {
		t.Fatalf("unexpected normalized phone %q", address.Query().Get("phone"))
	}
	message := address.Query().Get("text")
	for _, phrase := range []string{"Olá, *Ana*!", "Inovar Refrigeração", "12000 BTUs", "revisão ou limpeza de ar preventiva", "❄️🔧"} {
		if !strings.Contains(message, phrase) {
			t.Errorf("message does not contain %q: %s", phrase, message)
		}
	}
}

func TestFilterTeamRequestServicesMatchesLegacyFiltersAndPriority(t *testing.T) {
	services := []map[string]any{
		{"id": "completed", "status": "CONCLUIDO", "data_agendamento": "2026-09-01", "descricao": "Limpeza"},
		{"id": "future", "status": "AGENDADO", "data_agendamento": "2026-10-05", "descricao": "Instalação"},
		{"id": "today-pending", "status": "PENDENTE", "data_agendamento": "2026-10-03", "descricao": "Carga gás"},
		{"id": "cancelled", "status": "CANCELADO", "data_agendamento": "2026-10-02"},
		{"id": "overdue", "status": "AGENDADO", "data_agendamento": "2026-10-02", "descricao": "Reparo"},
		{"id": "overdue-pending", "status": "PENDENTE", "data_agendamento": "2026-10-01"},
		{"id": "in-progress", "status": "EM_ANDAMENTO", "data_agendamento": "2026-10-03"},
		{"id": "undated", "status": "PENDENTE", "descricao": "Orçamento"},
	}
	all := filterTeamRequestServices(services, "TODOS", "", "2026-10-03")
	wantIDs := []string{"overdue-pending", "overdue", "today-pending", "in-progress", "future", "undated"}
	if len(all) != len(wantIDs) {
		t.Fatalf("all services=%d: %#v", len(all), all)
	}
	for i, want := range wantIDs {
		if got := portalText(all[i]["id"]); got != want {
			t.Errorf("sorted[%d]=%q want %q", i, got, want)
		}
	}
	if got := filterTeamRequestServices(services, "AGENDADO", "", "2026-10-03"); len(got) != 2 {
		t.Fatalf("scheduled filter=%#v", got)
	}
	if got := filterTeamRequestServices(services, "PENDENTE", "gás", "2026-10-03"); len(got) != 1 || portalText(got[0]["id"]) != "today-pending" {
		t.Fatalf("pending search=%#v", got)
	}
	if got := filterTeamRequestServices(services, "CONCLUIDO", "", "2026-10-03"); len(got) != 1 || portalText(got[0]["id"]) != "completed" {
		t.Fatalf("explicit completed filter=%#v", got)
	}
	if got := countOverdueTeamRequests(services, "2026-10-03"); got != 2 {
		t.Fatalf("overdue count=%d want 2", got)
	}
}

func TestContactCenterChamadosAllFilterOmitsCompletedOrders(t *testing.T) {
	p := &serviceCatalogPage{
		teamQueueSection: teamQueueRequests,
		teamServices: []map[string]any{
			{"id": "done", "status": "CONCLUIDO", "descricao": "OS concluída", "customers": map[string]any{"nome": "Cliente concluído"}},
			{"id": "pending", "status": "PENDENTE", "descricao": "Chamado aberto", "customers": map[string]any{"nome": "Cliente pendente"}},
		},
	}
	markup := app.HTMLString(p.teamContactCenter())
	if p.teamRequestFilter != "TODOS" {
		t.Fatalf("initial called-request filter = %q, want TODOS", p.teamRequestFilter)
	}
	if !strings.Contains(markup, "Cliente pendente") {
		t.Fatalf("open called request missing from default view: %s", markup)
	}
	if strings.Contains(markup, "Cliente concluído") {
		t.Fatalf("completed order appeared with open called requests: %s", markup)
	}
}

func TestReturnQueueIncludesChamadosFilterAndPendingServiceActions(t *testing.T) {
	p := &serviceCatalogPage{
		teamQueueSection: teamQueueReturns,
		teamQueueFilter:  "chamados",
		teamServices: []map[string]any{
			{"id": "pending-1", "cliente_id": "client-1", "aparelho_id": "appliance-1", "status": "PENDENTE", "descricao": "Limpeza de Ar", "customers": map[string]any{"nome": "Ana Silva", "whatsapp": "27999990000"}, "air_conditioners": map[string]any{"id": "appliance-1", "marca": "LG", "modelo": "Dual", "btus": "12000", "ambiente": "Sala"}},
			{"id": "pending-2", "cliente_id": "client-1", "aparelho_id": "appliance-1", "status": "PENDENTE", "descricao": "Manutenção Preventiva", "customers": map[string]any{"nome": "Ana Silva", "whatsapp": "27999990000"}, "air_conditioners": map[string]any{"id": "appliance-1", "marca": "LG", "modelo": "Dual", "btus": "12000", "ambiente": "Sala"}},
			{"id": "pending-3", "cliente_id": "client-1", "aparelho_id": "appliance-2", "status": "PENDENTE", "descricao": "Instalação", "customers": map[string]any{"nome": "Ana Silva", "whatsapp": "27999990000"}, "air_conditioners": map[string]any{"id": "appliance-2", "marca": "LG", "modelo": "Inverter", "btus": "18000", "ambiente": "Quarto"}},
		},
		teamDashboard: teamDashboardSummary{ReturnEvents: []teamReturnItem{
			{CustomerID: "client-1", ApplianceID: "appliance-1", Customer: "Ana Silva", Appliance: "LG Dual", Capacity: "12000", Status: string(domain.ReturnNoHistory), PendingServiceIDs: []string{"pending-1", "pending-2"}},
			{CustomerID: "client-1", ApplianceID: "appliance-2", Customer: "Ana Silva", Appliance: "LG Inverter", Capacity: "18000", Status: string(domain.ReturnNoHistory), PendingServiceIDs: []string{"pending-3"}},
			{Customer: "Outra pessoa", Status: string(domain.ReturnOverdue)},
		}},
	}
	markup := app.HTMLString(p.teamContactCenter())
	for _, want := range []string{"Chamados (2)", "Ana Silva", "12.000 BTUs"} {
		if !strings.Contains(markup, want) {
			t.Errorf("called-device queue missing %q: %s", want, markup)
		}
	}
	if got := strings.Count(markup, "team-return-queue__select"); got != 2 {
		t.Fatalf("pending requests rendered in %d device cards, want 2: %s", got, markup)
	}
	if strings.Contains(markup, "Outra pessoa") {
		t.Fatal("Chamados filter rendered return-only items")
	}
}

func TestGroupTeamRequestServicesKeepsDifferentAppliancesSeparate(t *testing.T) {
	services := []map[string]any{
		{"id": "one", "cliente_id": "client", "aparelho_id": "a1", "customers": map[string]any{"nome": "Ana"}, "air_conditioners": map[string]any{"marca": "LG"}},
		{"id": "two", "cliente_id": "client", "aparelho_id": "a1", "customers": map[string]any{"nome": "Ana"}, "air_conditioners": map[string]any{"marca": "LG"}},
		{"id": "three", "cliente_id": "client", "aparelho_id": "a2", "customers": map[string]any{"nome": "Ana"}, "air_conditioners": map[string]any{"marca": "Samsung"}},
	}
	groups := groupTeamRequestServices(services)
	if len(groups) != 2 || len(groups[0].Services) != 2 || len(groups[1].Services) != 1 {
		t.Fatalf("request groups=%+v, want two appliances with request counts 2 and 1", groups)
	}
}

func TestReturnQueueOffersApplianceWorkflowActions(t *testing.T) {
	customer := map[string]any{
		"id":         "client-1",
		"appliances": []any{map[string]any{"id": "appliance-1", "cliente_id": "client-1", "marca": "LG"}},
	}
	p := &serviceCatalogPage{teamCustomers: []map[string]any{customer}}
	item := teamReturnItem{CustomerID: "client-1", ApplianceID: "appliance-1"}
	markup := app.HTMLString(app.Div().Body(p.teamReturnQuickActions(item)...))
	for _, want := range []string{"Ficha do Aparelho", "Agendar Retorno", "Ordem de Serviço", "Histórico", "Orçamento"} {
		if !strings.Contains(markup, want) {
			t.Errorf("return queue action missing %q: %s", want, markup)
		}
	}
	if actions := p.teamReturnQuickActions(teamReturnItem{CustomerID: "client-1"}); len(actions) != 0 {
		t.Fatalf("budget-only item without an appliance got appliance actions: %d", len(actions))
	}
}

func TestReturnQueueDrawerShowsLiveServiceHistoryAndLinkedBudget(t *testing.T) {
	appliance := map[string]any{"id": "appliance-1", "marca": "LG", "modelo": "Dual", "ambiente": "Sala"}
	services := []map[string]any{
		{"id": "pending", "cliente_id": "client-1", "aparelho_id": "appliance-1", "status": "PENDENTE", "descricao": "Reparo", "data_solicitacao": "2026-10-01", "customers": map[string]any{"nome": "Ana"}, "air_conditioners": appliance},
		{"id": "scheduled", "cliente_id": "client-1", "aparelho_id": "appliance-1", "status": "AGENDADO", "descricao": "Limpeza", "data_agendamento": "2026-10-10", "customers": map[string]any{"nome": "Ana"}, "air_conditioners": appliance},
		{"id": "running", "cliente_id": "client-1", "aparelho_id": "appliance-1", "status": "EM_ANDAMENTO", "descricao": "Instalação", "data_inicio": "2026-10-02", "customers": map[string]any{"nome": "Ana"}, "air_conditioners": appliance},
		{"id": "done", "cliente_id": "client-1", "aparelho_id": "appliance-1", "status": "CONCLUIDO", "descricao": "Higienização", "data_conclusao": "2026-09-01", "valor": float64(320), "customers": map[string]any{"nome": "Ana"}, "air_conditioners": appliance},
	}
	applianceID := "appliance-1"
	p := &serviceCatalogPage{
		teamServices:           services,
		teamHistoryApplianceID: "appliance-1",
		teamHistoryLegacyRows:  []map[string]any{{"aparelho_id": "appliance-1", "descricao": "Registro anterior", "data": "2025-01-01"}},
		teamBudgets:            []domain.BudgetEstimate{{ID: "budget-1", Number: "104", Status: domain.BudgetApproved, FinalValue: 850, ApplianceID: &applianceID, Items: []domain.BudgetItem{{Description: "Troca do compressor"}}}},
	}
	item := teamReturnItem{CustomerID: "client-1", ApplianceID: "appliance-1", BudgetID: "budget-1"}
	markup := app.HTMLString(p.teamReturnDrawer(item))
	for _, want := range []string{"Atendimento em tempo real", "Reparo", "Atender chamado", "Limpeza", "Instalação", "Finalizar", "Histórico do aparelho", "Histórico completo do aparelho", "Histórico anterior", "Registro anterior", "Higienização", "Orçamento aprovado", "Ver proposta", "Agendar serviço"} {
		if !strings.Contains(markup, want) {
			t.Errorf("expanded return drawer missing %q: %s", want, markup)
		}
	}
}

func TestReturnQueueHistoryActionExpandsTheMatchingDrawer(t *testing.T) {
	item := teamReturnItem{Customer: "Ana", Appliance: "LG • Sala", ApplianceID: "appliance-1", Status: string(domain.ReturnSoon)}
	p := &serviceCatalogPage{
		teamQueueSection: teamQueueReturns,
		teamDashboard:    teamDashboardSummary{ReturnEvents: []teamReturnItem{item}},
		teamServices: []map[string]any{{
			"id": "service-1", "cliente_id": "client-1", "aparelho_id": "appliance-1", "status": "CONCLUIDO",
			"descricao": "Limpeza", "customers": map[string]any{"nome": "Ana"},
		}},
		teamHistoryApplianceID: "appliance-1",
		teamHistoryLegacyRows:  []map[string]any{{"aparelho_id": "appliance-1", "descricao": "Histórico carregado"}},
	}
	p.teamQueueSelectedKey = teamReturnQueueKey(item)
	markup := app.HTMLString(p.teamContactCenter())
	for _, want := range []string{`class="team-return-drawer"`, "Histórico do aparelho", "Histórico carregado"} {
		if !strings.Contains(markup, want) {
			t.Errorf("history click did not expose matching return drawer content %q: %s", want, markup)
		}
	}
}

func TestReturnQueueFilterExposesExplicitSelectedState(t *testing.T) {
	page := &serviceCatalogPage{teamQueueSection: teamQueueReturns, teamQueueFilter: "todos"}
	markup := app.HTMLString(page.teamContactCenter())
	if !strings.Contains(markup, "team-return-filter--selected") || !strings.Contains(markup, `aria-pressed="True"`) || !strings.Contains(markup, `Todos (0)</button>`) {
		t.Fatalf("selected return filter must expose its active state: %s", markup)
	}
	if !strings.Contains(markup, `aria-pressed="false"`) || !strings.Contains(markup, `Chamados (0)</button>`) {
		t.Fatalf("unselected return filters must expose aria-pressed=false: %s", markup)
	}
	if !strings.Contains(markup, `aria-selected="True"`) || !strings.Contains(markup, `aria-selected="false"`) {
		t.Fatalf("selected and unselected tabs must expose explicit aria-selected values: %s", markup)
	}
}

func TestTeamRequestWhatsAppURLMatchesLegacyPortugueseTemplate(t *testing.T) {
	service := map[string]any{
		"descricao": "Manutenção_Corretiva",
		"customers": map[string]any{"nome": "Ana Silva", "whatsapp": "(27) 99999-0000"},
	}
	address, err := url.Parse(teamRequestWhatsAppURL(service))
	if err != nil {
		t.Fatal(err)
	}
	if address.Host != "wa.me" || address.Path != "/5527999990000" {
		t.Fatalf("WhatsApp destination=%s", address)
	}
	want := "Olá Ana Silva, aqui é da Inovar Refrigeração! Vi sua solicitação no app para o serviço de Manutenção Corretiva. Vamos confirmar o horário de atendimento?"
	if got := address.Query().Get("text"); got != want {
		t.Fatalf("WhatsApp text=%q want %q", got, want)
	}
}

func TestTruncateTeamObservationUTF16MatchesLegacySliceWithoutBreakingUTF8(t *testing.T) {
	if got := truncateTeamObservationUTF16("a😀b", 3); got != "a😀" {
		t.Fatalf("truncation split or miscounted UTF-16 units: %q", got)
	}
	if got := truncateTeamObservationUTF16("áβç", 2); got != "áβ" {
		t.Fatalf("UTF-8 text was not retained correctly: %q", got)
	}
}
