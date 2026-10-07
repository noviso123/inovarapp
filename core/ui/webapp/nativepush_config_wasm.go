//go:build js && wasm

package webapp

import "inovarapp/core/adapter/httpapi"

func configuredNativePushSenders() (httpapi.AndroidPushSender, httpapi.IOSPushSender) {
	return nil, nil
}
