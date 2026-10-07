package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"inovarapp/core/adapter/supabase"
	"inovarapp/core/domain"
)

type CustomerPortalHandler struct {
	Supabase *supabase.Client
}

type customerPortalResponse struct {
	OK    bool                       `json:"ok"`
	Data  *domain.CustomerPortalData `json:"data,omitempty"`
	Error string                     `json:"error,omitempty"`
}

func (h CustomerPortalHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeCustomerPortalJSON(w, http.StatusMethodNotAllowed, customerPortalResponse{Error: "Método não permitido"})
		return
	}
	if h.Supabase == nil {
		writeCustomerPortalJSON(w, http.StatusInternalServerError, customerPortalResponse{Error: "Backend não configurado"})
		return
	}
	caller, err := h.Supabase.AuthenticateCaller(r.Context(), supabase.BearerToken(r.Header.Get("Authorization")))
	if err != nil {
		writeCustomerPortalJSON(w, http.StatusUnauthorized, customerPortalResponse{Error: "Não autenticado"})
		return
	}
	if caller.Role != domain.RoleCustomer {
		writeCustomerPortalJSON(w, http.StatusForbidden, customerPortalResponse{Error: "Área exclusiva do cliente"})
		return
	}

	query := url.Values{"select": {"*"}, "profile_id": {"eq." + caller.UserID}, "limit": {"2"}}
	customers, err := h.readRows(r, caller.Token, "/rest/v1/customers?"+query.Encode())
	if err != nil {
		writeCustomerPortalJSON(w, http.StatusBadGateway, customerPortalResponse{Error: "Não foi possível carregar os dados do portal."})
		return
	}
	if len(customers) == 0 {
		writeCustomerPortalJSON(w, http.StatusOK, customerPortalResponse{OK: true, Data: &domain.CustomerPortalData{
			Appliances: []map[string]any{}, Services: []map[string]any{}, Budgets: []map[string]any{},
		}})
		return
	}
	if len(customers) != 1 {
		writeCustomerPortalJSON(w, http.StatusConflict, customerPortalResponse{Error: "Há mais de um cadastro vinculado a esta conta."})
		return
	}
	customerID, _ := customers[0]["id"].(string)
	if strings.TrimSpace(customerID) == "" {
		writeCustomerPortalJSON(w, http.StatusBadGateway, customerPortalResponse{Error: "Cadastro do cliente inválido."})
		return
	}

	data := &domain.CustomerPortalData{Customer: customers[0]}
	data.Appliances, err = h.readRows(r, caller.Token, portalRowsPath("air_conditioners", customerID, ""))
	if err == nil {
		data.Services, err = h.readRows(r, caller.Token, portalRowsPath("services", customerID, "data_solicitacao.desc"))
	}
	if err == nil {
		data.Budgets, err = h.readRows(r, caller.Token, portalRowsPath("budgets", customerID, "created_at.desc"))
	}
	if err != nil {
		writeCustomerPortalJSON(w, http.StatusBadGateway, customerPortalResponse{Error: "Não foi possível carregar os dados do portal."})
		return
	}
	writeCustomerPortalJSON(w, http.StatusOK, customerPortalResponse{OK: true, Data: data})
}

func portalRowsPath(table, customerID, order string) string {
	query := url.Values{"select": {"*"}, "cliente_id": {"eq." + customerID}}
	if order != "" {
		query.Set("order", order)
	}
	return "/rest/v1/" + table + "?" + query.Encode()
}

func (h CustomerPortalHandler) readRows(r *http.Request, token, path string) ([]map[string]any, error) {
	result, err := h.Supabase.UserRequest(r.Context(), token, path, supabase.RequestOptions{Method: http.MethodGet})
	if err != nil {
		return nil, err
	}
	if result.StatusCode != http.StatusOK {
		return nil, errors.New("Supabase portal read was rejected")
	}
	rows := []map[string]any{}
	if err := json.Unmarshal(result.Body, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func writeCustomerPortalJSON(w http.ResponseWriter, status int, response customerPortalResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}

type CustomerAppliancesHandler struct{ Supabase *supabase.Client }

func (h CustomerAppliancesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
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
		"marca": true, "modelo": true, "btus": true, "tipo": true,
		"ambiente": true, "ultima_manutencao": true,
	})
	if !ok {
		return
	}
	var brand, applianceType string
	if json.Unmarshal(fields["marca"], &brand) != nil || strings.TrimSpace(brand) == "" || len([]rune(brand)) > 80 {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Informe uma marca válida."})
		return
	}
	if json.Unmarshal(fields["tipo"], &applianceType) != nil || !validCustomerApplianceType(applianceType) {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Selecione um tipo de aparelho válido."})
		return
	}
	var btus int
	if json.Unmarshal(fields["btus"], &btus) != nil || !validCustomerApplianceBTUs(btus) {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Selecione uma capacidade válida."})
		return
	}
	if raw := fields["modelo"]; len(raw) > 0 && string(raw) != "null" {
		var model string
		if json.Unmarshal(raw, &model) != nil || len([]rune(model)) > 120 {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "O modelo informado é inválido."})
			return
		}
	}
	if raw := fields["ultima_manutencao"]; len(raw) > 0 && string(raw) != "null" {
		var date string
		if json.Unmarshal(raw, &date) != nil || date != "" && !validCustomerServiceDate(date) {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "A data da última manutenção é inválida."})
			return
		}
	}
	query := url.Values{"select": {"id"}, "profile_id": {"eq." + caller.UserID}, "limit": {"2"}}
	result, err := h.Supabase.UserRequest(r.Context(), caller.Token, "/rest/v1/customers?"+query.Encode(), supabase.RequestOptions{Method: http.MethodGet})
	var customers []struct {
		ID string `json:"id"`
	}
	if err != nil || result.StatusCode != http.StatusOK || json.Unmarshal(result.Body, &customers) != nil || len(customers) != 1 || strings.TrimSpace(customers[0].ID) == "" {
		writeResourceJSON(w, http.StatusConflict, map[string]string{"error": "Não foi possível identificar o cadastro do cliente."})
		return
	}
	fields["marca"], _ = json.Marshal(strings.TrimSpace(brand))
	fields["tipo"], _ = json.Marshal(applianceType)
	fields["cliente_id"], _ = json.Marshal(customers[0].ID)
	forwardUserResource(w, r, h.Supabase, "/rest/v1/air_conditioners", http.MethodPost, fields, "", true)
}

func validCustomerApplianceType(value string) bool {
	switch value {
	case "Split Hi-Wall", "Inverter", "Cassete", "Piso Teto", "Janela", "Multi Split", "Portátil":
		return true
	default:
		return false
	}
}

func validCustomerApplianceBTUs(value int) bool {
	switch value {
	case 7000, 9000, 12000, 18000, 24000, 30000, 36000, 48000, 60000:
		return true
	default:
		return false
	}
}
