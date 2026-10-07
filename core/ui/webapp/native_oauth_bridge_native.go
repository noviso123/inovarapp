//go:build !js || !wasm

package webapp

func nativeOAuthAvailable() bool                        { return false }
func launchNativeOAuth(string) bool                     { return false }
func registerNativeOAuthCallbacks(func(string), func()) {}
