package httpapi

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/google/uuid"

	"inovarapp/core/adapter/supabase"
)

// ServiceHistoryHandler exposes the technical history under the signed-in
// team's JWT so the project's Supabase RLS policies remain authoritative.
type ServiceHistoryHandler struct {
	Supabase            *supabase.Client
	ScheduleMaintenance MaintenanceScheduler
}

var serviceHistoryFields = map[string]bool{
	"service_id": true, "cliente_id": true, "aparelho_id": true, "data": true,
	"descricao": true, "problema": true, "diagnostico": true, "solucao": true,
	"pecas_utilizadas": true, "observacoes": true, "valor": true,
}

var legacyServiceHistoryFields = map[string]bool{
	"cliente_id": true, "aparelho_id": true, "tipo": true, "status": true,
	"valor": true, "data_agendamento": true, "descricao": true, "observacoes": true,
}

func (h ServiceHistoryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
		writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Somente a equipe pode acessar o histórico técnico"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		query := url.Values{"select": {"*"}, "order": {"data.desc"}}
		if r.URL.Query().Get("origem") == "timeline-services" {
			query = url.Values{"select": {"id,cliente_id,aparelho_id,data_solicitacao,data_agendamento,data_inicio,data_conclusao,data_cancelamento,motivo_cancelamento,descricao,problema,tipo,status,valor,observacoes"}, "order": {"data_solicitacao.desc"}}
			forwardUserResource(w, r, h.Supabase, "/rest/v1/services?"+query.Encode(), http.MethodGet, nil, "", false)
			return
		}
		if r.URL.Query().Get("origem") == "services" {
			query = url.Values{"select": {"id,cliente_id,aparelho_id,data_agendamento,data_conclusao,descricao,tipo,status,valor,observacoes"}, "status": {"eq.CONCLUIDO"}, "order": {"data_agendamento.desc"}}
			forwardUserResource(w, r, h.Supabase, "/rest/v1/services?"+query.Encode(), http.MethodGet, nil, "", false)
			return
		}
		if r.URL.Query().Get("origem") == "retornos" {
			query = url.Values{"select": {"id,cliente_id,aparelho_id,data_agendamento,data_conclusao,tipo,status,observacoes"}, "status": {"eq.CONCLUIDO"}, "order": {"data_conclusao.desc.nullslast,data_agendamento.desc"}}
			forwardUserResource(w, r, h.Supabase, "/rest/v1/services?"+query.Encode(), http.MethodGet, nil, "", false)
			return
		}
		if r.URL.Query().Get("origem") == "retornos-avulsos" {
			query = url.Values{"select": {"id,service_id,cliente_id,aparelho_id,data,descricao,problema,diagnostico,solucao,pecas_utilizadas,observacoes,valor"}, "order": {"data.desc"}}
			forwardUserResource(w, r, h.Supabase, "/rest/v1/service_history?"+query.Encode(), http.MethodGet, nil, "", false)
			return
		}
		forwardUserResource(w, r, h.Supabase, "/rest/v1/service_history?"+query.Encode(), http.MethodGet, nil, "", false)
	case http.MethodPost:
		var fields map[string]json.RawMessage
		var ok bool
		if r.URL.Query().Get("retroativo") == "1" {
			fields, ok = decodeResourceFields(w, r, legacyServiceHistoryFields)
		} else {
			fields, ok = decodeResourceFields(w, r, serviceHistoryFields)
		}
		if !ok {
			return
		}
		if r.URL.Query().Get("retroativo") == "1" {
			if forwardUserResource(w, r, h.Supabase, "/rest/v1/services", http.MethodPost, fields, "return=representation", true) {
				syncMaintenanceAfterMutation(r.Context(), h.ScheduleMaintenance)
			}
			return
		}
		if forwardUserResource(w, r, h.Supabase, "/rest/v1/service_history", http.MethodPost, fields, "return=representation", true) {
			syncMaintenanceAfterMutation(r.Context(), h.ScheduleMaintenance)
		}
	case http.MethodDelete:
		if !isTeamRole(caller.Role) {
			writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Somente a equipe pode excluir registros de histórico"})
			return
		}
		if r.URL.Query().Get("origem") != "retornos-avulsos" {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Origem do histórico inválida"})
			return
		}
		mutation, ok := decodeResourceMutation(w, r, nil)
		if !ok {
			return
		}
		if uuid.Validate(mutation.ID) != nil {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Identificador do histórico inválido"})
			return
		}
		if forwardUserResource(w, r, h.Supabase, resourceByID("service_history", "id", mutation.ID), http.MethodDelete, nil, "return=minimal", false) {
			syncMaintenanceAfterMutation(r.Context(), h.ScheduleMaintenance)
		}
	default:
		writeResourceJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Método não permitido"})
	}
}

// ApplianceMaintenanceHandler updates only the last-maintenance date. Keeping
// the field allowlist narrow prevents this lifecycle action from editing a
// customer's appliance profile accidentally.
type ApplianceMaintenanceHandler struct {
	Supabase            *supabase.Client
	ScheduleMaintenance MaintenanceScheduler
}

func (h ApplianceMaintenanceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch && r.Method != http.MethodPut {
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
	if !isTeamRole(caller.Role) {
		writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Somente a equipe pode atualizar a manutenção do aparelho"})
		return
	}
	mutation, ok := decodeResourceMutation(w, r, map[string]bool{"ultima_manutencao": true})
	if !ok {
		return
	}
	if forwardUserResource(w, r, h.Supabase, resourceByID("air_conditioners", "id", mutation.ID), http.MethodPatch, mutation.Fields, "return=minimal", false) {
		syncMaintenanceAfterMutation(r.Context(), h.ScheduleMaintenance)
	}
}

func rawFieldString(fields map[string]json.RawMessage, key string) string {
	var value string
	_ = json.Unmarshal(fields[key], &value)
	return value
}
