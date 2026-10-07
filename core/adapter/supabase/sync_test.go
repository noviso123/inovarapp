package supabase

import (
	"encoding/json"
	"testing"
	"time"

	"inovarapp/core/domain"
)

func TestMapSupabaseCustomerToLocalPreservesDefaultsAndUTF8(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 34, 56, 789000000, time.FixedZone("UTC-3", -3*60*60))
	btus := 9000.0
	client := MapSupabaseCustomerToLocal(SupabaseCustomer{
		ID: "client-1", Name: "João da Silva", WhatsApp: "27999999999", Appliances: []SupabaseAirConditioner{{
			ID: "appliance-1", Brand: "LG", BTU: &btus, GasType: "R-32", Voltage: "Bivolt", SerialNumber: "SN-77", InstallationSite: "Cobertura", InstallDate: "2025-08-10", Notes: "Dreno externo.",
		}},
	}, now)
	if client.Name != "João da Silva" || client.City == nil || *client.City != "Serra" || client.CreatedAt != "2026-10-03T15:34:56.789Z" {
		t.Fatalf("customer mapping/defaults changed: %#v", client)
	}
	if client.Document == nil || *client.Document != "" || client.Notes == nil || *client.Notes != "" || len(client.Appliances) != 1 {
		t.Fatalf("empty fields or nested appliances changed: %#v", client)
	}
	appliance := client.Appliances[0]
	if appliance.ClientID != client.ID || appliance.CapacityBTU != "9.000" || appliance.Room != "Sala de Estar" || appliance.Type != domain.ApplianceSplit || appliance.GasType == nil || *appliance.GasType != domain.Refrigerant32 || appliance.Voltage == nil || *appliance.Voltage != domain.VoltageBivolt || appliance.Model == nil || *appliance.Model != "" || appliance.SerialNumber != "SN-77" || appliance.InstallationSite != "Cobertura" || appliance.InstallDate == nil || *appliance.InstallDate != "2025-08-10" || appliance.Notes == nil || *appliance.Notes != "Dreno externo." {
		t.Fatalf("appliance mapping/defaults changed: %#v", appliance)
	}
	var encoded map[string]any
	data, err := json.Marshal(client)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &encoded); err != nil {
		t.Fatal(err)
	}
	if encoded["document"] != "" || encoded["address"] != "" || encoded["city"] != "Serra" {
		t.Fatalf("mapped empty values were omitted: %s", data)
	}
}

func TestMapServiceTypeToAndFromSupabase(t *testing.T) {
	cases := []struct {
		local    domain.ServiceType
		database string
	}{
		{domain.ServiceCleaning, "LIMPEZA"},
		{domain.ServiceInstallation, "INSTALACAO"},
		{domain.ServiceCorrective, "MANUTENCAO_CORRETIVA"},
		{domain.ServicePreventive, "MANUTENCAO_PREVENTIVA"},
		{domain.ServiceRefrigerant, "RECARGA_GAS"},
		{domain.ServiceTechnicalAssessment, "AVALIACAO"},
		{domain.ServiceOther, "OUTRO"},
	}
	for _, tc := range cases {
		if got := MapServiceTypeToSupabase(string(tc.local)); got != tc.database {
			t.Errorf("to Supabase %q = %q, want %q", tc.local, got, tc.database)
		}
		if got := MapSupabaseServiceTypeToLocal(tc.database); tc.local != domain.ServiceOther && got != tc.local {
			t.Errorf("from Supabase %q = %q, want %q", tc.database, got, tc.local)
		}
	}
	if got := MapServiceTypeToSupabase("Conserto / Carga Gás"); got != "MANUTENCAO_CORRETIVA" {
		t.Fatalf("legacy UI label mapping changed: %q", got)
	}
}

