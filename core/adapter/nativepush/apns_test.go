package nativepush

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"strings"
	"testing"

	"inovarapp/core/adapter/push"
)

func TestAPNsSenderSignsProviderTokenAndSendsAlert(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	privateKey := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.URL.Host != "api.sandbox.push.apple.com" || request.URL.Path != "/3/device/"+strings.Repeat("ab", 32) {
			t.Errorf("APNs endpoint=%s", request.URL)
		}
		if request.Header.Get("apns-topic") != "com.inovarapp.mobile" || request.Header.Get("apns-push-type") != "alert" || request.Header.Get("apns-priority") != "10" {
			t.Errorf("APNs headers=%v", request.Header)
		}
		jwt := strings.TrimPrefix(request.Header.Get("authorization"), "bearer ")
		parts := strings.Split(jwt, ".")
		if len(parts) != 3 {
			t.Fatalf("provider JWT parts=%d", len(parts))
		}
		var header map[string]string
		headerJSON, _ := base64.RawURLEncoding.DecodeString(parts[0])
		_ = json.Unmarshal(headerJSON, &header)
		var claims map[string]any
		claimsJSON, _ := base64.RawURLEncoding.DecodeString(parts[1])
		_ = json.Unmarshal(claimsJSON, &claims)
		if header["alg"] != "ES256" || header["kid"] != "KEY123" || claims["iss"] != "TEAM123" {
			t.Fatalf("header=%v claims=%v", header, claims)
		}
		signature, _ := base64.RawURLEncoding.DecodeString(parts[2])
		if len(signature) != 64 {
			t.Fatalf("ES256 signature size=%d", len(signature))
		}
		digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		r, s := new(big.Int).SetBytes(signature[:32]), new(big.Int).SetBytes(signature[32:])
		if !ecdsa.Verify(&key.PublicKey, digest[:], r, s) {
			t.Fatal("provider JWT signature is invalid")
		}
		var payload struct {
			APS struct {
				Alert map[string]string `json:"alert"`
			} `json:"aps"`
			URL string `json:"url"`
		}
		if json.NewDecoder(request.Body).Decode(&payload) != nil || payload.APS.Alert["title"] != "Aviso" || payload.APS.Alert["body"] != "Corpo" || payload.URL != "/agenda" {
			t.Errorf("APNs payload=%+v", payload)
		}
		return testResponse(http.StatusOK, ""), nil
	})}
	sender, err := NewAPNs(APNsConfig{TeamID: "TEAM123", KeyID: "KEY123", BundleID: "com.inovarapp.mobile", PrivateKey: privateKey, Environment: "sandbox"}, client)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := sender.Send(context.Background(), strings.Repeat("ab", 32), "Aviso", "Corpo", "/agenda"); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 {
		t.Fatalf("APNs sends=%d", calls)
	}
}

func TestAPNsSenderClassifiesExpiredTokenAndValidatesDeviceToken(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return testResponse(http.StatusGone, `{"reason":"Unregistered"}`), nil
	})}
	sender, err := NewAPNs(APNsConfig{TeamID: "TEAM", KeyID: "KEY", PrivateKey: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))}, client)
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.Send(context.Background(), "not-a-device-token", "Aviso", "Corpo", "/"); err == nil {
		t.Fatal("accepted malformed APNs token")
	}
	if err := sender.Send(context.Background(), strings.Repeat("cd", 32), "Aviso", "Corpo", "/"); err != push.ErrUnregistered {
		t.Fatalf("send error=%v want ErrUnregistered", err)
	}
}

func TestAPNsConstructorRequiresAppleCredentialsAndKnownEnvironment(t *testing.T) {
	if _, err := NewAPNs(APNsConfig{TeamID: "TEAM", KeyID: "KEY", Environment: "other"}, nil); err == nil {
		t.Fatal("accepted unknown APNs environment")
	}
	if _, err := NewAPNs(APNsConfig{TeamID: "TEAM", KeyID: "KEY", PrivateKey: "invalid"}, nil); err == nil {
		t.Fatal("accepted malformed APNs key")
	}
}
