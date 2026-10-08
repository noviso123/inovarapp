package webapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/domain"
)

var teamFinanceCompletionMarker = regexp.MustCompile(`\[DATA_CONCLUSAO:(\d{4}-\d{2}-\d{2})\]`)

type teamOperationsData struct {
	services             []map[string]any
	appointments         []map[string]any
	customers            []map[string]any
	budgets              []domain.BudgetEstimate
	returnHistory        []map[string]any
	standaloneHistory    []map[string]any
	profile              domain.TechnicianProfile
	servicesErr          error
	appointmentsErr      error
	customersErr         error
	budgetsErr           error
	returnHistoryErr     error
	standaloneHistoryErr error
	resourceError        string
	err                  error
}

// Independent dashboard reads run together so the screen waits for the
// slowest resource once instead of adding each request's latency in sequence.
func loadTeamOperationsData(ctx context.Context, baseURL, token string) teamOperationsData {
	var data teamOperationsData
	var wg sync.WaitGroup
	readRows := func(endpoint string, target *[]map[string]any, targetErr *error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rows, err := getTeamRows(ctx, baseURL+endpoint, token)
			*target, *targetErr = rows, err
		}()
	}
	readRows("/api/servicos", &data.services, &data.servicesErr)
	readRows("/api/agendamentos", &data.appointments, &data.appointmentsErr)
	readRows("/api/clientes", &data.customers, &data.customersErr)
	readRows("/api/historico?origem=retornos", &data.returnHistory, &data.returnHistoryErr)
	readRows("/api/historico?origem=retornos-avulsos", &data.standaloneHistory, &data.standaloneHistoryErr)
	wg.Add(2)
	go func() {
		defer wg.Done()
		data.budgets, data.budgetsErr = getTeamBudgets(ctx, baseURL+"/api/orcamentos", token)
	}()
	go func() {
		defer wg.Done()
		data.profile = getTeamProfile(ctx, baseURL+"/api/configuracoes", token)
	}()
	wg.Wait()
	for _, resourceErr := range []struct {
		name string
		err  error
	}{
		{"serviços", data.servicesErr},
		{"agenda", data.appointmentsErr},
		{"clientes e aparelhos", data.customersErr},
		{"orçamentos", data.budgetsErr},
		{"histórico de retornos", data.returnHistoryErr},
		{"histórico avulso", data.standaloneHistoryErr},
	} {
		if resourceErr.err != nil {
			data.resourceError, data.err = resourceErr.name, resourceErr.err
			return data
		}
	}
	return data
}

func (p *serviceCatalogPage) loadTeamOperations(ctx app.Context) {
	if p.caller == nil || p.session == nil || (p.caller.Role != "ADMIN" && p.caller.Role != "TECNICO") || p.teamLoading || p.teamLoadedFor == p.caller.UserID {
		return
	}
	userID, token, role := p.caller.UserID, p.session.AccessToken, p.caller.Role
	p.loadTeamCalendar(ctx)
	pageURL := app.Window().URL()
	if pageURL == nil || pageURL.Scheme == "" || pageURL.Host == "" {
		return
	}
	baseURL := apiBaseURL()
	p.teamLoading, p.teamError = true, ""
	failedResource := "serviços"
	go func() {
		pendingSyncErr := syncOfflineTeamSettings(ctx, baseURL, token, userID, role)
		if pendingSyncErr == nil {
			pendingSyncErr = syncOfflineTeamCustomers(ctx, baseURL, token, userID, role)
		}
		if pendingSyncErr == nil {
			pendingSyncErr = syncOfflineTeamServices(ctx, baseURL, token, userID, role)
		}
		if pendingSyncErr == nil {
			pendingSyncErr = syncOfflineTeamBudgets(ctx, baseURL, token, userID, role)
		}
		if pendingSyncErr == nil {
			pendingSyncErr = syncOfflineTeamMutations(ctx, baseURL, token, userID, role)
		}
		if pendingSyncErr != nil && !isOfflineNetworkError(pendingSyncErr) {
			p.teamError = "Há cadastros offline aguardando sincronização. Os dados locais foram preservados."
		}
		data := loadTeamOperationsData(ctx, baseURL, token)
		err := data.err
		failedResource = data.resourceError
		if err == nil {
			services, appointments, customers, budgets := data.services, data.appointments, data.customers, data.budgets
			returnHistory := append(data.returnHistory, data.standaloneHistory...)
			p.teamServices, p.teamAppointments, p.teamCustomers, p.teamBudgets, p.teamReturnHistory = services, appointments, customers, budgets, returnHistory
			p.teamProfile = data.profile
			cachedSnapshot, snapshotErr := loadOfflineSnapshot(userID)
			if snapshotErr == nil {
				p.teamProfile = *mergeOfflineTeamProfile(offlineProfile(p.teamProfile), cachedSnapshot.PendingSettings)
			}
			if p.teamProfile.DefaultReturnMonths == 0 {
				p.teamProfile.DefaultReturnMonths = 3
			}
			if p.teamProfile.DefaultWarrantyDays == 0 {
				p.teamProfile.DefaultWarrantyDays = 90
			}
			if p.teamProfile.DefaultPrice == 0 {
				p.teamProfile.DefaultPrice = 250
			}
			p.teamDashboard = buildTeamDashboardSummary(customers, services, appointments, budgets, returnHistory, p.teamProfile.DefaultReturnMonths, dashboardCivilToday(time.Now()))
			p.teamLoadedFor = userID
			p.offlineMode = false
			if snapshotErr == nil {
				p.teamServices, p.teamAppointments = mergePendingOfflineServices(services, appointments, cachedSnapshot.PendingServices)
				p.teamCustomers = mergePendingOfflineCustomers(customers, cachedSnapshot.PendingCustomers)
				p.teamBudgets = mergePendingOfflineBudgets(budgets, cachedSnapshot.PendingBudgets)
				projection := cachedSnapshot
				projection.TeamServices, projection.TeamAppointments = p.teamServices, p.teamAppointments
				projection.TeamCustomers, projection.TeamBudgets = p.teamCustomers, p.teamBudgets
				applyPendingOfflineTeamMutations(&projection)
				p.teamServices, p.teamAppointments = projection.TeamServices, projection.TeamAppointments
				p.teamCustomers, p.teamBudgets = projection.TeamCustomers, projection.TeamBudgets
				p.teamDashboard = buildTeamDashboardSummary(p.teamCustomers, p.teamServices, p.teamAppointments, p.teamBudgets, returnHistory, p.teamProfile.DefaultReturnMonths, dashboardCivilToday(time.Now()))
			}
			cacheTeamSnapshot(userID, role, services, appointments, customers, returnHistory, budgets, p.teamProfile)
		}
		if err != nil && isOfflineNetworkError(err) {
			if snapshot, cacheErr := loadOfflineSnapshot(userID); cacheErr == nil && snapshot.Role == role && snapshot.TeamProfile != nil {
				p.teamServices, p.teamAppointments, p.teamCustomers = snapshot.TeamServices, snapshot.TeamAppointments, snapshot.TeamCustomers
				p.teamBudgets, p.teamReturnHistory, p.teamProfile = snapshot.TeamBudgets, snapshot.TeamHistory, *snapshot.TeamProfile
				p.teamLoadedFor, p.offlineMode = userID, true
				p.teamDashboard = buildTeamDashboardSummary(p.teamCustomers, p.teamServices, p.teamAppointments, p.teamBudgets, p.teamReturnHistory, p.teamProfile.DefaultReturnMonths, dashboardCivilToday(time.Now()))
				p.teamError = ""
			} else {
				p.teamError = "Não foi possível acessar o banco ao carregar " + failedResource + ". Os dados deste dispositivo foram preservados. Verifique a conexão e tente novamente."
			}
		} else if err != nil {
			p.teamError = "A API ou o banco recusou a consulta de " + failedResource + ". Nenhum cadastro foi apagado. Confira sua sessão e as permissões da integração."
		}
		p.teamLoading = false
		ctx.Update()
		if pendingSyncErr != nil {
			p.schedulePendingTeamMutationRetry(ctx)
		}
	}()
}

// Starts a replay attempt after a new local mutation is queued. A failed
// attempt schedules another load, so the queue drains automatically once the
// browser regains connectivity.
func (p *serviceCatalogPage) retryPendingOfflineMutations(ctx app.Context) {
	p.teamLoadedFor = ""
	p.loadTeamOperations(ctx)
	p.schedulePendingTeamMutationRetry(ctx)
}

func (p *serviceCatalogPage) retryTeamData(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.teamLoading || p.session == nil || p.caller == nil {
		return
	}
	p.teamLoadedFor = ""
	p.loadTeamOperations(ctx)
	ctx.Update()
}

func (p *serviceCatalogPage) schedulePendingTeamMutationRetry(ctx app.Context) {
	if p.teamMutationRetryScheduled {
		return
	}
	p.teamMutationRetryScheduled = true
	ctx.After(30*time.Second, func(next app.Context) {
		p.teamMutationRetryScheduled = false
		if p.session == nil || p.caller == nil || !hasPendingOfflineTeamMutations(p.caller.UserID) {
			return
		}
		if p.teamLoading {
			p.schedulePendingTeamMutationRetry(next)
			return
		}
		p.teamLoadedFor = ""
		p.loadTeamOperations(next)
	})
}

func isOfflineNetworkError(err error) bool {
	var networkErr *url.Error
	return errors.As(err, &networkErr)
}

