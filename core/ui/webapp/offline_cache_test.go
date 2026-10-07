package webapp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"inovarapp/core/domain"
)

func TestOfflineSnapshotIsScopedToAuthenticatedAccountAndRole(t *testing.T) {
	snapshot := newOfflineAccountSnapshot("user-1", domain.RoleAdmin)
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := decodeOfflineAccountSnapshot(encoded, "user-1"); !ok {
		t.Fatal("valid account snapshot was rejected")
	}
	if _, ok := decodeOfflineAccountSnapshot(encoded, "user-2"); ok {
		t.Fatal("snapshot was readable under a different account")
	}
	snapshot.Role = domain.Role("UNKNOWN")
	encoded, _ = json.Marshal(snapshot)
	if _, ok := decodeOfflineAccountSnapshot(encoded, "user-1"); ok {
		t.Fatal("snapshot with an unsupported cached role was accepted")
	}
}

func TestOfflineProfileDoesNotPersistCalendarToken(t *testing.T) {
	profile := domain.TechnicianProfile{CalendarToken: "calendar-secret", Name: "Técnica"}
	cached := offlineProfile(profile)
	if cached == nil || cached.CalendarToken != "" || cached.Name != "Técnica" {
		t.Fatalf("unexpected cached profile: %#v", cached)
	}
	encoded, err := json.Marshal(cached)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "" || strings.Contains(string(encoded), "calendar-secret") {
		t.Fatalf("cached profile appears to contain a secret: %s", encoded)
	}
}

