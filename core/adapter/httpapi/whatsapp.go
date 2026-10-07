package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/adapter/whatsapp"
	"inovarapp/core/adapter/whatsappqueue"
)

type WhatsAppHandler struct {
	Supabase *supabase.Client
	Sender   whatsapp.Sender
	Defaults whatsapp.Config
	HTTP     whatsapp.HTTPDoer
}

type whatsappRequest struct {
	Action           string     `json:"acao"`
	Text             string     `json:"texto"`
	DocumentURL      string     `json:"documento_url"`
	DocumentName     string     `json:"documento_nome"`
	Mode             string     `json:"modo"`
	Phone            string     `json:"telefone"`
	EventType        string     `json:"evento"`
	IdempotencyKey   string     `json:"idempotencia"`
	SourceEntityType string     `json:"origem_tipo"`
	SourceEntityID   string     `json:"origem_id"`
	ScheduledAt      *time.Time `json:"agendado_para"`
	ExpiresAt        *time.Time `json:"expira_em"`
}

func (h WhatsAppHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
		writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Somente a equipe Inovar"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	var input whatsappRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Solicitação inválida"})
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Envie somente um objeto JSON válido"})
		return
	}
	if input.Action != "enviar" && input.Action != "status" && input.Action != "conectar" && input.Action != "desconectar" {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Ação inválida"})
		return
	}
	config := h.loadConfig(r.Context())
	switch input.Action {
	case "status":
		h.status(w, r, config)
		return
	case "conectar":
		h.connect(w, r, config, input.Phone)
		return
	case "desconectar":
		h.disconnect(w, r, config, input.Mode)
		return
	}
	if strings.TrimSpace(input.Phone) == "" || strings.TrimSpace(input.Text) == "" && strings.TrimSpace(input.DocumentURL) == "" {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Informe telefone e (texto ou documento_url)"})
		return
	}
	key := strings.TrimSpace(input.IdempotencyKey)
	if key == "" {
		key = "manual:" + uuid.NewString()
	}
	eventType := strings.TrimSpace(input.EventType)
	if eventType == "" {
		eventType = "mensagem_equipe"
	}
	if err := whatsappqueue.Enqueue(r.Context(), h.Supabase, whatsappqueue.EnqueueInput{
		IdempotencyKey: key, EventType: eventType, RecipientPhone: input.Phone, MessageText: input.Text,
		DocumentURL: input.DocumentURL, DocumentName: input.DocumentName, ScheduledAt: timeValue(input.ScheduledAt),
		ExpiresAt: input.ExpiresAt, SourceEntityType: input.SourceEntityType, SourceEntityID: input.SourceEntityID,
	}); err != nil {
		writeResourceJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "A mensagem não foi enfileirada. Verifique a migração da fila e a configuração do Supabase."})
		return
	}
	configured := strings.TrimSpace(config.ProprioURL) != "" && strings.TrimSpace(config.ProprioToken) != "" && strings.TrimSpace(config.ProprioSession) != ""
	writeResourceJSON(w, http.StatusAccepted, map[string]any{"ok": true, "enfileirado": true, "status": "pendente", "configurado": configured, "mensagem": "Mensagem registrada na fila automática do WhatsApp."})
}

