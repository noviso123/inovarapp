package supabase

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSignInWithPasswordUsesAnonKeyAndReturnsCompatibleSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/auth/v1/token" || r.URL.Query().Get("grant_type") != "password" {
			t.Errorf("request = %s %s", r.Method, r.URL.String())
		}
		if got := r.Header.Get("apikey"); got != "public-anon" {
			t.Errorf("apikey = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer public-anon" {
			t.Errorf("authorization = %q", got)
		}
		if got := r.Header.Get("X-Supabase-Api-Version"); got != authAPIVersion {
			t.Errorf("auth API version = %q", got)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["email"] != "tecnico@example.com" || body["password"] != "senha-segura" {
			t.Errorf("sign-in body = %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"access-1","refresh_token":"refresh-1","token_type":"bearer","expires_in":3600,"provider_token":"google-access","provider_refresh_token":"google-refresh","session_extension":{"kept":true},"user":{"id":"user-1","email":"tecnico@example.com","user_metadata":{"must_change_password":true},"identities":[{"provider":"google","identity_id":"identity-1"}],"aud":"authenticated"}}`)
	}))
	defer server.Close()

	client := testClient(t, server.URL, "public-anon", "secret-service")
	session, err := client.SignInWithPassword(context.Background(), " tecnico@example.com ", "senha-segura")
	if err != nil {
		t.Fatal(err)
	}
	if session.AccessToken != "access-1" || session.RefreshToken != "refresh-1" || session.User == nil || session.User.ID != "user-1" {
		t.Fatalf("unexpected session: %#v", session)
	}
	if raw := string(session.User.UserMetadata["must_change_password"]); raw != "true" {
		t.Fatalf("user metadata was not preserved: %s", raw)
	}
	if session.ProviderToken != "google-access" || session.ProviderRefreshToken != "google-refresh" {
		t.Fatalf("provider tokens were not preserved: %#v", session)
	}
	encoded, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &stored); err != nil {
		t.Fatal(err)
	}
	var storedUser map[string]json.RawMessage
	if err := json.Unmarshal(stored["user"], &storedUser); err != nil {
		t.Fatal(err)
	}
	if _, ok := stored["session_extension"]; !ok {
		t.Fatalf("unknown session field was dropped: %s", encoded)
	}
	if _, ok := storedUser["identities"]; !ok {
		t.Fatalf("user identities were dropped: %s", stored["user"])
	}
	if string(storedUser["aud"]) != `"authenticated"` {
		t.Fatalf("user audience was dropped: %s", stored["user"])
	}
	now := time.Now().Unix()
	if session.ExpiresAt < now+3598 || session.ExpiresAt > now+3601 {
		t.Fatalf("derived expires_at = %d, now = %d", session.ExpiresAt, now)
	}
}

func TestPublicAuthRequestDoesNotUsePublishableKeyAsBearer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("apikey"); got != "sb_publishable_test" {
			t.Errorf("apikey = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("publishable API key must not be sent as a bearer token; authorization = %q", got)
		}
		_, _ = io.WriteString(w, `{"access_token":"access","refresh_token":"refresh","token_type":"bearer","expires_in":3600,"user":{"id":"user-1"}}`)
	}))
	defer server.Close()

	client := testClient(t, server.URL, "sb_publishable_test", "sb_secret_test")
	if _, err := client.SignInWithPassword(context.Background(), "user@example.com", "password"); err != nil {
		t.Fatal(err)
	}
}

