package webpush

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSenderBuildsEncryptedWebPushRequestAndVAPIDAuth(t *testing.T) {
	clientKey, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	subscription := Subscription{Endpoint: "http://push.example.test/send"}
	subscription.Keys.P256DH = base64.RawURLEncoding.EncodeToString(clientKey.PublicKey().Bytes())
	subscription.Keys.Auth = base64.RawURLEncoding.EncodeToString(make([]byte, 16))
	validSubscription := subscription
	validSubscription.Endpoint = "https://push.example.test/send"
	if err := ValidateSubscription(validSubscription); err != nil {
		t.Fatalf("valid browser subscription: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Encoding") != "aes128gcm" || !strings.HasPrefix(r.Header.Get("Authorization"), "vapid t=") {
			t.Errorf("unexpected Web Push headers: %v", r.Header)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read encrypted body: %v", err)
		}
		if len(body) < 16+4+1+65+16 || body[20] != 65 {
			t.Errorf("invalid aes128gcm record length=%d", len(body))
		}
		if r.Header.Get("Crypto-Key") == "" {
			t.Error("VAPID public key missing")
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	subscription.Endpoint = server.URL
	sender := Sender{PrivateKey: base64.RawURLEncoding.EncodeToString(append(make([]byte, 31), 1)), Subject: "mailto:test@example.com", AllowHTTP: true, HTTPClient: server.Client()}
	if err := sender.Send(context.Background(), subscription, Payload{Title: "Aviso", Body: "Atualização", URL: "/agenda"}); err != nil {
		t.Fatal(err)
	}
}

func TestSenderDropsExpiredSubscriptionSignalAndRejectsBadEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusGone) }))
	defer server.Close()
	clientKey, _ := ecdh.P256().GenerateKey(rand.Reader)
	subscription := Subscription{Endpoint: server.URL}
	subscription.Keys.P256DH = base64.RawURLEncoding.EncodeToString(clientKey.PublicKey().Bytes())
	subscription.Keys.Auth = base64.RawURLEncoding.EncodeToString(make([]byte, 16))
	sender := Sender{PrivateKey: base64.RawURLEncoding.EncodeToString(append(make([]byte, 31), 1)), AllowHTTP: true, HTTPClient: server.Client()}
	if err := sender.Send(context.Background(), subscription, Payload{}); err != ErrSubscriptionExpired {
		t.Fatalf("expired error=%v", err)
	}
	subscription.Endpoint = "http://127.0.0.1/private"
	if err := sender.Send(context.Background(), subscription, Payload{}); err == nil {
		t.Fatal("rejected insecure endpoint")
	}
	if err := ValidateSubscription(subscription); err == nil {
		t.Fatal("accepted non-HTTPS endpoint")
	}
}
