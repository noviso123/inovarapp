package webapp

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/domain"
)

func TestCustomerPortalMaintenanceDateUsesUTCDayLikeLegacy(t *testing.T) {
	// In Brazil this instant is still the previous local calendar day, while
	// JavaScript's toISOString().slice(0, 10) persists the UTC date.
	local := time.Date(2026, time.October, 4, 22, 30, 0, 0, time.FixedZone("BRT", -3*60*60))
	if got, want := customerPortalMaintenanceDate(local), "2026-10-05"; got != want {
		t.Fatalf("maintenance date=%q want UTC legacy day %q", got, want)
	}
}

func TestCustomerPortalMaintenanceRemindersMatchSixMonthCalendarCycle(t *testing.T) {
	brt := time.FixedZone("BRT", -3*60*60)
	current := time.Date(2026, time.October, 5, 12, 0, 0, 0, brt)
	appliances := []map[string]any{
		{"marca": "LG", "ambiente": "Sala", "ultima_manutencao": "2026-04-05"}, // vence hoje
		{"marca": "Gree", "ultima_manutencao": "2026-04-12"},                   // vence em 7 dias
		{"marca": "Midea", "ultima_manutencao": "2026-04-13"},                  // fora da janela
		{"marca": "Daikin", "ultima_manutencao": "2025-12-31"},                 // vencido
		{"marca": "Inválido", "ultima_manutencao": "2026-02-30"},               // data inválida
	}
	got := customerMaintenanceReminders(appliances, current, brt)
	if len(got) != 3 {
		t.Fatalf("reminders=%#v, want due today, due in seven days and overdue", got)
	}
	if got[0].Days != 0 || got[0].Date != "05/10" || got[0].Brand != "LG" {
		t.Errorf("today reminder=%#v", got[0])
	}
	if got[1].Days != 7 || got[1].Date != "12/10" {
		t.Errorf("seven-day reminder=%#v", got[1])
	}
	if got[2].Days >= 0 {
		t.Errorf("overdue reminder=%#v", got[2])
	}
	if got := customerMaintenanceReminders([]map[string]any{{"ultima_manutencao": "2024-08-31"}}, current, brt); len(got) != 1 || got[0].Date != "28/02" {
		t.Fatalf("month-end clamp reminder=%#v, want February 28", got)
	}
}

func TestCustomerMaintenanceReminderCopyAndWhatsAppLinkPreserveLegacyContract(t *testing.T) {
	if got, want := customerMaintenanceReminderText([]customerMaintenanceReminder{{Brand: "LG", Room: "Sala", Date: "05/10", Days: -2}}), "O ciclo de limpeza de ar do seu LG (Sala) venceu há 2 dia(s) (retornava em 05/10). Manter o ciclo evita fungos, mau cheiro e maior consumo de energia."; got != want {
		t.Fatalf("reminder copy=%q want %q", got, want)
	}
	if got, want := customerMaintenanceReminderText([]customerMaintenanceReminder{{}, {}}), "Os ciclos de limpeza de ar de 2 aparelhos estão no prazo de limpeza de ar (ciclo de 6 meses)."; got != want {
		t.Fatalf("multiple reminder copy=%q want %q", got, want)
	}
	for phone, want := range map[string]string{"(27) 99827-9185": "5527998279185", "+55 27 99827-9185": "5527998279185", "": "5527999999999"} {
		if got := customerBusinessWhatsAppPhone(phone); got != want {
			t.Errorf("normalized company phone for %q = %q want %q", phone, got, want)
		}
	}
	if got, want := customerBusinessWhatsAppURL("27998279185"), "https://wa.me/5527998279185?text=Ol%C3%A1%20sou%20cliente%20da%20Inovar%20e%20gostaria%20de%20tirar%20uma%20d%C3%BAvida"; got != want {
		t.Fatalf("WhatsApp link=%q want %q", got, want)
	}
}

