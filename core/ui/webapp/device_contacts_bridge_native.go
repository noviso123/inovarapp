//go:build !js || !wasm

package webapp

func deviceContactPickerAvailable() bool { return false }

func pickDeviceContacts() <-chan deviceContactResult {
	out := make(chan deviceContactResult, 1)
	out <- deviceContactResult{}
	return out
}
