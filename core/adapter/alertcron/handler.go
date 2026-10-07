package alertcron

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	mailadapter "inovarapp/core/adapter/email"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/adapter/webpush"
	"inovarapp/core/adapter/whatsapp"
	"inovarapp/core/adapter/whatsappqueue"
	"inovarapp/core/domain"
)

const technicianConfigPath = "/storage/v1/object/documentos-inovar/config/tecnico.json"

type MailSender interface {
	Send(context.Context, mailadapter.Config, mailadapter.Message) (string, error)
}
type WhatsAppSender interface {
	SendText(context.Context, whatsapp.Config, string, string) error
}
type PushSender interface {
	Send(context.Context, webpush.Subscription, webpush.Payload) error
}

type Handler struct {
	Supabase         *supabase.Client
	Secret           string
	Months           int
	MaxLateDays      int
	WhatsAppDefaults whatsapp.Config
	EmailDefaults    mailadapter.Config
	PushDefaults     webpush.Sender
	Mail             MailSender
	WhatsApp         WhatsAppSender
	Push             PushSender
	QueueSender      whatsapp.Sender
	Now              func() time.Time
}

type customerRow struct {
	ID        string `json:"id"`
	Name      string `json:"nome"`
	WhatsApp  string `json:"whatsapp"`
	ProfileID string `json:"profile_id"`
	Active    *bool  `json:"ativo"`
}
type profileRow struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}
type applianceRow struct {
	ID              string          `json:"id"`
	CustomerID      string          `json:"cliente_id"`
	Brand           string          `json:"marca"`
	Model           string          `json:"modelo"`
	BTUs            json.RawMessage `json:"btus"`
	Room            string          `json:"ambiente"`
	LastMaintenance string          `json:"ultima_manutencao"`
}
type historyRow struct {
	CustomerID   string `json:"cliente_id"`
	ApplianceID  string `json:"aparelho_id"`
	Date         string `json:"data"`
	Observations string `json:"observacoes"`
}
type serviceRow struct {
	ID             string          `json:"id"`
	CustomerID     string          `json:"cliente_id"`
	ApplianceID    string          `json:"aparelho_id"`
	Type           string          `json:"tipo"`
	Description    string          `json:"descricao"`
	Status         string          `json:"status"`
	ScheduledDate  string          `json:"data_agendamento"`
	CompletionDate string          `json:"data_conclusao"`
	ScheduledTime  string          `json:"hora_agendamento"`
	Observations   string          `json:"observacoes"`
	Value          json.RawMessage `json:"valor"`
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Método não permitido"})
		return
	}
	if h.Supabase == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Backend não configurado"})
		return
	}
	if strings.TrimSpace(h.Secret) == "" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "CRON_SECRET não configurado"})
		return
	}
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(header, "Bearer ") {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Não autorizado"})
		return
	}
	auth := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if len(auth) != len(h.Secret) || subtle.ConstantTimeCompare([]byte(auth), []byte(h.Secret)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Não autorizado"})
		return
	}
	now := time.Now()
	if h.Now != nil {
		now = h.Now()
	}
	now = now.UTC()
	if strings.HasSuffix(r.URL.Path, "/whatsapp-fila") {
		config, err := h.loadConfig(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Não foi possível carregar a configuração do WhatsApp"})
			return
		}
		// One send per invocation stays inside the Vercel function deadline even
		// when the provider or private PDF storage is slow. Supabase wakes the
		// worker every minute, so a backlog drains continuously.
		result, err := whatsappqueue.ProcessDue(r.Context(), h.Supabase, h.QueueSender, h.whatsAppConfig(config), now, 1)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Falha ao processar fila WhatsApp", "detalhe": short(err)})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "executadoEm": now.Format(time.RFC3339Nano), "fila": result})
		return
	}
	if strings.HasSuffix(r.URL.Path, "/agenda-uma-hora") {
		result, err := h.runOneHour(r.Context(), now)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Falha no cron de alertas", "detalhe": short(err)})
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	result, err := h.runDaily(r.Context(), now)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Falha no cron de alertas", "detalhe": short(err)})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h Handler) runDaily(ctx context.Context, now time.Time) (map[string]any, error) {
	customers, profiles, appliances, histories, completed, config, err := h.loadBase(ctx)
	if err != nil {
		return nil, err
	}
	profileEmail := map[string]string{}
	for _, profile := range profiles {
		profileEmail[profile.ID] = profile.Email
	}
	domainCustomers := make([]domain.AlertCustomer, 0, len(customers))
	customerByID := map[string]domain.AlertCustomer{}
	for _, row := range customers {
		active := row.Active == nil || *row.Active
		customer := domain.AlertCustomer{ID: row.ID, Name: row.Name, WhatsApp: row.WhatsApp, ProfileID: row.ProfileID, Email: profileEmail[row.ProfileID], Active: active}
		domainCustomers = append(domainCustomers, customer)
		customerByID[row.ID] = customer
	}
	domainAppliances := make([]domain.AlertAppliance, 0, len(appliances))
	applianceByID := map[string]domain.AlertAppliance{}
	for _, row := range appliances {
		appliance := domain.AlertAppliance{ID: row.ID, CustomerID: row.CustomerID, Brand: row.Brand, Model: row.Model, BTUs: scalarString(row.BTUs), Room: row.Room, LastMaintenance: row.LastMaintenance}
		domainAppliances = append(domainAppliances, appliance)
		applianceByID[row.ID] = appliance
	}
	domainHistory := make([]domain.AlertHistory, 0, len(histories))
	for _, row := range histories {
		domainHistory = append(domainHistory, domain.AlertHistory{CustomerID: row.CustomerID, ApplianceID: row.ApplianceID, Date: row.Date, Observations: row.Observations})
	}
	domainCompleted := make([]domain.AlertService, 0, len(completed))
	for _, row := range completed {
		domainCompleted = append(domainCompleted, domainService(row))
	}
	months := configInt(config, "defaultReturnMonths")
	if months < 1 {
		months = h.Months
	}
	if months < 1 {
		months = 3
	}
	maxLate := h.MaxLateDays
	localNow := now.In(saoPauloLocation())
	alerts := domain.BuildPreventiveAlertsWithDefault(domainCustomers, domainAppliances, domainHistory, domainCompleted, months, maxLate, localNow)
	interval := configInt(config, "lembrete_intervalo_dias")
	if interval == 0 {
		interval = 7
	}
	if interval < 1 {
		interval = 1
	}
	legacyCycles := configStringLog(config["lembretes_log"])
	cycleWhatsApp := configStringLog(config["lembretes_ciclo_whatsapp_log"])
	cycleWhatsAppAttempts := configStringLog(config["lembretes_ciclo_whatsapp_tentativas"])
	cycleEmail := configStringLog(config["lembretes_ciclo_email_log"])
	cycleEmailAttempts := configStringLog(config["lembretes_ciclo_email_tentativas"])
	cyclePush := configStringLog(config["lembretes_ciclo_push_log"])
	cyclePushAttempts := configStringLog(config["lembretes_ciclo_push_tentativas"])
	nowStamp := now.Format(time.RFC3339Nano)
	results := make([]map[string]any, 0, len(alerts))
	changedKeys := map[string]bool{}
	mailConfig := h.mailConfig(config)
	waConfig := h.whatsAppConfig(config)
	for _, alert := range alerts {
		name := firstWord(alert.Customer.Name)
		equipment := domain.AlertEquipmentDescription(&alert.Appliance, false)
		key := alert.Customer.ID + "|" + alert.Appliance.ID
		situation := "vence nos próximos dias"
		if alert.DaysToDue < 0 {
			situation = fmt.Sprintf("está em atraso há %d dias", int(math.Abs(float64(alert.DaysToDue))))
		}
		business := "Inovar Refrigeração"
		readSetting(config, "businessName", &business)
		text := h.reminderText(config, "lembrete_ciclo_vencido", map[string]string{"cliente": name, "empresa": business, "equipamento": equipment, "data_ultima": formatDate(alert.LastServiceDate), "situacao": situation, "meses": fmt.Sprint(alert.ReturnMonths)})
		channels := map[string]any{}
		if !withinReminderWindow(localNow) {
			results = append(results, map[string]any{"cliente": alert.Customer.Name, "aparelho": equipment, "retorno": alert.ExpectedDate, "pulado": true, "motivo": "fora do horário de envio (09h–20h, horário de Brasília)"})
			continue
		}
		legacyThrottled := logWithin(legacyCycles[key], now, time.Duration(interval)*24*time.Hour) &&
			!hasRecentDelivery(cycleWhatsApp, key, now, time.Duration(interval)*24*time.Hour) &&
			!hasRecentDelivery(cycleEmail, key, now, time.Duration(interval)*24*time.Hour) &&
			!hasRecentDelivery(cyclePush, key, now, time.Duration(interval)*24*time.Hour)
		if alert.Customer.WhatsApp != "" && !legacyThrottled {
			if !deliveryRecentlySent(cycleWhatsApp, key, now, time.Duration(interval)*24*time.Hour) {
				bucket := maintenanceReminderBucket(alert.ExpectedDate, interval, localNow)
				expires := maintenanceReminderExpiry(alert.ExpectedDate, bucket, interval)
				idempotency := fmt.Sprintf("manutencao:%s:%s:%s:%d", alert.Customer.ID, alert.Appliance.ID, alert.ExpectedDate, bucket)
				err := whatsappqueue.Enqueue(ctx, h.Supabase, whatsappqueue.EnqueueInput{
					IdempotencyKey: idempotency, EventType: "lembrete_manutencao_recorrente",
					RecipientPhone: alert.Customer.WhatsApp, MessageText: text,
					ExpiresAt:        &expires,
					SourceEntityType: "aparelho", SourceEntityID: alert.Appliance.ID,
					Metadata: map[string]any{
						"ultima_manutencao": alert.LastServiceDate, "proxima_manutencao": alert.ExpectedDate,
						"intervalo_meses": alert.ReturnMonths, "dias_ate_vencimento": alert.DaysToDue,
						"equipamento": equipment, "cliente": alert.Customer.Name,
					},
				})
				if err != nil {
					channels["whatsapp"] = map[string]any{"enfileirado": false, "status": "erro", "motivo": short(err)}
				} else {
					channels["whatsapp"] = map[string]any{"enfileirado": true, "status": "pendente"}
				}
			} else {
				channels["whatsapp"] = map[string]any{"enfileirado": false, "pulado": true, "motivo": "lembrete já enviado neste intervalo"}
			}
		}
		if alert.Customer.Email != "" {
			if !legacyThrottled && !deliveryRecentlySent(cycleEmail, key, now, time.Duration(interval)*24*time.Hour) && !deliveryRecentlyAttempted(cycleEmailAttempts, key, now, time.Hour) {
				body := fmt.Sprintf(`<p>Olá, <b>%s</b>!</p><p>A manutenção/limpeza de ar do seu <b>%s</b> (realizada em %s) %s — o ciclo recomendado é de %d meses.</p><p>Manter o ciclo evita fungos, bactérias, mau cheiro e consumo elevado de energia.</p><p>Responda este e-mail ou chame no WhatsApp para agendar sua visita! 🗓️</p><p><b>Inovar — Refrigeração</b></p>`, html.EscapeString(name), html.EscapeString(equipment), html.EscapeString(formatDate(alert.LastServiceDate)), html.EscapeString(situation), alert.ReturnMonths)
				result := h.sendEmail(ctx, mailConfig, mailadapter.Message{To: alert.Customer.Email, Subject: "❄️ Inovar: hora da manutenção preventiva do seu ar-condicionado", HTML: body})
				channels["email"] = result
				cycleEmailAttempts[key] = nowStamp
				changedKeys["lembretes_ciclo_email_tentativas"] = true
				if result["enviado"] == true {
					cycleEmail[key] = nowStamp
					legacyCycles[key] = nowStamp
					changedKeys["lembretes_ciclo_email_log"] = true
					changedKeys["lembretes_log"] = true
				}
			}
		}
		if alert.Customer.ProfileID != "" {
			if !legacyThrottled && !deliveryRecentlySent(cyclePush, key, now, time.Duration(interval)*24*time.Hour) && !deliveryRecentlyAttempted(cyclePushAttempts, key, now, time.Hour) {
				result := h.sendPushToUser(ctx, alert.Customer.ProfileID, webpush.Payload{Title: "Manutenção preventiva Inovar", Body: fmt.Sprintf("Seu %s %s. Toque para solicitar o atendimento.", equipment, situation), URL: "/", Path: "/"})
				channels["push"] = result
				cyclePushAttempts[key] = nowStamp
				changedKeys["lembretes_ciclo_push_tentativas"] = true
				if pushDelivered(result) {
					cyclePush[key] = nowStamp
					legacyCycles[key] = nowStamp
					changedKeys["lembretes_ciclo_push_log"] = true
					changedKeys["lembretes_log"] = true
				}
			}
		}
		if legacyThrottled {
			channels["todos"] = map[string]any{"pulado": true, "motivo": "lembrete já registrado antes da atualização"}
		}
		results = append(results, map[string]any{"cliente": alert.Customer.Name, "aparelho": equipment, "retorno": alert.ExpectedDate, "canais": channels})
	}
	monthTomorrow := localNow.AddDate(0, 0, 1).Format("2006-01-02")
	tomorrow, err := h.loadServices(ctx, serviceQuery(map[string]string{"status": "eq.AGENDADO", "data_agendamento": "eq." + monthTomorrow, "select": "id,cliente_id,aparelho_id,tipo,descricao,status,data_agendamento,hora_agendamento,valor"}))
	if err != nil {
		return nil, err
	}
	agendaResults := make([]map[string]any, 0, len(tomorrow))
	agendaWhatsApp := configStringLog(config["lembretes_vespera_whatsapp_log"])
	agendaWhatsAppAttempts := configStringLog(config["lembretes_vespera_whatsapp_tentativas"])
	agendaEmail := configStringLog(config["lembretes_vespera_email_log"])
	agendaEmailAttempts := configStringLog(config["lembretes_vespera_email_tentativas"])
	agendaPush := configStringLog(config["lembretes_vespera_push_log"])
	agendaPushAttempts := configStringLog(config["lembretes_vespera_push_tentativas"])
	for _, service := range tomorrow {
		customer, ok := customerByID[service.CustomerID]
		if !ok {
			continue
		}
		var appliance *domain.AlertAppliance
		if value, exists := applianceByID[service.ApplianceID]; exists {
			appliance = &value
		}
		reminder := domain.BuildAgendaReminder(domainService(service), customer, appliance)
		key := service.ID + "|" + service.ScheduledDate + "|" + service.ScheduledTime
		first := firstWord(customer.Name)
		hour := service.ScheduledTime
		business := "Inovar Refrigeração"
		readSetting(config, "businessName", &business)
		text := h.reminderText(config, "lembrete_vespera", map[string]string{"cliente": first, "empresa": business, "data": formatDate(service.ScheduledDate), "hora": func() string {
			if hour != "" {
				return " às " + hour
			}
			return ""
		}(), "servico": reminder.ServiceName, "equipamento": reminder.ApplianceDescription})
		channels := map[string]any{}
		if !withinReminderWindow(localNow) {
			agendaResults = append(agendaResults, map[string]any{"cliente": customer.Name, "data": service.ScheduledDate, "hora": hour, "pulado": true, "motivo": "fora do horário de envio (09h–20h, horário de Brasília)"})
			continue
		}
		if customer.WhatsApp != "" && !deliveryRecentlySent(agendaWhatsApp, key, now, 365*24*time.Hour) {
			expires := appointmentExpiry(service.ScheduledDate, service.ScheduledTime, 3*time.Hour)
			err := whatsappqueue.Enqueue(ctx, h.Supabase, whatsappqueue.EnqueueInput{
				IdempotencyKey: "agenda-vespera:" + service.ID + ":" + service.ScheduledDate + ":" + service.ScheduledTime,
				EventType:      "lembrete_agendamento_vespera", RecipientPhone: customer.WhatsApp, MessageText: text,
				ExpiresAt: &expires, SourceEntityType: "servico", SourceEntityID: service.ID,
				Metadata: map[string]any{"data_agendamento": service.ScheduledDate, "hora_agendamento": service.ScheduledTime, "servico": reminder.ServiceName, "equipamento": reminder.ApplianceDescription},
			})
			if err != nil {
				channels["whatsapp"] = map[string]any{"enfileirado": false, "status": "erro", "motivo": short(err)}
			} else {
				channels["whatsapp"] = map[string]any{"enfileirado": true, "status": "pendente"}
			}
		}
		if customer.Email != "" && !deliveryRecentlySent(agendaEmail, key, now, 365*24*time.Hour) && !deliveryRecentlyAttempted(agendaEmailAttempts, key, now, time.Hour) {
			kind := strings.Replace(service.Type, "_", " ", 1)
			body := fmt.Sprintf(`<p>Olá, <b>%s</b>!</p><p>Seu atendimento está agendado para <b>amanhã, %s%s</b>.</p><p>❄️ Serviço: %s — %s</p><p>Qualquer imprevisto, é só nos chamar! <b>Inovar Refrigeração</b></p>`, html.EscapeString(first), html.EscapeString(formatDate(service.ScheduledDate)), html.EscapeString(func() string {
				if hour != "" {
					return " às " + hour
				}
				return ""
			}()), html.EscapeString(kind), html.EscapeString(reminder.ApplianceDescription))
			result := h.sendEmail(ctx, mailConfig, mailadapter.Message{To: customer.Email, Subject: "⏰ Lembrete InovarApp: seu atendimento é amanhã!", HTML: body})
			channels["email"] = result
			agendaEmailAttempts[key] = nowStamp
			changedKeys["lembretes_vespera_email_tentativas"] = true
			if result["enviado"] == true {
				agendaEmail[key] = nowStamp
				changedKeys["lembretes_vespera_email_log"] = true
			}
		}
		if customer.ProfileID != "" && !deliveryRecentlySent(agendaPush, key, now, 365*24*time.Hour) && !deliveryRecentlyAttempted(agendaPushAttempts, key, now, time.Hour) {
			body := reminder.ServiceName
			if hour != "" {
				body += " às " + hour
			}
			body += ". Toque para conferir os detalhes."
			result := h.sendPushToUser(ctx, customer.ProfileID, webpush.Payload{Title: "Atendimento agendado para amanhã", Body: body, URL: "/", Path: "/"})
			channels["push"] = result
			agendaPushAttempts[key] = nowStamp
			changedKeys["lembretes_vespera_push_tentativas"] = true
			if pushDelivered(result) {
				agendaPush[key] = nowStamp
				changedKeys["lembretes_vespera_push_log"] = true
			}
		}
		agendaResults = append(agendaResults, map[string]any{"cliente": customer.Name, "data": service.ScheduledDate, "hora": hour, "canais": channels})
	}
	config["lembretes_ciclo_whatsapp_log"] = mustJSON(pruneDeliveryLog(cycleWhatsApp, now, 45*24*time.Hour))
	config["lembretes_ciclo_whatsapp_tentativas"] = mustJSON(pruneDeliveryLog(cycleWhatsAppAttempts, now, 3*24*time.Hour))
	config["lembretes_ciclo_email_log"] = mustJSON(pruneDeliveryLog(cycleEmail, now, 45*24*time.Hour))
	config["lembretes_ciclo_email_tentativas"] = mustJSON(pruneDeliveryLog(cycleEmailAttempts, now, 3*24*time.Hour))
	config["lembretes_ciclo_push_log"] = mustJSON(pruneDeliveryLog(cyclePush, now, 45*24*time.Hour))
	config["lembretes_ciclo_push_tentativas"] = mustJSON(pruneDeliveryLog(cyclePushAttempts, now, 3*24*time.Hour))
	config["lembretes_vespera_whatsapp_log"] = mustJSON(pruneDeliveryLog(agendaWhatsApp, now, 45*24*time.Hour))
	config["lembretes_vespera_whatsapp_tentativas"] = mustJSON(pruneDeliveryLog(agendaWhatsAppAttempts, now, 3*24*time.Hour))
	config["lembretes_vespera_email_log"] = mustJSON(pruneDeliveryLog(agendaEmail, now, 45*24*time.Hour))
	config["lembretes_vespera_email_tentativas"] = mustJSON(pruneDeliveryLog(agendaEmailAttempts, now, 3*24*time.Hour))
	config["lembretes_vespera_push_log"] = mustJSON(pruneDeliveryLog(agendaPush, now, 45*24*time.Hour))
	config["lembretes_vespera_push_tentativas"] = mustJSON(pruneDeliveryLog(agendaPushAttempts, now, 3*24*time.Hour))
	config["lembretes_log"] = mustJSON(pruneDeliveryLog(legacyCycles, now, 45*24*time.Hour))
	for key := range changedKeys {
		// Ensure pruning is persisted even when no new send was attempted.
		changedKeys[key] = true
	}
	if len(changedKeys) > 0 {
		keys := make([]string, 0, len(changedKeys))
		for key := range changedKeys {
			keys = append(keys, key)
		}
		if err := h.saveConfig(ctx, config, keys...); err != nil {
			return nil, err
		}
	}
	return map[string]any{"ok": true, "executadoEm": now.Format(time.RFC3339Nano), "ciclosVencidos": len(alerts), "lembretesAgenda": len(agendaResults), "lembretesUmaHora": 0, "whatsappConfigurado": whatsConfigured(waConfig), "emailConfigurado": emailConfigured(mailConfig), "disparosAgenda": agendaResults}, nil
}