func TestCustomerPortalServiceStatusChangesNotifyOnlyExistingServices(t *testing.T) {
	services := []map[string]any{
		{"id": "service-1", "status": "CONCLUIDO", "tipo": "MANUTENCAO_PREVENTIVA"},
		{"id": "service-2", "status": "PENDENTE", "tipo": "LIMPEZA"},
	}
	current, initialNotifications := customerPortalServiceStatusChanges(services, nil)
	if len(current) != 2 || len(initialNotifications) != 0 {
		t.Fatalf("initial snapshot=%#v notifications=%#v", current, initialNotifications)
	}
	services[0]["status"] = "AGENDADO"
	services[1]["status"] = "CANCELADO"
	updated, notifications := customerPortalServiceStatusChanges(services, current)
	if len(updated) != 2 || len(notifications) != 2 {
		t.Fatalf("updated snapshot=%#v notifications=%#v", updated, notifications)
	}
	if notifications[0] != (customerPortalStatusNotification{Title: "✅ Serviço AGENDADO pela Inovar", Body: "MANUTENCAO PREVENTIVA"}) || notifications[1] != (customerPortalStatusNotification{Title: "⚠️ Serviço cancelado", Body: "LIMPEZA"}) {
		t.Fatalf("status notifications=%#v", notifications)
	}
	if _, repeated := customerPortalServiceStatusChanges(services, updated); len(repeated) != 0 {
		t.Fatalf("unchanged statuses repeated notifications: %#v", repeated)
	}
}

func TestCustomerPortalRendersPreventiveMaintenanceReminderAndScheduleAction(t *testing.T) {
	page := &serviceCatalogPage{portalBusinessWhats: "5527998279185", portalData: &domain.CustomerPortalData{Appliances: []map[string]any{{
		"id": "appliance-1", "marca": "LG", "ambiente": "Sala", "ultima_manutencao": time.Now().AddDate(0, -6, 0).Format("2006-01-02"),
	}}}}
	markup := app.HTMLString(page.customerPortalSection())
	for _, expected := range []string{"portal-maintenance-alert", "Hora da manutenção preventiva!", "Agendar Agora", "O ciclo de limpeza de ar do seu LG", "Seus equipamentos monitorados pela Inovar", "Adicionar Aparelho", "WhatsApp Inovar", "https://wa.me/5527998279185"} {
		if !strings.Contains(markup, expected) {
			t.Errorf("portal reminder markup missing %q: %s", expected, markup)
		}
	}
}

func TestCustomerPortalServiceOptionsMatchLegacyDefaultAndLabels(t *testing.T) {
	want := []customerPortalServiceOption{
		{Value: "LIMPEZA", Label: "Limpeza de Ar"},
		{Value: "MANUTENCAO_PREVENTIVA", Label: "Manutenção Preventiva"},
		{Value: "MANUTENCAO_CORRETIVA", Label: "Conserto / Reparo (Não está gelando / Barulho)"},
		{Value: "RECARGA_GAS", Label: "Carga ou Teste de Gás Refrigerante"},
		{Value: "INSTALACAO", Label: "Instalação / Desinstalação"},
		{Value: "AVALIACAO", Label: "Visita Técnica de Avaliação"},
	}
	got := customerPortalServiceOptions()
	if len(got) != len(want) {
		t.Fatalf("service option count=%d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("service option[%d]=%#v want %#v", i, got[i], want[i])
		}
	}
}

func TestCustomerServiceRequestPayloadPreservesSeparateProblemAndNotes(t *testing.T) {
	got := customerServiceRequestPayload(" LIMPEZA ", "appliance-1", "2026-10-10", "  Vazando água  ", "  ligar antes  ")
	want := map[string]any{"tipo": "LIMPEZA", "aparelho_id": "appliance-1", "data_agendamento": "2026-10-10", "problema": "Vazando água", "observacoes": "ligar antes"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("payload=%#v want %#v", got, want)
	}
	empty := customerServiceRequestPayload("LIMPEZA", "", "", "", "")
	if len(empty) != 1 || empty["tipo"] != "LIMPEZA" {
		t.Fatalf("empty optional fields should be omitted: %#v", empty)
	}
}

func TestSelectedCustomerApplianceKeepsValidChoiceAndFallsBackOnlyWhenEmpty(t *testing.T) {
	apps := []map[string]any{{"id": "first"}, {"id": "second"}}
	for current, want := range map[string]string{"": "first", "second": "second", "removed": ""} {
		if got := selectedCustomerAppliance(current, apps); got != want {
			t.Errorf("selectedCustomerAppliance(%q)=%q want %q", current, got, want)
		}
	}
}

func TestCustomerPortalServiceDateFormatsTimestampInBrowserLocalZone(t *testing.T) {
	brt := time.FixedZone("BRT", -3*60*60)
	got := portalServiceDateInLocation("2026-10-05T02:30:00Z", brt)
	if got != "04/10/2026" {
		t.Fatalf("timestamp rendered as %q, want local date 04/10/2026", got)
	}
	if got := portalServiceDateInLocation("2026-10-05", brt); got != "05/10/2026" {
		t.Fatalf("date-only value shifted across timezone: %q", got)
	}
	if got := portalServiceDateInLocation("not-a-date", brt); got != "" {
		t.Fatalf("invalid date rendered as %q", got)
	}
}

