package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"inovarapp/core/adapter/supabase"
)

type recordedPushNotice struct{ title, body string }
type recordingTeamNotifier struct{ notices []recordedPushNotice }

func (n *recordingTeamNotifier) NotifyTeam(_ context.Context, title, body string) {
	n.notices = append(n.notices, recordedPushNotice{title: title, body: body})
}

type recordingAndroidSender struct{ tokens []string }

func (s *recordingAndroidSender) SendAndroid(_ context.Context, token, _, _, _ string) error {
	s.tokens = append(s.tokens, token)
	return nil
}

func TestTeamPushNotifierFansOutOnlyToAdminAndTechnicianTokens(t *testing.T) {
	const (
		adminID = "11111111-1111-4111-8111-111111111111"
		techID  = "22222222-2222-4222-8222-222222222222"
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer service-role" {
			t.Errorf("unexpected authorization %q", r.Header.Get("Authorization"))
		}
		switch {
		case r.URL.Path == "/rest/v1/profiles":
			if r.URL.Query().Get("tipo") != "in.(ADMIN,TECNICO)" || r.URL.Query().Get("select") != "id" {
				t.Errorf("unsafe team recipient query: %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `[{"id":"`+adminID+`"},{"id":"`+techID+`"}]`)
		case r.Method == http.MethodPost && r.URL.Path == "/storage/v1/object/list/documentos-inovar":
			var request struct {
				Prefix string `json:"prefix"`
			}
			_ = json.NewDecoder(r.Body).Decode(&request)
			if request.Prefix != "push-native/"+adminID+"/" && request.Prefix != "push-native/"+techID+"/" {
				t.Errorf("unexpected token prefix %q", request.Prefix)
				http.Error(w, "bad prefix", http.StatusBadRequest)
				return
			}
			_, _ = io.WriteString(w, `[{"name":"`+strings.Repeat("a", 64)+`.json"}]`)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/push-native/"):
			if strings.Contains(r.URL.Path, adminID) {
				_, _ = io.WriteString(w, `{"platform":"android","token":"admin-device-token-123456789"}`)
			} else if strings.Contains(r.URL.Path, techID) {
				_, _ = io.WriteString(w, `{"platform":"android","token":"tech-device-token-1234567890"}`)
			} else {
				t.Errorf("token lookup escaped team profile: %s", r.URL.Path)
				http.NotFound(w, r)
			}
		default:
			t.Errorf("unexpected Supabase request %s %s", r.Method, r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "service-role", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	sender := &recordingAndroidSender{}
	TeamPushNotifier{Supabase: client, AndroidSender: sender}.NotifyTeam(t.Context(), "Novo chamado", "Verifique a central.")
	if len(sender.tokens) != 2 || sender.tokens[0] == sender.tokens[1] {
		t.Fatalf("sent tokens=%v", sender.tokens)
	}
}