func timeValue(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func (h WhatsAppHandler) status(w http.ResponseWriter, r *http.Request, config whatsapp.Config) {
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	if strings.TrimSpace(config.ProprioURL) == "" || strings.TrimSpace(config.ProprioToken) == "" || strings.TrimSpace(config.ProprioSession) == "" {
		writeResourceJSON(w, http.StatusOK, map[string]any{"configurado": false, "conectado": false, "mensagem": "A conexão automática do WhatsApp ainda não está disponível no servidor. Tente novamente mais tarde."})
		return
	}
	body, status, err := h.proprioRequest(r.Context(), config, http.MethodGet, "/v1/sessions/"+url.PathEscape(config.ProprioSession), nil)
	if err != nil {
		writeResourceJSON(w, http.StatusOK, map[string]any{"configurado": true, "conectado": false, "estado": "indisponivel", "mensagem": "O serviço WhatsApp está temporariamente indisponível. A tela tentará novamente."})
		return
	}
	state := whatsappStateFromSession(body, status)
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		state = "credenciais_invalidas"
	} else if status >= 500 {
		state = "indisponivel"
	}
	connected := state == "open"
	message := "Sessão criada, mas o WhatsApp ainda não foi pareado. Gere o QR na tela de conexão."
	if connected {
		message = "WhatsApp conectado! Os disparos automáticos estão ativos."
	} else if state == "connecting" {
		message = "Conectando... aguarde alguns segundos e verifique novamente."
	} else if state == "nao_encontrada" {
		message = "A sessão ainda não foi criada. Toque em Conectar para iniciar o pareamento."
	} else if state == "credenciais_invalidas" {
		message = "O serviço WhatsApp recusou a credencial configurada no servidor. Revise WHATSAPP_OWN_TOKEN."
	} else if state == "indisponivel" {
		message = "O serviço WhatsApp está temporariamente indisponível. A tela tentará novamente."
	} else if state == "passkey_required" || state == "passkey_processing" || state == "passkey_confirmation" {
		message = "O WhatsApp solicitou uma chave de acesso. Conclua a confirmação no cliente de pareamento do serviço e atualize o estado."
	} else if state == "pairing_failed" {
		message = "O pareamento falhou. Gere um novo QR ou código e tente novamente."
	} else if state == "logged_out" {
		message = "A sessão foi encerrada pelo WhatsApp. Remova a sessão e conecte a conta novamente."
	} else if state == "temporarily_banned" {
		message = "O WhatsApp bloqueou temporariamente esta sessão. Aguarde e verifique o estado antes de tentar novamente."
	} else if state == "client_outdated" {
		message = "O serviço de conexão precisa de atualização para acompanhar o protocolo atual do WhatsApp."
	} else if state == "stream_replaced" {
		message = "A sessão foi substituída por outra conexão. Atualize o estado e conecte novamente se necessário."
	} else if state == "connection_failed" {
		message = "A conexão falhou no serviço WhatsApp. Verifique o serviço e tente gerar um novo QR."
	}
	result := map[string]any{"configurado": true, "instancia": config.ProprioSession, "conta": stringValue(body["account"]), "conectado": connected, "estado": state, "mensagem": message}
	if !connected && state == "connecting" {
		session := url.PathEscape(config.ProprioSession)
		if stringValue(body["pairing_method"]) == "phone" {
			pairing, pairingStatus, pairingErr := h.proprioRequest(r.Context(), config, http.MethodGet, "/v1/sessions/"+session+"/pairing", nil)
			if pairingErr == nil && pairingStatus == http.StatusOK {
				result["pairingCode"] = stringValue(pairing["code"])
			}
		} else if qr, qrErr := h.qrImageBase64(r, config, session); qrErr == nil && qr != "" {
			result["qr"] = qr
		}
	} else if !connected && state == "close" {
		if qr, qrErr := h.qrImageBase64(r, config, url.PathEscape(config.ProprioSession)); qrErr == nil && qr != "" {
			result["qr"] = qr
		}
	}
	writeResourceJSON(w, http.StatusOK, result)
}

func whatsappStateFromSession(body map[string]any, httpStatus int) string {
	state := stringValue(body["status"])
	switch state {
	case "connected":
		return "open"
	case "connecting", "pairing", "reconnecting":
		return "connecting"
	case "temporarily_banned", "client_outdated", "stream_replaced", "logged_out", "connection_failed", "pairing_failed", "passkey_required", "passkey_processing", "passkey_confirmation":
		return state
	case "disconnected":
		return "close"
	case "":
		if httpStatus == 404 {
			return "nao_encontrada"
		}
		if httpStatus >= 200 && httpStatus < 300 {
			return "desconhecido"
		}
		return "nao_encontrada"
	}
	return "close"
}

