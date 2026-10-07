package webapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"inovarapp/core/domain"
)

func syncOfflineTeamSettings(ctx context.Context, baseURL, token, userID string, role domain.Role) error {
	offlineSnapshotMu.Lock()
	defer offlineSnapshotMu.Unlock()
	snapshot, err := loadOfflineSnapshot(userID)
	if err != nil || snapshot.Role != role || len(snapshot.PendingSettings) == 0 {
		return nil
	}
	payload := make(map[string]any, len(snapshot.PendingSettings))
	for key, raw := range snapshot.PendingSettings {
		if !offlineSettingsFields()[key] {
			return errors.New("offline settings contain a forbidden field")
		}
		var value any
		if json.Unmarshal(raw, &value) != nil {
			return errors.New("offline settings payload is invalid")
		}
		payload[key] = value
	}
	result, err := sendTeamJSONResult(ctx, baseURL+"/api/configuracoes", token, http.MethodPost, payload)
	if err != nil {
		return err
	}
	if result["ok"] != true {
		return errors.New("server rejected offline settings")
	}
	snapshot.PendingSettings = nil
	snapshot.SavedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return saveOfflineSnapshot(snapshot)
}
