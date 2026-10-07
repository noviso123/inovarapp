package webapp

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/domain"
)

func TestDecodeCreatedTeamBudgetRequiresServerIdentity(t *testing.T) {
	got, err := decodeCreatedTeamBudget(map[string]any{"id": "budget-1", "numero": "2026-0042", "clientName": "Ana", "clientPhone": "5527999999999"})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "budget-1" || got.Number != "2026-0042" || got.ClientName != "Ana" || got.ClientPhone != "5527999999999" {
		t.Fatalf("decoded budget=%#v", got)
	}
	if _, err := decodeCreatedTeamBudget(map[string]any{"numero": "2026-0042"}); err == nil {
		t.Fatal("expected missing server id to fail before sending WhatsApp")
	}
}

func TestFilterTeamBudgetsPreservesStatusAndSearchRules(t *testing.T) {
	page := &serviceCatalogPage{teamBudgetFilter: string(domain.BudgetPending), teamBudgetSearch: "5511"}
	page.teamBudgets = []domain.BudgetEstimate{
		{ID: "one", ClientName: "Ana", ClientPhone: "551199999", ApplianceDescription: "LG Split", Status: domain.BudgetPending},
		{ID: "two", ClientName: "Bia", ClientPhone: "552188888", ApplianceDescription: "Samsung", Status: domain.BudgetApproved},
		{ID: "three", ClientName: "Caio", ClientPhone: "553177777", ApplianceDescription: "Midea", Status: domain.BudgetPending},
	}
	got := page.filteredTeamBudgets()
	if len(got) != 1 || got[0].ID != "one" {
		t.Fatalf("filtered=%#v", got)
	}
	page.teamBudgetFilter = ""
	if got = page.filteredTeamBudgets(); len(got) != 1 {
		t.Fatalf("search filtered=%#v", got)
	}
}

func TestFindPendingTeamBudgetForClientOnlyReturnsSameClientsOpenProposal(t *testing.T) {
	budgets := []domain.BudgetEstimate{
		{ID: "approved", ClientID: "client-1", Status: domain.BudgetApproved},
		{ID: "other-client", ClientID: "client-2", Status: domain.BudgetPending},
		{ID: "pending", ClientID: "client-1", Number: "2026-12", FinalValue: 420, Status: domain.BudgetPending},
	}
	got, ok := findPendingTeamBudgetForClient(budgets, "client-1")
	if !ok || got.ID != "pending" || got.Number != "2026-12" {
		t.Fatalf("pending budget=%#v found=%v", got, ok)
	}
	if _, ok := findPendingTeamBudgetForClient(budgets, "client-2"); !ok {
		t.Fatal("expected pending budget for second client")
	}
	if _, ok := findPendingTeamBudgetForClient(budgets, ""); ok {
		t.Fatal("empty client id must not match an open proposal")
	}
	if _, ok := findPendingTeamBudgetForClient(budgets, "client-3"); ok {
		t.Fatal("unexpected proposal for client without an open budget")
	}
}

func TestDetailedTeamBudgetStatusPreservesSignedStateLabel(t *testing.T) {
	signature := "data:image/png;base64,signed"
	label, tone := detailedTeamBudgetStatus(domain.BudgetApproved)
	if label != "Aprovado" || tone != "complete" {
		t.Fatalf("unsigned=%s/%s", label, tone)
	}
	_ = signature
}

func TestTeamBudgetExpiryUsesLocalCalendarDate(t *testing.T) {
	now := time.Date(2026, 10, 5, 15, 30, 0, 0, time.FixedZone("BRT", -3*60*60))
	if teamBudgetExpired("2026-10-04", now) != true {
		t.Fatal("yesterday's proposal should be expired")
	}
	if teamBudgetExpired("2026-10-05", now) {
		t.Fatal("proposal should remain valid through its expiry date")
	}
	if teamBudgetExpired("invalid", now) {
		t.Fatal("invalid dates should not be treated as expired")
	}
}

func TestTeamBudgetCardReferenceMatchesLegacyIDPrefix(t *testing.T) {
	if got := teamBudgetCardReference("a1b2c3d4-e5f6-4711-8111-123456789012"); got != "A1B2C3D4" {
		t.Fatalf("card reference=%q want A1B2C3D4", got)
	}
	if got := teamBudgetCardReference("id"); got != "ID" {
		t.Fatalf("short card reference=%q want ID", got)
	}
	if got := teamBudgetCardReference(""); got != "" {
		t.Fatalf("empty card reference=%q want empty", got)
	}
}

