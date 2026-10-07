package webapp

import (
	"fmt"
	"strings"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

type teamCalendarDay struct {
	Date  time.Time
	Items []teamAgendaItem
}

func monthCalendarDays(month string, items []teamAgendaItem, location *time.Location) (time.Time, []teamCalendarDay) {
	first, err := time.ParseInLocation("2006-01", month, location)
	if err != nil {
		now := time.Now().In(location)
		first = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, location)
	}
	gridStart := first.AddDate(0, 0, -int(first.Weekday()))
	days := make([]teamCalendarDay, 42)
	byDate := make(map[string][]teamAgendaItem)
	for _, item := range items {
		if item.Date == "" {
			continue
		}
		if _, err := time.ParseInLocation("2006-01-02", item.Date, location); err == nil {
			byDate[item.Date] = append(byDate[item.Date], item)
		}
	}
	for i := range days {
		date := gridStart.AddDate(0, 0, i)
		days[i] = teamCalendarDay{Date: date, Items: byDate[date.Format("2006-01-02")]}
	}
	return first, days
}

func monthCalendarItems(groups teamAgendaGroups) []teamAgendaItem {
	items := make([]teamAgendaItem, 0, len(groups.Overdue)+len(groups.Upcoming)+len(groups.RecentlyCompleted))
	items = append(items, groups.Overdue...)
	items = append(items, groups.Upcoming...)
	items = append(items, groups.RecentlyCompleted...)
	return items
}

func allTeamAgendaItems(services, appointments []map[string]any) []teamAgendaItem {
	items := make([]teamAgendaItem, 0, len(services))
	appointmentsByService := make(map[string]map[string]any)
	for _, appointment := range appointments {
		serviceID := portalText(appointment["service_id"])
		if serviceID != "" {
			appointmentsByService[serviceID] = appointment
		}
	}
	for _, service := range services {
		status := strings.ToUpper(agendaText(service["status"]))
		if status != "AGENDADO" && status != "EM_ANDAMENTO" && status != "CONCLUIDO" {
			continue
		}
		appointment := appointmentsByService[portalText(service["id"])]
		items = append(items, makeTeamAgendaItem(withAppointmentSchedule(service, appointment)))
	}
	return items
}

// withAppointmentSchedule keeps the agenda aligned with the authoritative appointment
// row when legacy or offline writes left its date/time ahead of the service record.
// Copy the service map so rendering/edit handlers still receive the original row.
func withAppointmentSchedule(service, appointment map[string]any) map[string]any {
	if len(appointment) == 0 {
		return service
	}
	copy := make(map[string]any, len(service)+2)
	for key, value := range service {
		copy[key] = value
	}
	if date := portalText(appointment["data"]); date != "" {
		copy["data_agendamento"] = date
	}
	if hour := portalText(appointment["hora"]); hour != "" {
		copy["hora_agendamento"] = hour
	}
	return copy
}

func (p *serviceCatalogPage) shiftTeamAgendaMonth(delta int) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		location, err := time.LoadLocation("America/Sao_Paulo")
		if err != nil {
			location = time.Local
		}
		month, _ := monthCalendarDays(p.teamAgendaMonth, nil, location)
		p.teamAgendaMonth = month.AddDate(0, delta, 0).Format("2006-01")
		ctx.Update()
	}
}

func (p *serviceCatalogPage) shiftTeamAgendaWeek(delta int) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		location, err := time.LoadLocation("America/Sao_Paulo")
		if err != nil {
			location = time.Local
		}
		anchor, err := time.ParseInLocation("2006-01-02", p.teamAgendaWeek, location)
		if err != nil {
			anchor = time.Now().In(location)
		}
		p.teamAgendaWeek = anchor.AddDate(0, 0, 7*delta).Format("2006-01-02")
		ctx.Update()
	}
}

func (p *serviceCatalogPage) setTeamAgendaView(view string) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		p.teamAgendaView = view
		ctx.Update()
	}
}

func (p *serviceCatalogPage) setTeamAgendaStatus(ctx app.Context, event app.Event) {
	p.teamAgendaStatus = event.Get("target").Get("value").String()
	ctx.Update()
}

