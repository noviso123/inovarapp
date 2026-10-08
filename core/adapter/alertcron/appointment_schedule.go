package alertcron

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"inovarapp/core/adapter/whatsappqueue"
	"inovarapp/core/domain"
)

func (h Handler) syncAppointmentRows(ctx context.Context, now time.Time, customers []customerRow, appliances []applianceRow, config map[string]json.RawMessage) error {
	location := saoPauloLocation()
	localNow := now.In(location)
	var services []serviceRow
	if err := h.readAllRows(ctx, serviceQuery(map[string]string{"status": "eq.AGENDADO", "data_agendamento": "gte." + localNow.Format("2006-01-02"), "select": "id,cliente_id,aparelho_id,tipo,descricao,status,data_agendamento,hora_agendamento"}), &services); err != nil {
		return err
	}
	byCustomer := map[string]customerRow{}
	for _, customer := range customers {
		if customer.Active == nil || *customer.Active {
			byCustomer[customer.ID] = customer
		}
	}
	byAppliance := map[string]domain.AlertAppliance{}
	for _, a := range appliances {
		byAppliance[a.ID] = domain.AlertAppliance{Brand: a.Brand, Model: a.Model, BTUs: scalarString(a.BTUs), Room: a.Room}
	}
	business := "Inovar Refrigeração"
	readSetting(config, "businessName", &business)
	for _, service := range services {
		customer, ok := byCustomer[service.CustomerID]
		if !ok || strings.TrimSpace(customer.WhatsApp) == "" {
			continue
		}
		day, err := time.ParseInLocation("2006-01-02", service.ScheduledDate, location)
		if err != nil {
			continue
		}
		clock := service.ScheduledTime
		if len(clock) > 5 {
			clock = clock[:5]
		}
		var appointment time.Time
		if clock != "" {
			appointment, err = time.ParseInLocation("2006-01-02 15:04", service.ScheduledDate+" "+clock, location)
			if err != nil {
				continue
			}
		}
		equipment := "equipamento não informado no cadastro"
		if a, ok := byAppliance[service.ApplianceID]; ok {
			equipment = domain.AlertEquipmentDescription(&a, true)
		}
		name := domain.AlertServiceName(service.Type, service.Description)
		previousDay := day.AddDate(0, 0, -1)
		plans := []struct {
			event, key, template string
			scheduled, expires   time.Time
		}{
			{"lembrete_agendamento_vespera", "agenda-vespera:", "lembrete_vespera", previousDay.Add(9 * time.Hour), previousDay.Add(20 * time.Hour)},
		}
		if !appointment.IsZero() {
			plans = append(plans, struct {
				event, key, template string
				scheduled, expires   time.Time
			}{"lembrete_agendamento_uma_hora", "agenda-uma-hora:", "lembrete_uma_hora", appointment.Add(-time.Hour), appointment})
		}
		for _, plan := range plans {
			if !plan.expires.After(now) {
				continue
			}
			if plan.event == "lembrete_agendamento_uma_hora" && now.Sub(plan.scheduled) > 15*time.Minute {
				continue
			}
			hourText := clock
			if plan.event == "lembrete_agendamento_vespera" && clock != "" {
				hourText = " às " + clock
			}
			text := h.reminderText(config, plan.template, map[string]string{"cliente": firstWord(customer.Name), "empresa": business, "data": formatDate(service.ScheduledDate), "hora": hourText, "servico": name, "equipamento": equipment})
			if err := whatsappqueue.Enqueue(ctx, h.Supabase, whatsappqueue.EnqueueInput{IdempotencyKey: plan.key + service.ID + ":" + service.ScheduledDate + ":" + service.ScheduledTime, EventType: plan.event, RecipientPhone: customer.WhatsApp, MessageText: text, ScheduledAt: plan.scheduled, ExpiresAt: &plan.expires, SourceEntityType: "servico", SourceEntityID: service.ID, Metadata: map[string]any{"data_agendamento": service.ScheduledDate, "hora_agendamento": clock, "servico": name, "equipamento": equipment, "cliente": customer.Name, "fuso_horario": "America/Sao_Paulo"}}); err != nil {
				return err
			}
		}
	}
	return nil
}
