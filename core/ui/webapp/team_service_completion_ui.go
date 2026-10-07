package webapp

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/domain"
)

type teamCompletionForm struct {
	service      map[string]any
	serviceType  string
	checklist    map[string]any
	laborPrice   float64
	partsPrice   float64
	partsUsed    string
	payment      string
	warrantyDays int
	cycleMonths  int
	notes        string
	saving       bool
	message      string
}

var completionChecklistDefaults = map[string]any{
	"filtrosLavados": true, "serpentinaHigienizada": true, "turbinaLimpa": true,
	"drenoDesobstruido": true, "bandejaSanitizada": true, "condensadoraLavada": true,
	"aplicacaoBactericida": true, "testeEletricoCorrente": true,
	"correnteAmperes": "", "pressaoGasPSI": "", "saltoTermicoDeltaT": "",
	"temperaturaRetorno": "", "temperaturaInsuflamento": "",
	"suporteNivelado": true, "vacuoMicrons": "", "testeNitrogenio": true,
	"valvulasLiberadas": true, "superaquecimentoUtil": "", "capacitorTestado": "",
	"gasAdicionadoGramas": "", "pecasSubstituidas": "", "diagnosticoTecnico": "",
}

func newTeamCompletionForm(service map[string]any, profile domain.TechnicianProfile) *teamCompletionForm {
	serviceType := portalText(service["tipo"])
	if serviceType == "OUTRO" && portalText(service["descricao"]) != "" {
		serviceType = portalText(service["descricao"])
	} else {
		serviceType = string(supabase.MapSupabaseServiceTypeToLocal(serviceType))
	}
	if serviceType == "" {
		serviceType = string(domain.ServiceCleaning)
	}
	labor := profile.DefaultPrice
	if labor == 0 {
		labor = 250
	}
	warranty := teamCompletionWarrantyDays(serviceType, profile)
	months := profile.DefaultReturnMonths
	if months == 0 {
		months = 6
	}
	checklist := make(map[string]any, len(completionChecklistDefaults))
	for key, value := range completionChecklistDefaults {
		checklist[key] = value
	}
	return &teamCompletionForm{
		service: service, serviceType: serviceType, checklist: checklist,
		laborPrice: labor, payment: "PIX", warrantyDays: warranty, cycleMonths: months,
		partsPrice: 0, notes: portalText(service["problema"]),
	}
}

var warrantyDaysPattern = regexp.MustCompile(`\d+`)

func teamCompletionWarrantyDays(serviceType string, profile domain.TechnicianProfile) int {
	fallback := profile.DefaultWarrantyDays
	if fallback == 0 {
		fallback = 90
	}
	for _, entry := range domain.BuildServiceCatalog(&profile) {
		matches := entry.FixedType != "" && string(entry.FixedType) == domain.NormalizeServiceName(serviceType)
		if entry.FixedType == "" {
			matches = entry.Name == serviceType
		}
		if !matches {
			continue
		}
		warranty := entry.DefaultWarranty
		if !entry.Edited && entry.Card != nil {
			warranty = entry.Card.DefaultWarranty
		}
		if days, err := strconv.Atoi(warrantyDaysPattern.FindString(warranty)); err == nil {
			return days
		}
		break
	}
	return fallback
}

func (p *serviceCatalogPage) openTeamCompletion(service map[string]any) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		if portalText(service["id"]) == "" {
			return
		}
		if _, ok := service["air_conditioners"].(map[string]any); !ok {
			p.teamActionMessage = "Cadastre o aparelho na ficha do cliente antes de finalizar."
			return
		}
		p.teamActionMessage = ""
		p.teamCompletion = newTeamCompletionForm(service, p.teamProfile)
	}
}

func (p *serviceCatalogPage) closeTeamCompletion(ctx app.Context, event app.Event) {
	event.PreventDefault()
	if p.teamCompletion != nil && p.teamCompletion.saving {
		return
	}
	p.teamCompletion = nil
}

