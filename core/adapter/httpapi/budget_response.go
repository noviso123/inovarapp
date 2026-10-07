package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"inovarapp/core/adapter/supabase"
	"inovarapp/core/adapter/whatsapp"
	"inovarapp/core/adapter/whatsappqueue"
	"inovarapp/core/domain"
)

const budgetResponseConfigPath = "/storage/v1/object/documentos-inovar/config/tecnico.json"

type BudgetResponseHandler struct {
	Supabase         *supabase.Client
	Notifier         PushNotifier
	WhatsApp         whatsapp.Sender
	WhatsAppDefaults whatsapp.Config
	Now              func() time.Time
}

type budgetResponseRequest struct {
	BudgetID  string `json:"orcamento_id"`
	Action    string `json:"acao"`
	Signature string `json:"assinatura"`
}

type budgetResponseRow struct {
	ID          string `json:"id"`
	ClientID    string `json:"cliente_id"`
	Description string `json:"descricao"`
	Status      string `json:"status"`
}

type budgetResponseCustomer struct {
	ID       string `json:"id"`
	WhatsApp string `json:"whatsapp"`
}

type budgetResponseMetadata map[string]json.RawMessage

type budgetResponseConfig struct {
	Messages     map[string]string `json:"mensagensWhats"`
	BusinessName string            `json:"businessName"`
	Phone        string            `json:"phone"`
}

func (h BudgetResponseHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAccountJSON(w, http.StatusMethodNotAllowed, accountResponse{Error: "Método não permitido"})
		return
	}
	if h.Supabase == nil || h.Supabase.RequireServiceRole() != nil {
		writeAccountJSON(w, http.StatusInternalServerError, accountResponse{Error: "Backend não configurado"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var request budgetResponseRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeAccountJSON(w, http.StatusBadRequest, accountResponse{Error: "Solicitação inválida"})
		return
	}
	if strings.TrimSpace(request.BudgetID) == "" {
		writeAccountJSON(w, http.StatusBadRequest, accountResponse{Error: "Orçamento não informado"})
		return
	}
	action, err := domain.ParseBudgetResponseAction(request.Action)
	if err != nil {
		writeAccountJSON(w, http.StatusBadRequest, accountResponse{Error: "Ação inválida"})
		return
	}
	if len(request.Signature) > 400000 {
		writeAccountJSON(w, http.StatusRequestEntityTooLarge, accountResponse{Error: "Assinatura muito grande"})
		return
	}
	caller, err := h.Supabase.AuthenticateCaller(r.Context(), supabase.BearerToken(r.Header.Get("Authorization")))
	if err != nil {
		writeAccountJSON(w, http.StatusUnauthorized, accountResponse{Error: "Não autenticado"})
		return
	}

	budget, ok := h.readBudgetAsCaller(r.Context(), caller, request.BudgetID)
	if !ok {
		writeAccountJSON(w, http.StatusNotFound, accountResponse{Error: "Orçamento não encontrado para este usuário"})
		return
	}
	ownsBudget := caller.Role != domain.RoleCustomer
	if caller.Role == domain.RoleCustomer {
		customerID, err := h.callerCustomerID(r.Context(), caller)
		ownsBudget = err == nil && customerID != "" && customerID == budget.ClientID
	}
	newStatus, err := domain.PlanBudgetResponse(caller.Role, action, budget.Status, ownsBudget, request.Signature != "")
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrBudgetNotOwned):
			writeAccountJSON(w, http.StatusForbidden, accountResponse{Error: "Este orçamento não pertence ao seu cadastro"})
		case errors.Is(err, domain.ErrBudgetSignatureRequired):
			writeAccountJSON(w, http.StatusBadRequest, accountResponse{Error: "Assinatura digital obrigatoria para aprovacao pelo cliente"})
		case errors.Is(err, domain.ErrBudgetAlreadyAnswered):
			writeAccountJSON(w, http.StatusConflict, accountResponse{Error: "Este orçamento já foi " + strings.ToLower(budget.Status)})
		default:
			writeAccountJSON(w, http.StatusBadRequest, accountResponse{Error: "Ação inválida"})
		}
		return
	}

	metadata := make(budgetResponseMetadata)
	if budget.Description != "" {
		_ = json.Unmarshal([]byte(budget.Description), &metadata)
		if metadata == nil {
			metadata = make(budgetResponseMetadata)
		}
	}
	var signedAt any
	var signedAtRaw json.RawMessage
	if request.Signature != "" {
		at := h.now().UTC().Format("2006-01-02T15:04:05.000Z")
		signedAt, signedAtRaw = at, mustRawJSON(at)
	} else {
		signedAtRaw = json.RawMessage("null")
	}
	var signatureValue any
	if request.Signature != "" {
		signatureValue = request.Signature
	}
	metadata["assinatura"] = mustRawJSON(signatureValue)
	metadata["assinatura_em"] = signedAtRaw
	metadata["resposta_cliente"] = mustRawJSON(action)
	encodedMetadata, err := json.Marshal(metadata)
	if err != nil {
		writeAccountJSON(w, http.StatusInternalServerError, accountResponse{Error: "Falha ao atualizar o orçamento"})
		return
	}
	query := url.Values{}
	query.Set("id", "eq."+request.BudgetID)
	updated, err := h.Supabase.ServiceRequest(r.Context(), "/rest/v1/budgets?"+query.Encode(), supabase.RequestOptions{
		Method: http.MethodPatch, Prefer: "return=minimal",
		Body: map[string]string{"status": newStatus, "descricao": string(encodedMetadata)},
	})
	if err != nil || (updated.StatusCode != http.StatusOK && updated.StatusCode != http.StatusNoContent) {
		writeAccountJSON(w, http.StatusInternalServerError, accountResponse{Error: "Falha ao atualizar o orçamento"})
		return
	}
	whatsAppQueued := h.notifyBudgetResponse(r.Context(), action, budget, metadata)
	if h.Notifier != nil {
		if action == "APROVAR" {
			h.Notifier.NotifyTeam(r.Context(), "Orçamento aprovado pelo cliente!", "Assinatura registrada — agende o serviço.")
		} else {
			h.Notifier.NotifyTeam(r.Context(), "Orçamento recusado", "O cliente recusou a proposta.")
		}
	}
	message := "Orçamento aprovado e assinado! A Inovar foi notificada."
	if action == "RECUSAR" {
		message = "Orçamento recusado. A Inovar foi notificada."
	}
	if whatsAppQueued {
		message += " A notificação do WhatsApp está na fila e será acompanhada até o serviço aceitar o envio."
	} else {
		message += " O status foi salvo, mas a notificação do WhatsApp não entrou na fila."
	}
	writeBudgetResponseJSON(w, http.StatusOK, map[string]any{
		"ok": true, "status": newStatus, "assinado_em": signedAt,
		"mensagem": message, "whatsapp_enfileirado": whatsAppQueued,
	})
}

