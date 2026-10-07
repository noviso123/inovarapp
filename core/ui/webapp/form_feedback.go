package webapp

import (
	"strings"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

func formErrorNotice(message string) app.UI {
	if strings.TrimSpace(message) == "" {
		return app.Div().Class("form-feedback--empty")
	}
	return app.P().Class("portal-section__error").Attr("role", "alert").Body(app.Text(message))
}
