//go:build js && wasm

package webapp

import (
	"encoding/base64"
	"errors"
	"syscall/js"
)

func browserPushSupported() bool {
	navigator := js.Global().Get("navigator")
	window := js.Global().Get("window")
	return navigator.Type() == js.TypeObject && navigator.Get("serviceWorker").Type() == js.TypeObject &&
		window.Type() == js.TypeObject && window.Get("PushManager").Type() == js.TypeFunction &&
		js.Global().Get("Notification").Type() == js.TypeFunction
}

func browserNotificationSupported() bool {
	return js.Global().Get("Notification").Type() == js.TypeFunction
}

func beginBrowserNotificationPermissionRequest() <-chan browserPermissionResult {
	result := make(chan browserPermissionResult, 1)
	if !browserNotificationSupported() {
		result <- browserPermissionResult{err: errors.New("Este navegador não oferece notificações do sistema.")}
		return result
	}
	promise := js.Global().Get("Notification").Call("requestPermission")
	go func() {
		permission, err := awaitJSPromise(promise)
		if err != nil {
			result <- browserPermissionResult{err: errors.New("Não foi possível solicitar permissão para notificações.")}
			return
		}
		result <- browserPermissionResult{permission: permission.String()}
	}()
	return result
}

func beginNativePushRequest() <-chan nativePushResult {
	result := make(chan nativePushResult, 1)
	window := js.Global().Get("window")
	if window.Type() != js.TypeObject || window.Get("inovarRequestNativePush").Type() != js.TypeFunction {
		result <- nativePushResult{err: errors.New("native push bridge unavailable")}
		return result
	}
	promise := window.Call("inovarRequestNativePush")
	go func() {
		value, err := awaitJSPromise(promise)
		if err != nil {
			result <- nativePushResult{err: err}
			return
		}
		result <- nativePushResult{platform: value.Get("platform").String(), token: value.Get("token").String()}
	}()
	return result
}

func nativePushAvailable() bool {
	window := js.Global().Get("window")
	return window.Type() == js.TypeObject && window.Get("inovarNativePushAvailable").Type() == js.TypeBoolean && window.Get("inovarNativePushAvailable").Bool()
}

func nativePushPermissionGranted() bool {
	window := js.Global().Get("window")
	return window.Type() == js.TypeObject && window.Get("inovarNativePushPermitted").Type() == js.TypeBoolean && window.Get("inovarNativePushPermitted").Bool()
}

func requestNativePushToken() (string, string, error) {
	window := js.Global().Get("window")
	if window.Type() != js.TypeObject || window.Get("inovarRequestNativePush").Type() != js.TypeFunction {
		return "", "", errors.New("native push bridge unavailable")
	}
	ready := window.Get("inovarNativePushReady")
	if ready.Type() == js.TypeObject {
		if _, err := awaitJSPromise(ready); err != nil {
			return "", "", err
		}
	}
	value, err := awaitJSPromise(window.Call("inovarRequestNativePush"))
	if err != nil {
		return "", "", err
	}
	return value.Get("platform").String(), value.Get("token").String(), nil
}

func listenForNativePushToken(callback func(string, string)) {
	window := js.Global().Get("window")
	if window.Type() != js.TypeObject {
		return
	}
	fn := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) < 2 {
			return nil
		}
		platform, token := args[0].String(), args[1].String()
		if platform != "ios" && platform != "android" || token == "" {
			return nil
		}
		callback(platform, token)
		return nil
	})
	window.Set("inovarOnNativePushToken", fn)
	value := window.Get("inovarNativePushToken")
	if value.Type() == js.TypeObject {
		fn.Invoke(value.Get("platform"), value.Get("token"))
	}
}

func notifyWhatsAppConnected() {
	notifyDevice("InovarApp", "WhatsApp conectado e salvo! Disparos automáticos ativos.")
}

func notifyDevice(title, body string) {
	if desktopNativeNotify(title, body) {
		return
	}
	notification := js.Global().Get("Notification")
	if notification.Type() != js.TypeFunction || notification.Get("permission").String() != "granted" {
		return
	}
	options := js.Global().Get("Object").New()
	options.Set("body", body)
	defer func() { _ = recover() }()
	js.Global().Get("Notification").New(title, options)
}