func (h Handler) runOneHour(ctx context.Context, now time.Time) (map[string]any, error) {
	config, err := h.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	location, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		location = time.FixedZone("BRT", -3*60*60)
	}
	localNow := now.In(location)
	today := localNow.Format("2006-01-02")
	services, err := h.loadServices(ctx, serviceQuery(map[string]string{"status": "eq.AGENDADO", "data_agendamento": "eq." + today, "select": "id,cliente_id,tipo,descricao,hora_agendamento,data_agendamento"}))
	if err != nil {
		return nil, err
	}
	var customers []customerRow
	var profiles []profileRow
	if err := h.readRows(ctx, "/rest/v1/customers?select=id,nome,whatsapp,profile_id,ativo&ativo=eq.true", &customers); err != nil {
		return nil, err
	}
	if err := h.readRows(ctx, "/rest/v1/profiles?select=id,email", &profiles); err != nil {
		return nil, err
	}
	emailByProfile := map[string]string{}
	for _, profile := range profiles {
		emailByProfile[profile.ID] = profile.Email
	}
	customerByID := map[string]domain.AlertCustomer{}
	for _, row := range customers {
		customerByID[row.ID] = domain.AlertCustomer{ID: row.ID, Name: row.Name, WhatsApp: row.WhatsApp, ProfileID: row.ProfileID, Email: emailByProfile[row.ProfileID], Active: row.Active == nil || *row.Active}
	}
	legacyLog := configStringLog(config["lembretes_uma_hora_log"])
	whatsLog := configStringLog(config["lembretes_uma_hora_whatsapp_log"])
	whatsAttempts := configStringLog(config["lembretes_uma_hora_whatsapp_tentativas"])
	pushLog := configStringLog(config["lembretes_uma_hora_push_log"])
	pushAttempts := configStringLog(config["lembretes_uma_hora_push_tentativas"])
	results := make([]map[string]any, 0)
	changedKeys := map[string]bool{}
	waConfig := h.whatsAppConfig(config)
	mailConfig := h.mailConfig(config)
	business := "Inovar Refrigeração"
	readSetting(config, "businessName", &business)
	for _, service := range services {
		if service.ScheduledTime == "" {
			continue
		}
		parts := strings.Split(service.ScheduledTime[:min(5, len(service.ScheduledTime))], ":")
		if len(parts) != 2 {
			continue
		}
		hour, e1 := parseInt(parts[0])
		minute, e2 := parseInt(parts[1])
		if e1 != nil || e2 != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
			continue
		}
		scheduled := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), hour, minute, 0, 0, location)
		minutes := int(math.Floor(scheduled.Sub(localNow).Minutes() + 0.5))
		if minutes < 45 || minutes > 75 {
			continue
		}
		customer, ok := customerByID[service.CustomerID]
		if !ok {
			continue
		}
		name := domain.AlertServiceName(service.Type, service.Description)
		key := service.ID + "|" + service.ScheduledDate + "|" + service.ScheduledTime[:min(5, len(service.ScheduledTime))]
		channels := map[string]any{}
		first := firstWord(customer.Name)
		date := formatDate(service.ScheduledDate)
		hourText := service.ScheduledTime[:min(5, len(service.ScheduledTime))]
		if customer.WhatsApp != "" && !deliveryRecentlySent(whatsLog, key, now, 365*24*time.Hour) {
			message := h.reminderText(config, "lembrete_uma_hora", map[string]string{"cliente": first, "empresa": business, "data": date, "hora": hourText, "servico": name})
			expires := appointmentExpiry(service.ScheduledDate, service.ScheduledTime, 2*time.Hour)
			err := whatsappqueue.Enqueue(ctx, h.Supabase, whatsappqueue.EnqueueInput{
				IdempotencyKey: "agenda-uma-hora:" + service.ID + ":" + service.ScheduledDate + ":" + service.ScheduledTime,
				EventType:      "lembrete_agendamento_uma_hora", RecipientPhone: customer.WhatsApp, MessageText: message,
				ExpiresAt: &expires, SourceEntityType: "servico", SourceEntityID: service.ID,
				Metadata: map[string]any{"data_agendamento": service.ScheduledDate, "hora_agendamento": hourText, "servico": name, "cliente": customer.Name},
			})
			if err != nil {
				channels["whatsapp"] = map[string]any{"enfileirado": false, "status": "erro", "motivo": short(err)}
			} else {
				channels["whatsapp"] = map[string]any{"enfileirado": true, "status": "pendente"}
			}
		}
		legacyPushSent := logWithin(legacyLog[service.ID], now, 24*time.Hour)
		if customer.ProfileID != "" && !legacyPushSent && !deliveryRecentlySent(pushLog, key, now, 365*24*time.Hour) && !deliveryRecentlyAttempted(pushAttempts, key, now, 12*time.Minute) {
			push := h.sendPushToUser(ctx, customer.ProfileID, webpush.Payload{Title: "Seu atendimento é em aproximadamente 1 hora", Body: fmt.Sprintf("%s às %s. Toque para ver os detalhes.", name, hourText), URL: "/", Path: "/", Tag: "agenda-1h-" + service.ID})
			channels["push"] = push
			pushAttempts[key] = now.Format(time.RFC3339Nano)
			changedKeys["lembretes_uma_hora_push_tentativas"] = true
			if pushDelivered(push) {
				pushLog[key] = now.Format(time.RFC3339Nano)
				changedKeys["lembretes_uma_hora_push_log"] = true
			}
		}
		if len(channels) > 0 {
			results = append(results, map[string]any{"cliente": customer.Name, "data": date, "hora": hourText, "canais": channels})
		}
	}
	config["lembretes_uma_hora_whatsapp_log"] = mustJSON(pruneDeliveryLog(whatsLog, now, 45*24*time.Hour))
	config["lembretes_uma_hora_whatsapp_tentativas"] = mustJSON(pruneDeliveryLog(whatsAttempts, now, 3*24*time.Hour))
	config["lembretes_uma_hora_push_log"] = mustJSON(pruneDeliveryLog(pushLog, now, 45*24*time.Hour))
	config["lembretes_uma_hora_push_tentativas"] = mustJSON(pruneDeliveryLog(pushAttempts, now, 3*24*time.Hour))
	if len(changedKeys) > 0 {
		keys := make([]string, 0, len(changedKeys))
		for key := range changedKeys {
			keys = append(keys, key)
		}
		if err := h.saveConfig(ctx, config, keys...); err != nil {
			return nil, err
		}
	}
	return map[string]any{"ok": true, "executadoEm": now.Format(time.RFC3339Nano), "lembretesUmaHora": len(results), "resultados": results, "whatsappConfigurado": whatsConfigured(waConfig), "emailConfigurado": emailConfigured(mailConfig)}, nil
}

