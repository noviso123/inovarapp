package webapp

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/domain"
)

var (
	teamServiceInternalMarkers    = regexp.MustCompile(`\[(?:GARANTIA_DIAS|DATA_CONCLUSAO|DATA_INICIO|DATA_CANCELAMENTO|MOTIVO_CANCELAMENTO|PROXIMO_RETORNO|MAO_OBRA|PECAS):[^\]]+\]|\[CHECKLIST:\{(?s:.*?)\}\]`)
	teamServiceChecklistMarker    = regexp.MustCompile(`\[CHECKLIST:(\{(?s:.*?)\})\]`)
	teamServicePaymentMarker      = regexp.MustCompile(`Pagamento: ([^.]+)\.`)
	teamServiceWarrantyMarker     = regexp.MustCompile(`\[GARANTIA_DIAS:(\d+)\]`)
	teamServiceReturnMarker       = regexp.MustCompile(`\[PROXIMO_RETORNO:([^\]]+)\]`)
	teamServiceLaborMarker        = regexp.MustCompile(`\[MAO_OBRA:(\d+(?:\.\d+)?)\]`)
	teamServicePartsMarker        = regexp.MustCompile(`\[PECAS:(\d+(?:\.\d+)?)\]`)
	teamServiceStartMarker        = regexp.MustCompile(`\[DATA_INICIO:([^\]]+)\]`)
	teamServiceDoneMarker         = regexp.MustCompile(`\[DATA_CONCLUSAO:([^\]]+)\]`)
	teamServiceCancelDateMarker   = regexp.MustCompile(`\[DATA_CANCELAMENTO:([^\]]+)\]`)
	teamServiceCancelReasonMarker = regexp.MustCompile(`\[MOTIVO_CANCELAMENTO:([^\]]+)\]`)
)

func (p *serviceCatalogPage) openTeamServiceDetail(service map[string]any) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		if service == nil || portalText(service["id"]) == "" {
			return
		}
		p.showTeamServiceDetail(ctx, service)
	}
}

func (p *serviceCatalogPage) showTeamServiceDetail(ctx app.Context, service map[string]any) {
	if service == nil || portalText(service["id"]) == "" {
		return
	}
	p.teamServiceDetail = service
	p.teamServiceReceiptEdit = false
	p.teamServiceDeleteNotice = ""
	p.teamPDFNotice = ""
	p.teamServicePhotoID = portalText(service["id"])
	p.teamServicePhotos = nil
	p.teamServiceWhatsAppURL = ""
	p.teamServicePhotoNotice = "Carregando fotos da OS..."
	p.loadTeamServicePhotos(ctx, p.teamServicePhotoID)
	ctx.Update()
}

func (p *serviceCatalogPage) closeTeamServiceDetail(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if !p.teamPDFBusy {
		p.teamServiceDetail = nil
		p.teamPDFNotice = ""
		p.teamServicePhotoID = ""
		p.teamServicePhotos = nil
		p.teamServiceWhatsAppURL = ""
	}
	ctx.Update()
}

