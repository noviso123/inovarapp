//go:build js && wasm

package webapp

import (
	"encoding/base64"
	"errors"
	"syscall/js"
)

var desktopBridgeFuncs []js.Func

func desktopWailsBinding() js.Value {
	global := js.Global()
	goObject := global.Get("go")
	if goObject.Type() != js.TypeObject {
		return js.Undefined()
	}
	main := goObject.Get("main")
	if main.Type() != js.TypeObject {
		return js.Undefined()
	}
	bridge := main.Get("DesktopApp")
	if bridge.Type() != js.TypeObject {
		return js.Undefined()
	}
	return bridge
}

func desktopWailsAvailable() bool {
	bridge := desktopWailsBinding()
	window := js.Global().Get("window")
	return bridge.Type() == js.TypeObject && bridge.Get("SaveFile").Type() == js.TypeFunction &&
		window.Type() == js.TypeObject && window.Get("runtime").Type() == js.TypeObject
}

// saveDesktopFileFromBlob uses the Wails native save dialog. It returns
// handled=false outside Wails so browser/PWA downloads keep their normal path.
func saveDesktopFileFromBlob(filename string, blob js.Value) (handled bool, err error) {
	if !desktopWailsAvailable() {
		return false, nil
	}
	buffer, err := awaitJSPromise(blob.Call("arrayBuffer"))
	if err != nil {
		return true, err
	}
	bytes := js.Global().Get("Uint8Array").New(buffer)
	data := make([]byte, bytes.Get("byteLength").Int())
	if copied := js.CopyBytesToGo(data, bytes); copied != len(data) {
		return true, errors.New("não foi possível preparar o arquivo para salvar")
	}
	promise := desktopWailsBinding().Call("SaveFile", filename, base64.StdEncoding.EncodeToString(data))
	if _, err := awaitJSPromise(promise); err != nil {
		return true, err
	}
	return true, nil
}

func desktopOpenExternal(raw string) bool {
	if !desktopWailsAvailable() {
		return false
	}
	runtime := js.Global().Get("window").Get("runtime")
	open := runtime.Get("BrowserOpenURL")
	if open.Type() != js.TypeFunction {
		return false
	}
	open.Invoke(raw)
	return true
}

func desktopNativeNotify(title, body string) bool {
	if !desktopWailsAvailable() {
		return false
	}
	bridge := desktopWailsBinding()
	if bridge.Get("Notify").Type() != js.TypeFunction {
		return false
	}
	bridge.Call("Notify", title, body)
	return true
}

func beginDesktopNotificationAuthorization() <-chan desktopNotificationResult {
	result := make(chan desktopNotificationResult, 1)
	if !desktopWailsAvailable() {
		result <- desktopNotificationResult{err: errors.New("Notificações nativas indisponíveis neste aplicativo.")}
		return result
	}
	runtime := js.Global().Get("window").Get("runtime")
	availablePromise := runtime.Call("IsNotificationAvailable")
	go func() {
		allowed, err := awaitDesktopNotificationAuthorization(availablePromise)
		result <- desktopNotificationResult{allowed: allowed, err: err}
	}()
	return result
}

func awaitDesktopNotificationAuthorization(availablePromise js.Value) (bool, error) {
	runtime := js.Global().Get("window").Get("runtime")
	available, err := awaitJSPromise(availablePromise)
	if err != nil || !available.Bool() {
		if err != nil {
			return false, err
		}
		return false, errors.New("Este sistema não oferece notificações nativas.")
	}
	permission, err := awaitJSPromise(runtime.Call("CheckNotificationAuthorization"))
	if err != nil {
		return false, err
	}
	if permission.Bool() {
		return true, nil
	}
	permission, err = awaitJSPromise(runtime.Call("RequestNotificationAuthorization"))
	if err != nil {
		return false, err
	}
	return permission.Bool(), nil
}

func desktopSetClipboard(value string) js.Value {
	if !desktopWailsAvailable() {
		return js.Undefined()
	}
	set := js.Global().Get("window").Get("runtime").Get("ClipboardSetText")
	if set.Type() != js.TypeFunction {
		return js.Undefined()
	}
	return set.Invoke(value)
}

func listenForDesktopOAuth(callback func(string)) {
	if !desktopWailsAvailable() {
		return
	}
	window := js.Global().Get("window")
	runtime := window.Get("runtime")
	if runtime.Get("EventsOn").Type() == js.TypeFunction {
		listener := js.FuncOf(func(_ js.Value, args []js.Value) any {
			if len(args) > 0 && isDesktopOAuthCallbackURL(args[0].String()) {
				callback(args[0].String())
			}
			return nil
		})
		runtime.Call("EventsOn", "desktop:oauth-url", listener)
		desktopBridgeFuncs = append(desktopBridgeFuncs, listener)
	}
	method := desktopWailsBinding().Get("PendingOAuthURL")
	if method.Type() != js.TypeFunction {
		return
	}
	pending := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 && args[0].Type() == js.TypeString && isDesktopOAuthCallbackURL(args[0].String()) {
			callback(args[0].String())
		}
		return nil
	})
	rejected := js.FuncOf(func(js.Value, []js.Value) any { return nil })
	method.Invoke().Call("then", pending).Call("catch", rejected)
	desktopBridgeFuncs = append(desktopBridgeFuncs, pending, rejected)
}