func (h Handler) loadBase(ctx context.Context) ([]customerRow, []profileRow, []applianceRow, []historyRow, []serviceRow, map[string]json.RawMessage, error) {
	var customers []customerRow
	var profiles []profileRow
	var appliances []applianceRow
	var histories []historyRow
	var completed []serviceRow
	queries := []struct {
		path string
		out  any
	}{{"/rest/v1/customers?select=id,nome,whatsapp,profile_id,ativo&ativo=eq.true", &customers}, {"/rest/v1/profiles?select=id,email", &profiles}, {"/rest/v1/air_conditioners?select=id,cliente_id,marca,modelo,btus,ambiente,ultima_manutencao", &appliances}, {"/rest/v1/service_history?select=cliente_id,aparelho_id,data,observacoes&order=data.desc&limit=2000", &histories}, {"/rest/v1/services?status=eq.CONCLUIDO&select=cliente_id,aparelho_id,data_agendamento,data_conclusao,status,observacoes&order=data_conclusao.desc.nullslast,data_agendamento.desc&limit=2000", &completed}}
	errs := make([]error, len(queries))
	var wg sync.WaitGroup
	for i, query := range queries {
		wg.Add(1)
		go func(i int, path string, out any) {
			defer wg.Done()
			errs[i] = h.readAllRows(ctx, path, out)
		}(i, query.path, query.out)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, nil, nil, nil, nil, nil, err
		}
	}
	config, err := h.loadConfig(ctx)
	return customers, profiles, appliances, histories, completed, config, err
}
func (h Handler) loadServices(ctx context.Context, path string) ([]serviceRow, error) {
	var rows []serviceRow
	err := h.readRows(ctx, path, &rows)
	return rows, err
}
func (h Handler) readRows(ctx context.Context, path string, target any) error {
	result, err := h.Supabase.ServiceRequest(ctx, path, supabase.RequestOptions{Method: http.MethodGet})
	if err != nil {
		return err
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		return fmt.Errorf("consulta Supabase retornou status %d", result.StatusCode)
	}
	if err := json.Unmarshal(result.Body, target); err != nil {
		return fmt.Errorf("resposta Supabase inválida: %w", err)
	}
	return nil
}

