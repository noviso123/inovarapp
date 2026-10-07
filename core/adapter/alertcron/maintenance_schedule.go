package alertcron

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"inovarapp/core/adapter/supabase"
	"inovarapp/core/adapter/whatsappqueue"
	"inovarapp/core/domain"
)

// SyncMaintenance persists future reminders without sending any message.
// The queue worker alone delivers them when scheduled_at becomes due.
func (h Handler) SyncMaintenance(ctx context.Context, now time.Time) error {
	customers, _, appliances, histories, completed, config, err := h.loadBase(ctx)
	if err != nil {
		return err
	}
	return h.syncMaintenanceRows(ctx, now, customers, appliances, histories, completed, config)
}

func (h Handler) syncMaintenanceRows(ctx context.Context, now time.Time, customers []customerRow, appliances []applianceRow, histories []historyRow, completed []serviceRow, config map[string]json.RawMessage) error {
	var dc []domain.AlertCustomer
	for _, row := range customers {
		dc = append(dc, domain.AlertCustomer{ID: row.ID, Name: row.Name, WhatsApp: row.WhatsApp, Active: row.Active == nil || *row.Active})
	}
	var da []domain.AlertAppliance
	for _, row := range appliances {
		da = append(da, domain.AlertAppliance{ID: row.ID, CustomerID: row.CustomerID, Brand: row.Brand, Model: row.Model, BTUs: scalarString(row.BTUs), Room: row.Room, LastMaintenance: row.LastMaintenance})
	}
	var dh []domain.AlertHistory
	for _, row := range histories {
		dh = append(dh, domain.AlertHistory{CustomerID: row.CustomerID, ApplianceID: row.ApplianceID, Date: row.Date, Observations: row.Observations})
	}
	var ds []domain.AlertService
	for _, row := range completed {
		ds = append(ds, domainService(row))
	}
	months := configInt(config, "defaultReturnMonths")
	if months < 1 {
		months = h.Months
	}
	if months < 1 {
		months = 3
	}
	interval := configInt(config, "lembrete_intervalo_dias")
	if interval < 1 {
		interval = 7
	}
	localNow := now.In(saoPauloLocation())
	cycles := domain.BuildPreventiveSchedules(dc, da, dh, ds, months, h.MaxLateDays, localNow)
	for _, cycle := range cycles {
		if cycle.Customer.WhatsApp == "" {
			continue
		}
		bucket := maintenanceReminderBucket(cycle.ExpectedDate, interval, localNow)
		key := fmt.Sprintf("manutencao:%s:%s:%s:%d", cycle.Customer.ID, cycle.Appliance.ID, cycle.ExpectedDate, bucket)
		// Retire reminders belonging to an earlier maintenance cycle, preserving
		// the current cycle's retries and the complete delivery history.
		query := url.Values{"source_entity_type": {"eq.aparelho"}, "source_entity_id": {"eq." + cycle.Appliance.ID}, "event_type": {"eq.lembrete_manutencao_recorrente"}, "status": {"eq.pendente"}, "metadata->>proxima_manutencao": {"neq." + cycle.ExpectedDate}}
		res, err := h.Supabase.ServiceRequest(ctx, "/rest/v1/whatsapp_message_queue?"+query.Encode(), supabase.RequestOptions{Method: http.MethodPatch, Body: map[string]any{"status": "cancelado", "last_error": "Ciclo de manutenção atualizado", "updated_at": now.UTC()}, Prefer: "return=minimal"})
		if err != nil {
			return err
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return fmt.Errorf("não foi possível atualizar o ciclo da fila (status %d)", res.StatusCode)
		}
		// Existing overdue cycles are handled by the daily reminder flow.
		if cycle.DaysToDue < 0 {
			continue
		}
		scheduled, err := time.ParseInLocation("2006-01-02", cycle.ExpectedDate, saoPauloLocation())
		if err != nil {
			return err
		}
		scheduled = scheduled.Add(9 * time.Hour)
		business := "Inovar Refrigeração"
		readSetting(config, "businessName", &business)
		equipment := domain.AlertEquipmentDescription(&cycle.Appliance, false)
		text := h.reminderText(config, "lembrete_ciclo_vencido", map[string]string{"cliente": firstWord(cycle.Customer.Name), "empresa": business, "equipamento": equipment, "data_ultima": formatDate(cycle.LastServiceDate), "situacao": "tem manutenção preventiva prevista para " + formatDate(cycle.ExpectedDate), "meses": fmt.Sprint(cycle.ReturnMonths)})
		expires := maintenanceReminderExpiry(cycle.ExpectedDate, bucket, interval)
		if err := whatsappqueue.Enqueue(ctx, h.Supabase, whatsappqueue.EnqueueInput{IdempotencyKey: key, EventType: "lembrete_manutencao_recorrente", RecipientPhone: cycle.Customer.WhatsApp, MessageText: text, ScheduledAt: scheduled, ExpiresAt: &expires, SourceEntityType: "aparelho", SourceEntityID: cycle.Appliance.ID, Metadata: map[string]any{"ultima_manutencao": cycle.LastServiceDate, "proxima_manutencao": cycle.ExpectedDate, "intervalo_meses": cycle.ReturnMonths, "equipamento": equipment, "cliente": cycle.Customer.Name, "horario_envio": "09:00", "fuso_horario": "America/Sao_Paulo"}}); err != nil {
			return err
		}
	}
	return nil
}
