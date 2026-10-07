package supabase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const authAPIVersion = "2024-01-01"

var ErrInvalidAuthResponse = errors.New("invalid Supabase Auth response")

// AuthUser contains identity fields returned by Supabase Auth. AppMetadata and
// UserMetadata are data only; authorization continues to use the profile read
// with the caller JWT in AuthenticateCaller.
type AuthUser struct {
	ID           string                     `json:"id"`
	Email        string                     `json:"email,omitempty"`
	AppMetadata  map[string]json.RawMessage `json:"app_metadata,omitempty"`
	UserMetadata map[string]json.RawMessage `json:"user_metadata,omitempty"`
	extraFields  map[string]json.RawMessage
}

func (u *AuthUser) UnmarshalJSON(data []byte) error {
	type wire AuthUser
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	extra, err := unknownJSONFields(data, "id", "email", "app_metadata", "user_metadata")
	if err != nil {
		return err
	}
	*u = AuthUser(decoded)
	u.extraFields = extra
	return nil
}

func (u AuthUser) MarshalJSON() ([]byte, error) {
	type wire AuthUser
	encoded, err := json.Marshal(wire(u))
	if err != nil {
		return nil, err
	}
	return mergeUnknownJSONFields(encoded, u.extraFields)
}

// AuthSession matches the session fields persisted by the current Supabase JS
// client, including the rotating refresh token and the user's metadata.
type AuthSession struct {
	AccessToken          string    `json:"access_token"`
	TokenType            string    `json:"token_type"`
	ExpiresIn            int64     `json:"expires_in"`
	ExpiresAt            int64     `json:"expires_at,omitempty"`
	RefreshToken         string    `json:"refresh_token"`
	ProviderToken        string    `json:"provider_token,omitempty"`
	ProviderRefreshToken string    `json:"provider_refresh_token,omitempty"`
	User                 *AuthUser `json:"user,omitempty"`
	extraFields          map[string]json.RawMessage
}

func (s *AuthSession) UnmarshalJSON(data []byte) error {
	type wire AuthSession
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	extra, err := unknownJSONFields(data,
		"access_token", "token_type", "expires_in", "expires_at", "refresh_token",
		"provider_token", "provider_refresh_token", "user",
	)
	if err != nil {
		return err
	}
	*s = AuthSession(decoded)
	s.extraFields = extra
	return nil
}

func (s AuthSession) MarshalJSON() ([]byte, error) {
	type wire AuthSession
	encoded, err := json.Marshal(wire(s))
	if err != nil {
		return nil, err
	}
	return mergeUnknownJSONFields(encoded, s.extraFields)
}

// AuthError preserves the status and safe error fields returned by Auth.
// Callers handling password recovery should still show a generic success
// response so they do not disclose whether an email has an account.
type AuthError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *AuthError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if text := http.StatusText(e.StatusCode); text != "" {
		return fmt.Sprintf("Supabase Auth request failed: %s", text)
	}
	return "Supabase Auth request failed"
}

// SignInWithPassword starts an email/password session with the public anon key.
// The service-role key is never involved in a client authentication flow.
func (c *Client) SignInWithPassword(ctx context.Context, email, password string) (AuthSession, error) {
	email = strings.TrimSpace(email)
	if email == "" || password == "" {
		return AuthSession{}, errors.New("email and password are required")
	}
	query := url.Values{"grant_type": {"password"}}
	result, err := c.publicAuthRequest(ctx, "/auth/v1/token?"+query.Encode(), RequestOptions{
		Method: http.MethodPost,
		Body: map[string]string{
			"email":    email,
			"password": password,
		},
	})
	if err != nil {
		return AuthSession{}, err
	}
	return decodeAuthSession(result, "", true)
}