func (p *serviceCatalogPage) teamServiceDetailDialog() app.UI {
	service := p.teamServiceDetail
	if service == nil {
		return app.Div()
	}
	customer, _ := service["customers"].(map[string]any)
	appliance, _ := service["air_conditioners"].(map[string]any)
	id := strings.ToUpper(portalText(service["id"]))
	if len(id) > 8 {
		id = id[:8]
	}
	serviceStatus := strings.ToUpper(portalText(service["status"]))
	date := teamServiceLifecycleDate(service)
	if date == "" {
		date = portalText(service["data_agendamento"])
	}
	serviceName := teamServiceDisplayName(service)
	applianceName := teamServiceApplianceSummary(appliance)
	observations := portalText(service["observacoes"])
	humanNotes := formatTeamHistoryDisplay(observations).Notes
	checklistRows := teamServiceChecklistRows(service)
	payment := ""
	if match := teamServicePaymentMarker.FindStringSubmatch(observations); len(match) == 2 {
		payment = match[1]
	}
	warranty := p.teamProfile.DefaultWarrantyDays
	if warranty == 0 {
		warranty = 90
	}
	if match := teamServiceWarrantyMarker.FindStringSubmatch(observations); len(match) == 2 {
		if parsed, err := strconv.Atoi(match[1]); err == nil {
			if parsed > 0 {
				warranty = parsed
			}
		}
	}
	returnDate := markerValue(teamServiceReturnMarker, observations)
	baseDate := date
	if baseDate == "" {
		baseDate = time.Now().Format("2006-01-02")
	}
	base, baseErr := time.ParseInLocation("2006-01-02", baseDate[:min(len(baseDate), 10)], time.Local)
	if baseErr == nil {
		returnDate = teamServiceRecommendedReturn(base, returnDate, warranty)
	}
	fields := []struct{ label, value string }{
		{"Cliente", firstNonEmptyBudget(portalText(customer["nome"]), "Cliente")},
		{"Aparelho", applianceName},
		{"Serviço Realizado", serviceName},
		{teamServiceSummaryDateLabel(serviceStatus), portalDate(date)},
	}
	rows := make([]app.UI, 0, len(fields)+5)
	for _, field := range fields {
		if field.value == "" {
			continue
		}
		rows = append(rows, app.Div().Class("portal-service__heading team-service-detail__field").Body(app.Strong().Body(app.Text(field.label)), app.Span().Body(app.Text(field.value))))
	}
	if problem := portalText(service["problema"]); problem != "" {
		rows = append(rows, app.Div().Class("team-service-detail__notes").Body(app.Strong().Body(app.Text("Problema relatado")), app.P().Body(app.Text(problem))))
	}
	rows = append(rows, teamServiceFinancialBreakdown(service, observations, payment)...)
	rows = append(rows, teamServiceLifecyclePanel(service, teamServiceLifecycleStatus(serviceStatus), observations))
	if len(checklistRows) > 0 {
		technicalRows := make([]app.UI, 0, len(checklistRows))
		for _, item := range checklistRows {
			technicalRows = append(technicalRows, app.Div().Class("portal-service__heading team-service-detail__field").Body(
				app.Strong().Body(app.Text(item.Label)), app.Span().Body(app.Text(item.Value)),
			))
		}
		rows = append(rows, app.Section().Class("team-service-technical team-service-detail__panel").Body(
			app.Strong().Body(app.Text("Especificações Técnicas Registradas:")),
			app.Div().Class("team-service-technical__grid").Body(technicalRows...),
		))
	}
	if humanNotes != "" {
		rows = append(rows, app.Div().Class("team-service-detail__notes").Body(app.Strong().Body(app.Text("Observações e checklist")), app.P().Body(app.Text(humanNotes))))
	}
	if payment != "" {
		rows = append(rows, app.Div().Class("portal-service__heading team-service-detail__field").Body(app.Strong().Body(app.Text("Forma de pagamento")), app.Span().Body(app.Text(payment))))
	}
	if baseErr == nil {
		active, remaining, expiry, ok := teamServiceWarrantyState(baseDate, warranty, time.Now())
		if ok {
			label := "Garantia Expirada"
			message := "Expirou em " + portalDate(expiry)
			if active {
				label = "Garantia Inovar Ativa"
				message = "Válida por mais " + strconv.Itoa(remaining) + " dias (até " + portalDate(expiry) + ")"
			}
			rows = append(rows, app.Section().Class("portal-service team-service-detail__warranty").Body(
				app.Strong().Body(app.Text(label)), app.P().Body(app.Text(message)),
			))
		}
	}
	if returnDate != "" {
		rows = append(rows, app.Div().Class("portal-service__heading team-service-detail__field").Body(app.Strong().Body(app.Text("Próximo Retorno Recomendado")), app.Span().Body(app.Text(portalDate(returnDate)))))
	}
	serviceID := portalText(service["id"])
	actions := []app.UI{
		app.Button().Class("auth-submit team-service-detail-actions__primary").Type("button").Disabled(p.teamPDFBusy).OnClick(p.downloadTeamServicePDF(false)).Body(app.Text("Imprimir / baixar comprovante da OS")),
		app.Button().Class("auth-link team-service-detail-actions__whatsapp").Type("button").Disabled(p.teamPDFBusy).OnClick(p.downloadTeamServicePDF(true)).Body(app.Text("Enviar comprovante pelo WhatsApp")),
	}
	for _, budget := range p.teamBudgets {
		if budget.ServiceID != nil && *budget.ServiceID == serviceID {
			actions = append(actions, app.Button().Class("auth-link").Type("button").Disabled(p.teamBudgetBusy).OnClick(p.downloadTeamBudgetPDF(budget)).Body(app.Text("Reimprimir orçamento (PDF)")))
			break
		}
	}
	actions = append(actions,
		app.Button().Class("auth-link").Type("button").OnClick(func(ctx app.Context, event app.Event) {
			event.PreventDefault()
			p.openTeamServiceEdit(service)(ctx, event)
			p.teamServiceReceiptEdit = true
			ctx.Update()
		}).Body(app.Text("Editar ordem de serviço")),
		app.Button().Class("auth-link team-service-detail-actions__danger").Type("button").OnClick(func(ctx app.Context, event app.Event) {
			event.PreventDefault()
			p.askTeamCustomerAction(teamDeleteServiceAction+serviceID)(ctx, event)
		}).Body(app.Text("Excluir ordem de serviço")),
	)
	if applianceID := portalText(appliance["id"]); applianceID != "" {
		actions = append(actions, app.Button().Class("auth-link").Type("button").OnClick(func(ctx app.Context, event app.Event) {
			p.teamServiceDetail = nil
			p.openTeamHistoryForm(applianceID)(ctx, event)
			ctx.Update()
		}).Body(app.Text("Registrar OS anterior")))
	}
	servicePhotos := p.teamServicePhotosPanel()
	return app.Div().Class("auth-backdrop team-service-detail-backdrop").Body(app.Div().Class("auth-dialog team-service-detail-dialog").Body(
		app.Div().Class("auth-dialog__heading").Body(app.Div().Body(app.P().Class("catalog__eyebrow").Body(app.Text("INOVAR REFRIGERAÇÃO • COMPROVANTE & GARANTIA")), app.H2().Class("auth-dialog__title").Body(app.Text("Ordem de Serviço #"+id))), app.Button().Class("auth-close").Type("button").OnClick(p.closeTeamServiceDetail).Body(app.Text("Fechar"))),
		app.Div().Class("service-grid").Body(rows...),
		formErrorNotice(p.teamServiceDeleteNotice),
		p.teamServiceReceiptEditPanel(service),
		servicePhotos,
		app.P().Class("portal-section__intro").Body(app.Text(p.teamPDFNotice)),
		teamServiceWhatsAppFallbackLink(p.teamServiceWhatsAppURL),
		app.Div().Class("portal-budget__actions team-service-detail-actions").Body(actions...),
	))
}

