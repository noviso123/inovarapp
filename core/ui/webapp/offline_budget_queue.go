package webapp

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"inovarapp/core/domain"
)

func enqueueOfflineTeamBudget(userID string, role domain.Role, budget domain.BudgetEstimate) error {
	offlineSnapshotMu.Lock()
	defer offlineSnapshotMu.Unlock()
	snapshot, err := loadOfflineSnapshot(userID)
	if err != nil {
		return err
	}
	if snapshot.Role != role || uuid.Validate(budget.ID) != nil || uuid.Validate(budget.ClientID) != nil || len(budget.Items) == 0 {
		return errors.New("offline budget identifiers or contents are invalid")
	}
	for _, pending := range snapshot.PendingBudgets {
		if pending.ID == budget.ID {
			return nil
		}
	}
	snapshot.PendingBudgets = append(snapshot.PendingBudgets, budget)
	snapshot.TeamBudgets = mergePendingOfflineBudgets(snapshot.TeamBudgets, snapshot.PendingBudgets)
	snapshot.SavedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return saveOfflineSnapshot(snapshot)
}

func enqueueTeamBudgetOffline(userID string, role domain.Role, budget domain.BudgetEstimate, customer map[string]any) error {
	if customer != nil {
		if err := enqueueOfflineTeamCustomer(userID, role, customer, nil); err != nil {
			return err
		}
	}
	return enqueueOfflineTeamBudget(userID, role, budget)
}

func syncOfflineTeamBudgets(ctx context.Context, baseURL, token, userID string, role domain.Role) error {
	offlineSnapshotMu.Lock()
	defer offlineSnapshotMu.Unlock()
	snapshot, err := loadOfflineSnapshot(userID)
	if err != nil || snapshot.Role != role || len(snapshot.PendingBudgets) == 0 {
		return nil
	}
	for len(snapshot.PendingBudgets) > 0 {
		pending := snapshot.PendingBudgets[0]
		remote, err := getTeamBudgets(ctx, baseURL+"/api/orcamentos", token)
		if err != nil {
			return err
		}
		if !hasTeamBudgetID(remote, pending.ID) {
			if _, err = sendTeamJSONResult(ctx, baseURL+"/api/orcamentos", token, http.MethodPost, pending); err != nil {
				return err
			}
		}
		snapshot.PendingBudgets = snapshot.PendingBudgets[1:]
		snapshot.SavedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err = saveOfflineSnapshot(snapshot); err != nil {
			return err
		}
	}
	return nil
}

func hasTeamBudgetID(budgets []domain.BudgetEstimate, id string) bool {
	for _, budget := range budgets {
		if budget.ID == id {
			return true
		}
	}
	return false
}

func mergePendingOfflineBudgets(remote, pending []domain.BudgetEstimate) []domain.BudgetEstimate {
	merged := append([]domain.BudgetEstimate(nil), remote...)
	for _, budget := range pending {
		if !hasTeamBudgetID(merged, budget.ID) {
			merged = append(merged, budget)
		}
	}
	return merged
}
