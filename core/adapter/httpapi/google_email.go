package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"inovarapp/core/adapter/supabase"
	"inovarapp/core/domain"
)

const googleEmailUserInfoURL = "https://openidconnect.googleapis.com/v1/userinfo"

type GoogleEmailHandler struct {
	Supabase *supabase.Client
	HTTP     interface {
		Do(*http.Request) (*http.Response, error)
	}
	IdentityAPI  string
	ClientID     string
	ClientSecret string
}

func (h GoogleEmailHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeResourceJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Método não permitido"})
		return
	}
	if h.Supabase == nil {
		writeResourceJSON(w, http.StatusInternalServerError, map[string]string{"error": "Backend não configurado"})
		return
	}
	caller, err := h.Supabase.AuthenticateCaller(r.Context(), supabase.BearerToken(r.Header.Get("Authorization")))
	if err != nil {
		writeResourceJSON(w, http.StatusUnauthorized, map[string]string{"error": "Não autenticado"})
		return
	}
	if caller.Role != domain.RoleAdmin && caller.Role != domain.RoleTechnician {
		writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Somente a equipe Inovar conecta o Gmail"})
		return
	}
	settings := SettingsHandler{Supabase: h.Supabase}
	config, err := settings.loadSettings(r.Context())
	if errors.Is(err, errTechnicianSettingsNotFound) {
		config, err = defaultTechnicianSettings(), nil
	}
	if err != nil {
		writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível consultar a conexão Gmail"})
		return
	}
	if r.Method == http.MethodGet {
		var account, refreshToken string
		_ = json.Unmarshal(config["email_google_account"], &account)
		_ = json.Unmarshal(config["email_google_refresh_token"], &refreshToken)
		writeResourceJSON(w, http.StatusOK, map[string]any{
			"connected":  strings.TrimSpace(refreshToken) != "" && strings.TrimSpace(account) != "",
			"email":      account,
			"configured": strings.TrimSpace(h.ClientID) != "" && strings.TrimSpace(h.ClientSecret) != "",
		})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	var input struct {
		Action               string `json:"action"`
		ProviderToken        string `json:"providerToken"`
		ProviderRefreshToken string `json:"providerRefreshToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Solicitação inválida"})
		return
	}
	switch input.Action {
	case "disconnect":
		config["email_google_refresh_token"] = rawJSON("")
		config["email_google_account"] = rawJSON("")
		if err := settings.saveSettings(r.Context(), config); err != nil {
			writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível desconectar o Gmail"})
			return
		}
		writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true, "connected": false})
	case "connect":
		if strings.TrimSpace(input.ProviderToken) == "" || len(input.ProviderToken) > 10000 || len(input.ProviderRefreshToken) > 10000 {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Autorização Google inválida. Conecte novamente e aceite a permissão de envio."})
			return
		}
		if strings.TrimSpace(h.ClientID) == "" || strings.TrimSpace(h.ClientSecret) == "" {
			writeResourceJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "A integração Google OAuth não está configurada no servidor"})
			return
		}
		identity, err := h.googleIdentity(r.Context(), input.ProviderToken)
		if err != nil || !identity.Verified {
			writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Não foi possível validar a conta Google. Autorize novamente e tente de novo."})
			return
		}
		linked, err := h.googleIdentityLinked(r.Context(), caller, identity.Subject)
		if err != nil || !linked {
			writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Conecte a mesma conta Google vinculada ao seu login Inovar."})
			return
		}
		var savedEmail, savedRefresh string
		_ = json.Unmarshal(config["email_google_account"], &savedEmail)
		_ = json.Unmarshal(config["email_google_refresh_token"], &savedRefresh)
		refreshToken := strings.TrimSpace(input.ProviderRefreshToken)
		if refreshToken == "" && strings.EqualFold(savedEmail, identity.Email) {
			refreshToken = savedRefresh
		}
		if refreshToken == "" {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "O Google não retornou a autorização renovável. Tente conectar novamente e aceite todas as permissões."})
			return
		}
		config["email_google_account"] = rawJSON(strings.TrimSpace(identity.Email))
		config["email_google_refresh_token"] = rawJSON(refreshToken)
		if err := settings.saveSettings(r.Context(), config); err != nil {
			writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível salvar a conexão Gmail no Supabase"})
			return
		}
		writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true, "connected": true, "email": identity.Email})
	default:
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Ação inválida"})
	}
}

type googleEmailIdentity struct {
	Subject  string `json:"sub"`
	Email    string `json:"email"`
	Verified bool   `json:"email_verified"`
}

func (h GoogleEmailHandler) googleIdentity(ctx context.Context, token string) (googleEmailIdentity, error) {
	endpoint := strings.TrimSpace(h.IdentityAPI)
	if endpoint == "" {
		endpoint = googleEmailUserInfoURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return googleEmailIdentity{}, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := h.client().Do(request)
	if err != nil {
		return googleEmailIdentity{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
		return googleEmailIdentity{}, errors.New("Google identity lookup failed")
	}
	var identity googleEmailIdentity
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&identity); err != nil || identity.Subject == "" || strings.TrimSpace(identity.Email) == "" {
		return googleEmailIdentity{}, errors.New("Google identity response is incomplete")
	}
	return identity, nil
}

func (h GoogleEmailHandler) googleIdentityLinked(ctx context.Context, caller supabase.Caller, subject string) (bool, error) {
	result, err := h.Supabase.UserRequest(ctx, caller.Token, "/auth/v1/user", supabase.RequestOptions{Method: http.MethodGet})
	if err != nil || result.StatusCode != http.StatusOK {
		return false, errors.New("could not read authenticated identities")
	}
	var user struct {
		Identities []struct {
			Provider     string `json:"provider"`
			IdentityData struct {
				Subject string `json:"sub"`
			} `json:"identity_data"`
		} `json:"identities"`
	}
	if err := json.Unmarshal(result.Body, &user); err != nil {
		return false, err
	}
	for _, identity := range user.Identities {
		if identity.Provider == "google" && identity.IdentityData.Subject == subject {
			return true, nil
		}
	}
	return false, nil
}

func (h GoogleEmailHandler) client() interface {
	Do(*http.Request) (*http.Response, error)
} {
	if h.HTTP != nil {
		return h.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
