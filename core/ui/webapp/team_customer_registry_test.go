package webapp

import (
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/adapter/supabase"
)

func TestTeamCustomerRegistryStartsCollapsedAndShowsFullDetailsWhenExpanded(t *testing.T) {
	page := &serviceCatalogPage{
		caller: &supabase.Caller{Role: "TECNICO"},
		teamCustomers: []map[string]any{{
			"id": "client-1", "nome": "Maria Alves", "whatsapp": "27999990000", "observacoes": "Ligar antes da visita.",
			"appliances": []any{map[string]any{"id": "appliance-1", "marca": "LG", "btus": float64(12000), "ambiente": "Sala", "tipo": "Inverter", "gas_tipo": "R-32", "tensao": "Bivolt"}},
		}},
	}
	collapsed := app.HTMLString(page.teamCustomerRegistry())
	if !strings.Contains(collapsed, `aria-expanded="false"`) || strings.Contains(collapsed, "Ordem de Serviço") {
		t.Fatalf("customer card should start collapsed: %s", collapsed)
	}
	page.teamCustomerExpandedID = "client-1"
	expanded := app.HTMLString(page.teamCustomerRegistry())
	for _, expected := range []string{`aria-expanded`, "Ligar antes da visita.", "Ordem de Serviço", "Inverter • R-32 • Bivolt", "12.000 BTUs", "Sala"} {
		if !strings.Contains(expanded, expected) {
			t.Errorf("expanded customer card missing %q: %s", expected, expanded)
		}
	}
}

func TestNewCustomerApplianceDefaultsAreSelectedInForm(t *testing.T) {
	page := &serviceCatalogPage{teamCustomerForm: newTeamCustomerForm()}
	markup := app.HTMLString(page.teamCustomerCreateForm())
	for _, value := range []string{"LG", "12000", "Split Hi-Wall"} {
		selectStart := strings.Index(markup, `<select value="`+value+`">`)
		if selectStart < 0 {
			t.Errorf("new customer select for %q missing: %s", value, markup)
			continue
		}
		selectEnd := strings.Index(markup[selectStart:], `</select>`)
		if selectEnd < 0 {
			t.Errorf("new customer select for %q is not closed: %s", value, markup)
			continue
		}
		selectedOption := markup[selectStart : selectStart+selectEnd]
		if !strings.Contains(selectedOption, `value="`+value+`"`) || !strings.Contains(selectedOption, `selected`) {
			t.Errorf("new customer default option %q is not selected: %s", value, selectedOption)
		}
	}
	if strings.Contains(markup, `selected="false"`) {
		t.Fatalf("nonselected options must not emit a selected boolean attribute: %s", markup)
	}
}

func TestTeamCustomerSearchMatchesProfileNotesAndApplianceSerial(t *testing.T) {
	page := &serviceCatalogPage{teamCustomerSearch: "serial-77", teamCustomers: []map[string]any{{
		"id": "client-1", "nome": "Maria", "observacoes": "Preferência de contato",
		"appliances": []any{map[string]any{"id": "appliance-1", "marca": "LG", "numero_serie": "SERIAL-77"}},
	}}}
	markup := app.HTMLString(page.teamCustomerRegistry())
	if !strings.Contains(markup, "Maria") || strings.Contains(markup, "Nenhum cliente encontrado") {
		t.Fatalf("search by appliance serial failed: %s", markup)
	}
	page.teamCustomerSearch = "not found"
	markup = app.HTMLString(page.teamCustomerRegistry())
	if !strings.Contains(markup, "Nenhum cliente encontrado para") || !strings.Contains(markup, "not found") {
		t.Fatalf("empty search state missing: %s", markup)
	}
}

func TestTeamCustomerTemporaryAccessDialogShowsCredentialsAndCopyAction(t *testing.T) {
	page := &serviceCatalogPage{teamCustomerTemporaryAccess: &teamCustomerTemporaryAccess{Email: "cliente@example.com", Password: "Temp123!"}}
	markup := app.HTMLString(page.teamCustomerTemporaryAccessDialog())
	for _, expected := range []string{"Senha temporária criada", "cliente@example.com", "Temp123!", "Copiar acesso", "Concluir"} {
		if !strings.Contains(markup, expected) {
			t.Errorf("temporary access dialog missing %q: %s", expected, markup)
		}
	}
}

func TestTeamApplianceDialogIsRenderedOutsideCustomersSection(t *testing.T) {
	page := &serviceCatalogPage{teamApplianceForm: &teamApplianceForm{
		ClientID: "client-1", Brand: "LG", Capacity: "12000", Type: "Split Hi-Wall", Room: "Sala",
	}}
	markup := app.HTMLString(page.Render())
	for _, expected := range []string{"Adicionar aparelho", "Capacidade", "Ambiente / cômodo", "Salvar aparelho"} {
		if !strings.Contains(markup, expected) {
			t.Errorf("global appliance dialog missing %q: %s", expected, markup)
		}
	}
}
