package webapp

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type teamAgendaItem struct {
	ID, Status, Date, Time                       string
	Started, Completed, Cancelled                string
	Service, Customer, Phone, Address, Appliance string
	Value                                        float64
	Source                                       map[string]any
}

type teamAgendaGroups struct {
	Overdue, Upcoming, RecentlyCompleted []teamAgendaItem
}

func splitTeamAgenda(items []teamAgendaItem, today string) (upcoming, overdue, completed []teamAgendaItem) {
	for _, item := range items {
		if item.Status == "CONCLUIDO" {
			completed = append(completed, item)
		} else if item.Date < today {
			overdue = append(overdue, item)
		} else {
			upcoming = append(upcoming, item)
		}
	}
	sort.SliceStable(upcoming, func(i, j int) bool { return upcoming[i].Date < upcoming[j].Date })
	sort.SliceStable(overdue, func(i, j int) bool { return overdue[i].Date < overdue[j].Date })
	if len(completed) > 8 {
		completed = completed[len(completed)-8:]
	}
	for left, right := 0, len(completed)-1; left < right; left, right = left+1, right-1 {
		completed[left], completed[right] = completed[right], completed[left]
	}
	return upcoming, overdue, completed
}

func buildTeamAgenda(services []map[string]any, today string) teamAgendaGroups {
	items := make([]teamAgendaItem, 0, len(services))
	for _, service := range services {
		status := strings.ToUpper(agendaText(service["status"]))
		if status != "AGENDADO" && status != "EM_ANDAMENTO" && status != "CONCLUIDO" {
			continue
		}
		items = append(items, makeTeamAgendaItem(service))
	}
	upcoming, overdue, completed := splitTeamAgenda(items, today)
	return teamAgendaGroups{Upcoming: upcoming, Overdue: overdue, RecentlyCompleted: completed}
}

// The React AgendaTab exposes completion only after a service has been started.
func canFinishTeamAgenda(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "EM_ANDAMENTO")
}

func makeTeamAgendaItem(service map[string]any) teamAgendaItem {
	item := teamAgendaItem{
		ID: agendaText(service["id"]), Status: strings.ToUpper(agendaText(service["status"])),
		Date: agendaDate(service["data_agendamento"]), Time: agendaText(service["hora_agendamento"]),
		Started: agendaDate(service["data_inicio"]), Completed: agendaDate(service["data_conclusao"]),
		Cancelled: agendaDate(service["data_cancelamento"]), Service: agendaText(service["tipo"]),
		Value: agendaNumber(service["valor"]), Source: service,
	}
	if item.Service == "OUTRO" && agendaText(service["descricao"]) != "" {
		item.Service = agendaText(service["descricao"])
	}
	if customer, ok := service["customers"].(map[string]any); ok {
		item.Customer = agendaText(customer["nome"])
		item.Phone = agendaText(customer["whatsapp"])
		item.Address = strings.TrimSpace(strings.Join(nonEmptyAgendaParts(
			agendaText(customer["endereco"]), agendaText(customer["bairro"]), agendaText(customer["cidade"]), agendaText(customer["estado"]),
		), " • "))
	}
	if appliance, ok := service["air_conditioners"].(map[string]any); ok {
		capacity := ""
		if appliance["btus"] != nil {
			if value, err := strconv.ParseFloat(fmt.Sprint(appliance["btus"]), 64); err == nil && value > 0 {
				capacity = portalBTUs(value)
			}
		}
		item.Appliance = strings.TrimSpace(strings.Join(nonEmptyAgendaParts(
			agendaText(appliance["marca"])+" "+agendaText(appliance["modelo"]), capacity, agendaText(appliance["ambiente"]),
		), " • "))
	}
	if item.Date == "" {
		item.Date = agendaDate(service["data_solicitacao"])
	}
	return item
}

func agendaSortDate(item teamAgendaItem) string {
	if item.Completed != "" {
		return item.Completed
	}
	return item.Date
}

func agendaDate(value any) string {
	date := agendaText(value)
	if len(date) >= 10 {
		return date[:10]
	}
	return date
}

func agendaText(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func agendaNumber(value any) float64 {
	switch number := value.(type) {
	case float64:
		return number
	case int:
		return float64(number)
	default:
		return 0
	}
}

func nonEmptyAgendaParts(parts ...string) []string {
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}