func teamServiceWarrantyState(serviceDate string, warrantyDays int, now time.Time) (active bool, daysRemaining int, expiry string, ok bool) {
	if len(serviceDate) < 10 {
		return false, 0, "", false
	}
	base, err := time.ParseInLocation("2006-01-02", serviceDate[:10], now.Location())
	if err != nil {
		return false, 0, "", false
	}
	expires := base.AddDate(0, 0, warrantyDays)
	dayNumber := func(value time.Time) int64 {
		year, month, day := value.Date()
		return time.Date(year, month, day, 0, 0, 0, 0, time.UTC).Unix() / 86400
	}
	daysRemaining = int(dayNumber(expires) - dayNumber(now))
	return daysRemaining >= 0, daysRemaining, expires.Format("2006-01-02"), true
}

func teamServiceRecommendedReturn(serviceDate time.Time, recordedReturn string, warrantyDays int) string {
	if len(recordedReturn) >= 10 {
		if parsed, err := time.ParseInLocation("2006-01-02", recordedReturn[:10], serviceDate.Location()); err == nil {
			return parsed.Format("2006-01-02")
		}
	}
	return serviceDate.AddDate(0, 0, warrantyDays).Format("2006-01-02")
}

func teamServiceFinancialBreakdown(service map[string]any, observations, payment string) []app.UI {
	total := agendaNumber(service["valor"])
	parts, labor := markerMoney(teamServicePartsMarker, observations), markerMoney(teamServiceLaborMarker, observations)
	if labor == 0 {
		if parts != 0 {
			labor = total - parts
		} else {
			labor = total
		}
	}
	items := []app.UI{
		app.Div().Class("portal-service__heading").Body(app.Strong().Body(app.Text("Mão de Obra Técnica:")), app.Span().Body(app.Text("R$ "+serviceOrderMoney(labor)))),
	}
	if parts > 0 {
		label := "Peças / Materiais"
		if used := portalText(service["pecas_utilizadas"]); used != "" {
			label += " (" + used + ")"
		}
		items = append(items, app.Div().Class("portal-service__heading").Body(app.Strong().Body(app.Text(label+":")), app.Span().Body(app.Text("R$ "+serviceOrderMoney(parts)))))
	}
	totalText := "R$ " + serviceOrderMoney(total)
	method := firstNonEmptyBudget(payment, portalText(service["forma_pagamento"]))
	if method != "" {
		totalText += " • " + method
	}
	items = append(items, app.Div().Class("portal-service__heading").Body(app.Strong().Body(app.Text("VALOR TOTAL:")), app.Span().Body(app.Text(totalText))))
	return []app.UI{app.Section().Class("team-service-technical team-service-detail__panel").Body(app.Div().Class("portal-service__heading").Body(app.Strong().Body(app.Text("Discriminação de Valores"))), app.Div().Class("team-service-technical__grid").Body(items...))}
}