func TestMapSupabaseServiceToLocalRestoresMarkersAndLifecycle(t *testing.T) {
	now := time.Date(2026, 10, 3, 23, 0, 0, 0, time.FixedZone("UTC-3", -3*60*60))
	profile := domain.TechnicianProfile{DefaultReturnMonths: 6, DefaultWarrantyDays: 90}
	row := SupabaseServiceRow{
		ID: "service-1", ClientID: "client-1", ApplianceID: "appliance-1", Type: "LIMPEZA",
		Status: "CONCLUIDO", ScheduledDate: "2026-01-31", StartedDate: "2026-01-30T00:00:00Z",
		Value: 210, Notes: "Concluído. [DATA_INICIO:2026-01-29] [DATA_CONCLUSAO:2026-01-31] [GARANTIA_DIAS:0] [PROXIMO_RETORNO:2026-07-31] [MAO_OBRA:120.50] [PECAS:35] [CHECKLIST:{\"filtrosLavados\":false,\"correnteAmperes\":\"3.2\",\"pecasSubstituidas\":\"Filtro antibacteriano\"}]",
	}
	got := MapSupabaseServiceToLocal(row, profile, now)
	if got.Date != "2026-01-31" || got.ScheduledDate != "2026-01-31" || got.CompletionDate != "2026-01-31" || got.ReturnDate != "2026-07-31" {
		t.Fatalf("lifecycle dates changed: %#v", got)
	}
	if got.Status != domain.MaintenanceCompleted || got.ServiceType != domain.ServiceCleaning || got.Price != 210 || got.WarrantyDays != 0 || got.PaymentMethod != domain.PaymentPIX {
		t.Fatalf("status/rule mapping changed: %#v", got)
	}
	if !got.CompletedAt.Present || got.CompletedAt.Value == nil || *got.CompletedAt.Value != "2026-01-31T12:00:00" || !got.StartedAt.Present || got.StartedAt.Value == nil || *got.StartedAt.Value != "2026-01-30T12:00:00" {
		t.Fatalf("lifecycle timestamps changed: %#v", got)
	}
	if got.LaborPrice == nil || *got.LaborPrice != 120.5 || got.PartsPrice == nil || *got.PartsPrice != 35 {
		t.Fatalf("price markers changed: %#v", got)
	}
	if got.Checklist == nil || got.Checklist.FiltersWashed == nil || *got.Checklist.FiltersWashed || got.Checklist.CurrentAmps != "3.2" || got.PartsUsed != "Filtro antibacteriano" {
		t.Fatalf("checklist marker changed: %#v", got.Checklist)
	}
	if got.Notes == nil || *got.Notes != "Concluído." {
		t.Fatalf("internal markers were not removed: %#v", got.Notes)
	}
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var serialized map[string]json.RawMessage
	if err := json.Unmarshal(data, &serialized); err != nil {
		t.Fatal(err)
	}
	if string(serialized["cancelledAt"]) != "null" {
		t.Fatalf("explicit lifecycle null lost: %s", data)
	}
}

func TestMapSupabaseServiceDefaultsAndMarkerSource(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	profile := domain.TechnicianProfile{}
	row := SupabaseServiceRow{
		ID: "service-2", ClientID: "client-2", Type: "OUTRO", Description: "Instalação de suporte",
		Status: "AGENDADO", RequestDate: "2026-10-04T12:30:00Z", Problem: "[DATA_CONCLUSAO:2026-10-01] Ruído no aparelho",
	}
	got := MapSupabaseServiceToLocal(row, profile, now)
	if got.Date != "2026-10-04" || got.ScheduledDate != "2026-10-04" || got.CompletionDate != "" || !got.CompletedAt.Present || got.CompletedAt.Value != nil {
		t.Fatalf("scheduled row lifecycle/default changed: %#v", got)
	}
	if got.ServiceType != domain.ServiceType("Instalação de suporte") || got.Status != domain.MaintenanceScheduled || got.ReturnDate != "" || got.WarrantyDays != 90 {
		t.Fatalf("scheduled row mapping/default changed: %#v", got)
	}
	if got.Notes == nil || *got.Notes != "Ruído no aparelho" {
		t.Fatalf("display notes should remove markers while lifecycle parsing remains observation-only: %#v", got.Notes)
	}
}

func TestAddMonthsISOUsesMonthEndAndFallback(t *testing.T) {
	location := time.FixedZone("UTC-3", -3*60*60)
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, location)
	if got := addMonthsISO("2026-01-31", 6, location, now); got != "2026-07-31" {
		t.Fatalf("month-end return = %q", got)
	}
	if got := addMonthsISO("not-a-date", 6, location, now); got != "2026-10-03" {
		t.Fatalf("invalid date fallback = %q", got)
	}
}

func TestPortugueseNumberAndJavaScriptWhitespaceFormatting(t *testing.T) {
	if got := formatBrazilianNumber(9000.45678); got != "9.000,457" {
		t.Fatalf("Brazilian BTU formatting = %q", got)
	}
	if got := collapseJavaScriptWhitespace("  Serviço\u00a0 realizado\u2028com\uFEFF cuidado  "); got != "Serviço realizado com cuidado" {
		t.Fatalf("note whitespace normalization = %q", got)
	}
}
