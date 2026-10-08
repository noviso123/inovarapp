//go:build !js || !wasm

package webapp

func prepareQRCodeDownload(_ []byte, fallback string) string { return fallback }
func releaseQRCodeDownload(string)                           {}
