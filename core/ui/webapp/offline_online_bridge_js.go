//go:build js && wasm

package webapp

import "syscall/js"

var offlineOnlineCallback js.Func
var offlineOnlineCallbackRegistered bool

func registerOfflineOnlineCallback(callback func()) {
	window := js.Global().Get("window")
	if window.IsUndefined() || window.IsNull() {
		return
	}
	if offlineOnlineCallbackRegistered {
		window.Call("removeEventListener", "online", offlineOnlineCallback)
		offlineOnlineCallback.Release()
	}
	offlineOnlineCallback = js.FuncOf(func(this js.Value, args []js.Value) any {
		go callback()
		return nil
	})
	offlineOnlineCallbackRegistered = true
	window.Call("addEventListener", "online", offlineOnlineCallback)
}
