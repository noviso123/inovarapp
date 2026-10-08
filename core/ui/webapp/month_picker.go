package webapp

import (
	"fmt"
	"time"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

// A regular select avoids Safari's intrinsic width for input[type=month].
func (p *serviceCatalogPage) compactMonthPicker(value string, destination *string) app.UI {
	year := time.Now().Year()
	selected, err := time.Parse("2006-01", value)
	first, last := year-10, year+5
	if err == nil {
		if selected.Year() < first {
			first = selected.Year()
		}
		if selected.Year() > last {
			last = selected.Year()
		}
	}
	names := []string{"Jan", "Fev", "Mar", "Abr", "Mai", "Jun", "Jul", "Ago", "Set", "Out", "Nov", "Dez"}
	options := []app.UI{}
	for y := last; y >= first; y-- {
		for m := 12; m >= 1; m-- {
			key := fmt.Sprintf("%04d-%02d", y, m)
			options = append(options, app.Option().Value(key).Selected(key == value).Body(app.Text(fmt.Sprintf("%s / %d", names[m-1], y))))
		}
	}
	return app.Select().Class("team-month-picker").Attr("value", value).
		Style("box-sizing", "border-box").Style("width", "100%").Style("max-width", "100%").Style("min-width", "0").
		Style("height", "46px").Style("min-height", "46px").Style("font-size", "16px").Style("padding", "8px 12px").
		OnChange(p.ValueTo(destination)).Body(options...)
}