func (p *serviceCatalogPage) setCompletionText(key string) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		if p.teamCompletion != nil {
			value := event.Get("target").Get("value").String()
			p.teamCompletion.checklist[key] = value
			if key == "pecasSubstituidas" {
				p.teamCompletion.partsUsed = value
			}
		}
	}
}

func (p *serviceCatalogPage) setCompletionServiceType(ctx app.Context, event app.Event) {
	if p.teamCompletion == nil {
		return
	}
	p.teamCompletion.serviceType = event.Get("target").Get("value").String()
	p.teamCompletion.warrantyDays = teamCompletionWarrantyDays(p.teamCompletion.serviceType, p.teamProfile)
}

func (p *serviceCatalogPage) setCompletionCheck(key string) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		if p.teamCompletion != nil {
			p.teamCompletion.checklist[key] = event.Get("target").Get("checked").Bool()
		}
	}
}

func (p *serviceCatalogPage) submitTeamCompletion(ctx app.Context, event app.Event) {
	event.PreventDefault()
	f := p.teamCompletion
	if f == nil || f.saving || p.session == nil {
		return
	}
	serviceID, clientID := portalText(f.service["id"]), portalText(f.service["cliente_id"])
	applianceID := portalText(f.service["aparelho_id"])
	if serviceID == "" || clientID == "" || applianceID == "" {
		f.message = "Não foi possível identificar o cliente, aparelho ou ordem de serviço."
		return
	}
	checklist := teamCompletionChecklist(f)
	now := time.Now()
	completion, err := domain.BuildServiceCompletion(f.serviceType, f.payment, checklist, f.laborPrice, f.partsPrice, f.warrantyDays, f.cycleMonths, now)
	if err != nil {
		f.message = "Informe o tipo de serviço e a forma de pagamento para concluir."
		return
	}
	payloadFields := map[string]any{
		"tipo": supabase.MapServiceTypeToSupabase(f.serviceType), "status": "CONCLUIDO",
		"valor": completion.Total, "data_conclusao": completion.Date, "descricao": f.serviceType,
		"problema": strings.TrimSpace(f.notes), "observacoes": completion.Observations,
	}
	pageURL := app.Window().URL()
	if pageURL == nil || pageURL.Scheme == "" || pageURL.Host == "" {
		f.message = "Não foi possível conectar ao servidor."
		return
	}
	baseURL, token := apiBaseURL(), p.session.AccessToken
	f.saving, f.message = true, ""
	go func() {
		err := sendTeamJSON(ctx, baseURL+"/api/servicos", token, http.MethodPatch, map[string]any{"id": serviceID, "fields": payloadFields})
		if err != nil {
			// Compatibilidade com bases anteriores à coluna data_conclusao.
			delete(payloadFields, "data_conclusao")
			payloadFields["observacoes"] = completion.Observations + " [DATA_CONCLUSAO:" + completion.Date + "]"
			err = sendTeamJSON(ctx, baseURL+"/api/servicos", token, http.MethodPatch, map[string]any{"id": serviceID, "fields": payloadFields})
		}
		if err != nil {
			f.saving, f.message = false, "Não foi possível concluir no banco. Os dados do formulário continuam disponíveis; tente novamente."
			ctx.Update()
			return
		}
		// These two records were best-effort in the existing app; the completed
		// order remains authoritative if either secondary write fails.
		_ = sendTeamJSON(ctx, baseURL+"/api/aparelho-manutencao", token, http.MethodPatch, map[string]any{
			"id": applianceID, "fields": map[string]any{"ultima_manutencao": completion.Date},
		})
		history := map[string]any{
			"service_id": serviceID, "cliente_id": clientID, "aparelho_id": applianceID,
			"data": completion.Date, "descricao": f.serviceType, "problema": strings.TrimSpace(f.notes),
			"solucao": f.serviceType, "pecas_utilizadas": f.partsUsed,
			"observacoes": completion.ChecklistJSON, "valor": completion.Total,
		}
		_ = sendTeamJSON(ctx, baseURL+"/api/historico", token, http.MethodPost, history)
		pdfResult := requestServiceOrderPDF(ctx, baseURL+"/api/os-pdf", token, serviceID, true)
		if pdfResult.err == nil && pdfResult.status == http.StatusOK {
			switch pdfResult.whatsapp {
			case "queued":
				f.message = "Ordem de serviço concluída; comprovante e mensagem registrados na fila do WhatsApp."
				p.registerLocalNotification(ctx, "OS na fila do WhatsApp", "O comprovante será acompanhado até o serviço aceitar o envio.")
			case "queue-error":
				f.message = "Ordem de serviço concluída, mas o comprovante não entrou na fila do WhatsApp. Verifique a configuração da fila."
			case "sent":
				f.message = "Ordem de serviço concluída; PDF enviado ao cliente pelo WhatsApp."
				customerName := "o cliente"
				if customer, ok := f.service["customers"].(map[string]any); ok && portalText(customer["nome"]) != "" {
					customerName = portalText(customer["nome"])
				}
				p.registerLocalNotification(ctx, "OS enviada no WhatsApp", "O comprovante de "+customerName+" foi enviado automaticamente.")
			case "sent-link":
				f.message = "Ordem de serviço concluída; a mensagem com o link do PDF foi enviada pelo WhatsApp, mas o anexo não foi aceito."
				customerName := "o cliente"
				if customer, ok := f.service["customers"].(map[string]any); ok && portalText(customer["nome"]) != "" {
					customerName = portalText(customer["nome"])
				}
				p.registerLocalNotification(ctx, "Link da OS enviado no WhatsApp", "O comprovante de "+customerName+" foi enviado como link; o anexo PDF não foi aceito.")
			case "failed":
				f.message = "Ordem de serviço concluída. Não foi possível enviar o PDF pelo WhatsApp."
			default:
				f.message = "Ordem de serviço concluída. O PDF foi gerado; o WhatsApp não está configurado."
			}
			if pdfResult.url != "" {
				p.teamReceiptURL = pdfResult.url
			}
		} else {
			f.message = "Ordem de serviço concluída. Não foi possível gerar ou enviar o PDF automático."
		}
		customerName := "o cliente"
		if customer, ok := f.service["customers"].(map[string]any); ok && portalText(customer["nome"]) != "" {
			customerName = portalText(customer["nome"])
		}
		p.registerLocalNotification(ctx, "Ordem de Serviço concluída", "OS de "+f.serviceType+" registrada para "+customerName+".")
		f.saving = false
		p.teamActionMessage = f.message
		p.teamCompletion = nil
		p.teamLoadedFor = ""
		p.loadTeamOperations(ctx)
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) teamCompletionDialog() app.UI {
	f := p.teamCompletion
	if f == nil {
		return app.Div()
	}
	service := f.service
	customerName := "Cliente"
	if customer, ok := service["customers"].(map[string]any); ok && portalText(customer["nome"]) != "" {
		customerName = portalText(customer["nome"])
	}
	appliance, _ := service["air_conditioners"].(map[string]any)
	device := strings.TrimSpace(portalText(appliance["marca"]) + " " + portalText(appliance["modelo"]))
	if device == "" {
		device = portalText(appliance["ambiente"])
	}
	if device == "" {
		device = "Ar-Condicionado"
	}
	catalog := domain.BuildServiceCatalog(&p.teamProfile)
	serviceOptions := make([]app.UI, 0, len(catalog))
	for _, entry := range catalog {
		serviceOptions = append(serviceOptions, app.Option().Value(teamCompletionCatalogValue(entry)).Body(app.Text(entry.Name)))
	}
	checklist := []app.UI{app.H3().Class("team-section__heading-title").Body(app.Text("Procedimento Técnico"))}
	addChecks := func(entries ...[2]string) {
		for _, item := range entries {
			checked, _ := f.checklist[item[0]].(bool)
			checklist = append(checklist, app.Label().Class("auth-field team-completion-check").Body(
				app.Input().Type("checkbox").Checked(checked).OnChange(p.setCompletionCheck(item[0])), app.Text(item[1]),
			))
		}
	}
	addText := func(key, label, placeholder string) {
		checklist = append(checklist, app.Label().Class("auth-field").Body(app.Text(label), app.Input().Type("text").Value(portalText(f.checklist[key])).Placeholder(placeholder).OnChange(p.setCompletionText(key))))
	}
	addTextarea := func(key, label, placeholder string) {
		checklist = append(checklist, app.Label().Class("auth-field").Body(app.Text(label), app.Textarea().Rows(3).Text(portalText(f.checklist[key])).Placeholder(placeholder).OnChange(p.setCompletionText(key))))
	}
	switch domain.NormalizeServiceName(f.serviceType) {
	case string(domain.ServiceCleaning):
		addChecks([2]string{"filtrosLavados", "Filtros desmontados e lavados"}, [2]string{"serpentinaHigienizada", "Serpentina evaporadora escovada e limpa"}, [2]string{"turbinaLimpa", "Turbina desobstruída (remoção de limo)"}, [2]string{"drenoDesobstruido", "Dreno e mangueiras desobstruídos"}, [2]string{"bandejaSanitizada", "Bandeja de condensado sanitizada"}, [2]string{"condensadoraLavada", "Condensadora externa lavada"}, [2]string{"aplicacaoBactericida", "Aplicação de bactericida homologado"}, [2]string{"testeEletricoCorrente", "Teste elétrico e consumo do compressor"})
		addText("correnteAmperes", "Corrente (A)", "")
		addText("pressaoGasPSI", "Pressão (PSI)", "")
		addText("saltoTermicoDeltaT", "Delta T (Salto)", "")
	case string(domain.ServiceInstallation):
		addChecks([2]string{"suporteNivelado", "Suporte evaporadora/condensadora com nível de bolha"}, [2]string{"testeNitrogenio", "Teste de estanqueidade das flanges com Nitrogênio"}, [2]string{"valvulasLiberadas", "Abertura das válvulas de serviço após estanqueidade"}, [2]string{"testeEletricoCorrente", "Fiação PP conforme norma do fabricante e disjuntor"})
		addText("vacuoMicrons", "Vácuo da Linha (Microns)", "Informe o valor medido")
		addText("superaquecimentoUtil", "Superaquecimento Útil", "Ex.: 5°C a 7°C")
	case string(domain.ServiceCorrective):
		addText("diagnosticoTecnico", "Diagnóstico / Defeito Identificado", "Descreva o defeito identificado")
		addText("capacitorTestado", "Teste do Capacitor (µF)", "Ex.: 35 µF")
		addText("pecasSubstituidas", "Peça(s) Substituída(s)", "Descreva as peças")
	case string(domain.ServiceRefrigerant):
		gasType := firstNonEmptyBudget(portalText(appliance["gas_tipo"]), "R-410A")
		gasOptions := []app.UI{app.Option().Value("R-410A").Body(app.Text("R-410A")), app.Option().Value("R-32").Body(app.Text("R-32")), app.Option().Value("R-22").Body(app.Text("R-22")), app.Option().Value("Outro").Body(app.Text("Outro"))}
		if gasType != "R-410A" && gasType != "R-32" && gasType != "R-22" && gasType != "Outro" {
			gasOptions = append([]app.UI{app.Option().Value(gasType).Body(app.Text(gasType))}, gasOptions...)
		}
		checklist = append(checklist, app.Label().Class("auth-field").Body(
			app.Text("Tipo de Gás"),
			app.Select().Attr("value", gasType).Disabled(true).Attr("title", "Edite o gás na Ficha do Aparelho do cliente").Body(gasOptions...),
		))
		addText("gasAdicionadoGramas", "Carga na Balança (g)", "Ex.: 550 gramas")
	default:
		addTextarea("diagnosticoTecnico", "Diagnóstico / Descrição do Serviço", "Descreva o que foi verificado e executado no equipamento")
		addText("pecasSubstituidas", "Peças e Materiais Utilizados", "Ex.: Capacitor, filtro de linha")
	}
	return app.Div().Class("auth-backdrop").Body(app.Div().Class("auth-dialog team-completion-dialog").Body(
		app.Div().Class("auth-dialog__heading").Body(app.Div().Body(
			app.P().Class("catalog__eyebrow").Body(app.Text("ORDEM DE SERVIÇO")),
			app.H2().Class("auth-dialog__title").Body(app.Text("Checklist e Ordem de Serviço")),
			app.P().Class("auth-dialog__intro").Body(app.Text(customerName+" • "+device)),
		), app.Button().Class("auth-close").Type("button").Disabled(f.saving).OnClick(p.closeTeamCompletion).Body(app.Text("Fechar"))),
		app.Form().Class("auth-form team-completion-form").OnSubmit(p.submitTeamCompletion).Body(
			app.Label().Class("auth-field").Body(app.Text("Tipo de Atendimento Técnico"), app.Select().Attr("value", f.serviceType).OnChange(p.setCompletionServiceType).Body(serviceOptions...)),
			app.Div().Class("team-completion-checklist").Body(checklist...),
			app.H3().Class("team-section__heading-title").Body(app.Text("Fechamento da Ordem de Serviço")),
			app.Label().Class("auth-field").Body(app.Text("Mão de Obra (R$)"), app.Input().Type("number").Attr("min", 0).Attr("step", 10).Value(strconv.FormatFloat(f.laborPrice, 'f', -1, 64)).OnChange(func(ctx app.Context, event app.Event) {
				f.laborPrice, _ = strconv.ParseFloat(event.Get("target").Get("value").String(), 64)
			})),
			app.Label().Class("auth-field").Body(app.Text("Peças / Materiais (R$)"), app.Input().Type("number").Attr("min", 0).Attr("step", 10).Value(strconv.FormatFloat(f.partsPrice, 'f', -1, 64)).OnChange(func(ctx app.Context, event app.Event) {
				f.partsPrice, _ = strconv.ParseFloat(event.Get("target").Get("value").String(), 64)
			})),
			app.Label().Class("auth-field").Body(app.Text("Forma de Pagamento"), app.Select().Attr("value", f.payment).OnChange(p.ValueTo(&f.payment)).Body(app.Option().Value("PIX").Body(app.Text("PIX")), app.Option().Value("Cartão Crédito").Body(app.Text("Cartão Crédito")), app.Option().Value("Cartão Débito").Body(app.Text("Cartão Débito")), app.Option().Value("Dinheiro").Body(app.Text("Dinheiro")), app.Option().Value("A Faturar").Body(app.Text("A Faturar")))),
			app.Label().Class("auth-field").Body(app.Text("Garantia (dias)"), app.Input().Type("number").Attr("min", 0).Value(strconv.Itoa(f.warrantyDays)).OnChange(func(ctx app.Context, event app.Event) {
				f.warrantyDays, _ = strconv.Atoi(event.Get("target").Get("value").String())
			})),
			app.Label().Class("auth-field").Body(app.Text("Próxima manutenção (meses)"), app.Input().Type("number").Attr("min", 0).Value(strconv.Itoa(f.cycleMonths)).OnChange(func(ctx app.Context, event app.Event) {
				f.cycleMonths, _ = strconv.Atoi(event.Get("target").Get("value").String())
			})),
			app.Label().Class("auth-field").Body(app.Text("Peças / Materiais Utilizados"), app.Input().Type("text").Value(f.partsUsed).OnChange(p.ValueTo(&f.partsUsed))),
			app.Label().Class("auth-field").Body(app.Text("Observações"), app.Input().Type("text").Value(f.notes).OnChange(p.ValueTo(&f.notes))),
			app.P().Class("portal-section__intro").Body(app.Text("Total: R$ "+formatPortalMoney(f.laborPrice+f.partsPrice))),
			formErrorNotice(f.message),
			app.Div().Class("portal-budget__actions").Body(app.Button().Class("auth-link").Type("button").Disabled(f.saving).OnClick(p.closeTeamCompletion).Body(app.Text("Cancelar")), app.Button().Class("auth-submit").Type("submit").Disabled(f.saving).Body(app.Text("Concluir Ordem de Serviço"))),
		),
	))
}

func teamCompletionCatalogValue(entry domain.CatalogEntry) string {
	if entry.FixedType != "" {
		return string(entry.FixedType)
	}
	return entry.Name
}
