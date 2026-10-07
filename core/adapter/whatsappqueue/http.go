package whatsappqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/domain"
)

type HTTPHandler struct {
	Supabase        *supabase.Client
	SyncMaintenance func(context.Context, time.Time) error
}

type queueView struct {
	ID            string         `json:"id"`
	EventType     string         `json:"event_type"`
	Recipient     string         `json:"recipient"`
	Preview       string         `json:"preview,omitempty"`
	Sensitive     bool           `json:"sensitive"`
	ScheduledAt   time.Time      `json:"scheduled_at"`
	NextAttemptAt time.Time      `json:"next_attempt_at"`
	ExpiresAt     *time.Time     `json:"expires_at,omitempty"`
	Status        string         `json:"status"`
	AttemptCount  int            `json:"attempt_count"`
	LastError     string         `json:"last_error,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	SentAt        *time.Time     `json:"sent_at,omitempty"`
	SourceEntity  string         `json:"source_entity,omitempty"`
	Context       map[string]any `json:"context,omitempty"`
	History       []attemptView  `json:"history,omitempty"`
}

type attemptView struct {
	Number     int       `json:"attempt_number"`
	Status     string    `json:"status"`
	Error      string    `json:"error_message,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}

func (h HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	if h.Supabase == nil {
		queueJSON(w, http.StatusInternalServerError, map[string]string{"error": "Banco de dados não configurado"})
		return
	}
	caller, err := h.Supabase.AuthenticateCaller(r.Context(), supabase.BearerToken(r.Header.Get("Authorization")))
	if err != nil {
		queueJSON(w, http.StatusUnauthorized, map[string]string{"error": "Sessão inválida"})
		return
	}
	if caller.Role != domain.RoleAdmin && caller.Role != domain.RoleTechnician {
		queueJSON(w, http.StatusForbidden, map[string]string{"error": "Somente técnicos e administradores podem consultar esta fila"})
		return
	}
	if err := h.Supabase.RequireServiceRole(); err != nil {
		queueJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Fila indisponível: configure a credencial de serviço do Supabase no backend"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		if h.SyncMaintenance != nil {
			if err := h.SyncMaintenance(r.Context(), time.Now()); err != nil {
				queueJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível registrar os próximos lembretes de manutenção. Atualize a fila para tentar novamente.", "etapa": "agendar_manutencao"})
				return
			}
		}
		h.list(w, r.Context())
	case http.MethodPost:
		h.action(w, r)
	default:
		queueJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Método não permitido"})
	}
}

func (h HTTPHandler) list(w http.ResponseWriter, ctx context.Context) {
	selectFields := "id,event_type,recipient_phone,message_text,sensitive,scheduled_at,next_attempt_at,expires_at,status,attempt_count,last_error,created_at,sent_at,source_entity_type,metadata"
	activeQuery := url.Values{"select": {selectFields}, "status": {"in.(pendente,processando)"}, "order": {"scheduled_at.asc"}, "limit": {"500"}}
	historyQuery := url.Values{"select": {selectFields}, "status": {"in.(enviado,falha,cancelado,expirado)"}, "order": {"created_at.desc"}, "limit": {"300"}}
	activeResult, activeErr := h.Supabase.ServiceRequest(ctx, tablePath+"?"+activeQuery.Encode(), supabase.RequestOptions{Method: http.MethodGet})
	if activeErr != nil || activeResult.StatusCode != http.StatusOK {
		queueDependencyFailure(w, "mensagens pendentes", "mensagens_pendentes", activeResult, activeErr)
		return
	}
	historyResult, historyErr := h.Supabase.ServiceRequest(ctx, tablePath+"?"+historyQuery.Encode(), supabase.RequestOptions{Method: http.MethodGet})
	if historyErr != nil || historyResult.StatusCode != http.StatusOK {
		queueDependencyFailure(w, "histórico da fila", "historico_fila", historyResult, historyErr)
		return
	}
	var messages, historyMessages []Message
	if err := json.Unmarshal(activeResult.Body, &messages); err != nil {
		queueJSON(w, http.StatusBadGateway, map[string]string{"error": "Resposta inválida da fila no Supabase"})
		return
	}
	if err := json.Unmarshal(historyResult.Body, &historyMessages); err == nil {
		messages = append(messages, historyMessages...)
	}
	countsResult, countsErr := h.Supabase.ServiceRequest(ctx, "/rest/v1/rpc/whatsapp_message_queue_counts", supabase.RequestOptions{Method: http.MethodPost, Body: map[string]any{}})
	counts := map[string]int{"pendente": 0, "processando": 0, "enviado": 0, "expirado": 0, "falha": 0, "cancelado": 0}
	if countsErr != nil || countsResult.StatusCode < 200 || countsResult.StatusCode >= 300 {
		queueDependencyFailure(w, "contagens da fila", "contagens_fila", countsResult, countsErr)
		return
	}
	if json.Unmarshal(countsResult.Body, &counts) != nil {
		queueJSON(w, http.StatusBadGateway, map[string]string{"error": "O Supabase retornou contagens inválidas para a fila.", "etapa": "contagens_fila"})
		return
	}
	historyByMessage := map[string][]attemptView{}
	if len(messages) > 0 {
		ids := make([]string, 0, len(messages))
		for _, message := range messages {
			ids = append(ids, message.ID)
		}
		attemptQuery := url.Values{
			"select":     {"message_id,attempt_number,status,error_message,started_at,finished_at"},
			"message_id": {"in.(" + strings.Join(ids, ",") + ")"},
			"order":      {"started_at.desc"}, "limit": {"1000"},
		}
		attemptsResult, attemptsErr := h.Supabase.ServiceRequest(ctx, "/rest/v1/whatsapp_message_attempts?"+attemptQuery.Encode(), supabase.RequestOptions{Method: http.MethodGet})
		if attemptsErr != nil || attemptsResult.StatusCode != http.StatusOK {
			queueDependencyFailure(w, "histórico de tentativas", "tentativas", attemptsResult, attemptsErr)
			return
		}
		var attempts []struct {
			MessageID  string    `json:"message_id"`
			Number     int       `json:"attempt_number"`
			Status     string    `json:"status"`
			Error      string    `json:"error_message"`
			StartedAt  time.Time `json:"started_at"`
			FinishedAt time.Time `json:"finished_at"`
		}
		if json.Unmarshal(attemptsResult.Body, &attempts) != nil {
			queueJSON(w, http.StatusBadGateway, map[string]string{"error": "O Supabase retornou um histórico de tentativas inválido.", "etapa": "tentativas"})
			return
		}
		for _, attempt := range attempts {
			historyByMessage[attempt.MessageID] = append(historyByMessage[attempt.MessageID], attemptView{
				Number: attempt.Number, Status: attempt.Status, Error: attempt.Error,
				StartedAt: attempt.StartedAt, FinishedAt: attempt.FinishedAt,
			})
		}
	}
	views := make([]queueView, 0, len(messages))
	for _, message := range messages {
		view := queueView{
			ID: message.ID, EventType: message.EventType, Recipient: maskPhone(message.RecipientPhone),
			Sensitive: message.Sensitive, ScheduledAt: message.ScheduledAt, NextAttemptAt: message.NextAttemptAt,
			ExpiresAt: message.ExpiresAt,
			Status:    message.Status, AttemptCount: message.AttemptCount, LastError: message.LastError,
			CreatedAt: message.CreatedAt, SentAt: message.SentAt, SourceEntity: message.SourceEntityType,
			History: historyByMessage[message.ID], Context: message.Metadata,
		}
		if !message.Sensitive {
			view.Preview = preview(message.MessageText)
		}
		views = append(views, view)
	}
	queueJSON(w, http.StatusOK, map[string]any{"items": views, "counts": counts, "loaded_at": time.Now().UTC()})
}

