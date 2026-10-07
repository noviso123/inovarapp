package webapp

import (
	"strings"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/adapter/cep"
)

func cepAddressSummary(address cep.Address) string {
	parts := make([]string, 0, 3)
	if value := strings.TrimSpace(address.Street); value != "" {
		parts = append(parts, value)
	}
	if value := strings.TrimSpace(address.Neighborhood); value != "" {
		parts = append(parts, value)
	}
	city := strings.TrimSpace(address.City)
	state := strings.TrimSpace(address.State)
	if city != "" && state != "" {
		city += "/" + state
	} else if state != "" {
		city = state
	}
	if city != "" {
		parts = append(parts, city)
	}
	return strings.Join(parts, " · ")
}

func cepAddressSuggestion(address *cep.Address, onSelect app.EventHandler) app.UI {
	if address == nil {
		return app.Span()
	}
	summary := cepAddressSummary(*address)
	if summary == "" {
		return app.Span()
	}
	return app.Button().Class("cep-address-suggestion").Type("button").Attr("aria-label", "Usar endereço encontrado: "+summary).OnClick(onSelect).Body(
		app.Span().Class("cep-address-suggestion__title").Body(app.Text("Endereço encontrado · toque para selecionar")),
		app.Strong().Class("cep-address-suggestion__address").Body(app.Text(summary)),
	)
}

func cepSearchFeedback(message string) app.UI {
	if strings.TrimSpace(message) == "" {
		return app.Span()
	}
	return app.P().Class("cep-search-feedback").Attr("role", "status").Body(app.Text(message))
}
