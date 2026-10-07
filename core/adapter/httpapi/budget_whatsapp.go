package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"inovarapp/core/adapter/pdf"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/adapter/whatsapp"
	"inovarapp/core/adapter/whatsappqueue"
	"inovarapp/core/domain"
)

type BudgetWhatsAppHandler struct {
	Supabase *supabase.Client
	Sender   whatsapp.Sender
	Defaults whatsapp.Config
}

type budgetWhatsAppRequest struct {
	BudgetID string `json:"budget_id"`
}

var budgetWhatsAppSafePath = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func (h BudgetWhatsAppHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	if !isTeamRole(caller.Role) {
		writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Somente a equipe Inovar pode enviar propostas"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	var request budgetWhatsAppRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Informe um orçamento válido"})
		return
	}
	if _, err := uuid.Parse(strings.TrimSpace(request.BudgetID)); err != nil {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Informe um orçamento válido"})
		return
	}
	budgetHandler := BudgetPDFHandler{Supabase: h.Supabase}
	budget, err := budgetHandler.loadBudgetForPDF(r.Context(), caller, request.BudgetID)
	if err != nil {
		writeResourceJSON(w, http.StatusNotFound, map[string]string{"error": "Orçamento não encontrado"})
		return
	}
	if _, valid := whatsapp.NormalizePhone(budget.ClientPhone); !valid {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Este orçamento não tem um telefone válido para contato"})
		return
	}
	profile := budgetHandler.loadBudgetPDFProfile(r.Context())
	output, err := pdf.BuildBudgetPDF(budget, profile)
	if err != nil {
		writeResourceJSON(w, http.StatusInternalServerError, map[string]string{"error": "Não foi possível gerar o PDF da proposta"})
		return
	}
	name := budgetWhatsAppSafePath.ReplaceAllString(budget.ClientName, "_")
	if name == "" {
		name = "Cliente"
	}
	path := fmt.Sprintf("os-orcamentos/%s/%s-ORCAMENTO_%s.pdf", time.Now().UTC().Format("2006-01-02"), uuid.NewString(), name)
	if err := h.Supabase.UploadPrivateObject(r.Context(), path, "application/pdf", output); err != nil {
		writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível armazenar o PDF da proposta"})
		return
	}
	signedURL, err := h.Supabase.CreatePrivateSignedURL(r.Context(), path, 7*24*60*60)
	if err != nil {
		writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível criar o link privado da proposta"})
		return
	}
	message := domain.WhatsAppBudgetMessage(budget, profile)
	manualURL := budgetManualWhatsAppURL(budget.ClientPhone, message+"\n\n📄 *Proposta em PDF:* "+signedURL)
	fileName := "PROPOSTA_" + name + ".pdf"
	expires := budgetQueueExpiry(budget.ValidUntil)
	if err := whatsappqueue.Enqueue(r.Context(), h.Supabase, whatsappqueue.EnqueueInput{
		IdempotencyKey: "orcamento-pdf:" + budget.ID + ":" + time.Now().UTC().Format("200601021504"),
		EventType:      "orcamento_gerado", RecipientPhone: budget.ClientPhone, MessageText: message,
		DocumentStoragePath: path, DocumentName: fileName, ExpiresAt: &expires,
		SourceEntityType: "orcamento", SourceEntityID: budget.ID,
	}); err != nil {
		writeResourceJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "enfileirado": false, "error": "Orçamento salvo, mas o envio não entrou na fila. Verifique a migração da fila no Supabase.", "whatsapp_url": manualURL})
		return
	}
	writeResourceJSON(w, http.StatusOK, map[string]any{
		"ok": true, "configurado": true, "enfileirado": true, "status": "pendente",
		"mensagem": "Orçamento e PDF foram registrados na fila automática do WhatsApp.",
	})
}

func budgetQueueExpiry(validUntil string) time.Time {
	location, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		location = time.FixedZone("BRT", -3*60*60)
	}
	if due, parseErr := time.ParseInLocation("2006-01-02", strings.TrimSpace(validUntil), location); parseErr == nil {
		return time.Date(due.Year(), due.Month(), due.Day(), 23, 59, 59, 0, location).UTC()
	}
	return time.Now().UTC().Add(8 * 24 * time.Hour)
}

func budgetManualWhatsAppURL(phone, message string) string {
	number, valid := whatsapp.NormalizePhone(phone)
	if !valid {
		return ""
	}
	return "https://wa.me/" + number + "?text=" + url.QueryEscape(message)
}
