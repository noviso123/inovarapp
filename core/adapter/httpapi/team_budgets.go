package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"inovarapp/core/adapter/supabase"
	"inovarapp/core/adapter/whatsappqueue"
	"inovarapp/core/domain"
)

type TeamBudgetMutationHandler struct{ Supabase *supabase.Client }

type teamBudgetMutation struct {
	ID        string                 `json:"id"`
	Action    string                 `json:"acao"`
	Status    string                 `json:"status"`
	ServiceID string                 `json:"service_id"`
	Budget    *domain.BudgetEstimate `json:"orcamento"`
}

type mutableBudgetRow struct {
	ID             string  `json:"id"`
	Number         string  `json:"numero"`
	ClientID       string  `json:"cliente_id"`
	Date           string  `json:"data"`
	ValidUntil     string  `json:"validade"`
	Description    string  `json:"descricao"`
	ServiceType    string  `json:"tipo_servico"`
	LaborValue     float64 `json:"valor_mao_obra"`
	MaterialsValue float64 `json:"valor_material"`
	TotalValue     float64 `json:"valor_total"`
	Conditions     string  `json:"condicoes"`
	Status         string  `json:"status"`
	ServiceID      string  `json:"service_id"`
}

func (h TeamBudgetMutationHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch && r.Method != http.MethodDelete {
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
		writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Somente a equipe pode atualizar orçamentos"})
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if _, err := uuid.Parse(id); err != nil {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Orçamento não informado"})
		return
	}
	if r.Method == http.MethodDelete {
		q := url.Values{"id": {"eq." + id}}
		result, err := h.Supabase.UserRequest(r.Context(), caller.Token, "/rest/v1/budgets?"+q.Encode(), supabase.RequestOptions{Method: http.MethodDelete, Prefer: "return=minimal"})
		if err != nil || result.StatusCode < 200 || result.StatusCode >= 300 {
			writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível excluir o orçamento"})
			return
		}
		_ = whatsappqueue.CancelPendingBySource(r.Context(), h.Supabase, "orcamento", id, "Orçamento excluído; envio pendente cancelado")
		writeResourceJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var input teamBudgetMutation
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Atualização inválida"})
		return
	}
	if input.ID != "" && input.ID != id {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Orçamento divergente"})
		return
	}
	q := url.Values{"id": {"eq." + id}, "select": {"*"}}
	result, err := h.Supabase.UserRequest(r.Context(), caller.Token, "/rest/v1/budgets?"+q.Encode(), supabase.RequestOptions{Method: http.MethodGet})
	if err != nil || result.StatusCode != http.StatusOK {
		writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível carregar o orçamento"})
		return
	}
	row, ok := decodeMutableBudgetRow(result.Body)
	if !ok {
		writeResourceJSON(w, http.StatusNotFound, map[string]string{"error": "Orçamento não encontrado"})
		return
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal([]byte(row.Description), &fields)
	if fields == nil {
		fields = map[string]json.RawMessage{}
	}
	patch := map[string]any{}
	switch input.Action {
	case "STATUS":
		action, parseErr := domain.ParseBudgetResponseAction(input.Status)
		if parseErr != nil {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Status inválido"})
			return
		}
		status, transitionErr := domain.PlanBudgetResponse(caller.Role, action, row.Status, true, true)
		if transitionErr != nil {
			writeResourceJSON(w, http.StatusConflict, map[string]string{"error": "Este orçamento já foi respondido"})
			return
		}
		row.Status = status
		patch["status"] = status
		fields["resposta_cliente"], _ = json.Marshal(string(action))
	case "CANCELAR":
		if row.Status != "RASCUNHO" && row.Status != "ENVIADO" && row.Status != "APROVADO" {
			writeResourceJSON(w, http.StatusConflict, map[string]string{"error": "Somente um orçamento pendente ou aprovado pode ser cancelado"})
			return
		}
		paid := false
		_ = json.Unmarshal(fields["pago"], &paid)
		serviceID := firstStringValue(fields["service_id"], row.ServiceID)
		if paid || serviceID != "" {
			writeResourceJSON(w, http.StatusConflict, map[string]string{"error": "Este orçamento já foi recebido ou vinculado a uma ordem de serviço e não pode ser cancelado"})
			return
		}
		row.Status = "CANCELADO"
		patch["status"] = row.Status
		fields["cancelado_em"], _ = json.Marshal(time.Now().UTC().Format(time.RFC3339Nano))
	case "RECEBIDO":
		if row.Status != "APROVADO" {
			writeResourceJSON(w, http.StatusConflict, map[string]string{"error": "Somente orçamento aprovado pode ser marcado como recebido"})
			return
		}
		if input.Budget != nil && input.Budget.ID != "" && input.Budget.ID != id {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Orçamento divergente"})
			return
		}
		encoded, marshalErr := mergeReceivedBudgetMetadata(row, input.Budget, time.Now())
		if marshalErr != nil {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Metadados inválidos"})
			return
		}
		row.Description = encoded
	case "LINK_SERVICE":
		if row.Status != "APROVADO" {
			writeResourceJSON(w, http.StatusConflict, map[string]string{"error": "Somente orçamento aprovado pode ser convertido em OS"})
			return
		}
		if _, err := uuid.Parse(input.ServiceID); err != nil || row.ServiceID != "" {
			writeResourceJSON(w, http.StatusConflict, map[string]string{"error": "Orçamento já vinculado ou OS inválida"})
			return
		}
		row.ServiceID = input.ServiceID
		meta := map[string]json.RawMessage{}
		_ = json.Unmarshal([]byte(row.Description), &meta)
		meta["service_id"] = mustRawJSON(input.ServiceID)
		encoded, marshalErr := json.Marshal(meta)
		if marshalErr != nil {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Metadados inválidos"})
			return
		}
		row.Description = string(encoded)
		patch["service_id"] = input.ServiceID
	case "EDIT":
		if input.Budget == nil || input.Budget.ID != id {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Orçamento inválido"})
			return
		}
		budget := *input.Budget
		if strings.TrimSpace(budget.ClientName) == "" || len(budget.Items) == 0 || !validBudgetDate(budget.Date) || !validBudgetDate(budget.ValidUntil) || budget.Discount < 0 {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Confira cliente, itens, desconto e datas"})
			return
		}
		for i := range budget.Items {
			item := &budget.Items[i]
			if strings.TrimSpace(item.Description) == "" || item.Quantity <= 0 || item.UnitPrice < 0 || item.Category != domain.BudgetItemService && item.Category != domain.BudgetItemPart && item.Category != domain.BudgetItemMaterial {
				writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Confira descrição, quantidade, preço e categoria dos itens"})
				return
			}
			item.TotalPrice = domain.BudgetLineTotal(item.Quantity, item.UnitPrice)
		}
		totals := domain.CalculateBudgetTotals(budget.Items, budget.Discount)
		budget.TotalValue, budget.FinalValue = totals.Subtotal, totals.FinalValue
		write, err := supabase.MapLocalBudgetToSupabase(budget, row.Number, row.ClientID, "Equipe Inovar")
		if err != nil {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Não foi possível preparar o orçamento"})
			return
		}
		oldMeta := map[string]json.RawMessage{}
		_ = json.Unmarshal([]byte(row.Description), &oldMeta)
		newMeta := map[string]json.RawMessage{}
		_ = json.Unmarshal([]byte(write.Description), &newMeta)
		for key, value := range oldMeta {
			if _, exists := newMeta[key]; !exists {
				newMeta[key] = value
			}
		}
		for _, key := range []string{"assinatura", "assinatura_em", "pago", "pago_em", "valor_recebido", "service_id", "resposta_cliente"} {
			if value, ok := oldMeta[key]; ok {
				newMeta[key] = value
			}
		}
		encoded, err := json.Marshal(newMeta)
		if err != nil {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Metadados inválidos"})
			return
		}
		patch = map[string]any{
			"data": write.Date, "validade": write.ValidUntil, "descricao": string(encoded),
			"tipo_servico": write.ServiceType, "valor_mao_obra": write.LaborValue,
			"valor_material": write.MaterialsValue, "valor_total": write.TotalValue,
			"condicoes": write.Conditions,
		}
		row.Description = string(encoded)
	default:
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Ação inválida"})
		return
	}
	if input.Action == "STATUS" || input.Action == "CANCELAR" {
		encoded, marshalErr := json.Marshal(fields)
		if marshalErr != nil {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Metadados inválidos"})
			return
		}
		row.Description = string(encoded)
	}
	patch["descricao"] = row.Description
	updateQ := url.Values{"id": {"eq." + id}}
	updated, err := h.Supabase.UserRequest(r.Context(), caller.Token, "/rest/v1/budgets?"+updateQ.Encode(), supabase.RequestOptions{Method: http.MethodPatch, Body: patch, Prefer: "return=minimal"})
	if err != nil || (updated.StatusCode != http.StatusOK && updated.StatusCode != http.StatusNoContent) {
		writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível salvar o orçamento"})
		return
	}
	if strings.EqualFold(row.Status, "CANCELADO") || strings.EqualFold(input.Action, "CANCELAR") {
		_ = whatsappqueue.CancelPendingBySource(r.Context(), h.Supabase, "orcamento", id, "Orçamento cancelado; envio pendente cancelado")
	}
	writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true, "status": row.Status})
}

