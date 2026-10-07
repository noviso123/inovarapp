package httpapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"inovarapp/core/adapter/supabase"
	"inovarapp/core/domain"
)

// CustomersHandler ports the staff customer registry while keeping both the
// customer and appliance reads/writes under the caller JWT and Supabase RLS.
type CustomersHandler struct{ Supabase *supabase.Client }

var customerFields = map[string]bool{
	"nome": true, "cpf_cnpj": true, "whatsapp": true, "endereco": true,
	"numero": true, "complemento": true, "bairro": true, "cidade": true,
	"estado": true, "observacoes": true, "ativo": true,
}

var customerCreateFields = func() map[string]bool {
	fields := make(map[string]bool, len(customerFields)+1)
	for key, allowed := range customerFields {
		fields[key] = allowed
	}
	fields["id"] = true
	return fields
}()

func (h CustomersHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
		writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Somente a equipe pode acessar o cadastro de clientes"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.list(w, r, caller)
	case http.MethodPost:
		fields, ok := decodeResourceFields(w, r, customerCreateFields)
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
		if fields["nome"] == nil {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Informe o nome do cliente"})
			return
		}
		forwardUserResource(w, r, h.Supabase, "/rest/v1/customers", http.MethodPost, fields, "return=representation", true)
	case http.MethodPatch, http.MethodPut:
		mutation, ok := decodeResourceMutation(w, r, customerFields)
		if !ok {
			return
		}
		if len(mutation.Fields) == 0 {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Nenhum campo válido para atualizar"})
			return
		}
		forwardUserResource(w, r, h.Supabase, resourceByID("customers", "id", mutation.ID), http.MethodPatch, mutation.Fields, "return=minimal", false)
	case http.MethodDelete:
		if caller.Role != domain.RoleAdmin {
			writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Somente ADMIN pode excluir clientes"})
			return
		}
		mutation, ok := decodeResourceMutation(w, r, nil)
		if !ok {
			return
		}
		forwardUserResource(w, r, h.Supabase, resourceByID("customers", "id", mutation.ID), http.MethodDelete, nil, "return=minimal", false)
	default:
		writeResourceJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Método não permitido"})
	}
}

// TeamBudgetsHandler returns database-backed budgets to the authenticated
// staff client. RLS is evaluated with the caller JWT, never the service role.
type TeamBudgetsHandler struct{ Supabase *supabase.Client }

func (h TeamBudgetsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	if !isTeamRole(caller.Role) {
		writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Somente a equipe pode acessar os orçamentos"})
		return
	}
	if r.Method == http.MethodPost {
		h.create(w, r, caller.Token)
		return
	}
	path := "/rest/v1/budgets?" + url.Values{"select": {"*"}, "order": {"created_at.desc"}}.Encode()
	result, err := h.Supabase.UserRequest(r.Context(), caller.Token, path, supabase.RequestOptions{Method: http.MethodGet})
	if err != nil || result.StatusCode != http.StatusOK {
		writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível carregar os orçamentos"})
		return
	}
	var rows []supabase.SupabaseBudget
	if err := json.Unmarshal(result.Body, &rows); err != nil {
		writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Resposta de orçamentos inválida"})
		return
	}
	budgets := make([]domain.BudgetEstimate, 0, len(rows))
	for _, row := range rows {
		budgets = append(budgets, supabase.MapSupabaseBudgetToLocal(row))
	}
	w.Header().Set("Cache-Control", "no-store")
	writeResourceJSON(w, http.StatusOK, budgets)
}