func queueDependencyFailure(w http.ResponseWriter, resource, stage string, result supabase.Result, err error) {
	message := ""
	status := result.StatusCode
	switch {
	case err != nil:
		message = "Não foi possível alcançar o Supabase ao consultar " + resource + ". Verifique a disponibilidade do banco e a URL configurada no Vercel."
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		message = "O Supabase recusou o acesso a " + resource + ". Confirme se SUPABASE_URL e SUPABASE_SERVICE_ROLE_KEY (ou SUPABASE_SECRET_KEY) pertencem ao mesmo projeto."
	case status == http.StatusNotFound:
		message = "O recurso do Supabase necessário para " + resource + " não foi encontrado (HTTP 404). Aplique as migrações no projeto usado pelo Vercel e atualize o cache da API."
	default:
		message = fmt.Sprintf("O Supabase respondeu HTTP %d ao consultar %s.", status, resource)
	}
	queueJSON(w, http.StatusServiceUnavailable, map[string]any{"error": message, "etapa": stage, "http_status": status})
}

func (h HTTPHandler) action(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var input struct {
		Action string `json:"acao"`
		ID     string `json:"id"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || uuid.Validate(strings.TrimSpace(input.ID)) != nil {
		queueJSON(w, http.StatusBadRequest, map[string]string{"error": "Informe uma mensagem válida"})
		return
	}
	var query url.Values
	body := map[string]any{"updated_at": time.Now().UTC()}
	switch input.Action {
	case "cancelar":
		query = url.Values{"id": {"eq." + input.ID}, "status": {"eq.pendente"}}
		body["status"] = "cancelado"
	case "tentar_novamente":
		query = url.Values{"id": {"eq." + input.ID}, "status": {"in.(pendente,falha)"}}
		body["status"], body["next_attempt_at"], body["last_error"] = "pendente", time.Now().UTC(), nil
	default:
		queueJSON(w, http.StatusBadRequest, map[string]string{"error": "Ação inválida"})
		return
	}
	result, err := h.Supabase.ServiceRequest(r.Context(), tablePath+"?"+query.Encode(), supabase.RequestOptions{Method: http.MethodPatch, Body: body, Prefer: "return=representation"})
	if err != nil || result.StatusCode < 200 || result.StatusCode >= 300 {
		queueJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Não foi possível atualizar a mensagem"})
		return
	}
	var updated []json.RawMessage
	if json.Unmarshal(result.Body, &updated) != nil || len(updated) == 0 {
		queueJSON(w, http.StatusConflict, map[string]string{"error": "A mensagem já mudou de estado ou não pode receber esta ação"})
		return
	}
	queueJSON(w, http.StatusOK, map[string]any{"ok": true, "status": body["status"]})
}

func preview(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 150 {
		value = value[:147] + "..."
	}
	return value
}

func maskPhone(value string) string {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, value)
	if len(digits) < 4 {
		return "telefone cadastrado"
	}
	return fmt.Sprintf("•••• %s", digits[len(digits)-4:])
}

func queueJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