// Read every page rather than silently losing customers to the PostgREST row cap.
func (h Handler) readAllRows(ctx context.Context, path string, target any) error {
	u, err := url.Parse(path)
	if err != nil {
		return err
	}
	q := u.Query()
	order := q.Get("order")
	if order == "" {
		order = "id.asc"
	} else {
		order += ",id.asc"
	}
	q.Set("order", order)
	q.Set("limit", "500")
	rows := make([]json.RawMessage, 0)
	for offset := 0; ; offset += 500 {
		if err := ctx.Err(); err != nil {
			return err
		}
		q.Set("offset", strconv.Itoa(offset))
		u.RawQuery = q.Encode()
		var page []json.RawMessage
		if err := h.readRows(ctx, u.String(), &page); err != nil {
			return err
		}
		rows = append(rows, page...)
		if len(page) < 500 {
			break
		}
	}
	data, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}
func (h Handler) loadConfig(ctx context.Context) (map[string]json.RawMessage, error) {
	result, err := h.Supabase.ServiceRequest(ctx, technicianConfigPath, supabase.RequestOptions{Method: http.MethodGet})
	if err != nil {
		return nil, err
	}
	if result.StatusCode == http.StatusNotFound || result.StatusCode == http.StatusBadRequest {
		return map[string]json.RawMessage{}, nil
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		return nil, fmt.Errorf("leitura de configurações retornou status %d", result.StatusCode)
	}
	var config map[string]json.RawMessage
	if json.Unmarshal(result.Body, &config) != nil || config == nil {
		return nil, errors.New("configurações inválidas")
	}
	return config, nil
}
func (h Handler) saveConfig(ctx context.Context, config map[string]json.RawMessage, changedKeys ...string) error {
	latest, err := h.loadConfig(ctx)
	if err != nil {
		return err
	}
	if latest == nil {
		latest = map[string]json.RawMessage{}
	}
	for _, changedKey := range changedKeys {
		if value, exists := config[changedKey]; exists {
			latest[changedKey] = value
		}
	}
	result, err := h.Supabase.ServiceRequest(ctx, technicianConfigPath, supabase.RequestOptions{Method: http.MethodPost, Headers: http.Header{"X-Upsert": []string{"true"}}, Body: latest})
	if err != nil {
		return err
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		return fmt.Errorf("gravação de configurações retornou status %d", result.StatusCode)
	}
	return nil
}

