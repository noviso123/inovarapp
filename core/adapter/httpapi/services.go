package httpapi

import (
	"bytes"
	"encoding/json"
	"github.com/google/uuid"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"inovarapp/core/adapter/supabase"
	"inovarapp/core/adapter/whatsappqueue"
	"inovarapp/core/domain"
)

type ServicesHandler struct {
	Supabase            *supabase.Client
	ScheduleMaintenance MaintenanceScheduler
}

type AppointmentsHandler struct {
	Supabase            *supabase.Client
	ScheduleMaintenance MaintenanceScheduler
}

type resourceMutation struct {
	ID        string                     `json:"id"`
	ServiceID string                     `json:"service_id"`
	Fields    map[string]json.RawMessage `json:"fields"`
}

var serviceFields = map[string]bool{
	"cliente_id": true, "aparelho_id": true, "tecnico_id": true, "tipo": true,
	"descricao": true, "problema": true, "data_solicitacao": true,
	"data_agendamento": true, "hora_agendamento": true, "data_inicio": true,
	"data_conclusao": true, "data_cancelamento": true, "motivo_cancelamento": true,
	"status": true, "valor": true, "observacoes": true,
}

var serviceCreateFields = func() map[string]bool {
	fields := make(map[string]bool, len(serviceFields)+1)
	for field, allowed := range serviceFields {
		fields[field] = allowed
	}
	fields["id"] = true
	return fields
}()

var appointmentFields = map[string]bool{
	"service_id": true, "cliente_id": true, "data": true, "hora": true,
	"status": true, "observacoes": true,
}

