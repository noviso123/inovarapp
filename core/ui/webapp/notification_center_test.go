package webapp

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

func TestNotificationHistoryCapsAtSixtyAndPrependsNewest(t *testing.T) {
	items := make([]localNotification, notificationHistoryLimit)
	for i := range items {
		items[i] = localNotification{ID: fmt.Sprintf("%d", i), When: time.Date(2026, 1, 1, 0, i, 0, 0, time.UTC).Format(time.RFC3339)}
	}
	got := addLocalNotification(items, "Novo", "texto", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	if len(got) != notificationHistoryLimit || got[0].Title != "Novo" || got[notificationHistoryLimit-1].ID != "58" {
		t.Fatalf("history cap/order invalid: len=%d first=%+v last=%+v", len(got), got[0], got[len(got)-1])
	}
}

func TestNotificationReadOperationsAndUnreadCount(t *testing.T) {
	items := []localNotification{{ID: "new"}, {ID: "old", Read: true}}
	if got := countUnreadNotifications(items); got != 1 {
		t.Fatalf("unread=%d want 1", got)
	}
	got := markNotificationRead(items, "new")
	if countUnreadNotifications(got) != 0 || items[0].Read {
		t.Fatalf("read update=%+v original=%+v", got, items)
	}
}

func TestNotificationCenterRendersEmptyStateAndControls(t *testing.T) {
	p := &serviceCatalogPage{notificationCenterOpen: true}
	markup := app.HTMLString(p.notificationCenter())
	for _, want := range []string{"Central de notificações", "Limpar", "Sua caixa está tranquila", "Quando houver novidades", `role="dialog"`} {
		if !strings.Contains(markup, want) {
			t.Errorf("notification center missing %q", want)
		}
	}
}

func TestNotificationCenterRendersUnreadBadgeAndReadItem(t *testing.T) {
	when := time.Now().Add(-24 * time.Hour)
	p := &serviceCatalogPage{notificationCenterOpen: true, notifications: []localNotification{
		{ID: "new", Title: "Orçamento gerado", Text: "Proposta nº 12 criada.", When: when.Format(time.RFC3339Nano)},
		{ID: "old", Title: "OS concluída", Text: "Registro salvo.", Read: true},
	}}
	markup := app.HTMLString(p.notificationCenter())
	for _, want := range []string{`class="notification-center__badge"`, ">1<", "Orçamento gerado", "OS concluída", "Ler todas", "Todas", "Não lidas", "Ontem"} {
		if !strings.Contains(markup, want) {
			t.Errorf("notification center missing %q: %s", want, markup)
		}
	}
}

func TestNotificationCenterUsesSingleBellAndRendersUnreadFilter(t *testing.T) {
	p := &serviceCatalogPage{notificationCenterOpen: true, notificationFilter: "nao-lidas", notifications: []localNotification{
		{ID: "new", Title: "Aviso", Text: "Mensagem", When: time.Now().UTC().Format(time.RFC3339Nano)},
		{ID: "old", Title: "Lido", Text: "Mensagem", Read: true, When: time.Now().UTC().Format(time.RFC3339Nano)},
	}}
	markup := app.HTMLString(p.notificationCenter())
	if strings.Count(markup, `class="notification-center__bell"`) != 1 {
		t.Fatalf("expected one bell control icon, got markup: %s", markup)
	}
	if strings.Contains(markup, "Lido") || !strings.Contains(markup, "Aviso") || !strings.Contains(markup, `notification-center__filter--active`) {
		t.Fatalf("unread filter rendered incorrectly: %s", markup)
	}
}

func TestNotificationRelativeTime(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		at   time.Time
		want string
	}{
		{now.Add(-20 * time.Second), "Agora"},
		{now.Add(-5 * time.Minute), "Há 5 min"},
		{now.Add(-2 * time.Hour), "Hoje, " + now.Add(-2*time.Hour).Local().Format("15:04")},
		{now.Add(-24 * time.Hour), "Ontem, " + now.Add(-24*time.Hour).Local().Format("15:04")},
	} {
		if got := notificationRelativeTime(tc.at, now); got != tc.want {
			t.Errorf("notificationRelativeTime(%s)=%q want %q", tc.at, got, tc.want)
		}
	}
}

func TestNotificationToastRendersLegacyInfoTextAndDismissControl(t *testing.T) {
	p := &serviceCatalogPage{notificationToastTitle: "Agendamento confirmado", notificationToastText: "Limpeza em 04/10/2026 — consulte a Agenda."}
	markup := app.HTMLString(p.notificationToast())
	for _, want := range []string{`role="status"`, "Agendamento confirmado — Limpeza em 04/10/2026 — consulte a Agenda.", "Fechar aviso", "notification-toast__close"} {
		if !strings.Contains(markup, want) {
			t.Errorf("notification toast missing %q: %s", want, markup)
		}
	}
}
