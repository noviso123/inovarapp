//go:build js && wasm

package webapp

import "syscall/js"

var nativeOAuthFuncs []js.Func

func nativeOAuthAvailable() bool {
	if desktopWailsAvailable() {
		return true
	}
	window := js.Global().Get("window")
	if window.Type() != js.TypeObject {
		return false
	}
	available := window.Get("inovarNativeOAuthAvailable")
	return available.Type() == js.TypeBoolean && available.Bool() && window.Get("inovarOpenOAuth").Type() == js.TypeFunction
}

func launchNativeOAuth(authorizeURL string) bool {
	if desktopOpenExternal(authorizeURL) {
		return true
	}
	window := js.Global().Get("window")
	open := window.Get("inovarOpenOAuth")
	if !nativeOAuthAvailable() || open.Type() != js.TypeFunction {
		return false
	}
	open.Invoke(authorizeURL)
	return true
}

func registerNativeOAuthCallbacks(onURL func(string), onError func()) {
	listenForDesktopOAuth(onURL)
	global := js.Global()
	urlCallback := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) > 0 {
			if callbackURL := args[0].String(); isNativeOAuthCallbackURL(callbackURL) {
				onURL(callbackURL)
			}
		}
		return nil
	})
	errorCallback := js.FuncOf(func(this js.Value, args []js.Value) any {
		onError()
		return nil
	})
	nativeOAuthFuncs = append(nativeOAuthFuncs, urlCallback, errorCallback)
	global.Set("inovarHandleOAuthURL", urlCallback)
	global.Set("inovarHandleOAuthError", errorCallback)

	window := global.Get("window")
	pendingURL := window.Get("inovarPendingOAuthURL")
	if pendingURL.Type() == js.TypeString {
		window.Set("inovarPendingOAuthURL", js.Null())
		if callbackURL := pendingURL.String(); isNativeOAuthCallbackURL(callbackURL) {
			onURL(callbackURL)
		}
	}
	pendingError := window.Get("inovarPendingOAuthError")
	if pendingError.Type() == js.TypeBoolean && pendingError.Bool() {
		window.Set("inovarPendingOAuthError", false)
		onError()
	}
}
