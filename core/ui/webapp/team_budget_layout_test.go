package webapp

import (
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

func TestTeamBudgetPanelKeepsCreateActionVisibleInItsOwnSection(t *testing.T) {
	page := &serviceCatalogPage{}
	markup := app.HTMLString(page.detailedTeamBudgetsPanel())
	for _, expected := range []string{`class="team-budget-page-heading"`, `class="auth-submit team-budget-create-action"`, "＋ Novo orçamento"} {
		if !strings.Contains(markup, expected) {
			t.Errorf("budget panel missing %q: %s", expected, markup)
		}
	}
}
