package webapp

import (
	"errors"
	"strings"
	"testing"
	"time"

	"inovarapp/core/domain"
)

func TestNewTeamAppointmentFormUsesLegacyDefaultsAndInitialTarget(t *testing.T) {
	now := time.Date(2026, 10, 4, 15, 30, 0, 0, time.FixedZone("BRT", -3*60*60))
	customers := []map[string]any{
		{"id": "client-1", "nome": "Ana", "appliances": []any{}},
		{"id": "client-2", "nome": "Bia", "appliances": []any{map[string]any{"id": "appliance-2"}}},
	}
	form := newTeamAppointmentForm(domain.TechnicianProfile{DefaultPrice: 275}, customers, now)
	if form.Date != "2026-10-04" || form.Time != "09:00" || form.ReturnDate != "2027-04-04" || form.ServiceType != string(domain.ServiceCleaning) || form.Price != "275" {
		t.Fatalf("appointment defaults=%+v", form)
	}
	if form.ClientID != "client-1" || form.ApplianceID != "" {
		t.Fatalf("initial target=%+v", form)
	}
	form = newTeamAppointmentForm(domain.TechnicianProfile{}, []map[string]any{{"id": "client-1", "appliances": []any{map[string]any{"id": "appliance-1"}}}}, now)
	if form.ClientID != "client-1" || form.ApplianceID != "appliance-1" || form.Price != "250" {
		t.Fatalf("fallback defaults=%+v", form)
	}
}

func TestNewTeamAppointmentFormForApplianceKeepsQueueTarget(t *testing.T) {
	now := time.Date(2026, 10, 4, 15, 30, 0, 0, time.FixedZone("BRT", -3*60*60))
	customers := []map[string]any{
		{"id": "client-1", "appliances": []any{map[string]any{"id": "appliance-1"}}},
		{"id": "client-2", "appliances": []any{map[string]any{"id": "appliance-2"}}},
	}
	form := newTeamAppointmentFormForAppliance(domain.TechnicianProfile{}, customers, "client-2", "appliance-2", now)
	if form.ClientID != "client-2" || form.ApplianceID != "appliance-2" || form.Date != "2026-10-04" {
		t.Fatalf("contextual appointment target=%+v", form)
	}
}

func TestAppointmentApplianceMustBelongToSelectedCustomer(t *testing.T) {
	customers := []map[string]any{
		{"id": "client-1", "appliances": []any{map[string]any{"id": "appliance-1"}}},
		{"id": "client-2", "appliances": []any{map[string]any{"id": "appliance-2"}}},
	}
	if !appointmentApplianceBelongsToClient(customers, "client-1", "appliance-1") {
		t.Fatal("expected appliance to be linked to selected customer")
	}
	if appointmentApplianceBelongsToClient(customers, "client-1", "appliance-2") {
		t.Fatal("accepted appliance linked to a different customer")
	}
}

func TestAppointmentDateFormattingAndMessageTemplate(t *testing.T) {
	if got := appointmentDateBR("2026-10-05"); got != "05/10/2026" {
		t.Fatalf("formatted date=%q", got)
	}
	message := domain.ApplyWhatsAppPlaceholders(domain.ResolveWhatsAppTemplate(nil, "agendamento_criado"), map[string]string{
		"cliente": "Ana", "data": "05/10/2026", "servico": "Higienização", "empresa": "Inovar Refrigeração",
	})
	if message == "" || !strings.Contains(message, "Ana") || !strings.Contains(message, "05/10/2026") || !strings.Contains(message, "Higienização") || !strings.Contains(message, "Inovar Refrigeração") {
		t.Fatalf("confirmation message=%q", message)
	}
}

func TestAppointmentConfirmationRequiresSavedAppointmentAndPhone(t *testing.T) {
	tests := []struct {
		name          string
		serviceID     string
		phone         string
		appointmentErr error
		want          bool
	}{
		{name: "saved appointment with phone", serviceID: "service-1", phone: "27999991234", want: true},
		{name: "appointment write failed", serviceID: "service-1", phone: "27999991234", appointmentErr: errors.New("save failed")},
		{name: "missing service id", phone: "27999991234"},
		{name: "missing phone", serviceID: "service-1"},
		{name: "blank phone", serviceID: "service-1", phone: "   "},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := appointmentConfirmationCanBeSent(test.serviceID, test.phone, test.appointmentErr); got != test.want {
				t.Fatalf("confirmation allowed=%t, want %t", got, test.want)
			}
		})
	}
}
