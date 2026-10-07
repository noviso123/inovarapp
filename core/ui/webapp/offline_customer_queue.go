package webapp

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"inovarapp/core/domain"
)

func enqueueOfflineTeamCustomer(userID string, role domain.Role, customer, appliance map[string]any) error {
	offlineSnapshotMu.Lock()
	defer offlineSnapshotMu.Unlock()
	snapshot, err := loadOfflineSnapshot(userID)
	if err != nil {
		return err
	}
	if snapshot.Role != role {
		return errors.New("offline snapshot role does not match")
	}
	customerID, _ := customer["id"].(string)
	applianceID, _ := appliance["id"].(string)
	if uuid.Validate(customerID) != nil || (applianceID != "" && (uuid.Validate(applianceID) != nil || portalText(appliance["cliente_id"]) != customerID)) {
		return errors.New("offline customer identifiers are missing")
	}
	for _, pending := range snapshot.PendingCustomers {
		if pending.CustomerID == customerID {
			return nil
		}
	}
	snapshot.PendingCustomers = append(snapshot.PendingCustomers, offlineTeamCustomer{CustomerID: customerID, ApplianceID: applianceID, Customer: customer, Appliance: appliance})
	snapshot.TeamCustomers = append(snapshot.TeamCustomers, stripOfflineIdentityFields([]map[string]any{customer})[0])
	snapshot.SavedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return saveOfflineSnapshot(snapshot)
}

// syncOfflineTeamCustomers replays creates with stable UUIDs. Before retrying a
// POST it checks the account-scoped list, so a lost response cannot duplicate a
// customer or appliance that Supabase already committed.
func syncOfflineTeamCustomers(ctx context.Context, baseURL, token, userID string, role domain.Role) error {
	offlineSnapshotMu.Lock()
	defer offlineSnapshotMu.Unlock()
	snapshot, err := loadOfflineSnapshot(userID)
	if err != nil || snapshot.Role != role || len(snapshot.PendingCustomers) == 0 {
		return nil
	}
	for len(snapshot.PendingCustomers) > 0 {
		pending := snapshot.PendingCustomers[0]
		customers, err := getTeamRows(ctx, baseURL+"/api/clientes", token)
		if err != nil {
			return err
		}
		customer := findOfflineCustomer(customers, pending.CustomerID)
		if customer == nil {
			customerPayload := cloneAnyMap(pending.Customer)
			delete(customerPayload, "appliances")
			delete(customerPayload, "created_at")
			if _, err = sendTeamJSONResult(ctx, baseURL+"/api/clientes", token, http.MethodPost, customerPayload); err != nil {
				return err
			}
			customer = map[string]any{"id": pending.CustomerID}
		}
		if pending.ApplianceID != "" && pending.Appliance != nil && !offlineCustomerHasAppliance(customer, pending.ApplianceID) {
			appliance := cloneAnyMap(pending.Appliance)
			appliance["cliente_id"] = pending.CustomerID
			if _, err = sendTeamJSONResult(ctx, baseURL+"/api/aparelhos", token, http.MethodPost, appliance); err != nil {
				return err
			}
		}
		snapshot.PendingCustomers = snapshot.PendingCustomers[1:]
		snapshot.SavedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := saveOfflineSnapshot(snapshot); err != nil {
			return err
		}
	}
	return nil
}

func findOfflineCustomer(customers []map[string]any, id string) map[string]any {
	for _, customer := range customers {
		if portalText(customer["id"]) == id {
			return customer
		}
	}
	return nil
}

func offlineCustomerHasAppliance(customer map[string]any, applianceID string) bool {
	rows, _ := customer["appliances"].([]any)
	for _, raw := range rows {
		if appliance, ok := raw.(map[string]any); ok && portalText(appliance["id"]) == applianceID {
			return true
		}
	}
	return false
}

func cloneAnyMap(source map[string]any) map[string]any {
	copy := make(map[string]any, len(source)+1)
	for key, value := range source {
		copy[key] = value
	}
	return copy
}

func newOfflineTeamCustomerIDs() (string, string) {
	return uuid.NewString(), uuid.NewString()
}
