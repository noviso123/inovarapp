package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"inovarapp/core/adapter/supabase"
	"inovarapp/core/domain"
)

const technicianSettingsPath = "/storage/v1/object/documentos-inovar/config/tecnico.json"

var errTechnicianSettingsNotFound = errors.New("technician settings are not stored")

var technicianSettingKeys = []string{
	"businessName", "cnpj", "address", "name", "phone", "pixKey", "pixType",
	"defaultReturnMonths", "defaultWarrantyDays", "defaultPrice", "assinatura",
	"email_api_key", "email_from", "email_gmail_user", "email_gmail_pass", "email_google_account", "email_google_refresh_token",
	"tiposServicosCustom", "mensagensWhats", "lembrete_intervalo_dias",
	"tiposFixosRemovidos", "tiposFixosEditados", "calendario_token",
}

type SettingsHandler struct {
	Supabase    *supabase.Client
	EmailAPIKey string
	EmailFrom   string
}

type settingsResponse struct {
	OK     bool                       `json:"ok"`
	Config map[string]json.RawMessage `json:"config,omitempty"`
	Error  string                     `json:"error,omitempty"`
}

func (h SettingsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeSettingsJSON(w, http.StatusMethodNotAllowed, settingsResponse{Error: "Método não permitido"})
		return
	}
	if h.Supabase == nil {
		writeSettingsJSON(w, http.StatusInternalServerError, settingsResponse{Error: "Backend não configurado"})
		return
	}
	caller, err := h.Supabase.AuthenticateCaller(r.Context(), supabase.BearerToken(r.Header.Get("Authorization")))
	if err != nil {
		writeSettingsJSON(w, http.StatusUnauthorized, settingsResponse{Error: "Não autenticado"})
		return
	}
	if r.Method == http.MethodGet {
		h.get(w, r, caller)
		return
	}
	if caller.Role == domain.RoleCustomer {
		writeSettingsJSON(w, http.StatusForbidden, settingsResponse{Error: "Somente a equipe Inovar altera as configurações"})
		return
	}
	h.save(w, r)
}

func (h SettingsHandler) get(w http.ResponseWriter, r *http.Request, caller supabase.Caller) {
	config, err := h.loadSettings(r.Context())
	if err != nil {
		if !errors.Is(err, errTechnicianSettingsNotFound) {
			writeSettingsJSON(w, http.StatusInternalServerError, settingsResponse{Error: "Não foi possível carregar as configurações."})
			return
		}
		config = defaultTechnicianSettings()
		profile, profileErr := h.readSettingsProfile(r.Context(), caller)
		if profileErr != nil {
			writeSettingsJSON(w, http.StatusInternalServerError, settingsResponse{Error: "Não foi possível carregar as configurações."})
			return
		}
		if caller.Role == domain.RoleAdmin || caller.Role == domain.RoleTechnician {
			config["name"] = rawJSON(settingsFirstNonEmpty(profile.Name, "Gabriel Nascimento"))
			config["phone"] = rawJSON(profile.Phone)
		}
		if err := h.saveSettings(r.Context(), config); err != nil {
			writeSettingsJSON(w, http.StatusInternalServerError, settingsResponse{Error: "Não foi possível salvar as configurações."})
			return
		}
	}

	if rawEmpty(config["pixKey"]) {
		config["pixKey"] = rawJSON("gabrielnascimento458@gmail.com")
	}
	if rawEmpty(config["pixType"]) {
		config["pixType"] = rawJSON("email")
	}
	if rawEmpty(config["name"]) {
		config["name"] = rawJSON("Gabriel")
	}
	if rawEmpty(config["phone"]) {
		config["phone"] = rawJSON("27998279185")
	}
	if (caller.Role == domain.RoleAdmin || caller.Role == domain.RoleTechnician) && string(config["assinatura"]) != `"/inovar-brand/INOVAR_SIGNATURE_GABRIEL.png"` {
		config["assinatura"] = rawJSON("/inovar-brand/INOVAR_SIGNATURE_GABRIEL.png")
		if err := h.saveSettings(r.Context(), config); err != nil {
			writeSettingsJSON(w, http.StatusInternalServerError, settingsResponse{Error: "Não foi possível salvar as configurações."})
			return
		}
	}
	visible := maskTechnicianSettings(config, caller.Role == domain.RoleCustomer)
	removeLegacyWhatsAppSettings(visible)
	if caller.Role == domain.RoleCustomer {
		visible = customerVisibleSettings(visible)
	}
	writeSettingsJSON(w, http.StatusOK, settingsResponse{OK: true, Config: visible})
}

