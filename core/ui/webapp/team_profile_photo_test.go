package webapp

import (
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/domain"
)

func TestTeamAccountControlRendersProfilePhotoActionsForStaff(t *testing.T) {
	p := &serviceCatalogPage{
		caller:              &supabase.Caller{Role: domain.RoleTechnician, UserID: "tech-1"},
		session:             &supabase.AuthSession{AccessToken: "token", User: &supabase.AuthUser{ID: "tech-1", Email: "tech@example.com"}},
		teamProfilePhotoURL: "https://storage.example/photo.jpg?v=2",
	}
	markup := app.HTMLString(p.accountControl())
	for _, text := range []string{"Trocar minha foto de perfil", "Foto de perfil", "Remover foto", "tech@example.com", "Sair"} {
		if !strings.Contains(markup, text) {
			t.Errorf("staff account control missing %q: %s", text, markup)
		}
	}
}

func TestTeamAccountControlDoesNotOfferStaffPhotoToCustomer(t *testing.T) {
	p := &serviceCatalogPage{caller: &supabase.Caller{Role: domain.RoleCustomer, UserID: "client-1"}, session: &supabase.AuthSession{AccessToken: "token"}}
	markup := app.HTMLString(p.accountControl())
	if strings.Contains(markup, "Trocar minha foto") || strings.Contains(markup, "Remover foto") {
		t.Fatalf("customer account unexpectedly got staff photo actions: %s", markup)
	}
}