func subscribeBrowserPush(publicKey string) (string, error) {
	if !browserPushSupported() {
		return "", errors.New("Este navegador ainda não oferece notificações para o aplicativo.")
	}
	if publicKey == "" {
		return "", errors.New("Notificações ainda não estão configuradas neste ambiente.")
	}
	permission, err := awaitJSPromise(js.Global().Get("Notification").Call("requestPermission"))
	if err != nil {
		return "", errors.New("Não foi possível solicitar permissão para notificações.")
	}
	if permission.String() != "granted" {
		return "", errors.New("Permissão de notificações não concedida. Você pode ativá-la nas configurações do navegador.")
	}
	key, err := base64.RawURLEncoding.DecodeString(publicKey)
	if err != nil {
		return "", errors.New("A chave pública de notificações está inválida.")
	}
	registration, err := awaitJSPromise(js.Global().Get("navigator").Get("serviceWorker").Get("ready"))
	if err != nil {
		return "", errors.New("O serviço offline ainda não está pronto. Tente novamente em instantes.")
	}
	applicationKey := js.Global().Get("Uint8Array").New(len(key))
	js.CopyBytesToJS(applicationKey, key)
	options := js.Global().Get("Object").New()
	options.Set("userVisibleOnly", true)
	options.Set("applicationServerKey", applicationKey)
	subscription, err := awaitJSPromise(registration.Get("pushManager").Call("subscribe", options))
	if err != nil {
		return "", errors.New("Não foi possível registrar este dispositivo para notificações.")
	}
	return js.Global().Get("JSON").Call("stringify", subscription.Call("toJSON")).String(), nil
}

func currentBrowserPushSubscription() (string, bool, error) {
	if !browserPushSupported() {
		return "", false, errors.New("Este navegador ainda não oferece notificações para o aplicativo.")
	}
	registration, err := awaitJSPromise(js.Global().Get("navigator").Get("serviceWorker").Get("ready"))
	if err != nil {
		return "", false, err
	}
	subscription, err := awaitJSPromise(registration.Get("pushManager").Call("getSubscription"))
	if err != nil {
		return "", false, err
	}
	if subscription.IsNull() || subscription.IsUndefined() {
		return "", false, nil
	}
	return js.Global().Get("JSON").Call("stringify", subscription.Call("toJSON")).String(), true, nil
}

func unsubscribeBrowserPush() error {
	if !browserPushSupported() {
		return nil
	}
	registration, err := awaitJSPromise(js.Global().Get("navigator").Get("serviceWorker").Get("ready"))
	if err != nil {
		return err
	}
	subscription, err := awaitJSPromise(registration.Get("pushManager").Call("getSubscription"))
	if err != nil || subscription.IsNull() || subscription.IsUndefined() {
		return err
	}
	unsubscribed, err := awaitJSPromise(subscription.Call("unsubscribe"))
	if err != nil {
		return err
	}
	if !unsubscribed.Bool() {
		return errors.New("browser did not remove the push subscription")
	}
	return nil
}

func awaitJSPromise(promise js.Value) (js.Value, error) {
	type result struct {
		value js.Value
		err   error
	}
	done := make(chan result, 1)
	resolve := js.FuncOf(func(_ js.Value, args []js.Value) any {
		value := js.Undefined()
		if len(args) > 0 {
			value = args[0]
		}
		done <- result{value: value}
		return nil
	})
	reject := js.FuncOf(func(_ js.Value, args []js.Value) any {
		message := "browser promise rejected"
		if len(args) > 0 {
			if args[0].Type() == js.TypeObject && args[0].Get("message").Type() == js.TypeString {
				message = args[0].Get("message").String()
			} else if args[0].Type() == js.TypeString {
				message = args[0].String()
			}
		}
		done <- result{err: errors.New(message)}
		return nil
	})
	promise.Call("then", resolve).Call("catch", reject)
	resultValue := <-done
	resolve.Release()
	reject.Release()
	return resultValue.value, resultValue.err
}