func customerVisibleSettings(config map[string]json.RawMessage) map[string]json.RawMessage {
	visible := make(map[string]json.RawMessage, 2)
	for _, key := range []string{"businessName", "phone"} {
		if value, ok := config[key]; ok {
			visible[key] = append(json.RawMessage(nil), value...)
		}
	}
	return visible
}

func (h SettingsHandler) save(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	var incoming map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil || incoming == nil {
		writeSettingsJSON(w, http.StatusBadRequest, settingsResponse{Error: "Solicitação inválida"})
		return
	}
	legacyCustomTypes, hasLegacyCustomTypes := incoming["tipos_servicos_custom"]
	if hasLegacyCustomTypes && isJSONArray(legacyCustomTypes) && !isJSONArray(incoming["tiposServicosCustom"]) {
		incoming["tiposServicosCustom"] = legacyCustomTypes
	}
	delete(incoming, "tipos_servicos_custom")
	current, err := h.loadSettings(r.Context())
	if err != nil {
		if !errors.Is(err, errTechnicianSettingsNotFound) {
			writeSettingsJSON(w, http.StatusInternalServerError, settingsResponse{Error: "Não foi possível carregar as configurações."})
			return
		}
		current = defaultTechnicianSettings()
	}
	for _, key := range technicianSettingKeys {
		value, exists := incoming[key]
		if !exists || string(value) == "null" {
			continue
		}
		var stringValue string
		isString := json.Unmarshal(value, &stringValue) == nil
		if isString && strings.HasPrefix(stringValue, "***") {
			continue
		}
		if isSensitiveSetting(key) && isString && strings.TrimSpace(stringValue) == "" {
			continue
		}
		current[key] = value
	}
	removeLegacyWhatsAppSettings(current)
	if rawEmpty(current["email_api_key"]) && h.EmailAPIKey != "" {
		current["email_api_key"] = rawJSON(h.EmailAPIKey)
	}
	if rawEmpty(current["email_from"]) && h.EmailFrom != "" {
		current["email_from"] = rawJSON(h.EmailFrom)
	}
	if err := h.saveSettings(r.Context(), current); err != nil {
		writeSettingsJSON(w, http.StatusInternalServerError, settingsResponse{Error: "Não foi possível salvar as configurações."})
		return
	}
	writeSettingsJSON(w, http.StatusOK, settingsResponse{OK: true, Config: maskTechnicianSettings(current, false)})
}