func (h Handler) sendWhatsApp(ctx context.Context, cfg whatsapp.Config, phone, message string) map[string]any {
	sender := h.WhatsApp
	if sender == nil {
		sender = whatsapp.Sender{}
	}
	err := sender.SendText(ctx, cfg, phone, message)
	return map[string]any{"enviado": err == nil, "motivo": errorText(err)}
}
func (h Handler) sendEmail(ctx context.Context, cfg mailadapter.Config, message mailadapter.Message) map[string]any {
	sender := h.Mail
	if sender == nil {
		sender = mailadapter.ConfiguredSender{}
	}
	_, err := sender.Send(ctx, cfg, message)
	return map[string]any{"enviado": err == nil, "motivo": errorText(err)}
}
func (h Handler) sendPushToUser(ctx context.Context, userID string, payload webpush.Payload) map[string]any {
	if h.Supabase == nil {
		return map[string]any{"sent": 0, "skipped": true}
	}
	if h.Push == nil && (strings.TrimSpace(h.PushDefaults.PublicKey) == "" || strings.TrimSpace(h.PushDefaults.PrivateKey) == "") {
		return map[string]any{"sent": 0, "skipped": true}
	}
	records, err := h.Supabase.ListPushSubscriptions(ctx, userID)
	if err != nil {
		return map[string]any{"sent": 0, "error": short(err)}
	}
	sender := h.Push
	if sender == nil {
		sender = h.PushDefaults
	}
	sent := 0
	expired := 0
	for _, record := range records {
		var sub webpush.Subscription
		if json.Unmarshal(record.Data, &sub) != nil {
			continue
		}
		sendErr := sender.Send(ctx, sub, payload)
		if errors.Is(sendErr, webpush.ErrSubscriptionExpired) {
			if h.Supabase.DeletePushSubscription(ctx, record.ObjectPath) == nil {
				expired++
			}
			continue
		}
		if sendErr == nil {
			sent++
		}
	}
	return map[string]any{"sent": sent, "expired": expired}
}

