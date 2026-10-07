package webapp

import (
	"strings"
	"testing"
	"time"

	"inovarapp/core/domain"
)

func TestNewTeamDirectServiceFormUsesLegacyInitialValues(t *testing.T) {
	now := time.Date(2026, 10, 4, 23, 30, 0, 0, time.FixedZone("BRT", -3*60*60))
	customers := []map[string]any{{
		"id":         "client-1",
		"appliances": []any{map[string]any{"id": "appliance-1"}},
	}}
	form := newTeamDirectServiceForm(customers, now)
	if form.ClientID != "client-1" || form.ApplianceID != "appliance-1" || form.ServiceType != string(domain.ServiceCleaning) || form.Date != "2026-10-06" || form.Price != "250" || form.Status != "AGENDADO" {
		t.Fatalf("initial direct service form=%+v", form)
	}
}

func TestNewTeamDirectServiceFormUsesLegacyUTCTomorrow(t *testing.T) {
	// React computes new Date(Date.now() + 86400000).toISOString().slice(0, 10).
	// Use a fixed offset and a local date whose UTC day is already the next day.
	now := time.Date(2026, 3, 9, 0, 30, 0, 0, time.FixedZone("BRT", -3*60*60))
	form := newTeamDirectServiceForm(nil, now)
	if form.Date != "2026-03-10" {
		t.Fatalf("date=%q, want UTC tomorrow 2026-03-10", form.Date)
	}
}

func TestTeamDirectServiceAndAppointmentPayloadsMatchLegacyContract(t *testing.T) {
	form := teamDirectServiceForm{
		ClientID: "client-1", ApplianceID: "appliance-1", ServiceType: " Instalação ",
		Date: "2026-10-05", Status: "AGENDADO", Notes: "  levar escada  ",
	}
	service := teamDirectServicePayload(form, 325.5)
	wantService := map[string]any{
		"cliente_id": "client-1", "aparelho_id": "appliance-1", "tipo": "INSTALACAO",
		"descricao": "Instalação", "status": "AGENDADO", "valor": 325.5,
		"data_agendamento": "2026-10-05", "observacoes": "levar escada",
	}
	for key, want := range wantService {
		if service[key] != want {
			t.Errorf("service[%q]=%v, want %v", key, service[key], want)
		}
	}
	appointment := teamDirectAppointmentPayload(form, "service-1")
	wantAppointment := map[string]any{
		"service_id": "service-1", "cliente_id": "client-1", "data": "2026-10-05",
		"status": "AGENDADO", "observacoes": "levar escada",
	}
	for key, want := range wantAppointment {
		if appointment[key] != want {
			t.Errorf("appointment[%q]=%v, want %v", key, appointment[key], want)
		}
	}
}

func TestTeamDirectServiceRetryReusesServiceAndAppointment(t *testing.T) {
	form := teamDirectServiceForm{ServiceID: "service-1"}
	if got := teamDirectServiceMethod(form); got != "PATCH" {
		t.Fatalf("service retry method=%q, want PATCH", got)
	}
	if got := teamDirectServiceMethod(teamDirectServiceForm{}); got != "POST" {
		t.Fatalf("new service method=%q, want POST", got)
	}
	appointments := []map[string]any{{"service_id": "service-1"}}
	if !teamDirectAppointmentExists(appointments, "service-1") {
		t.Fatal("existing appointment was not found")
	}
	if got := teamDirectAppointmentMethod(true, teamDirectAppointmentExists(appointments, "service-1")); got != "PATCH" {
		t.Fatalf("retry appointment method=%q, want PATCH", got)
	}
	if got := teamDirectAppointmentMethod(true, teamDirectAppointmentExists(nil, "service-1")); got != "POST" {
		t.Fatalf("missing appointment method=%q, want POST", got)
	}
}

func TestRegisteredServiceMessageUsesTemplateAndFormattedDate(t *testing.T) {
	profile := domain.TechnicianProfile{BusinessName: "Inovar Refrigeração"}
	message := registeredServiceMessage(profile, "Maria Silva", "Instalação", "2026-10-05")
	for _, want := range []string{"Maria", "Instalação", "05/10/2026", "Inovar Refrigeração"} {
		if !strings.Contains(message, want) {
			t.Fatalf("message missing %q: %q", want, message)
		}
	}
}
