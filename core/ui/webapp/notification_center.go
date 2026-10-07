package webapp

import (
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

const notificationStorageKey = "inovarapp_notifs_v1"
const notificationHistoryLimit = 60

type localNotification struct {
	ID    string `json:"id"`
	When  string `json:"quando"`
	Title string `json:"titulo"`
	Text  string `json:"texto"`
	Read  bool   `json:"lida"`
}

func countUnreadNotifications(items []localNotification) int {
	count := 0
	for _, item := range items {
		if !item.Read {
			count++
		}
	}
	return count
}

func addLocalNotification(items []localNotification, title, body string, now time.Time) []localNotification {
	item := localNotification{ID: uuid.NewString(), When: now.UTC().Format(time.RFC3339Nano), Title: title, Text: body}
	updated := make([]localNotification, 0, min(len(items)+1, notificationHistoryLimit))
	updated = append(updated, item)
	updated = append(updated, items...)
	if len(updated) > notificationHistoryLimit {
		updated = updated[:notificationHistoryLimit]
	}
	return updated
}

func markNotificationRead(items []localNotification, id string) []localNotification {
	updated := append([]localNotification(nil), items...)
	for i := range updated {
		if updated[i].ID == id {
			updated[i].Read = true
			break
		}
	}
	return updated
}

func (p *serviceCatalogPage) loadLocalNotifications(ctx app.Context) {
	var items []localNotification
	if err := ctx.LocalStorage().Get(notificationStorageKey, &items); err != nil {
		items = nil
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].When > items[j].When })
	if len(items) > notificationHistoryLimit {
		items = items[:notificationHistoryLimit]
	}
	p.notifications = items
}

func (p *serviceCatalogPage) registerLocalNotification(ctx app.Context, title, body string) {
	p.loadLocalNotifications(ctx)
	p.notifications = addLocalNotification(p.notifications, title, body, time.Now())
	_ = ctx.LocalStorage().Set(notificationStorageKey, p.notifications)
	p.notificationToastTitle, p.notificationToastText = title, body
	notifyDevice(title, body)
	if p.session != nil && p.session.AccessToken != "" {
		token, endpoint := p.session.AccessToken, apiBaseURL()+"/api/notificacoes"
		go func() {
			_ = sendTeamJSON(ctx, endpoint, token, "POST", map[string]any{"acao": "enviar", "titulo": title, "texto": body, "url": "/"})
		}()
	}
	ctx.Update()
}

func (p *serviceCatalogPage) notificationToast() app.UI {
	return app.Div().Class("notification-toast").Role("status").Body(
		app.Span().Class("notification-toast__icon").Body(app.Text("●")),
		app.Span().Body(app.Text(p.notificationToastTitle+" — "+p.notificationToastText)),
		app.Button().Class("notification-toast__close").Type("button").Title("Fechar aviso").OnClick(func(ctx app.Context, event app.Event) {
			event.PreventDefault()
			p.notificationToastTitle, p.notificationToastText = "", ""
			ctx.Update()
		}).Body(app.Text("×")),
	)
}