func (p *serviceCatalogPage) teamMonthlyCalendar(calendarItems []teamAgendaItem) app.UI {
	location, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		location = time.Local
	}
	now := time.Now().In(location)
	monthValue := strings.TrimSpace(p.teamAgendaMonth)
	if monthValue == "" {
		monthValue = now.Format("2006-01")
		p.teamAgendaMonth = monthValue
	}
	if p.teamAgendaStatus == "" {
		p.teamAgendaStatus = "TODOS"
	}
	items := filterTeamAgendaStatus(calendarItems, p.teamAgendaStatus)
	first, days := monthCalendarDays(monthValue, items, location)
	labels := [...]string{"Dom", "Seg", "Ter", "Qua", "Qui", "Sex", "Sáb"}
	headers := make([]app.UI, len(labels))
	for i, label := range labels {
		headers[i] = app.Div().Class("team-calendar__weekday").Body(app.Text(label))
	}
	cells := make([]app.UI, 0, len(days))
	for _, day := range days {
		classes := "team-calendar__day"
		if day.Date.Month() != first.Month() {
			classes += " team-calendar__day--outside"
		}
		if day.Date.Format("2006-01-02") == now.Format("2006-01-02") {
			classes += " team-calendar__day--today"
		}
		items := []app.UI{app.Span().Class("team-calendar__date").Body(app.Text(day.Date.Day()))}
		for i, item := range day.Items {
			if i == 3 {
				items = append(items, app.Span().Class("team-calendar__more").Body(app.Text(fmt.Sprintf("+%d", len(day.Items)-3))))
				break
			}
			label := item.Customer
			if label == "" {
				label = item.Service
			}
			_, tone := portalServiceStatus(item.Status)
			items = append(items, app.Button().Class("team-calendar__event team-calendar__event--"+tone).Type("button").OnClick(p.openTeamServiceEdit(item.Source)).Body(app.Text(label)))
		}
		cells = append(cells, app.Div().Class(classes).Body(items...))
	}
	return app.Section().ID("team-calendar").Class("team-calendar").Body(
		p.teamCalendarControls(first),
		app.Div().Class("team-calendar__toolbar").Body(
			app.Button().Class("auth-link").Type("button").OnClick(p.shiftTeamAgendaMonth(-1)).Body(app.Text("‹")),
			app.Strong().Body(app.Text(monthNamePortuguese(first.Month())+" "+fmt.Sprint(first.Year()))),
			app.Button().Class("auth-link").Type("button").OnClick(p.shiftTeamAgendaMonth(1)).Body(app.Text("›")),
		),
		app.Div().Class("team-calendar__grid").Body(append(headers, cells...)...),
		app.P().Class("team-calendar__hint").Body(app.Text("Selecione um atendimento para abrir a edição da OS. Datas e horários vêm dos agendamentos sincronizados.")),
	)
}

