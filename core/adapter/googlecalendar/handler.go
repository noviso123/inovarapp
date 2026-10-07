package googlecalendar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"inovarapp/core/adapter/supabase"
)

type Handler struct {
	Supabase                         *supabase.Client
	HTTP                             HTTPDoer
	ClientID, ClientSecret           string
	GoogleAPI, OAuthAPI, IdentityAPI string
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
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
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "A sincronização da agenda operacional é exclusiva da equipe."})
		return
	}
	var stored Connection
	exists, err := h.Supabase.LoadGoogleConnection(r.Context(), caller.UserID, &stored)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Falha ao consultar a conexão Google."})
		return
	}
	if stored.Events == nil {
		stored.Events = map[string]string{}
	}
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]any{"connected": exists && !stored.Disconnected, "lastSync": stored.LastSync, "renewable": stored.RefreshToken != "" && h.ClientID != "" && h.ClientSecret != ""})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	var input struct {
		Action               string `json:"action"`
		ProviderToken        string `json:"providerToken"`
		ProviderRefreshToken string `json:"providerRefreshToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Solicitação inválida"})
		return
	}
	switch input.Action {
	case "disconnect":
		stored.Disconnected = true
		err = h.Supabase.StoreGoogleConnection(r.Context(), caller.UserID, stored)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível desconectar a agenda."})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case "connect":
		if input.ProviderToken == "" || len(input.ProviderToken) > 10000 || len(input.ProviderRefreshToken) > 10000 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Autorização Google inválida."})
			return
		}
		if err := h.connect(r.Context(), caller, &stored, input.ProviderToken, input.ProviderRefreshToken); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
			return
		}
		if err := h.Supabase.StoreGoogleConnection(r.Context(), caller.UserID, stored); err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível salvar a conexão Google."})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case "sync":
		if !exists || stored.Disconnected {
			writeJSON(w, http.StatusOK, map[string]bool{"connected": false})
			return
		}
		result, err := Sync(r.Context(), h.Supabase, caller, &stored, h.HTTP, h.ClientID, h.ClientSecret, time.Now(), Endpoints{CalendarBase: h.calendarBase(), Token: h.oauthEndpoint()})
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Ação inválida"})
	}
}

func (h Handler) connect(ctx context.Context, caller supabase.Caller, current *Connection, token, refresh string) error {
	var identity struct {
		Sub string `json:"sub"`
	}
	if err := h.googleJSON(ctx, http.MethodGet, h.identityEndpoint(), token, nil, &identity); err != nil {
		return errors.New("Conecte a conta Google vinculada ao seu login Inovar.")
	}
	userResult, err := h.Supabase.UserRequest(ctx, caller.Token, "/auth/v1/user", supabase.RequestOptions{Method: http.MethodGet})
	if err != nil || userResult.StatusCode != http.StatusOK {
		return errors.New("Não foi possível confirmar a conta Google vinculada.")
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
		return errors.New("Não foi possível confirmar a conta Google vinculada.")
	}
	linked := false
	for _, candidate := range user.Identities {
		if candidate.Provider == "google" && candidate.IdentityData.Sub == identity.Sub {
			linked = true
			break
		}
	}
	if !linked {
		return errors.New("Conecte a conta Google vinculada ao seu login Inovar.")
	}
	var permission json.RawMessage
	if err := h.googleJSON(ctx, http.MethodGet, h.calendarBase()+"?maxResults=1", token, nil, &permission); err != nil {
		return errors.New("Autorize o acesso ao Google Agenda ao conectar.")
	}
	if refresh == "" {
		refresh = current.RefreshToken
	}
	*current = Connection{AccessToken: token, RefreshToken: refresh, ExpiresAt: time.Now().Add(50 * time.Minute).UnixMilli(), Events: current.Events, LastSync: current.LastSync}
	return nil
}

func (h Handler) googleJSON(ctx context.Context, method, endpoint, token string, body any, output any) error {
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	status, response, err := h.googleRequest(ctx, method, endpoint, token, encoded)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("Google returned status %d", status)
	}
	if output != nil {
		return json.Unmarshal(response, output)
	}
	return nil
}
func (h Handler) googleRequest(ctx context.Context, method, endpoint, token string, body []byte) (int, []byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := h.client().Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	return response.StatusCode, data, err
}
func (h Handler) client() HTTPDoer {
	if h.HTTP != nil {
		return h.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second}
}
func (h Handler) calendarBase() string {
	if h.GoogleAPI != "" {
		return strings.TrimRight(h.GoogleAPI, "/") + "/calendars/primary/events"
	}
	return "https://www.googleapis.com/calendar/v3/calendars/primary/events"
}
func (h Handler) oauthEndpoint() string {
	if h.OAuthAPI != "" {
		return h.OAuthAPI
	}
	return "https://oauth2.googleapis.com/token"
}
func (h Handler) identityEndpoint() string {
	if h.IdentityAPI != "" {
		return h.IdentityAPI
	}
	return "https://openidconnect.googleapis.com/v1/userinfo"
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