func serviceOrderMoney(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func teamServiceApplianceSummary(appliance map[string]any) string {
	brand := portalText(appliance["marca"])
	capacity := teamServiceCapacityText(appliance["btus"])
	room := portalText(appliance["ambiente"])
	if brand == "" && capacity == "" && room == "" {
		return "Aparelho não informado"
	}
	var summary strings.Builder
	summary.WriteString(brand)
	if capacity != "" {
		if summary.Len() > 0 {
			summary.WriteByte(' ')
		}
		summary.WriteString(capacity + " BTUs")
	}
	if room != "" {
		if summary.Len() > 0 {
			summary.WriteByte(' ')
		}
		summary.WriteString("(" + room + ")")
	}
	return summary.String()
}

func teamServiceCapacityText(value any) string {
	switch number := value.(type) {
	case float64:
		return strconv.FormatFloat(number, 'f', -1, 64)
	case int:
		return strconv.Itoa(number)
	case int64:
		return strconv.FormatInt(number, 10)
	case string:
		return strings.TrimSpace(number)
	default:
		return ""
	}
}

func teamServiceDisplayName(service map[string]any) string {
	typeName := portalText(service["tipo"])
	if typeName == "OUTRO" && portalText(service["descricao"]) != "" {
		return domain.NormalizeServiceName(portalText(service["descricao"]))
	}
	return string(supabase.MapSupabaseServiceTypeToLocal(typeName))
}

func teamServiceSummaryDateLabel(status string) string {
	switch status {
	case "CONCLUIDO":
		return "Data de Conclusão"
	case "CANCELADO":
		return "Data de Cancelamento"
	default:
		return "Data Agendada"
	}
}

func teamServiceLifecycleStatus(status string) string {
	switch status {
	case "CONCLUIDO":
		return "Concluído"
	case "EM_ANDAMENTO":
		return "Em Andamento"
	case "CANCELADO":
		return "Cancelado"
	case "AGENDADO":
		return "Agendado"
	default:
		return "Agendado"
	}
}

func markerMoney(marker *regexp.Regexp, observations string) float64 {
	match := marker.FindStringSubmatch(observations)
	if len(match) != 2 {
		return 0
	}
	value, _ := strconv.ParseFloat(match[1], 64)
	return value
}

func teamServiceLifecyclePanel(service map[string]any, status string, observations string) app.UI {
	start := firstNonEmptyBudget(portalText(service["data_inicio"]), markerValue(teamServiceStartMarker, observations))
	done := firstNonEmptyBudget(portalText(service["data_conclusao"]), markerValue(teamServiceDoneMarker, observations))
	if done == "" && strings.EqualFold(portalText(service["status"]), "CONCLUIDO") {
		done = firstNonEmptyBudget(portalText(service["data_agendamento"]), portalText(service["data_solicitacao"]))
	}
	cancelled := firstNonEmptyBudget(portalText(service["data_cancelamento"]), markerValue(teamServiceCancelDateMarker, observations))
	reason := firstNonEmptyBudget(portalText(service["motivo_cancelamento"]), markerValue(teamServiceCancelReasonMarker, observations))
	dateText := func(value string) string {
		if formatted := portalDate(value); formatted != "" {
			return formatted
		}
		return "—"
	}
	scheduled := dateText(portalText(service["data_agendamento"]))
	if hour := portalText(service["hora_agendamento"]); hour != "" {
		scheduled += " " + hour
	}
	items := []struct{ label, value string }{
		{"📅 Agendamento:", scheduled},
		{"▶️ Início:", dateText(start)},
		{"✅ Conclusão:", dateText(done)},
		{"✖ Cancelamento:", dateText(cancelled)},
	}
	cards := make([]app.UI, 0, len(items))
	for _, item := range items {
		cards = append(cards, app.Div().Class("portal-service__heading team-service-detail__field").Body(app.Strong().Body(app.Text(item.label)), app.Span().Body(app.Text(item.value))))
	}
	content := []app.UI{
		app.Div().Class("portal-service__heading").Body(app.Strong().Body(app.Text("Ciclo de Vida do Serviço")), app.Span().Body(app.Text(status))),
		app.Div().Class("team-service-detail__lifecycle-grid").Body(cards...),
	}
	if reason != "" {
		content = append(content, app.P().Body(app.Text("Motivo: "+reason)))
	}
	return app.Section().Class("team-service-technical team-service-detail__panel").Body(content...)
}

func markerValue(marker *regexp.Regexp, observations string) string {
	match := marker.FindStringSubmatch(observations)
	if len(match) == 2 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

type teamServiceChecklistRow struct {
	Label string
	Value string
}

func teamServiceChecklistRows(service map[string]any) []teamServiceChecklistRow {
	checklist := teamServiceChecklistData(portalText(service["observacoes"]))
	if len(checklist) == 0 {
		return nil
	}
	items := []struct{ key, label string }{
		{"filtrosLavados", "Filtros lavados"},
		{"serpentinaHigienizada", "Serpentina higienizada"},
		{"turbinaLimpa", "Turbina limpa"},
		{"drenoDesobstruido", "Dreno desobstruído"},
		{"bandejaSanitizada", "Bandeja sanitizada"},
		{"condensadoraLavada", "Condensadora lavada"},
		{"aplicacaoBactericida", "Bactericida aplicado"},
		{"testeEletricoCorrente", "Teste elétrico"},
		{"correnteAmperes", "Corrente de operação"},
		{"pressaoGasPSI", "Pressão do gás"},
		{"saltoTermicoDeltaT", "Salto térmico (ΔT)"},
		{"temperaturaInsuflamento", "Temperatura de insuflamento"},
		{"temperaturaRetorno", "Temperatura de retorno"},
		{"suporteNivelado", "Suporte nivelado"},
		{"vacuoMicrons", "Vácuo atingido"},
		{"testeNitrogenio", "Teste com nitrogênio"},
		{"valvulasLiberadas", "Válvulas liberadas"},
		{"superaquecimentoUtil", "Superaquecimento"},
		{"capacitorTestado", "Capacitor testado"},
		{"gasAdicionadoGramas", "Gás adicionado"},
		{"pecasSubstituidas", "Peças substituídas"},
		{"diagnosticoTecnico", "Diagnóstico técnico"},
	}
	rows := make([]teamServiceChecklistRow, 0, len(items))
	for _, item := range items {
		value, exists := teamServiceChecklistValue(checklist, item.key)
		if !exists || value == nil {
			continue
		}
		formatted := ""
		switch typed := value.(type) {
		case bool:
			if typed {
				formatted = "Sim"
			} else {
				formatted = "Não"
			}
		case string:
			formatted = strings.TrimSpace(typed)
		case float64:
			formatted = strconv.FormatFloat(typed, 'f', -1, 64)
		default:
			formatted = strings.TrimSpace(portalText(value))
		}
		if formatted != "" {
			rows = append(rows, teamServiceChecklistRow{Label: item.label, Value: formatted})
		}
	}
	return rows
}

func teamServiceChecklistData(observations string) map[string]any {
	observations = strings.TrimSpace(observations)
	if observations == "" {
		return nil
	}
	candidates := make([]string, 0, 3)
	if marker := teamServiceChecklistMarker.FindStringSubmatch(observations); len(marker) == 2 {
		candidates = append(candidates, strings.TrimSpace(marker[1]))
	}
	candidates = append(candidates, teamServiceJSONObjects(observations)...)
	candidates = append(candidates, observations)
	for _, part := range strings.Split(observations, "|") {
		candidates = append(candidates, strings.TrimSpace(part))
	}
	for _, candidate := range candidates {
		if candidate == "" || !strings.HasPrefix(candidate, "{") {
			continue
		}
		var checklist map[string]any
		if json.Unmarshal([]byte(candidate), &checklist) != nil || len(checklist) == 0 {
			continue
		}
		for key := range checklist {
			if knownTeamServiceChecklistKey(key) {
				return checklist
			}
		}
	}
	return nil
}

func teamServiceChecklistValue(checklist map[string]any, wanted string) (any, bool) {
	for key, value := range checklist {
		if normalizeTeamServiceChecklistKey(key) == normalizeTeamServiceChecklistKey(wanted) {
			return value, true
		}
	}
	// Older records used the label without the first capital in "vacuomicrons".
	if normalizeTeamServiceChecklistKey(wanted) == "vacuomicrons" {
		for key, value := range checklist {
			if normalizeTeamServiceChecklistKey(key) == "vacuomicrons" {
				return value, true
			}
		}
	}
	return nil, false
}

func normalizeTeamServiceChecklistKey(key string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, key)
}

func knownTeamServiceChecklistKey(key string) bool {
	switch normalizeTeamServiceChecklistKey(key) {
	case "filtroslavados", "serpentinahigienizada", "turbinalimpa", "drenodesobstruido",
		"bandejasanitizada", "condensadoralavada", "aplicacaobactericida", "testeeletricocorrente",
		"correnteamperes", "pressaogaspsi", "saltotermicodeltat", "temperaturainsuflamento",
		"temperaturaretorno", "suportenivelado", "vacuomicrons", "testenitrogenio", "valvulasliberadas",
		"superaquecimentoutil", "capacitortestado", "gasadicionadogramas", "pecassubstituidas", "diagnosticotecnico":
		return true
	default:
		return false
	}
}

func teamServiceJSONObjects(value string) []string {
	objects := make([]string, 0, 2)
	for cursor := 0; cursor < len(value); {
		relative := strings.IndexByte(value[cursor:], '{')
		if relative < 0 {
			break
		}
		start := cursor + relative
		depth, inString, escaped := 0, false, false
		found := false
		for index := start; index < len(value); index++ {
			char := value[index]
			if inString {
				if escaped {
					escaped = false
				} else if char == '\\' {
					escaped = true
				} else if char == '"' {
					inString = false
				}
				continue
			}
			switch char {
			case '"':
				inString = true
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					objects = append(objects, value[start:index+1])
					cursor, found = index+1, true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			cursor = start + 1
		}
	}
	return objects
}

func stripTeamServiceChecklistJSON(value string) string {
	for _, fragment := range teamServiceJSONObjects(value) {
		var payload map[string]any
		if json.Unmarshal([]byte(fragment), &payload) != nil {
			continue
		}
		recognized := false
		for key := range payload {
			if knownTeamServiceChecklistKey(key) {
				recognized = true
				break
			}
		}
		if recognized {
			value = strings.Replace(value, fragment, "", 1)
		}
	}
	return value
}

const teamServicePhotoDeleteAction = "delete-service-photo:"

func (p *serviceCatalogPage) loadTeamServicePhotos(ctx app.Context, serviceID string) {
	if p.session == nil || serviceID == "" {
		return
	}
	token := p.session.AccessToken
	go func() {
		result, err := sendTeamJSONResult(ctx, apiBaseURL()+"/api/documentos", token, http.MethodPost, map[string]any{"acao": "fotos", "serviceId": serviceID})
		remotePhotos := make([]appliancePhoto, 0)
		remoteOK := err == nil
		if remoteOK {
			if raw, ok := result["fotos"].([]any); ok {
				encoded, _ := json.Marshal(raw)
				if json.Unmarshal(encoded, &remotePhotos) != nil {
					remoteOK = false
				}
			} else {
				remoteOK = false
			}
		}
		localPhotos, localErr := listLocalServicePhotos(serviceID)
		if p.session == nil || p.session.AccessToken != token || p.teamServicePhotoID != serviceID {
			return
		}
		if localErr == nil {
			p.teamServicePhotos = mergeServicePhotos(remotePhotos, localPhotos)
		} else {
			p.teamServicePhotos = remotePhotos
		}
		if remoteOK {
			p.teamServicePhotoNotice = ""
		} else if len(localPhotos) > 0 {
			p.teamServicePhotoNotice = "Sem conexão com o servidor. Exibindo também as fotos salvas neste dispositivo."
		} else {
			p.teamServicePhotoNotice = "Não foi possível carregar as fotos da OS."
		}
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) chooseTeamServicePhotos(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.teamServicePhotoBusy || p.teamServicePhotoID == "" || p.session == nil {
		return
	}
	p.teamServicePhotoBusy = true
	p.teamServicePhotoNotice = "Preparando imagens..."
	selected := startAppliancePhotoPicker()
	go func() {
		files := <-selected
		if len(files) == 0 {
			p.teamServicePhotoBusy = false
			p.teamServicePhotoNotice = ""
			ctx.Update()
			return
		}
		if p.session == nil || p.teamServicePhotoID == "" {
			p.teamServicePhotoBusy = false
			p.teamServicePhotoNotice = "Sessão indisponível."
			ctx.Update()
			return
		}
		serviceID, token := p.teamServicePhotoID, p.session.AccessToken
		uploaded, savedLocally := 0, 0
		for _, file := range files {
			_, err := sendTeamJSONResult(ctx, apiBaseURL()+"/api/documentos", token, http.MethodPost, map[string]any{
				"acao": "upload", "pasta": "fotos", "serviceId": serviceID, "nome": file.Name, "base64": file.Base64,
			})
			if err == nil {
				uploaded++
				continue
			}
			if saveLocalServicePhoto(serviceID, file) == nil {
				savedLocally++
			}
		}
		p.teamServicePhotoBusy = false
		if savedLocally > 0 && uploaded > 0 {
			p.teamServicePhotoNotice = "Enviadas " + itoaPhoto(uploaded) + " e salvas neste dispositivo " + itoaPhoto(savedLocally) + ". As fotos locais ainda não foram sincronizadas."
		} else if savedLocally > 0 {
			p.teamServicePhotoNotice = "Fotos salvas neste dispositivo. Elas ainda não foram sincronizadas com o servidor."
		} else if uploaded == len(files) {
			p.teamServicePhotoNotice = "Fotos enviadas."
		} else if uploaded > 0 {
			p.teamServicePhotoNotice = "Enviadas " + itoaPhoto(uploaded) + " de " + itoaPhoto(len(files)) + " fotos. As demais não puderam ser salvas."
		} else {
			p.teamServicePhotoNotice = "Não foi possível enviar nem salvar as fotos neste dispositivo."
		}
		ctx.Update()
		p.loadTeamServicePhotos(ctx, serviceID)
	}()
}

func (p *serviceCatalogPage) askDeleteTeamServicePhoto(objectPath string) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		p.teamCustomerConfirm = teamServicePhotoDeleteAction + objectPath
		ctx.Update()
	}
}

func (p *serviceCatalogPage) teamServicePhotosPanel() app.UI {
	photos := make([]app.UI, 0, len(p.teamServicePhotos))
	for _, photo := range p.teamServicePhotos {
		photos = append(photos, app.Article().Class("portal-service").Body(
			app.A().Href(photo.URL).Target("_blank").Rel("noopener noreferrer").Body(app.Img().Src(photo.URL).Alt(photo.Name).Style("max-width", "180px").Style("max-height", "140px")),
			app.P().Body(app.Text(photo.Name)),
			app.Button().Class("auth-link").Type("button").Disabled(p.teamServicePhotoBusy).OnClick(p.askDeleteTeamServicePhoto(photo.Path)).Body(app.Text("Excluir foto")),
		))
	}
	notice := p.teamServicePhotoNotice
	if len(photos) == 0 && notice == "" {
		notice = "Nenhuma foto cadastrada nesta OS."
	}
	return app.Section().Class("portal-service team-service-detail__photos").Body(
		app.Div().Class("portal-service__heading").Body(app.Strong().Body(app.Text("Registro fotográfico da OS")), app.Span().Body(app.Text(itoaPhoto(len(photos))+" fotos"))),
		app.P().Class("portal-section__intro").Body(app.Text(notice)),
		app.Button().Class("auth-submit team-service-photos__select").Type("button").Disabled(p.teamServicePhotoBusy).OnClick(p.chooseTeamServicePhotos).Body(app.Text("Selecionar fotos")),
		app.Div().Class("service-grid").Body(photos...),
	)
}

func teamServiceLifecycleDate(service map[string]any) string {
	switch strings.ToUpper(portalText(service["status"])) {
	case "CONCLUIDO":
		return firstDashboardDate(portalText(service["data_conclusao"]), portalText(service["data_agendamento"]), portalText(service["data_solicitacao"]))
	case "CANCELADO":
		return firstDashboardDate(portalText(service["data_cancelamento"]), portalText(service["data_agendamento"]), portalText(service["data_solicitacao"]))
	case "EM_ANDAMENTO":
		return firstDashboardDate(portalText(service["data_inicio"]), portalText(service["data_agendamento"]), portalText(service["data_solicitacao"]))
	default:
		return firstDashboardDate(portalText(service["data_agendamento"]), portalText(service["data_solicitacao"]))
	}
}

func (p *serviceCatalogPage) downloadTeamServicePDF(sendWhatsApp bool) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		if p.teamServiceDetail == nil || p.session == nil || p.teamPDFBusy {
			return
		}
		serviceID, token := portalText(p.teamServiceDetail["id"]), p.session.AccessToken
		p.teamPDFBusy = true
		p.teamServiceWhatsAppURL = ""
		p.teamPDFNotice = "Gerando PDF da ordem de serviço..."
		ctx.Update()
		go func() {
			result := requestServiceOrderPDF(ctx, apiBaseURL()+"/api/os-pdf", token, serviceID, sendWhatsApp)
			if p.session == nil || p.session.AccessToken != token {
				return
			}
			p.teamPDFBusy = false
			if result.err != nil || result.status != http.StatusOK {
				p.teamPDFNotice = "Não foi possível gerar o PDF da ordem de serviço."
			} else if !sendWhatsApp {
				p.teamPDFNotice = "PDF gerado e baixado."
				p.registerLocalNotification(ctx, "Documento gerado", "A Ordem de Serviço foi baixada em PDF.")
			} else {
				switch result.whatsapp {
				case "queued":
					p.teamPDFNotice = "PDF gerado e registrado na fila do WhatsApp."
					p.registerLocalNotification(ctx, "Documento na fila do WhatsApp", "A Ordem de Serviço será acompanhada até o serviço aceitar o envio.")
				case "queue-error":
					p.teamPDFNotice = "PDF gerado, mas não entrou na fila do WhatsApp. Verifique a configuração da fila."
				case "sent":
					p.teamPDFNotice = "PDF gerado e enviado pelo WhatsApp."
					p.registerLocalNotification(ctx, "Documento enviado no WhatsApp", "A Ordem de Serviço foi enviada ao cliente.")
				case "failed":
					p.teamPDFNotice = "PDF gerado, mas o envio pelo WhatsApp falhou."
				default:
					p.teamPDFNotice = "PDF gerado. Abra o WhatsApp para compartilhar o comprovante."
				}
				if result.url != "" {
					p.teamReceiptURL = result.url
					if result.whatsapp != "sent" && p.teamServiceDetail != nil {
						customer, _ := p.teamServiceDetail["customers"].(map[string]any)
						message := teamServiceReceiptMessage(p.teamServiceDetail, p.teamProfile)
						p.teamServiceWhatsAppURL = serviceOrderWhatsAppFallbackURL(portalText(customer["whatsapp"]), message, result.url)
					}
				}
			}
			ctx.Update()
		}()
	}
}

