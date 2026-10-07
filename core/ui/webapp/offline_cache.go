package webapp

import (
	"encoding/json"
	"errors"
	"maps"
	"sync"
	"time"

	"inovarapp/core/domain"
)

var offlineSnapshotMu sync.Mutex

// offlineAccountSnapshot contains only the latest read model for one account.
// It never stores access/refresh tokens or integration secrets.
type offlineAccountSnapshot struct {
	ID               string                     `json:"id"`
	UserID           string                     `json:"userId"`
	Role             domain.Role                `json:"role"`
	SavedAt          string                     `json:"savedAt"`
	TeamServices     []map[string]any           `json:"teamServices,omitempty"`
	TeamAppointments []map[string]any           `json:"teamAppointments,omitempty"`
	TeamCustomers    []map[string]any           `json:"teamCustomers,omitempty"`
	TeamBudgets      []domain.BudgetEstimate    `json:"teamBudgets,omitempty"`
	TeamHistory      []map[string]any           `json:"teamHistory,omitempty"`
	TeamProfile      *domain.TechnicianProfile  `json:"teamProfile,omitempty"`
	PortalData       *domain.CustomerPortalData `json:"portalData,omitempty"`
	PendingCustomers []offlineTeamCustomer      `json:"pendingCustomers,omitempty"`
	PendingServices  []offlineTeamService       `json:"pendingServices,omitempty"`
	PendingBudgets   []domain.BudgetEstimate    `json:"pendingBudgets,omitempty"`
	PendingSettings  map[string]json.RawMessage `json:"pendingSettings,omitempty"`
	PendingMutations []offlineTeamMutation      `json:"pendingMutations,omitempty"`
}

type offlineTeamCustomer struct {
	CustomerID  string         `json:"customerId"`
	ApplianceID string         `json:"applianceId"`
	Customer    map[string]any `json:"customer"`
	Appliance   map[string]any `json:"appliance"`
}

type offlineTeamService struct {
	ServiceID   string         `json:"serviceId"`
	Service     map[string]any `json:"service"`
	Appointment map[string]any `json:"appointment,omitempty"`
}

func newOfflineAccountSnapshot(userID string, role domain.Role) offlineAccountSnapshot {
	return offlineAccountSnapshot{ID: userID, UserID: userID, Role: role, SavedAt: time.Now().UTC().Format(time.RFC3339Nano)}
}

func (s offlineAccountSnapshot) validFor(userID string) bool {
	if userID == "" || s.UserID != userID || s.ID != userID || s.SavedAt == "" {
		return false
	}
	return s.Role == domain.RoleCustomer || s.Role == domain.RoleAdmin || s.Role == domain.RoleTechnician
}

func decodeOfflineAccountSnapshot(data []byte, userID string) (offlineAccountSnapshot, bool) {
	var snapshot offlineAccountSnapshot
	if json.Unmarshal(data, &snapshot) != nil || !snapshot.validFor(userID) {
		return offlineAccountSnapshot{}, false
	}
	return snapshot, true
}

func offlineProfile(profile domain.TechnicianProfile) *domain.TechnicianProfile {
	profile.CalendarToken = ""
	return &profile
}

func cacheTeamSnapshot(userID string, role domain.Role, services, appointments, customers, history []map[string]any, budgets []domain.BudgetEstimate, profile domain.TechnicianProfile) {
	offlineSnapshotMu.Lock()
	defer offlineSnapshotMu.Unlock()
	snapshot, err := loadOfflineSnapshot(userID)
	if err != nil {
		snapshot = newOfflineAccountSnapshot(userID, role)
	}
	snapshot.Role, snapshot.SavedAt = role, time.Now().UTC().Format(time.RFC3339Nano)
	snapshot.TeamServices, snapshot.TeamAppointments = mergePendingOfflineServices(services, appointments, snapshot.PendingServices)
	snapshot.TeamCustomers = mergePendingOfflineCustomers(customers, snapshot.PendingCustomers)
	snapshot.TeamBudgets = mergePendingOfflineBudgets(budgets, snapshot.PendingBudgets)
	snapshot.TeamHistory, snapshot.TeamProfile = history, offlineProfile(profile)
	if len(snapshot.PendingSettings) > 0 {
		snapshot.TeamProfile = mergeOfflineTeamProfile(snapshot.TeamProfile, snapshot.PendingSettings)
	}
	applyPendingOfflineTeamMutations(&snapshot)
	_ = saveOfflineSnapshot(snapshot)
}