func TestOfflineSettingsAllowOnlyNonSecretProfileAndMessageFields(t *testing.T) {
	settings, err := sanitizeOfflineSettings(map[string]any{
		"name": "Técnica", "mensagensWhats": map[string]string{"concluido": "Olá {cliente}"},
		"lembrete_intervalo_dias": 14, "whatsapp_proprio_token": "secret", "email_gmail_pass": "secret", "calendario_token": "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(settings) != 3 || settings["name"] == nil || settings["mensagensWhats"] == nil || settings["lembrete_intervalo_dias"] == nil {
		t.Fatalf("safe pending settings missing or secret fields accepted: %#v", settings)
	}
	for _, key := range []string{"whatsapp_proprio_token", "email_gmail_pass", "calendario_token"} {
		if settings[key] != nil {
			t.Fatalf("sensitive setting %q entered offline queue", key)
		}
	}
}

func TestPendingOfflineSettingsOverlayRefreshedProfile(t *testing.T) {
	profile := offlineProfile(domain.TechnicianProfile{Name: "Old", DefaultReturnMonths: 6})
	merged := mergeOfflineTeamProfile(profile, map[string]json.RawMessage{
		"name": json.RawMessage(`"Novo nome"`), "defaultReturnMonths": json.RawMessage(`12`),
		"mensagensWhats": json.RawMessage(`{"concluido":"Atendimento finalizado para {cliente}"}`),
	})
	if merged.Name != "Novo nome" || merged.DefaultReturnMonths != 12 || merged.WhatsAppMessages["concluido"] == "" {
		t.Fatalf("pending settings were not overlaid: %+v", merged)
	}
}

func TestOfflineMutationAllowlistAndSecretFilter(t *testing.T) {
	for _, tc := range []struct {
		path, method string
		want         bool
	}{
		{"/api/clientes", "PATCH", true},
		{"/api/servicos", "DELETE", true},
		{"/api/agendamentos", "POST", false},
		{"/api/whatsapp", "POST", false},
		{"/api/contas", "DELETE", false},
	} {
		if got := offlineTeamMutationSupported(tc.path, tc.method); got != tc.want {
			t.Errorf("offlineTeamMutationSupported(%q, %q)=%v want %v", tc.path, tc.method, got, tc.want)
		}
	}
	if containsOfflineSecretField([]byte(`{"fields":{"name":"Ana"}}`)) {
		t.Fatal("ordinary profile data rejected")
	}
	for _, payload := range []string{`{"password":"secret"}`, `{"provider_token":"secret"}`, `{"api-key":"secret"}`} {
		if !containsOfflineSecretField([]byte(payload)) {
			t.Errorf("secret field accepted into the offline mutation queue: %s", payload)
		}
	}
}

func TestPendingOfflineMutationsProjectUpdatesDeletesAndBudgetStatus(t *testing.T) {
	serviceID, customerID, budgetID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	snapshot := offlineAccountSnapshot{
		TeamCustomers:    []map[string]any{{"id": customerID, "nome": "Ana", "appliances": []any{map[string]any{"id": "device-1", "marca": "LG"}}}},
		TeamServices:     []map[string]any{{"id": serviceID, "status": "AGENDADO", "valor": 100}},
		TeamAppointments: []map[string]any{{"service_id": serviceID, "data": "2026-10-05"}},
		TeamBudgets:      []domain.BudgetEstimate{{ID: budgetID, Status: domain.BudgetPending}},
		PendingMutations: []offlineTeamMutation{
			{Method: "PATCH", Path: "/api/clientes", Payload: json.RawMessage(`{"id":"` + customerID + `","fields":{"nome":"Ana Atualizada"}}`)},
			{Method: "PATCH", Path: "/api/servicos", Payload: json.RawMessage(`{"id":"` + serviceID + `","fields":{"valor":250}}`)},
			{Method: "PATCH", Path: "/api/agendamentos", Payload: json.RawMessage(`{"service_id":"` + serviceID + `","fields":{"data":"2026-10-12"}}`)},
			{Method: "PATCH", Path: "/api/orcamento-equipe?id=" + budgetID, Payload: json.RawMessage(`{"id":"` + budgetID + `","acao":"STATUS","status":"APROVAR"}`)},
			{Method: "DELETE", Path: "/api/servicos", Payload: json.RawMessage(`{"id":"` + serviceID + `"}`)},
		},
	}
	applyPendingOfflineTeamMutations(&snapshot)
	if len(snapshot.TeamCustomers) != 1 || portalText(snapshot.TeamCustomers[0]["nome"]) != "Ana Atualizada" {
		t.Fatalf("customer patch not projected: %#v", snapshot.TeamCustomers)
	}
	if len(snapshot.TeamServices) != 0 {
		t.Fatalf("service delete not projected: %#v", snapshot.TeamServices)
	}
	if len(snapshot.TeamAppointments) != 1 || portalText(snapshot.TeamAppointments[0]["data"]) != "2026-10-12" {
		t.Fatalf("appointment patch not projected: %#v", snapshot.TeamAppointments)
	}
	if len(snapshot.TeamBudgets) != 1 || snapshot.TeamBudgets[0].Status != domain.BudgetApproved {
		t.Fatalf("budget state patch not projected: %#v", snapshot.TeamBudgets)
	}
}

func TestOfflineCustomerSnapshotDropsProfileLinksWithoutMutatingViewModel(t *testing.T) {
	customers := []map[string]any{{"id": "customer-1", "profile_id": "auth-user-1", "user_id": "auth-user-1", "nome": "Ana"}}
	cached := stripOfflineIdentityFields(customers)
	if customers[0]["profile_id"] != "auth-user-1" {
		t.Fatal("cache sanitizer mutated the live view model")
	}
	if _, ok := cached[0]["profile_id"]; ok {
		t.Fatalf("offline snapshot retained profile_id: %#v", cached[0])
	}
	if _, ok := cached[0]["user_id"]; ok {
		t.Fatalf("offline snapshot retained user_id: %#v", cached[0])
	}
	if cached[0]["nome"] != "Ana" {
		t.Fatalf("offline snapshot dropped customer data: %#v", cached[0])
	}
}

func TestOfflineCustomerMutationUsesStableUUIDsAndNestedEquipment(t *testing.T) {
	customerID, applianceID := newOfflineTeamCustomerIDs()
	if uuid.Validate(customerID) != nil || uuid.Validate(applianceID) != nil || customerID == applianceID {
		t.Fatalf("invalid/non-unique offline IDs: customer=%q appliance=%q", customerID, applianceID)
	}
	customer := map[string]any{"id": customerID, "appliances": []any{map[string]any{"id": applianceID}}}
	if !offlineCustomerHasAppliance(customer, applianceID) || offlineCustomerHasAppliance(customer, "different") {
		t.Fatalf("appliance membership check failed for local snapshot: %#v", customer)
	}
}

func TestPendingOfflineCustomerSurvivesRemoteSnapshotRefreshAndPartialApplianceSync(t *testing.T) {
	remote := []map[string]any{{
		"id": "customer-1", "nome": "Ana", "appliances": []any{map[string]any{"id": "existing-appliance"}},
	}}
	queued := []offlineTeamCustomer{{
		CustomerID: "customer-1", ApplianceID: "pending-appliance",
		Customer:  map[string]any{"id": "customer-1", "nome": "Ana", "profile_id": "must-not-cache", "appliances": []any{map[string]any{"id": "pending-appliance"}}},
		Appliance: map[string]any{"id": "pending-appliance", "marca": "LG"},
	}}
	merged := mergePendingOfflineCustomers(remote, queued)
	if len(merged) != 1 || !offlineCustomerHasAppliance(merged[0], "existing-appliance") || !offlineCustomerHasAppliance(merged[0], "pending-appliance") {
		t.Fatalf("partial offline customer disappeared after refresh: %#v", merged)
	}
	if _, exists := merged[0]["profile_id"]; exists {
		t.Fatalf("account link leaked into the offline view: %#v", merged[0])
	}
	if _, exists := remote[0]["profile_id"]; exists {
		t.Fatalf("merge mutated remote view data: %#v", remote[0])
	}
}

func TestPendingOfflineBudgetCustomerWithoutApplianceMergesWithoutInventingEquipment(t *testing.T) {
	customerID := uuid.NewString()
	queued := []offlineTeamCustomer{{CustomerID: customerID, Customer: map[string]any{
		"id": customerID, "nome": "Cliente de orçamento avulso", "whatsapp": "5527999999999", "appliances": []any{},
	}}}
	merged := mergePendingOfflineCustomers(nil, queued)
	if len(merged) != 1 || portalText(merged[0]["id"]) != customerID || portalText(merged[0]["nome"]) != "Cliente de orçamento avulso" {
		t.Fatalf("offline budget customer missing: %#v", merged)
	}
	if offlineCustomerHasAppliance(merged[0], "any") {
		t.Fatalf("a customer created for an ad-hoc budget gained a fake appliance: %#v", merged[0])
	}
}

func TestPendingOfflineServiceSurvivesRemoteRefreshWithItsAppointment(t *testing.T) {
	id := uuid.NewString()
	queued := []offlineTeamService{{ServiceID: id, Service: map[string]any{"id": id, "status": "AGENDADO"}, Appointment: map[string]any{"service_id": id, "data": "2026-10-06"}}}
	services, appointments := mergePendingOfflineServices(nil, nil, queued)
	if findOfflineService(services, id) == nil || !teamDirectAppointmentExists(appointments, id) {
		t.Fatalf("pending service/appointment disappeared: services=%#v appointments=%#v", services, appointments)
	}
	services, appointments = mergePendingOfflineServices([]map[string]any{{"id": id, "status": "AGENDADO"}}, nil, queued)
	if len(services) != 1 || len(appointments) != 1 {
		t.Fatalf("partial remote sync was not reconciled: services=%#v appointments=%#v", services, appointments)
	}
}

func TestPendingOfflineBudgetSurvivesRemoteRefreshAndDeduplicatesByStableID(t *testing.T) {
	id := uuid.NewString()
	queued := []domain.BudgetEstimate{{ID: id, ClientID: uuid.NewString(), ClientName: "Ana", Status: domain.BudgetPending}}
	merged := mergePendingOfflineBudgets(nil, queued)
	if len(merged) != 1 || merged[0].ID != id {
		t.Fatalf("pending budget missing: %#v", merged)
	}
	merged = mergePendingOfflineBudgets([]domain.BudgetEstimate{{ID: id, Number: "2026-0042"}}, queued)
	if len(merged) != 1 || merged[0].Number != "2026-0042" {
		t.Fatalf("remote budget was duplicated or overwritten: %#v", merged)
	}
}