// SessionFromAccessToken validates a session returned by an OAuth or recovery
// redirect and retrieves the authenticated user's profile from Supabase Auth.
func (c *Client) SessionFromAccessToken(ctx context.Context, accessToken string) (AuthSession, error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return AuthSession{}, ErrUnauthorized
	}
	result, err := c.userAuthRequest(ctx, accessToken, "/auth/v1/user", RequestOptions{Method: http.MethodGet})
	if err != nil {
		return AuthSession{}, err
	}
	var user AuthUser
	if err := result.DecodeJSON(&user); err != nil || strings.TrimSpace(user.ID) == "" {
		return AuthSession{}, ErrInvalidAuthResponse
	}
	return AuthSession{AccessToken: accessToken, TokenType: "bearer", User: &user}, nil
}

// SignInWithOAuthURL returns the provider authorization URL without server
// credentials. The browser performs the redirect and returns the session.
func (c *Client) SignInWithOAuthURL(provider, redirectTo string) (string, error) {
	return c.SignInWithOAuthURLAndScopes(provider, redirectTo, "email profile", nil)
}

// SignInWithOAuthURLAndScopes starts a provider flow with explicitly requested
// scopes and query parameters, without exposing server credentials.
func (c *Client) SignInWithOAuthURLAndScopes(provider, redirectTo, scopes string, queryParams url.Values) (string, error) {
	provider = strings.TrimSpace(provider)
	if provider != "google" {
		return "", errors.New("unsupported OAuth provider")
	}
	u := *c.baseURL
	u.Path = strings.TrimRight(c.baseURL.Path, "/") + "/auth/v1/authorize"
	query := url.Values{"provider": {provider}}
	if strings.TrimSpace(scopes) != "" {
		query.Set("scopes", scopes)
	}
	if strings.TrimSpace(redirectTo) != "" {
		query.Set("redirect_to", redirectTo)
	}
	for key, values := range queryParams {
		for _, value := range values {
			query.Add(key, value)
		}
	}
	u.RawQuery = query.Encode()
	return u.String(), nil
}

// LinkIdentityOAuthURL begins Supabase's authenticated identity-linking OAuth
// flow and returns the provider URL for the browser to open.
func (c *Client) LinkIdentityOAuthURL(ctx context.Context, token, provider, redirectTo, scopes string, queryParams url.Values) (string, error) {
	if provider != "google" {
		return "", errors.New("unsupported OAuth provider")
	}
	query := url.Values{"provider": {provider}, "scopes": {scopes}, "redirect_to": {redirectTo}, "skip_http_redirect": {"true"}}
	for key, values := range queryParams {
		for _, value := range values {
			query.Add(key, value)
		}
	}
	result, err := c.userAuthRequest(ctx, token, "/auth/v1/user/identities/authorize?"+query.Encode(), RequestOptions{Method: http.MethodGet})
	if err != nil {
		return "", err
	}
	var response struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(result.Body, &response) != nil || strings.TrimSpace(response.URL) == "" {
		return "", ErrInvalidAuthResponse
	}
	return response.URL, nil
}

// RefreshSession exchanges a one-time refresh token and carries it forward if
// the server omits a replacement token. Callers must persist the returned
// session before making another refresh request.
func (c *Client) RefreshSession(ctx context.Context, current AuthSession) (AuthSession, error) {
	refreshToken := strings.TrimSpace(current.RefreshToken)
	if refreshToken == "" {
		return AuthSession{}, ErrUnauthorized
	}
	query := url.Values{"grant_type": {"refresh_token"}}
	result, err := c.publicAuthRequest(ctx, "/auth/v1/token?"+query.Encode(), RequestOptions{
		Method: http.MethodPost,
		Body:   map[string]string{"refresh_token": refreshToken},
	})
	if err != nil {
		return AuthSession{}, err
	}
	refreshed, err := decodeAuthSession(result, refreshToken, false)
	if err != nil {
		return AuthSession{}, err
	}
	return mergeAuthSession(refreshed, current), nil
}

// SignOut uses Supabase JS's default global scope so existing behavior is
// preserved. The client should also clear its locally stored session.
func (c *Client) SignOut(ctx context.Context, accessToken string) error {
	return c.SignOutWithScope(ctx, accessToken, "global")
}