func (h Handler) reminderText(config map[string]json.RawMessage, key string, vars map[string]string) string {
	var messages map[string]string
	_ = json.Unmarshal(config["mensagensWhats"], &messages)
	return domain.ApplyWhatsAppPlaceholders(domain.ResolveWhatsAppTemplate(messages, key), vars)
}
func (h Handler) mailConfig(config map[string]json.RawMessage) mailadapter.Config {
	out := h.EmailDefaults
	readSetting(config, "email_google_refresh_token", &out.GoogleRefreshToken)
	readSetting(config, "email_google_account", &out.Username)
	readSetting(config, "email_gmail_user", &out.Username)
	readSetting(config, "email_gmail_pass", &out.Password)
	readSetting(config, "email_api_key", &out.APIKey)
	readSetting(config, "email_from", &out.From)
	if strings.TrimSpace(out.GoogleRefreshToken) != "" {
		readSetting(config, "email_google_account", &out.Username)
	}
	return out
}
func (h Handler) whatsAppConfig(config map[string]json.RawMessage) whatsapp.Config {
	out := h.WhatsAppDefaults
	if out.ProprioSession == "" {
		out.ProprioSession = "inovar"
	}
	return out
}
func readSetting(config map[string]json.RawMessage, key string, target *string) {
	var value string
	if json.Unmarshal(config[key], &value) == nil && strings.TrimSpace(value) != "" {
		*target = value
	}
}
func emailConfigured(config mailadapter.Config) bool {
	return mailadapter.Configured(config)
}
func whatsConfigured(config whatsapp.Config) bool {
	return strings.TrimSpace(config.ProprioURL) != "" && strings.TrimSpace(config.ProprioToken) != ""
}
func serviceQuery(values map[string]string) string {
	query := url.Values{}
	for key, value := range values {
		query.Set(key, value)
	}
	return "/rest/v1/services?" + query.Encode()
}
func domainService(row serviceRow) domain.AlertService {
	return domain.AlertService{ID: row.ID, CustomerID: row.CustomerID, ApplianceID: row.ApplianceID, Type: row.Type, Description: row.Description, Status: row.Status, ScheduledDate: row.ScheduledDate, CompletionDate: row.CompletionDate, ScheduledTime: row.ScheduledTime, Observations: row.Observations}
}
func scalarString(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var number json.Number
	if json.Unmarshal(raw, &number) == nil {
		return number.String()
	}
	return ""
}
func configInt(config map[string]json.RawMessage, key string) int {
	var n int
	if json.Unmarshal(config[key], &n) == nil {
		return n
	}
	var f float64
	if json.Unmarshal(config[key], &f) == nil {
		return int(f)
	}
	return 0
}
func configStringLog(raw json.RawMessage) map[string]string {
	out := map[string]string{}
	var source map[string]json.RawMessage
	if json.Unmarshal(raw, &source) != nil {
		return out
	}
	for key, value := range source {
		var text string
		if json.Unmarshal(value, &text) == nil {
			out[key] = text
		}
	}
	return out
}
func deliveryRecentlySent(log map[string]string, key string, now time.Time, interval time.Duration) bool {
	return logWithin(log[key], now, interval)
}
func deliveryRecentlyAttempted(log map[string]string, key string, now time.Time, interval time.Duration) bool {
	return logWithin(log[key], now, interval)
}
func hasRecentDelivery(log map[string]string, key string, now time.Time, interval time.Duration) bool {
	return deliveryRecentlySent(log, key, now, interval)
}
func logWithin(value string, now time.Time, interval time.Duration) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}
	stamp, err := parseTimestamp(value)
	if err != nil {
		return false
	}
	elapsed := now.Sub(stamp)
	return elapsed >= 0 && elapsed < interval
}
func pruneDeliveryLog(log map[string]string, now time.Time, retention time.Duration) map[string]string {
	pruned := make(map[string]string, len(log))
	for key, value := range log {
		stamp, err := parseTimestamp(value)
		if err == nil && now.Sub(stamp) >= 0 && now.Sub(stamp) < retention {
			pruned[key] = value
		}
	}
	return pruned
}
func pushDelivered(result map[string]any) bool {
	if result == nil {
		return false
	}
	switch sent := result["sent"].(type) {
	case int:
		return sent > 0
	case float64:
		return sent > 0
	}
	return false
}
func withinReminderWindow(now time.Time) bool {
	hour := now.In(saoPauloLocation()).Hour()
	return hour >= 9 && hour < 20
}

