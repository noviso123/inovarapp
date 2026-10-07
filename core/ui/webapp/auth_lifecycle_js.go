//go:build js && wasm

package webapp

import "syscall/js"

// watchAuthLifecycle refreshes Go's view of Supabase's localStorage session
// after another tab changes it and when the app becomes active again.
func watchAuthLifecycle(currentKey, legacyKey string, callback func()) func() {
	window := js.Global().Get("window")
	document := js.Global().Get("document")
	storage := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		key := args[0].Get("key")
		if key.IsNull() || key.String() == legacyKey || key.String() == currentKey {
			callback()
		}
		return nil
	})
	active := js.FuncOf(func(js.Value, []js.Value) any {
		if document.Get("visibilityState").String() == "visible" {
			callback()
		}
		return nil
	})
	window.Call("addEventListener", "storage", storage)
	window.Call("addEventListener", "focus", active)
	document.Call("addEventListener", "visibilitychange", active)
	return func() {
		window.Call("removeEventListener", "storage", storage)
		window.Call("removeEventListener", "focus", active)
		document.Call("removeEventListener", "visibilitychange", active)
		storage.Release()
		active.Release()
	}
}

func watchNotificationStorage(callback func()) func() {
	window := js.Global().Get("window")
	document := js.Global().Get("document")
	storage := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			key := args[0].Get("key")
			if !key.IsNull() && key.String() != notificationStorageKey {
				return nil
			}
		}
		callback()
		return nil
	})
	active := js.FuncOf(func(js.Value, []js.Value) any {
		if document.Get("visibilityState").String() == "visible" {
			callback()
		}
		return nil
	})
	window.Call("addEventListener", "storage", storage)
	window.Call("addEventListener", "focus", active)
	document.Call("addEventListener", "visibilitychange", active)
	return func() {
		window.Call("removeEventListener", "storage", storage)
		window.Call("removeEventListener", "focus", active)
		document.Call("removeEventListener", "visibilitychange", active)
		storage.Release()
		active.Release()
	}
}