// SignOutWithScope validates the server scope before sending a revocation
// request. A missing or already expired session still clears successfully,
// matching the current client library's sign-out behavior.
func (c *Client) SignOutWithScope(ctx context.Context, accessToken, scope string) error {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return nil
	}
	if scope != "global" && scope != "local" && scope != "others" {
		return errors.New("invalid sign-out scope")
	}
	query := url.Values{"scope": {scope}}
	result, err := c.userAuthRequest(ctx, accessToken, "/auth/v1/logout?"+query.Encode(), RequestOptions{
		Method: http.MethodPost,
	})
	if err != nil {
		var authErr *AuthError
		if errors.As(err, &authErr) && (authErr.StatusCode == http.StatusUnauthorized || authErr.StatusCode == http.StatusForbidden || authErr.StatusCode == http.StatusNotFound) {
			return nil
		}
		return err
	}
	return requireAuthSuccess(result)
}

// RequestPasswordReset requests an email link. The caller must use a generic
// user-facing success message for both known and unknown addresses.
func (c *Client) RequestPasswordReset(ctx context.Context, email, redirectTo string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return errors.New("email is required")
	}
	query := url.Values{}
	if redirectTo != "" {
		query.Set("redirect_to", redirectTo)
	}
	path := "/auth/v1/recover"
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	result, err := c.publicAuthRequest(ctx, path, RequestOptions{
		Method: http.MethodPost,
		Body:   map[string]string{"email": email},
	})
	if err != nil {
		return err
	}
	return requireAuthSuccess(result)
}

// UpdatePassword changes the password for the authenticated user, including a
// session created by the password recovery flow.
func (c *Client) UpdatePassword(ctx context.Context, accessToken, password string) error {
	return c.UpdatePasswordWithMetadata(ctx, accessToken, password, nil)
}

// UpdatePasswordWithMetadata changes the password and optional user metadata
// using the authenticated Supabase session.
func (c *Client) UpdatePasswordWithMetadata(ctx context.Context, accessToken, password string, metadata map[string]any) error {
	if strings.TrimSpace(accessToken) == "" {
		return ErrUnauthorized
	}
	if password == "" {
		return errors.New("password is required")
	}
	body := map[string]any{"password": password}
	if len(metadata) != 0 {
		body["data"] = metadata
	}
	result, err := c.userAuthRequest(ctx, accessToken, "/auth/v1/user", RequestOptions{
		Method: http.MethodPut,
		Body:   body,
	})
	if err != nil {
		return err
	}
	return requireAuthSuccess(result)
}

func (c *Client) publicAuthRequest(ctx context.Context, path string, options RequestOptions) (Result, error) {
	// New publishable API keys are not JWTs. Send them only in apikey; legacy
	// anon keys are JWTs and remain valid as the public Auth bearer token.
	bearer := c.anonKey
	if strings.HasPrefix(strings.TrimSpace(bearer), "sb_publishable_") {
		bearer = ""
	}
	result, err := c.do(ctx, c.anonKey, bearer, path, withAuthAPIVersion(options))
	if err != nil {
		return Result{}, err
	}
	if err := authResponseError(result); err != nil {
		return Result{}, err
	}
	return result, nil
}

func (c *Client) userAuthRequest(ctx context.Context, token, path string, options RequestOptions) (Result, error) {
	if strings.TrimSpace(token) == "" {
		return Result{}, ErrUnauthorized
	}
	result, err := c.do(ctx, c.anonKey, token, path, withAuthAPIVersion(options))
	if err != nil {
		return Result{}, err
	}
	if err := authResponseError(result); err != nil {
		return Result{}, err
	}
	return result, nil
}

func withAuthAPIVersion(options RequestOptions) RequestOptions {
	options.Headers = options.Headers.Clone()
	if options.Headers == nil {
		options.Headers = make(http.Header)
	}
	options.Headers.Set("X-Supabase-Api-Version", authAPIVersion)
	return options
}