func (h BudgetResponseHandler) readBudgetAsCaller(ctx context.Context, caller supabase.Caller, budgetID string) (budgetResponseRow, bool) {
	rows, status, err := h.readBudget(ctx, budgetID, supabase.RequestOptions{Method: http.MethodGet}, caller.Token)
	if err == nil && status == http.StatusOK && len(rows) > 0 {
		return rows[0], true
	}
	if caller.Role != domain.RoleCustomer {
		return budgetResponseRow{}, false
	}
	// The legacy handler only consults the privileged table after a successful
	// RLS query returned no visible row. Transport and HTTP errors must not turn
	// the privileged lookup result into an alternate authorization path.
	if err != nil || status != http.StatusOK || len(rows) != 0 {
		return budgetResponseRow{}, false
	}
	customerID, err := h.callerCustomerID(ctx, caller)
	if err != nil || customerID == "" {
		return budgetResponseRow{}, false
	}
	rows, status, err = h.readBudget(ctx, budgetID, supabase.RequestOptions{Method: http.MethodGet}, "")
	if err != nil || status != http.StatusOK || len(rows) == 0 || rows[0].ClientID != customerID {
		return budgetResponseRow{}, false
	}
	return rows[0], true
}

func (h BudgetResponseHandler) readBudget(ctx context.Context, budgetID string, options supabase.RequestOptions, token string) ([]budgetResponseRow, int, error) {
	query := url.Values{}
	query.Set("id", "eq."+budgetID)
	query.Set("select", "id,cliente_id,descricao,status")
	path := "/rest/v1/budgets?" + query.Encode()
	var result supabase.Result
	var err error
	if token != "" {
		result, err = h.Supabase.UserRequest(ctx, token, path, options)
	} else {
		result, err = h.Supabase.ServiceRequest(ctx, path, options)
	}
	if err != nil {
		return nil, 0, err
	}
	var rows []budgetResponseRow
	if result.StatusCode == http.StatusOK {
		if err := json.Unmarshal(result.Body, &rows); err != nil {
			return nil, result.StatusCode, err
		}
	}
	return rows, result.StatusCode, nil
}

func (h BudgetResponseHandler) callerCustomerID(ctx context.Context, caller supabase.Caller) (string, error) {
	query := url.Values{}
	query.Set("profile_id", "eq."+caller.UserID)
	query.Set("select", "id")
	result, err := h.Supabase.UserRequest(ctx, caller.Token, "/rest/v1/customers?"+query.Encode(), supabase.RequestOptions{Method: http.MethodGet})
	if err != nil {
		return "", err
	}
	if result.StatusCode != http.StatusOK {
		return "", fmt.Errorf("customer lookup failed with status %d", result.StatusCode)
	}
	var customers []budgetResponseCustomer
	if err := json.Unmarshal(result.Body, &customers); err != nil {
		return "", err
	}
	if len(customers) != 1 {
		return "", nil
	}
	return customers[0].ID, nil
}

