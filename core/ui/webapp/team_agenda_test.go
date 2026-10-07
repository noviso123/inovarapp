package webapp

import (
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

func TestBuildTeamAgendaGroupsSortsAndLimitsItems(t *testing.T) {
	services := []map[string]any{
		{"id": "late", "status": "AGENDADO", "data_agendamento": "2026-10-02", "tipo": "LIMPEZA"},
		{"id": "future-late", "status": "EM_ANDAMENTO", "data_agendamento": "2026-10-08", "tipo": "OUTRO", "descricao": "Reparo", "customers": map[string]any{"nome": "Ana", "whatsapp": "11999", "endereco": "Rua A", "bairro": "Centro"}, "air_conditioners": map[string]any{"marca": "Ar", "modelo": "X", "ambiente": "Sala"}},
		{"id": "done-old", "status": "CONCLUIDO", "data_agendamento": "2026-09-01", "data_conclusao": "2026-09-01"},
		{"id": "done-new", "status": "CONCLUIDO", "data_agendamento": "2026-10-01", "data_conclusao": "2026-10-01"},
		{"id": "cancelled", "status": "CANCELADO", "data_agendamento": "2026-10-02"},
		{"id": "pending", "status": "PENDENTE", "data_agendamento": "2026-10-02"},
	}
	groups := buildTeamAgenda(services, "2026-10-03")
	if len(groups.Overdue) != 1 || groups.Overdue[0].ID != "late" {
		t.Fatalf("overdue=%#v", groups.Overdue)
	}
	if len(groups.Upcoming) != 1 || groups.Upcoming[0].ID != "future-late" || groups.Upcoming[0].Date != "2026-10-08" {
		t.Fatalf("upcoming=%#v", groups.Upcoming)
	}
	item := groups.Upcoming[0]
	if item.Service != "Reparo" || item.Customer != "Ana" || item.Address != "Rua A • Centro" || item.Appliance != "Ar X • Sala" {
		t.Fatalf("agenda details=%#v", item)
	}
	if len(groups.RecentlyCompleted) != 2 || groups.RecentlyCompleted[0].ID != "done-new" {
		t.Fatalf("completed=%#v", groups.RecentlyCompleted)
	}
}

func TestBuildTeamAgendaCapsCompletedHistoryAtEight(t *testing.T) {
	services := make([]map[string]any, 10)
	for i := range services {
		services[i] = map[string]any{"id": string(rune('a' + i)), "status": "CONCLUIDO", "data_conclusao": "2026-10-" + string(rune('0'+i%9+1))}
	}
	groups := buildTeamAgenda(services, "2026-10-03")
	if len(groups.RecentlyCompleted) != 8 {
		t.Fatalf("completed items=%d, want 8", len(groups.RecentlyCompleted))
	}
}

func TestBuildTeamAgendaRecentCompletedUsesScheduledDateLikeReact(t *testing.T) {
	services := []map[string]any{
		{"id": "scheduled-latest", "status": "CONCLUIDO", "data_agendamento": "2026-10-09", "data_conclusao": "2026-10-01"},
		{"id": "completed-latest", "status": "CONCLUIDO", "data_agendamento": "2026-10-02", "data_conclusao": "2026-10-10"},
	}
	groups := buildTeamAgenda(services, "2026-10-11")
	if len(groups.RecentlyCompleted) != 2 || groups.RecentlyCompleted[0].ID != "completed-latest" || groups.RecentlyCompleted[1].ID != "scheduled-latest" {
		t.Fatalf("recent completed order=%v, want last eight from the date-sorted agenda reversed", groups.RecentlyCompleted)
	}
}

func TestSplitTeamAgendaMatchesReactFilters(t *testing.T) {
	items := []teamAgendaItem{
		{Status: "AGENDADO", Date: "2026-10-05"},
		{Status: "EM_ANDAMENTO", Date: "2026-10-06"},
		{Status: "CONCLUIDO", Date: "2026-10-07"},
		{Status: "AGENDADO", Date: "2026-10-04"},
	}
	upcoming, overdue, completed := splitTeamAgenda(items, "2026-10-05")
	if len(upcoming) != 2 || len(overdue) != 1 || len(completed) != 1 {
		t.Fatalf("split counts upcoming=%d overdue=%d completed=%d", len(upcoming), len(overdue), len(completed))
	}
}

func TestAgendaFinalizeActionRequiresServiceAlreadyInProgress(t *testing.T) {
	if canFinishTeamAgenda("AGENDADO") || canFinishTeamAgenda("CONCLUIDO") || !canFinishTeamAgenda("EM_ANDAMENTO") {
		t.Fatal("agenda completion availability differs from React status flow")
	}
	page := &serviceCatalogPage{}
	for status, wantFinalize := range map[string]bool{"AGENDADO": false, "EM_ANDAMENTO": true} {
		markup := app.HTMLString(page.teamAgendaCard(teamAgendaItem{ID: "s1", Status: status, Source: map[string]any{"id": "s1", "status": status}}))
		if strings.Contains(markup, ">Finalizar<") != wantFinalize {
			t.Errorf("status %s finalization button presence=%v want %v: %s", status, strings.Contains(markup, ">Finalizar<"), wantFinalize, markup)
		}
	}
}

func TestAgendaSectionsMatchLegacyEmptyStates(t *testing.T) {
	markup := app.HTMLString(app.Div().Body((&serviceCatalogPage{}).teamAgendaSections(teamAgendaGroups{})...))
	if !strings.Contains(markup, "Próximos agendamentos") || !strings.Contains(markup, "Nenhum agendamento futuro. Use &#34;Agendar Retorno&#34;") {
		t.Fatalf("empty upcoming section missing: %s", markup)
	}
	if strings.Contains(markup, "Atrasados (pendente de execução)") || strings.Contains(markup, "Concluídos recentemente") {
		t.Fatalf("empty overdue/completed sections should be hidden like React: %s", markup)
	}
}

func TestTeamOperationsShowsSpecificRecoverableDatabaseLoadError(t *testing.T) {
	page := &serviceCatalogPage{teamError: "A API ou o banco recusou a consulta de clientes e aparelhos. Nenhum cadastro foi apagado. Confira sua sessão e as permissões da integração."}
	markup := app.HTMLString(page.teamOperationsSection())
	for _, expected := range []string{"consulta de clientes e aparelhos", "Nenhum cadastro foi apagado", "Tentar carregar novamente", `role="alert"`} {
		if !strings.Contains(markup, expected) {
			t.Errorf("recoverable data error missing %q: %s", expected, markup)
		}
	}
}

func TestTeamAgendaSummaryAndCardPreserveReactDetails(t *testing.T) {
	page := &serviceCatalogPage{teamServices: []map[string]any{
		{"id": "upcoming", "status": "AGENDADO", "data_agendamento": "2026-10-07"},
		{"id": "done", "status": "CONCLUIDO", "data_agendamento": "2026-10-05"},
	}, teamActiveSection: "calendar"}
	markup := app.HTMLString(page.teamOperationsSection())
	if !strings.Contains(markup, "Agenda de Atendimentos") || !strings.Contains(markup, "1 próximos") {
		t.Fatalf("agenda heading/count missing: %s", markup)
	}
	card := app.HTMLString(page.teamAgendaCard(teamAgendaItem{Status: "AGENDADO", Date: "2026-10-05"}))
	if !strings.Contains(card, ">—<") {
		t.Fatalf("zero-value card placeholder missing: %s", card)
	}
}
