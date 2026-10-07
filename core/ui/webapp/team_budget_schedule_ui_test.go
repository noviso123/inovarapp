package webapp

import (
	"errors"
	"strings"
	"testing"

	"inovarapp/core/domain"
)

func TestBudgetScheduledServiceDescriptionAndType(t *testing.T) {
	budget := domain.BudgetEstimate{ApplianceDescription: "Ar condicionado 12.000 BTUs"}
	if got := budgetScheduledServiceDescription(budget); got != budget.ApplianceDescription {
		t.Fatalf("fallback description = %q", got)
	}
	if got := budgetScheduledServiceType(budget); got != "OUTRO" {
		t.Fatalf("fallback type = %q", got)
	}
	budget.Items = []domain.BudgetItem{{Description: "Higienização completa"}}
	if got := budgetScheduledServiceDescription(budget); got != "Higienização completa" {
		t.Fatalf("item description = %q", got)
	}
	if got := budgetScheduledServiceType(budget); got != "Higienização completa" {
		t.Fatalf("item type = %q", got)
	}
}

func TestBudgetScheduledApplianceIDFallback(t *testing.T) {
	customer := map[string]any{"appliances": []any{map[string]any{"id": "equipment-1"}}}
	if got := budgetScheduledApplianceID(domain.BudgetEstimate{}, customer); got != "equipment-1" {
		t.Fatalf("single-appliance fallback = %q", got)
	}
	wanted := "equipment-2"
	if got := budgetScheduledApplianceID(domain.BudgetEstimate{ApplianceID: &wanted}, customer); got != wanted {
		t.Fatalf("budget appliance = %q", got)
	}
	multiple := map[string]any{"appliances": []any{map[string]any{"id": "1"}, map[string]any{"id": "2"}}}
	if got := budgetScheduledApplianceID(domain.BudgetEstimate{}, multiple); got != "" {
		t.Fatalf("ambiguous appliance fallback = %q", got)
	}
}

func TestTeamCustomerHasApplianceValidatesOwnership(t *testing.T) {
	customer := map[string]any{"appliances": []any{map[string]any{"id": "equipment-1"}}}
	if !teamCustomerHasAppliance(customer, "equipment-1") {
		t.Fatal("expected linked appliance to belong to the customer")
	}
	if teamCustomerHasAppliance(customer, "equipment-other") {
		t.Fatal("accepted an appliance that is not linked to the customer")
	}
}

func TestBudgetApplianceSelectionMatchesCustomerEquipmentRules(t *testing.T) {
	withDevice := map[string]any{"appliances": []any{map[string]any{"id": "equipment-1"}}}
	withoutDevices := map[string]any{"appliances": []any{}}
	if !isValidBudgetApplianceSelection(withDevice, "equipment-1") {
		t.Fatal("expected valid linked equipment selection")
	}
	if isValidBudgetApplianceSelection(withDevice, "") || isValidBudgetApplianceSelection(withDevice, "other") {
		t.Fatal("accepted missing or unrelated equipment for customer with equipment")
	}
	if !isValidBudgetApplianceSelection(withoutDevices, "") || isValidBudgetApplianceSelection(withoutDevices, "equipment-1") {
		t.Fatal("unexpected equipment selection for customer without registered equipment")
	}
}

func TestCloneBudgetMetadataPreservesValuesWithoutAliasing(t *testing.T) {
	originalSignature, originalPaid, originalAmount := "signed", true, 125.0
	form := teamBudgetForm{
		Signature:      cloneBudgetString(&originalSignature),
		Paid:           cloneBudgetBool(&originalPaid),
		AmountReceived: cloneBudgetFloat(&originalAmount),
	}
	originalSignature, originalPaid, originalAmount = "changed", false, 0
	if *form.Signature != "signed" || !*form.Paid || *form.AmountReceived != 125 {
		t.Fatalf("cloned metadata changed with its source: %#v", form)
	}
}

func TestBudgetScheduledWhatsAppMessageUsesTemplateAndPlaceholders(t *testing.T) {
	profile := domain.TechnicianProfile{BusinessName: "Clima João", WhatsAppMessages: map[string]string{
		"orcamento_agendado": "Oi {{cliente}}: {{data}} às {{hora}}, {{valor}} — {{empresa}} ({{app}}).",
	}}
	got := budgetScheduledWhatsAppMessage(profile, domain.BudgetEstimate{FinalValue: 125.5}, "Maria da Silva", "2026-10-05", "09:30")
	for _, want := range []string{"Oi Maria:", "05/10/2026 às 09:30", "R$ 125.50", "Clima João", "https://inovarapp.vercel.app"} {
		if !strings.Contains(got, want) {
			t.Errorf("message %q does not contain %q", got, want)
		}
	}
	if strings.Contains(got, "{{") {
		t.Fatalf("unresolved placeholder in %q", got)
	}
}

func TestBudgetScheduleAppointmentRequiresServiceID(t *testing.T) {
	if got := budgetScheduleAppointmentError("", nil); got == nil {
		t.Fatal("missing service ID must fail instead of reporting a confirmed appointment")
	}
	if got := budgetScheduleAppointmentError("service-1", nil); got != nil {
		t.Fatalf("successful appointment save returned error: %v", got)
	}
	saveErr := errors.New("network unavailable")
	if got := budgetScheduleAppointmentError("service-1", saveErr); !errors.Is(got, saveErr) {
		t.Fatalf("appointment save error was lost: %v", got)
	}
	if got := budgetSchedulePersistenceError("service-1", true, nil); got != nil {
		t.Fatalf("saved service and appointment returned error: %v", got)
	}
	if got := budgetSchedulePersistenceError("service-1", false, nil); got == nil {
		t.Fatal("missing appointment must not be treated as a fully persisted schedule")
	}
}

func TestBudgetScheduleAppointmentMethodHandlesLinkedServiceWithoutAppointment(t *testing.T) {
	if got := budgetScheduleAppointmentMethod(true, true); got != "PATCH" {
		t.Fatalf("existing appointment method=%q, want PATCH", got)
	}
	if got := budgetScheduleAppointmentMethod(true, false); got != "POST" {
		t.Fatalf("missing linked-service appointment method=%q, want POST", got)
	}
	if got := budgetScheduleAppointmentMethod(false, false); got != "POST" {
		t.Fatalf("new service appointment method=%q, want POST", got)
	}
}
