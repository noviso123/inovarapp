package webapp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"inovarapp/core/domain"
)

type offlineTeamMutation struct {
	ID      string          `json:"id"`
	Method  string          `json:"method"`
	Path    string          `json:"path"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Queued  string          `json:"queuedAt"`
}

var offlineMutableResources = map[string]bool{
	"/api/clientes": true, "/api/aparelhos": true, "/api/servicos": true,
	"/api/agendamentos": true, "/api/orcamento-equipe": true,
	"/api/aparelho-manutencao": true, "/api/historico": true,
}

func enqueueOfflineTeamMutation(userID string, role domain.Role, endpoint, method string, payload any) error {
	parsed, err := url.Parse(endpoint)
	if err != nil || !offlineTeamMutationSupported(parsed.Path, method) {
		return errors.New("offline mutation is not supported")
	}
	var encoded json.RawMessage
	if payload != nil {
		body, marshalErr := json.Marshal(payload)
		if marshalErr != nil || containsOfflineSecretField(body) {
			return errors.New("offline mutation payload contains unsupported data")
		}
		encoded = body
	}
	offlineSnapshotMu.Lock()
	defer offlineSnapshotMu.Unlock()
	snapshot, err := loadOfflineSnapshot(userID)
	if err != nil || snapshot.Role != role {
		return errors.New("offline snapshot is unavailable for this account")
	}
	path := parsed.RequestURI()
	for _, pending := range snapshot.PendingMutations {
		if pending.Method == method && pending.Path == path && string(pending.Payload) == string(encoded) {
			return nil
		}
	}
	snapshot.PendingMutations = append(snapshot.PendingMutations, offlineTeamMutation{ID: uuid.NewString(), Method: method, Path: path, Payload: encoded, Queued: time.Now().UTC().Format(time.RFC3339Nano)})
	applyPendingOfflineTeamMutations(&snapshot)
	snapshot.SavedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return saveOfflineSnapshot(snapshot)
}

func offlineTeamMutationSupported(path, method string) bool {
	return offlineMutableResources[path] && (method == http.MethodPatch || method == http.MethodDelete)
}

func (p *serviceCatalogPage) deferOfflineTeamMutation(endpoint, method string, payload any, cause error) bool {
	if !isOfflineNetworkError(cause) || p.caller == nil {
		return false
	}
	if err := enqueueOfflineTeamMutation(p.caller.UserID, p.caller.Role, endpoint, method, payload); err != nil {
		return false
	}
	p.offlineMode = true
	return true
}

func containsOfflineSecretField(body []byte) bool {
	var value any
	if json.Unmarshal(body, &value) != nil {
		return true
	}
	return offlineValueContainsSecret(value)
}

func offlineValueContainsSecret(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			name := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
			if strings.Contains(name, "password") || strings.Contains(name, "token") || strings.Contains(name, "apikey") || strings.Contains(name, "secret") || name == "authorization" {
				return true
			}
			if offlineValueContainsSecret(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if offlineValueContainsSecret(child) {
				return true
			}
		}
	}
	return false
}

func syncOfflineTeamMutations(ctx context.Context, baseURL, token, userID string, role domain.Role) error {
	offlineSnapshotMu.Lock()
	defer offlineSnapshotMu.Unlock()
	snapshot, err := loadOfflineSnapshot(userID)
	if err != nil || snapshot.Role != role {
		return nil
	}
	for len(snapshot.PendingMutations) > 0 {
		pending := snapshot.PendingMutations[0]
		if !offlineTeamMutationSupported(pathOnly(pending.Path), pending.Method) {
			return errors.New("offline queue contains an invalid mutation")
		}
		var payload any
		if len(pending.Payload) > 0 {
			if err := json.Unmarshal(pending.Payload, &payload); err != nil || containsOfflineSecretField(pending.Payload) {
				return errors.New("offline queue contains an invalid payload")
			}
		}
		if _, err := sendTeamJSONResult(ctx, strings.TrimRight(baseURL, "/")+pending.Path, token, pending.Method, payload); err != nil {
			return err
		}
		snapshot.PendingMutations = snapshot.PendingMutations[1:]
		snapshot.SavedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := saveOfflineSnapshot(snapshot); err != nil {
			return err
		}
	}
	return nil
}

func pathOnly(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}
	return parsed.Path
}

func applyPendingOfflineTeamMutations(snapshot *offlineAccountSnapshot) {
	if snapshot == nil {
		return
	}
	for _, mutation := range snapshot.PendingMutations {
		var payload map[string]any
		if json.Unmarshal(mutation.Payload, &payload) != nil || payload == nil {
			continue
		}
		resource := pathOnly(mutation.Path)
		id := portalText(payload["id"])
		if id == "" {
			query, _ := url.ParseQuery(strings.TrimPrefix(strings.TrimPrefix(mutation.Path, resource), "?"))
			id = query.Get("id")
			if strings.HasPrefix(id, "eq.") {
				id = strings.TrimPrefix(id, "eq.")
			}
		}
		fields, _ := payload["fields"].(map[string]any)
		switch resource {
		case "/api/clientes":
			if mutation.Method == http.MethodDelete {
				snapshot.TeamCustomers = filterOfflineRows(snapshot.TeamCustomers, "id", id)
			} else if row := findOfflineCustomer(snapshot.TeamCustomers, id); row != nil {
				mergeOfflineFields(row, fields)
			}
		case "/api/aparelhos", "/api/aparelho-manutencao":
			if mutation.Method == http.MethodDelete {
				for _, customer := range snapshot.TeamCustomers {
					customer["appliances"] = filterOfflineAnyRows(customer["appliances"], "id", id)
				}
			} else {
				for _, customer := range snapshot.TeamCustomers {
					for _, appliance := range applianceMaps(customer) {
						if portalText(appliance["id"]) == id {
							mergeOfflineFields(appliance, fields)
						}
					}
				}
			}
		case "/api/servicos":
			if mutation.Method == http.MethodDelete {
				snapshot.TeamServices = filterOfflineRows(snapshot.TeamServices, "id", id)
			} else if row := findOfflineService(snapshot.TeamServices, id); row != nil {
				mergeOfflineFields(row, fields)
			}
		case "/api/agendamentos":
			serviceID := portalText(payload["service_id"])
			if serviceID == "" {
				serviceID = id
			}
			if mutation.Method == http.MethodDelete {
				snapshot.TeamAppointments = filterOfflineRows(snapshot.TeamAppointments, "service_id", serviceID)
			} else {
				for _, row := range snapshot.TeamAppointments {
					if portalText(row["service_id"]) == serviceID {
						mergeOfflineFields(row, fields)
					}
				}
			}
		case "/api/orcamento-equipe":
			if mutation.Method == http.MethodDelete {
				snapshot.TeamBudgets = filterOfflineBudgets(snapshot.TeamBudgets, id)
				continue
			}
			for index := range snapshot.TeamBudgets {
				if snapshot.TeamBudgets[index].ID != id {
					continue
				}
				switch portalText(payload["acao"]) {
				case "STATUS":
					switch portalText(payload["status"]) {
					case "APROVAR":
						snapshot.TeamBudgets[index].Status = domain.BudgetApproved
					case "RECUSAR":
						snapshot.TeamBudgets[index].Status = domain.BudgetDeclined
					}
				case "CANCELAR":
					snapshot.TeamBudgets[index].Status = domain.BudgetCancelled
					now := time.Now().UTC().Format(time.RFC3339Nano)
					snapshot.TeamBudgets[index].CancelledAt = &now
				case "RECEBIDO":
					paid := true
					snapshot.TeamBudgets[index].Paid = &paid
				}
			}
		case "/api/historico":
			if mutation.Method == http.MethodDelete {
				snapshot.TeamHistory = filterOfflineRows(snapshot.TeamHistory, "id", id)
			}
		}
	}
}

func mergeOfflineFields(row, fields map[string]any) {
	for key, value := range fields {
		row[key] = value
	}
}

func filterOfflineRows(rows []map[string]any, key, id string) []map[string]any {
	if id == "" {
		return rows
	}
	result := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if portalText(row[key]) != id {
			result = append(result, row)
		}
	}
	return result
}

func filterOfflineAnyRows(value any, key, id string) []any {
	rows, _ := value.([]any)
	result := make([]any, 0, len(rows))
	for _, raw := range rows {
		row, _ := raw.(map[string]any)
		if row == nil || portalText(row[key]) != id {
			result = append(result, raw)
		}
	}
	return result
}

func filterOfflineBudgets(rows []domain.BudgetEstimate, id string) []domain.BudgetEstimate {
	result := make([]domain.BudgetEstimate, 0, len(rows))
	for _, row := range rows {
		if row.ID != id {
			result = append(result, row)
		}
	}
	return result
}
