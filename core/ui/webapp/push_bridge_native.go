//go:build !js || !wasm

package webapp

import (
	"errors"
)

func browserPushSupported() bool         { return false }
func browserNotificationSupported() bool { return false }
func beginBrowserNotificationPermissionRequest() <-chan browserPermissionResult {
	result := make(chan browserPermissionResult, 1)
	result <- browserPermissionResult{err: errors.New("Notificações do navegador não estão disponíveis.")}
	return result
}
func beginNativePushRequest() <-chan nativePushResult {
	result := make(chan nativePushResult, 1)
	result <- nativePushResult{err: errors.New("native push is unavailable")}
	return result
}
func nativePushAvailable() bool                     { return false }
func nativePushPermissionGranted() bool             { return false }
func listenForNativePushToken(func(string, string)) {}
func requestNativePushToken() (string, string, error) {
	return "", "", errors.New("native push is unavailable")
}
func notifyWhatsAppConnected()    {}
func notifyDevice(string, string) {}
func subscribeBrowserPush(string) (string, error) {
	return "", errors.New("Notificações estão disponíveis na versão de navegador do aplicativo.")
}
func currentBrowserPushSubscription() (string, bool, error) { return "", false, nil }
func unsubscribeBrowserPush() error                         { return nil }
