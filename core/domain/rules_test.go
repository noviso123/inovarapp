package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestReturnStatusForBoundaries(t *testing.T) {
	today := time.Date(2026, 10, 3, 23, 30, 0, 0, time.FixedZone("UTC-3", -3*60*60))
	cases := []struct {
		days int
		want ReturnStatus
	}{
		{-1, ReturnOverdue}, {0, ReturnThisWeek}, {7, ReturnThisWeek},
		{8, ReturnSoon}, {30, ReturnSoon}, {31, ReturnOnTime},
	}
	for _, tc := range cases {
		target := today.AddDate(0, 0, tc.days)
		if got := ReturnStatusFor(target, today); got != tc.want {
			t.Errorf("days %d: got %q, want %q", tc.days, got, tc.want)
		}
	}
}

func TestAddMonthsClamped(t *testing.T) {
	cases := []struct {
		in     time.Time
		months int
		want   string
	}{
		{time.Date(2025, 1, 31, 9, 15, 0, 0, time.UTC), 1, "2025-02-28T09:15:00Z"},
		{time.Date(2024, 1, 31, 9, 15, 0, 0, time.UTC), 1, "2024-02-29T09:15:00Z"},
		{time.Date(2026, 10, 3, 9, 15, 0, 0, time.UTC), -1, "2026-09-03T09:15:00Z"},
	}
	for _, tc := range cases {
		if got := AddMonthsClamped(tc.in, tc.months).Format(time.RFC3339); got != tc.want {
			t.Errorf("got %s, want %s", got, tc.want)
		}
	}
}

func TestNormalizeServiceName(t *testing.T) {
	cases := []struct{ input, want string }{
		{"Avalia" + string(rune(0xFFFD)) + "o T" + string(rune(0xFFFD)) + "cnica", "Avaliação Técnica"},
		{"Avaliação Técnica", "Avaliação Técnica"},
		{"Avaliação Tecnica antiga", "Avaliação Técnica"},
		{"Avaliação Tcn antiga", "Avaliação Técnica"},
		{"Avaliação", "Avaliação Técnica"},
		{"Limpeza de Ar Completa", "Limpeza de Ar"},
		{"Limpeza residencial", "Limpeza residencial"},
		{"", "Outro"},
		{"Instalação", "Instalação"},
	}
	for _, tc := range cases {
		if got := NormalizeServiceName(tc.input); got != tc.want {
			t.Errorf("NormalizeServiceName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestJSONKeepsPortugueseAndOptionalChecklistState(t *testing.T) {
	falseValue := false
	notes := "Verificação — pressão e dreno"
	input := MaintenanceRecord{ID: "á-1", ServiceType: ServiceTechnicalAssessment, Notes: &notes, Checklist: &ChecklistData{FiltersWashed: &falseValue}}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var output MaintenanceRecord
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	if output.ID != input.ID || output.Notes == nil || *output.Notes != notes || output.Checklist == nil || output.Checklist.FiltersWashed == nil || *output.Checklist.FiltersWashed {
		t.Fatalf("round trip changed UTF-8 or false-vs-omitted checklist state: %#v", output)
	}
}

func TestJSONUsesExistingClientFieldNames(t *testing.T) {
	input := MaintenanceRecord{
		ID: "os-1", ClientID: "cl-1", ApplianceID: "ap-1", Date: "2026-10-03",
		ReturnDate: "2027-04-03", ServiceType: ServiceCleaning, Price: 210,
		PaymentMethod: PaymentPIX, WarrantyDays: 90, Status: MaintenanceScheduled,
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"clientId", "applianceId", "returnDate", "serviceType", "paymentMethod", "warrantyDays"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("expected existing JSON field %q in %s", key, data)
		}
	}
	for _, key := range []string{"client_id", "aparelho_id", "data_retorno", "tipo_servico"} {
		if _, ok := fields[key]; ok {
			t.Errorf("unexpected translated JSON field %q in %s", key, data)
		}
	}
}

func TestReturnStatusForDate(t *testing.T) {
	today := time.Date(2026, 10, 3, 23, 0, 0, 0, time.FixedZone("UTC-3", -3*60*60))
	if got := ReturnStatusForDate("", today); got != ReturnNoHistory {
		t.Fatalf("empty date: %q", got)
	}
	if got := ReturnStatusForDate("invalid", today); got != ReturnOnTime {
		t.Fatalf("invalid date: %q", got)
	}
	if got := ReturnStatusForDate("2026-10-04", today); got != ReturnThisWeek {
		t.Fatalf("next civil day: %q", got)
	}
	if got := ReturnStatusForDate("2026-10-04T23:59:00-03:00", today); got != ReturnThisWeek {
		t.Fatalf("ISO timestamp should use its local civil date: %q", got)
	}
	if got := ReturnStatusForDate("2026-10-02T23:59:00-03:00", today); got != ReturnOverdue {
		t.Fatalf("previous local civil date should be overdue: %q", got)
	}
}

func TestReturnStatusPresentationMatchesLegacyPortugueseLabelsAndColorTokens(t *testing.T) {
	cases := []struct {
		status                   ReturnStatus
		label, badge, badgeText  string
		border, accent, gradient string
	}{
		{ReturnOverdue, "Atrasado", "bg-red-500/20 text-red-400 border border-red-500/40", "text-red-400", "border-l-red-500", "text-red-400", "from-red-950/40 via-slate-900 to-slate-900"},
		{ReturnThisWeek, "Esta semana", "bg-amber-500/20 text-amber-300 border border-amber-500/40", "text-amber-300", "border-l-amber-400", "text-amber-300", "from-amber-950/40 via-slate-900 to-slate-900"},
		{ReturnSoon, "Em breve", "bg-sky-500/20 text-sky-300 border border-sky-500/40", "text-sky-300", "border-l-sky-400", "text-sky-300", "from-sky-950/40 via-slate-900 to-slate-900"},
		{ReturnOnTime, "Em dia", "bg-emerald-500/20 text-emerald-400 border border-emerald-500/40", "text-emerald-400", "border-l-emerald-500", "text-emerald-400", "from-emerald-950/40 via-slate-900 to-slate-900"},
		{ReturnNoHistory, "Sem histórico", "bg-violet-500/20 text-violet-300 border border-violet-500/40", "text-violet-300", "border-l-violet-400", "text-violet-300", "from-violet-950/40 via-slate-900 to-slate-900"},
	}
	for _, tc := range cases {
		if got := ReturnStatusLabel(tc.status); got != tc.label {
			t.Errorf("label for %s = %q, want %q", tc.status, got, tc.label)
		}
		style := ReturnStatusPresentation(tc.status)
		if style.BadgeBackground != tc.badge || style.BadgeText != tc.badgeText || style.Border != tc.border || style.Accent != tc.accent || style.Gradient != tc.gradient {
			t.Errorf("style for %s = %+v", tc.status, style)
		}
	}
}