func TestSignInWithPasswordPreservesSupabaseAuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"code":"invalid_credentials","msg":"Invalid login credentials"}`)
	}))
	defer server.Close()

	client := testClient(t, server.URL, "public-anon", "")
	_, err := client.SignInWithPassword(context.Background(), "a@example.com", "wrong")
	var authErr *AuthError
	if !errors.As(err, &authErr) || authErr.StatusCode != http.StatusUnauthorized || authErr.Code != "invalid_credentials" || authErr.Message != "Invalid login credentials" {
		t.Fatalf("error = %#v", err)
	}
}

func TestAuthSessionRoundTripsSupabaseJSLocalStorageShape(t *testing.T) {
	const fixture = `{"access_token":"access-token","token_type":"bearer","expires_in":3600,"expires_at":1791148800,"refresh_token":"refresh-token","provider_token":"google-token","provider_refresh_token":"google-refresh","user":{"id":"user-1","aud":"authenticated","role":"authenticated","email":"cliente@example.com","app_metadata":{"provider":"google","providers":["google"]},"user_metadata":{"must_change_password":false,"nome":"Cliente UTF-8"},"identities":[{"provider":"google","identity_data":{"email":"cliente@example.com"}}],"created_at":"2026-10-04T12:00:00Z","updated_at":"2026-10-04T12:00:00Z"}}`
	var session AuthSession
	if err := json.Unmarshal([]byte(fixture), &session); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip map[string]any
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"access_token", "token_type", "expires_in", "expires_at", "refresh_token", "provider_token", "provider_refresh_token", "user"} {
		if _, ok := roundTrip[key]; !ok {
			t.Errorf("Supabase JS session field %q was lost: %s", key, encoded)
		}
	}
	user, ok := roundTrip["user"].(map[string]any)
	if !ok {
		t.Fatalf("user field type = %T", roundTrip["user"])
	}
	if user["aud"] != "authenticated" || user["created_at"] != "2026-10-04T12:00:00Z" {
		t.Errorf("unknown user fields were not preserved: %#v", user)
	}
	if _, ok := user["identities"].([]any); !ok {
		t.Errorf("Supabase identity list was not preserved: %#v", user["identities"])
	}
	metadata, ok := user["user_metadata"].(map[string]any)
	if !ok || metadata["must_change_password"] != false || metadata["nome"] != "Cliente UTF-8" {
		t.Errorf("user metadata was not preserved: %#v", user["user_metadata"])
	}
}

func TestRefreshSessionRotatesRefreshTokenAndAllowsOmittedUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("grant_type") != "refresh_token" {
			t.Errorf("grant_type = %q", r.URL.Query().Get("grant_type"))
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["refresh_token"] != "refresh-old" {
			t.Errorf("refresh token = %q", body["refresh_token"])
		}
		_, _ = io.WriteString(w, `{"access_token":"access-2","refresh_token":"refresh-new","expires_in":1800}`)
	}))
	defer server.Close()

	client := testClient(t, server.URL, "public-anon", "")
	session, err := client.RefreshSession(context.Background(), AuthSession{RefreshToken: "refresh-old"})
	if err != nil {
		t.Fatal(err)
	}
	if session.AccessToken != "access-2" || session.RefreshToken != "refresh-new" || session.User != nil || session.ExpiresIn != 1800 {
		t.Fatalf("unexpected refreshed session: %#v", session)
	}
}

func TestRefreshSessionRetainsOldTokenWhenServerOmitsReplacement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"access_token":"access-2","expires_in":1800}`)
	}))
	defer server.Close()

	client := testClient(t, server.URL, "public-anon", "")
	previous := AuthSession{
		RefreshToken:         "refresh-current",
		ProviderToken:        "google-access",
		ProviderRefreshToken: "google-refresh",
		User: &AuthUser{
			ID:           "user-1",
			UserMetadata: map[string]json.RawMessage{"must_change_password": json.RawMessage("true")},
			extraFields:  map[string]json.RawMessage{"identities": json.RawMessage(`[{"provider":"google"}]`)},
		},
		extraFields: map[string]json.RawMessage{"session_extension": json.RawMessage(`{"kept":true}`)},
	}
	session, err := client.RefreshSession(context.Background(), previous)
	if err != nil {
		t.Fatal(err)
	}
	if session.RefreshToken != "refresh-current" || session.ProviderToken != "google-access" || session.ProviderRefreshToken != "google-refresh" {
		t.Fatalf("refreshed session lost existing credentials: %#v", session)
	}
	if session.User == nil || session.User.ID != "user-1" || string(session.User.UserMetadata["must_change_password"]) != "true" || session.User.extraFields["identities"] == nil || session.extraFields["session_extension"] == nil {
		t.Fatalf("refreshed session dropped existing user data: %#v", session)
	}
}

