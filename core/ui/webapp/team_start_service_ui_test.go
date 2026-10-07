package webapp

import (
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/adapter/supabase"
)

func TestTeamStartServiceFormCanOpenFromCustomerAppliance(t *testing.T) {
	form := newTeamStartServiceFormForAppliance("client-1", "appliance-1")
	if form.ClientID != "client-1" || form.ApplianceID != "appliance-1" || form.Step != 3 || form.ServiceType != "Limpeza de Ar" {
		t.Fatalf("contextual start form=%+v", form)
	}
}

func TestTeamStartServiceFormFromRequestKeepsExactOpenOrder(t *testing.T) {
	service := map[string]any{
		"id": "service-2", "cliente_id": "client-1", "aparelho_id": "appliance-1",
		"status": "PENDENTE", "tipo": "LIMPEZA", "descricao": "Limpeza de Ar",
	}
	form := newTeamStartServiceFormForRequest(service)
	if form.ServiceID != "service-2" || form.ClientID != "client-1" || form.ApplianceID != "appliance-1" || form.Step != 3 || form.ServiceType != "Limpeza de Ar" {
		t.Fatalf("request start form lost service context: %+v", form)
	}
}

func TestCustomerApplianceCardOffersContextualServiceOrder(t *testing.T) {
	p := &serviceCatalogPage{
		caller:                 &supabase.Caller{Role: "TECNICO"},
		teamCustomerExpandedID: "client-1",
		teamCustomers:          []map[string]any{{"id": "client-1", "nome": "Maria", "appliances": []any{map[string]any{"id": "appliance-1", "marca": "LG", "btus": float64(12000), "ambiente": "Sala", "tipo": "Inverter", "gas_tipo": "R-32", "tensao": "Bivolt"}}}},
	}
	markup := app.HTMLString(p.teamCustomerRegistry())
	for _, want := range []string{"Ordem de Serviço", "Inverter • R-32 • Bivolt", "12.000 BTUs", "Sala"} {
		if !strings.Contains(markup, want) {
			t.Errorf("customer appliance card missing %q: %s", want, markup)
		}
	}
}

func TestFindOpenTeamServiceForSelectedAppliance(t *testing.T) {
	services := []map[string]any{
		{"id": "other-client", "cliente_id": "client-2", "aparelho_id": "appliance-1", "status": "PENDENTE"},
		{"id": "other-appliance", "cliente_id": "client-1", "aparelho_id": "appliance-2", "status": "AGENDADO"},
		{"id": "completed", "cliente_id": "client-1", "aparelho_id": "appliance-1", "status": "CONCLUIDO"},
		{"id": "open", "cliente_id": "client-1", "aparelho_id": "appliance-1", "status": "AGENDADO"},
	}
	service, multiple := findOpenTeamService(services, "client-1", "appliance-1")
	if multiple || portalText(service["id"]) != "open" {
		t.Fatalf("service=%v multiple=%v", service, multiple)
	}
	services = append(services, map[string]any{"id": "legacy", "cliente_id": "client-1", "status": "EM_ANDAMENTO"})
	service, multiple = findOpenTeamService(services, "client-1", "appliance-1")
	if service != nil || !multiple {
		t.Fatalf("ambiguous open services were not reported: service=%v multiple=%v", service, multiple)
	}
}