func getTeamProfile(ctx context.Context, endpoint, token string) domain.TechnicianProfile {
	profile := domain.TechnicianProfile{}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return profile
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return profile
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return profile
	}
	var result struct {
		Config json.RawMessage `json:"config"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result) == nil {
		_ = json.Unmarshal(result.Config, &profile)
	}
	return profile
}

func getTeamRows(ctx context.Context, endpoint, token string) ([]map[string]any, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, io.ErrUnexpectedEOF
	}
	rows := []map[string]any{}
	err = json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&rows)
	return rows, err
}

func getTeamBudgets(ctx context.Context, endpoint, token string) ([]domain.BudgetEstimate, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, io.ErrUnexpectedEOF
	}
	budgets := []domain.BudgetEstimate{}
	err = json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&budgets)
	return budgets, err
}

func (p *serviceCatalogPage) startTeamService(serviceID string) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		if serviceID == "" || p.session == nil {
			return
		}
		p.updateTeamService(ctx, serviceID, map[string]any{"status": "EM_ANDAMENTO", "data_inicio": time.Now().Format("2006-01-02")}, "Não foi possível iniciar o serviço.")
	}
}

func (p *serviceCatalogPage) requestTeamCancellation(serviceID string) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		p.teamCancelServiceID = serviceID
		p.teamActionMessage = ""
		ctx.Update()
	}
}

func (p *serviceCatalogPage) closeTeamCancellation(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if !p.teamCancelSaving {
		p.teamCancelServiceID = ""
		ctx.Update()
	}
}

func (p *serviceCatalogPage) teamCancellationDialog() app.UI {
	serviceID := p.teamCancelServiceID
	if serviceID == "" {
		return app.Div()
	}
	clientName := "este cliente"
	for _, service := range p.teamServices {
		if portalText(service["id"]) != serviceID {
			continue
		}
		if customer, ok := service["customers"].(map[string]any); ok {
			clientName = firstNonEmptyBudget(portalText(customer["nome"]), clientName)
		}
		break
	}
	return app.Div().Class("auth-backdrop").Body(app.Div().Class("auth-dialog").Role("dialog").Body(
		app.H2().Class("auth-dialog__title").Body(app.Text("Cancelar ordem de serviço?")),
		app.P().Class("auth-dialog__intro").Body(app.Text("A OS de "+clientName+" será marcada como cancelada. Deseja continuar?")),
		formErrorNotice(p.teamActionMessage),
		app.Div().Class("portal-budget__actions").Body(
			app.Button().Class("auth-link").Type("button").Disabled(p.teamCancelSaving).OnClick(p.closeTeamCancellation).Body(app.Text("Não, manter OS")),
			app.Button().Class("auth-submit").Type("button").Disabled(p.teamCancelSaving).OnClick(p.cancelTeamService(serviceID)).Body(app.Text("Sim, cancelar OS")),
		),
	))
}

func (p *serviceCatalogPage) cancelTeamService(serviceID string) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		if p.teamCancelSaving || p.session == nil || serviceID == "" {
			return
		}
		pageURL := app.Window().URL()
		if pageURL == nil || pageURL.Scheme == "" || pageURL.Host == "" {
			p.teamActionMessage = "Não foi possível conectar ao servidor."
			ctx.Update()
			return
		}
		var service map[string]any
		for _, row := range p.teamServices {
			if portalText(row["id"]) == serviceID {
				service = row
				break
			}
		}
		if service == nil {
			p.teamActionMessage = "Não foi possível identificar a ordem de serviço."
			ctx.Update()
			return
		}
		client, _ := service["customers"].(map[string]any)
		profile := p.teamProfile
		now := time.Now()
		date := now.Format("2006-01-02")
		cancellation, err := domain.BuildServiceCancellation("Cancelado pelo técnico", portalText(service["observacoes"]), now)
		if err != nil {
			p.teamActionMessage = "Não foi possível preparar o cancelamento."
			ctx.Update()
			return
		}
		baseURL, token := apiBaseURL(), p.session.AccessToken
		p.teamCancelSaving = true
		ctx.Update()
		go func() {
			fields := map[string]any{"status": "CANCELADO", "data_cancelamento": date, "motivo_cancelamento": cancellation.Reason}
			failure := sendTeamJSON(ctx, baseURL+"/api/servicos", token, http.MethodPatch, map[string]any{"id": serviceID, "fields": fields})
			if failure != nil {
				fallbackNotes, buildErr := domain.BuildServiceCancellation(cancellation.Reason, portalText(service["observacoes"]), now)
				if buildErr != nil {
					failure = buildErr
				} else {
					failure = sendTeamJSON(ctx, baseURL+"/api/servicos", token, http.MethodPatch, map[string]any{"id": serviceID, "fields": map[string]any{"status": "CANCELADO", "observacoes": fallbackNotes.Observations}})
				}
			}
			if failure != nil {
				p.teamActionMessage = "Não foi possível cancelar a OS. Tente novamente."
				p.teamCancelSaving = false
				ctx.Update()
				return
			}
			message := teamLegacyStatusMessage(profile, service, client, "status_cancelado")
			notice := "Serviço cancelado."
			if phone := portalText(client["whatsapp"]); phone != "" && message != "" {
				result, sendErr := sendTeamJSONResult(ctx, baseURL+"/api/whatsapp", token, http.MethodPost, map[string]any{"acao": "enviar", "evento": "servico_cancelado", "origem_tipo": "servico", "origem_id": serviceID, "telefone": phone, "texto": message})
				if sendErr != nil || result["ok"] != true {
					notice = "Serviço cancelado. Não foi possível enviar a notificação pelo WhatsApp."
				}
			}
			p.teamActionMessage, p.teamCancelSaving = notice, false
			p.teamCancelServiceID = ""
			p.teamLoadedFor = ""
			p.loadTeamOperations(ctx)
			ctx.Update()
		}()
	}
}

// teamLegacyStatusMessage mirrors the legacy agenda's status-notification text.
// That UI uses the persisted service type with only its first underscore changed.
func teamLegacyStatusMessage(profile domain.TechnicianProfile, service, client map[string]any, key string) string {
	template := domain.ResolveWhatsAppTemplate(profile.WhatsAppMessages, key)
	name := portalText(client["nome"])
	if fields := strings.Fields(name); len(fields) > 0 {
		name = fields[0]
	}
	serviceType := strings.Replace(portalText(service["tipo"]), "_", " ", 1)
	business := profile.BusinessName
	if business == "" {
		business = "Inovar Refrigeração"
	}
	return domain.ApplyWhatsAppPlaceholders(template, map[string]string{
		"cliente": name, "servico": serviceType, "empresa": business,
	})
}

func teamServiceStatusMessage(profile domain.TechnicianProfile, service, client map[string]any, key string) string {
	template := domain.ResolveWhatsAppTemplate(profile.WhatsAppMessages, key)
	name := portalText(client["nome"])
	if fields := strings.Fields(name); len(fields) > 0 {
		name = fields[0]
	}
	serviceType := portalText(service["descricao"])
	if serviceType == "" {
		serviceType = string(supabase.MapSupabaseServiceTypeToLocal(portalText(service["tipo"])))
	}
	business := profile.BusinessName
	if business == "" {
		business = "Inovar Refrigeração"
	}
	values := map[string]string{"cliente": name, "servico": serviceType, "empresa": business}
	return domain.ApplyWhatsAppPlaceholders(template, values)
}

func (p *serviceCatalogPage) openTeamSchedule(service map[string]any) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		serviceID := portalText(service["id"])
		if strings.EqualFold(portalText(service["status"]), "PENDENTE") {
			p.teamCallScheduleServiceID = serviceID
			p.teamCallScheduleDate = teamCallScheduleDefaultDate(time.Now())
			p.teamCallScheduleTime = "09:00"
			p.teamCallScheduleNotes = ""
			p.teamCallScheduleMessage = ""
			ctx.Update()
			return
		}
		p.teamScheduleServiceID = serviceID
		status := strings.ToUpper(portalText(service["status"]))
		p.teamScheduleDate = portalText(service["data_agendamento"])
		p.teamScheduleTime = portalText(service["hora_agendamento"])
		if status == "CONCLUIDO" || p.teamScheduleDate == "" {
			p.teamScheduleDate = teamCallScheduleDefaultDate(time.Now())
			if p.teamScheduleTime == "" {
				p.teamScheduleTime = "09:00"
			}
		}
		for _, appointment := range p.teamAppointments {
			if portalText(appointment["service_id"]) == serviceID {
				if value := portalText(appointment["data"]); value != "" {
					p.teamScheduleDate = value
				}
				if value := portalText(appointment["hora"]); value != "" {
					p.teamScheduleTime = value
				}
			}
		}
		p.teamActionMessage = ""
		ctx.Update()
	}
}

func (p *serviceCatalogPage) closeTeamSchedule(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if !p.teamScheduleSaving {
		p.teamScheduleServiceID = ""
		p.teamActionMessage = ""
	}
	ctx.Update()
}

func (p *serviceCatalogPage) teamScheduleDialog() app.UI {
	serviceID := p.teamScheduleServiceID
	if serviceID == "" {
		return app.Div()
	}
	var service map[string]any
	for _, candidate := range p.teamServices {
		if portalText(candidate["id"]) == serviceID {
			service = candidate
			break
		}
	}
	if service == nil {
		return app.Div().Class("auth-backdrop").Body(app.Div().Class("auth-dialog").Role("dialog").Body(
			app.H2().Class("auth-dialog__title").Body(app.Text("Agendamento indisponível")),
			app.P().Class("auth-dialog__intro").Body(app.Text("Não foi possível localizar esta ordem. Atualize a Central e tente novamente.")),
			app.Button().Class("auth-submit").Type("button").OnClick(p.closeTeamSchedule).Body(app.Text("Fechar")),
		))
	}
	status := strings.ToUpper(portalText(service["status"]))
	title, saveLabel := "Reagendar serviço", "Salvar reagendamento"
	if status == "CONCLUIDO" {
		title, saveLabel = "Reabrir e reagendar", "Sim, reabrir e reagendar"
	}
	client, _ := service["customers"].(map[string]any)
	clientName := firstNonEmptyBudget(portalText(client["nome"]), "Cliente")
	serviceName := firstNonEmptyBudget(portalText(service["descricao"]), portalText(service["tipo"]), "Serviço")
	return app.Div().Class("auth-backdrop").Body(app.Div().Class("auth-dialog team-reschedule-dialog").Role("dialog").Body(
		app.H2().Class("auth-dialog__title").Body(app.Text(title)),
		app.P().Class("auth-dialog__intro").Body(app.Text(clientName+" • "+serviceName)),
		app.P().Class("auth-dialog__intro").Body(app.Text("Ao confirmar, a ordem voltará para Agendada na data escolhida.")),
		app.Form().Class("auth-form").OnSubmit(p.saveTeamSchedule(serviceID)).Body(
			app.Label().Class("auth-field").Body(app.Text("Nova data *"), app.Input().Type("date").Required(true).Value(p.teamScheduleDate).Disabled(p.teamScheduleSaving).OnChange(p.ValueTo(&p.teamScheduleDate))),
			app.Label().Class("auth-field").Body(app.Text("Horário"), app.Input().Type("time").Value(p.teamScheduleTime).Disabled(p.teamScheduleSaving).OnChange(p.ValueTo(&p.teamScheduleTime))),
			formErrorNotice(p.teamActionMessage),
			app.Div().Class("portal-budget__actions").Body(
				app.Button().Class("auth-link").Type("button").Disabled(p.teamScheduleSaving).OnClick(p.closeTeamSchedule).Body(app.Text("Não, voltar")),
				app.Button().Class("auth-submit").Type("submit").Disabled(p.teamScheduleSaving).Body(app.Text(saveLabel)),
			),
		),
	))
}

func (p *serviceCatalogPage) closeTeamCallSchedule(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if !p.teamScheduleSaving {
		p.teamCallScheduleServiceID = ""
		p.teamCallScheduleMessage = ""
	}
	ctx.Update()
}

func (p *serviceCatalogPage) saveTeamCallSchedule(ctx app.Context, event app.Event) {
	event.PreventDefault()
	serviceID := p.teamCallScheduleServiceID
	if serviceID == "" || p.session == nil || p.teamScheduleSaving {
		return
	}
	if _, err := time.Parse("2006-01-02", p.teamCallScheduleDate); err != nil {
		p.teamCallScheduleMessage = "Informe uma data válida."
		ctx.Update()
		return
	}
	if _, err := time.Parse("15:04", p.teamCallScheduleTime); err != nil {
		p.teamCallScheduleMessage = "Informe um horário válido."
		ctx.Update()
		return
	}
	var service map[string]any
	for _, row := range p.teamServices {
		if portalText(row["id"]) == serviceID {
			service = row
			break
		}
	}
	if service == nil {
		p.teamCallScheduleMessage = "Não foi possível identificar o chamado. Atualize a tela e tente novamente."
		ctx.Update()
		return
	}
	var appointment map[string]any
	for _, row := range p.teamAppointments {
		if portalText(row["service_id"]) == serviceID {
			appointment = row
			break
		}
	}
	date, hour, notes := p.teamCallScheduleDate, p.teamCallScheduleTime, strings.TrimSpace(p.teamCallScheduleNotes)
	token, baseURL := p.session.AccessToken, apiBaseURL()
	clientID := portalText(service["cliente_id"])
	profile := p.teamProfile
	p.teamScheduleSaving, p.teamCallScheduleMessage = true, "Salvando agendamento..."
	ctx.Update()
	go func() {
		serviceFields := map[string]any{"status": "AGENDADO", "data_agendamento": date, "hora_agendamento": hour}
		err := sendTeamJSON(ctx, baseURL+"/api/servicos", token, http.MethodPatch, map[string]any{"id": serviceID, "fields": serviceFields})
		if err == nil {
			if appointment != nil {
				err = sendTeamJSON(ctx, baseURL+"/api/agendamentos", token, http.MethodPatch, map[string]any{
					"service_id": serviceID, "fields": map[string]any{"data": date, "hora": hour},
				})
			} else {
				err = sendTeamJSON(ctx, baseURL+"/api/agendamentos", token, http.MethodPost, teamCallScheduleAppointmentPayload(serviceID, clientID, date, hour, notes))
			}
		}
		if p.session == nil || p.session.AccessToken != token {
			return
		}
		p.teamScheduleSaving = false
		if err != nil {
			p.teamCallScheduleMessage = "Erro ao agendar — tente novamente."
			p.teamActionMessage = ""
			ctx.Update()
			return
		}
		p.teamCallScheduleServiceID = ""
		p.teamCallScheduleMessage = ""
		p.teamActionMessage = "Atendimento agendado para " + appointmentDateBR(date) + " às " + hour + "."
		p.registerLocalNotification(ctx, "Atendimento agendado", appointmentDateBR(date)+" às "+hour)
		p.teamLoadedFor = ""
		p.loadTeamOperations(ctx)
		ctx.Update()
		if client := appointmentClient(p.teamCustomers, clientID); client != nil && portalText(client["whatsapp"]) != "" {
			message := teamScheduledCallMessage(profile, portalText(client["nome"]), portalText(service["tipo"]), date, hour)
			waResult, waErr := sendTeamJSONResult(ctx, baseURL+"/api/whatsapp", token, http.MethodPost, map[string]any{
				"acao": "enviar", "evento": "retorno_agendado", "origem_tipo": "servico", "origem_id": serviceID, "telefone": client["whatsapp"], "texto": message,
			})
			if p.session == nil || p.session.AccessToken != token {
				return
			}
			if waErr == nil {
				if sent, _ := waResult["ok"].(bool); sent {
					p.teamActionMessage += " Confirmação registrada na fila do WhatsApp do cliente."
				} else {
					p.teamActionMessage += " Não foi possível enviar a confirmação pelo WhatsApp."
				}
			} else {
				p.teamActionMessage += " Não foi possível enviar a confirmação pelo WhatsApp."
			}
			ctx.Update()
		}
	}()
}

func teamCallScheduleDefaultDate(now time.Time) string {
	return now.AddDate(0, 0, 1).Format("2006-01-02")
}

func teamCallScheduleAppointmentPayload(serviceID, clientID, date, hour, notes string) map[string]any {
	return map[string]any{"service_id": serviceID, "cliente_id": clientID, "data": date, "hora": hour, "status": "AGENDADO", "observacoes": notes}
}

func teamScheduleOriginalDate(service, appointment map[string]any) string {
	date := portalText(service["data_agendamento"])
	if date == "" {
		date = portalText(appointment["data"])
	}
	if len(date) >= 10 {
		return date[:10]
	}
	return date
}

func teamScheduleOriginalTime(service, appointment map[string]any) string {
	hour := portalText(service["hora_agendamento"])
	if hour == "" && appointment != nil {
		hour = portalText(appointment["hora"])
	}
	return normalizeTeamScheduleTime(hour)
}

func normalizeTeamScheduleTime(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 5 {
		if _, err := time.Parse("15:04", value[:5]); err == nil {
			return value[:5]
		}
	}
	return value
}

func teamScheduleShouldNotify(oldDate, oldHour, newDate, newHour string) bool {
	if strings.TrimSpace(oldDate) != strings.TrimSpace(newDate) {
		return true
	}
	newHour = normalizeTeamScheduleTime(newHour)
	return newHour != "" && normalizeTeamScheduleTime(oldHour) != newHour
}

func teamRescheduledServiceMessage(profile domain.TechnicianProfile, clientName, serviceType, date, hour string) string {
	name := strings.TrimSpace(clientName)
	if parts := strings.Fields(name); len(parts) > 0 {
		name = parts[0]
	}
	serviceType = strings.Replace(serviceType, "_", " ", 1)
	hour = normalizeTeamScheduleTime(hour)
	if hour == "" {
		hour = "a confirmar"
	}
	return domain.ApplyWhatsAppPlaceholders(domain.ResolveWhatsAppTemplate(profile.WhatsAppMessages, "reagendado"), map[string]string{
		"cliente": name, "servico": serviceType, "data": appointmentDateBR(date), "hora": hour,
		"empresa": firstNonEmptyBudget(profile.BusinessName, "Inovar Refrigeração"),
	})
}

func teamScheduledCallMessage(profile domain.TechnicianProfile, clientName, serviceType, date, hour string) string {
	name := strings.TrimSpace(clientName)
	if parts := strings.Fields(name); len(parts) > 0 {
		name = parts[0]
	}
	serviceType = strings.Replace(serviceType, "_", " ", 1)
	template := domain.ResolveWhatsAppTemplate(profile.WhatsAppMessages, "chamado_agendado")
	return domain.ApplyWhatsAppPlaceholders(template, map[string]string{
		"cliente": name, "servico": serviceType, "data": appointmentDateBR(date), "hora": hour,
		"empresa": firstNonEmptyBudget(profile.BusinessName, "Inovar Refrigeração"), "app": domain.AppPublicURL,
	})
}

func (p *serviceCatalogPage) teamCallScheduleDialog() app.UI {
	serviceID := p.teamCallScheduleServiceID
	if serviceID == "" {
		return app.Div()
	}
	var service map[string]any
	for _, row := range p.teamServices {
		if portalText(row["id"]) == serviceID {
			service = row
			break
		}
	}
	clientName, serviceName, applianceName := "Cliente", "Solicitação", "não informado"
	if service != nil {
		if customer, ok := service["customers"].(map[string]any); ok {
			clientName = firstNonEmptyBudget(portalText(customer["nome"]), clientName)
		}
		serviceName = firstNonEmptyBudget(portalText(service["descricao"]), portalText(service["tipo"]), serviceName)
		if appliance, ok := service["air_conditioners"].(map[string]any); ok {
			applianceName = firstNonEmptyBudget(strings.TrimSpace(portalText(appliance["marca"])+" "+portalText(appliance["modelo"])), portalText(appliance["ambiente"]), applianceName)
		}
	}
	return app.Div().Class("auth-backdrop").Body(app.Div().Class("auth-dialog").Body(
		app.H2().Class("auth-dialog__title").Body(app.Text("Agendar Atendimento")),
		app.P().Class("auth-dialog__intro").Body(app.Text(clientName+" • "+serviceName)),
		app.Form().Class("auth-form").OnSubmit(p.saveTeamCallSchedule).Body(
			app.P().Class("auth-notice").Body(app.Text("Aparelho: "+applianceName)),
			app.Div().Class("service-grid").Body(
				app.Label().Class("auth-field").Body(app.Text("Data do Atendimento *"), app.Input().Type("date").Required(true).Value(p.teamCallScheduleDate).Disabled(p.teamScheduleSaving).OnChange(p.ValueTo(&p.teamCallScheduleDate))),
				app.Label().Class("auth-field").Body(app.Text("Horário *"), app.Input().Type("time").Required(true).Value(p.teamCallScheduleTime).Disabled(p.teamScheduleSaving).OnChange(p.ValueTo(&p.teamCallScheduleTime))),
			),
			app.Label().Class("auth-field").Body(app.Text("Observações do agendamento"), app.Textarea().Rows(2).Text(p.teamCallScheduleNotes).Placeholder("Ex: confirmar chegada 15 min antes...").Disabled(p.teamScheduleSaving).OnChange(p.ValueTo(&p.teamCallScheduleNotes))),
			app.P().Class("auth-notice").Body(app.Text("Ao salvar: a confirmação será enviada pelo WhatsApp se a integração estiver disponível com data e horário, e o serviço entrará na Agenda.")),
			formErrorNotice(p.teamCallScheduleMessage),
			app.Div().Class("portal-budget__actions").Body(
				app.Button().Class("auth-link").Type("button").Disabled(p.teamScheduleSaving).OnClick(p.closeTeamCallSchedule).Body(app.Text("Cancelar")),
				app.Button().Class("auth-submit").Type("submit").Disabled(p.teamScheduleSaving).Body(app.Text("Confirmar Agendamento")),
			),
		),
	))
}

func (p *serviceCatalogPage) saveTeamSchedule(serviceID string) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		if p.teamScheduleSaving {
			return
		}
		if p.session == nil {
			p.teamActionMessage = "Sua sessão expirou. Entre novamente para salvar o agendamento."
			ctx.Update()
			return
		}
		if _, err := time.Parse("2006-01-02", p.teamScheduleDate); err != nil {
			p.teamActionMessage = "Informe uma data válida para o reagendamento."
			ctx.Update()
			return
		}
		pageURL := app.Window().URL()
		if pageURL == nil || pageURL.Scheme == "" || pageURL.Host == "" {
			p.teamActionMessage = "Não foi possível conectar ao servidor."
			ctx.Update()
			return
		}
		baseURL, token := apiBaseURL(), p.session.AccessToken
		date, hour := p.teamScheduleDate, p.teamScheduleTime
		var service map[string]any
		for _, row := range p.teamServices {
			if portalText(row["id"]) == serviceID {
				service = row
				break
			}
		}
		if service == nil {
			p.teamActionMessage = "Não foi possível identificar o serviço."
			ctx.Update()
			return
		}
		var appointment map[string]any
		for _, row := range p.teamAppointments {
			if portalText(row["service_id"]) == serviceID {
				appointment = row
				break
			}
		}
		oldDate := teamScheduleOriginalDate(service, appointment)
		oldHour := teamScheduleOriginalTime(service, appointment)
		clientID := portalText(service["cliente_id"])
		client := appointmentClient(p.teamCustomers, clientID)
		profile := p.teamProfile
		serviceType := portalText(service["tipo"])
		hour = strings.TrimSpace(hour)
		if hour != "" {
			if _, err := time.Parse("15:04", hour); err != nil {
				p.teamActionMessage = "Informe um horário válido."
				ctx.Update()
				return
			}
		}
		phone := ""
		clientName := ""
		if client != nil {
			phone, clientName = portalText(client["whatsapp"]), portalText(client["nome"])
		}
		scheduleChanged := teamScheduleShouldNotify(oldDate, oldHour, date, hour)
		notificationHour := normalizeTeamScheduleTime(hour)
		if notificationHour == "" {
			notificationHour = oldHour
		}
		newSchedule := appointmentDateBR(date)
		if notificationHour != "" {
			newSchedule += " às " + notificationHour
		} else {
			newSchedule += " — horário a confirmar"
		}
		p.teamScheduleSaving, p.teamActionMessage = true, "Salvando reagendamento..."
		ctx.Update()
		go func() {
			serviceFields := map[string]any{"status": "AGENDADO", "data_agendamento": date}
			if hour != "" {
				serviceFields["hora_agendamento"] = hour
			}
			err := sendTeamJSON(ctx, baseURL+"/api/servicos", token, http.MethodPatch, map[string]any{"id": serviceID, "fields": serviceFields})
			if err == nil {
				appointmentFields := map[string]any{"data": date}
				if hour != "" {
					appointmentFields["hora"] = hour
				}
				if appointment != nil {
					appointmentFields["status"] = "AGENDADO"
					err = sendTeamJSON(ctx, baseURL+"/api/agendamentos", token, http.MethodPatch, map[string]any{"service_id": serviceID, "fields": appointmentFields})
				} else {
					appointmentFields["service_id"] = serviceID
					if clientID := portalText(service["cliente_id"]); clientID != "" {
						appointmentFields["cliente_id"] = clientID
					}
					appointmentFields["status"] = "AGENDADO"
					err = sendTeamJSON(ctx, baseURL+"/api/agendamentos", token, http.MethodPost, appointmentFields)
				}
			}
			p.teamScheduleSaving = false
			if err != nil {
				p.teamActionMessage = "A OS foi atualizada, mas não foi possível salvar o agendamento. Tente sincronizar novamente."
			} else {
				p.teamActionMessage = "Agendamento salvo."
				if scheduleChanged {
					p.registerLocalNotification(ctx, "Agendamento alterado", "Novo atendimento: "+newSchedule)
				}
				p.teamScheduleServiceID = ""
				p.teamLoadedFor = ""
				p.loadTeamOperations(ctx)
			}
			ctx.Update()
			if err == nil && scheduleChanged && phone != "" {
				message := teamRescheduledServiceMessage(profile, clientName, serviceType, date, notificationHour)
				result, sendErr := sendTeamJSONResult(ctx, baseURL+"/api/whatsapp", token, http.MethodPost, map[string]any{
					"acao": "enviar", "telefone": phone, "texto": message,
				})
				sent := false
				if sendErr == nil {
					sent, _ = result["ok"].(bool)
				}
				p.teamActionMessage += " " + teamRescheduleWhatsAppDeliveryNotice(sent)
				ctx.Update()
			}
		}()
	}
}

func teamRescheduleWhatsAppDeliveryNotice(sent bool) string {
	if sent {
		return "Aviso de reagendamento registrado na fila do WhatsApp."
	}
	return "Não foi possível avisar o cliente pelo WhatsApp."
}

func sendTeamJSON(ctx context.Context, endpoint, token, method string, payload any) error {
	_, err := sendTeamJSONResult(ctx, endpoint, token, method, payload)
	return err
}

func sendTeamJSONResult(ctx context.Context, endpoint, token, method string, payload any) (map[string]any, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, 16<<10))
		if readErr != nil {
			return nil, readErr
		}
		var problem map[string]any
		if json.Unmarshal(responseBody, &problem) == nil {
			if message, ok := problem["error"].(string); ok && strings.TrimSpace(message) != "" {
				return nil, fmt.Errorf("%s", strings.TrimSpace(message))
			}
			if message, ok := problem["mensagem"].(string); ok && strings.TrimSpace(message) != "" {
				return nil, fmt.Errorf("%s", strings.TrimSpace(message))
			}
		}
		return nil, fmt.Errorf("a API respondeu com status %d", response.StatusCode)
	}
	var result map[string]any
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if len(responseBody) > 0 {
		_ = json.Unmarshal(responseBody, &result)
	}
	return result, nil
}

func (p *serviceCatalogPage) updateTeamService(ctx app.Context, serviceID string, fields map[string]any, failure string) {
	pageURL := app.Window().URL()
	if pageURL == nil || p.session == nil {
		return
	}
	endpoint := apiBaseURL() + "/api/servicos"
	token := p.session.AccessToken
	payload := map[string]any{"id": serviceID, "fields": fields}
	p.teamError = ""
	go func() {
		body, _ := json.Marshal(payload)
		request, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(body))
		if err == nil {
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set("Content-Type", "application/json")
			var response *http.Response
			response, err = http.DefaultClient.Do(request)
			if err == nil {
				defer response.Body.Close()
				if response.StatusCode < 200 || response.StatusCode >= 300 {
					err = io.ErrUnexpectedEOF
				}
			}
		}
		if err != nil {
			if p.deferOfflineTeamMutation(endpoint, http.MethodPatch, payload, err) {
				p.teamError = "Alteração salva neste dispositivo e será sincronizada quando a conexão voltar."
			} else {
				p.teamError = failure
			}
		} else {
			p.teamError = ""
		}
		p.teamLoadedFor = ""
		if err == nil || p.offlineMode {
			p.loadTeamOperations(ctx)
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) teamOperationsSection() app.UI {
	view := p.teamActiveSection
	if view == "qr-code" {
		return app.Section().ID("team-operations").Class("team-view").Body(p.teamQRCodePanel())
	}
	if view == "" {
		view = "dashboard"
	}
	calendarItems := allTeamAgendaItems(p.teamServices, p.teamAppointments)
	today := time.Now().Format("2006-01-02")
	upcoming, overdue, completed := splitTeamAgenda(calendarItems, today)
	agenda := teamAgendaGroups{Upcoming: upcoming, Overdue: overdue, RecentlyCompleted: completed}
	content := []app.UI{}
	if p.teamCalendarNotice != "" {
		if view == "calendar" || view == "services" {
			content = append(content, app.Div().Class("team-calendar-notice").Role("status").Body(app.Text(p.teamCalendarNotice)))
		}
	}
	if p.teamLoading {
		content = append(content, app.P().Class("portal-section__empty").Body(app.Text("Carregando serviços e agenda...")))
	} else if p.teamError != "" {
		content = append(content, app.Div().Class("portal-section__error team-data-error").Role("alert").Body(
			app.P().Body(app.Text(p.teamError)),
			app.P().Body(app.Text("Os cadastros do banco não foram apagados por esta falha. Confira a conexão/permissões e tente carregar novamente.")),
			app.Button().Class("auth-link").Type("button").Disabled(p.teamLoading).OnClick(p.retryTeamData).Body(app.Text("Tentar carregar novamente")),
		))
	}
	if p.teamActionMessage != "" {
		content = append(content, app.P().Class("portal-section__intro").Body(app.Text(p.teamActionMessage)))
	}
	if p.teamNewAppointmentNotice != "" {
		content = append(content, app.P().Class("portal-section__intro").Body(app.Text(p.teamNewAppointmentNotice)))
	}
	if p.teamReceiptURL != "" {
		content = append(content, app.A().Class("auth-link").Href(p.teamReceiptURL).Target("_blank").Rel("noopener noreferrer").Body(app.Text("Abrir PDF da última OS")))
	}
	if view == "services" {
		content = append(content,
			app.Div().Class("team-view-heading").Body(app.Div().Body(app.P().Class("catalog__eyebrow").Body(app.Text("SERVIÇOS DE CAMPO")), app.H2().Class("catalog__title").Body(app.Text("Serviços de Climatização & Refrigeração")), app.P().Class("portal-section__intro").Body(app.Text("Serviços padronizados da Inovar para orçamento, agendamento e Ordem de Serviço.")))),
			app.Button().Class("auth-submit team-service-google-connect").Type("button").Disabled(p.teamCalendarBusy).OnClick(p.connectOrSyncGoogleCalendar).Body(app.Text(func() string {
				if p.teamCalendarBusy {
					return "Processando Google Agenda..."
				}
				if p.teamCalendarConnected {
					return "Sincronizar agora" + teamCalendarLastSyncLabel(p.teamCalendarLastSync)
				}
				return "Conectar Google Agenda"
			}())),
			app.Div().Class("team-service-actions").Body(
				app.Button().Class("auth-submit").Type("button").OnClick(p.openTeamAppointmentForm).Body(app.Text("＋ Agendar Retorno / Manutenção")),
				app.Button().Class("auth-link").Type("button").OnClick(p.openTeamDirectServiceForm).Body(app.Text("＋ Cadastrar Serviço (sem checklist)")),
				app.Button().Class("auth-submit").Type("button").OnClick(p.openTeamStartServiceForm).Body(app.Text("Iniciar Serviço (com checklist)")),
				app.Button().Class("auth-link").Type("button").OnClick(p.openTeamSettings).Body(app.Text("Configurações do app")),
			),
			p.teamServiceCatalogManager(),
		)
	}
	if view == "budgets" && p.teamBudgetOpenID != "" {
		for _, budget := range p.teamBudgets {
			if budget.ID == p.teamBudgetOpenID {
				content = append(content, app.Div().Class("auth-notice").Body(app.Strong().Body(app.Text("Orçamento • "+budget.ClientName)), app.P().Body(app.Text(firstNonEmptyBudget(budget.ApplianceDescription, "Serviço")+" • R$ "+formatPortalMoney(budget.FinalValue))), app.Button().Class("auth-link").Type("button").OnClick(func(ctx app.Context, event app.Event) { event.PreventDefault(); p.teamBudgetOpenID = ""; ctx.Update() }).Body(app.Text("Fechar detalhes"))))
				break
			}
		}
	}
	if view == "budgets" {
		content = append(content, p.detailedTeamBudgetsPanel())
	}
	if view == "finance" {
		content = append(content, p.teamFinancePanel())
	}
	if view == "dashboard" {
		content = append(content, p.teamDashboardCards()...)
		content = append(content, p.teamAgendaSections(agenda)...)
	}
	if view == "contact" {
		content = append(content, p.teamContactCenter())
	}
	if view == "whatsapp" {
		content = append(content, p.teamWhatsAppQueuePanel())
	}
	if view == "qr-code" {
		content = append(content, p.teamQRCodePanel())
	}
	serviceCards := make([]app.UI, 0, len(p.teamServices))
	for _, service := range p.teamServices {
		id, status := portalText(service["id"]), strings.ToUpper(portalText(service["status"]))
		storedType := portalText(service["tipo"])
		title := string(supabase.MapSupabaseServiceTypeToLocal(storedType))
		if storedType == "OUTRO" && portalText(service["descricao"]) != "" {
			title = portalText(service["descricao"])
		}
		if title == "" {
			title = "Atendimento"
		}
		customer := ""
		if row, ok := service["customers"].(map[string]any); ok {
			customer = portalText(row["nome"])
		}
		if customer != "" {
			title += " • " + customer
		}
		label, tone := portalServiceStatus(status)
		parts := []app.UI{
			app.Div().Class("portal-service__main").Body(
				app.Div().Class("portal-service__heading").Body(app.Strong().Body(app.Text(title)), app.Span().Class("portal-service__status portal-service__status--"+tone).Body(app.Text(label))),
				app.P().Class("portal-service__date").Body(app.Text("Data: "+portalDate(service["data_agendamento"]))),
			),
		}
		if status == "AGENDADO" {
			parts = append(parts, app.Button().Class("auth-submit").Type("button").OnClick(p.startTeamService(id)).Body(app.Text("Iniciar serviço")))
		}
		if status == "AGENDADO" || status == "EM_ANDAMENTO" {
			parts = append(parts, app.Button().Class("auth-submit").Type("button").OnClick(p.openTeamCompletion(service)).Body(app.Text("Finalizar serviço")))
		}
		if status == "PENDENTE" || status == "AGENDADO" || status == "EM_ANDAMENTO" {
			parts = append(parts, app.Button().Class("auth-link").Type("button").OnClick(p.requestTeamCancellation(id)).Body(app.Text("Cancelar OS")))
		}
		if status == "PENDENTE" || status == "AGENDADO" {
			label := "Agendar"
			if status == "AGENDADO" {
				label = "Reagendar"
			}
			parts = append(parts, app.Button().Class("auth-link").Type("button").OnClick(p.openTeamSchedule(service)).Body(app.Text(label)))
		}
		serviceCards = append(serviceCards, app.Article().Class("portal-service").Body(parts...))
	}
	if view == "services" {
		content = append(content, app.Div().ID("team-operations-services").Class("team-operations-services").Body(serviceCards...))
		if len(p.teamServices) == 0 && !p.teamLoading {
			content = append(content, app.P().Class("portal-section__empty").Body(app.Text("Nenhum serviço para exibir.")))
		}
	}
	if view == "calendar" {
		content = append(content, app.Div().Class("team-agenda__summary").Body(
			app.P().Class("team-agenda__summary-title").Body(app.Text("Agenda de Atendimentos")),
			app.P().Class("team-agenda__summary-subtitle").Body(app.Text("Alterne datas, conclua ou cancele — de qualquer plataforma")),
			app.Span().Class("team-agenda__summary-count").Body(app.Text(fmt.Sprintf("%d próximos", len(upcoming)))),
			app.Button().Class("auth-link team-agenda__google").Type("button").Disabled(p.teamCalendarBusy).OnClick(p.connectOrSyncGoogleCalendar).Body(app.Text(func() string {
				if p.teamCalendarBusy {
					return "Processando..."
				}
				if p.teamCalendarConnected {
					return "↻ Sincronizar Google Agenda"
				}
				return "▦ Conectar Google Agenda"
			}())),
		))
		if p.teamAgendaView == "week" {
			content = append(content, p.teamWeeklyCalendar(calendarItems))
		} else {
			content = append(content, p.teamMonthlyCalendar(calendarItems))
		}
		content = append(content, p.teamAgendaSections(agenda)...)
	}
	if p.teamCompletion != nil {
		content = append(content, p.teamCompletionDialog())
	}
	if p.teamServiceEdit != nil && !p.teamServiceReceiptEdit {
		content = append(content, p.teamServiceEditDialog())
	}
	if p.teamBudgetForm != nil {
		content = append(content, p.teamBudgetDialog())
	}
	if p.teamBudgetScheduleID != "" {
		content = append(content, p.teamBudgetScheduleDialog())
	}
	if p.teamNewAppointment != nil {
		content = append(content, p.teamAppointmentDialog())
	}
	if p.teamDirectServiceForm != nil {
		content = append(content, p.teamDirectServiceDialog())
	}
	if p.teamStartServiceForm != nil {
		content = append(content, p.teamStartServiceDialog())
	}
	if p.teamCallScheduleServiceID != "" {
		content = append(content, p.teamCallScheduleDialog())
	}
	if view == "customers" {
		content = append(content, p.teamCustomerRegistry())
	}
	if len(content) == 0 {
		content = append(content, app.P().Class("portal-section__empty").Body(app.Text("Selecione uma área do painel para começar.")))
	}
	return app.Section().ID("team-operations").Class("team-view team-view--"+view).Attr("aria-live", "polite").Body(content...)
}

func (p *serviceCatalogPage) teamFinancePanel() app.UI {
	month := p.teamFinanceMonth
	if month == "" {
		month = time.Now().Format("2006-01")
		p.teamFinanceMonth = month
	}
	monthLabel := month
	if len(month) >= 7 {
		monthLabel = month[5:7] + "/" + month[:4]
	}
	today := time.Now().Format("2006-01-02")
	servicesByID := map[string]map[string]any{}
	clientNames := map[string]string{}
	for _, customer := range p.teamCustomers {
		clientNames[portalText(customer["id"])] = portalText(customer["nome"])
	}
	completed, receivable := []map[string]any{}, []map[string]any{}
	receivedMonth, dueServices, receivableServices := 0.0, 0.0, 0.0
	completedTotals := map[string]float64{}
	for _, service := range p.teamServices {
		id := portalText(service["id"])
		servicesByID[id] = service
		status, value := strings.ToUpper(portalText(service["status"])), agendaNumber(service["valor"])
		if value <= 0 {
			continue
		}
		if status == "CONCLUIDO" {
			completed = append(completed, service)
			date := portalText(service["data_conclusao"])
			if date == "" {
				if match := teamFinanceCompletionMarker.FindStringSubmatch(portalText(service["observacoes"])); len(match) == 2 {
					date = match[1]
				}
			}
			if date == "" {
				date = portalText(service["data_agendamento"])
				if date == "" {
					date = portalText(service["data_solicitacao"])
				}
			}
			if p.teamFinanceAll || strings.HasPrefix(date, month) {
				receivedMonth += value
			}
		} else if status == "AGENDADO" || status == "EM_ANDAMENTO" {
			receivable = append(receivable, service)
			receivableServices += value
			date := portalText(service["data_agendamento"])
			if status == "AGENDADO" && len(date) >= 10 && date[:10] < today {
				dueServices += value
			}
		}
	}
	budgets := []domain.BudgetEstimate{}
	paidBudgets := []domain.BudgetEstimate{}
	receivableBudgets, paidBudgetTotal := 0.0, 0.0
	for _, budget := range p.teamBudgets {
		if budget.Paid != nil && *budget.Paid {
			paidBudgets = append(paidBudgets, budget)
			if budget.AmountReceived != nil {
				paidBudgetTotal += *budget.AmountReceived
			} else {
				paidBudgetTotal += budget.FinalValue
			}
			continue
		}
		if budget.Status != domain.BudgetApproved {
			continue
		}
		if budget.ServiceID != nil {
			if linked, ok := servicesByID[*budget.ServiceID]; ok && strings.EqualFold(portalText(linked["status"]), "CONCLUIDO") {
				continue
			}
		}
		budgets = append(budgets, budget)
		receivableBudgets += budget.FinalValue
	}
	if !p.teamFinanceAll {
		filtered := completed[:0]
		for _, service := range completed {
			date := portalText(service["data_conclusao"])
			if date == "" {
				if match := teamFinanceCompletionMarker.FindStringSubmatch(portalText(service["observacoes"])); len(match) == 2 {
					date = match[1]
				}
			}
			if date == "" {
				date = portalText(service["data_agendamento"])
				if date == "" {
					date = portalText(service["data_solicitacao"])
				}
			}
			if strings.HasPrefix(date, month) {
				filtered = append(filtered, service)
			}
		}
		completed = filtered
	}
	for _, service := range completed {
		completedTotals[portalText(service["cliente_id"])] += agendaNumber(service["valor"])
	}
	rows := []app.UI{}
	type financeCustomerGroup struct {
		id, name string
		total    float64
		services []map[string]any
	}
	groupByID := map[string]*financeCustomerGroup{}
	groups := []*financeCustomerGroup{}
	for _, customer := range p.teamCustomers {
		customerID := portalText(customer["id"])
		if total := completedTotals[customerID]; total > 0 {
			group := &financeCustomerGroup{id: customerID, name: firstNonEmptyBudget(portalText(customer["nome"]), "Cliente"), total: total}
			groupByID[customerID] = group
			groups = append(groups, group)
		}
	}
	for _, service := range completed {
		customerID := portalText(service["cliente_id"])
		group := groupByID[customerID]
		if group == nil {
			group = &financeCustomerGroup{id: customerID, name: "Cliente"}
			groupByID[customerID] = group
			groups = append(groups, group)
		}
		group.services = append(group.services, service)
	}
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].total > groups[j].total })
	for _, group := range groups {
		groupRows := []app.UI{app.Div().Class("portal-service__heading").Body(app.Strong().Body(app.Text(group.name)), app.Span().Class("portal-service__value").Body(app.Text("R$ "+formatPortalMoney(group.total))))}
		for _, service := range group.services {
			date := portalText(service["data_agendamento"])
			if date == "" {
				date = portalText(service["data_solicitacao"])
			}
			groupRows = append(groupRows, app.Div().Class("team-finance-completed-row").Body(
				app.Span().Body(app.Text(portalText(service["descricao"])+" • "+portalDate(date))),
				app.Strong().Class("team-finance-completed-row__value").Body(app.Text("R$ "+formatPortalMoney(agendaNumber(service["valor"])))),
				app.Button().Class("auth-link").Type("button").OnClick(p.openTeamServiceDetail(service)).Body(app.Text("Ver comprovante, garantia e ações da OS")),
			))
		}
		rows = append(rows, app.Article().Class("portal-service").Body(groupRows...))
	}
	if len(rows) == 0 {
		rows = append(rows, app.P().Class("portal-section__empty").Body(app.Text("Nenhum serviço executado neste período ainda.")))
	}
	sort.SliceStable(receivable, func(i, j int) bool {
		return portalText(receivable[i]["data_agendamento"]) < portalText(receivable[j]["data_agendamento"])
	})
	receivableServiceRows := []app.UI{}
	for _, service := range receivable {
		client := clientNames[portalText(service["cliente_id"])]
		if client == "" {
			client = "Cliente"
		}
		date := portalText(service["data_agendamento"])
		late := strings.EqualFold(portalText(service["status"]), "AGENDADO") && len(date) >= 10 && date[:10] < today
		tag := ""
		if late {
			tag = " • atrasado"
		} else if strings.EqualFold(portalText(service["status"]), "EM_ANDAMENTO") {
			tag = " • em andamento"
		}
		description := firstNonEmptyBudget(portalText(service["descricao"]), portalText(service["tipo"]))
		description = strings.ReplaceAll(description, "_", " ")
		receivableServiceRows = append(receivableServiceRows, app.Article().Class("portal-service").Body(
			app.Div().Class("portal-service__heading").Body(app.Strong().Body(app.Text(client)), app.Span().Body(app.Text("R$ "+formatPortalMoney(agendaNumber(service["valor"]))+tag))),
			app.P().Class("portal-service__date").Body(app.Text(description+" • "+portalDate(date))),
			func() app.UI {
				if portalText(service["status"]) == "AGENDADO" {
					return app.Button().Class("auth-submit").Type("button").OnClick(p.startTeamService(portalText(service["id"]))).Body(app.Text("Iniciar"))
				}
				return app.Span()
			}(),
			app.Button().Class("auth-submit").Type("button").OnClick(p.openTeamCompletion(service)).Body(app.Text("Finalizar")),
			app.Button().Class("auth-link").Type("button").OnClick(p.requestTeamCancellation(portalText(service["id"]))).Body(app.Text("Cancelar OS")),
		))
	}
	receivableBudgetRows := []app.UI{}
	for _, budget := range budgets {
		receivableBudgetRows = append(receivableBudgetRows, app.Article().Class("portal-service").Body(
			app.Div().Class("portal-service__heading").Body(app.Strong().Body(app.Text(budget.ClientName)), app.Span().Body(app.Text("R$ "+formatPortalMoney(budget.FinalValue)))),
			app.P().Class("portal-service__date").Body(app.Text(firstNonEmptyBudget(budget.ApplianceDescription, "Serviço")+" • criado em "+portalDate(budget.Date))),
			app.Button().Class("auth-link").Type("button").OnClick(func(ctx app.Context, event app.Event) {
				event.PreventDefault()
				p.teamBudgetOpenID = budget.ID
				ctx.Update()
			}).Body(app.Text("Abrir orçamento")),
			app.Button().Class("auth-submit").Type("button").Disabled(p.teamBudgetBusy).OnClick(p.confirmTeamBudget(budget, "RECEBIDO")).Body(app.Text("Marcar recebido")),
		))
	}
	if len(receivableServiceRows) == 0 {
		receivableServiceRows = append(receivableServiceRows, app.P().Class("portal-section__empty").Body(app.Text("Nenhum serviço agendado pendente de recebimento.")))
	}
	if len(receivableBudgetRows) == 0 {
		receivableBudgetRows = append(receivableBudgetRows, app.P().Class("portal-section__empty").Body(app.Text("Nenhum orçamento aprovado aguardando recebimento.")))
	}
	kpi := func(label string, value float64) app.UI {
		return app.Article().Class("portal-service").Body(app.P().Class("catalog__eyebrow").Body(app.Text(label)), app.Strong().Class("catalog__title").Body(app.Text("R$ "+formatPortalMoney(value))))
	}
	receivedLabel := "Recebido em " + monthLabel
	completedListLabel := "Serviços executados em " + monthLabel
	if p.teamFinanceAll {
		receivedLabel = "Recebido (histórico)"
		completedListLabel = "Serviços executados (histórico completo)"
	}
	pixKey := p.teamProfile.PIXKey
	pixLabel := "Copiar chave PIX"
	if p.teamFinanceCopied {
		pixLabel = "Chave copiada!"
	}
	pixButton := app.Button().Class("auth-submit").Type("button").Disabled(pixKey == "").OnClick(copyTextToClipboardNotice(pixKey, func(ctx app.Context, copied bool) {
		p.teamFinanceCopied = copied
		if copied {
			p.teamFinanceMessage = ""
			ctx.After(2*time.Second, func(next app.Context) {
				p.teamFinanceCopied = false
				next.Update()
			})
		} else {
			p.teamFinanceMessage = ""
		}
	})).Body(app.Text(pixLabel))
	pixActions := []app.UI{pixButton}
	content := []app.UI{
		app.Div().Class("catalog__heading team-finance__page-heading").Body(app.Div().Body(app.P().Class("catalog__eyebrow").Body(app.Text("FINANCEIRO")), app.H2().Class("catalog__title").Body(app.Text("Financeiro")), app.P().Class("portal-section__intro").Body(app.Text("Tudo que entrou, o que falta receber e o que atrasou"))), app.Div().Class("team-finance__heading-controls").Body(app.Label().Class("auth-field").Body(app.Text("Período"), app.Input().Type("month").Value(month).OnChange(p.ValueTo(&p.teamFinanceMonth))), app.Button().Class("auth-link team-finance__history-toggle").Type("button").OnClick(func(ctx app.Context, event app.Event) {
			event.PreventDefault()
			p.teamFinanceAll = !p.teamFinanceAll
			ctx.Update()
		}).Body(app.Text(func() string {
			if p.teamFinanceAll {
				return "Histórico completo ✓"
			}
			return "Ver tudo"
		}())))),
		app.Div().Class("service-grid team-finance__metrics").Body(kpi(receivedLabel, receivedMonth), kpi("A receber (total)", receivableServices+receivableBudgets), kpi("Atrasado (agendados vencidos)", dueServices), kpi("Orçamentos aprovados a faturar", receivableBudgets)),
		app.Article().Class("portal-service team-finance-pix").Body(app.Div().Body(app.P().Class("catalog__eyebrow").Body(app.Text("RECEBER VIA PIX • "+firstNonEmptyBudget(p.teamProfile.BusinessName, "Inovar Refrigeração"))), app.Strong().Body(app.Text(firstNonEmptyBudget(pixKey, "Configure sua chave em Configurações")))), app.Div().Class("portal-budget__actions").Body(pixActions...)),
		app.Section().Class("team-finance-section team-finance-section--completed").Body(
			app.Div().Class("team-finance-section__heading").Body(app.H3().Class("team-agenda__heading").Body(app.Text(completedListLabel)), app.Span().Class("team-finance-section__total").Body(app.Text("R$ "+formatPortalMoney(receivedMonth)))),
			app.Div().Class("service-grid team-finance-section__grid").Body(rows...),
		),
		app.Section().Class("team-finance-section").Body(app.Div().Class("team-finance-section__heading").Body(app.H3().Class("team-agenda__heading").Body(app.Text("A receber — serviços agendados")), app.Span().Class("team-finance-section__total").Body(app.Text("R$ "+formatPortalMoney(receivableServices)))), app.Div().Class("service-grid team-finance-section__grid").Body(receivableServiceRows...)),
		app.Section().Class("team-finance-section").Body(app.Div().Class("team-finance-section__heading").Body(app.H3().Class("team-agenda__heading").Body(app.Text("Orçamentos aprovados (a receber)")), app.Span().Class("team-finance-section__total").Body(app.Text("R$ "+formatPortalMoney(receivableBudgets)))), app.Div().Class("service-grid team-finance-section__grid").Body(receivableBudgetRows...)),
	}
	if len(paidBudgets) > 0 {
		content = append(content, app.H3().Class("team-agenda__heading").Body(app.Text("Recebimentos registrados (orçamentos) • R$ "+formatPortalMoney(paidBudgetTotal))))
		for _, budget := range paidBudgets {
			amount := budget.FinalValue
			if budget.AmountReceived != nil {
				amount = *budget.AmountReceived
			}
			date := "—"
			if budget.PaidAt != nil && strings.TrimSpace(*budget.PaidAt) != "" {
				date = portalDate(*budget.PaidAt)
			}
			content = append(content, app.Article().Class("portal-service").Body(
				app.Div().Class("portal-service__heading").Body(app.Strong().Body(app.Text(firstNonEmptyBudget(budget.ClientName, "Cliente"))), app.Span().Class("portal-service__value").Body(app.Text("R$ "+formatPortalMoney(amount)))),
				app.P().Class("portal-service__date").Body(app.Text(date)),
			))
		}
	}
	if p.teamFinanceMessage != "" {
		content = append(content, app.P().Class("portal-section__intro").Body(app.Text(p.teamFinanceMessage)))
	}
	return app.Section().ID("team-finance").Class("portal-section").Body(content...)
}

func (p *serviceCatalogPage) teamAgendaGroup(title, headingClass string, items []teamAgendaItem) []app.UI {
	if len(items) == 0 {
		return nil
	}
	group := []app.UI{app.H3().Class(headingClass).Body(app.Text(title))}
	for _, item := range items {
		group = append(group, p.teamAgendaCard(item))
	}
	return group
}

func (p *serviceCatalogPage) teamAgendaSections(agenda teamAgendaGroups) []app.UI {
	sections := make([]app.UI, 0, 3)
	if len(agenda.Overdue) > 0 {
		sections = append(sections, app.Div().ID("team-agenda-overdue").Class("team-agenda").Body(
			p.teamAgendaGroup("⚠ Atrasados (pendente de execução)", "team-agenda__heading team-agenda__heading--overdue", agenda.Overdue)...,
		))
	}
	upcoming := p.teamAgendaGroup("Próximos agendamentos", "team-agenda__heading", agenda.Upcoming)
	if len(agenda.Upcoming) == 0 {
		upcoming = append(upcoming, app.H3().Class("team-agenda__heading").Body(app.Text("Próximos agendamentos")))
		upcoming = append(upcoming, app.Div().Class("portal-section__empty").Body(app.Text("Nenhum agendamento futuro. Use \"Agendar Retorno\" ou aprove um orçamento para criar um.")))
	}
	sections = append(sections, app.Div().Class("team-agenda").Body(upcoming...))
	if len(agenda.RecentlyCompleted) > 0 {
		sections = append(sections, app.Div().Class("team-agenda").Body(
			p.teamAgendaGroup("Concluídos recentemente", "team-agenda__heading team-agenda__heading--complete", agenda.RecentlyCompleted)...,
		))
	}
	return sections
}

func (p *serviceCatalogPage) teamAgendaCard(item teamAgendaItem) app.UI {
	statusLabel, tone := portalServiceStatus(item.Status)
	if item.Service != "" && item.Service != "OUTRO" {
		item.Service = string(supabase.MapSupabaseServiceTypeToLocal(item.Service))
	}
	if item.Service == "" {
		item.Service = "Atendimento"
	}
	title := item.Service
	if item.Customer != "" {
		title += " • " + item.Customer
	}
	lines := []app.UI{
		app.Div().Class("portal-service__heading").Body(
			app.Strong().Body(app.Text(title)),
			app.Span().Class("portal-service__status portal-service__status--"+tone).Body(app.Text(statusLabel)),
		),
		app.P().Class("portal-service__date").Body(app.Text("Agendamento: " + portalDate(item.Date) + agendaTimeLabel(item.Time))),
	}
	if item.Appliance != "" {
		lines = append(lines, app.P().Class("portal-service__date").Body(app.Text(item.Appliance)))
	}
	if item.Phone != "" || item.Address != "" {
		contact := item.Phone
		if item.Address != "" {
			if contact != "" {
				contact += " • "
			}
			contact += item.Address
		}
		lines = append(lines, app.P().Class("portal-service__date").Body(app.Text(contact)))
	}
	cycle := []string{}
	if item.Started != "" {
		cycle = append(cycle, "Início: "+portalDate(item.Started))
	}
	if item.Completed != "" {
		cycle = append(cycle, "Conclusão: "+portalDate(item.Completed))
	}
	if item.Cancelled != "" {
		cycle = append(cycle, "Cancelamento: "+portalDate(item.Cancelled))
	}
	if len(cycle) > 0 {
		lines = append(lines, app.P().Class("portal-service__date").Body(app.Text(strings.Join(cycle, " • "))))
	}
	if item.Value > 0 {
		lines = append(lines, app.P().Class("portal-service__value").Body(app.Text(formatPortalMoney(item.Value))))
	} else {
		lines = append(lines, app.P().Class("portal-service__value").Body(app.Text("—")))
	}
	parts := []app.UI{app.Div().Class("portal-service__main").Body(lines...)}
	parts = append(parts,
		app.Button().Class("auth-link").Type("button").OnClick(p.openTeamServiceEdit(item.Source)).Body(app.Text("Editar serviço")),
		app.Button().Class("auth-link").Type("button").OnClick(p.askTeamCustomerAction(teamDeleteServiceAction+item.ID)).Body(app.Text("Excluir serviço")),
	)
	if item.Status == "AGENDADO" {
		parts = append(parts, app.Button().Class("auth-submit").Type("button").OnClick(p.startTeamService(item.ID)).Body(app.Text("Iniciar serviço")))
	}
	if canFinishTeamAgenda(item.Status) {
		parts = append(parts, app.Button().Class("auth-submit").Type("button").OnClick(p.openTeamCompletion(item.Source)).Body(app.Text("Finalizar")))
	}
	if item.Status == "AGENDADO" || item.Status == "EM_ANDAMENTO" {
		parts = append(parts, app.Button().Class("auth-link").Type("button").OnClick(p.requestTeamCancellation(item.ID)).Body(app.Text("Cancelar OS")))
	}
	if item.Status == "AGENDADO" || item.Status == "EM_ANDAMENTO" || item.Status == "CONCLUIDO" {
		label := "Reagendar"
		if item.Status == "CONCLUIDO" {
			label = "Reabrir e reagendar"
		}
		parts = append(parts, app.Button().Class("auth-link").Type("button").OnClick(p.openTeamSchedule(item.Source)).Body(app.Text(label)))
	}
	return app.Article().Class("portal-service team-agenda__card team-finance-completed-card").Body(parts...)
}

func agendaTimeLabel(value string) string {
	if strings.TrimSpace(value) == "" {
		return " • Horário a combinar"
	}
	return " • " + strings.TrimSpace(value)
}

func (p *serviceCatalogPage) teamDashboardCards() []app.UI {
	s := p.teamDashboard
	metric := func(label string, value any) app.UI {
		tone := dashboardMetricTone(label, s)
		return app.Article().Class("portal-service team-dashboard-kpi team-dashboard-kpi--"+tone).Body(
			app.P().Class("catalog__eyebrow").Body(app.Text(label)),
			app.Strong().Class("catalog__title").Body(app.Text(value)),
		)
	}
	items := []struct {
		label string
		value any
	}{
		{"Clientes", s.Customers},
		{"Retornos atrasados", s.ReturnsOverdue},
		{"Vencendo esta semana", s.ReturnsThisWeek},
		{"Sem histórico", s.NoHistory},
		{"Chamados abertos", s.PendingRequests},
		{"Orçamentos", s.TotalBudgets},
		{"Ordens de serviço", s.TotalServices},
		{"Faturamento", dashboardRevenueLabel(s.Revenue)},
	}
	cards := make([]app.UI, 0, len(items))
	for _, item := range items {
		target := dashboardNavigationFor(item.label)
		card := app.A().Class("team-dashboard-link").Href("#"+target.Anchor).Attr("title", "Abrir "+item.label).OnClick(func(ctx app.Context, event app.Event) {
			p.teamActiveSection = teamNavigationKey(target.Anchor)
			switch target.Section {
			case "returns":
				p.teamQueueSection, p.teamQueueFilter, p.teamQueueSearch = teamQueueReturns, target.Filter, ""
			case teamQueueRequests, teamQueueHistory:
				p.teamQueueSection, p.teamQueueSearch = target.Section, ""
			case "calendar":
				p.teamAgendaStatus = target.Filter
			}
			ctx.Update()
		}).Body(metric(item.label, item.value))
		cards = append(cards, card)
	}
	return []app.UI{
		app.Div().ID("team-dashboard-metrics").Class("service-grid team-dashboard-metrics").Body(
			cards...,
		),
		p.teamContactCenter(),
		app.P().Class("portal-section__intro").Body(app.Text("Faturamento: serviços concluídos e orçamentos aprovados ainda sem OS concluída vinculada.")),
	}
}

func dashboardMetricTone(label string, summary teamDashboardSummary) string {
	switch label {
	case "Retornos atrasados", "OS atrasadas":
		if summary.ReturnsOverdue+summary.Overdue > 0 {
			return "danger"
		}
		return "neutral"
	case "Vencendo esta semana", "Agendamentos":
		if summary.ReturnsThisWeek+summary.Scheduled > 0 {
			return "warning"
		}
		return "neutral"
	case "Sem histórico":
		if summary.NoHistory > 0 {
			return "purple"
		}
		return "neutral"
	case "Chamados abertos":
		if summary.PendingRequests > 0 {
			return "purple"
		}
		return "neutral"
	case "Orçamentos":
		return "warning"
	case "Ordens de serviço", "Faturamento", "Concluídos":
		return "success"
	default:
		return "info"
	}
}

type dashboardNavigation struct {
	Anchor  string
	Section string
	Filter  string
}

func dashboardNavigationFor(label string) dashboardNavigation {
	switch label {
	case "Clientes", "Aparelhos":
		return dashboardNavigation{Anchor: "team-customers"}
	case "Chamados abertos":
		return dashboardNavigation{Anchor: "team-contact-center", Section: teamQueueRequests}
	case "Ordens de serviço", "Concluídos", "Cancelados":
		return dashboardNavigation{Anchor: "team-contact-center", Section: teamQueueHistory}
	case "Agendamentos":
		return dashboardNavigation{Anchor: "team-calendar", Section: "calendar", Filter: "AGENDADO"}
	case "Em andamento":
		return dashboardNavigation{Anchor: "team-calendar", Section: "calendar", Filter: "EM_ANDAMENTO"}
	case "Retornos atrasados":
		return dashboardNavigation{Anchor: "team-contact-center", Section: "returns", Filter: string(domain.ReturnOverdue)}
	case "Vencendo esta semana":
		return dashboardNavigation{Anchor: "team-contact-center", Section: "returns", Filter: string(domain.ReturnThisWeek)}
	case "Sem histórico":
		return dashboardNavigation{Anchor: "team-contact-center", Section: "returns", Filter: string(domain.ReturnNoHistory)}
	case "OS atrasadas":
		return dashboardNavigation{Anchor: "team-agenda-overdue"}
	case "Orçamentos":
		return dashboardNavigation{Anchor: "team-budgets"}
	case "Faturamento":
		return dashboardNavigation{Anchor: "team-finance"}
	default:
		return dashboardNavigation{Anchor: "team-operations"}
	}
}

func teamReturnStatusLabel(status string) string {
	return domain.ReturnStatusLabel(domain.ReturnStatus(status))
}

func customerInitials(name string) string {
	runes := []rune(strings.TrimSpace(name))
	if len(runes) > 2 {
		runes = runes[:2]
	}
	return strings.ToUpper(string(runes))
}

func (p *serviceCatalogPage) teamCustomerRegistry() app.UI {
	filtered := make([]map[string]any, 0, len(p.teamCustomers))
	needle := strings.TrimSpace(p.teamCustomerSearch)
	for _, customer := range p.teamCustomers {
		name, phone := portalText(customer["nome"]), portalText(customer["whatsapp"])
		customerSearchFields := []string{name, phone, portalText(customer["bairro"]), portalText(customer["endereco"]), portalText(customer["cidade"]), portalText(customer["email"]), portalText(customer["observacoes"]), portalText(customer["cep"])}
		appliances, _ := customer["appliances"].([]any)
		for _, raw := range appliances {
			appliance, _ := raw.(map[string]any)
			customerSearchFields = append(customerSearchFields, portalText(appliance["marca"]), portalText(appliance["modelo"]), portalText(appliance["ambiente"]), portalText(appliance["btus"]), portalText(appliance["numero_serie"]), portalText(appliance["local_instalacao"]))
		}
		if searchMatches(needle, customerSearchFields...) {
			filtered = append(filtered, customer)
		}
	}
	items := make([]app.UI, 0, len(filtered))
	for _, customer := range filtered {
		name := portalText(customer["nome"])
		if name == "" {
			name = "Cliente sem nome"
		}
		details := []string{}
		if phone := portalText(customer["whatsapp"]); phone != "" {
			details = append(details, phone)
		}
		address := strings.Join(strings.Fields(strings.Join([]string{portalText(customer["endereco"]), portalText(customer["bairro"]), portalText(customer["cidade"])}, " ")), " ")
		if address != "" {
			details = append(details, address)
		}
		appliances, _ := customer["appliances"].([]any)
		applianceCount := len(appliances)
		applianceCards := make([]app.UI, 0, len(appliances))
		for _, raw := range appliances {
			appliance, _ := raw.(map[string]any)
			if appliance == nil {
				continue
			}
			label := strings.TrimSpace(portalText(appliance["marca"]) + " " + portalText(appliance["modelo"]))
			if btu := portalBTUs(appliance["btus"]); btu != "Capacidade não informada" {
				label += " • " + btu
			}
			if label != "" {
				applianceID := portalText(appliance["id"])
				applianceCards = append(applianceCards, app.Li().Class("service-card__item").Body(
					app.Div().Class("portal-service__heading").Body(
						app.Strong().Body(app.Text(label)),
						app.Span().Class("catalog__eyebrow").Body(app.Text(portalText(appliance["ambiente"]))),
					),
					app.P().Class("portal-section__intro").Body(app.Text(teamCustomerApplianceTechnicalSummary(appliance))),
					app.Div().Class("team-appliance-actions").Body(
						app.Button().Class("auth-submit team-appliance-action team-appliance-action--primary").Type("button").OnClick(p.openTeamStartServiceForAppliance(portalText(customer["id"]), applianceID)).Body(app.Text("Ordem de Serviço")),
						app.Button().Class("auth-link team-appliance-action").Type("button").OnClick(p.openTeamApplianceForm(customer, appliance)).Body(app.Text("Editar aparelho")),
						app.Button().Class("auth-link team-appliance-action").Type("button").OnClick(p.toggleAppliancePhotos(applianceID)).Body(app.Text("Fotos")),
						app.Button().Class("auth-link team-appliance-action").Type("button").OnClick(p.toggleApplianceHistory(applianceID)).Body(app.Text("Histórico completo")),
						func() app.UI {
							if p.caller != nil && p.caller.Role == domain.RoleAdmin {
								return app.Button().Class("auth-link team-appliance-action team-appliance-action--danger").Type("button").OnClick(p.askTeamCustomerAction("delete-appliance:" + applianceID)).Body(app.Text("Excluir aparelho"))
							}
							return app.Span()
						}(),
					),
				))
			}
		}
		if len(applianceCards) == 0 {
			applianceCards = append(applianceCards, app.Li().Class("service-card__item").Body(app.Text("Nenhum aparelho cadastrado")))
		}
		customerID := portalText(customer["id"])
		customerActions := []app.UI{
			app.Button().Class("auth-link team-customer-action").Type("button").OnClick(p.openTeamCustomerForm(customer)).Body(app.Text("Editar cadastro")),
			app.Button().Class("auth-link team-customer-action").Type("button").OnClick(p.openTeamApplianceForm(customer, nil)).Body(app.Text("Adicionar aparelho")),
			app.Button().Class("auth-link team-customer-action").Type("button").OnClick(p.askTeamCustomerAction("reset-password:" + customerID)).Body(app.Text("Redefinir senha")),
		}
		if p.caller != nil && p.caller.Role == domain.RoleAdmin {
			customerActions = append(customerActions, app.Button().Class("auth-link team-customer-action team-customer-action--danger").Type("button").OnClick(p.askTeamCustomerAction("delete-customer:"+customerID)).Body(app.Text("Excluir cliente")))
		}
		initials := customerInitials(name)
		applianceCountLabel := "aparelhos"
		if applianceCount == 1 {
			applianceCountLabel = "aparelho"
		}
		heading := app.Button().Class("team-customer-card__heading").Type("button").Attr("aria-expanded", ariaBoolean(p.teamCustomerExpandedID == customerID)).OnClick(func(ctx app.Context, event app.Event) {
			event.PreventDefault()
			if p.teamCustomerExpandedID == customerID {
				p.teamCustomerExpandedID = ""
			} else {
				p.teamCustomerExpandedID = customerID
			}
			ctx.Update()
		}).Body(
			app.Span().Class("team-customer-card__initials").Body(app.Text(initials)),
			app.Span().Class("team-customer-card__identity").Body(app.Strong().Body(app.Text(name)), app.Span().Body(app.Text(strings.Join(details, " • ")))),
			app.Span().Class("team-customer-card__count").Body(app.Text(applianceCount), app.Text(" "+applianceCountLabel)),
			app.Span().Class("team-customer-card__chevron").Body(app.Text(map[bool]string{true: "⌃", false: "⌄"}[p.teamCustomerExpandedID == customerID])),
		)
		cardContent := []app.UI{heading}
		if p.teamCustomerExpandedID == customerID {
			expanded := []app.UI{}
			if notes := strings.TrimSpace(portalText(customer["observacoes"])); notes != "" {
				expanded = append(expanded, app.P().Class("team-customer-card__notes").Body(app.Text(notes)))
			}
			expanded = append(expanded,
				app.Div().Class("portal-budget__actions team-customer-actions").Body(customerActions...),
				teamCustomerQuickContactLinks(portalText(customer["whatsapp"])),
				app.H4().Class("team-customer-card__section-title").Body(app.Text("Aparelhos Cadastrados")),
				app.Ul().Class("service-card__items").Body(applianceCards...),
				teamCustomerBudgetPanel(p.teamBudgets, customerID),
				p.appliancePhotosPanel(),
				p.applianceHistoryPanel(),
			)
			cardContent = append(cardContent, app.Div().Class("team-customer-card__details").Body(expanded...))
		}
		items = append(items, app.Article().Class("portal-service team-customer-card").Body(cardContent...))
	}
	search := app.Input().Type("search").Class("team-search-input").Placeholder("Buscar por cliente, telefone, aparelho...").Value(p.teamCustomerSearch).OnInput(p.ValueTo(&p.teamCustomerSearch))
	if len(items) == 0 {
		items = append(items, app.P().Class("portal-section__empty").Body(app.Text("Nenhum cliente encontrado para \""+p.teamCustomerSearch+"\".")))
	}
	registry := app.Div().ID("team-customers").Class("team-customer-registry").Body(
		app.Div().Class("catalog__heading").Body(app.Div().Body(app.P().Class("catalog__eyebrow").Body(app.Text("CADASTRO")), app.H2().Class("catalog__title").Body(app.Text("Clientes e aparelhos"))), app.Span().Class("catalog__count").Body(app.Text(len(filtered)), app.Text(" clientes"))),
		app.Div().Class("team-customer-toolbar").Body(
			app.Label().Class("auth-field team-search-field").Body(app.Text("Buscar cliente ou aparelho"), app.Span().Class("team-search-control").Body(app.Span().Class("team-search-icon").Body(app.Text("⌕")), search)),
			app.Button().Class("auth-submit").Type("button").OnClick(p.openTeamCustomerForm(nil)).Body(app.Text("＋ Novo Cliente")),
		),
		app.P().Class("portal-section__intro").Body(app.Text(p.teamCustomerNotice)),
		app.Div().Class("team-customer-grid").Body(items...),
	)
	return registry
}

func teamCustomerActionDialog(p *serviceCatalogPage, action string) app.UI {
	if action == "" {
		return app.Div()
	}
	message := teamCustomerActionMessage(p.teamServiceDetail, action)
	deleteAction := strings.HasPrefix(action, "delete-") || strings.HasPrefix(action, teamDeleteServiceAction) || strings.HasPrefix(action, teamPhotoDeleteAction) || strings.HasPrefix(action, teamServicePhotoDeleteAction)
	title, noLabel, yesLabel := "Confirmar ação", "Cancelar", "Confirmar"
	if deleteAction {
		title, noLabel, yesLabel = "Confirmar exclusão", "Não, manter", "Sim, excluir"
	}
	return app.Div().Class("auth-backdrop").Style("z-index", "80").Body(app.Div().Class("auth-dialog team-confirm-dialog").Role("dialog").Attr("aria-label", title).Body(
		app.H2().Class("auth-dialog__title").Body(app.Text(title)),
		app.P().Class("auth-dialog__intro team-confirm-dialog__message").Body(app.Text(message)),
		app.Div().Class("portal-budget__actions team-confirm-dialog__actions").Body(
			app.Button().Class("auth-link").Type("button").Disabled(p.teamCustomerActionSaving).OnClick(p.cancelTeamCustomerAction).Body(app.Text(noLabel)),
			app.Button().Class("auth-submit").Type("button").Disabled(p.teamCustomerActionSaving).OnClick(p.runTeamCustomerAction).Body(app.Text(yesLabel)),
		),
	))
}

func teamCustomerActionMessage(serviceDetail map[string]any, action string) string {
	switch {
	case strings.HasPrefix(action, teamDeleteLegacyHistoryAction):
		return "Excluir este registro de histórico permanentemente? Esta ação não pode ser desfeita."
	case strings.HasPrefix(action, teamDeleteHistoryServiceAction):
		return "Excluir esta ordem de serviço permanentemente? Esta ação não pode ser desfeita."
	case strings.HasPrefix(action, "delete-customer:"):
		return "Excluir o cliente e seus aparelhos? Esta ação não pode ser desfeita."
	case strings.HasPrefix(action, "delete-appliance:"):
		return "Excluir este aparelho? Esta ação não pode ser desfeita."
	case strings.HasPrefix(action, "reset-password:"):
		return "Gerar uma nova senha temporária? A senha atual deixará de funcionar."
	case strings.HasPrefix(action, teamPhotoDeleteAction):
		return "Excluir esta foto do aparelho? Esta ação não pode ser desfeita."
	case strings.HasPrefix(action, teamServicePhotoDeleteAction):
		return "Excluir esta foto da ordem de serviço? Esta ação não pode ser desfeita."
	case strings.HasPrefix(action, teamDeleteServiceAction):
		serviceID := strings.TrimPrefix(action, teamDeleteServiceAction)
		if serviceDetail != nil && portalText(serviceDetail["id"]) == serviceID {
			return "Deseja realmente excluir esta Ordem de Serviço do histórico?"
		}
		return "Excluir este serviço permanentemente do banco? Esta ação não pode ser desfeita."
	default:
		return "Confirma esta ação?"
	}
}

