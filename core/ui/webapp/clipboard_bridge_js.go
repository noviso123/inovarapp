//go:build js && wasm

package webapp

import (
	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"syscall/js"
)

func copyTextToClipboard(value string) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		clipboard := js.Global().Get("navigator").Get("clipboard")
		if clipboard.IsUndefined() || clipboard.IsNull() {
			promise := desktopSetClipboard(value)
			if promise.Type() == js.TypeObject && promise.Get("then").Type() == js.TypeFunction {
				onError := js.FuncOf(func(js.Value, []js.Value) any { return nil })
				desktopBridgeFuncs = append(desktopBridgeFuncs, onError)
				promise.Call("catch", onError)
			}
			return
		}
		clipboard.Call("writeText", value)
	}
}

func copyTextToClipboardNotice(value string, onResult func(app.Context, bool)) app.EventHandler {
	return func(ctx app.Context, event app.Event) {
		event.PreventDefault()
		clipboard := js.Global().Get("navigator").Get("clipboard")
		if clipboard.IsUndefined() || clipboard.IsNull() {
			promise := desktopSetClipboard(value)
			if promise.Type() != js.TypeObject || promise.Get("then").Type() != js.TypeFunction {
				onResult(ctx, false)
				ctx.Update()
				return
			}
			var resolve, reject js.Func
			resolve = js.FuncOf(func(js.Value, []js.Value) any {
				onResult(ctx, true)
				ctx.Update()
				resolve.Release()
				reject.Release()
				return nil
			})
			reject = js.FuncOf(func(js.Value, []js.Value) any {
				onResult(ctx, false)
				ctx.Update()
				resolve.Release()
				reject.Release()
				return nil
			})
			promise.Call("then", resolve).Call("catch", reject)
			return
		}
		promise := clipboard.Call("writeText", value)
		var resolve, reject js.Func
		resolve = js.FuncOf(func(js.Value, []js.Value) any {
			onResult(ctx, true)
			ctx.Update()
			resolve.Release()
			reject.Release()
			return nil
		})
		reject = js.FuncOf(func(js.Value, []js.Value) any {
			onResult(ctx, false)
			ctx.Update()
			resolve.Release()
			reject.Release()
			return nil
		})
		promise.Call("then", resolve).Call("catch", reject)
	}
}
