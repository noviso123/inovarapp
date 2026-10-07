package supabase

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

type CreateAdminUserInput struct {
	Email              string
	Password           string
	Name               string
	Phone              string
	MustChangePassword bool
}

// CreateAdminUser creates a confirmed account through Supabase's server-only
// Admin API. It must only be called after the operation is authorized.
func (c *Client) CreateAdminUser(ctx context.Context, input CreateAdminUserInput) (AuthUser, error) {
	if strings.TrimSpace(c.serviceRoleKey) == "" {
		return AuthUser{}, ErrNotConfigured
	}
	if strings.TrimSpace(input.Email) == "" || input.Password == "" {
		return AuthUser{}, errors.New("email and password are required")
	}
	body := map[string]any{
		"email":         strings.ToLower(strings.TrimSpace(input.Email)),
		"password":      input.Password,
		"email_confirm": true,
		"user_metadata": map[string]any{
			"nome":                 strings.TrimSpace(input.Name),
			"telefone":             strings.TrimSpace(input.Phone),
			"must_change_password": input.MustChangePassword,
		},
	}
	return c.adminAuthUser(ctx, "/auth/v1/admin/users", http.MethodPost, body)
}

func (c *Client) GetAdminUser(ctx context.Context, userID string) (AuthUser, error) {
	if strings.TrimSpace(c.serviceRoleKey) == "" {
		return AuthUser{}, ErrNotConfigured
	}
	if strings.TrimSpace(userID) == "" {
		return AuthUser{}, ErrUnauthorized
	}
	return c.adminAuthUser(ctx, "/auth/v1/admin/users/"+url.PathEscape(userID), http.MethodGet, nil)
}

func (c *Client) UpdateAdminUser(ctx context.Context, userID string, body map[string]any) (AuthUser, error) {
	if strings.TrimSpace(c.serviceRoleKey) == "" {
		return AuthUser{}, ErrNotConfigured
	}
	if strings.TrimSpace(userID) == "" || len(body) == 0 {
		return AuthUser{}, errors.New("user ID and update fields are required")
	}
	return c.adminAuthUser(ctx, "/auth/v1/admin/users/"+url.PathEscape(userID), http.MethodPut, body)
}

func (c *Client) adminAuthUser(ctx context.Context, path, method string, body any) (AuthUser, error) {
	result, err := c.ServiceRequest(ctx, path, RequestOptions{
		Method: method,
		Body:   body,
		Headers: http.Header{
			"X-Supabase-Api-Version": []string{authAPIVersion},
		},
	})
	if err != nil {
		return AuthUser{}, err
	}
	if result.StatusCode < http.StatusOK || result.StatusCode >= http.StatusMultipleChoices {
		return AuthUser{}, authResponseError(result)
	}
	var user AuthUser
	if err := json.Unmarshal(result.Body, &user); err != nil {
		return AuthUser{}, err
	}
	if user.ID == "" {
		return AuthUser{}, ErrInvalidAuthResponse
	}
	return user, nil
}