func (h WhatsAppHandler) connect(w http.ResponseWriter, r *http.Request, config whatsapp.Config, phone string) {
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	if strings.TrimSpace(config.ProprioURL) == "" || strings.TrimSpace(config.ProprioToken) == "" || strings.TrimSpace(config.ProprioSession) == "" {
		writeResourceJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "A conexão automática do WhatsApp ainda não está disponível no servidor."})
		return
	}
	session := url.PathEscape(config.ProprioSession)
	if phone = strings.TrimSpace(phone); phone != "" {
		number, valid := whatsapp.NormalizePhone(phone)
		if !valid {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Informe um telefone válido com DDD."})
			return
		}
		phone = number
	}
	connectBody, connectStatus, err := h.proprioRequest(r.Context(), config, http.MethodPost, "/v1/sessions/"+session+"/connect", nil)
	if err != nil {
		writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Falha ao falar com o sistema WhatsApp", "detalhe": truncateWhatsAppError(err.Error(), 140)})
		return
	}
	if connectStatus == http.StatusNotFound {
		_, createStatus, createErr := h.proprioRequest(r.Context(), config, http.MethodPost, "/v1/sessions", map[string]string{"id": config.ProprioSession})
		if createErr == nil && (createStatus >= 200 && createStatus < 300 || createStatus == http.StatusConflict) {
			connectBody, connectStatus, err = h.proprioRequest(r.Context(), config, http.MethodPost, "/v1/sessions/"+session+"/connect", nil)
		}
		if err != nil || connectStatus < 200 || connectStatus >= 300 {
			writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível criar ou iniciar a sessão WhatsApp no serviço próprio."})
			return
		}
	}
	state := whatsappStateFromSession(connectBody, connectStatus)
	if connectStatus == http.StatusUnauthorized || connectStatus == http.StatusForbidden {
		writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "O serviço WhatsApp recusou a credencial configurada no servidor."})
		return
	}
	if connectStatus < 200 || connectStatus >= 300 {
		writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": firstNonEmptyWhatsApp(stringValue(connectBody["message"]), stringValue(connectBody["error"]), "Não foi possível iniciar a conexão WhatsApp.")})
		return
	}
	if state == "open" {
		writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true, "conectado": true, "mensagem": "WhatsApp já está conectado!"})
		return
	}
	if phone != "" {
		number, valid := whatsapp.NormalizePhone(phone)
		if !valid {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Informe um telefone válido com DDD."})
			return
		}
		pairBody, pairStatus, pairErr := h.proprioRequest(r.Context(), config, http.MethodPost, "/v1/sessions/"+session+"/pair", map[string]string{"phone": number})
		if pairErr != nil || pairStatus < 200 || pairStatus >= 300 {
			statusCode := http.StatusBadGateway
			message := "Não foi possível gerar o código de pareamento. Atualize a conexão e tente novamente."
			if pairErr != nil {
				message = "O serviço WhatsApp não respondeu durante a geração do código. Tente novamente em alguns segundos."
			} else if pairStatus == http.StatusGatewayTimeout {
				statusCode = http.StatusGatewayTimeout
				message = "O WhatsApp ainda está preparando o pareamento. Aguarde alguns segundos e tente gerar o código novamente."
			} else if code := whatsappRemoteErrorCode(pairBody); code == "passkey_pending" {
				message = "O WhatsApp solicitou uma confirmação de chave de acesso. Conclua essa etapa antes de gerar outro código."
			} else if remote := whatsappRemoteError(pairBody); remote != "" {
				message = remote
			}
			writeResourceJSON(w, statusCode, map[string]string{"error": message})
			return
		}
		writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true, "conectado": false, "pairingCode": stringValue(pairBody["code"]), "estado": state})
		return
	}
	qrBase64, qrErr := h.qrImageBase64(r, config, session)
	if qrErr != nil || qrBase64 == "" {
		writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true, "configurado": true, "conectado": false, "estado": state, "mensagem": "A sessão Go iniciou a conexão e está aguardando o QR Code. Esta tela será atualizada automaticamente."})
		return
	}
	writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true, "conectado": false, "qr": qrBase64, "estado": state})
}