func maintenanceReminderBucket(expectedDate string, intervalDays int, today time.Time) int {
	due, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(expectedDate), today.Location())
	if err != nil || intervalDays < 1 {
		return 0
	}
	startToday := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())
	startDue := time.Date(due.Year(), due.Month(), due.Day(), 0, 0, 0, 0, today.Location())
	if !startToday.After(startDue) {
		return 0
	}
	daysLate := int(startToday.Sub(startDue).Hours() / 24)
	return daysLate / intervalDays
}

func maintenanceReminderExpiry(expectedDate string, bucket, intervalDays int) time.Time {
	location := saoPauloLocation()
	due, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(expectedDate), location)
	if err != nil || intervalDays < 1 || bucket < 0 {
		return time.Now().UTC().Add(7 * 24 * time.Hour)
	}
	return due.AddDate(0, 0, (bucket+1)*intervalDays).UTC()
}

func appointmentExpiry(date, clock string, grace time.Duration) time.Time {
	location := saoPauloLocation()
	parsedDate, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(date), location)
	if err != nil {
		return time.Now().UTC().Add(24 * time.Hour)
	}
	clock = strings.TrimSpace(clock)
	if len(clock) >= 5 {
		if hour, hourErr := strconv.Atoi(clock[:2]); hourErr == nil {
			if minute, minuteErr := strconv.Atoi(clock[3:5]); minuteErr == nil {
				return time.Date(parsedDate.Year(), parsedDate.Month(), parsedDate.Day(), hour, minute, 0, 0, location).Add(grace).UTC()
			}
		}
	}
	return time.Date(parsedDate.Year(), parsedDate.Month(), parsedDate.Day(), 23, 59, 0, 0, location).Add(grace).UTC()
}
func mustJSON(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}
func parseTimestamp(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.000Z", "2006-01-02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, errors.New("invalid timestamp")
}
func firstWord(name string) string {
	parts := strings.Fields(strings.TrimSpace(name))
	if len(parts) == 0 {
		return "Cliente"
	}
	return parts[0]
}
func formatDate(value string) string {
	value = domainDate(value)
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return ""
	}
	return date.Format("02/01/2006")
}
func domainDate(value string) string {
	if len(value) >= 10 {
		value = value[:10]
	}
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return ""
	}
	return value
}
func errorText(err error) string {
	if err == nil {
		return "ok"
	}
	return short(err)
}
func short(err error) string {
	value := strings.NewReplacer("\r", " ", "\n", " ").Replace(err.Error())
	if len(value) > 300 {
		value = value[:300]
	}
	return value
}
func parseInt(value string) (int, error) {
	return strconv.Atoi(value)
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func serviceDateQuery(date string) string {
	return "/rest/v1/services?" + url.Values{"status": {"eq.AGENDADO"}, "data_agendamento": {"eq." + date}}.Encode()
}

