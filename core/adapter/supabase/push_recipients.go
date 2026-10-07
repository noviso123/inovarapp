package supabase

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// ListTeamProfileIDs returns only administrator and technician profile IDs for
// server-side fan-out after an authenticated customer action.
func (c *Client) ListTeamProfileIDs(ctx context.Context) ([]string, error) {
	query := url.Values{"select": {"id"}, "tipo": {"in.(ADMIN,TECNICO)"}}
	result, err := c.ServiceRequest(ctx, "/rest/v1/profiles?"+query.Encode(), RequestOptions{Method: http.MethodGet})
	if err != nil {
		return nil, err
	}
	if result.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list team push recipients returned status %d", result.StatusCode)
	}
	var profiles []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(result.Body, &profiles); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		if profile.ID != "" {
			ids = append(ids, profile.ID)
		}
	}
	return ids, nil
}