func TestSignOutUsesCurrentGlobalDefaultAndCallerToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/auth/v1/logout" || r.URL.Query().Get("scope") != "global" {
			t.Errorf("request = %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("Authorization") != "Bearer user-access" || r.Header.Get("apikey") != "public-anon" {
			t.Errorf("unexpected credentials: authorization=%q apikey=%q", r.Header.Get("Authorization"), r.Header.Get("apikey"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := testClient(t, server.URL, "public-anon", "secret-service")
	if err := client.SignOut(context.Background(), "user-access"); err != nil {
		t.Fatal(err)
	}
}

func TestSignOutExpiredSessionIsTreatedAsLocalClear(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"msg":"session missing"}`)
	}))
	defer server.Close()

	client := testClient(t, server.URL, "public-anon", "")
	if err := client.SignOut(context.Background(), "expired-token"); err != nil {
		t.Fatalf("sign out error = %v", err)
	}
	if err := client.SignOutWithScope(context.Background(), "token", "invalid"); err == nil {
		t.Fatal("invalid scope must be rejected")
	}
}

func TestRequestPasswordResetMatchesCurrentRedirectContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/auth/v1/recover" {
			t.Errorf("request = %s %s", r.Method, r.URL.String())
		}
		if got := r.URL.Query().Get("redirect_to"); got != "https://inovar.example/?recovery=1" {
			t.Errorf("redirect_to = %q", got)
		}
		if r.Header.Get("Authorization") != "Bearer public-anon" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["email"] != "cliente@example.com" {
			t.Errorf("recovery email = %q", body["email"])
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := testClient(t, server.URL, "public-anon", "secret-service")
	if err := client.RequestPasswordReset(context.Background(), " cliente@example.com ", "https://inovar.example/?recovery=1"); err != nil {
		t.Fatal(err)
	}
}

func TestUpdatePasswordUsesAuthenticatedPUT(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/auth/v1/user" {
			t.Errorf("request = %s %s", r.Method, r.URL.String())
		}
		if r.Header.Get("Authorization") != "Bearer recovery-access" || r.Header.Get("apikey") != "public-anon" {
			t.Errorf("unexpected credentials")
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"password":"nova-senha"}` {
			t.Errorf("body = %s", body)
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()

	client := testClient(t, server.URL, "public-anon", "")
	if err := client.UpdatePassword(context.Background(), "recovery-access", "nova-senha"); err != nil {
		t.Fatal(err)
	}
	if err := client.UpdatePassword(context.Background(), "", "nova-senha"); err != ErrUnauthorized {
		t.Fatalf("missing-token error = %v", err)
	}
	if err := client.RequestPasswordReset(context.Background(), "", "https://inovar.example"); err == nil {
		t.Fatal("empty email must be rejected")
	}
}

func TestUpdatePasswordWithMetadataClearsForcedChangeFlag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if string(payload["password"]) != `"nova-senha"` || string(payload["data"]) != `{"must_change_password":false}` {
			t.Errorf("payload = %s", body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := testClient(t, server.URL, "public-anon", "")
	if err := client.UpdatePasswordWithMetadata(context.Background(), "recovery-access", "nova-senha", map[string]any{"must_change_password": false}); err != nil {
		t.Fatal(err)
	}
}

func TestOAuthURLAndAccessTokenSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/auth/v1/user" || r.Header.Get("Authorization") != "Bearer oauth-access" {
			t.Errorf("request = %s %s, authorization=%q", r.Method, r.URL, r.Header.Get("Authorization"))
		}
		_, _ = io.WriteString(w, `{"id":"oauth-user","email":"cliente@example.com","user_metadata":{"nome":"Cliente"}}`)
	}))
	defer server.Close()

	client := testClient(t, server.URL, "public-anon", "")
	oauthURL, err := client.SignInWithOAuthURL("google", "https://inovar.example/")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(oauthURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/auth/v1/authorize" || parsed.Query().Get("provider") != "google" || parsed.Query().Get("redirect_to") != "https://inovar.example/" || parsed.Query().Get("scopes") != "email profile" {
		t.Fatalf("OAuth URL = %s", oauthURL)
	}
	if _, err := client.SignInWithOAuthURL("facebook", ""); err == nil {
		t.Fatal("unsupported provider must be rejected")
	}
	session, err := client.SessionFromAccessToken(context.Background(), "oauth-access")
	if err != nil {
		t.Fatal(err)
	}
	if session.AccessToken != "oauth-access" || session.User == nil || session.User.ID != "oauth-user" || session.User.Email != "cliente@example.com" {
		t.Fatalf("session = %#v", session)
	}
}

func TestAuthErrorParsingDoesNotExposeResponseBodyWhenMalformed(t *testing.T) {
	err := authResponseError(Result{StatusCode: http.StatusBadGateway, Body: []byte("<html>secret reverse proxy details</html>")})
	if err == nil || !strings.Contains(err.Error(), "Bad Gateway") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unexpected error: %v", err)
	}
}

