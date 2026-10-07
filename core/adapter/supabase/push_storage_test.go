package supabase

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"
)

func TestPushStorageKeepsSubscriptionsScopedToTheirOwner(t *testing.T) {
	const userID = "11111111-1111-4111-8111-111111111111"
	var objectPath string
	var stored []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer service-role" {
			t.Errorf("unexpected auth=%q", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/storage/v1/object/documentos-inovar/push/"):
			objectPath = strings.TrimPrefix(r.URL.Path, "/storage/v1/object/documentos-inovar/")
			stored, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/storage/v1/object/list/documentos-inovar":
			_, _ = w.Write([]byte(`[ {"name":"` + path.Base(objectPath) + `"} ]`))
		case r.Method == http.MethodGet && r.URL.Path == "/storage/v1/object/documentos-inovar/"+objectPath:
			_, _ = w.Write(stored)
		case r.Method == http.MethodDelete && r.URL.Path == "/storage/v1/object/documentos-inovar/"+objectPath:
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected storage request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := New(Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "service-role", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	subscription := json.RawMessage(`{"endpoint":"https://push.example.test/send/abc","keys":{"p256dh":"public","auth":"secret"}}`)
	if err := client.StorePushSubscription(t.Context(), userID, subscription); err != nil {
		t.Fatal(err)
	}
	if !validPushObjectPath(objectPath) {
		t.Fatalf("stored path=%q", objectPath)
	}
	items, err := client.ListPushSubscriptions(t.Context(), userID)
	if err != nil || len(items) != 1 || items[0].ObjectPath != objectPath {
		t.Fatalf("subscriptions=%+v err=%v", items, err)
	}
	if string(items[0].Data) != string(subscription) {
		t.Fatalf("stored subscription=%s", items[0].Data)
	}
	if err := client.DeletePushSubscription(t.Context(), objectPath); err != nil {
		t.Fatal(err)
	}
	if err := client.StorePushSubscription(t.Context(), userID, json.RawMessage(`{"endpoint":"http://127.0.0.1/secret"}`)); err == nil {
		t.Fatal("accepted insecure subscription endpoint")
	}
}

func TestNativePushTokenUsesDedicatedPrivatePathAndValidatesInput(t *testing.T) {
	const userID = "11111111-1111-4111-8111-111111111111"
	var objectPath string
	var stored []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/storage/v1/object/documentos-inovar/push-native/") {
			objectPath = strings.TrimPrefix(r.URL.Path, "/storage/v1/object/documentos-inovar/")
			stored, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/storage/v1/object/list/documentos-inovar" {
			var query struct {
				Prefix string `json:"prefix"`
			}
			_ = json.NewDecoder(r.Body).Decode(&query)
			if query.Prefix != "push-native/"+userID+"/" {
				t.Errorf("native push list prefix=%q", query.Prefix)
			}
			_, _ = w.Write([]byte(`[{"name":"` + path.Base(objectPath) + `"}]`))
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/storage/v1/object/documentos-inovar/"+objectPath {
			_, _ = w.Write(stored)
			return
		}
		if r.Method == http.MethodDelete && r.URL.Path == "/storage/v1/object/documentos-inovar/"+objectPath {
			w.WriteHeader(http.StatusOK)
			return
		}
		t.Errorf("unexpected storage request %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}))
	defer server.Close()
	client, err := New(Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "service-role", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	token := "native-device-token-1234567890"
	if err := client.StoreNativePushToken(t.Context(), userID, "android", token); err != nil {
		t.Fatal(err)
	}
	if !validNativePushObjectPath(objectPath) || strings.Contains(objectPath, token) {
		t.Fatalf("unsafe native push path %q", objectPath)
	}
	var saved map[string]string
	if err := json.Unmarshal(stored, &saved); err != nil || saved["platform"] != "android" || saved["token"] != token {
		t.Fatalf("stored token=%v err=%v", saved, err)
	}
	items, err := client.ListNativePushTokens(t.Context(), userID)
	if err != nil || len(items) != 1 || items[0].Platform != "android" || items[0].Token != token || items[0].ObjectPath != objectPath {
		t.Fatalf("native tokens=%+v err=%v", items, err)
	}
	if err := client.DeleteNativePushToken(t.Context(), userID, "android", token); err != nil {
		t.Fatal(err)
	}
	if err := client.StoreNativePushToken(t.Context(), userID, "android", "short"); err == nil {
		t.Fatal("accepted short native token")
	}
	if err := client.StoreNativePushToken(t.Context(), userID, "android", "native-token-with-newline-123\n"); err == nil {
		t.Fatal("accepted token containing a newline")
	}
}
