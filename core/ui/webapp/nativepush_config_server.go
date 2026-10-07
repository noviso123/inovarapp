//go:build !js || !wasm

package webapp

import (
	"log"
	"sync"

	"inovarapp/core/adapter/httpapi"
	"inovarapp/core/adapter/nativepush"
)

var platformPushSenderOnce sync.Once
var configuredAndroidSender httpapi.AndroidPushSender
var configuredIOSSender httpapi.IOSPushSender

func configuredNativePushSenders() (httpapi.AndroidPushSender, httpapi.IOSPushSender) {
	platformPushSenderOnce.Do(func() {
		if credentials := configuredValue("FIREBASE_SERVICE_ACCOUNT_JSON"); credentials != "" {
			sender, err := nativepush.New([]byte(credentials), nil)
			if err != nil {
				log.Printf("Android push disabled: invalid Firebase service account (%v)", err)
			} else {
				configuredAndroidSender = sender
			}
		}
		if configuredValue("APNS_TEAM_ID") != "" || configuredValue("APNS_KEY_ID") != "" || configuredValue("APNS_AUTH_KEY") != "" {
			sender, err := nativepush.NewAPNs(nativepush.APNsConfig{
				TeamID: configuredValue("APNS_TEAM_ID"), KeyID: configuredValue("APNS_KEY_ID"),
				BundleID: configuredValue("APNS_BUNDLE_ID"), PrivateKey: configuredValue("APNS_AUTH_KEY"),
				Environment: configuredValue("APNS_ENVIRONMENT"),
			}, nil)
			if err != nil {
				log.Printf("iOS push disabled: invalid APNs configuration (%v)", err)
			} else {
				configuredIOSSender = sender
			}
		}
	})
	return configuredAndroidSender, configuredIOSSender
}
