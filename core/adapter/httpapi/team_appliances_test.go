package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTeamApplianceCreateUsesCallerJWTAndAllowlist(t *testing.T) {
	var inserted map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"tech-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"tech-1","tipo":"TECNICO"}]`)
		case "/rest/v1/air_conditioners":
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer tech-token" {
				t.Errorf("request method/auth=%s %q", r.Method, r.Header.Get("Authorization"))
			}
			if err := json.NewDecoder(r.Body).Decode(&inserted); err != nil {
				t.Fatal(err)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"appliance-new"}`)
		default:
			t.Errorf("unexpected request %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/aparelhos", strings.NewReader(`{"id":"550e8400-e29b-41d4-a716-446655440001","cliente_id":"c1","marca":"LG","btus":12000,"tipo":"Split Hi-Wall","ambiente":"Sala","gas_tipo":"R-32","tensao":"Bivolt","admin":true}`))
	req.Header.Set("Authorization", "Bearer tech-token")
	res := httptest.NewRecorder()
	(TeamAppliancesHandler{Supabase: accountTestClient(t, server)}).ServeHTTP(res, req)
	if res.Code != http.StatusCreated || inserted["id"] != "550e8400-e29b-41d4-a716-446655440001" || inserted["cliente_id"] != "c1" || inserted["btus"] != float64(12000) || inserted["gas_tipo"] != "R-32" || inserted["tensao"] != "Bivolt" || inserted["admin"] != nil {
		t.Fatalf("status=%d inserted=%#v response=%s", res.Code, inserted, res.Body.String())
	}
}

func TestTeamApplianceMutationRejectsCustomerAndInvalidCapacity(t *testing.T) {
	for _, tc := range []struct {
		role, body string
		want       int
	}{
		{"CLIENTE", `{"cliente_id":"c1","marca":"LG","btus":12000,"tipo":"Split Hi-Wall","ambiente":"Sala"}`, http.StatusForbidden},
		{"TECNICO", `{"cliente_id":"c1","marca":"LG","btus":12345,"tipo":"Split Hi-Wall","ambiente":"Sala"}`, http.StatusBadRequest},
		{"TECNICO", `{"id":"local-appliance","cliente_id":"c1","marca":"LG","btus":12000,"tipo":"Split Hi-Wall","ambiente":"Sala"}`, http.StatusBadRequest},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/auth/v1/user":
				_, _ = io.WriteString(w, `{"id":"user-1"}`)
			case "/rest/v1/profiles":
				_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"`+tc.role+`"}]`)
			default:
				t.Errorf("request unexpectedly forwarded: %s", r.URL.String())
				http.NotFound(w, r)
			}
		}))
		req := httptest.NewRequest(http.MethodPost, "/api/aparelhos", strings.NewReader(tc.body))
		req.Header.Set("Authorization", "Bearer user-token")
		res := httptest.NewRecorder()
		(TeamAppliancesHandler{Supabase: accountTestClient(t, server)}).ServeHTTP(res, req)
		server.Close()
		if res.Code != tc.want {
			t.Errorf("role=%s status=%d want=%d body=%s", tc.role, res.Code, tc.want, res.Body.String())
		}
	}
}
