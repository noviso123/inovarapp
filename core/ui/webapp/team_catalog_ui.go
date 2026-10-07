package webapp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/domain"
)

type teamCatalogForm struct {
	Key, Name, Price, Description, AverageTime, Warranty, Badge, Items string
	Message                                                            string
}

func (p *serviceCatalogPage) openTeamCatalogForm(entry *domain.CatalogEntry) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		form := newTeamCatalogForm(entry)
		p.teamCatalogForm, p.teamCatalogNotice = form, ""
		ctx.Update()
	}
}

func newTeamCatalogForm(entry *domain.CatalogEntry) *teamCatalogForm {
	form := &teamCatalogForm{Price: "250"}
	if entry == nil {
		return form
	}
	form.Key, form.Name = entry.Key, entry.Name
	form.Price = strconv.FormatFloat(entry.Price, 'f', -1, 64)
	form.Description, form.AverageTime, form.Warranty, form.Badge = entry.Description, entry.AverageTime, entry.DefaultWarranty, entry.Badge
	items := entry.Items
	if entry.Card != nil {
		form.Description = firstNonEmptyBudget(form.Description, entry.Card.Description)
		form.AverageTime = firstNonEmptyBudget(form.AverageTime, entry.Card.AverageTime)
		form.Warranty = firstNonEmptyBudget(form.Warranty, entry.Card.DefaultWarranty)
		form.Badge = firstNonEmptyBudget(form.Badge, entry.Card.Badge)
		if len(items) == 0 {
			items = entry.Card.IncludedItems
		}
	}
	form.Items = strings.Join(items, "\n")
	return form
}

func teamServiceCatalogCard(entry domain.CatalogEntry) (domain.ServiceCard, string) {
	var card domain.ServiceCard
	if entry.Card != nil {
		card = *entry.Card
		card.IncludedItems = append([]string(nil), entry.Card.IncludedItems...)
	} else {
		card = domain.ServiceCard{
			Title: entry.Name, Subtitle: "Serviço personalizado da equipe",
			Description: "Tipo de serviço cadastrado pela equipe Inovar. Disponível no checklist, em orçamentos e agendamentos.",
		}
	}
	card.Title = entry.Name
	if entry.Description != "" {
		card.Description = entry.Description
	}
	if entry.AverageTime != "" {
		card.AverageTime = entry.AverageTime
	}
	if entry.DefaultWarranty != "" {
		card.DefaultWarranty = entry.DefaultWarranty
	}
	if entry.Badge != "" {
		card.Badge = entry.Badge
	}
	if len(entry.Items) > 0 {
		card.IncludedItems = append([]string(nil), entry.Items...)
	}
	price := card.ReferencePrice
	if entry.Price > 0 && (entry.Card == nil || entry.Edited) {
		price = "R$ " + formatPortalMoney(entry.Price)
	}
	if price == "" {
		price = "Sob consulta"
	}
	if card.AverageTime == "" {
		card.AverageTime = "—"
	}
	if card.DefaultWarranty == "" {
		card.DefaultWarranty = "—"
	}
	return card, price
}

func (p *serviceCatalogPage) saveTeamCatalogForm(ctx app.Context, event app.Event) {
	event.PreventDefault()
	f := p.teamCatalogForm
	if f == nil || p.session == nil || p.teamCatalogSaving {
		return
	}
	name := strings.TrimSpace(f.Name)
	price, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(f.Price), ",", "."), 64)
	if name == "" || err != nil || price < 0 {
		f.Message = "Informe o nome do serviço e um preço válido."
		ctx.Update()
		return
	}
	data := domain.DadosTipoServico{Name: name, Price: price, Description: strings.TrimSpace(f.Description), AverageTime: strings.TrimSpace(f.AverageTime), DefaultWarranty: strings.TrimSpace(f.Warranty), Badge: strings.TrimSpace(f.Badge)}
	for _, item := range strings.Split(f.Items, "\n") {
		if item = strings.TrimSpace(item); item != "" {
			data.Items = append(data.Items, item)
		}
	}
	profile := p.teamProfile
	profile, err = applyTeamCatalogChange(profile, f.Key, data)
	if err != nil {
		f.Message = "Este tipo de serviço não foi encontrado. Atualize a tela e tente novamente."
		ctx.Update()
		return
	}
	p.persistTeamCatalogProfile(ctx, profile, "Serviço \""+name+"\" salvo no catálogo.")
}

func (p *serviceCatalogPage) requestTeamCatalogDelete(key string) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		p.teamCatalogConfirmKey = key
		ctx.Update()
	}
}