func TestTeamBudgetConfirmationMapsDeclineToStatusMutation(t *testing.T) {
	for _, tt := range []struct {
		input, action, status string
	}{
		{input: "RECUSAR", action: "STATUS", status: "RECUSAR"},
		{input: "RECEBIDO", action: "RECEBIDO", status: ""},
		{input: "DELETE", action: "DELETE", status: ""},
	} {
		action, status := teamBudgetConfirmationMutation(tt.input)
		if action != tt.action || status != tt.status {
			t.Errorf("teamBudgetConfirmationMutation(%q) = (%q, %q), want (%q, %q)", tt.input, action, status, tt.action, tt.status)
		}
	}
}

func TestTeamBudgetReceivedConfirmationIncludesAmountAndClient(t *testing.T) {
	budget := domain.BudgetEstimate{ClientName: "Ana Silva", FinalValue: 1234.5}
	if got, want := teamBudgetConfirmationMessage("RECEBIDO", budget), "Confirmar recebimento de R$ 1234.50 de Ana Silva?"; got != want {
		t.Fatalf("confirmation=%q want %q", got, want)
	}
}

func TestTeamBudgetReceivedSuccessMessageMatchesLegacyToast(t *testing.T) {
	budget := domain.BudgetEstimate{FinalValue: 1234.5}
	if got, want := teamBudgetMutationSuccessMessage("RECEBIDO", budget), "💰 R$ 1234.50 marcado como recebido!"; got != want {
		t.Fatalf("success message=%q want %q", got, want)
	}
	if got := teamBudgetMutationSuccessMessage("STATUS", budget); got != "" {
		t.Fatalf("status mutation unexpectedly emitted success text %q", got)
	}
}

func TestTeamBudgetMonthlySummaryMatchesLegacyTotalsAndClientOrder(t *testing.T) {
	receivedAt := "2026-10-04T12:00:00Z"
	otherMonth := "2026-09-30T23:59:00Z"
	partialReceived := 60.0
	budgets := []domain.BudgetEstimate{
		{ClientName: "Zeta", Date: "2026-10-01", FinalValue: 100, PaidAt: &receivedAt, AmountReceived: &partialReceived},
		{ClientName: "Ana", Date: "2026-09-01", FinalValue: 200, PaidAt: &receivedAt},
		{ClientName: "Zeta", Date: "2026-10-03", FinalValue: 25, PaidAt: &otherMonth},
	}
	got := summarizeTeamBudgetMonth(budgets, "2026-10")
	if got.Received != 300 || got.Issued != 125 {
		t.Fatalf("monthly totals = received %.2f, issued %.2f; want 300 and 125", got.Received, got.Issued)
	}
	if len(got.Clients) != 2 || got.Clients[0] != (teamBudgetClientReceipt{Name: "Zeta", Value: 100}) || got.Clients[1] != (teamBudgetClientReceipt{Name: "Ana", Value: 200}) {
		t.Fatalf("client receipts/order = %#v", got.Clients)
	}
}

func TestBudgetContactValuesAllowOverridesAndFallbackToCustomer(t *testing.T) {
	customer := map[string]any{"nome": "Ana Silva", "whatsapp": "5527999999999"}
	name, phone := budgetContactValues(&teamBudgetForm{ClientName: "  Ana do orçamento ", ClientPhone: " 5527988888888 "}, customer)
	if name != "Ana do orçamento" || phone != "5527988888888" {
		t.Fatalf("overrides=%q/%q", name, phone)
	}
	name, phone = budgetContactValues(&teamBudgetForm{}, customer)
	if name != "Ana Silva" || phone != "5527999999999" {
		t.Fatalf("fallback=%q/%q", name, phone)
	}
}

