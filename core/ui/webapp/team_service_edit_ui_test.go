package webapp

import (
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

func TestParseTeamServicePriceAcceptsHTMLAndBrazilianDecimals(t *testing.T) {
	for input, want := range map[string]float64{"235.50": 235.5, "1.234,50": 1234.5, "250": 250} {
		got, err := parseTeamServicePrice(input)
		if err != nil || got != want {
			t.Errorf("parseTeamServicePrice(%q) = %v, %v; want %v", input, got, err, want)
		}
	}
	if _, err := parseTeamServicePrice("inválido"); err == nil {
		t.Fatal("invalid price unexpectedly accepted")
	}
}

func TestTeamServiceEditableNotesHideInternalMarkers(t *testing.T) {
	got := teamServiceEditableNotes(`Nota técnica. [GARANTIA_DIAS:90] Garantia de 90 dias. [CHECKLIST:{"filtrosLavados":true}] [MAO_OBRA:200]`)
	if got != "Nota técnica." {
		t.Fatalf("editable notes = %q", got)
	}
}

func TestReceiptEditPanelMatchesLegacyEditableFields(t *testing.T) {
	p := &serviceCatalogPage{teamServiceReceiptEdit: true, teamServiceEdit: &teamServiceEditForm{Date: "2026-10-04", Price: "250", WarrantyDays: "90", Status: "AGENDADO"}}
	ui := p.teamServiceReceiptEditPanel(map[string]any{"id": "service-1"})
	var rendered strings.Builder
	app.PrintHTML(&rendered, ui)
	markup := rendered.String()
	for _, expected := range []string{"Data do serviço", "Valor (R$)", "Garantia (dias)", "Status atual", "Agendado", "Salvar no Banco"} {
		if !strings.Contains(markup, expected) {
			t.Errorf("receipt editor is missing %q: %s", expected, markup)
		}
	}
	if strings.Contains(markup, `value="CONCLUIDO"`) || strings.Contains(markup, `value="CANCELADO"`) {
		t.Fatalf("receipt edit panel must not expose direct lifecycle status transitions: %s", markup)
	}
}

func TestTeamServiceEditDialogDoesNotRenderEmptyErrorBanner(t *testing.T) {
	p := &serviceCatalogPage{teamServiceEdit: &teamServiceEditForm{ID: "service-1", Date: "2026-10-04", OriginalDate: "2026-10-04", Price: "250", WarrantyDays: "90", Status: "AGENDADO"}}
	var rendered strings.Builder
	app.PrintHTML(&rendered, p.teamServiceEditDialog())
	markup := rendered.String()
	if strings.Contains(markup, "portal-section__error") {
		t.Fatalf("empty validation feedback rendered an error banner: %s", markup)
	}
	if !strings.Contains(markup, "Salvar no Banco") {
		t.Fatalf("service edit dialog omitted its save action: %s", markup)
	}
}

func TestTeamServiceEditDialogKeepsStatusReadOnlyAndExplainsLifecycleActions(t *testing.T) {
	p := &serviceCatalogPage{teamServiceEdit: &teamServiceEditForm{ID: "service-1", Date: "2026-10-05", OriginalDate: "2026-10-04", Price: "250", WarrantyDays: "90", Status: "AGENDADO"}}
	var rendered strings.Builder
	app.PrintHTML(&rendered, p.teamServiceEditDialog())
	markup := rendered.String()
	for _, expected := range []string{"Status atual", "Agendado", "use a ação correspondente", "Agenda será atualizada", "avisado pelo WhatsApp"} {
		if !strings.Contains(markup, expected) {
			t.Errorf("service edit dialog is missing %q: %s", expected, markup)
		}
	}
	if strings.Contains(markup, `value="CONCLUIDO"`) || strings.Contains(markup, `value="CANCELADO"`) {
		t.Fatalf("service edit dialog must not expose direct lifecycle status transitions: %s", markup)
	}
}

func TestTeamServiceEditDateFieldPreservesLifecycleDates(t *testing.T) {
	for _, test := range []struct {
		status, field, marker string
	}{
		{"AGENDADO", "data_agendamento", ""},
		{"EM_ANDAMENTO", "data_agendamento", ""},
		{"CONCLUIDO", "data_conclusao", "DATA_CONCLUSAO"},
		{"CANCELADO", "data_cancelamento", "DATA_CANCELAMENTO"},
	} {
		field, marker := teamServiceEditDateField(test.status)
		if field != test.field || marker != test.marker {
			t.Errorf("teamServiceEditDateField(%q) = (%q, %q), want (%q, %q)", test.status, field, marker, test.field, test.marker)
		}
	}
}

func TestTeamServiceStatusLabel(t *testing.T) {
	for status, want := range map[string]string{"AGENDADO": "Agendado", "EM_ANDAMENTO": "Em andamento", "CONCLUIDO": "Concluído", "CANCELADO": "Cancelado", "": "Pendente"} {
		if got := teamServiceStatusLabel(status); got != want {
			t.Errorf("teamServiceStatusLabel(%q) = %q, want %q", status, got, want)
		}
	}
}

func TestFormErrorNoticeOnlyRendersVisibleAlertForMessages(t *testing.T) {
	var empty strings.Builder
	app.PrintHTML(&empty, formErrorNotice(" \n "))
	if strings.Contains(empty.String(), "portal-section__error") {
		t.Fatalf("blank message rendered a visible alert: %s", empty.String())
	}
	var visible strings.Builder
	app.PrintHTML(&visible, formErrorNotice("Falha ao salvar"))
	if !strings.Contains(visible.String(), `role="alert"`) || !strings.Contains(visible.String(), "Falha ao salvar") {
		t.Fatalf("non-empty message was not rendered as an alert: %s", visible.String())
	}
}

func TestValidCivilDateRejectsNormalizedOrInvalidDates(t *testing.T) {
	for _, value := range []string{"2024-02-29", "2026-10-04"} {
		if !validCivilDate(value) {
			t.Errorf("validCivilDate(%q) = false", value)
		}
	}
	for _, value := range []string{"2026-02-29", "2026-13-01", "2026-1-01", ""} {
		if validCivilDate(value) {
			t.Errorf("validCivilDate(%q) = true", value)
		}
	}
}
