package webapp

import (
	"math"
	"sort"
	"strings"
	"time"

	"inovarapp/core/domain"
)

func dashboardRevenueLabel(value float64) string {
	formatted := formatPortalMoney(math.Round(value))
	return "R$ " + strings.TrimSuffix(formatted, ",00")
}

// teamDashboardSummary contains counts derived from the authenticated Go
// operations payloads loaded from authenticated Go endpoints.
type teamDashboardSummary struct {
	Customers       int
	Appliances      int
	TotalServices   int
	TotalBudgets    int
	PendingRequests int
	Scheduled       int
	InProgress      int
	Completed       int
	Cancelled       int
	Overdue         int
	ReturnsOverdue  int
	ReturnsThisWeek int
	ReturnsSoon     int
	NoHistory       int
	ReturnEvents    []teamReturnItem
	Upcoming        int
	Revenue         float64
	RecentServices  []teamAgendaItem
}

type teamReturnItem struct {
	CustomerID, ApplianceID, Phone                         string
	Customer, Appliance, Date, Status, LastMaintenanceDate string
	LastMaintenanceService                                 string
	Brand, Model, Room, Type, Capacity                     string
	BudgetID                                               string
	ScheduledServiceID, ScheduledTime                      string
	PendingServiceIDs, InProgressServiceIDs                []string
}

func buildTeamDashboardSummary(customers, services, appointments []map[string]any, budgets []domain.BudgetEstimate, history []map[string]any, defaultMonths int, today time.Time) teamDashboardSummary {
	summary := teamDashboardSummary{Customers: len(customers)}
	for _, customer := range customers {
		if appliances, ok := customer["appliances"].([]any); ok {
			summary.Appliances += len(appliances)
		} else if appliances, ok := customer["appliances"].([]map[string]any); ok {
			summary.Appliances += len(appliances)
		}
	}
	todayISO := today.Format("2006-01-02")
	agenda := buildTeamAgenda(services, todayISO)
	summary.TotalServices = len(services)
	summary.TotalBudgets = len(budgets)
	summary.ReturnEvents, summary.ReturnsOverdue, summary.ReturnsThisWeek, summary.ReturnsSoon, summary.NoHistory = buildReturnQueue(customers, services, appointments, budgets, history, defaultMonths, today)
	for _, service := range services {
		switch strings.ToUpper(agendaText(service["status"])) {
		case "AGENDADO":
			summary.Scheduled++
		case "PENDENTE":
			summary.PendingRequests++
		case "EM_ANDAMENTO":
			summary.InProgress++
		case "CONCLUIDO":
			summary.Completed++
			summary.Revenue += agendaNumber(service["valor"])
		case "CANCELADO":
			summary.Cancelled++
		}
	}
	completedServices := make(map[string]bool, summary.Completed)
	for _, service := range services {
		if strings.ToUpper(agendaText(service["status"])) == "CONCLUIDO" {
			completedServices[agendaText(service["id"])] = true
		}
	}
	for _, budget := range budgets {
		if budget.Status != domain.BudgetApproved {
			continue
		}
		if budget.ServiceID != nil && completedServices[*budget.ServiceID] {
			continue
		}
		summary.Revenue += budget.FinalValue
	}
	// The legacy dashboard adds approved budgets unless their linked service is
	// already completed; retain that de-duplication rule here.
	summary.RecentServices = append(summary.RecentServices, agenda.RecentlyCompleted...)
	sort.SliceStable(summary.RecentServices, func(i, j int) bool { return summary.RecentServices[i].Completed > summary.RecentServices[j].Completed })
	return summary
}

