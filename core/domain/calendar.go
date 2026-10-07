package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type CalendarInvite struct {
	ID              string
	Title           string
	Description     string
	Location        string
	Date            string
	Time            string
	DurationMinutes *int
}

// CalendarEvent is the shared event shape used by calendar exports and feeds.
type CalendarEvent struct {
	UID         string
	Summary     string
	Description string
	Location    string
	Start       time.Time
	End         time.Time
}

// CalendarEventFromInvite parses the local date/time used by the scheduling
// form and computes a wall-clock end time, matching the existing invite flow.
func CalendarEventFromInvite(invite CalendarInvite, location *time.Location) (CalendarEvent, error) {
	if location == nil {
		location = time.Local
	}
	date := strings.TrimSpace(invite.Date)
	clock := strings.TrimSpace(invite.Time)
	if clock == "" {
		clock = "09:00"
	}
	start, err := time.ParseInLocation("2006-01-02 15:04", date+" "+clock, location)
	if err != nil {
		return CalendarEvent{}, fmt.Errorf("invalid calendar date or time: %w", err)
	}
	duration := 120
	if invite.DurationMinutes != nil {
		duration = *invite.DurationMinutes
	}
	end := time.Date(start.Year(), start.Month(), start.Day(), start.Hour(), start.Minute()+duration, start.Second(), start.Nanosecond(), start.Location())
	return CalendarEvent{
		UID:         invite.ID,
		Summary:     invite.Title,
		Description: invite.Description,
		Location:    invite.Location,
		Start:       start,
		End:         end,
	}, nil
}

// CalendarInviteICS produces a downloadable, floating-local-time ICS invite
// with the one-hour reminder used by the current app.
func CalendarInviteICS(invite CalendarInvite, generatedAt time.Time, location *time.Location) (string, error) {
	event, err := CalendarEventFromInvite(invite, location)
	if err != nil {
		return "", err
	}
	if generatedAt.IsZero() {
		generatedAt = time.Now()
	}
	uid := event.UID
	if uid == "" {
		uid = fmt.Sprintf("%d-%s", generatedAt.UnixMilli(), randomCalendarID())
	}
	stamp := generatedAt.UTC().Format("20060102T150405Z")
	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//InovarApp//PT-BR",
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
		"BEGIN:VEVENT",
		"UID:" + EscapeICSText(uid) + "@inovarapp",
		"DTSTAMP:" + stamp,
		"DTSTART:" + event.Start.Format("20060102T150405"),
		"DTEND:" + event.End.Format("20060102T150405"),
		"SUMMARY:" + EscapeICSText(event.Summary),
		"DESCRIPTION:" + EscapeICSText(event.Description),
		"LOCATION:" + EscapeICSText(event.Location),
		"BEGIN:VALARM",
		"TRIGGER:-PT1H",
		"ACTION:DISPLAY",
		"DESCRIPTION:" + EscapeICSText(event.Summary),
		"END:VALARM",
		"END:VEVENT",
		"END:VCALENDAR",
	}
	return strings.Join(lines, "\r\n"), nil
}

func GoogleCalendarURL(invite CalendarInvite, location *time.Location) (string, error) {
	event, err := CalendarEventFromInvite(invite, location)
	if err != nil {
		return "", err
	}
	query := url.Values{
		"action":   {"TEMPLATE"},
		"text":     {event.Summary},
		"dates":    {event.Start.Format("20060102T150405") + "/" + event.End.Format("20060102T150405")},
		"details":  {event.Description},
		"location": {event.Location},
	}
	return "https://calendar.google.com/calendar/render?" + query.Encode(), nil
}

func OutlookCalendarURL(invite CalendarInvite, location *time.Location) (string, error) {
	event, err := CalendarEventFromInvite(invite, location)
	if err != nil {
		return "", err
	}
	duration := 120
	if invite.DurationMinutes != nil {
		duration = *invite.DurationMinutes
	}
	query := url.Values{
		"path":     {"/calendar/action/compose"},
		"rru":      {"addevent"},
		"subject":  {event.Summary},
		"startdt":  {event.Start.UTC().Format("2006-01-02T15:04:05.000Z")},
		"enddt":    {event.Start.Add(time.Duration(duration) * time.Minute).UTC().Format("2006-01-02T15:04:05.000Z")},
		"body":     {event.Description},
		"location": {event.Location},
	}
	return "https://outlook.live.com/calendar/0/deeplink/compose?" + query.Encode(), nil
}

func randomCalendarID() string {
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(random[:])
}

// EscapeICSText escapes text values according to the calendar format used by
// the current application while preserving Portuguese UTF-8 characters.
func EscapeICSText(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, ";", "\\;")
	value = strings.ReplaceAll(value, ",", "\\,")
	value = strings.ReplaceAll(value, "\r\n", "\n")
	return strings.ReplaceAll(value, "\n", "\\n")
}

// FormatCalendarFeed emits the UTC calendar feed consumed by Google, Apple,
// Outlook, and Android calendar subscriptions.
func FormatCalendarFeed(events []CalendarEvent, generatedAt time.Time) string {
	if generatedAt.IsZero() {
		generatedAt = time.Now()
	}
	stamp := generatedAt.UTC().Format("20060102T150405Z")
	lines := []string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//Inovar Refrigeracao//InovarApp//PT-BR",
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
		"X-WR-CALNAME:Inovar Refrigeração — Agenda",
		"X-WR-TIMEZONE:America/Sao_Paulo",
		"REFRESH-INTERVAL;VALUE=DURATION:PT2H",
		"X-PUBLISHED-TTL:PT2H",
	}
	for _, event := range events {
		lines = append(lines,
			"BEGIN:VEVENT",
			"UID:"+EscapeICSText(event.UID),
			"DTSTAMP:"+stamp,
			"DTSTART:"+event.Start.UTC().Format("20060102T150405Z"),
			"DTEND:"+event.End.UTC().Format("20060102T150405Z"),
			"SUMMARY:"+EscapeICSText(event.Summary),
			"DESCRIPTION:"+EscapeICSText(event.Description),
			"LOCATION:"+EscapeICSText(event.Location),
			"END:VEVENT",
		)
	}
	lines = append(lines, "END:VCALENDAR")
	return strings.Join(lines, "\r\n")
}
