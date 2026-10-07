package nativepush

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"inovarapp/core/adapter/push"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestFCMSenderExchangesAndCachesSignedServiceAccountToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	privateKey, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	credentials, _ := json.Marshal(ServiceAccount{
		ProjectID: "inovar-test", ClientEmail: "push@example.test",
		PrivateKey: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKey})),
		TokenURI:   "https://oauth.example.test/token",
	})
	oauthCalls, fcmCalls := 0, 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Host {
		case "oauth.example.test":
			oauthCalls++
			body, _ := io.ReadAll(request.Body)
			form, _ := url.ParseQuery(string(body))
			if form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
				t.Fatalf("grant_type=%q", form.Get("grant_type"))
			}
			parts := strings.Split(form.Get("assertion"), ".")
			if len(parts) != 3 {
				t.Fatalf("JWT assertion parts=%d", len(parts))
			}
			claimsJSON, _ := base64.RawURLEncoding.DecodeString(parts[1])
			var claims map[string]any
			if json.Unmarshal(claimsJSON, &claims) != nil || claims["iss"] != "push@example.test" || claims["scope"] != fcmScope {
				t.Fatalf("JWT claims=%s", claimsJSON)
			}
			unsigned := parts[0] + "." + parts[1]
			digest := sha256.Sum256([]byte(unsigned))
			signature, _ := base64.RawURLEncoding.DecodeString(parts[2])
			if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], signature); err != nil {
				t.Fatalf("invalid JWT signature: %v", err)
			}
			return testResponse(http.StatusOK, `{"access_token":"oauth-access","expires_in":3600}`), nil
		case "fcm.googleapis.com":
			fcmCalls++
			if request.Header.Get("Authorization") != "Bearer oauth-access" {
				t.Errorf("authorization=%q", request.Header.Get("Authorization"))
			}
			var message struct {
				Message struct {
					Token        string            `json:"token"`
					Notification map[string]string `json:"notification"`
					Data         map[string]string `json:"data"`
				} `json:"message"`
			}
			if json.NewDecoder(request.Body).Decode(&message) != nil || message.Message.Token != "device-token" || message.Message.Notification["title"] != "Aviso" || message.Message.Data["url"] != "/agenda" {
				t.Errorf("message=%+v", message)
			}
			return testResponse(http.StatusOK, `{"name":"projects/inovar-test/messages/123"}`), nil
		default:
			t.Fatalf("unexpected host %q", request.URL.Host)
			return nil, nil
		}
	})}
	sender, err := New(credentials, client)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := sender.SendAndroid(context.Background(), "device-token", "Aviso", "Corpo", "/agenda"); err != nil {
			t.Fatal(err)
		}
	}
	if oauthCalls != 1 || fcmCalls != 2 {
		t.Fatalf("OAuth calls=%d FCM calls=%d", oauthCalls, fcmCalls)
	}
}

func TestFCMSenderClassifiesUnregisteredToken(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	privateKey, _ := x509.MarshalPKCS8PrivateKey(key)
	credentials, _ := json.Marshal(ServiceAccount{ProjectID: "test", ClientEmail: "push@example.test", PrivateKey: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKey})), TokenURI: "https://oauth.example.test/token"})
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "oauth.example.test" {
			return testResponse(http.StatusOK, `{"access_token":"token","expires_in":3600}`), nil
		}
		return testResponse(http.StatusNotFound, `{"error":{"details":[{"errorCode":"UNREGISTERED"}]}}`), nil
	})}
	sender, err := New(credentials, client)
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.SendAndroid(context.Background(), "stale-token", "Aviso", "Corpo", "/"); err != push.ErrUnregistered {
		t.Fatalf("send error=%v want ErrUnregistered", err)
	}
}

func testResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