func buildReturnQueue(customers, services, appointments []map[string]any, budgets []domain.BudgetEstimate, history []map[string]any, defaultMonths int, today time.Time) ([]teamReturnItem, int, int, int, int) {
	if defaultMonths <= 0 {
		defaultMonths = 6
	}
	servicesByAppliance := make(map[string][]map[string]any)
	servicesWithoutApplianceByCustomer := make(map[string][]map[string]any)
	executingWithoutAppliance := make(map[string]bool)
	serviceIDs, completedByApplianceDate := indexExistingServices(services)
	for _, service := range services {
		applianceID := agendaText(service["aparelho_id"])
		if applianceID != "" {
			servicesByAppliance[applianceID] = append(servicesByAppliance[applianceID], service)
		} else if clientID := agendaText(service["cliente_id"]); clientID != "" {
			servicesWithoutApplianceByCustomer[clientID] = append(servicesWithoutApplianceByCustomer[clientID], service)
			if strings.ToUpper(agendaText(service["status"])) == "EM_ANDAMENTO" {
				executingWithoutAppliance[clientID] = true
			}
		}
	}
	for _, row := range history {
		applianceID := agendaText(row["aparelho_id"])
		if applianceID == "" || historyAlreadyRepresented(row, serviceIDs, completedByApplianceDate) {
			continue
		}
		row = serviceHistoryAsCompletedService(row)
		servicesByAppliance[applianceID] = append(servicesByAppliance[applianceID], row)
	}
	latestStandaloneHistory := make(map[string]map[string]any)
	for _, row := range history {
		applianceID := agendaText(row["aparelho_id"])
		if applianceID == "" || historyAlreadyRepresented(row, serviceIDs, completedByApplianceDate) {
			continue
		}
		current := latestStandaloneHistory[applianceID]
		if current == nil || firstDashboardDate(agendaText(row["data"]), agendaText(row["data_conclusao"])) > firstDashboardDate(agendaText(current["data"]), agendaText(current["data_conclusao"])) {
			latestStandaloneHistory[applianceID] = row
		}
	}
	appointmentsByService := make(map[string]map[string]any)
	for _, appointment := range appointments {
		appointmentsByService[agendaText(appointment["service_id"])] = appointment
	}
	budgetsByCustomer := make(map[string][]domain.BudgetEstimate)
	for _, budget := range budgets {
		if budget.Status == domain.BudgetPending || budget.Status == domain.BudgetApproved {
			budgetsByCustomer[budget.ClientID] = append(budgetsByCustomer[budget.ClientID], budget)
		}
	}
	var items []teamReturnItem
	var overdue, week, soon, noHistory int
	var budgetOnlyItems []teamReturnItem
	seenBudgetOnly := make(map[string]bool)
	for _, customer := range customers {
		customerID, customerName := agendaText(customer["id"]), agendaText(customer["nome"])
		appliances, _ := customer["appliances"].([]any)
		if len(appliances) != 0 {
			continue
		}
		for _, budget := range budgetsByCustomer[customerID] {
			if budget.ApplianceID != nil && *budget.ApplianceID != "" {
				continue
			}
			key := "budget:" + budget.ID
			if seenBudgetOnly[key] {
				continue
			}
			date := today.Format("2006-01-02")
			if budget.Status != domain.BudgetApproved {
				date = firstDashboardDate(budget.ValidUntil, date)
			}
			label := firstDashboardString(budget.ApplianceDescription, budget.EquipmentName, "Equipamento do orçamento")
			status := domain.ReturnStatusForDate(date, today)
			budgetOnlyItems = append(budgetOnlyItems, teamReturnItem{CustomerID: customerID, Phone: agendaText(customer["whatsapp"]), Customer: customerName, Appliance: label, Date: date, Status: string(status), BudgetID: budget.ID})
			seenBudgetOnly[key] = true
		}
		for _, service := range servicesWithoutApplianceByCustomer[customerID] {
			if strings.ToUpper(agendaText(service["status"])) != "PENDENTE" {
				continue
			}
			name := firstDashboardString(agendaText(service["descricao"]), agendaText(service["tipo"]), "Chamado do cliente")
			items = append(items, teamReturnItem{CustomerID: customerID, Phone: agendaText(customer["whatsapp"]), Customer: customerName, Appliance: name, Status: string(domain.ReturnNoHistory), PendingServiceIDs: []string{agendaText(service["id"])}})
		}
	}
	for _, customer := range customers {
		customerID, customerName := agendaText(customer["id"]), agendaText(customer["nome"])
		appliances, _ := customer["appliances"].([]any)
		if executingWithoutAppliance[customerID] {
			continue
		}
		unlinkedBudgetAssigned := false
		for _, raw := range appliances {
			appliance, _ := raw.(map[string]any)
			if appliance == nil {
				continue
			}
			applianceID := agendaText(appliance["id"])
			name := strings.TrimSpace(agendaText(appliance["marca"]) + " " + agendaText(appliance["modelo"]))
			if name == "" {
				name = "Aparelho"
			}
			lastMaintenanceDate := firstDashboardDate(agendaText(appliance["ultima_manutencao"]))
			lastMaintenanceService := ""
			var lastCompleted map[string]any
			var scheduled map[string]any
			pendingServiceIDs := []string{}
			inProgressServiceIDs := []string{}
			for _, service := range servicesByAppliance[applianceID] {
				switch strings.ToUpper(agendaText(service["status"])) {
				case "PENDENTE":
					pendingServiceIDs = append(pendingServiceIDs, agendaText(service["id"]))
				case "CONCLUIDO":
					if lastCompleted == nil || serviceEffectiveDate(service) > serviceEffectiveDate(lastCompleted) {
						lastCompleted = service
					}
				case "AGENDADO":
					appointment := appointmentsByService[agendaText(service["id"])]
					date := firstDashboardDate(agendaText(appointment["data"]), agendaText(service["data_agendamento"]))
					if date != "" && (scheduled == nil || date < serviceEffectiveDate(scheduled)) {
						scheduled = service
						if appointment != nil {
							copy := make(map[string]any, len(service)+1)
							for key, value := range service {
								copy[key] = value
							}
							copy["data_agendamento"] = date
							copy["hora_agendamento"] = appointment["hora"]
							scheduled = copy
						}
					}
				case "EM_ANDAMENTO":
					inProgressServiceIDs = append(inProgressServiceIDs, agendaText(service["id"]))
				}
			}
			if standalone := latestStandaloneHistory[applianceID]; standalone != nil && (lastCompleted == nil || firstDashboardDate(agendaText(standalone["data"]), agendaText(standalone["data_conclusao"])) > serviceEffectiveDate(lastCompleted)) {
				lastCompleted = serviceHistoryAsCompletedService(standalone)
			}
			if lastCompleted != nil {
				lastMaintenanceDate = serviceEffectiveDate(lastCompleted)
				lastMaintenanceService = domain.AlertServiceName(agendaText(lastCompleted["tipo"]), agendaText(lastCompleted["descricao"]))
			}
			if applianceDate := firstDashboardDate(agendaText(appliance["ultima_manutencao"])); applianceDate > lastMaintenanceDate {
				lastMaintenanceDate = applianceDate
				lastMaintenanceService = "Data informada no cadastro"
			}
			for _, service := range servicesWithoutApplianceByCustomer[customerID] {
				switch strings.ToUpper(agendaText(service["status"])) {
				case "PENDENTE":
					pendingServiceIDs = append(pendingServiceIDs, agendaText(service["id"]))
				case "EM_ANDAMENTO":
					inProgressServiceIDs = append(inProgressServiceIDs, agendaText(service["id"]))
				}
			}
			returnDate := ""
			status := domain.ReturnNoHistory
			budgetID := ""
			if scheduled != nil {
				returnDate = serviceEffectiveDate(scheduled)
			} else if lastCompleted != nil {
				returnDate = markerDate(agendaText(lastCompleted["observacoes"]), "PROXIMO_RETORNO")
				if returnDate == "" {
					base := firstDashboardDate(agendaText(lastCompleted["data_conclusao"]), agendaText(lastCompleted["data_agendamento"]), lastMaintenanceDate)
					if base != "" {
						if date, err := time.Parse("2006-01-02", base); err == nil {
							returnDate = firstDashboardDate(agendaText(lastCompleted["return_date"]), markerDate(agendaText(lastCompleted["observacoes"]), "PROXIMO_RETORNO"))
							if returnDate == "" {
								returnDate = domain.AddMonthsClamped(date, defaultMonths).Format("2006-01-02")
							}
						}
					}
				}
			} else if lastMaintenanceDate != "" {
				if date, err := time.Parse("2006-01-02", lastMaintenanceDate); err == nil {
					returnDate = domain.AddMonthsClamped(date, defaultMonths).Format("2006-01-02")
				}
			} else {
				for _, budget := range budgetsByCustomer[customerID] {
					linkedAppliance := ""
					if budget.ApplianceID != nil {
						linkedAppliance = *budget.ApplianceID
					}
					if linkedAppliance != "" && linkedAppliance != applianceID || linkedAppliance == "" && unlinkedBudgetAssigned {
						continue
					}
					if budget.Status == domain.BudgetApproved {
						returnDate = today.Format("2006-01-02")
					} else {
						returnDate = firstDashboardDate(budget.ValidUntil, today.Format("2006-01-02"))
					}
					budgetID = budget.ID
					if linkedAppliance == "" {
						unlinkedBudgetAssigned = true
					}
					break
				}
			}
			status = domain.ReturnStatusForDate(returnDate, today)
			switch status {
			case domain.ReturnOverdue:
				overdue++
			case domain.ReturnThisWeek:
				week++
			case domain.ReturnSoon:
				soon++
			case domain.ReturnNoHistory:
				noHistory++
			}
			item := teamReturnItem{
				CustomerID: customerID, ApplianceID: applianceID, Phone: agendaText(customer["whatsapp"]),
				Customer: customerName, Appliance: name, Date: returnDate, Status: string(status), LastMaintenanceDate: lastMaintenanceDate,
				LastMaintenanceService: lastMaintenanceService,
				Brand:                  agendaText(appliance["marca"]), Model: agendaText(appliance["modelo"]), Room: agendaText(appliance["ambiente"]), BudgetID: budgetID,
				Type: agendaText(appliance["tipo"]), Capacity: agendaText(appliance["btus"]),
				PendingServiceIDs: pendingServiceIDs, InProgressServiceIDs: inProgressServiceIDs,
			}
			if scheduled != nil {
				item.ScheduledServiceID = agendaText(scheduled["id"])
				item.ScheduledTime = agendaText(scheduled["hora_agendamento"])
			}
			items = append(items, item)
		}
	}
	for _, item := range budgetOnlyItems {
		items = append(items, item)
		switch item.Status {
		case string(domain.ReturnOverdue):
			overdue++
		case string(domain.ReturnThisWeek):
			week++
		case string(domain.ReturnSoon):
			soon++
		case string(domain.ReturnNoHistory):
			noHistory++
		}
	}
	return items, overdue, week, soon, noHistory
}

