package webapp

import (
	"fmt"
	"testing"
	"time"
)

func TestMonthCalendarDaysBuildsSundayFirstSixWeekGrid(t *testing.T) {
	location := time.FixedZone("BRT", -3*60*60)
	items := []teamAgendaItem{
		{ID: "one", Date: "2026-10-01", Customer: "Ana"},
		{ID: "two", Date: "2026-10-01", Customer: "Bia"},
		{ID: "invalid", Date: "2026-02-30"},
		{ID: "empty"},
	}
	first, days := monthCalendarDays("2026-10", items, location)
	if first.Format("2006-01-02") != "2026-10-01" || len(days) != 42 {
		t.Fatalf("first=%s days=%d", first, len(days))
	}
	if got := days[0].Date.Format("2006-01-02"); got != "2026-09-27" {
		t.Fatalf("grid must start Sunday before month, got %s", got)
	}
	if got := days[4].Date.Format("2006-01-02"); got != "2026-10-01" || len(days[4].Items) != 2 {
		t.Fatalf("October 1 cell date=%s items=%v", got, days[4].Items)
	}
	if got := days[35].Date.Format("2006-01-02"); got != "2026-11-01" {
		t.Fatalf("sixth week must continue into November, got %s", got)
	}
}

func TestMonthCalendarDefaultsInvalidMonthAndMapsItemsFromAgendaGroups(t *testing.T) {
	location := time.UTC
	first, days := monthCalendarDays("not-a-month", nil, location)
	if first.Day() != 1 || len(days) != 42 {
		t.Fatalf("invalid month should default to current month: %s (%d cells)", first, len(days))
	}
	groups := teamAgendaGroups{
		Overdue: []teamAgendaItem{{ID: "late"}}, Upcoming: []teamAgendaItem{{ID: "next"}}, RecentlyCompleted: []teamAgendaItem{{ID: "done"}},
	}
	items := monthCalendarItems(groups)
	if len(items) != 3 || items[0].ID != "late" || items[1].ID != "next" || items[2].ID != "done" {
		t.Fatalf("calendar items=%+v", items)
	}
}

func TestPortugueseMonthLabels(t *testing.T) {
	if got := monthNamePortuguese(time.March); got != "Março" {
		t.Fatalf("March label=%q", got)
	}
	if got := weekdayNamePortuguese(time.Wednesday); got != "Qua" {
		t.Fatalf("Wednesday label=%q", got)
	}
}

func TestWeekCalendarDaysCrossMonthAndFilterStatus(t *testing.T) {
	location := time.FixedZone("BRT", -3*60*60)
	items := []teamAgendaItem{
		{ID: "scheduled", Status: "AGENDADO", Date: "2026-11-01"},
		{ID: "completed", Status: "CONCLUIDO", Date: "2026-11-01"},
		{ID: "next-week", Status: "AGENDADO", Date: "2026-11-02"},
	}
	start, days := weekCalendarDays("2026-11-01", items, location)
	if start.Format("2006-01-02") != "2026-11-01" || len(days) != 7 {
		t.Fatalf("week start=%s days=%d", start, len(days))
	}
	if len(days[0].Items) != 2 || len(filterTeamAgendaStatus(items, "AGENDADO")) != 2 || len(filterTeamAgendaStatus(items, "CONCLUIDO")) != 1 {
		t.Fatalf("first day=%+v status filtering failed", days[0])
	}
	if got := days[1].Date.Format("2006-01-02"); got != "2026-11-02" || days[1].Items[0].ID != "next-week" {
		t.Fatalf("second day=%s items=%+v", got, days[1].Items)
	}
}

func TestAllTeamAgendaItemsKeepsOlderCompletedRowsAndServiceScheduleDate(t *testing.T) {
	services := []map[string]any{
		{"id": "done-old", "status": "CONCLUIDO", "data_agendamento": "2026-01-01"},
		{"id": "scheduled", "status": "AGENDADO", "data_agendamento": "2026-02-01"},
		{"id": "cancelled", "status": "CANCELADO", "data_agendamento": "2026-03-01"},
	}
	for i := 0; i < 12; i++ {
		services = append(services, map[string]any{"id": fmt.Sprintf("done-%02d", i), "status": "CONCLUIDO", "data_agendamento": "2026-04-01"})
	}
	items := allTeamAgendaItems(services, nil)
	if len(items) != 14 {
		t.Fatalf("all calendar items=%d; expected all non-cancelled rows", len(items))
	}
	var scheduled teamAgendaItem
	for _, item := range items {
		if item.ID == "scheduled" {
			scheduled = item
		}
		if item.ID == "cancelled" {
			t.Fatal("cancelled OS must not appear in agenda calendar")
		}
	}
	if scheduled.Date != "2026-02-01" {
		t.Fatalf("agenda should use the service schedule fields, matching the React agenda: %+v", scheduled)
	}
}

func TestAllTeamAgendaItemsUsesLinkedAppointmentDateAndTime(t *testing.T) {
	services := []map[string]any{
		{"id": "scheduled", "status": "AGENDADO", "data_agendamento": "2026-10-05", "hora_agendamento": "08:00"},
		{"id": "without-appointment", "status": "AGENDADO", "data_agendamento": "2026-10-06"},
	}
	appointments := []map[string]any{
		{"service_id": "scheduled", "data": "2026-10-07", "hora": "14:30"},
		{"service_id": "orphan", "data": "2026-10-08", "hora": "10:00"},
	}
	items := allTeamAgendaItems(services, appointments)
	if len(items) != 2 {
		t.Fatalf("agenda items=%d, want service rows only", len(items))
	}
	if items[0].Date != "2026-10-07" || items[0].Time != "14:30" {
		t.Fatalf("linked appointment schedule=%+v, want appointment date/time", items[0])
	}
	if items[1].Date != "2026-10-06" || items[1].Time != "" {
		t.Fatalf("unlinked service schedule=%+v, want service fallback", items[1])
	}
	if services[0]["data_agendamento"] != "2026-10-05" || services[0]["hora_agendamento"] != "08:00" {
		t.Fatal("merging appointment schedule mutated the original service row")
	}
}