func (h BudgetResponseHandler) notifyBudgetResponse(ctx context.Context, action domain.BudgetResponseAction, budget budgetResponseRow, metadata budgetResponseMetadata) bool {
	config := h.readResponseConfig(ctx)
	values := map[string]string{
		"cliente": metadataString(metadata, "clientName", "Cliente"),
		"numero":  metadataString(metadata, "numero", ""),
		"valor":   "",
		"empresa": config.BusinessName,
		"app":     domain.AppPublicURL,
	}
	if values["empresa"] == "" {
		values["empresa"] = "Inovar Refrigeração"
	}
	if raw := metadata["totalValue"]; len(raw) != 0 {
		var total float64
		if json.Unmarshal(raw, &total) == nil {
			values["valor"] = "R$ " + strconv.FormatFloat(total, 'f', 2, 64)
		}
	}
	technicalKey, technicalMessage := "orcamento_recusado_tecnico", "⚠️ *Orçamento recusado.*\n\nO cliente *{{cliente}}* recusou a proposta.\n\n— InovarApp"
	if action == "APROVAR" {
		technicalKey, technicalMessage = "orcamento_aprovado_tecnico", "✅ *Orçamento aprovado e assinado!*\n\nO cliente *{{cliente}}* aprovou a proposta (assinatura registrada). Hora de agendar o serviço!\n\n— InovarApp"
	}
	technicalMessage = expandAccountMessage(config.Messages, technicalKey, technicalMessage, values)
	queued := false
	if config.Phone != "" {
		queued = h.enqueueBudgetResponse(ctx, "orcamento-resposta:"+budget.ID+":"+string(action)+":equipe", "orcamento_resposta_equipe", config.Phone, technicalMessage, budget.ID) == nil
	}
	if action == "APROVAR" {
		customerQuery := url.Values{}
		customerQuery.Set("id", "eq."+budget.ClientID)
		customerQuery.Set("select", "id,whatsapp")
		result, err := h.Supabase.ServiceRequest(ctx, "/rest/v1/customers?"+customerQuery.Encode(), supabase.RequestOptions{Method: http.MethodGet})
		if err != nil || result.StatusCode != http.StatusOK {
			return queued
		}
		var customers []budgetResponseCustomer
		if json.Unmarshal(result.Body, &customers) != nil || len(customers) == 0 || customers[0].WhatsApp == "" {
			return queued
		}
		message := expandAccountMessage(config.Messages, "orcamento_aprovado_cliente",
			"✅ *Recebemos sua aprovação*, {{cliente}}!\n\nProposta nº {{numero}} ({{valor}}) confirmada com assinatura digital.\n\nNossa equipe vai entrar em contato para agendar o serviço. Qualquer dúvida, chame por aqui! ❄️\n\n*{{empresa}}*", values)
		if h.enqueueBudgetResponse(ctx, "orcamento-resposta:"+budget.ID+":"+string(action)+":cliente", "orcamento_resposta_cliente", customers[0].WhatsApp, message, budget.ID) == nil {
			queued = true
		}
	}
	return queued
}

func (h BudgetResponseHandler) enqueueBudgetResponse(ctx context.Context, key, event, phone, message, budgetID string) error {
	return whatsappqueue.Enqueue(ctx, h.Supabase, whatsappqueue.EnqueueInput{
		IdempotencyKey: key, EventType: event, RecipientPhone: phone, MessageText: message,
		SourceEntityType: "orcamento", SourceEntityID: budgetID,
	})
}

func (h BudgetResponseHandler) readResponseConfig(ctx context.Context) budgetResponseConfig {
	config := budgetResponseConfig{}
	result, err := h.Supabase.ServiceRequest(ctx, budgetResponseConfigPath, supabase.RequestOptions{Method: http.MethodGet})
	if err == nil && result.StatusCode >= http.StatusOK && result.StatusCode < http.StatusMultipleChoices {
		var stored budgetResponseConfig
		if json.Unmarshal(result.Body, &stored) == nil {
			config.Messages, config.Phone, config.BusinessName = stored.Messages, stored.Phone, stored.BusinessName
		}
	}
	return config
}

func (h BudgetResponseHandler) sendBudgetWhatsApp(ctx context.Context, config whatsapp.Config, phone, message string) error {
	sendContext, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	return h.WhatsApp.SendText(sendContext, config, phone, message)
}

func (h BudgetResponseHandler) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

func metadataString(metadata budgetResponseMetadata, key, fallback string) string {
	var value string
	if json.Unmarshal(metadata[key], &value) == nil && strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func expandAccountMessage(messages map[string]string, key, fallback string, values map[string]string) string {
	message := domain.ResolveWhatsAppTemplate(messages, key)
	if message == "" {
		message = fallback
	}
	return domain.ApplyWhatsAppPlaceholders(message, values)
}

func mustRawJSON(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage("null")
	}
	return encoded
}

func writeBudgetResponseJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
