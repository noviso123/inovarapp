package webapp

import (
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/domain"
)

func TestTeamHistoryInitialSelectionUsesFirstCatalogEntryAndPrice(t *testing.T) {
	profile := domain.TechnicianProfile{
		DefaultPrice: 250,
		EditedFixedServiceTypes: []domain.EditedFixedServiceType{{
			Type:             string(domain.ServiceCleaning),
			DadosTipoServico: domain.DadosTipoServico{Name: "Limpeza Comercial", Price: 385},
		}},
	}
	serviceType, price := teamHistoryInitialSelection(profile)
	if serviceType != "Limpeza Comercial" || price != 385 {
		t.Fatalf("initial selection=(%q, %v), want (Limpeza Comercial, 385)", serviceType, price)
	}
}

func TestTeamHistoryDisplaysLocalizedDatesAndReturnCycle(t *testing.T) {
	row := map[string]any{"status": "CONCLUIDO", "data_conclusao": "2026-01-31", "observacoes": "finalizado"}
	if got := teamHistoryReturnDate(row, 6); got != "2026-07-31" {
		t.Fatalf("calculated return date=%q", got)
	}
	row["observacoes"] = "[PROXIMO_RETORNO:2026-08-15]"
	if got := teamHistoryReturnDate(row, 6); got != "2026-08-15" {
		t.Fatalf("marked return date=%q", got)
	}
	row["status"] = "AGENDADO"
	if got := teamHistoryReturnDate(row, 6); got != "" {
		t.Fatalf("non-completed service return date=%q", got)
	}
	if got := formatTeamHistoryDate("2026-01-31T12:00:00Z"); got != "31/01/2026" {
		t.Fatalf("formatted date=%q", got)
	}
	p := &serviceCatalogPage{teamHistoryApplianceID: "device-1", teamProfile: domain.TechnicianProfile{DefaultReturnMonths: 6}, teamHistoryRows: []map[string]any{{"aparelho_id": "device-1", "descricao": "Limpeza de Ar", "status": "CONCLUIDO", "data_conclusao": "2026-01-31", "valor": float64(210)}}}
	markup := app.HTMLString(p.applianceHistoryPanel())
	for _, want := range []string{"31/01/2026", "Próximo retorno", "31/07/2026", "R$ 210,00"} {
		if !strings.Contains(markup, want) {
			t.Errorf("history panel missing %q: %s", want, markup)
		}
	}
}

func TestTeamHistoryTypeChangeUsesConfiguredPriceAndKeepsManualPriceWithoutCatalogPrice(t *testing.T) {
	profile := domain.TechnicianProfile{
		CustomServiceTypes: []domain.CustomServiceType{{Name: "Higienização Premium", Price: 520}},
	}
	form := &teamHistoryForm{Price: "300"}
	updateTeamHistoryServiceType(form, profile, "Higienização Premium")
	if form.Type != "Higienização Premium" || form.Price != "520" {
		t.Fatalf("selected type/price=(%q, %q), want custom catalog price", form.Type, form.Price)
	}
	updateTeamHistoryServiceType(form, profile, "Serviço sem preço")
	if form.Type != "Serviço sem preço" || form.Price != "520" {
		t.Fatalf("type without price should retain current price: form=%+v", form)
	}
}

func TestTeamHistoryInitialSelectionFallsBackWhenCatalogHasNoPrice(t *testing.T) {
	profile := domain.TechnicianProfile{DefaultPrice: 275, RemovedFixedServiceTypes: []string{
		string(domain.ServiceCleaning), string(domain.ServiceInstallation), string(domain.ServiceCorrective),
		string(domain.ServiceRefrigerant), string(domain.ServicePreventive), string(domain.ServiceTechnicalAssessment),
	}}
	serviceType, price := teamHistoryInitialSelection(profile)
	if serviceType != string(domain.ServiceCleaning) || price != 275 {
		t.Fatalf("fallback selection=(%q, %v), want cleaning and default price", serviceType, price)
	}
	serviceType, price = teamHistoryInitialSelection(domain.TechnicianProfile{})
	if serviceType != string(domain.ServiceCleaning) || price != 280 {
		t.Fatalf("empty-profile selection=(%q, %v), want first catalog item and suggested price 280", serviceType, price)
	}
}
