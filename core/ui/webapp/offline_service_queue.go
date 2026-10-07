package webapp

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"inovarapp/core/domain"
)

func enqueueOfflineTeamService(userID string, role domain.Role, serviceID string, service, appointment map[string]any) error {
	offlineSnapshotMu.Lock()
	defer offlineSnapshotMu.Unlock()
	snapshot, err := loadOfflineSnapshot(userID)
	if err != nil {
		return err
	}
	if snapshot.Role != role || uuid.Validate(serviceID) != nil || portalText(service["id"]) != serviceID {
		return errors.New("offline service identifiers are invalid")
	}
	if appointment != nil && portalText(appointment["service_id"]) != serviceID {
		return errors.New("offline appointment does not match service")
	}
	for _, pending := range snapshot.PendingServices {
		if pending.ServiceID == serviceID {
			return nil
		}
	}
	snapshot.PendingServices = append(snapshot.PendingServices, offlineTeamService{ServiceID: serviceID, Service: service, Appointment: appointment})
	snapshot.TeamServices, snapshot.TeamAppointments = mergePendingOfflineServices(snapshot.TeamServices, snapshot.TeamAppointments, snapshot.PendingServices)
	snapshot.SavedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return saveOfflineSnapshot(snapshot)
}

// Replay checks the server before each POST. A response lost after a successful
// insert is therefore recovered on the next attempt using the same UUID.
func syncOfflineTeamServices(ctx context.Context, baseURL, token, userID string, role domain.Role) error {
	offlineSnapshotMu.Lock()
	defer offlineSnapshotMu.Unlock()
	snapshot, err := loadOfflineSnapshot(userID)
	if err != nil || snapshot.Role != role || len(snapshot.PendingServices) == 0 {
		return nil
	}
	for len(snapshot.PendingServices) > 0 {
		pending := snapshot.PendingServices[0]
		services, err := getTeamRows(ctx, baseURL+"/api/servicos", token)
		if err != nil {
			return err
		}
		if findOfflineService(services, pending.ServiceID) == nil {
			payload := cloneAnyMap(pending.Service)
			delete(payload, "created_at")
			if _, err = sendTeamJSONResult(ctx, baseURL+"/api/servicos", token, http.MethodPost, payload); err != nil {
				return err
			}
		}
		if pending.Appointment != nil {
			appointments, loadErr := getTeamRows(ctx, baseURL+"/api/agendamentos", token)
			if loadErr != nil {
				return loadErr
			}
			if !teamDirectAppointmentExists(appointments, pending.ServiceID) {
				if err = sendTeamJSON(ctx, baseURL+"/api/agendamentos", token, http.MethodPost, pending.Appointment); err != nil {
					return err
				}
			}
		}
		snapshot.PendingServices = snapshot.PendingServices[1:]
		snapshot.SavedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err = saveOfflineSnapshot(snapshot); err != nil {
			return err
		}
	}
	return nil
}

func findOfflineService(services []map[string]any, id string) map[string]any {
	for _, service := range services {
		if portalText(service["id"]) == id {
			return service
		}
	}
	return nil
}

func mergePendingOfflineServices(services, appointments []map[string]any, pending []offlineTeamService) ([]map[string]any, []map[string]any) {
	mergedServices := append([]map[string]any(nil), services...)
	mergedAppointments := append([]map[string]any(nil), appointments...)
	for _, item := range pending {
		if findOfflineService(mergedServices, item.ServiceID) == nil {
			mergedServices = append(mergedServices, cloneAnyMap(item.Service))
		}
		if item.Appointment != nil && !teamDirectAppointmentExists(mergedAppointments, item.ServiceID) {
			mergedAppointments = append(mergedAppointments, cloneAnyMap(item.Appointment))
		}
	}
	return mergedServices, mergedAppointments
}