func (h WhatsAppHandler) qrImageBase64(r *http.Request, config whatsapp.Config, session string) (string, error) {
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, strings.TrimRight(config.ProprioURL, "/")+"/v1/sessions/"+url.PathEscape(session)+"/pairing?format=png", nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+config.ProprioToken)
	client := h.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("QR indisponível (HTTP %d)", response.StatusCode)
	}
	png, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}

func (h WhatsAppHandler) disconnect(w http.ResponseWriter, r *http.Request, config whatsapp.Config, mode string) {
	if mode != "logout" && mode != "apagar" {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "modo deve ser \"logout\" ou \"apagar\""})
		return
	}
	if strings.TrimSpace(config.ProprioURL) == "" || strings.TrimSpace(config.ProprioToken) == "" || strings.TrimSpace(config.ProprioSession) == "" {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Sistema WhatsApp não configurado"})
		return
	}
	session := url.PathEscape(config.ProprioSession)
	if mode == "apagar" {
		body, status, err := h.proprioRequest(r.Context(), config, http.MethodDelete, "/v1/sessions/"+session, nil)
		if err != nil || status < 200 || status >= 300 && status != http.StatusNotFound {
			detail, _ := json.Marshal(body)
			writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Falha ao " + mode, "detalhe": truncateWhatsAppError(firstNonEmptyWhatsApp(string(detail), errorText(err, status)), 180)})
			return
		}
		writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true, "modo": mode, "mensagem": "Conta removida. Ao gerar o QR, uma sessão nova será criada para o número que você conectar."})
		return
	}
	body, status, err := h.proprioRequest(r.Context(), config, http.MethodPost, "/v1/sessions/"+session+"/disconnect", nil)
	if err != nil || status < 200 || status >= 300 {
		detail, _ := json.Marshal(body)
		writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Falha ao " + mode, "detalhe": truncateWhatsAppError(firstNonEmptyWhatsApp(string(detail), errorText(err, status)), 180)})
		return
	}
	writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true, "modo": mode, "mensagem": "WhatsApp desconectado. Gere um novo QR para conectar (mesmo ou outro número)."})
}

func (h WhatsAppHandler) proprioRequest(ctx context.Context, config whatsapp.Config, method, route string, payload any) (map[string]any, int, error) {
	base := strings.TrimRight(strings.TrimSpace(config.ProprioURL), "/")
	endpoint := base + "/" + strings.TrimLeft(route, "/")
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, 0, err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set("Authorization", "Bearer "+config.ProprioToken)
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	client := h.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if readErr != nil {
		return nil, response.StatusCode, readErr
	}
	var decoded map[string]any
	if len(data) > 0 {
		if err := json.Unmarshal(data, &decoded); err != nil {
			decoded = map[string]any{}
		}
	}
	return decoded, response.StatusCode, nil
}

func (h WhatsAppHandler) loadConfig(_ context.Context) whatsapp.Config {
	config := h.Defaults
	if config.ProprioSession == "" {
		config.ProprioSession = "inovar"
	}
	return config
}

func stringValue(value any) string { text, _ := value.(string); return text }

func whatsappRemoteError(body map[string]any) string {
	if value := stringValue(body["message"]); value != "" {
		return value
	}
	if value := stringValue(body["error"]); value != "" {
		return value
	}
	if nested, ok := body["error"].(map[string]any); ok {
		return stringValue(nested["message"])
	}
	return ""
}

func whatsappRemoteErrorCode(body map[string]any) string {
	if nested, ok := body["error"].(map[string]any); ok {
		return stringValue(nested["code"])
	}
	return stringValue(body["code"])
}

func firstNonEmptyWhatsApp(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func truncateWhatsAppError(value string, max int) string {
	if len(value) > max {
		return value[:max]
	}
	return value
}

func errorText(err error, status int) string {
	if err != nil {
		return err.Error()
	}
	return fmt.Sprintf("Sistema WhatsApp returned %d", status)
}