func decodeMutableBudgetRow(body []byte) (mutableBudgetRow, bool) {
	var rows []mutableBudgetRow
	if err := json.Unmarshal(body, &rows); err == nil {
		if len(rows) != 1 {
			return mutableBudgetRow{}, false
		}
		return rows[0], true
	}
	var row mutableBudgetRow
	if err := json.Unmarshal(body, &row); err != nil || row.ID == "" {
		return mutableBudgetRow{}, false
	}
	return row, true
}

func firstStringValue(value json.RawMessage, fallback string) string {
	var text string
	if json.Unmarshal(value, &text) == nil && strings.TrimSpace(text) != "" {
		return text
	}
	return fallback
}

func mergeReceivedBudgetMetadata(row mutableBudgetRow, snapshot *domain.BudgetEstimate, now time.Time) (string, error) {
	meta := map[string]json.RawMessage{}
	if err := json.Unmarshal(bytes.TrimSpace([]byte(row.Description)), &meta); err != nil || meta == nil {
		meta = map[string]json.RawMessage{}
	}
	var snapshotNumber, snapshotApplianceID, snapshotServiceID, snapshotSignature, snapshotSignedAt any
	receivedValue := row.TotalValue
	if snapshot != nil {
		snapshotNumber = snapshot.Number
		snapshotApplianceID = snapshot.ApplianceID
		snapshotServiceID = snapshot.ServiceID
		snapshotSignature = snapshot.Signature
		snapshotSignedAt = snapshot.SignedAt
		receivedValue = snapshot.FinalValue
	}
	fallbackNumber := firstNonEmptyBudgetString(stringValue(snapshotNumber), row.Number)
	meta["numero"] = mustRawJSON(firstStringValue(meta["numero"], fallbackNumber))
	meta["applianceId"] = preserveBudgetMetadataValue(meta["applianceId"], snapshotApplianceID)
	serviceID := firstNonEmptyBudgetString(row.ServiceID, stringValue(snapshotServiceID), firstStringValue(meta["service_id"], ""))
	meta["service_id"] = mustRawJSONNullable(serviceID)
	meta["assinatura"] = preserveBudgetMetadataValue(meta["assinatura"], snapshotSignature)
	meta["assinatura_em"] = preserveBudgetMetadataValue(meta["assinatura_em"], snapshotSignedAt)
	meta["pago"] = mustRawJSON(true)
	meta["pago_em"] = mustRawJSON(now.UTC().Format(time.RFC3339Nano))
	meta["valor_recebido"] = mustRawJSON(receivedValue)
	encoded, err := json.Marshal(meta)
	return string(encoded), err
}

func preserveBudgetMetadataValue(current json.RawMessage, fallback any) json.RawMessage {
	var value string
	if json.Unmarshal(current, &value) == nil && strings.TrimSpace(value) != "" {
		return current
	}
	return mustRawJSON(fallback)
}

func mustRawJSONNullable(value string) json.RawMessage {
	if value == "" {
		return json.RawMessage("null")
	}
	return mustRawJSON(value)
}

func firstNonEmptyBudgetString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" && value != "null" {
			return value
		}
	}
	return ""
}