func filterTeamAgendaStatus(items []teamAgendaItem, status string) []teamAgendaItem {
	if status == "" || status == "TODOS" {
		return items
	}
	filtered := make([]teamAgendaItem, 0, len(items))
	for _, item := range items {
		if item.Status == status {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func weekCalendarDays(anchor string, items []teamAgendaItem, location *time.Location) (time.Time, []teamCalendarDay) {
	date, err := time.ParseInLocation("2006-01-02", anchor, location)
	if err != nil {
		date = time.Now().In(location)
	}
	start := date.AddDate(0, 0, -int(date.Weekday()))
	_, monthDays := monthCalendarDays(start.Format("2006-01"), items, location)
	days := make([]teamCalendarDay, 0, 7)
	for _, day := range monthDays {
		if !day.Date.Before(start) && day.Date.Before(start.AddDate(0, 0, 7)) {
			days = append(days, day)
		}
	}
	return start, days
}

func (p *serviceCatalogPage) teamCalendarControls(date time.Time) app.UI {
	view := p.teamAgendaView
	if view != "week" {
		view = "month"
	}
	weekClass, monthClass := "auth-link", "auth-link"
	if view == "week" {
		weekClass = "auth-submit"
	} else {
		monthClass = "auth-submit"
	}
	return app.Div().Class("team-calendar__controls").Body(
		app.Div().Class("team-calendar__views").Body(
			app.Button().Class(monthClass).Type("button").OnClick(p.setTeamAgendaView("month")).Body(app.Text("Mês")),
			app.Button().Class(weekClass).Type("button").OnClick(p.setTeamAgendaView("week")).Body(app.Text("Semana")),
		),
		app.Label().Class("team-calendar__filter").Body(app.Text("Status"), app.Select().Attr("value", p.teamAgendaStatus).OnChange(p.setTeamAgendaStatus).Body(
			app.Option().Value("TODOS").Body(app.Text("Todos")),
			app.Option().Value("AGENDADO").Body(app.Text("Agendados")),
			app.Option().Value("EM_ANDAMENTO").Body(app.Text("Em andamento")),
			app.Option().Value("CONCLUIDO").Body(app.Text("Concluídos")),
		)),
		app.Button().Class("auth-link").Type("button").OnClick(func(ctx app.Context, event app.Event) {
			event.PreventDefault()
			p.teamAgendaStatus = "TODOS"
			now := time.Now().In(date.Location())
			p.teamAgendaMonth = now.Format("2006-01")
			p.teamAgendaWeek = now.Format("2006-01-02")
			ctx.Update()
		}).Body(app.Text("Hoje")),
	)
}

func (p *serviceCatalogPage) teamWeeklyCalendar(items []teamAgendaItem) app.UI {
	location, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		location = time.Local
	}
	now := time.Now().In(location)
	if _, err := time.ParseInLocation("2006-01-02", p.teamAgendaWeek, location); err != nil {
		p.teamAgendaWeek = now.Format("2006-01-02")
	}
	start, weekDays := weekCalendarDays(p.teamAgendaWeek, filterTeamAgendaStatus(items, p.teamAgendaStatus), location)
	end := start.AddDate(0, 0, 6)
	columns := make([]app.UI, 0, len(weekDays))
	for _, day := range weekDays {
		classes := "team-calendar__weekday"
		if day.Date.Format("2006-01-02") == now.Format("2006-01-02") {
			classes += " team-calendar__weekday--today"
		}
		events := []app.UI{app.Strong().Body(app.Text(weekdayNamePortuguese(day.Date.Weekday()) + " " + day.Date.Format("02/01")))}
		for _, item := range day.Items {
			label := strings.TrimSpace(item.Time + " " + item.Customer)
			if item.Customer == "" {
				label = strings.TrimSpace(item.Time + " " + item.Service)
			}
			_, tone := portalServiceStatus(item.Status)
			events = append(events, app.Button().Class("team-calendar__event team-calendar__event--"+tone).Type("button").OnClick(p.openTeamServiceEdit(item.Source)).Body(app.Text(label)))
		}
		if len(day.Items) == 0 {
			events = append(events, app.Span().Class("team-calendar__more").Body(app.Text("Sem atendimentos")))
		}
		columns = append(columns, app.Div().Class(classes).Body(events...))
	}
	return app.Section().ID("team-calendar").Class("team-calendar").Body(
		p.teamCalendarControls(start),
		app.Div().Class("team-calendar__toolbar").Body(
			app.Button().Class("auth-link").Type("button").OnClick(p.shiftTeamAgendaWeek(-1)).Body(app.Text("‹ Semana anterior")),
			app.Strong().Body(app.Text(start.Format("02/01")+" – "+end.Format("02/01/2006"))),
			app.Button().Class("auth-link").Type("button").OnClick(p.shiftTeamAgendaWeek(1)).Body(app.Text("Próxima semana ›")),
		),
		app.Div().Class("team-calendar__week").Body(columns...),
		app.P().Class("team-calendar__hint").Body(app.Text("Selecione um atendimento para abrir a edição da OS.")),
	)
}

func monthNamePortuguese(month time.Month) string {
	names := [...]string{"", "Janeiro", "Fevereiro", "Março", "Abril", "Maio", "Junho", "Julho", "Agosto", "Setembro", "Outubro", "Novembro", "Dezembro"}
	return names[month]
}

func weekdayNamePortuguese(day time.Weekday) string {
	names := [...]string{"Dom", "Seg", "Ter", "Qua", "Qui", "Sex", "Sáb"}
	return names[day]
}
