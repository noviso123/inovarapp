package email

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestResendSenderPreservesUTF8AndUsesServerCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emails" || r.Method != http.MethodPost {
			t.Errorf("request=%s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer resend-secret" {
			t.Errorf("authorization leaked or missing: %q", r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["from"] != "Inovar <inovar@example.com>" || body["subject"] != "Olá — João" || body["html"] != "<p>❄️ manutenção</p>" {
			t.Errorf("body=%#v", body)
		}
		_, _ = w.Write([]byte(`{"id":"mail-123"}`))
	}))
	defer server.Close()
	// ResendSender intentionally targets the official API. A test-only transport
	// redirects that request to the local fixture without allowing real network IO.
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r.URL.Scheme = "http"
		r.URL.Host = strings.TrimPrefix(server.URL, "http://")
		return http.DefaultTransport.RoundTrip(r)
	})
	sender := ResendSender{Client: &http.Client{Transport: transport}}
	id, err := sender.Send(context.Background(), Config{APIKey: "resend-secret", From: "Inovar <inovar@example.com>"}, Message{To: "cliente@example.com", Subject: "Olá — João", HTML: "<p>❄️ manutenção</p>"})
	if err != nil || id != "mail-123" {
		t.Fatalf("id=%q error=%v", id, err)
	}
}