func (p *serviceCatalogPage) deleteTeamCatalogEntry(ctx app.Context, event app.Event) {
	event.PreventDefault()
	key := p.teamCatalogConfirmKey
	if key == "" || p.session == nil || p.teamCatalogSaving {
		return
	}
	profile, name, err := removeTeamCatalogEntry(p.teamProfile, key)
	if err != nil {
		p.teamCatalogConfirmKey = ""
		ctx.Update()
		return
	}
	p.teamCatalogConfirmKey = ""
	p.persistTeamCatalogProfile(ctx, profile, "Serviço \""+name+"\" removido do catálogo.")
}

func containsCatalogValue(values []string, value string) bool {
	for _, existing := range values {
		if domain.NormalizeServiceName(existing) == domain.NormalizeServiceName(value) {
			return true
		}
	}
	return false
}

func applyTeamCatalogChange(profile domain.TechnicianProfile, key string, data domain.DadosTipoServico) (domain.TechnicianProfile, error) {
	switch {
	case key == "":
		profile.CustomServiceTypes = append(profile.CustomServiceTypes, domain.CustomServiceType(data))
	case strings.HasPrefix(key, "fixo:"):
		typ := strings.TrimPrefix(key, "fixo:")
		for i := range profile.EditedFixedServiceTypes {
			if domain.NormalizeServiceName(profile.EditedFixedServiceTypes[i].Type) == domain.NormalizeServiceName(typ) {
				profile.EditedFixedServiceTypes[i] = domain.EditedFixedServiceType{Type: typ, DadosTipoServico: data}
				return profile, nil
			}
		}
		profile.EditedFixedServiceTypes = append(profile.EditedFixedServiceTypes, domain.EditedFixedServiceType{Type: typ, DadosTipoServico: data})
	default:
		index, err := strconv.Atoi(strings.TrimPrefix(key, "custom:"))
		if err != nil || index < 0 || index >= len(profile.CustomServiceTypes) {
			return profile, fmt.Errorf("invalid catalog key %q", key)
		}
		profile.CustomServiceTypes[index] = domain.CustomServiceType(data)
	}
	return profile, nil
}

func removeTeamCatalogEntry(profile domain.TechnicianProfile, key string) (domain.TechnicianProfile, string, error) {
	if strings.HasPrefix(key, "fixo:") {
		typ := strings.TrimPrefix(key, "fixo:")
		name := typ
		for _, entry := range domain.BuildServiceCatalog(&profile) {
			if entry.Key == key {
				name = entry.Name
				break
			}
		}
		if !containsCatalogValue(profile.RemovedFixedServiceTypes, typ) {
			profile.RemovedFixedServiceTypes = append(profile.RemovedFixedServiceTypes, typ)
		}
		return profile, name, nil
	}
	index, err := strconv.Atoi(strings.TrimPrefix(key, "custom:"))
	if err != nil || !strings.HasPrefix(key, "custom:") || index < 0 || index >= len(profile.CustomServiceTypes) {
		return profile, "", fmt.Errorf("invalid catalog key %q", key)
	}
	name := profile.CustomServiceTypes[index].Name
	profile.CustomServiceTypes = append(profile.CustomServiceTypes[:index], profile.CustomServiceTypes[index+1:]...)
	return profile, name, nil
}

func (p *serviceCatalogPage) persistTeamCatalogProfile(ctx app.Context, profile domain.TechnicianProfile, success string) {
	if p.session == nil || p.teamCatalogSaving {
		return
	}
	token := p.session.AccessToken
	p.teamCatalogSaving, p.teamCatalogNotice = true, "Salvando catálogo..."
	ctx.Update()
	go func() {
		result, err := sendTeamJSONResult(ctx, apiBaseURL()+"/api/configuracoes", token, http.MethodPost, profile)
		if p.session == nil || p.session.AccessToken != token {
			return
		}
		p.teamCatalogSaving = false
		if err != nil || result["ok"] != true {
			p.teamCatalogNotice = "Não foi possível salvar o catálogo. Verifique sua conexão e tente novamente."
			ctx.Update()
			return
		}
		if config, ok := result["config"].(map[string]any); ok {
			if encoded, marshalErr := json.Marshal(config); marshalErr == nil {
				var saved domain.TechnicianProfile
				if json.Unmarshal(encoded, &saved) == nil {
					profile = mergeTechnicianProfile(profile, saved)
				}
			}
		}
		p.teamProfile, p.teamCatalogForm = profile, nil
		p.teamCatalogNotice = success
		ctx.Update()
	}()
}

