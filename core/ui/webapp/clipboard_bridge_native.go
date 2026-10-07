//go:build !js || !wasm

package webapp

import "github.com/maxence-charriere/go-app/v11/pkg/app"

func copyTextToClipboard(value string) app.EventHandler {
	return func(app.Context, app.Event) {}
}

func copyTextToClipboardNotice(value string, onResult func(app.Context, bool)) app.EventHandler {
	return func(ctx app.Context, event app.Event) { event.PreventDefault(); onResult(ctx, false); ctx.Update() }
}
