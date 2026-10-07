package domain

import (
	"testing"
	"time"
)

func TestBuildPreventiveAlertsUsesNewestHistoryAndHonorsLateWindow(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	customers := []AlertCustomer{{ID: "client-1", Name: "João Silva", Active: true}, {ID: "inactive", Name: "Inativo", Active: false}}
	appliances := []AlertAppliance{{ID: "ac-1", CustomerID: "client-1", Brand: "LG", Model: "Dual", Room: "Sala"}, {ID: "ac-old", CustomerID: "client-1", Brand: "Midea", LastMaintenance: "2024-01-01"}, {ID: "inactive-ac", CustomerID: "inactive", LastMaintenance: "2026-03-01"}}
	history := []AlertHistory{{CustomerID: "client-1", ApplianceID: "ac-1", Date: "2026-02-20"}}
	services := []AlertService{{CustomerID: "client-1", ApplianceID: "ac-1", Status: "CONCLUIDO", ScheduledDate: "2026-03-01"}, {CustomerID: "client-1", ApplianceID: "ac-1", Status: "PENDENTE", ScheduledDate: "2026-09-20"}}
	got := BuildPreventiveAlerts(customers, appliances, history, services, 6, 180, now)
	if len(got) != 1 || got[0].LastServiceDate != "2026-03-01" || got[0].ExpectedDate != "2026-09-01" || got[0].DaysToDue != -32 {
		t.Fatalf("alerts=%#v", got)
	}
}

func TestBuildPreventiveAlertsIncludesFiveDayLeadAndExcludesTooOld(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	customers := []AlertCustomer{{ID: "c", Name: "Cliente", Active: true}}
	appliances := []AlertAppliance{{ID: "lead", CustomerID: "c", LastMaintenance: "2026-04-08"}, {ID: "old", CustomerID: "c", LastMaintenance: "2025-09-01"}}
	got := BuildPreventiveAlerts(customers, appliances, nil, nil, 6, 180, now)
	if len(got) != 1 || got[0].Appliance.ID != "lead" || got[0].DaysToDue != 5 {
		t.Fatalf("alerts=%#v", got)
	}
}

func TestBuildAgendaReminderUsesServiceLabelAndEquipmentFallback(t *testing.T) {
	reminder := BuildAgendaReminder(AlertService{Type: "MANUTENCAO_PREVENTIVA", Description: "{\"tipo\":\"old\"}"}, AlertCustomer{Name: "Ágata"}, nil)
	if reminder.ServiceName != "Manutenção Preventiva" || reminder.ApplianceDescription != "equipamento não informado no cadastro" {
		t.Fatalf("reminder=%#v", reminder)
	}
	appliance := AlertAppliance{Brand: "Daikin", Model: "Eco", BTUs: "12000", Room: "Quarto"}
	if got := AlertEquipmentDescription(&appliance, false); got != "Daikin Eco 12000 BTUs (Quarto)" {
		t.Fatalf("cycle equipment=%q", got)
	}
	if got := AlertEquipmentDescription(&appliance, true); got != "Daikin Eco 12000 BTUs — Quarto" {
		t.Fatalf("agenda equipment=%q", got)
	}
}

func TestPreventiveCycleHonorsConfiguredMonthsAndMonthEnd(t *testing.T) {
	for _, tc := range []struct {
		months    int
		last, due string
	}{
		{2, "2026-07-31", "2026-09-30"},
		{3, "2026-01-31", "2026-04-30"},
		{0, "2026-01-31", "2026-04-30"},
	} {
		now, _ := time.Parse("2006-01-02", tc.due)
		got := BuildPreventiveAlerts([]AlertCustomer{{ID: "c", Active: true}}, []AlertAppliance{{ID: "a", CustomerID: "c", LastMaintenance: tc.last}}, nil, nil, tc.months, 180, now)
		if len(got) != 1 || got[0].ExpectedDate != tc.due || got[0].DaysToDue != 0 {
			t.Fatalf("cycle %+v: got %+v", tc, got)
		}
	}
}

func TestPreventiveCycleKeepsExplicitReturnFromSameDayService(t *testing.T) {
	got := BuildPreventiveAlerts([]AlertCustomer{{ID: "c", Active: true}}, []AlertAppliance{{ID: "a", CustomerID: "c", LastMaintenance: "2026-07-07"}},
		[]AlertHistory{{CustomerID: "c", ApplianceID: "a", Date: "2026-07-07"}},
		[]AlertService{{CustomerID: "c", ApplianceID: "a", Status: "CONCLUIDO", CompletionDate: "2026-07-07", Observations: "[PROXIMO_RETORNO:2026-09-07]"}},
		3, 180, time.Date(2026, 9, 7, 23, 0, 0, 0, time.UTC))
	if len(got) != 1 || got[0].ExpectedDate != "2026-09-07" || got[0].DaysToDue != 0 {
		t.Fatalf("explicit cycle lost: %+v", got)
	}
}

