//go:build !js || !wasm

package webapp

func desktopWailsAvailable() bool { return false }

func beginDesktopNotificationAuthorization() <-chan desktopNotificationResult {
	result := make(chan desktopNotificationResult, 1)
	result <- desktopNotificationResult{err: nil}
	return result
}
