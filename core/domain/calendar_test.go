package domain

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCalendarInviteICSMatchesLocalInviteContract(t *testing.T) {
	duration := 120
	location := time.FixedZone("BRT", -3*60*60)
	generatedAt := time.Date(2026, 10, 3, 13, 0, 0, 0, location)
	ics, err := CalendarInviteICS(CalendarInvite{
		ID:              "svc-123",
		Title:           "Limpeza; avaliação, João",
		Description:     "Primeira linha\nSegunda linha",
		Location:        "Rua A, 10",
		Date:            "2026-10-03",
		Time:            "09:30",
		DurationMinutes: &duration,
	}, generatedAt, location)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"PRODID:-//InovarApp//PT-BR",
		"UID:svc-123@inovarapp",
		"DTSTAMP:20261003T160000Z",
		"DTSTART:20261003T093000",
		"DTEND:20261003T113000",
		"SUMMARY:Limpeza\\; avaliação\\, João",
		"DESCRIPTION:Primeira linha\\nSegunda linha",
		"LOCATION:Rua A\\, 10",
		"TRIGGER:-PT1H",
	} {
		if !strings.Contains(ics, want) {
			t.Errorf("invite missing %q in:\n%s", want, ics)
		}
	}
}

func TestCalendarLinksPreserveLocalAndUTCDateContracts(t *testing.T) {
	duration := 120
	location := time.FixedZone("BRT", -3*60*60)
	invite := CalendarInvite{
		Title:           "Visita técnica",
		Description:     "Verificação elétrica",
		Location:        "Vitória",
		Date:            "2026-10-03",
		Time:            "09:30",
		DurationMinutes: &duration,
	}

	googleURL, err := GoogleCalendarURL(invite, location)
	if err != nil {
		t.Fatal(err)
	}
	google, err := url.Parse(googleURL)
	if err != nil {
		t.Fatal(err)
	}
	if google.Host != "calendar.google.com" || google.Query().Get("action") != "TEMPLATE" || google.Query().Get("dates") != "20261003T093000/20261003T113000" || google.Query().Get("text") != invite.Title {
		t.Fatalf("unexpected Google Calendar link: %s", googleURL)
	}

	outlookURL, err := OutlookCalendarURL(invite, location)
	if err != nil {
		t.Fatal(err)
	}
	outlook, err := url.Parse(outlookURL)
	if err != nil {
		t.Fatal(err)
	}
	if outlook.Host != "outlook.live.com" || outlook.Query().Get("startdt") != "2026-10-03T12:30:00.000Z" || outlook.Query().Get("enddt") != "2026-10-03T14:30:00.000Z" {
		t.Fatalf("unexpected Outlook link: %s", outlookURL)
	}
}

func TestCalendarInviteRejectsInvalidDate(t *testing.T) {
	if _, err := CalendarEventFromInvite(CalendarInvite{Date: "03/10/2026"}, time.UTC); err == nil {
		t.Fatal("expected malformed date to be rejected")
	}
}

func TestFormatCalendarFeedUsesUTCAndEscapesUTF8Fields(t *testing.T) {
	start := time.Date(2026, 10, 3, 12, 30, 0, 0, time.UTC)
	event := CalendarEvent{
		UID:         "inovar-serviço-1@inovarapp",
		Summary:     "Limpeza; técnica, básica — João",
		Description: "Linha 1\r\nLinha 2",
		Location:    "Rua A, 10; Vitória",
		Start:       start,
		End:         start.Add(2 * time.Hour),
	}
	generatedAt := time.Date(2026, 10, 3, 13, 0, 0, 0, time.FixedZone("BRT", -3*60*60))
	ics := FormatCalendarFeed([]CalendarEvent{event}, generatedAt)

	for _, want := range []string{
		"X-WR-CALNAME:Inovar Refrigeração — Agenda",
		"X-WR-TIMEZONE:America/Sao_Paulo",
		"UID:inovar-serviço-1@inovarapp",
		"DTSTAMP:20261003T160000Z",
		"DTSTART:20261003T123000Z",
		"DTEND:20261003T143000Z",
		"SUMMARY:Limpeza\\; técnica\\, básica — João",
		"DESCRIPTION:Linha 1\\nLinha 2",
		"LOCATION:Rua A\\, 10\\; Vitória",
		"END:VCALENDAR",
	} {
		if !strings.Contains(ics, want) {
			t.Errorf("calendar feed missing %q in:\n%s", want, ics)
		}
	}
	if strings.Contains(strings.ReplaceAll(ics, "\r\n", ""), "\n") {
		t.Fatal("calendar lines must use CRLF without raw line breaks in values")
	}
}

func TestFormatCalendarFeedSupportsNoEvents(t *testing.T) {
	ics := FormatCalendarFeed(nil, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	if !strings.HasSuffix(ics, "END:VCALENDAR") || strings.Contains(ics, "BEGIN:VEVENT") {
		t.Fatalf("unexpected empty feed: %q", ics)
	}
}
