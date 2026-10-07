package webapp

import (
	"strings"
	"testing"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/domain"
)

func TestServiceOrderDataMapsRowsAndPreservesFalseChecklistValue(t *testing.T) {
	form := &teamCompletionForm{
		serviceType: string(domain.ServiceCleaning), payment: "PIX", partsUsed: "Filtro", notes: "Ruído na turbina",
		laborPrice: 200, partsPrice: 35,
		service: map[string]any{
			"id": "service-12345678", "cliente_id": "client-1", "aparelho_id": "appliance-1",
			"customers":        map[string]any{"nome": "João Silva", "whatsapp": "27999991234", "endereco": "Rua A", "bairro": "Centro", "cidade": "Vitória"},
			"air_conditioners": map[string]any{"marca": "Consul", "modelo": "Inverter", "btus": float64(12000), "ambiente": "Quarto"},
		},
	}
	form.checklist = map[string]any{"filtrosLavados": false, "saltoTermicoDeltaT": "9°C"}
	completion, err := domain.BuildServiceCompletion(form.serviceType, "PIX", form.checklist, form.laborPrice, form.partsPrice, 90, 6, time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	document, err := serviceOrderData(form, completion, domain.TechnicianProfile{BusinessName: "Inovar Refrigeração"})
	if err != nil {
		t.Fatal(err)
	}
	if document.Client.Name != "João Silva" || document.Client.City == nil || *document.Client.City != "Vitória" || document.Appliance.CapacityBTU != "12000" || document.Record.ID != "service-12345678" {
		t.Fatalf("mapped PDF data=%#v", document)
	}
	if document.Record.Checklist == nil || document.Record.Checklist.FiltersWashed == nil || *document.Record.Checklist.FiltersWashed || document.Record.Price != 235 {
		t.Fatalf("checklist or total lost: %#v", document.Record)
	}
}

func TestTeamCompletionChecklistCarriesFreeformPartsIntoPersistentChecklist(t *testing.T) {
	form := &teamCompletionForm{
		partsUsed: "Filtro antibacteriano e sensor",
		checklist: map[string]any{"filtrosLavados": false, "pecasSubstituidas": "Peça cadastrada no protocolo"},
	}
	checklist := teamCompletionChecklist(form)
	if checklist["pecasSubstituidas"] != "Filtro antibacteriano e sensor" {
		t.Fatalf("freeform parts were not copied to checklist: %#v", checklist)
	}
	if checklist["filtrosLavados"] != false || form.checklist["pecasSubstituidas"] != "Peça cadastrada no protocolo" {
		t.Fatalf("completion checklist mutated or lost existing values: got=%#v form=%#v", checklist, form.checklist)
	}
}

func TestServiceOrderMessageUsesSavedTemplateAndRemovesUnknownPlaceholders(t *testing.T) {
	profile := domain.TechnicianProfile{BusinessName: "Inovar Refrigeração", WhatsAppMessages: map[string]string{"os_concluida": "{{cliente}} {{os}} {{servico}} {{data}} {{garantia}} {{empresa}} {{naoExiste}}"}}
	form := &teamCompletionForm{serviceType: "Limpeza de Ar", service: map[string]any{"id": "abc12345"}}
	completion := domain.ServiceCompletion{Date: "2026-10-03", WarrantyDays: 90}
	message := serviceOrderMessage(profile, form, completion, map[string]any{"nome": "João Silva"})
	if !strings.Contains(message, "João OS#ABC123 Limpeza de Ar 03/10/2026 90 dias Inovar Refrigeração") {
		t.Fatalf("message=%q", message)
	}
	if strings.Contains(message, "naoExiste") || strings.Contains(message, "{{") {
		t.Fatalf("unresolved placeholder in %q", message)
	}
}

func TestTeamCompletionWarrantyMatchesSelectedCatalogService(t *testing.T) {
	profile := domain.TechnicianProfile{
		DefaultWarrantyDays:     45,
		CustomServiceTypes:      []domain.CustomServiceType{{Name: "Higienização Premium", DefaultWarranty: "120 dias"}},
		EditedFixedServiceTypes: []domain.EditedFixedServiceType{{Type: string(domain.ServiceCleaning), DadosTipoServico: domain.DadosTipoServico{Name: "Limpeza Comercial", DefaultWarranty: "180 dias"}}},
	}
	for _, test := range []struct {
		serviceType string
		want        int
	}{
		{string(domain.ServiceInstallation), 180}, // first value in "180 a 365 dias", same as the React form
		{string(domain.ServiceCleaning), 180},
		{"Higienização Premium", 120},
		{"Tipo sem garantia numérica", 45},
	} {
		if got := teamCompletionWarrantyDays(test.serviceType, profile); got != test.want {
			t.Errorf("warranty for %q = %d, want %d", test.serviceType, got, test.want)
		}
	}
	form := newTeamCompletionForm(map[string]any{"tipo": "INSTALACAO"}, profile)
	if form.warrantyDays != 180 {
		t.Fatalf("initial form warranty = %d, want catalog default 180", form.warrantyDays)
	}
}

func TestRefrigerantCompletionShowsReadOnlyApplianceGasType(t *testing.T) {
	page := &serviceCatalogPage{teamCompletion: newTeamCompletionForm(map[string]any{
		"id": "service-1", "tipo": "RECARGA_GAS",
		"air_conditioners": map[string]any{"gas_tipo": "R-32"},
	}, domain.TechnicianProfile{})}
	markup := app.HTMLString(page.teamCompletionDialog())
	if !strings.Contains(markup, "Tipo de Gás") || !strings.Contains(markup, `value="R-32"`) || !strings.Contains(markup, "Edite o gás na Ficha do Aparelho do cliente") {
		t.Fatalf("refrigerant checklist should show the appliance gas as read-only: %s", markup)
	}
}
