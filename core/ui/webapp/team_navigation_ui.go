package webapp

import (
	"strings"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

type teamNavigationItem struct {
	label  string
	anchor string
	count  int
}

func teamNavigationKey(anchor string) string {
	switch anchor {
	case "team-dashboard-metrics":
		return "dashboard"
	case "team-contact-center":
		return "contact"
	case "team-budgets":
		return "budgets"
	case "team-calendar":
		return "calendar"
	case "team-finance":
		return "finance"
	case "team-operations-services":
		return "services"
	case "team-customers":
		return "customers"
	case "team-whatsapp-queue":
		return "whatsapp"
	case "team-qr-code":
		return "qr-code"
	default:
		return "dashboard"
	}
}

// restoreTeamSectionFromURL makes staff screens shareable and allows a direct
// link to open the WhatsApp outbox after the saved session is restored.
func (p *serviceCatalogPage) restoreTeamSectionFromURL() {
	pageURL := app.Window().URL()
	if pageURL == nil {
		return
	}
	if section := teamSectionFromURLFragment(pageURL.Fragment); section != "dashboard" {
		p.teamActiveSection = section
	}
}

func teamSectionFromURLFragment(fragment string) string {
	return teamNavigationKey(strings.TrimPrefix(fragment, "/"))
}

func (p *serviceCatalogPage) loadWhatsAppQueueIfSelected(ctx app.Context) {
	if p.teamActiveSection == "whatsapp" {
		p.loadWhatsAppQueue(ctx)
	}
}

func (p *serviceCatalogPage) selectTeamSection(anchor string) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		p.teamActiveSection = teamNavigationKey(anchor)
		p.teamNavOpen = false
		if pageURL := app.Window().URL(); pageURL != nil {
			pageURL.Fragment = anchor
			ctx.Page().ReplaceURL(pageURL)
		}
		if anchor == "team-whatsapp-queue" {
			p.loadWhatsAppQueue(ctx)
		}
		ctx.Update()
	}
}

func (p *serviceCatalogPage) toggleTeamNavigation(ctx app.Context, event app.Event) {
	event.PreventDefault()
	p.teamNavOpen = !p.teamNavOpen
	ctx.Update()
}

