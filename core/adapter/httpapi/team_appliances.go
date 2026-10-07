package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"inovarapp/core/adapter/supabase"
)

// TeamAppliancesHandler exposes the staff-side appliance CRUD through the
// signed-in user's JWT so Supabase RLS continues to enforce row ownership.
type TeamAppliancesHandler struct{ Supabase *supabase.Client }

var teamApplianceFields = map[string]bool{
	"cliente_id": true, "marca": true, "modelo": true, "btus": true,
	"tipo": true, "ambiente": true, "numero_serie": true,
	"local_instalacao": true, "data_instalacao": true, "ultima_manutencao": true, "observacoes": true,
	"gas_tipo": true, "tensao": true,
}

var teamApplianceCreateFields = func() map[string]bool {
	fields := make(map[string]bool, len(teamApplianceFields)+1)
	for key, allowed := range teamApplianceFields {
		fields[key] = allowed
	}
	fields["id"] = true
	return fields
}()

func (h TeamAppliancesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Supabase == nil {
		writeResourceJSON(w, http.StatusInternalServerError, map[string]string{"error": "Backend não configurado"})
		return
	}
	caller, err := h.Supabase.AuthenticateCaller(r.Context(), supabase.BearerToken(r.Header.Get("Authorization")))
	if err != nil {
		writeResourceJSON(w, http.StatusUnauthorized, map[string]string{"error": "Não autenticado"})
		return
	}
	if !isTeamRole(caller.Role) {
		writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Somente a equipe pode alterar aparelhos"})
		return
	}
	switch r.Method {
	case http.MethodPost:
		fields, ok := decodeResourceFields(w, r, teamApplianceCreateFields)
		if !ok {
			return
		}
		if rawID, exists := fields["id"]; exists {
			var id string
			if json.Unmarshal(rawID, &id) != nil || uuid.Validate(id) != nil {
				writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Identificador local inválido"})
				return
			}
		}
		if !validTeamApplianceFields(fields, true) {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Informe cliente, marca, capacidade, tipo e ambiente válidos"})
			return
		}
		forwardUserResource(w, r, h.Supabase, "/rest/v1/air_conditioners", http.MethodPost, fields, "return=representation", true)
	case http.MethodPatch, http.MethodPut:
		mutation, ok := decodeResourceMutation(w, r, teamApplianceFields)
		if !ok {
			return
		}
		if !validTeamApplianceFields(mutation.Fields, false) {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Dados do aparelho inválidos"})
			return
		}
		forwardUserResource(w, r, h.Supabase, resourceByID("air_conditioners", "id", mutation.ID), http.MethodPatch, mutation.Fields, "return=minimal", false)
	case http.MethodDelete:
		mutation, ok := decodeResourceMutation(w, r, nil)
		if !ok {
			return
		}
		forwardUserResource(w, r, h.Supabase, resourceByID("air_conditioners", "id", mutation.ID), http.MethodDelete, nil, "return=minimal", false)
	default:
		writeResourceJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Método não permitido"})
	}
}

func validTeamApplianceFields(fields map[string]json.RawMessage, requireAll bool) bool {
	readString := func(key string, required bool) (string, bool) {
		raw, exists := fields[key]
		if !exists {
			return "", !required
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", false
		}
		value = strings.TrimSpace(value)
		return value, (!required || value != "")
	}
	for _, field := range []string{"cliente_id", "marca", "tipo", "ambiente"} {
		value, ok := readString(field, requireAll)
		if !ok {
			return false
		}
		if field == "tipo" && value != "" && !validCustomerApplianceType(value) {
			return false
		}
	}
	if raw, exists := fields["btus"]; exists {
		var capacity int
		if json.Unmarshal(raw, &capacity) != nil || !validCustomerApplianceBTUs(capacity) {
			return false
		}
	} else if requireAll {
		return false
	}
	if _, ok := readString("ultima_manutencao", false); !ok {
		return false
	}
	for key, allowed := range map[string]map[string]bool{
		"gas_tipo": {"R-410A": true, "R-32": true, "R-22": true, "Outro": true},
		"tensao":   {"220V": true, "110V": true, "Bivolt": true},
	} {
		value, ok := readString(key, false)
		if !ok || value != "" && !allowed[value] {
			return false
		}
	}
	return true
}
