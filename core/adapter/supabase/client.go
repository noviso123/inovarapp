// Package supabase implements the server-side Supabase Auth and REST boundary.
// Calls made with a user's token remain subject to Postgres RLS. Service-role
// calls are separate and must only follow an authorization decision in a use case.
package supabase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"inovarapp/core/domain"
)

var (
	ErrUnauthorized  = errors.New("supabase caller is not authorized")
	ErrNotConfigured = errors.New("supabase credentials are not configured")
)

// TransportError marks an unavailable Supabase request/response stream. Callers
// can distinguish a transient network failure from an HTTP error response.
type TransportError struct {
	Err error
}

func (e *TransportError) Error() string { return "Supabase transport failed: " + e.Err.Error() }
func (e *TransportError) Unwrap() error { return e.Err }

const defaultMaxResponseBytes int64 = 16 << 20

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type Config struct {
	URL              string
	AnonKey          string
	ServiceRoleKey   string
	HTTPClient       HTTPDoer
	MaxResponseBytes int64
}

type Client struct {
	baseURL          *url.URL
	anonKey          string
	serviceRoleKey   string
	httpClient       HTTPDoer
	maxResponseBytes int64
}

type RequestOptions struct {
	Method  string
	Body    any
	Prefer  string
	Headers http.Header
}

type Result struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

func (r Result) DecodeJSON(target any) error {
	if len(r.Body) == 0 {
		return io.EOF
	}
	if err := json.Unmarshal(r.Body, target); err != nil {
		return fmt.Errorf("decode Supabase response: %w", err)
	}
	return nil
}

type Caller struct {
	UserID string      `json:"userId"`
	Role   domain.Role `json:"role"`
	Token  string      `json:"-"`
}

// BaseURL returns the configured Supabase origin without credentials.
func (c *Client) BaseURL() string {
	if c == nil || c.baseURL == nil {
		return ""
	}
	return strings.TrimRight(c.baseURL.String(), "/")
}

type profile struct {
	ID   string      `json:"id"`
	Role domain.Role `json:"tipo"`
}

func New(cfg Config) (*Client, error) {
	baseURL, err := url.Parse(strings.TrimSpace(cfg.URL))
	if err != nil || baseURL == nil || (baseURL.Scheme != "https" && baseURL.Scheme != "http") || baseURL.Host == "" || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, errors.New("invalid Supabase URL")
	}
	if strings.TrimSpace(cfg.AnonKey) == "" {
		return nil, ErrNotConfigured
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 20 * time.Second}
	}
	if cfg.MaxResponseBytes <= 0 {
		cfg.MaxResponseBytes = defaultMaxResponseBytes
	}
	return &Client{
		baseURL:          baseURL,
		anonKey:          cfg.AnonKey,
		serviceRoleKey:   cfg.ServiceRoleKey,
		httpClient:       cfg.HTTPClient,
		maxResponseBytes: cfg.MaxResponseBytes,
	}, nil
}

func FromEnv() (*Client, error) {
	return New(Config{
		URL:            envFirst("SUPABASE_URL", "VITE_SUPABASE_URL"),
		AnonKey:        envFirst("SUPABASE_ANON_KEY", "SUPABASE_PUBLISHABLE_KEY", "VITE_SUPABASE_ANON_KEY"),
		ServiceRoleKey: envFirst("SUPABASE_SERVICE_ROLE_KEY", "SUPABASE_SECRET_KEY"),
	})
}

func envFirst(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	return ""
}

// BearerToken extracts the authorization token using the existing API contract.
func BearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

// UserRequest sends a request with the public anon key and caller JWT. Supabase
// applies RLS to the caller, so this is the default for application data access.
func (c *Client) UserRequest(ctx context.Context, token, path string, options RequestOptions) (Result, error) {
	if strings.TrimSpace(token) == "" {
		return Result{}, ErrUnauthorized
	}
	return c.do(ctx, c.anonKey, token, path, options)
}

// ServiceRequest sends a privileged service-role request. Call it only from a
// server use case after explicit authentication, ownership, and role checks.
func (c *Client) ServiceRequest(ctx context.Context, path string, options RequestOptions) (Result, error) {
	if strings.TrimSpace(c.serviceRoleKey) == "" {
		return Result{}, ErrNotConfigured
	}
	// Supabase's new sb_secret keys belong in apikey only. They are not JWTs,
	// so sending one as Authorization: Bearer causes Supabase to reject it.
	bearer := c.serviceRoleKey
	if strings.HasPrefix(strings.TrimSpace(bearer), "sb_secret_") {
		bearer = ""
	}
	return c.do(ctx, c.serviceRoleKey, bearer, path, options)
}