func (h TeamBudgetsHandler) create(w http.ResponseWriter, r *http.Request, token string) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var budget domain.BudgetEstimate
	if err := json.NewDecoder(r.Body).Decode(&budget); err != nil {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Orçamento inválido"})
		return
	}
	if budget.ClientID == "" || budget.ClientID == "avulso" || strings.TrimSpace(budget.ClientName) == "" || len(budget.Items) == 0 || !validBudgetDate(budget.Date) || !validBudgetDate(budget.ValidUntil) {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Informe cliente, itens e datas válidas para o orçamento"})
		return
	}
	if budget.ID != "" && uuid.Validate(budget.ID) != nil {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Identificador do orçamento inválido"})
		return
	}
	for i := range budget.Items {
		item := &budget.Items[i]
		if strings.TrimSpace(item.Description) == "" || item.Quantity <= 0 || item.UnitPrice < 0 || (item.Category != domain.BudgetItemService && item.Category != domain.BudgetItemPart && item.Category != domain.BudgetItemMaterial) {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Confira descrição, quantidade, preço e categoria dos itens"})
			return
		}
		item.TotalPrice = domain.BudgetLineTotal(item.Quantity, item.UnitPrice)
	}
	totals := domain.CalculateBudgetTotals(budget.Items, budget.Discount)
	budget.TotalValue, budget.FinalValue = totals.Subtotal, totals.FinalValue
	budget.Status = domain.BudgetPending
	number := budget.Number
	if number == "" {
		number = h.nextBudgetNumber(r, token)
	}
	write, err := supabase.MapLocalBudgetToSupabase(budget, number, budget.ClientID, "Equipe Inovar")
	if err != nil {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Não foi possível preparar o orçamento"})
		return
	}
	result, err := h.Supabase.UserRequest(r.Context(), token, "/rest/v1/budgets", supabase.RequestOptions{Method: http.MethodPost, Body: write, Prefer: "return=representation"})
	if err != nil || result.StatusCode < http.StatusOK || result.StatusCode >= http.StatusMultipleChoices {
		writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível salvar o orçamento"})
		return
	}
	var rows []supabase.SupabaseBudget
	if json.Unmarshal(result.Body, &rows) != nil || len(rows) != 1 {
		var row supabase.SupabaseBudget
		if json.Unmarshal(result.Body, &row) != nil || row.ID == "" {
			writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "O orçamento foi salvo, mas a resposta não pôde ser lida"})
			return
		}
		rows = []supabase.SupabaseBudget{row}
	}
	saved := supabase.MapSupabaseBudgetToLocal(rows[0])
	saved.ClientName, saved.ClientPhone = budget.ClientName, budget.ClientPhone
	saved.ClientAddress, saved.ClientDocument = budget.ClientAddress, budget.ClientDocument
	saved.EquipmentName, saved.ApplianceDescription = budget.EquipmentName, budget.ApplianceDescription
	w.Header().Set("Cache-Control", "no-store")
	writeResourceJSON(w, http.StatusCreated, saved)
}

func (h TeamBudgetsHandler) nextBudgetNumber(r *http.Request, token string) string {
	result, err := h.Supabase.UserRequest(r.Context(), token, "/rest/v1/rpc/proximo_numero_orcamento", supabase.RequestOptions{Method: http.MethodPost, Body: map[string]any{}})
	if err == nil && result.StatusCode == http.StatusOK {
		var number string
		if json.Unmarshal(result.Body, &number) == nil && number != "" {
			return number
		}
	}
	return time.Now().Format("2006") + "-" + strconv.FormatInt(time.Now().UnixMilli()%10000, 10)
}

func validBudgetDate(value string) bool {
	date, err := time.Parse("2006-01-02", value)
	return err == nil && date.Format("2006-01-02") == value
}

func (h CustomersHandler) list(w http.ResponseWriter, r *http.Request, caller supabase.Caller) {
	var customersResult, appliancesResult supabase.Result
	var customersErr, appliancesErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		customersResult, customersErr = h.Supabase.UserRequest(r.Context(), caller.Token, "/rest/v1/customers?"+url.Values{
			"select": {"*"}, "order": {"nome.asc"},
		}.Encode(), supabase.RequestOptions{Method: http.MethodGet})
	}()
	go func() {
		defer wg.Done()
		appliancesResult, appliancesErr = h.Supabase.UserRequest(r.Context(), caller.Token, "/rest/v1/air_conditioners?select=*", supabase.RequestOptions{Method: http.MethodGet})
	}()
	wg.Wait()
	if customersErr != nil || customersResult.StatusCode != http.StatusOK {
		writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível carregar os clientes"})
		return
	}
	var customers []map[string]any
	if err := json.Unmarshal(customersResult.Body, &customers); err != nil {
		writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Resposta de clientes inválida"})
		return
	}
	if appliancesErr != nil || appliancesResult.StatusCode != http.StatusOK {
		writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível carregar os aparelhos"})
		return
	}
	var appliances []map[string]any
	if err := json.Unmarshal(appliancesResult.Body, &appliances); err != nil {
		writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Resposta de aparelhos inválida"})
		return
	}
	byClient := make(map[string][]map[string]any, len(customers))
	for _, appliance := range appliances {
		id, _ := appliance["cliente_id"].(string)
		if id != "" {
			byClient[id] = append(byClient[id], appliance)
		}
	}
	for _, customer := range customers {
		id, _ := customer["id"].(string)
		customer["appliances"] = byClient[id]
	}
	writeResourceJSON(w, http.StatusOK, customers)
}