func authResponseError(result Result) error {
	if result.StatusCode >= http.StatusOK && result.StatusCode < http.StatusMultipleChoices {
		return nil
	}
	var response struct {
		Code             string `json:"code"`
		ErrorCode        string `json:"error_code"`
		Error            string `json:"error"`
		Message          string `json:"message"`
		Msg              string `json:"msg"`
		ErrorDescription string `json:"error_description"`
	}
	_ = json.Unmarshal(result.Body, &response)
	code := response.ErrorCode
	if code == "" {
		code = response.Code
	}
	message := response.Message
	if message == "" {
		message = response.Msg
	}
	if message == "" {
		message = response.ErrorDescription
	}
	if message == "" {
		message = response.Error
	}
	return &AuthError{StatusCode: result.StatusCode, Code: code, Message: message}
}

func requireAuthSuccess(result Result) error {
	return authResponseError(result)
}

func decodeAuthSession(result Result, previousRefreshToken string, requireUser bool) (AuthSession, error) {
	var session AuthSession
	if err := json.Unmarshal(result.Body, &session); err != nil {
		return AuthSession{}, fmt.Errorf("decode Supabase Auth session: %w", err)
	}
	if session.AccessToken == "" {
		return AuthSession{}, ErrInvalidAuthResponse
	}
	if session.RefreshToken == "" {
		session.RefreshToken = previousRefreshToken
	}
	if session.RefreshToken == "" {
		return AuthSession{}, ErrInvalidAuthResponse
	}
	if session.TokenType == "" {
		session.TokenType = "bearer"
	}
	if session.ExpiresAt == 0 && session.ExpiresIn > 0 {
		session.ExpiresAt = time.Now().Unix() + session.ExpiresIn
	}
	if requireUser && (session.User == nil || strings.TrimSpace(session.User.ID) == "") {
		return AuthSession{}, ErrInvalidAuthResponse
	}
	return session, nil
}

func mergeAuthSession(refreshed, previous AuthSession) AuthSession {
	if refreshed.ProviderToken == "" {
		refreshed.ProviderToken = previous.ProviderToken
	}
	if refreshed.ProviderRefreshToken == "" {
		refreshed.ProviderRefreshToken = previous.ProviderRefreshToken
	}
	if refreshed.User == nil {
		refreshed.User = previous.User
	} else if previous.User != nil {
		refreshed.User = mergeAuthUser(*refreshed.User, *previous.User)
	}
	refreshed.extraFields = mergeRawJSONFields(refreshed.extraFields, previous.extraFields)
	return refreshed
}

func mergeAuthUser(refreshed, previous AuthUser) *AuthUser {
	if refreshed.ID == "" {
		refreshed.ID = previous.ID
	}
	if refreshed.Email == "" {
		refreshed.Email = previous.Email
	}
	refreshed.AppMetadata = mergeRawJSONFields(refreshed.AppMetadata, previous.AppMetadata)
	refreshed.UserMetadata = mergeRawJSONFields(refreshed.UserMetadata, previous.UserMetadata)
	refreshed.extraFields = mergeRawJSONFields(refreshed.extraFields, previous.extraFields)
	return &refreshed
}

func unknownJSONFields(data []byte, known ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for _, key := range known {
		delete(fields, key)
	}
	return fields, nil
}

func mergeUnknownJSONFields(encoded []byte, extra map[string]json.RawMessage) ([]byte, error) {
	if len(extra) == 0 {
		return encoded, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return nil, err
	}
	for key, value := range extra {
		if _, exists := fields[key]; !exists {
			fields[key] = value
		}
	}
	return json.Marshal(fields)
}

func mergeRawJSONFields(current, previous map[string]json.RawMessage) map[string]json.RawMessage {
	if len(previous) == 0 {
		return current
	}
	merged := make(map[string]json.RawMessage, len(current)+len(previous))
	for key, value := range current {
		merged[key] = value
	}
	for key, value := range previous {
		if _, exists := merged[key]; !exists {
			merged[key] = value
		}
	}
	return merged
}

