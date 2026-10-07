package domain

import (
	"math"
	"sort"
	"strings"
	"time"
)

type AlertCustomer struct {
	ID, Name, WhatsApp, ProfileID, Email string
	Active                               bool
}
type AlertAppliance struct{ ID, CustomerID, Brand, Model, BTUs, Room, LastMaintenance string }
type AlertHistory struct{ CustomerID, ApplianceID, Date, Observations string }
type AlertService struct {
	ID, CustomerID, ApplianceID, Type, Description, Status            string
	ScheduledDate, CompletionDate, ScheduledTime, Value, Observations string
}
type PreventiveAlert struct {
	Customer                      AlertCustomer
	Appliance                     AlertAppliance
	LastServiceDate, ExpectedDate string
	DaysToDue, ReturnMonths       int
}
type AgendaReminder struct {
	Customer                          AlertCustomer
	Service                           AlertService
	ApplianceDescription, ServiceName string
}

// BuildPreventiveAlerts preserves the legacy cycle calculation: latest completed
// history/service/appliance-maintenance date wins, alerts start five days before
// the due date and stop after maxLateDays past due.
func BuildPreventiveAlerts(customers []AlertCustomer, appliances []AlertAppliance, history []AlertHistory, services []AlertService, months, maxLateDays int, now time.Time) []PreventiveAlert {
	return BuildPreventiveAlertsWithDefault(customers, appliances, history, services, months, maxLateDays, now)
}

// BuildPreventiveAlertsWithDefault uses the technician's configured interval as
// a fallback and honors a return date explicitly saved on a completed service.
func BuildPreventiveAlertsWithDefault(customers []AlertCustomer, appliances []AlertAppliance, history []AlertHistory, services []AlertService, months, maxLateDays int, now time.Time) []PreventiveAlert {
	if months <= 0 {
		months = 6
	}
	if maxLateDays <= 0 {
		maxLateDays = 180
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second(), now.Nanosecond(), time.UTC)
	type serviceCycle struct{ serviceDate, returnDate string }
	last := map[string]serviceCycle{}
	consider := func(customerID, applianceID, value, returnDate string) {
		d := alertISODate(value)
		if customerID == "" || applianceID == "" || d == "" {
			return
		}
		key := customerID + "|" + applianceID
		if existing, ok := last[key]; !ok || d > existing.serviceDate {
			last[key] = serviceCycle{serviceDate: d, returnDate: alertISODate(returnDate)}
		}
	}
	for _, entry := range history {
		consider(entry.CustomerID, entry.ApplianceID, entry.Date, alertReturnDate(entry.Observations))
	}
	for _, service := range services {
		if strings.EqualFold(service.Status, "CONCLUIDO") {
			date := service.CompletionDate
			if alertISODate(date) == "" {
				date = service.ScheduledDate
			}
			consider(service.CustomerID, service.ApplianceID, date, alertReturnDate(service.Observations))
		}
	}
	for _, appliance := range appliances {
		consider(appliance.CustomerID, appliance.ID, appliance.LastMaintenance, "")
	}
	customerByID := make(map[string]AlertCustomer, len(customers))
	for _, customer := range customers {
		if customer.Active {
			customerByID[customer.ID] = customer
		}
	}
	applianceByID := make(map[string]AlertAppliance, len(appliances))
	for _, appliance := range appliances {
		applianceByID[appliance.ID] = appliance
	}
	alerts := make([]PreventiveAlert, 0)
	for key, cycle := range last {
		customerID, applianceID, ok := strings.Cut(key, "|")
		if !ok {
			continue
		}
		customer, ok := customerByID[customerID]
		if !ok {
			continue
		}
		appliance, ok := applianceByID[applianceID]
		if !ok {
			continue
		}
		date, err := time.Parse("2006-01-02", cycle.serviceDate)
		if err != nil {
			continue
		}
		expected := date.AddDate(0, months, 0)
		if cycle.returnDate != "" {
			if custom, parseErr := time.Parse("2006-01-02", cycle.returnDate); parseErr == nil {
				expected = custom
			}
		}
		days := int(math.Floor(expected.Sub(today).Hours()/24 + 0.5))
		if days > 5 || days < -maxLateDays {
			continue
		}
		returnMonths := monthsBetween(date, expected)
		alerts = append(alerts, PreventiveAlert{Customer: customer, Appliance: appliance, LastServiceDate: cycle.serviceDate, ExpectedDate: expected.Format("2006-01-02"), DaysToDue: days, ReturnMonths: returnMonths})
	}
	sort.Slice(alerts, func(i, j int) bool {
		if alerts[i].ExpectedDate == alerts[j].ExpectedDate {
			return alerts[i].Customer.ID+alerts[i].Appliance.ID < alerts[j].Customer.ID+alerts[j].Appliance.ID
		}
		return alerts[i].ExpectedDate < alerts[j].ExpectedDate
	})
	return alerts
}

func alertReturnDate(observations string) string {
	const marker = "[PROXIMO_RETORNO:"
	start := strings.Index(observations, marker)
	if start < 0 {
		return ""
	}
	start += len(marker)
	end := strings.IndexByte(observations[start:], ']')
	if end < 0 {
		return ""
	}
	return alertISODate(observations[start : start+end])
}

func monthsBetween(start, end time.Time) int {
	months := (end.Year()-start.Year())*12 + int(end.Month()-start.Month())
	if end.Day() < start.Day() {
		months--
	}
	if months < 1 {
		return 1
	}
	return months
}

func BuildAgendaReminder(service AlertService, customer AlertCustomer, appliance *AlertAppliance) AgendaReminder {
	return AgendaReminder{Customer: customer, Service: service, ApplianceDescription: AlertEquipmentDescription(appliance, true), ServiceName: AlertServiceName(service.Type, service.Description)}
}

func AlertEquipmentDescription(appliance *AlertAppliance, legacyDetail bool) string {
	if appliance == nil {
		return "equipamento não informado no cadastro"
	}
	if legacyDetail {
		room := appliance.Room
		if room == "" {
			room = "ambiente não informado"
		}
		return strings.TrimSpace(strings.Join(nonEmptyAlertParts(appliance.Brand, appliance.Model, appliance.BTUs+" BTUs —", room), " "))
	}
	room := appliance.Room
	if room == "" {
		room = "aparelho"
	}
	return strings.TrimSpace(strings.Join(nonEmptyAlertParts(appliance.Brand, appliance.Model, appliance.BTUs+" BTUs ("+room+")"), " "))
}

func AlertServiceName(kind, description string) string {
	description = strings.TrimSpace(description)
	if description != "" && !strings.HasPrefix(description, "{") && !strings.HasPrefix(description, "[") {
		return description
	}
	labels := map[string]string{"LIMPEZA": "Limpeza de Ar", "INSTALACAO": "Instalação", "MANUTENCAO_PREVENTIVA": "Manutenção Preventiva", "MANUTENCAO_CORRETIVA": "Manutenção Corretiva", "RECARGA_GAS": "Recarga de Gás", "AVALIACAO": "Avaliação Técnica", "OUTRO": "Serviço"}
	if label := labels[kind]; label != "" {
		return label
	}
	if kind != "" {
		return strings.ReplaceAll(kind, "_", " ")
	}
	return "Serviço"
}

func alertISODate(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 10 {
		value = value[:10]
	}
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return ""
	}
	return value
}
func nonEmptyAlertParts(parts ...string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			out = append(out, strings.TrimSpace(part))
		}
	}
	return out
}