func TestCustomerServiceCardDateUsesLocalZoneForScheduledISOAndPreservesCivilDates(t *testing.T) {
	brt := time.FixedZone("BRT", -3*60*60)
	if got, want := customerServiceCardDateInLocation("2026-10-05T02:30:00Z", "2026-10-03T15:00:00Z", brt), "Data marcada: 04/10/2026"; got != want {
		t.Errorf("scheduled timestamp = %q want %q", got, want)
	}
	if got, want := customerServiceCardDateInLocation("2026-10-05", nil, brt), "Data marcada: 05/10/2026"; got != want {
		t.Errorf("scheduled civil date = %q want %q", got, want)
	}
	if got, want := customerServiceCardDateInLocation(nil, "2026-10-05T02:30:00Z", brt), "Solicitado em: 04/10/2026"; got != want {
		t.Errorf("request timestamp = %q want %q", got, want)
	}
}

func TestCustomerPortalServiceTypeLabelReplacesOnlyFirstUnderscore(t *testing.T) {
	for input, want := range map[string]string{
		"MANUTENCAO_PREVENTIVA": "MANUTENCAO PREVENTIVA",
		"TIPO_INTERNO_EXTRA":    "TIPO INTERNO_EXTRA",
		"LIMPEZA":               "LIMPEZA",
	} {
		if got := customerPortalServiceTypeLabel(input); got != want {
			t.Errorf("customerPortalServiceTypeLabel(%q)=%q want %q", input, got, want)
		}
	}
}

func TestCustomerPortalServiceAmountMatchesLegacyToFixed(t *testing.T) {
	for _, test := range []struct {
		value any
		want  string
		show  bool
	}{
		{1234.5, "1234.50", true}, {"1234.5", "1234.50", true}, {0.0, "", false}, {"", "", false}, {"invalid", "NaN", true}, {"0", "0.00", true},
	} {
		got, show := portalLegacyServiceAmount(test.value)
		if got != test.want || show != test.show {
			t.Errorf("portalLegacyServiceAmount(%#v)=(%q,%v) want (%q,%v)", test.value, got, show, test.want, test.show)
		}
	}
}

func TestCustomerProfilePhotoURLCacheBustingPreservesSignedQuery(t *testing.T) {
	if got := cacheBustCustomerProfilePhoto(""); got != "" {
		t.Fatalf("empty photo URL = %q", got)
	}
	got := cacheBustCustomerProfilePhoto("https://storage.example/photo.jpg?token=signed")
	if !strings.HasPrefix(got, "https://storage.example/photo.jpg?token=signed&v=") {
		t.Fatalf("cache-busted signed URL = %q", got)
	}
}

func TestCustomerProfileCEPFieldMatchesLegacyLoadingAndInputContract(t *testing.T) {
	page := &serviceCatalogPage{portalProfileForm: customerProfileForm{PostalCode: "29060-270"}, portalProfileCEPLooking: true}
	markup := app.HTMLString(customerProfilePostalCodeField(page))
	for _, expected := range []string{`inputmode="numeric"`, `value="29060-270"`, `aria-label="Buscando CEP"`} {
		if !strings.Contains(markup, expected) {
			t.Errorf("profile CEP field missing %q: %s", expected, markup)
		}
	}
	if got := customerProfileCEPLoading(false); len(got) != 0 {
		t.Fatalf("idle CEP field should not show spinner: %#v", got)
	}
}

func TestCustomerProfileFormRetainsLegacyGroupedAddressFields(t *testing.T) {
	page := &serviceCatalogPage{portalProfileOpen: true}
	markup := app.HTMLString(page.customerProfileDialog())
	for _, expected := range []string{"WhatsApp", "CEP (preenche o endereço automático)", "Endereço", "Número", "Bairro", "Cidade", "Salvar Dados"} {
		if !strings.Contains(markup, expected) {
			t.Errorf("profile form missing %q: %s", expected, markup)
		}
	}
}

func TestCustomerPasswordFeedbackAppearsOnPortalAfterDialogCloses(t *testing.T) {
	page := &serviceCatalogPage{portalPasswordNotice: "Senha alterada com sucesso! Use-a no próximo login.", portalData: &domain.CustomerPortalData{}}
	markup := app.HTMLString(page.customerPortalSection())
	if !strings.Contains(markup, "Senha alterada com sucesso!") {
		t.Fatalf("password success message should remain visible outside the closed dialog: %s", markup)
	}
}