func (h ServicesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Supabase == nil {
		writeResourceJSON(w, http.StatusInternalServerError, map[string]string{"error": "Backend não configurado"})
		return
	}
	caller, err := h.Supabase.AuthenticateCaller(r.Context(), supabase.BearerToken(r.Header.Get("Authorization")))
	if err != nil {
		writeResourceJSON(w, http.StatusUnauthorized, map[string]string{"error": "Não autenticado"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		query := url.Values{"select": {"*,customers:cliente_id(id,nome,whatsapp,endereco,bairro,cidade,estado),air_conditioners:aparelho_id(id,marca,modelo,btus,ambiente)"}, "order": {"data_solicitacao.desc"}}
		forwardUserResource(w, r, h.Supabase, "/rest/v1/services?"+query.Encode(), http.MethodGet, nil, "", false)
	case http.MethodPost:
		if !isTeamRole(caller.Role) {
			writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Somente a equipe pode cadastrar serviços por esta rota"})
			return
		}
		fields, ok := decodeResourceFields(w, r, serviceCreateFields)
		if !ok {
			return
		}
		if rawID, exists := fields["id"]; exists {
			var id string
			if json.Unmarshal(rawID, &id) != nil || uuid.Validate(id) != nil {
				writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Identificador de serviço inválido"})
				return
			}
		}
		if forwardUserResource(w, r, h.Supabase, "/rest/v1/services", http.MethodPost, fields, "", true) {
			syncMaintenanceAfterMutation(r.Context(), h.ScheduleMaintenance)
		}
	case http.MethodPatch, http.MethodPut:
		if !isTeamRole(caller.Role) {
			writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Somente a equipe pode alterar serviços"})
			return
		}
		mutation, ok := decodeResourceMutation(w, r, serviceFields)
		if !ok {
			return
		}
		if forwardUserResource(w, r, h.Supabase, resourceByID("services", "id", mutation.ID), http.MethodPatch, mutation.Fields, "", true) {
			if serviceQueueMustBeCancelled(mutation.Fields) {
				_ = whatsappqueue.CancelPendingBySource(r.Context(), h.Supabase, "servico", mutation.ID, "Agendamento alterado ou serviço encerrado; aviso anterior cancelado")
			}
			syncMaintenanceAfterMutation(r.Context(), h.ScheduleMaintenance)
		}
	case http.MethodDelete:
		if !isTeamRole(caller.Role) {
			writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Somente a equipe pode excluir serviços"})
			return
		}
		mutation, ok := decodeResourceMutation(w, r, nil)
		if !ok {
			return
		}
		if forwardUserResource(w, r, h.Supabase, resourceByID("services", "id", mutation.ID), http.MethodDelete, nil, "return=minimal", false) {
			_ = whatsappqueue.CancelPendingBySource(r.Context(), h.Supabase, "servico", mutation.ID, "Ordem de serviço excluída; aviso pendente cancelado")
			syncMaintenanceAfterMutation(r.Context(), h.ScheduleMaintenance)
		}
	default:
		writeResourceJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Método não permitido"})
	}
}

// CustomerServicesHandler returns only rows linked to the authenticated customer's profile.
type CustomerServicesHandler struct {
	Supabase *supabase.Client
	Notifier PushNotifier
}

func (h CustomerServicesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	if caller.Role != "CLIENTE" {
		writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Área exclusiva do cliente"})
		return
	}
	// Resolve profile -> customer using RLS, then scope services to that customer.
	profiles, err := h.Supabase.UserRequest(r.Context(), caller.Token, "/rest/v1/customers?"+url.Values{"select": {"id"}, "profile_id": {"eq." + caller.UserID}, "limit": {"2"}}.Encode(), supabase.RequestOptions{Method: http.MethodGet})
	if err != nil || profiles.StatusCode != http.StatusOK {
		writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível carregar as solicitações."})
		return
	}
	var customers []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(profiles.Body, &customers); err != nil || len(customers) > 1 {
		writeResourceJSON(w, http.StatusConflict, map[string]string{"error": "Não foi possível identificar o cadastro do cliente."})
		return
	}
	if len(customers) == 0 {
		if r.Method == http.MethodPost {
			writeResourceJSON(w, http.StatusConflict, map[string]string{"error": "Não foi possível identificar seu cadastro."})
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = io.WriteString(w, "[]")
		return
	}
	if r.Method == http.MethodPost {
		fields, ok := decodeResourceFields(w, r, map[string]bool{"aparelho_id": true, "tipo": true, "problema": true, "data_agendamento": true, "observacoes": true})
		if !ok {
			return
		}
		var serviceType string
		if err := json.Unmarshal(fields["tipo"], &serviceType); err != nil || !validCustomerServiceType(serviceType) {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Selecione um tipo de serviço válido."})
			return
		}
		if raw := fields["data_agendamento"]; len(raw) > 0 && string(raw) != "null" {
			var date string
			if json.Unmarshal(raw, &date) != nil || date != "" && !validCustomerServiceDate(date) {
				writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "A data preferida é inválida."})
				return
			}
		}
		if applianceRaw := fields["aparelho_id"]; len(applianceRaw) > 0 && string(applianceRaw) != "null" {
			var applianceID string
			if json.Unmarshal(applianceRaw, &applianceID) != nil || strings.TrimSpace(applianceID) == "" {
				writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Aparelho inválido."})
				return
			}
			ownedQuery := url.Values{"select": {"id"}, "id": {"eq." + applianceID}, "cliente_id": {"eq." + customers[0].ID}, "limit": {"1"}}
			owned, err := h.Supabase.UserRequest(r.Context(), caller.Token, "/rest/v1/air_conditioners?"+ownedQuery.Encode(), supabase.RequestOptions{Method: http.MethodGet})
			var ownedAppliances []struct {
				ID string `json:"id"`
			}
			if err != nil || owned.StatusCode != http.StatusOK || json.Unmarshal(owned.Body, &ownedAppliances) != nil || len(ownedAppliances) != 1 || ownedAppliances[0].ID != applianceID {
				writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Aparelho não pertence ao seu cadastro."})
				return
			}
		}
		customerID, _ := json.Marshal(customers[0].ID)
		fields["cliente_id"], fields["status"] = customerID, json.RawMessage(`"PENDENTE"`)
		response := httptest.NewRecorder()
		forwardUserResource(response, r, h.Supabase, "/rest/v1/services", http.MethodPost, fields, "", true)
		if response.Code >= 200 && response.Code < 300 && h.Notifier != nil {
			h.Notifier.NotifyTeam(r.Context(), "Novo chamado de cliente", "Um cliente solicitou atendimento — confira a Central de Atendimento.")
		}
		for key, values := range response.Header() {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(response.Code)
		_, _ = w.Write(response.Body.Bytes())
		return
	}
	query := url.Values{"select": {"*"}, "cliente_id": {"eq." + customers[0].ID}, "order": {"data_solicitacao.desc"}}
	forwardUserResource(w, r, h.Supabase, "/rest/v1/services?"+query.Encode(), http.MethodGet, nil, "", false)
}

func validCustomerServiceType(serviceType string) bool {
	switch serviceType {
	case "INSTALACAO", "MANUTENCAO_PREVENTIVA", "MANUTENCAO_CORRETIVA", "LIMPEZA", "RECARGA_GAS", "AVALIACAO", "OUTRO":
		return true
	default:
		return false
	}
}

func validCustomerServiceDate(date string) bool {
	parsed, err := time.Parse("2006-01-02", date)
	return err == nil && parsed.Format("2006-01-02") == date
}

func (h AppointmentsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Supabase == nil {
		writeResourceJSON(w, http.StatusInternalServerError, map[string]string{"error": "Backend não configurado"})
		return
	}
	caller, err := h.Supabase.AuthenticateCaller(r.Context(), supabase.BearerToken(r.Header.Get("Authorization")))
	if err != nil {
		writeResourceJSON(w, http.StatusUnauthorized, map[string]string{"error": "Não autenticado"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		query := url.Values{"select": {"*"}, "order": {"data.asc"}}
		forwardUserResource(w, r, h.Supabase, "/rest/v1/appointments?"+query.Encode(), http.MethodGet, nil, "", false)
	case http.MethodPost:
		if !isTeamRole(caller.Role) {
			writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Somente a equipe pode criar agendamentos"})
			return
		}
		fields, ok := decodeResourceFields(w, r, appointmentFields)
		if !ok {
			return
		}
		if forwardUserResource(w, r, h.Supabase, "/rest/v1/appointments", http.MethodPost, fields, "", true) {
			syncMaintenanceAfterMutation(r.Context(), h.ScheduleMaintenance)
		}
	case http.MethodPatch, http.MethodPut:
		if !isTeamRole(caller.Role) {
			writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Somente a equipe pode alterar agendamentos"})
			return
		}
		mutation, ok := decodeResourceMutation(w, r, appointmentFields)
		if !ok {
			return
		}
		if mutation.ServiceID == "" {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Agendamento não informado"})
			return
		}
		if forwardUserResource(w, r, h.Supabase, resourceByID("appointments", "service_id", mutation.ServiceID), http.MethodPatch, mutation.Fields, "", true) {
			syncMaintenanceAfterMutation(r.Context(), h.ScheduleMaintenance)
		}
	default:
		writeResourceJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Método não permitido"})
	}
}

func isTeamRole(role domain.Role) bool {
	return role == domain.RoleAdmin || role == domain.RoleTechnician
}

func decodeResourceFields(w http.ResponseWriter, r *http.Request, allow map[string]bool) (map[string]json.RawMessage, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	var fields map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&fields); err != nil || fields == nil {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Solicitação inválida"})
		return nil, false
	}
	fields = filterResourceFields(fields, allow)
	if len(fields) == 0 {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Nenhum campo válido foi informado"})
		return nil, false
	}
	return fields, true
}

func decodeResourceMutation(w http.ResponseWriter, r *http.Request, allow map[string]bool) (resourceMutation, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	var mutation resourceMutation
	if err := json.NewDecoder(r.Body).Decode(&mutation); err != nil {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Solicitação inválida"})
		return resourceMutation{}, false
	}
	if mutation.ID == "" && mutation.ServiceID == "" {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Registro não informado"})
		return resourceMutation{}, false
	}
	if allow != nil {
		mutation.Fields = filterResourceFields(mutation.Fields, allow)
		if len(mutation.Fields) == 0 {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Nenhum campo válido foi informado"})
			return resourceMutation{}, false
		}
	}
	return mutation, true
}

func filterResourceFields(fields map[string]json.RawMessage, allow map[string]bool) map[string]json.RawMessage {
	filtered := make(map[string]json.RawMessage, len(fields))
	for key, value := range fields {
		if allow[key] && len(value) != 0 {
			filtered[key] = value
		}
	}
	return filtered
}

func resourceByID(table, key, id string) string {
	query := url.Values{}
	query.Set(key, "eq."+id)
	return "/rest/v1/" + table + "?" + query.Encode()
}

func forwardUserResource(w http.ResponseWriter, r *http.Request, client *supabase.Client, path, method string, body any, prefer string, single bool) bool {
	headers := make(http.Header)
	if single {
		headers.Set("Accept", "application/vnd.pgrst.object+json")
	}
	result, err := client.UserRequest(r.Context(), supabase.BearerToken(r.Header.Get("Authorization")), path, supabase.RequestOptions{
		Method: method, Body: body, Prefer: prefer, Headers: headers,
	})
	if err != nil {
		writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível acessar os dados do atendimento"})
		return false
	}
	if result.StatusCode < http.StatusOK || result.StatusCode >= http.StatusMultipleChoices {
		message := "Não foi possível acessar os dados do atendimento"
		if result.StatusCode == http.StatusUnauthorized || result.StatusCode == http.StatusForbidden {
			message = "Você não tem permissão para esta operação"
		}
		writeResourceJSON(w, result.StatusCode, map[string]string{"error": message})
		return false
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(result.StatusCode)
	if len(result.Body) != 0 {
		_, _ = io.Copy(w, bytes.NewReader(result.Body))
	} else {
		_, _ = io.WriteString(w, `{"ok":true}`)
	}
	return true
}

func serviceQueueMustBeCancelled(fields map[string]json.RawMessage) bool {
	if _, changedDate := fields["data_agendamento"]; changedDate {
		return true
	}
	if _, changedTime := fields["hora_agendamento"]; changedTime {
		return true
	}
	if rawStatus, exists := fields["status"]; exists {
		var status string
		if json.Unmarshal(rawStatus, &status) == nil && strings.ToUpper(strings.TrimSpace(status)) != "AGENDADO" {
			return true
		}
	}
	return false
}

func writeResourceJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
