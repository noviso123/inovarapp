package httpapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"inovarapp/core/adapter/supabase"
	"inovarapp/core/domain"
)

// CustomerProfileHandler lets a client update only the contact/address fields
// exposed by the legacy "Meus Dados" form. The row is derived from the caller
// profile, never from an ID supplied by the browser.
type CustomerProfileHandler struct{ Supabase *supabase.Client }

func (h CustomerProfileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
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
	if caller.Role != domain.RoleCustomer {
		writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Área exclusiva do cliente"})
		return
	}
	fields, ok := decodeResourceFields(w, r, map[string]bool{
		"whatsapp": true, "endereco": true, "numero": true, "bairro": true, "cidade": true,
	})
	if !ok {
		return
	}
	for field, raw := range fields {
		var value string
		if json.Unmarshal(raw, &value) != nil || len([]rune(value)) > 240 {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Confira os dados informados."})
			return
		}
		fields[field], _ = json.Marshal(strings.TrimSpace(value))
	}
	query := url.Values{"select": {"id"}, "profile_id": {"eq." + caller.UserID}, "limit": {"2"}}
	result, err := h.Supabase.UserRequest(r.Context(), caller.Token, "/rest/v1/customers?"+query.Encode(), supabase.RequestOptions{Method: http.MethodGet})
	var customers []struct {
		ID string `json:"id"`
	}
	if err != nil || result.StatusCode != http.StatusOK || json.Unmarshal(result.Body, &customers) != nil || len(customers) != 1 || strings.TrimSpace(customers[0].ID) == "" {
		writeResourceJSON(w, http.StatusConflict, map[string]string{"error": "Não foi possível identificar seu cadastro."})
		return
	}
	forwardUserResource(w, r, h.Supabase, resourceByID("customers", "id", customers[0].ID), http.MethodPatch, fields, "return=minimal", false)
}
