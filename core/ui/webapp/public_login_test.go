package webapp

import (
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

func TestUnauthenticatedHomeShowsWelcomeAndEntryActionsWithoutPublicCatalog(t *testing.T) {
	markup := app.HTMLString((&serviceCatalogPage{}).Render())
	for _, want := range []string{"Bem-vindo ao InovarApp", "Entrar no App", "Sou Cliente • Criar Minha Conta"} {
		if !strings.Contains(markup, want) {
			t.Errorf("welcome screen missing %q: %s", want, markup)
		}
	}
	if strings.Contains(markup, `type="password"`) {
		t.Fatal("password field should appear only after opening the sign-in dialog")
	}
	for _, forbidden := range []string{"Serviços disponíveis", "service-card__price", "R$ 210 - 310", "R$ 900 - 1300"} {
		if strings.Contains(markup, forbidden) {
			t.Errorf("unauthenticated screen exposed public catalog content %q", forbidden)
		}
	}
}
