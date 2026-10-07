package webapp

import (
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

func TestTeamServiceLifecycleDateUsesStatusSpecificDate(t *testing.T) {
	cases := []struct {
		status string
		row    map[string]any
		want   string
	}{
		{"CONCLUIDO", map[string]any{"data_conclusao": "2026-10-04", "data_agendamento": "2026-10-02"}, "2026-10-04"},
		{"CANCELADO", map[string]any{"data_cancelamento": "2026-10-05", "data_agendamento": "2026-10-02"}, "2026-10-05"},
		{"EM_ANDAMENTO", map[string]any{"data_inicio": "2026-10-03", "data_agendamento": "2026-10-02"}, "2026-10-03"},
		{"AGENDADO", map[string]any{"data_agendamento": "2026-10-02"}, "2026-10-02"},
	}
	for _, tc := range cases {
		tc.row["status"] = tc.status
		if got := teamServiceLifecycleDate(tc.row); got != tc.want {
			t.Errorf("status %s: got %q want %q", tc.status, got, tc.want)
		}
	}
}

func TestTeamServiceDetailHidesStructuredObservationMarkers(t *testing.T) {
	observations := `Texto que o cliente informou. [GARANTIA_DIAS:90] [PROXIMO_RETORNO:2027-04-03] [CHECKLIST:{"filtrosLavados":true}] [MAO_OBRA:200]`
	got := strings.TrimSpace(teamServiceInternalMarkers.ReplaceAllString(observations, ""))
	if got != "Texto que o cliente informou." || strings.Contains(got, "[") {
		t.Fatalf("visible notes or internal markers changed unexpectedly: %q", got)
	}
}

func TestTeamServiceChecklistRowsPreserveRecordedFacts(t *testing.T) {
	cases := []struct {
		name string
		row  map[string]any
		want []teamServiceChecklistRow
	}{
		{"installation", map[string]any{"tipo": "INSTALACAO", "observacoes": `[CHECKLIST:{"vacuoMicrons":"420","testeNitrogenio":true,"saltoTermicoDeltaT":"9°C","correnteAmperes":"4.2 A"}]`}, []teamServiceChecklistRow{{"Corrente de operação", "4.2 A"}, {"Salto térmico (ΔT)", "9°C"}, {"Vácuo atingido", "420"}, {"Teste com nitrogênio", "Sim"}}},
		{"corrective", map[string]any{"tipo": "MANUTENCAO_CORRETIVA", "pecas_utilizadas": "Capacitor", "observacoes": `[CHECKLIST:{"diagnosticoTecnico":"Falha elétrica","pecasSubstituidas":"Relé","capacitorTestado":"35 µF"}]`}, []teamServiceChecklistRow{{"Capacitor testado", "35 µF"}, {"Peças substituídas", "Relé"}, {"Diagnóstico técnico", "Falha elétrica"}}},
		{"gas", map[string]any{"tipo": "RECARGA_GAS", "observacoes": `[CHECKLIST:{"gasAdicionadoGramas":"300 g","pressaoGasPSI":"120","testeNitrogenio":false,"saltoTermicoDeltaT":"8°C"}]`}, []teamServiceChecklistRow{{"Pressão do gás", "120"}, {"Salto térmico (ΔT)", "8°C"}, {"Teste com nitrogênio", "Não"}, {"Gás adicionado", "300 g"}}},
		{"cleaning", map[string]any{"tipo": "LIMPEZA", "observacoes": `[CHECKLIST:{"filtrosLavados":true,"turbinaLimpa":false,"saltoTermicoDeltaT":"10°C"}]`}, []teamServiceChecklistRow{{"Filtros lavados", "Sim"}, {"Turbina limpa", "Não"}, {"Salto térmico (ΔT)", "10°C"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := teamServiceChecklistRows(tc.row)
			if len(got) != len(tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("row %d: got %#v, want %#v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestTeamServiceChecklistRowsIgnoreMissingOrInvalidMarker(t *testing.T) {
	for _, observations := range []string{"sem checklist", `[CHECKLIST:{"x":true]`, `[CHECKLIST:{}]`} {
		if got := teamServiceChecklistRows(map[string]any{"tipo": "LIMPEZA", "observacoes": observations}); len(got) != 0 {
			t.Errorf("observations %q returned rows: %#v", observations, got)
		}
	}
}

func TestTeamServiceFinancialMarkersParseDecimalAmounts(t *testing.T) {
	observations := `[MAO_OBRA:199.90] [PECAS:35.5]`
	if got := markerMoney(teamServiceLaborMarker, observations); got != 199.90 {
		t.Fatalf("labor amount = %v, want 199.90", got)
	}
	if got := markerMoney(teamServicePartsMarker, observations); got != 35.5 {
		t.Fatalf("parts amount = %v, want 35.5", got)
	}
	if got := markerMoney(teamServicePartsMarker, "sem marcador"); got != 0 {
		t.Fatalf("missing marker amount = %v, want 0", got)
	}
}

func TestServiceOrderRecommendedReturnMatchesReactWarrantyFallback(t *testing.T) {
	base := time.Date(2026, time.January, 31, 0, 0, 0, 0, time.Local)
	if got := teamServiceRecommendedReturn(base, "", 90); got != "2026-05-01" {
		t.Fatalf("fallback return = %q, want warranty expiry 2026-05-01", got)
	}
	if got := teamServiceRecommendedReturn(base, "2026-06-12T00:00:00Z", 90); got != "2026-06-12" {
		t.Fatalf("recorded return = %q, want 2026-06-12", got)
	}
	if got := teamServiceRecommendedReturn(base, "2026-02-30", 90); got != "2026-05-01" {
		t.Fatalf("invalid return fallback = %q, want 2026-05-01", got)
	}
}

func TestServiceOrderMoneyMatchesLegacyFixedDecimals(t *testing.T) {
	for value, want := range map[float64]string{1234.5: "1234.50", 0: "0.00", 19.999: "20.00"} {
		if got := serviceOrderMoney(value); got != want {
			t.Errorf("serviceOrderMoney(%v) = %q, want %q", value, got, want)
		}
	}
}

func TestServiceOrderFinancialSummaryDoesNotInventPaymentMethod(t *testing.T) {
	markup := app.HTMLString(app.Div().Body(teamServiceFinancialBreakdown(map[string]any{"valor": 1234.5}, "", "")...))
	if strings.Contains(markup, "PIX") {
		t.Fatalf("financial summary invented a payment method: %s", markup)
	}
	if !strings.Contains(markup, "R$ 1234.50") {
		t.Fatalf("financial summary diverged from legacy decimal formatting: %s", markup)
	}
}

func TestTeamServiceLifecycleMarkersAreRecognized(t *testing.T) {
	observations := `[DATA_INICIO:2026-10-01] [DATA_CONCLUSAO:2026-10-02] [DATA_CANCELAMENTO:2026-10-03] [MOTIVO_CANCELAMENTO:Cliente solicitou]`
	markers := []struct {
		pattern *regexp.Regexp
		want    string
	}{
		{teamServiceStartMarker, "2026-10-01"},
		{teamServiceDoneMarker, "2026-10-02"},
		{teamServiceCancelDateMarker, "2026-10-03"},
		{teamServiceCancelReasonMarker, "Cliente solicitou"},
	}
	for _, marker := range markers {
		if got := markerValue(marker.pattern, observations); got != marker.want {
			t.Errorf("marker %s = %q, want %q", marker.pattern, got, marker.want)
		}
	}
}

func TestTeamServiceWarrantyStateUsesCivilDaysAndInclusiveExpiry(t *testing.T) {
	location := time.FixedZone("America/Sao_Paulo", -3*60*60)
	now := time.Date(2026, 10, 4, 23, 30, 0, 0, location)
	active, remaining, expiry, ok := teamServiceWarrantyState("2026-10-01", 3, now)
	if !ok || !active || remaining != 0 || expiry != "2026-10-04" {
		t.Fatalf("expiry day state = (%t, %d, %q, %t), want active today with 0 days", active, remaining, expiry, ok)
	}
	active, remaining, _, ok = teamServiceWarrantyState("2026-10-01", 3, now.AddDate(0, 0, 1))
	if !ok || active || remaining != -1 {
		t.Fatalf("day after expiry = (%t, %d, %t), want expired by one day", active, remaining, ok)
	}
	if _, _, _, ok := teamServiceWarrantyState("bad-date", 90, now); ok {
		t.Fatal("invalid service date must not produce a warranty state")
	}
}

func TestTeamServiceSummaryMatchesLegacyLabelsAndValues(t *testing.T) {
	if got := teamServiceApplianceSummary(map[string]any{"marca": "LG", "modelo": "Dual Inverter", "btus": float64(12000), "ambiente": "Sala"}); got != "LG 12000 BTUs (Sala)" {
		t.Fatalf("appliance summary = %q", got)
	}
	if got := teamServiceDisplayName(map[string]any{"tipo": "INSTALACAO", "descricao": "texto operacional"}); got != "Instalação" {
		t.Fatalf("mapped service name = %q", got)
	}
	if got := teamServiceDisplayName(map[string]any{"tipo": "OUTRO", "descricao": "Reparo Especial"}); got != "Reparo Especial" {
		t.Fatalf("custom service name = %q", got)
	}
	for status, want := range map[string]string{"CONCLUIDO": "Data de Conclusão", "CANCELADO": "Data de Cancelamento", "EM_ANDAMENTO": "Data Agendada"} {
		if got := teamServiceSummaryDateLabel(status); got != want {
			t.Errorf("summary date label for %s = %q, want %q", status, got, want)
		}
	}
}

func TestServiceOrderWhatsAppFallbackURLContainsPDFAndPortugueseText(t *testing.T) {
	href := serviceOrderWhatsAppFallbackURL("(27) 99999-1234", "Olá *Ana*! Comprovante", "https://files.example.test/os.pdf?token=a&b=c")
	parsed, err := url.Parse(href)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Host != "wa.me" || parsed.Path != "/5527999991234" {
		t.Fatalf("unexpected WhatsApp address: %s", href)
	}
	message := parsed.Query().Get("text")
	if !strings.Contains(message, "Olá *Ana*! Comprovante") || !strings.Contains(message, "📄 *OS em PDF:* https://files.example.test/os.pdf?token=a&b=c") {
		t.Fatalf("fallback message lost text or PDF link: %q", message)
	}
	if got := serviceOrderWhatsAppFallbackURL("", "text", "https://files.example.test/os.pdf"); got != "" {
		t.Fatalf("fallback URL without customer phone = %q, want empty", got)
	}
}

func TestDeleteOSConfirmationMatchesLegacyTextAndKeepsOtherServicePrompt(t *testing.T) {
	service := map[string]any{"id": "os-123"}
	if got := teamCustomerActionMessage(service, teamDeleteServiceAction+"os-123"); got != "Deseja realmente excluir esta Ordem de Serviço do histórico?" {
		t.Fatalf("OS detail confirmation = %q", got)
	}
	if got := teamCustomerActionMessage(nil, teamDeleteServiceAction+"os-123"); got != "Excluir este serviço permanentemente do banco? Esta ação não pode ser desfeita." {
		t.Fatalf("generic service confirmation changed: %q", got)
	}
}

func TestDeleteServiceConfirmationOffersExplicitYesAndNo(t *testing.T) {
	p := &serviceCatalogPage{}
	markup := app.HTMLString(teamCustomerActionDialog(p, teamDeleteServiceAction+"os-123"))
	for _, want := range []string{"team-confirm-dialog", "team-confirm-dialog__message", "team-confirm-dialog__actions", "Confirmar exclusão", "Não, manter", "Sim, excluir", "não pode ser desfeita"} {
		if !strings.Contains(markup, want) {
			t.Errorf("delete confirmation is missing %q: %s", want, markup)
		}
	}
}

func TestCompletedServiceScheduleDialogExplainsReopenAction(t *testing.T) {
	p := &serviceCatalogPage{
		teamScheduleServiceID: "os-123",
		teamScheduleDate:      "2026-10-06",
		teamServices: []map[string]any{{
			"id": "os-123", "status": "CONCLUIDO", "descricao": "Limpeza", "customers": map[string]any{"nome": "Ana"},
		}},
	}
	markup := app.HTMLString(p.teamScheduleDialog())
	for _, want := range []string{"Reabrir e reagendar", "Nova data", "Sim, reabrir e reagendar", "Agendada"} {
		if !strings.Contains(markup, want) {
			t.Errorf("reopen dialog is missing %q: %s", want, markup)
		}
	}
}

func TestServiceDetailRendersDeleteFailureAsRestoredRecordNotice(t *testing.T) {
	p := &serviceCatalogPage{teamServiceDetail: map[string]any{"id": "os-123"}, teamServiceDeleteNotice: "Não foi possível excluir do banco — item restaurado."}
	ui := p.teamServiceDetailDialog()
	markup := app.HTMLString(ui)
	if !strings.Contains(markup, "item restaurado") {
		t.Fatalf("OS detail does not show the restored-record notice: %s", markup)
	}
}

func TestServiceDetailUsesTechnicalLayoutAndWarrantyFallback(t *testing.T) {
	service := map[string]any{
		"id": "os-12345678", "tipo": "INSTALACAO", "status": "CONCLUIDO", "data_conclusao": "2026-01-31",
		"valor": float64(1234.5), "observacoes": `[GARANTIA_DIAS:90] [CHECKLIST:{"vacuoMicrons":"420","testeNitrogenio":true}]`,
		"customers": map[string]any{"nome": "Ana"}, "air_conditioners": map[string]any{"marca": "LG", "btus": float64(12000), "ambiente": "Sala"},
	}
	p := &serviceCatalogPage{teamServiceDetail: service}
	markup := app.HTMLString(p.teamServiceDetailDialog())
	for _, expected := range []string{"team-service-technical", "Especificações Técnicas Registradas:", "Vácuo atingido", "420", "Próximo Retorno Recomendado", "01/05/2026", "R$ 1234.50"} {
		if !strings.Contains(markup, expected) {
			t.Errorf("service details are missing %q: %s", expected, markup)
		}
	}
}

func TestServicePhotoPanelAllowsMoreThanSixPhotos(t *testing.T) {
	photos := make([]appliancePhoto, 7)
	for i := range photos {
		photos[i] = appliancePhoto{Name: "foto", Path: "fotos-os/os/foto", URL: "https://example.test/foto"}
	}
	p := &serviceCatalogPage{teamServicePhotos: photos, teamServicePhotoID: "os-1"}
	markup := app.HTMLString(p.teamServicePhotosPanel())
	if strings.Contains(markup, `disabled="true"`) || strings.Contains(markup, "até 6") || !strings.Contains(markup, "7 fotos") {
		t.Fatalf("photo panel still limits attachments at six: %s", markup)
	}
}