// RequireServiceRole reports whether this server client is configured for
// privileged operations without exposing the credential itself.
func (c *Client) RequireServiceRole() error {
	if c == nil || strings.TrimSpace(c.serviceRoleKey) == "" {
		return ErrNotConfigured
	}
	return nil
}

func (c *Client) do(ctx context.Context, apiKey, bearer, path string, options RequestOptions) (Result, error) {
	target, err := c.endpoint(path)
	if err != nil {
		return Result{}, err
	}
	method := strings.TrimSpace(options.Method)
	if method == "" {
		method = http.MethodGet
	}
	var body io.Reader
	if options.Body != nil {
		encoded, err := json.Marshal(options.Body)
		if err != nil {
			return Result{}, fmt.Errorf("encode Supabase request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return Result{}, fmt.Errorf("create Supabase request: %w", err)
	}
	req.Header.Set("apikey", apiKey)
	if strings.TrimSpace(bearer) != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	req.Header.Set("Content-Type", "application/json")
	prefer := options.Prefer
	if prefer == "" {
		prefer = "return=representation"
	}
	req.Header.Set("Prefer", prefer)
	for name, values := range options.Headers {
		if strings.EqualFold(name, "Authorization") || strings.EqualFold(name, "apikey") {
			continue
		}
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Result{}, &TransportError{Err: err}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, c.maxResponseBytes+1))
	if err != nil {
		return Result{}, &TransportError{Err: err}
	}
	if int64(len(data)) > c.maxResponseBytes {
		return Result{}, errors.New("Supabase response exceeds configured limit")
	}
	return Result{StatusCode: resp.StatusCode, Header: resp.Header.Clone(), Body: data}, nil
}

func (c *Client) endpoint(path string) (*url.URL, error) {
	parsed, err := url.Parse(path)
	if err != nil || parsed == nil || parsed.IsAbs() || parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/") {
		return nil, errors.New("invalid Supabase resource path")
	}
	if !(strings.HasPrefix(parsed.Path, "/auth/v1/") || strings.HasPrefix(parsed.Path, "/rest/v1/") || strings.HasPrefix(parsed.Path, "/storage/v1/")) {
		return nil, errors.New("unsupported Supabase resource path")
	}
	target := c.baseURL.ResolveReference(parsed)
	if target.Scheme != c.baseURL.Scheme || !strings.EqualFold(target.Host, c.baseURL.Host) {
		return nil, errors.New("invalid Supabase resource origin")
	}
	return target, nil
}

// AuthenticateCaller asks Supabase Auth to validate the JWT, then reads the
// caller's profile with that same JWT so database RLS remains in force.
func (c *Client) AuthenticateCaller(ctx context.Context, token string) (Caller, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return Caller{}, ErrUnauthorized
	}
	userResult, err := c.UserRequest(ctx, token, "/auth/v1/user", RequestOptions{Method: http.MethodGet})
	if err != nil {
		return Caller{}, err
	}
	if userResult.StatusCode != http.StatusOK {
		return Caller{}, ErrUnauthorized
	}
	var authUser struct {
		ID string `json:"id"`
	}
	if err := userResult.DecodeJSON(&authUser); err != nil || strings.TrimSpace(authUser.ID) == "" {
		return Caller{}, ErrUnauthorized
	}

	query := url.Values{}
	query.Set("id", "eq."+authUser.ID)
	query.Set("select", "id,tipo")
	profilePath := "/rest/v1/profiles?" + query.Encode()
	profileResult, err := c.UserRequest(ctx, token, profilePath, RequestOptions{Method: http.MethodGet})
	if err != nil {
		return Caller{}, err
	}
	if profileResult.StatusCode != http.StatusOK {
		return Caller{}, ErrUnauthorized
	}
	var profiles []profile
	if err := profileResult.DecodeJSON(&profiles); err != nil || len(profiles) != 1 || profiles[0].ID != authUser.ID {
		return Caller{}, ErrUnauthorized
	}
	switch profiles[0].Role {
	case domain.RoleAdmin, domain.RoleTechnician, domain.RoleCustomer:
		return Caller{UserID: authUser.ID, Role: profiles[0].Role, Token: token}, nil
	default:
		return Caller{}, ErrUnauthorized
	}
}

