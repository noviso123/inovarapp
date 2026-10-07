package supabase

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"inovarapp/core/domain"
)

func TestUserRequestUsesCallerTokenAndAnonKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("apikey"); got != "public-anon" {
			t.Errorf("apikey = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer user-token" {
			t.Errorf("authorization = %q", got)
		}
		if got := r.Header.Get("Prefer"); got != "return=representation" {
			t.Errorf("prefer = %q", got)
		}
		if r.Method != http.MethodPatch {
			t.Errorf("method = %q", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"nome":"João"}` {
			t.Errorf("body = %s", body)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := testClient(t, server.URL, "public-anon", "secret-service")
	result, err := client.UserRequest(context.Background(), "user-token", "/rest/v1/customers?id=eq.1", RequestOptions{
		Method: http.MethodPatch,
		Body:   map[string]string{"nome": "João"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", result.StatusCode)
	}
}

func TestFromEnvAcceptsCurrentViteVariableNames(t *testing.T) {
	t.Setenv("SUPABASE_URL", "")
	t.Setenv("SUPABASE_ANON_KEY", "")
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "server-secret")
	t.Setenv("VITE_SUPABASE_URL", "https://example.supabase.co")
	t.Setenv("VITE_SUPABASE_ANON_KEY", "public-anon")

	client, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if client.baseURL.String() != "https://example.supabase.co" || client.anonKey != "public-anon" || client.serviceRoleKey != "server-secret" {
		t.Fatalf("environment mapping did not preserve current deployment configuration")
	}
}

func TestServiceRequestIsSeparateAndRequiresServerKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("apikey"); got != "server-secret" {
			t.Errorf("apikey = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer server-secret" {
			t.Errorf("authorization = %q", got)
		}
		_, _ = io.WriteString(w, `[]`)
	}))
	defer server.Close()

	client := testClient(t, server.URL, "public-anon", "server-secret")
	if _, err := client.ServiceRequest(context.Background(), "/rest/v1/customers", RequestOptions{}); err != nil {
		t.Fatal(err)
	}

	withoutServiceKey := testClient(t, server.URL, "public-anon", "")
	if _, err := withoutServiceKey.ServiceRequest(context.Background(), "/rest/v1/customers", RequestOptions{}); err != ErrNotConfigured {
		t.Fatalf("error = %v, want ErrNotConfigured", err)
	}
}

func TestAuthenticateCallerValidatesAuthAndRLSProfile(t *testing.T) {
	var profileRequest bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/auth/v1/user" {
			if r.Header.Get("Authorization") != "Bearer valid-user-token" {
				t.Errorf("auth endpoint did not receive caller token")
			}
			_, _ = io.WriteString(w, `{"id":"user-123"}`)
			return
		}
		if r.URL.Path == "/rest/v1/profiles" {
			profileRequest = true
			if r.URL.Query().Get("id") != "eq.user-123" || r.URL.Query().Get("select") != "id,tipo" {
				t.Errorf("profile query = %q", r.URL.RawQuery)
			}
			if r.Header.Get("Authorization") != "Bearer valid-user-token" {
				t.Errorf("profile query did not use caller JWT")
			}
			_ = json.NewEncoder(w).Encode([]profile{{ID: "user-123", Role: domain.RoleTechnician}})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := testClient(t, server.URL, "public-anon", "server-secret")
	caller, err := client.AuthenticateCaller(context.Background(), "valid-user-token")
	if err != nil {
		t.Fatal(err)
	}
	if !profileRequest || caller.UserID != "user-123" || caller.Role != domain.RoleTechnician || caller.Token != "valid-user-token" {
		t.Fatalf("unexpected caller: %#v; profile request: %v", caller, profileRequest)
	}
}

func TestAuthenticateCallerRejectsInvalidAuthAndMissingProfile(t *testing.T) {
	for _, tc := range []struct {
		name        string
		authStatus  int
		profileBody string
	}{
		{name: "invalid token", authStatus: http.StatusUnauthorized, profileBody: `[]`},
		{name: "no RLS-visible profile", authStatus: http.StatusOK, profileBody: `[]`},
		{name: "unknown role", authStatus: http.StatusOK, profileBody: `[{"id":"user-123","tipo":"VISITANTE"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/auth/v1/user" {
					w.WriteHeader(tc.authStatus)
					if tc.authStatus == http.StatusOK {
						_, _ = io.WriteString(w, `{"id":"user-123"}`)
					}
					return
				}
				_, _ = io.WriteString(w, tc.profileBody)
			}))
			defer server.Close()

			client := testClient(t, server.URL, "public-anon", "server-secret")
			if _, err := client.AuthenticateCaller(context.Background(), "valid-user-token"); err != ErrUnauthorized {
				t.Fatalf("error = %v, want ErrUnauthorized", err)
			}
		})
	}
}

func TestRejectsUnsafePathsAndResponseTooLarge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("x", 20))
	}))
	defer server.Close()

	client := testClient(t, server.URL, "public-anon", "server-secret")
	for _, path := range []string{"https://attacker.invalid/rest/v1/profiles", "//attacker.invalid/rest/v1/profiles", "/other/path"} {
		if _, err := client.UserRequest(context.Background(), "token", path, RequestOptions{}); err == nil {
			t.Errorf("path %q should be rejected", path)
		}
	}

	limited, err := New(Config{URL: server.URL, AnonKey: "public-anon", HTTPClient: server.Client(), MaxResponseBytes: 10})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := limited.UserRequest(context.Background(), "token", "/rest/v1/profiles", RequestOptions{}); err == nil {
		t.Fatal("expected oversized response error")
	}
}

func testClient(t *testing.T, baseURL, anonKey, serviceKey string) *Client {
	t.Helper()
	client, err := New(Config{URL: baseURL, AnonKey: anonKey, ServiceRoleKey: serviceKey})
	if err != nil {
		t.Fatal(err)
	}
	return client
}