func (p *serviceCatalogPage) teamWorkspaceNavigation() app.UI {
	items := []teamNavigationItem{
		{label: "Painel", anchor: "team-dashboard-metrics"},
		{label: "Central de Atendimento", anchor: "team-contact-center", count: p.teamDashboard.ReturnsOverdue + p.teamDashboard.ReturnsThisWeek},
		{label: "Propostas", anchor: "team-budgets", count: p.teamDashboard.TotalBudgets},
		{label: "Agenda", anchor: "team-calendar", count: p.teamDashboard.Scheduled + p.teamDashboard.InProgress},
		{label: "Financeiro", anchor: "team-finance"},
		{label: "Serviços", anchor: "team-operations-services", count: p.teamDashboard.TotalServices},
		{label: "Clientes", anchor: "team-customers", count: p.teamDashboard.Customers},
		{label: "Mensagens WhatsApp", anchor: "team-whatsapp-queue", count: p.teamWhatsAppQueueCounts["pendente"] + p.teamWhatsAppQueueCounts["processando"]},
		{label: "QR da Empresa", anchor: "team-qr-code"},
	}
	links := make([]app.UI, 0, len(items))
	for index, item := range items {
		label := []app.UI{
			app.Span().Class("team-workspace-nav__label team-workspace-nav__label--desktop").Body(app.Text(item.label)),
			app.Span().Class("team-workspace-nav__label team-workspace-nav__label--mobile").Body(app.Text(teamMobileNavLabel(item.anchor))),
		}
		if item.count > 0 {
			label = append(label, app.Span().Class("team-workspace-nav__count").Body(app.Text(item.count)))
		}
		className := "team-workspace-nav__link"
		if index < 4 {
			className += " team-workspace-nav__link--primary"
		} else {
			className += " team-workspace-nav__link--more"
		}
		if teamNavigationKey(item.anchor) == p.teamActiveSection || p.teamActiveSection == "" && item.anchor == "team-dashboard-metrics" {
			className += " team-workspace-nav__link--active"
		}
		links = append(links, app.Button().Class(className).Type("button").Attr("aria-current", func() string {
			if strings.Contains(className, "--active") {
				return "page"
			}
			return "false"
		}()).Attr("title", "Abrir "+item.label).OnClick(p.selectTeamSection(item.anchor)).Body(append([]app.UI{app.Span().Class("team-workspace-nav__icon").Body(app.Text(teamNavigationIcon(item.anchor)))}, label...)...))
	}
	linksClass := "team-workspace-nav__links"
	if p.teamNavOpen {
		linksClass += " team-workspace-nav__links--open"
	}
	navClass := "team-workspace-nav"
	if p.teamNavOpen {
		navClass += " team-workspace-nav--open"
	}
	toggleLabel := "Abrir mais seções"
	toggleText := "Mais"
	if p.teamNavOpen {
		toggleLabel, toggleText = "Fechar mais seções", "Fechar"
	}
	return app.Nav().Class(navClass).Attr("aria-label", "Navegação do painel da equipe").Body(
		app.Button().Class("team-workspace-nav__mobile-toggle").Type("button").Attr("aria-label", toggleLabel).Attr("aria-expanded", ariaBoolean(p.teamNavOpen)).OnClick(p.toggleTeamNavigation).Body(
			app.Span().Body(app.Text(toggleText)),
			app.Span().Class("team-workspace-nav__mobile-chevron").Body(app.Text(map[bool]string{true: "⌃", false: "⌄"}[p.teamNavOpen])),
		),
		app.Div().Class(linksClass).Body(links...),
		app.Button().Class("team-workspace-nav__quick-budget").Type("button").Attr("aria-label", "Criar orçamento completo").Attr("title", "Criar orçamento completo").OnClick(func(ctx app.Context, event app.Event) {
			p.teamNavOpen = false
			p.openTeamBudgetForm()(ctx, event)
		}).Body(
			app.Span().Class("team-workspace-nav__quick-budget-icon").Body(app.Text("＋")),
			app.Span().Class("team-workspace-nav__quick-budget-label").Body(app.Text("Orçamento")),
		),
		app.Div().Class("team-workspace-nav__actions").Body(
			app.Button().Class("team-workspace-nav__action team-workspace-nav__action--primary").Type("button").OnClick(p.openTeamBudgetForm()).Body(app.Text("▣ Novo Orçamento")),
			app.Button().Class("team-workspace-nav__action").Type("button").OnClick(p.openTeamCustomerForm(nil)).Body(app.Text("＋ Cliente")),
		),
	)
}

func teamNavigationIcon(anchor string) string {
	switch anchor {
	case "team-dashboard-metrics":
		return "⌂"
	case "team-contact-center":
		return "◉"
	case "team-budgets":
		return "▤"
	case "team-calendar":
		return "▦"
	case "team-finance":
		return "＄"
	case "team-operations-services":
		return "⚒"
	case "team-customers":
		return "♙"
	case "team-whatsapp-queue":
		return "◌"
	case "team-qr-code":
		return "▦"
	default:
		return "•"
	}
}

func teamMobileDestination(section string) string {
	switch section {
	case "contact":
		return "Central"
	case "budgets":
		return "Propostas"
	case "calendar":
		return "Agenda"
	case "finance":
		return "Financeiro"
	case "services":
		return "Serviços"
	case "customers":
		return "Clientes"
	case "whatsapp":
		return "WhatsApp"
	case "qr-code":
		return "QR da empresa"
	default:
		return "Painel"
	}
}

func teamMobileNavLabel(anchor string) string {
	switch anchor {
	case "team-dashboard-metrics":
		return "Painel"
	case "team-contact-center":
		return "Central"
	case "team-budgets":
		return "Propostas"
	case "team-calendar":
		return "Agenda"
	case "team-finance":
		return "Financeiro"
	case "team-operations-services":
		return "Serviços"
	case "team-customers":
		return "Clientes"
	case "team-whatsapp-queue":
		return "WhatsApp"
	case "team-qr-code":
		return "QR empresa"
	default:
		return "Painel"
	}
}

