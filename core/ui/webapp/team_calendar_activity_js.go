//go:build js && wasm

package webapp

import "syscall/js"

func watchTeamCalendarActivity(callback func()) func() {
	document := js.Global().Get("document")
	window := js.Global().Get("window")
	navigator := js.Global().Get("navigator")
	listener := js.FuncOf(func(js.Value, []js.Value) any {
		if document.Get("visibilityState").String() == "visible" && navigator.Get("onLine").Bool() {
			callback()
		}
		return nil
	})
	document.Call("addEventListener", "visibilitychange", listener)
	window.Call("addEventListener", "focus", listener)
	return func() {
		document.Call("removeEventListener", "visibilitychange", listener)
		window.Call("removeEventListener", "focus", listener)
		listener.Release()
	}
}
