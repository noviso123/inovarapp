//go:build js && wasm

package webapp

import "syscall/js"

// Prepare the file before the user taps, preserving Safari's download gesture.
func prepareQRCodeDownload(data []byte, fallback string) (result string) {
	result = fallback
	defer func() { _ = recover() }()
	buffer := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(buffer, data)
	blob := js.Global().Get("Blob").New([]any{buffer}, map[string]any{"type": "image/png"})
	return js.Global().Get("URL").Call("createObjectURL", blob).String()
}

func releaseQRCodeDownload(value string) {
	if len(value) > 5 && value[:5] == "blob:" {
		js.Global().Get("URL").Call("revokeObjectURL", value)
	}
}
