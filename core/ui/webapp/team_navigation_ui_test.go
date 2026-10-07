package webapp

import (
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

func TestTeamWorkspaceNavigationLinksEveryPrimaryModuleAndQuickAction(t *testing.T) {
	page := &serviceCatalogPage{teamDashboard: teamDashboardSummary{Customers: 4, TotalBudgets: 3, TotalServices: 8, ReturnsOverdue: 2}}
	markup := app.HTMLString(page.teamWorkspaceNavigation())
	for _, expected := range []string{
		`aria-label="Navegação do painel da equipe"`,
		`title="Abrir Painel"`, `title="Abrir Central de Atendimento"`, `title="Abrir Propostas"`,
		`title="Abrir Agenda"`, `title="Abrir Financeiro"`, `title="Abrir Serviços"`, `title="Abrir Clientes"`, `title="Abrir Mensagens WhatsApp"`,
		"Painel", "Central de Atendimento", "Propostas", "Agenda", "Financeiro", "Serviços", "Clientes", "Mensagens WhatsApp",
		"Novo Orçamento", "＋ Cliente",
		"team-workspace-nav__mobile-toggle", "team-workspace-nav__icon",
	} {
		if !strings.Contains(markup, expected) {
			t.Errorf("team navigation missing %q: %s", expected, markup)
		}
	}
	for _, removed := range []string{"+ Agendamento", "+ Proposta"} {
		if strings.Contains(markup, removed) {
			t.Errorf("navigation should not render removed redundant quick action %q: %s", removed, markup)
		}
	}
}

func TestTeamSectionCanBeOpenedFromWhatsAppQueueDeepLink(t *testing.T) {
	if got := teamSectionFromURLFragment("team-whatsapp-queue"); got != "whatsapp" {
		t.Fatalf("WhatsApp queue fragment selects %q, want whatsapp", got)
	}
}

func TestTeamWorkspaceMobileNavigationKeepsFourDestinationsAndMovesTheRestToMore(t *testing.T) {
	page := &serviceCatalogPage{}
	markup := app.HTMLString(page.teamWorkspaceNavigation())
	for _, expected := range []string{
		`class="team-workspace-nav__link team-workspace-nav__link--primary`,
		`class="team-workspace-nav__link team-workspace-nav__link--more`,
		`aria-label="Abrir mais seções"`,
		"team-workspace-nav__label--mobile",
		"Central",
	} {
		if !strings.Contains(markup, expected) {
			t.Errorf("mobile team navigation is missing %q: %s", expected, markup)
		}
	}
	page.teamNavOpen = true
	markup = app.HTMLString(page.teamWorkspaceNavigation())
	if !strings.Contains(markup, `class="team-workspace-nav team-workspace-nav--open"`) || !strings.Contains(markup, `aria-label="Fechar mais seções"`) {
		t.Fatalf("expanded mobile overflow menu state is not exposed: %s", markup)
	}
}

func TestDashboardRevenueAndMetricToneMatchPortugueseKPIFormat(t *testing.T) {
	if got, want := dashboardRevenueLabel(12345.6), "R$ 12.346"; got != want {
		t.Fatalf("revenue label=%q want %q", got, want)
	}
	summary := teamDashboardSummary{ReturnsOverdue: 1, ReturnsThisWeek: 2, PendingRequests: 1, NoHistory: 1}
	for label, want := range map[string]string{
		"Retornos atrasados":   "danger",
		"Vencendo esta semana": "warning",
		"Sem histórico":        "purple",
		"Chamados abertos":     "purple",
		"Faturamento":          "success",
	} {
		if got := dashboardMetricTone(label, summary); got != want {
			t.Errorf("metric tone for %q=%q want %q", label, got, want)
		}
	}
}
