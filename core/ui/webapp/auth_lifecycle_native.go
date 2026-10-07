//go:build !js || !wasm

package webapp

func watchAuthLifecycle(string, string, func()) func() { return func() {} }

func watchNotificationStorage(func()) func() { return func() {} }