func teamServiceReceiptMessage(service map[string]any, profile domain.TechnicianProfile) string {
	customer, _ := service["customers"].(map[string]any)
	appliance, _ := service["air_conditioners"].(map[string]any)
	date := teamServiceLifecycleDate(service)
	if strings.EqualFold(portalText(service["status"]), "CANCELADO") {
		date = firstDashboardDate(portalText(service["data_agendamento"]), portalText(service["data_solicitacao"]))
	}
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	observations := portalText(service["observacoes"])
	warranty := profile.DefaultWarrantyDays
	if match := teamServiceWarrantyMarker.FindStringSubmatch(observations); len(match) == 2 {
		if parsed, err := strconv.Atoi(match[1]); err == nil && parsed > 0 {
			warranty = parsed
		}
	}
	if warranty == 0 {
		warranty = 90
	}
	returnDate := markerValue(teamServiceReturnMarker, observations)
	if returnDate == "" {
		months := profile.DefaultReturnMonths
		if months == 0 {
			months = 6
		}
		if parsed, err := time.Parse("2006-01-02", date); err == nil {
			returnDate = domain.AddMonthsClamped(parsed, months).Format("2006-01-02")
		}
	}
	payment := markerValue(teamServicePaymentMarker, observations)
	if payment == "" {
		payment = "PIX"
	}
	return domain.BuildServiceOrderReceiptMessage(
		domain.Client{Name: portalText(customer["nome"])},
		domain.Appliance{Room: portalText(appliance["ambiente"]), Brand: portalText(appliance["marca"]), CapacityBTU: teamServiceCapacityText(appliance["btus"])},
		domain.MaintenanceRecord{Date: date, ReturnDate: returnDate, ServiceType: domain.ServiceType(teamServiceDisplayName(service)), Price: agendaNumber(service["valor"]), PaymentMethod: domain.PaymentMethod(payment), WarrantyDays: warranty},
		profile,
	)
}

func serviceOrderWhatsAppFallbackURL(phone, message, pdfURL string) string {
	phone = cleanTeamContactPhone(phone)
	if phone == "" || pdfURL == "" {
		return ""
	}
	message += "\n\n📄 *OS em PDF:* " + pdfURL
	return "https://wa.me/" + phone + "?text=" + url.QueryEscape(message)
}

func teamServiceWhatsAppFallbackLink(href string) app.UI {
	if href == "" {
		return app.Div()
	}
	return app.A().Class("auth-submit").Href(href).Target("_blank").Rel("noopener noreferrer").Body(app.Text("Abrir WhatsApp para enviar link do PDF"))
}