func (p *serviceCatalogPage) teamServiceCatalogManager() app.UI {
	entries := domain.BuildServiceCatalog(&p.teamProfile)
	cards := make([]app.UI, 0, len(entries))
	for i := range entries {
		entry := entries[i]
		price := "Sob consulta"
		if entry.Price > 0 {
			price = "R$ " + formatPortalMoney(entry.Price)
		}
		cards = append(cards, app.Article().Class("portal-service").Body(
			app.Div().Class("portal-service__heading").Body(app.Strong().Body(app.Text(entry.Name)), app.Span().Class("portal-service__status").Body(app.Text(price))),
			app.P().Class("portal-service__date").Body(app.Text(firstNonEmptyBudget(entry.Description, "Serviço disponível para orçamento, agendamento e Ordem de Serviço."))),
			app.Div().Class("portal-budget__actions").Body(
				app.Button().Class("auth-link").Type("button").OnClick(p.openTeamCatalogForm(&entry)).Body(app.Text("Editar")),
				app.Button().Class("auth-link").Type("button").Disabled(p.teamCatalogSaving).OnClick(p.requestTeamCatalogDelete(entry.Key)).Body(app.Text("Remover")),
			),
		))
	}
	content := []app.UI{
		app.Div().Class("portal-service__heading").Body(app.H3().Body(app.Text("Catálogo de serviços")), app.Button().Class("auth-submit").Type("button").Disabled(p.teamCatalogSaving).OnClick(p.openTeamCatalogForm(nil)).Body(app.Text("Novo Tipo de Serviço"))),
		app.P().Class("portal-section__intro").Body(app.Text("Gerencie os tipos usados em orçamentos, agendamentos, checklists e Ordens de Serviço.")),
	}
	if p.teamCatalogNotice != "" {
		content = append(content, app.P().Class("portal-section__intro").Body(app.Text(p.teamCatalogNotice)))
	}
	content = append(content, cards...)
	if p.teamCatalogConfirmKey != "" {
		entryName := p.teamCatalogConfirmKey
		confirmText := "Remover \"" + entryName + "\" do catálogo de serviços?"
		for _, entry := range entries {
			if entry.Key == p.teamCatalogConfirmKey {
				entryName = entry.Name
				break
			}
		}
		if strings.HasPrefix(p.teamCatalogConfirmKey, "fixo:") {
			confirmText = "Remover \"" + entryName + "\" do catálogo?\n\nEle deixará de aparecer no checklist, nos orçamentos e nos agendamentos futuros."
		} else {
			confirmText = "Remover \"" + entryName + "\" do catálogo de serviços?"
		}
		content = append(content, app.Div().Class("auth-notice").Body(
			app.P().Body(app.Text(confirmText)),
			app.Button().Class("auth-submit").Type("button").Disabled(p.teamCatalogSaving).OnClick(p.deleteTeamCatalogEntry).Body(app.Text("Confirmar remoção")),
			app.Button().Class("auth-link").Type("button").Disabled(p.teamCatalogSaving).OnClick(func(ctx app.Context, event app.Event) {
				event.PreventDefault()
				p.teamCatalogConfirmKey = ""
				ctx.Update()
			}).Body(app.Text("Cancelar")),
		))
	}
	if p.teamCatalogForm != nil {
		content = append(content, p.teamCatalogDialog())
	}
	return app.Div().Class("team-service-catalog").Body(content...)
}

func (p *serviceCatalogPage) teamCatalogDialog() app.UI {
	f := p.teamCatalogForm
	field := func(label string, target *string, kind string) app.UI {
		return app.Label().Class("auth-field team-catalog-field").Body(app.Text(label), app.Input().Type(kind).Value(*target).OnChange(p.ValueTo(target)))
	}
	title := "Novo Tipo de Serviço"
	if f.Key != "" {
		title = "Editar Tipo de Serviço"
	}
	return app.Div().Class("auth-backdrop team-catalog-backdrop").Body(app.Div().Class("auth-dialog team-catalog-dialog").Body(
		app.H2().Class("auth-dialog__title").Body(app.Text(title)),
		app.Form().Class("auth-form team-catalog-form").OnSubmit(func(ctx app.Context, event app.Event) { event.PreventDefault(); p.saveTeamCatalogForm(ctx, event) }).Body(
			field("Nome do serviço", &f.Name, "text"), field("Preço (R$)", &f.Price, "number"),
			field("Descrição", &f.Description, "text"), field("Tempo médio", &f.AverageTime, "text"),
			field("Garantia padrão", &f.Warranty, "text"), field("Selo / destaque", &f.Badge, "text"),
			app.Label().Class("auth-field team-catalog-field").Body(app.Text("Itens inclusos (um por linha)"), app.Textarea().Rows(5).Text(f.Items).OnChange(p.ValueTo(&f.Items))),
			formErrorNotice(f.Message),
			app.Div().Class("portal-budget__actions").Body(
				app.Button().Class("auth-link").Type("button").Disabled(p.teamCatalogSaving).OnClick(func(ctx app.Context, event app.Event) { event.PreventDefault(); p.teamCatalogForm = nil; ctx.Update() }).Body(app.Text("Cancelar")),
				app.Button().Class("auth-submit").Type("submit").Disabled(p.teamCatalogSaving).Body(app.Text("Salvar no catálogo")),
			),
		),
	))
}
