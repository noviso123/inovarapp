package webapp

import (
	"reflect"
	"testing"
)

func TestTeamApplianceEditPayloadMatchesReactPersistedFields(t *testing.T) {
	create := map[string]any{
		"cliente_id": "client-1", "marca": "LG", "modelo": "Dual Inverter", "btus": 12000,
		"tipo": "Inverter", "ambiente": "Sala", "gas_tipo": "R-32", "tensao": "Bivolt", "numero_serie": "serial-1",
		"local_instalacao": "Cobertura", "data_instalacao": "2024-01-02", "observacoes": "metadata",
	}
	got := teamApplianceEditPayload(create, "appliance-1")
	want := map[string]any{"id": "appliance-1", "fields": map[string]any{
		"marca": "LG", "modelo": "Dual Inverter", "btus": 12000, "tipo": "Inverter", "ambiente": "Sala", "gas_tipo": "R-32", "tensao": "Bivolt",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("edit payload=%#v, want %#v", got, want)
	}
}

func TestTeamApplianceDialogOptionsMatchReact(t *testing.T) {
	wantBrands := []string{"LG", "Gree", "Midea", "Samsung", "Daikin", "Carrier", "Elgin", "Fujitsu", "Consul", "Electrolux", "Springer", "TCL", "Outra"}
	if got := teamApplianceBrands(); !reflect.DeepEqual(got, wantBrands) {
		t.Fatalf("brands=%v, want %v", got, wantBrands)
	}
	wantCapacities := []string{"7000", "9000", "12000", "18000", "24000", "30000", "36000", "48000", "60000"}
	if got := teamApplianceCapacities(); !reflect.DeepEqual(got, wantCapacities) {
		t.Fatalf("capacities=%v, want %v", got, wantCapacities)
	}
	wantEditTypes := []string{"Split Hi-Wall", "Inverter", "Cassete", "Piso Teto", "Janela", "Multi Split", "Portátil"}
	if got := teamApplianceEditTypes(); !reflect.DeepEqual(got, wantEditTypes) {
		t.Fatalf("edit types=%v, want %v", got, wantEditTypes)
	}
	wantCreateTypes := []string{"Split Hi-Wall", "Inverter", "Cassete", "Piso Teto", "Janela"}
	if got := teamApplianceCreateTypes(); !reflect.DeepEqual(got, wantCreateTypes) {
		t.Fatalf("create types=%v, want %v", got, wantCreateTypes)
	}
	if got := teamApplianceCapacityLabel("12000"); got != "12.000" {
		t.Fatalf("capacity label=%q, want 12.000", got)
	}
}

func TestTeamApplianceCreatePayloadIncludesLegacyMaintenanceDate(t *testing.T) {
	form := teamApplianceForm{ClientID: "client-1", Brand: " LG ", Model: " Dual Inverter ", Type: "Inverter", Room: "Sala", GasType: "R-32", Voltage: "Bivolt"}
	got := teamAppliancePayload(form, 12000, "2026-10-04")
	want := map[string]any{
		"cliente_id": "client-1", "marca": "LG", "modelo": "Dual Inverter", "btus": 12000,
		"tipo": "Inverter", "ambiente": "Sala", "gas_tipo": "R-32", "tensao": "Bivolt", "numero_serie": "", "local_instalacao": "",
		"data_instalacao": "", "observacoes": "", "ultima_manutencao": "2026-10-04",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("create payload=%#v, want %#v", got, want)
	}
}

func TestTeamApplianceDialogTitleIncludesSelectedCustomer(t *testing.T) {
	customers := []map[string]any{{"id": "client-1", "nome": "Maria Silva"}}
	if got := teamApplianceDialogTitle(teamApplianceForm{ClientID: "client-1"}, customers); got != "Adicionar Aparelho para Maria Silva" {
		t.Fatalf("create title=%q", got)
	}
	if got := teamApplianceDialogTitle(teamApplianceForm{ClientID: "client-1", ID: "appliance-1"}, customers); got != "Editar aparelho" {
		t.Fatalf("edit title=%q", got)
	}
}