func mergeOfflineTeamProfile(profile *domain.TechnicianProfile, pending map[string]json.RawMessage) *domain.TechnicianProfile {
	if profile == nil || len(pending) == 0 {
		return profile
	}
	encoded, err := json.Marshal(profile)
	if err != nil {
		return profile
	}
	fields := map[string]json.RawMessage{}
	if json.Unmarshal(encoded, &fields) != nil {
		return profile
	}
	for key, value := range pending {
		fields[key] = value
	}
	encoded, err = json.Marshal(fields)
	if err != nil {
		return profile
	}
	var merged domain.TechnicianProfile
	if json.Unmarshal(encoded, &merged) != nil {
		return profile
	}
	return &merged
}

func mergePendingOfflineCustomers(remote []map[string]any, pending []offlineTeamCustomer) []map[string]any {
	merged := stripOfflineIdentityFields(remote)
	for _, queued := range pending {
		local := stripOfflineIdentityFields([]map[string]any{queued.Customer})[0]
		customer := findOfflineCustomer(merged, queued.CustomerID)
		if customer == nil {
			merged = append(merged, local)
			continue
		}
		if queued.ApplianceID == "" || queued.Appliance == nil {
			continue
		}
		if offlineCustomerHasAppliance(customer, queued.ApplianceID) {
			continue
		}
		copy := make(map[string]any, len(customer)+1)
		for key, value := range customer {
			copy[key] = value
		}
		appliances, _ := copy["appliances"].([]any)
		appliances = append(appliances, cloneAnyMap(queued.Appliance))
		copy["appliances"] = appliances
		for index := range merged {
			if portalText(merged[index]["id"]) == queued.CustomerID {
				merged[index] = copy
				break
			}
		}
	}
	return merged
}

func stripOfflineIdentityFields(customers []map[string]any) []map[string]any {
	result := make([]map[string]any, 0, len(customers))
	for _, customer := range customers {
		copy := make(map[string]any, len(customer))
		for key, value := range customer {
			if key != "profile_id" && key != "user_id" {
				copy[key] = value
			}
		}
		result = append(result, copy)
	}
	return result
}

func cachePortalSnapshot(userID string, role domain.Role, portal *domain.CustomerPortalData) {
	offlineSnapshotMu.Lock()
	defer offlineSnapshotMu.Unlock()
	snapshot, err := loadOfflineSnapshot(userID)
	if err != nil {
		snapshot = newOfflineAccountSnapshot(userID, role)
	}
	snapshot.Role, snapshot.SavedAt, snapshot.PortalData = role, time.Now().UTC().Format(time.RFC3339Nano), portal
	_ = saveOfflineSnapshot(snapshot)
}

func hasPendingOfflineTeamMutations(userID string) bool {
	offlineSnapshotMu.Lock()
	defer offlineSnapshotMu.Unlock()
	snapshot, err := loadOfflineSnapshot(userID)
	return err == nil && (len(snapshot.PendingCustomers) > 0 || len(snapshot.PendingServices) > 0 || len(snapshot.PendingBudgets) > 0 || len(snapshot.PendingSettings) > 0 || len(snapshot.PendingMutations) > 0)
}

func offlineSettingsFields() map[string]bool {
	return map[string]bool{
		"name": true, "businessName": true, "cnpj": true, "address": true, "phone": true,
		"pixKey": true, "pixType": true, "defaultReturnMonths": true, "defaultWarrantyDays": true,
		"defaultPrice": true, "tiposServicosCustom": true, "mensagensWhats": true,
		"lembrete_intervalo_dias": true, "tiposFixosRemovidos": true, "tiposFixosEditados": true,
	}
}

func sanitizeOfflineSettings(payload map[string]any) (map[string]json.RawMessage, error) {
	allowed := offlineSettingsFields()
	result := make(map[string]json.RawMessage, len(payload))
	for key, value := range payload {
		if !allowed[key] {
			continue
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		result[key] = encoded
	}
	if len(result) == 0 {
		return nil, errors.New("no offline-safe settings to persist")
	}
	return result, nil
}

func enqueueOfflineTeamSettings(userID string, role domain.Role, profile domain.TechnicianProfile, payload map[string]any) error {
	pending, err := sanitizeOfflineSettings(payload)
	if err != nil {
		return err
	}
	offlineSnapshotMu.Lock()
	defer offlineSnapshotMu.Unlock()
	snapshot, err := loadOfflineSnapshot(userID)
	if err != nil {
		snapshot = newOfflineAccountSnapshot(userID, role)
	}
	if snapshot.Role != role {
		return errors.New("offline settings role mismatch")
	}
	if snapshot.PendingSettings == nil {
		snapshot.PendingSettings = map[string]json.RawMessage{}
	}
	maps.Copy(snapshot.PendingSettings, pending)
	if snapshot.TeamProfile == nil {
		snapshot.TeamProfile = offlineProfile(profile)
	}
	snapshot.TeamProfile = mergeOfflineTeamProfile(snapshot.TeamProfile, pending)
	snapshot.SavedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return saveOfflineSnapshot(snapshot)
}