func (h SettingsHandler) loadSettings(ctx context.Context) (map[string]json.RawMessage, error) {
	result, err := h.Supabase.ServiceRequest(ctx, technicianSettingsPath, supabase.RequestOptions{Method: http.MethodGet})
	if err != nil {
		return nil, err
	}
	if result.StatusCode < http.StatusOK || result.StatusCode >= http.StatusMultipleChoices {
		if result.StatusCode == http.StatusNotFound {
			return nil, errTechnicianSettingsNotFound
		}
		return nil, fmt.Errorf("technician settings read failed with status %d", result.StatusCode)
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(result.Body, &config); err != nil || config == nil {
		return nil, errors.New("technician settings are invalid")
	}
	return config, nil
}

func (h SettingsHandler) saveSettings(ctx context.Context, config map[string]json.RawMessage) error {
	result, err := h.Supabase.ServiceRequest(ctx, technicianSettingsPath, supabase.RequestOptions{
		Method:  http.MethodPost,
		Headers: http.Header{"X-Upsert": []string{"true"}},
		Body:    config,
	})
	if err != nil {
		return err
	}
	if result.StatusCode < http.StatusOK || result.StatusCode >= http.StatusMultipleChoices {
		return errors.New("settings storage rejected the update")
	}
	return nil
}

func (h SettingsHandler) readSettingsProfile(ctx context.Context, caller supabase.Caller) (struct {
	Name  string `json:"nome"`
	Phone string `json:"telefone"`
}, error) {
	query := url.Values{}
	query.Set("id", "eq."+caller.UserID)
	query.Set("select", "nome,telefone,tipo")
	result, err := h.Supabase.UserRequest(ctx, caller.Token, "/rest/v1/profiles?"+query.Encode(), supabase.RequestOptions{Method: http.MethodGet})
	if err != nil {
		return struct {
			Name  string `json:"nome"`
			Phone string `json:"telefone"`
		}{}, err
	}
	if result.StatusCode != http.StatusOK {
		return struct {
			Name  string `json:"nome"`
			Phone string `json:"telefone"`
		}{}, errors.New("profile query failed")
	}
	var rows []struct {
		Name  string `json:"nome"`
		Phone string `json:"telefone"`
	}
	if err := json.Unmarshal(result.Body, &rows); err != nil || len(rows) != 1 {
		return struct {
			Name  string `json:"nome"`
			Phone string `json:"telefone"`
		}{}, errors.New("profile was not found")
	}
	return rows[0], nil
}

func defaultTechnicianSettings() map[string]json.RawMessage {
	defaults := map[string]any{
		"businessName": "Inovar Refrigeração", "cnpj": "36.020.014/0001-14", "address": "Serra",
		"name": "Gabriel", "phone": "27998279185", "pixKey": "gabrielnascimento458@gmail.com", "pixType": "email",
		"defaultReturnMonths": 6, "defaultWarrantyDays": 90, "defaultPrice": 250,
		"assinatura":    "/inovar-brand/INOVAR_SIGNATURE_GABRIEL.png",
		"email_api_key": "", "email_from": "", "email_gmail_user": "", "email_gmail_pass": "", "email_google_account": "", "email_google_refresh_token": "",
		"tiposServicosCustom": []any{}, "mensagensWhats": map[string]string{}, "lembrete_intervalo_dias": 7,
		"tiposFixosRemovidos": []any{}, "tiposFixosEditados": []any{}, "calendario_token": "",
	}
	config := make(map[string]json.RawMessage, len(defaults))
	for key, value := range defaults {
		config[key] = rawJSON(value)
	}
	return config
}

func maskTechnicianSettings(config map[string]json.RawMessage, hideCalendarToken bool) map[string]json.RawMessage {
	masked := make(map[string]json.RawMessage, len(config))
	for key, value := range config {
		masked[key] = append(json.RawMessage(nil), value...)
	}
	for _, key := range []string{"email_api_key", "email_gmail_pass", "email_google_refresh_token"} {
		if !rawEmpty(masked[key]) && string(masked[key]) != `""` {
			masked[key] = rawJSON("***configurada***")
		} else {
			masked[key] = rawJSON("")
		}
	}
	if hideCalendarToken {
		delete(masked, "calendario_token")
	}
	return masked
}

func isSensitiveSetting(key string) bool {
	switch key {
	case "email_gmail_user", "email_gmail_pass", "email_api_key", "email_google_refresh_token":
		return true
	default:
		return false
	}
}

func removeLegacyWhatsAppSettings(config map[string]json.RawMessage) {
	for _, key := range []string{
		"whatsapp_proprio_url", "whatsapp_proprio_token", "whatsapp_proprio_session",
		"whatsapp_evol_url", "whatsapp_evol_api_url", "whatsapp_evol_api_key",
		"whatsapp_evolution_url", "whatsapp_evolution_key", "whatsapp_evolution_token",
		"whatsapp_meta_token", "whatsapp_phone_id",
	} {
		delete(config, key)
	}
}

func rawJSON(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage("null")
	}
	return encoded
}

func rawEmpty(value json.RawMessage) bool {
	return len(value) == 0 || string(value) == "null" || string(value) == `""`
}

func isJSONArray(value json.RawMessage) bool {
	return len(value) > 0 && value[0] == '['
}

func settingsFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func writeSettingsJSON(w http.ResponseWriter, status int, response settingsResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}
