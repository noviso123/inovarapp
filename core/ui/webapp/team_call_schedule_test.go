package webapp

import (
	"testing"
	"time"

	"inovarapp/core/domain"
)

func TestTeamCallScheduleDefaultsAndAppointmentPayload(t *testing.T) {
	now := time.Date(2026, 10, 4, 23, 30, 0, 0, time.FixedZone("BRT", -3*60*60))
	if got := teamCallScheduleDefaultDate(now); got != "2026-10-05" {
		t.Fatalf("default date=%q", got)
	}
	payload := teamCallScheduleAppointmentPayload("service-1", "client-1", "2026-10-05", "09:00", "Chegar 15 min antes")
	if payload["service_id"] != "service-1" || payload["cliente_id"] != "client-1" || payload["data"] != "2026-10-05" || payload["hora"] != "09:00" || payload["status"] != "AGENDADO" || payload["observacoes"] != "Chegar 15 min antes" {
		t.Fatalf("appointment payload=%#v", payload)
	}
}

func TestTeamScheduledCallMessageMatchesLegacyPlaceholders(t *testing.T) {
	profile := domain.TechnicianProfile{
		BusinessName:     "Clima Inovar",
		WhatsAppMessages: map[string]string{"chamado_agendado": "{{cliente}}|{{servico}}|{{data}}|{{hora}}|{{empresa}}|{{app}}"},
	}
	got := teamScheduledCallMessage(profile, "Ana Silva", "MANUTENCAO_CORRETIVA", "2026-10-05", "09:30")
	want := "Ana|MANUTENCAO CORRETIVA|05/10/2026|09:30|Clima Inovar|" + domain.AppPublicURL
	if got != want {
		t.Fatalf("message=%q want=%q", got, want)
	}
}

func TestTeamRescheduledServiceMessageMatchesLegacyTemplate(t *testing.T) {
	profile := domain.TechnicianProfile{
		BusinessName:     "Clima Inovar",
		WhatsAppMessages: map[string]string{"reagendado": "{{cliente}}|{{servico}}|{{data}}|{{hora}}|{{empresa}}"},
	}
	got := teamRescheduledServiceMessage(profile, "Ana Silva", "MANUTENCAO_CORRETIVA", "2026-10-05", "09:30")
	want := "Ana|MANUTENCAO CORRETIVA|05/10/2026|09:30|Clima Inovar"
	if got != want {
		t.Fatalf("message=%q want=%q", got, want)
	}
}

func TestTeamScheduleShouldNotifyOnDateOrTimeChange(t *testing.T) {
	tests := []struct {
		name             string
		oldDate, oldHour string
		newDate, newHour string
		want             bool
	}{
		{name: "unchanged", oldDate: "2026-10-05", oldHour: "09:30:00", newDate: "2026-10-05", newHour: "09:30", want: false},
		{name: "date changed", oldDate: "2026-10-05", oldHour: "09:30", newDate: "2026-10-06", newHour: "09:30", want: true},
		{name: "time changed", oldDate: "2026-10-05", oldHour: "09:30:00", newDate: "2026-10-05", newHour: "10:00", want: true},
		{name: "blank time does not clear existing time", oldDate: "2026-10-05", oldHour: "09:30", newDate: "2026-10-05", newHour: "", want: false},
		{name: "date changed with blank new time", oldDate: "2026-10-05", oldHour: "09:30", newDate: "2026-10-06", newHour: "", want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := teamScheduleShouldNotify(test.oldDate, test.oldHour, test.newDate, test.newHour)
			if got != test.want {
				t.Fatalf("notify=%v want=%v", got, test.want)
			}
		})
	}
}

func TestTeamRescheduleWhatsAppDeliveryNoticeReportsBothOutcomes(t *testing.T) {
	if got := teamRescheduleWhatsAppDeliveryNotice(true); got != "Aviso de reagendamento registrado na fila do WhatsApp." {
		t.Fatalf("success notice=%q", got)
	}
	if got := teamRescheduleWhatsAppDeliveryNotice(false); got != "Não foi possível avisar o cliente pelo WhatsApp." {
		t.Fatalf("failure notice=%q", got)
	}
}

func TestTeamScheduleOriginalDatePrefersServiceDateAndFallsBackToAppointment(t *testing.T) {
	appointment := map[string]any{"data": "2026-10-05T00:00:00Z"}
	if got := teamScheduleOriginalDate(map[string]any{"data_agendamento": "2026-10-04"}, appointment); got != "2026-10-04" {
		t.Fatalf("service date=%q, want service scheduled date", got)
	}
	if got := teamScheduleOriginalDate(nil, appointment); got != "2026-10-05" {
		t.Fatalf("fallback date=%q, want appointment date", got)
	}
}
