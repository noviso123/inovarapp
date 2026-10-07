package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"inovarapp/core/adapter/push"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/adapter/webpush"
)

func TestNotificationsHandlerChecksAndRemovesOnlyTheAuthenticatedUsersSubscription(t *testing.T) {
	const userID = "11111111-1111-4111-8111-111111111111"
	const endpoint = "https://push.example.test/send/unique"
	objectExists := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/auth/v1/user":
			if r.Header.Get("Authorization") != "Bearer caller-token" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"id":"` + userID + `"}`))
		case r.URL.Path == "/rest/v1/profiles":
			_, _ = w.Write([]byte(`[{"id":"` + userID + `","tipo":"CLIENTE"}]`))
		case strings.HasPrefix(r.URL.Path, "/storage/v1/object/documentos-inovar/push/"):
			if !strings.Contains(r.URL.Path, "/push/"+userID+"/") {
				t.Errorf("subscription escaped account namespace: %q", r.URL.Path)
			}
			if r.Method == http.MethodDelete {
				objectExists = false
				w.WriteHeader(http.StatusOK)
				return
			}
			if objectExists {
				_, _ = w.Write([]byte(`{"subscription":{}}`))
				return
			}
			http.NotFound(w, r)
		default:
			t.Errorf("unexpected Supabase request: %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "service-role", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	subscription, _ := json.Marshal(webpush.Subscription{Endpoint: endpoint})
	handler := NotificationsHandler{Supabase: client}
	call := func(action string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"acao": action, "subscription": json.RawMessage(subscription)})
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/notificacoes", strings.NewReader(string(body)))
		request.Header.Set("Authorization", "Bearer caller-token")
		handler.ServeHTTP(recorder, request)
		return recorder
	}
	state := call("estado")
	if state.Code != http.StatusOK || !strings.Contains(state.Body.String(), `"active":true`) {
		t.Fatalf("state=%d %s", state.Code, state.Body.String())
	}
	removed := call("remover")
	if removed.Code != http.StatusOK {
		t.Fatalf("remove=%d %s", removed.Code, removed.Body.String())
	}
	state = call("estado")
	if state.Code != http.StatusOK || !strings.Contains(state.Body.String(), `"active":false`) {
		t.Fatalf("state after removal=%d %s", state.Code, state.Body.String())
	}
}

type fakeNativePushSender struct {
	sent []string
}

func (s *fakeNativePushSender) SendAndroid(_ context.Context, token, _, _, _ string) error {
	s.sent = append(s.sent, token)
	if token == "stale-android-token-123456" {
		return push.ErrUnregistered
	}
	return nil
}

type fakeIOSPushSender struct {
	sent []string
}

func (s *fakeIOSPushSender) Send(_ context.Context, token, _, _, _ string) error {
	s.sent = append(s.sent, token)
	return push.ErrUnregistered
}

func TestNotificationsRouteNativePushByPlatformAndDeleteExpiredTokens(t *testing.T) {
	const userID = "11111111-1111-4111-8111-111111111111"
	objects := map[string][]byte{}
	deleted := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/auth/v1/user":
			_, _ = w.Write([]byte(`{"id":"` + userID + `"}`))
		case r.URL.Path == "/rest/v1/profiles":
			_, _ = w.Write([]byte(`[{"id":"` + userID + `","tipo":"ADMIN"}]`))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/storage/v1/object/documentos-inovar/push-native/"):
			path := strings.TrimPrefix(r.URL.Path, "/storage/v1/object/documentos-inovar/")
			body, _ := io.ReadAll(r.Body)
			objects[path] = body
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/storage/v1/object/list/documentos-inovar":
			var input struct {
				Prefix string `json:"prefix"`
			}
			_ = json.NewDecoder(r.Body).Decode(&input)
			if input.Prefix != "push-native/"+userID+"/" {
				t.Errorf("token list escaped account prefix: %q", input.Prefix)
			}
			names := make([]map[string]string, 0, len(objects))
			for path := range objects {
				names = append(names, map[string]string{"name": path[strings.LastIndex(path, "/")+1:]})
			}
			_ = json.NewEncoder(w).Encode(names)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/storage/v1/object/documentos-inovar/push-native/"):
			path := strings.TrimPrefix(r.URL.Path, "/storage/v1/object/documentos-inovar/")
			if body, ok := objects[path]; ok {
				_, _ = w.Write(body)
			} else {
				http.NotFound(w, r)
			}
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/storage/v1/object/documentos-inovar/push-native/"):
			path := strings.TrimPrefix(r.URL.Path, "/storage/v1/object/documentos-inovar/")
			delete(objects, path)
			deleted++
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected Supabase request: %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "service-role", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ platform, token string }{
		{"android", "good-android-device-token-123456"},
		{"android", "stale-android-token-123456"},
		{"ios", strings.Repeat("ab", 32)},
	} {
		if err := client.StoreNativePushToken(t.Context(), userID, item.platform, item.token); err != nil {
			t.Fatal(err)
		}
	}
	nativeSender := &fakeNativePushSender{}
	iosSender := &fakeIOSPushSender{}
	handler := NotificationsHandler{Supabase: client, AndroidSender: nativeSender, IOSSender: iosSender}
	body := `{"acao":"enviar","titulo":"Aviso","texto":"Corpo","url":"/agenda"}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/notificacoes", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer caller-token")
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("send=%d %s", recorder.Code, recorder.Body.String())
	}
	if len(nativeSender.sent) != 2 {
		t.Fatalf("FCM received tokens=%v, want Android tokens only", nativeSender.sent)
	}
	if len(iosSender.sent) != 1 || iosSender.sent[0] != strings.Repeat("ab", 32) {
		t.Fatalf("APNs received tokens=%v, want the iOS device token only", iosSender.sent)
	}
	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["nativeSent"] != float64(1) || response["nativeExpired"] != float64(2) || deleted != 2 {
		t.Fatalf("response=%v deleted=%d", response, deleted)
	}
}