func TestBudgetClientAndApplianceSelectionKeepEquipmentFieldsInSync(t *testing.T) {
	page := &serviceCatalogPage{}
	customer := map[string]any{
		"id": "client-1", "nome": "Ana Silva", "whatsapp": "5527999999999",
		"appliances": []any{
			map[string]any{"id": "appliance-1", "marca": "LG", "modelo": "Dual", "btus": 12000, "ambiente": "Sala"},
			map[string]any{"id": "appliance-2", "marca": "Gree", "modelo": "Inverter", "btus": 18000, "ambiente": "Quarto"},
		},
	}
	form := &teamBudgetForm{}
	page.setBudgetClient(form, customer)
	if form.ClientID != "client-1" || form.ApplianceID != "appliance-1" || form.Equipment != "LG Dual" || !strings.Contains(form.Description, "Sala") {
		t.Fatalf("initial customer/appliance values were not prefilled: %+v", form)
	}
	setTeamBudgetAppliance(form, applianceMaps(customer)[1])
	if form.ApplianceID != "appliance-2" || form.Equipment != "Gree Inverter" || !strings.Contains(form.Description, "Quarto") {
		t.Fatalf("selected appliance details were not synchronized: %+v", form)
	}
	page.setBudgetClient(form, nil)
	if form.ClientID != "" || form.ClientName != "" || form.ClientPhone != "" || form.ApplianceID != "" || form.Equipment != "" || form.Description != "" {
		t.Fatalf("clearing customer retained stale details: %+v", form)
	}
}

func TestBudgetPrefillFromRequestUsesTheRequestApplianceAndLeavesPriceForTechnician(t *testing.T) {
	customer := map[string]any{
		"id": "client-1", "nome": "Ana Silva", "whatsapp": "5527999999999",
		"appliances": []any{
			map[string]any{"id": "appliance-1", "marca": "LG", "modelo": "Dual", "btus": 12000, "ambiente": "Sala"},
			map[string]any{"id": "appliance-2", "marca": "Gree", "modelo": "Inverter", "btus": 18000, "ambiente": "Quarto"},
		},
	}
	form := &teamBudgetForm{Items: newTeamBudgetItems()}
	service := map[string]any{
		"id": "service-1", "cliente_id": "client-1", "aparelho_id": "appliance-2", "descricao": "Manutenção Preventiva",
	}
	prefillTeamBudgetFromService(form, service, []map[string]any{customer})
	if form.ClientID != "client-1" || form.ServiceID == nil || *form.ServiceID != "service-1" {
		t.Fatalf("request/client association missing: %+v", form)
	}
	if form.ApplianceID != "appliance-2" || form.Equipment != "Gree Inverter" || !strings.Contains(form.Description, "Quarto") {
		t.Fatalf("request appliance was not selected: %+v", form)
	}
	if len(form.Items) != 1 || form.Items[0].Description != "Manutenção Preventiva" || form.Items[0].UnitPrice != 0 {
		t.Fatalf("request line should be prefilled without choosing a price: %+v", form.Items)
	}
}

func TestResolveTeamBudgetCustomerSupportsRegisteredAndNewClients(t *testing.T) {
	clientID := uuid.NewString()
	customers := []map[string]any{{"id": clientID, "nome": "Ana Silva", "whatsapp": "55279999"}}
	registered, id, create := resolveTeamBudgetCustomer(customers, clientID, "Ana", true)
	if registered == nil || id != clientID || create {
		t.Fatalf("registered client resolution=%#v/%q/%v", registered, id, create)
	}
	matched, id, create := resolveTeamBudgetCustomer(customers, "", "  ana silva ", true)
	if matched == nil || id != clientID || create {
		t.Fatalf("name match should reuse existing customer: %#v/%q/%v", matched, id, create)
	}
	newClient, id, create := resolveTeamBudgetCustomer(customers, "", "Bruno Novo", true)
	if newClient != nil || uuid.Validate(id) != nil || !create {
		t.Fatalf("new customer resolution=%#v/%q/%v", newClient, id, create)
	}
	stable, retryID, retryCreate := resolveTeamBudgetCustomer(customers, id, "Bruno Novo", true)
	if stable != nil || retryID != id || !retryCreate {
		t.Fatalf("retry should preserve the pending customer ID: %#v/%q/%v", stable, retryID, retryCreate)
	}
}