func indexExistingServices(services []map[string]any) (map[string]bool, map[string]bool) {
	ids, applianceDates := make(map[string]bool), make(map[string]bool)
	for _, service := range services {
		if id := agendaText(service["id"]); id != "" {
			ids[id] = true
		}
		if strings.ToUpper(agendaText(service["status"])) != "CONCLUIDO" {
			continue
		}
		applianceID := agendaText(service["aparelho_id"])
		date := serviceEffectiveDate(service)
		if applianceID != "" && date != "" {
			applianceDates[applianceID+"|"+date] = true
		}
	}
	return ids, applianceDates
}

func historyAlreadyRepresented(history map[string]any, serviceIDs, completedByApplianceDate map[string]bool) bool {
	if serviceID := agendaText(history["service_id"]); serviceID != "" && serviceIDs[serviceID] {
		return true
	}
	date := firstDashboardDate(agendaText(history["data"]), agendaText(history["data_conclusao"]), agendaText(history["data_agendamento"]))
	key := agendaText(history["aparelho_id"]) + "|" + date
	return date != "" && completedByApplianceDate[key]
}

func serviceHistoryAsCompletedService(history map[string]any) map[string]any {
	service := make(map[string]any, len(history)+4)
	for key, value := range history {
		service[key] = value
	}
	service["status"] = "CONCLUIDO"
	if service["data_conclusao"] == nil || agendaText(service["data_conclusao"]) == "" {
		service["data_conclusao"] = firstDashboardDate(agendaText(history["data"]), agendaText(history["data_agendamento"]))
	}
	service["return_date"] = markerDate(agendaText(history["observacoes"]), "PROXIMO_RETORNO")
	if service["observacoes"] == nil {
		service["observacoes"] = ""
	}
	return service
}

func serviceEffectiveDate(service map[string]any) string {
	return firstDashboardDate(agendaText(service["data_conclusao"]), agendaText(service["data_agendamento"]), agendaText(service["data_solicitacao"]))
}

func firstDashboardDate(values ...string) string {
	for _, value := range values {
		if len(value) >= 10 {
			return value[:10]
		}
	}
	return ""
}

func firstDashboardString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func markerDate(observations, marker string) string {
	needle := "[" + marker + ":"
	start := strings.Index(observations, needle)
	if start < 0 {
		return ""
	}
	start += len(needle)
	end := strings.IndexByte(observations[start:], ']')
	if end < 0 {
		return ""
	}
	value := observations[start : start+end]
	if len(value) == 10 && value[4] == '-' && value[7] == '-' {
		if _, err := time.Parse("2006-01-02", value); err == nil {
			return value
		}
	}
	return ""
}

func dashboardCivilToday(now time.Time) time.Time {
	location, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		location = now.Location()
	}
	local := now.In(location)
	year, month, day := local.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, location)
}