func (p *serviceCatalogPage) notificationCenter() app.UI {
	count := countUnreadNotifications(p.notifications)
	icon := app.Span().Class("notification-center__bell").Body(app.Text("🔔"))
	buttonBody := []app.UI{icon}
	if count > 0 {
		label := itoaPhoto(count)
		if count > 9 {
			label = "9+"
		}
		buttonBody = append(buttonBody, app.Span().Class("notification-center__badge").Body(app.Text(label)))
	}
	button := app.Button().Class("notification-center__toggle").Type("button").Title("Notificações").Aria("label", notificationAriaLabel(count)).Aria("expanded", ariaBoolean(p.notificationCenterOpen)).Body(buttonBody...).OnClick(func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		p.notificationCenterOpen = !p.notificationCenterOpen
		if p.notificationCenterOpen {
			p.loadLocalNotifications(ctx)
			if p.notificationFilter == "" {
				p.notificationFilter = "todas"
			}
		}
		ctx.Update()
	})
	if !p.notificationCenterOpen {
		return app.Div().Class("notification-center").Body(button)
	}

	controls := []app.UI{
		app.Button().Class("notification-center__action notification-center__action--clear").Type("button").Disabled(len(p.notifications) == 0).Title("Limpar histórico").OnClick(func(ctx app.Context, event app.Event) {
			event.PreventDefault()
			ctx.LocalStorage().Del(notificationStorageKey)
			p.notifications = nil
			p.notificationFilter = "todas"
			ctx.Update()
		}).Body(app.Text("Limpar")),
	}
	if count > 0 {
		controls = append(controls, app.Button().Class("notification-center__action notification-center__action--read-all").Type("button").OnClick(func(ctx app.Context, event app.Event) {
			event.PreventDefault()
			for i := range p.notifications {
				p.notifications[i].Read = true
			}
			_ = ctx.LocalStorage().Set(notificationStorageKey, p.notifications)
			ctx.Update()
		}).Body(app.Text("Ler todas")))
	}
	controls = append(controls, app.Button().Class("notification-center__close").Type("button").Title("Fechar notificações").Aria("label", "Fechar notificações").OnClick(func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		p.notificationCenterOpen = false
		ctx.Update()
	}).Body(app.Text("×")))

	rows := make([]app.UI, 0, len(p.notifications))
	visibleCount := 0
	if len(p.notifications) == 0 {
		rows = append(rows, app.Div().Class("notification-center__empty").Body(
			app.Div().Class("notification-center__empty-icon").Body(app.Text("🔔")),
			app.Strong().Body(app.Text("Sua caixa está tranquila")),
			app.P().Body(app.Text("Quando houver novidades sobre chamados, orçamentos ou serviços, elas aparecerão aqui.")),
		))
	} else {
		filter := p.notificationFilter
		if filter == "" {
			filter = "todas"
		}
		for _, item := range p.notifications {
			item := item
			if filter == "nao-lidas" && item.Read {
				continue
			}
			visibleCount++
			stamp := ""
			if parsed, err := time.Parse(time.RFC3339Nano, item.When); err == nil {
				stamp = notificationRelativeTime(parsed, time.Now())
			}
			className := "notification-center__item"
			if !item.Read {
				className += " notification-center__item--unread"
			}
			rows = append(rows, app.Div().Class("notification-center__row").Body(
				app.Span().Class("notification-center__item-icon").Body(app.Text("🔔")),
				app.Button().Class(className).Type("button").Title(func() string {
					if item.Read {
						return "Notificação lida"
					}
					return "Marcar como lida"
				}()).OnClick(func(ctx app.Context, event app.Event) {
					event.PreventDefault()
					p.notifications = markNotificationRead(p.notifications, item.ID)
					_ = ctx.LocalStorage().Set(notificationStorageKey, p.notifications)
					ctx.Update()
				}).Body(
					app.Div().Class("notification-center__item-heading").Body(app.Strong().Body(app.Text(item.Title)), app.Span().Body(app.Text(stamp))),
					app.P().Body(app.Text(item.Text)),
				),
				func() app.UI {
					if item.Read {
						return app.Span()
					}
					return app.Button().Class("notification-center__mark-read").Type("button").Title("Marcar como lida").Aria("label", "Marcar notificação como lida").OnClick(func(ctx app.Context, event app.Event) {
						event.PreventDefault()
						p.notifications = markNotificationRead(p.notifications, item.ID)
						_ = ctx.LocalStorage().Set(notificationStorageKey, p.notifications)
						ctx.Update()
					}).Body(app.Text("✓"))
				}(),
			))
		}
		if visibleCount == 0 {
			rows = append(rows, app.Div().Class("notification-center__empty notification-center__empty--filtered").Body(app.Text("Tudo foi lido. Não há notificações novas.")))
		}
	}

	var filterBar app.UI = app.Span()
	if len(p.notifications) > 0 {
		allClass, unreadClass := "notification-center__filter", "notification-center__filter"
		if p.notificationFilter == "todas" || p.notificationFilter == "" {
			allClass += " notification-center__filter--active"
		}
		if p.notificationFilter == "nao-lidas" {
			unreadClass += " notification-center__filter--active"
		}
		filterBar = app.Div().Class("notification-center__filters").Body(
			app.Button().Class(allClass).Type("button").OnClick(func(ctx app.Context, event app.Event) {
				event.PreventDefault()
				p.notificationFilter = "todas"
				ctx.Update()
			}).Body(app.Text("Todas"), app.Span().Body(app.Text(itoaPhoto(len(p.notifications))))),
			app.Button().Class(unreadClass).Type("button").OnClick(func(ctx app.Context, event app.Event) {
				event.PreventDefault()
				p.notificationFilter = "nao-lidas"
				ctx.Update()
			}).Body(app.Text("Não lidas"), app.Span().Body(app.Text(itoaPhoto(count)))),
		)
	}

	return app.Div().Class("notification-center").Body(
		button,
		app.Button().Class("notification-center__scrim").Type("button").Title("Fechar notificações").Aria("label", "Fechar notificações").OnClick(func(ctx app.Context, event app.Event) {
			event.PreventDefault()
			p.notificationCenterOpen = false
			ctx.Update()
		}),
		app.Div().Class("notification-center__panel").Role("dialog").Aria("label", "Central de notificações").Body(
			app.Div().Class("notification-center__header").Body(
				app.Span().Class("notification-center__header-icon").Body(app.Text("🔔")),
				app.Div().Class("notification-center__heading-copy").Body(
					app.Strong().Body(app.Text("Central de notificações")),
					app.Span().Body(app.Text(func() string {
						if count == 0 {
							return "Você está em dia"
						}
						return itoaPhoto(count) + " não lida(s)"
					}())),
				),
				app.Div().Class("notification-center__header-actions").Body(controls...),
			),
			filterBar,
			app.Div().Class("notification-center__list").Body(rows...),
			func() app.UI {
				if len(p.notifications) == 0 {
					return app.Span()
				}
				return app.Div().Class("notification-center__footer").Body(app.Text("✓ Sincronizado neste dispositivo"), app.Span().Body(app.Text(itoaPhoto(len(p.notifications))+"/"+itoaPhoto(notificationHistoryLimit))))
			}(),
		),
	)
}

func notificationAriaLabel(count int) string {
	if count == 0 {
		return "Notificações"
	}
	return "Notificações, " + itoaPhoto(count) + " não lidas"
}

func notificationRelativeTime(value, now time.Time) string {
	delta := now.Sub(value)
	if delta < 0 {
		return value.Local().Format("02/01 15:04")
	}
	if delta < time.Minute {
		return "Agora"
	}
	if delta < time.Hour {
		return "Há " + itoaPhoto(int(delta.Minutes())) + " min"
	}
	if value.Local().Format("2006-01-02") == now.Local().Format("2006-01-02") {
		return "Hoje, " + value.Local().Format("15:04")
	}
	if value.Local().Format("2006-01-02") == now.AddDate(0, 0, -1).Local().Format("2006-01-02") {
		return "Ontem, " + value.Local().Format("15:04")
	}
	return value.Local().Format("02/01 15:04")
}