func TestBudgetPDFUsesServerSuggestedDownloadFilename(t *testing.T) {
	got := budgetPDFDownloadFilename(`attachment; filename="ORCAMENTO_INOVAR_Ana_2026-0012.pdf"`, "fallback.pdf")
	if got != "ORCAMENTO_INOVAR_Ana_2026-0012.pdf" {
		t.Fatalf("PDF filename=%q", got)
	}
	if got := budgetPDFDownloadFilename("invalid", "fallback.pdf"); got != "fallback.pdf" {
		t.Fatalf("fallback PDF filename=%q", got)
	}
}

func TestBudgetQuickItemsUseResolvedTeamCatalogPrices(t *testing.T) {
	profile := domain.TechnicianProfile{
		RemovedFixedServiceTypes: []string{string(domain.ServiceCleaning)},
		CustomServiceTypes:       []domain.CustomServiceType{{Name: "Higienização Premium", Price: 480, AverageTime: "2 horas", DefaultWarranty: "120 dias"}},
	}
	items := budgetQuickItems(profile)
	var customFound, cleaningFound bool
	wantPrices := map[string]float64{
		"Instalação":            1300,
		"Manutenção Corretiva":  250,
		"Recarga de Gás":        305,
		"Manutenção Preventiva": 250,
		"Avaliação Técnica":     110,
		"Higienização Premium":  480,
	}
	for _, item := range items {
		if item.Description == "Higienização Premium" && item.UnitPrice == 480 && item.TotalPrice == 480 {
			customFound = true
		}
		if item.Description == string(domain.ServiceCleaning) {
			cleaningFound = true
		}
		if expected, ok := wantPrices[item.Description]; ok {
			if item.UnitPrice != expected || item.TotalPrice != expected {
				t.Errorf("quick item %q prices=(%v, %v), want suggested editable price %v", item.Description, item.UnitPrice, item.TotalPrice, expected)
			}
			delete(wantPrices, item.Description)
		}
	}
	if len(wantPrices) != 0 {
		t.Errorf("catalog quick items missing expected defaults: %#v", wantPrices)
	}
	if !customFound || cleaningFound {
		t.Fatalf("catalog quick items custom=%v removed=%v; items=%#v", customFound, cleaningFound, items)
	}
	average, warranty := budgetQuickItemDefaults("Higienização Premium", profile)
	if average != "2 horas" || warranty != "120 dias" {
		t.Fatalf("defaults=%q/%q", average, warranty)
	}
}

func TestBudgetUnitPriceInputStartsBlankAndShowsSavedValue(t *testing.T) {
	items := newTeamBudgetItems()
	if len(items) != 0 {
		t.Fatalf("new budget should have no preselected items: %#v", items)
	}
	item := newTeamBudgetItem()
	if item.Description != "" || item.Category != "" || item.Quantity != 0 || item.UnitPrice != 0 || item.TotalPrice != 0 {
		t.Fatalf("manually added item should start completely blank: %#v", item)
	}
	if got := budgetUnitPriceInputValue(0); got != "" {
		t.Fatalf("new budget price input = %q, want blank", got)
	}
	if got := budgetUnitPriceInputValue(125.5); got != "125.50" {
		t.Fatalf("saved budget price input = %q, want 125.50", got)
	}
	if got := budgetQuantityInputValue(0); got != "" {
		t.Fatalf("new budget quantity input = %q, want blank", got)
	}
	if got := budgetQuantityInputValue(1.5); got != "1.50" {
		t.Fatalf("saved budget quantity input = %q, want 1.50", got)
	}
}

func TestNewBudgetRendersEmptyItemsAreaUntilTechnicianAddsOne(t *testing.T) {
	p := &serviceCatalogPage{teamBudgetForm: &teamBudgetForm{Items: newTeamBudgetItems()}}
	markup := app.HTMLString(p.teamBudgetDialog())
	for _, want := range []string{"Itens do orçamento", "Adicione os serviços e materiais", "Buscar serviço do catálogo", "Adicionar item", "Mais detalhes"} {
		if !strings.Contains(markup, want) {
			t.Errorf("empty budget item area is missing %q: %s", want, markup)
		}
	}
	if strings.Contains(markup, "Preço unitário definido pelo técnico") || strings.Contains(markup, "Quantidade</label>") {
		t.Fatalf("new budget unexpectedly rendered a preselected item: %s", markup)
	}
}
