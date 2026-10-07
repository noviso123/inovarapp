// Package googlecontacts searches the authenticated team's linked Google
// contacts without exposing provider tokens or credentials to another user.
package googlecontacts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"inovarapp/core/adapter/supabase"
)

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type Handler struct {
	Supabase    *supabase.Client
	HTTP        HTTPDoer
	IdentityAPI string
	PeopleAPI   string
}

type Contact struct {
	Name  string `json:"nome"`
	Phone string `json:"telefone"`
	Email string `json:"email,omitempty"`
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Método não permitido"})
		return
	}
	if h.Supabase == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Backend não configurado"})
		return
	}
	caller, err := h.Supabase.AuthenticateCaller(r.Context(), supabase.BearerToken(r.Header.Get("Authorization")))
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Não autenticado"})
		return
	}
	if caller.Role != "ADMIN" && caller.Role != "TECNICO" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Somente a equipe Inovar pode pesquisar contatos."})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var input struct {
		ProviderToken string `json:"providerToken"`
		Query         string `json:"query"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Solicitação inválida"})
		return
	}
	input.Query = strings.TrimSpace(input.Query)
	if input.ProviderToken == "" || len(input.ProviderToken) > 10000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Sessão do Google ausente ou inválida."})
		return
	}
	if input.Query == "" || len(input.Query) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Informe um nome, telefone ou e-mail para buscar."})
		return
	}
	contacts, err := h.search(r.Context(), caller, input.ProviderToken, input.Query)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, errUnlinkedIdentity) {
			status = http.StatusForbidden
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"contacts": contacts})
}

var errUnlinkedIdentity = errors.New("Conecte a conta Google vinculada ao seu login Inovar.")

func (h Handler) search(ctx context.Context, caller supabase.Caller, providerToken, query string) ([]Contact, error) {
	var identity struct {
		Sub string `json:"sub"`
	}
	if err := h.googleJSON(ctx, http.MethodGet, h.identityURL(), providerToken, &identity); err != nil || identity.Sub == "" {
		return nil, errUnlinkedIdentity
	}
	userResult, err := h.Supabase.UserRequest(ctx, caller.Token, "/auth/v1/user", supabase.RequestOptions{Method: http.MethodGet})
	if err != nil || userResult.StatusCode != http.StatusOK {
		return nil, errors.New("Não foi possível confirmar a conta Google vinculada.")
	}
	var user struct {
		Identities []struct {
			Provider     string `json:"provider"`
			IdentityData struct {
				Sub string `json:"sub"`
			} `json:"identity_data"`
		} `json:"identities"`
	}
	if json.Unmarshal(userResult.Body, &user) != nil {
		return nil, errors.New("Não foi possível confirmar a conta Google vinculada.")
	}
	linked := false
	for _, item := range user.Identities {
		if item.Provider == "google" && item.IdentityData.Sub == identity.Sub {
			linked = true
			break
		}
	}
	if !linked {
		return nil, errUnlinkedIdentity
	}
	params := url.Values{"query": {query}, "pageSize": {"10"}, "readMask": {"names,phoneNumbers,emailAddresses"}}
	var response struct {
		Results []struct {
			Person struct {
				Names []struct {
					DisplayName string `json:"displayName"`
				} `json:"names"`
				Phones []struct {
					Value string `json:"value"`
				} `json:"phoneNumbers"`
				Emails []struct {
					Value string `json:"value"`
				} `json:"emailAddresses"`
			} `json:"person"`
		} `json:"results"`
	}
	if err := h.googleJSON(ctx, http.MethodGet, h.peopleURL()+"?"+params.Encode(), providerToken, &response); err != nil {
		return nil, errors.New("Não foi possível buscar nos contatos do Google. Verifique a permissão de contatos.")
	}
	contacts := make([]Contact, 0, len(response.Results))
	for _, result := range response.Results {
		contact := Contact{}
		if len(result.Person.Names) > 0 {
			contact.Name = result.Person.Names[0].DisplayName
		}
		if len(result.Person.Phones) > 0 {
			contact.Phone = normalizePhone(result.Person.Phones[0].Value)
		}
		if len(result.Person.Emails) > 0 {
			contact.Email = result.Person.Emails[0].Value
		}
		if contact.Name != "" || contact.Phone != "" || contact.Email != "" {
			contacts = append(contacts, contact)
		}
	}
	return contacts, nil
}

func (h Handler) googleJSON(ctx context.Context, method, endpoint, token string, output any) error {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := h.client().Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Google returned status %d", response.StatusCode)
	}
	if output != nil {
		return json.Unmarshal(data, output)
	}
	return nil
}

func (h Handler) client() HTTPDoer {
	if h.HTTP != nil {
		return h.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second}
}
func (h Handler) identityURL() string {
	if h.IdentityAPI != "" {
		return strings.TrimRight(h.IdentityAPI, "/")
	}
	return "https://openidconnect.googleapis.com/v1/userinfo"
}
func (h Handler) peopleURL() string {
	if h.PeopleAPI != "" {
		return strings.TrimRight(h.PeopleAPI, "/")
	}
	return "https://people.googleapis.com/v1/people:searchContacts"
}

func normalizePhone(value string) string {
	var normalized strings.Builder
	for _, char := range value {
		if char >= '0' && char <= '9' || char == '+' {
			normalized.WriteRune(char)
		}
	}
	return normalized.String()
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
