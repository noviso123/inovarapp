package googlecontacts

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"inovarapp/core/adapter/supabase"
)

func TestHandlerSearchesOnlyLinkedGoogleIdentityAndNormalizesPhone(t *testing.T) {
	const userID = "11111111-1111-4111-8111-111111111111"
	linkedSub := "google-user-1"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			if r.Header.Get("Authorization") != "Bearer user-token" {
				t.Errorf("user request token=%q", r.Header.Get("Authorization"))
			}
			_, _ = io.WriteString(w, `{"id":"`+userID+`","identities":[{"provider":"google","identity_data":{"sub":"`+linkedSub+`"}}]}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"`+userID+`","tipo":"TECNICO"}]`)
		case "/identity":
			if r.Header.Get("Authorization") != "Bearer provider-token" {
				t.Errorf("identity token=%q", r.Header.Get("Authorization"))
			}
			_, _ = io.WriteString(w, `{"sub":"google-user-1"}`)
		case "/people":
			if r.URL.Query().Get("query") != "João" || r.URL.Query().Get("readMask") != "names,phoneNumbers,emailAddresses" || r.URL.Query().Get("pageSize") != "10" {
				t.Errorf("query=%v", r.URL.Query())
			}
			if r.Header.Get("Authorization") != "Bearer provider-token" {
				t.Errorf("People API token=%q", r.Header.Get("Authorization"))
			}
			_, _ = io.WriteString(w, `{"results":[{"person":{"names":[{"displayName":"João Silva"}],"phoneNumbers":[{"value":"+55 (27) 99999-0000"}],"emailAddresses":[{"value":"joao@example.com"}]}},{"person":{}}]}`)
		default:
			t.Errorf("unexpected request %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon-key", ServiceRoleKey: "service-key"})
	if err != nil {
		t.Fatal(err)
	}
	handler := Handler{Supabase: client, IdentityAPI: server.URL + "/identity", PeopleAPI: server.URL + "/people"}
	req := httptest.NewRequest(http.MethodPost, "/api/google-contacts", strings.NewReader(`{"providerToken":"provider-token","query":" João "}`))
	req.Header.Set("Authorization", "Bearer user-token")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var result struct {
		Contacts []Contact `json:"contacts"`
	}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Contacts) != 1 || result.Contacts[0] != (Contact{Name: "João Silva", Phone: "+5527999990000", Email: "joao@example.com"}) {
		t.Fatalf("contacts=%#v", result.Contacts)
	}
}

func TestHandlerRejectsUnlinkedGoogleIdentity(t *testing.T) {
	const userID = "11111111-1111-4111-8111-111111111111"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"`+userID+`","identities":[{"provider":"google","identity_data":{"sub":"linked-account"}}]}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"`+userID+`","tipo":"ADMIN"}]`)
		case "/identity":
			_, _ = io.WriteString(w, `{"sub":"different-account"}`)
		default:
			t.Errorf("unexpected request %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := supabase.New(supabase.Config{URL: server.URL, AnonKey: "anon-key", ServiceRoleKey: "service-key"})
	if err != nil {
		t.Fatal(err)
	}
	handler := Handler{Supabase: client, IdentityAPI: server.URL + "/identity", PeopleAPI: server.URL + "/people"}
	req := httptest.NewRequest(http.MethodPost, "/api/google-contacts", strings.NewReader(`{"providerToken":"other-provider-token","query":"maria"}`))
	req.Header.Set("Authorization", "Bearer user-token")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden || !strings.Contains(res.Body.String(), "vinculada") {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestNormalizePhoneMatchesLegacyContactPicker(t *testing.T) {
	for _, test := range []struct{ input, want string }{
		{" +55 (27) 99999-0000 ", "+5527999990000"},
		{"(11) 9.8765-4321 ramal 22", "1198765432122"},
	} {
		if got := normalizePhone(test.input); got != test.want {
			t.Errorf("normalizePhone(%q)=%q want %q", test.input, got, test.want)
		}
	}
}
