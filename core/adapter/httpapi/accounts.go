package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/adapter/whatsapp"
	"inovarapp/core/adapter/whatsappqueue"
	"inovarapp/core/domain"
)

const (
	technicianConfigStoragePath = "/storage/v1/object/documentos-inovar/config/tecnico.json"
	publicAppURL                = "https://inovarapp.vercel.app"
)

var accountEmailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

type AccountsHandler struct {
	Supabase         *supabase.Client
	WhatsApp         whatsapp.Sender
	WhatsAppDefaults whatsapp.Config
}

type accountRequest struct {
	Action       string `json:"acao"`
	Email        string `json:"email"`
	Password     string `json:"senha"`
	Name         string `json:"nome"`
	Phone        string `json:"telefone"`
	WhatsApp     string `json:"whatsapp"`
	Address      string `json:"endereco"`
	Neighborhood string `json:"bairro"`
	City         string `json:"cidade"`
	CustomerID   string `json:"customer_id"`
}

type accountResponse struct {
	OK                  bool   `json:"ok,omitempty"`
	UserID              string `json:"user_id,omitempty"`
	Email               string `json:"email,omitempty"`
	Password            string `json:"senha_inicial,omitempty"`
	TemporaryPassword   string `json:"senha_temporaria,omitempty"`
	Message             string `json:"mensagem,omitempty"`
	WhatsAppEnviado     *bool  `json:"whatsapp_enviado,omitempty"`
	WhatsAppEnfileirado *bool  `json:"whatsapp_enfileirado,omitempty"`
	WhatsAppAviso       string `json:"whatsapp_aviso,omitempty"`
	Error               string `json:"error,omitempty"`
	AlreadyExists       bool   `json:"ja_existe,omitempty"`
}

type technicianAccountConfig struct {
	Messages map[string]string `json:"mensagensWhats"`
}

type accountCustomer struct {
	ID        string `json:"id"`
	Name      string `json:"nome"`
	WhatsApp  string `json:"whatsapp"`
	ProfileID string `json:"profile_id"`
}

type accountProfile struct {
	ID    string      `json:"id"`
	Email string      `json:"email"`
	Role  domain.Role `json:"tipo"`
}

func (h AccountsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeAccountJSON(w, http.StatusMethodNotAllowed, accountResponse{Error: "Método não permitido"})
		return
	}
	if h.Supabase == nil {
		writeAccountJSON(w, http.StatusInternalServerError, accountResponse{Error: "Backend não configurado"})
		return
	}
	if err := h.Supabase.RequireServiceRole(); err != nil {
		writeAccountJSON(w, http.StatusInternalServerError, accountResponse{Error: "Backend não configurado"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var request accountRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeAccountJSON(w, http.StatusBadRequest, accountResponse{Error: "Solicitação inválida"})
		return
	}

	switch request.Action {
	case "minha_conta":
		h.createOwnAccount(w, r, request)
	case "do_tecnico":
		caller, ok := h.authorizedTeamCaller(w, r)
		if !ok {
			return
		}
		h.createCustomerAccount(w, r, caller, request)
	case "redefinir_cliente":
		caller, ok := h.authorizedTeamCaller(w, r)
		if !ok {
			return
		}
		h.resetCustomerPassword(w, r, caller, request.CustomerID)
	default:
		writeAccountJSON(w, http.StatusBadRequest, accountResponse{Error: "Ação inválida"})
	}
}

func (h AccountsHandler) createOwnAccount(w http.ResponseWriter, r *http.Request, request accountRequest) {
	email := strings.ToLower(strings.TrimSpace(request.Email))
	if !accountEmailPattern.MatchString(email) {
		writeAccountJSON(w, http.StatusBadRequest, accountResponse{Error: "Informe um e-mail válido"})
		return
	}
	if len(request.Password) < 6 {
		writeAccountJSON(w, http.StatusBadRequest, accountResponse{Error: "A senha deve ter no mínimo 6 caracteres"})
		return
	}
	name := strings.TrimSpace(request.Name)
	if name == "" {
		writeAccountJSON(w, http.StatusBadRequest, accountResponse{Error: "Informe seu nome completo"})
		return
	}
	user, err := h.Supabase.CreateAdminUser(r.Context(), supabase.CreateAdminUserInput{
		Email: email, Password: request.Password, Name: name, Phone: strings.TrimSpace(request.WhatsApp),
	})
	if err != nil {
		h.writeCreateUserError(w, err, "Não foi possível criar sua conta", "Este e-mail já possui conta. Faça login normalmente.")
		return
	}

	if request.WhatsApp != "" || request.Address != "" || request.Neighborhood != "" || request.City != "" {
		h.patchCustomerForNewProfile(r.Context(), user.ID, request)
	}
	var whatsappQueued *bool
	whatsappNotice := ""
	if request.WhatsApp != "" {
		firstName := firstName(name)
		message := h.welcomeMessage(r.Context(), "boas_vindas_autocadastro", domain.ResolveWhatsAppTemplate(nil, "boas_vindas_autocadastro"),
			map[string]string{"cliente": firstName, "email": email, "empresa": "Inovar Refrigeração", "app": publicAppURL},
		)
		// General welcome messages have no time-sensitive credentials, so keep
		// them in the durable outbox until the WhatsApp account reconnects.
		queued := h.enqueueWelcome(r.Context(), "boas-vindas:autocadastro:"+user.ID, "boas_vindas_autocadastro", user.ID, request.WhatsApp, message, false, 0) == nil
		whatsappQueued = &queued
		if queued {
			whatsappNotice = "Mensagem de boas-vindas enfileirada. O envio será acompanhado na fila automática do WhatsApp."
		} else {
			whatsappNotice = "A conta foi criada, mas a mensagem de boas-vindas não entrou na fila. Verifique a migração e a credencial do Supabase."
		}
	}
	writeAccountJSON(w, http.StatusOK, accountResponse{
		OK: true, UserID: user.ID, Email: email, Message: "Conta criada! Entrando no seu portal...", WhatsAppEnfileirado: whatsappQueued, WhatsAppAviso: whatsappNotice,
	})
}

func (h AccountsHandler) createCustomerAccount(w http.ResponseWriter, r *http.Request, _ supabase.Caller, request accountRequest) {
	if !accountEmailPattern.MatchString(strings.TrimSpace(request.Email)) {
		writeAccountJSON(w, http.StatusBadRequest, accountResponse{Error: "Informe um e-mail válido para o cliente"})
		return
	}
	email := strings.ToLower(strings.TrimSpace(request.Email))
	password := request.Password
	if len(password) < 6 {
		password = "123456"
	}
	user, err := h.Supabase.CreateAdminUser(r.Context(), supabase.CreateAdminUserInput{
		Email: email, Password: password, Name: request.Name, Phone: request.Phone, MustChangePassword: true,
	})
	if err != nil {
		h.writeCreateUserError(w, err, "Não foi possível criar a conta do cliente", "O e-mail "+email+" já possui conta. O cliente pode entrar normalmente.")
		return
	}
	if request.CustomerID != "" && user.ID != "" {
		h.linkExistingCustomer(r.Context(), request.CustomerID, user.ID, request.Name)
	}
	var whatsappQueued *bool
	whatsappNotice := ""
	if request.Phone != "" {
		firstName := firstName(request.Name)
		if firstName == "" {
			firstName = "Cliente"
		}
		message := h.welcomeMessage(r.Context(), "boas_vindas_tecnico", domain.ResolveWhatsAppTemplate(nil, "boas_vindas_tecnico"),
			map[string]string{"cliente": firstName, "email": email, "senha": password, "empresa": "Inovar Refrigeração", "app": publicAppURL},
		)
		queued := h.enqueueWelcome(r.Context(), "boas-vindas:tecnico:"+user.ID, "boas_vindas_tecnico", user.ID, request.Phone, message, true, 24*time.Hour) == nil
		whatsappQueued = &queued
		if queued {
			whatsappNotice = "Mensagem de acesso enfileirada. Por conter senha temporária, ela expira em 24 horas se não for enviada."
		} else {
			whatsappNotice = "A conta foi criada, mas a mensagem de acesso não entrou na fila. Compartilhe os dados por um canal seguro e verifique a configuração da fila."
		}
	}
	writeAccountJSON(w, http.StatusOK, accountResponse{
		OK: true, UserID: user.ID, Email: email, Password: password,
		Message: "Conta criada! O cliente entra com " + email + " e a senha " + password + ".", WhatsAppEnfileirado: whatsappQueued, WhatsAppAviso: whatsappNotice,
	})
}

func (h AccountsHandler) resetCustomerPassword(w http.ResponseWriter, r *http.Request, _ supabase.Caller, customerID string) {
	customerID = strings.TrimSpace(customerID)
	if customerID == "" {
		writeAccountJSON(w, http.StatusBadRequest, accountResponse{Error: "Cliente não informado"})
		return
	}
	customer, err := h.findCustomer(r.Context(), customerID)
	if err != nil {
		writeAccountJSON(w, http.StatusInternalServerError, accountResponse{Error: "Não foi possível consultar o cliente"})
		return
	}
	if customer.ProfileID == "" {
		writeAccountJSON(w, http.StatusConflict, accountResponse{Error: "Este cliente ainda não possui uma conta de acesso vinculada."})
		return
	}
	profile, err := h.findAccountProfile(r.Context(), customer.ProfileID)
	if err != nil {
		writeAccountJSON(w, http.StatusInternalServerError, accountResponse{Error: "Não foi possível consultar a conta do cliente"})
		return
	}
	if profile.Email == "" || profile.Role != domain.RoleCustomer {
		writeAccountJSON(w, http.StatusConflict, accountResponse{Error: "A conta vinculada não é uma conta válida de cliente."})
		return
	}
	password, err := temporaryPassword()
	if err != nil {
		writeAccountJSON(w, http.StatusInternalServerError, accountResponse{Error: "Não foi possível gerar uma senha temporária"})
		return
	}
	authUser, err := h.Supabase.GetAdminUser(r.Context(), customer.ProfileID)
	if err != nil {
		writeAccountJSON(w, http.StatusInternalServerError, accountResponse{Error: "Não foi possível consultar a conta do cliente"})
		return
	}
	metadata := make(map[string]json.RawMessage, len(authUser.UserMetadata)+1)
	for key, value := range authUser.UserMetadata {
		metadata[key] = value
	}
	metadata["must_change_password"] = json.RawMessage("true")
	_, err = h.Supabase.UpdateAdminUser(r.Context(), customer.ProfileID, map[string]any{
		"password": password, "user_metadata": metadata,
	})
	if err != nil {
		var authError *supabase.AuthError
		if errors.As(err, &authError) && authError.Message != "" {
			writeAccountJSON(w, http.StatusBadRequest, accountResponse{Error: authError.Message})
		} else {
			writeAccountJSON(w, http.StatusBadRequest, accountResponse{Error: "Não foi possível redefinir a senha"})
		}
		return
	}

	var whatsappQueued *bool
	whatsappNotice := ""
	if customer.WhatsApp != "" {
		name := firstName(customer.Name)
		if name == "" {
			name = "Cliente"
		}
		message := "🔐 *Acesso temporário ao InovarApp*\n\nOlá, " + name + ". Sua senha foi redefinida pela equipe.\n\n" +
			"📧 *Login:* " + profile.Email + "\n🔑 *Senha temporária:* " + password + "\n📲 " + publicAppURL + "\n\n" +
			"No próximo acesso você deverá criar uma nova senha pessoal."
		queued := h.enqueueWelcome(r.Context(), "boas-vindas:redefinicao:"+uuid.NewString(), "redefinicao_senha", customer.ProfileID, customer.WhatsApp, message, true, time.Hour) == nil
		whatsappQueued = &queued
		if queued {
			whatsappNotice = "A mensagem da senha temporária entrou na fila; ela expira em uma hora se não for enviada."
		} else {
			whatsappNotice = "A senha foi redefinida, mas a mensagem não entrou na fila. Compartilhe a senha temporária por um canal seguro."
		}
	}
	writeAccountJSON(w, http.StatusOK, accountResponse{
		OK: true, Email: profile.Email, TemporaryPassword: password,
		Message: "Senha temporária criada. O cliente deverá alterá-la no próximo acesso.", WhatsAppEnfileirado: whatsappQueued, WhatsAppAviso: whatsappNotice,
	})
}

func (h AccountsHandler) authorizedTeamCaller(w http.ResponseWriter, r *http.Request) (supabase.Caller, bool) {
	token := supabase.BearerToken(r.Header.Get("Authorization"))
	if token == "" {
		writeAccountJSON(w, http.StatusUnauthorized, accountResponse{Error: "Não autenticado"})
		return supabase.Caller{}, false
	}
	caller, err := h.Supabase.AuthenticateCaller(r.Context(), token)
	if err != nil {
		writeAccountJSON(w, http.StatusUnauthorized, accountResponse{Error: "Sessão inválida"})
		return supabase.Caller{}, false
	}
	if caller.Role != domain.RoleAdmin && caller.Role != domain.RoleTechnician {
		writeAccountJSON(w, http.StatusForbidden, accountResponse{Error: "Somente técnicos e administradores podem realizar esta ação"})
		return supabase.Caller{}, false
	}
	return caller, true
}

func (h AccountsHandler) writeCreateUserError(w http.ResponseWriter, err error, fallback, duplicateMessage string) {
	var authError *supabase.AuthError
	if errors.As(err, &authError) {
		combined := strings.ToLower(authError.Code + " " + authError.Message)
		if strings.Contains(combined, "already") || strings.Contains(combined, "registered") || strings.Contains(combined, "exists") || strings.Contains(combined, "email_exists") {
			writeAccountJSON(w, http.StatusConflict, accountResponse{Error: duplicateMessage, AlreadyExists: true})
			return
		}
		if strings.TrimSpace(authError.Message) != "" {
			writeAccountJSON(w, http.StatusBadRequest, accountResponse{Error: authError.Message})
			return
		}
	}
	writeAccountJSON(w, http.StatusBadRequest, accountResponse{Error: fallback})
}

func (h AccountsHandler) patchCustomerForNewProfile(ctx context.Context, profileID string, request accountRequest) {
	query := url.Values{}
	query.Set("profile_id", "eq."+profileID)
	query.Set("select", "id")
	rows, err := h.getCustomerRows(ctx, query)
	if err != nil || len(rows) == 0 || rows[0].ID == "" {
		return
	}
	updates := map[string]any{}
	if value := strings.TrimSpace(request.WhatsApp); value != "" {
		updates["whatsapp"] = value
	}
	if value := strings.TrimSpace(request.Address); value != "" {
		updates["endereco"] = value
	}
	if value := strings.TrimSpace(request.Neighborhood); value != "" {
		updates["bairro"] = value
	}
	if value := strings.TrimSpace(request.City); value != "" {
		updates["cidade"] = value
	}
	if len(updates) == 0 {
		return
	}
	patchQuery := url.Values{}
	patchQuery.Set("id", "eq."+rows[0].ID)
	_, _ = h.Supabase.ServiceRequest(ctx, "/rest/v1/customers?"+patchQuery.Encode(), supabase.RequestOptions{Method: http.MethodPatch, Body: updates})
}

func (h AccountsHandler) linkExistingCustomer(ctx context.Context, customerID, profileID, name string) {
	deleteQuery := url.Values{}
	deleteQuery.Set("profile_id", "eq."+profileID)
	deleteQuery.Set("nome", "eq."+strings.TrimSpace(name))
	_, _ = h.Supabase.ServiceRequest(ctx, "/rest/v1/customers?"+deleteQuery.Encode(), supabase.RequestOptions{Method: http.MethodDelete})
	patchQuery := url.Values{}
	patchQuery.Set("id", "eq."+customerID)
	_, _ = h.Supabase.ServiceRequest(ctx, "/rest/v1/customers?"+patchQuery.Encode(), supabase.RequestOptions{
		Method: http.MethodPatch, Body: map[string]string{"profile_id": profileID},
	})
}

func (h AccountsHandler) findCustomer(ctx context.Context, customerID string) (accountCustomer, error) {
	query := url.Values{}
	query.Set("id", "eq."+customerID)
	query.Set("select", "id,nome,whatsapp,profile_id")
	rows, err := h.getCustomerRows(ctx, query)
	if err != nil {
		return accountCustomer{}, err
	}
	if len(rows) == 0 {
		return accountCustomer{}, nil
	}
	return rows[0], nil
}

func (h AccountsHandler) getCustomerRows(ctx context.Context, query url.Values) ([]accountCustomer, error) {
	result, err := h.Supabase.ServiceRequest(ctx, "/rest/v1/customers?"+query.Encode(), supabase.RequestOptions{Method: http.MethodGet})
	if err != nil {
		return nil, err
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		return nil, fmt.Errorf("customer query failed with status %d", result.StatusCode)
	}
	var rows []accountCustomer
	if err := json.Unmarshal(result.Body, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (h AccountsHandler) findAccountProfile(ctx context.Context, userID string) (accountProfile, error) {
	query := url.Values{}
	query.Set("id", "eq."+userID)
	query.Set("select", "id,email,tipo")
	result, err := h.Supabase.ServiceRequest(ctx, "/rest/v1/profiles?"+query.Encode(), supabase.RequestOptions{Method: http.MethodGet})
	if err != nil {
		return accountProfile{}, err
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		return accountProfile{}, fmt.Errorf("profile query failed with status %d", result.StatusCode)
	}
	var profiles []accountProfile
	if err := json.Unmarshal(result.Body, &profiles); err != nil {
		return accountProfile{}, err
	}
	if len(profiles) != 1 || profiles[0].ID != userID {
		return accountProfile{}, errors.New("profile not found")
	}
	return profiles[0], nil
}

func (h AccountsHandler) welcomeMessage(ctx context.Context, key, fallback string, values map[string]string) string {
	config := h.readAccountConfig(ctx)
	message := fallback
	if saved := strings.TrimSpace(config.Messages[key]); saved != "" {
		message = saved
	}
	return domain.ApplyWhatsAppPlaceholders(message, values)
}

func (h AccountsHandler) readAccountConfig(ctx context.Context) technicianAccountConfig {
	config := technicianAccountConfig{}
	if result, err := h.Supabase.ServiceRequest(ctx, technicianConfigStoragePath, supabase.RequestOptions{Method: http.MethodGet}); err == nil && result.StatusCode >= 200 && result.StatusCode < 300 {
		var stored technicianAccountConfig
		if json.Unmarshal(result.Body, &stored) == nil {
			config.Messages = stored.Messages
		}
	}
	return config
}

func (h AccountsHandler) sendWelcome(ctx context.Context, phone, message string) error {
	sender := h.WhatsApp
	config := (WhatsAppHandler{Supabase: h.Supabase, Defaults: h.WhatsAppDefaults}).loadConfig(ctx)
	if config.ProprioSession == "" {
		config.ProprioSession = "inovar"
	}
	if config.ProprioURL == "" || config.ProprioToken == "" || config.ProprioSession == "" {
		return errors.New("WhatsApp não configurado no servidor")
	}
	sendContext, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	return sender.SendText(sendContext, config, phone, message)
}

func (h AccountsHandler) enqueueWelcome(ctx context.Context, key, event, sourceID, phone, message string, sensitive bool, ttl time.Duration) error {
	var expires *time.Time
	if ttl > 0 {
		deadline := time.Now().UTC().Add(ttl)
		expires = &deadline
	}
	return whatsappqueue.Enqueue(ctx, h.Supabase, whatsappqueue.EnqueueInput{
		IdempotencyKey: key, EventType: event, SourceEntityType: "perfil", SourceEntityID: sourceID,
		RecipientPhone: phone, MessageText: message, Sensitive: sensitive, ExpiresAt: expires,
	})
}

func temporaryPassword() (string, error) {
	uppercase := "ABCDEFGHJKLMNPQRSTUVWXYZ"
	lowercase := "abcdefghijkmnopqrstuvwxyz"
	numbers := "23456789"
	specials := "!@#"
	password := make([]byte, 0, 10)
	for _, pool := range []string{uppercase, lowercase, numbers, specials} {
		character, err := pickRandom(pool)
		if err != nil {
			return "", err
		}
		password = append(password, character)
	}
	for len(password) < 10 {
		character, err := pickRandom(uppercase + lowercase + numbers)
		if err != nil {
			return "", err
		}
		password = append(password, character)
	}
	for i := len(password) - 1; i > 0; i-- {
		j, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return "", err
		}
		index := int(j.Int64())
		password[i], password[index] = password[index], password[i]
	}
	return string(password), nil
}

func pickRandom(characters string) (byte, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(int64(len(characters))))
	if err != nil {
		return 0, err
	}
	return characters[value.Int64()], nil
}

func firstName(name string) string {
	parts := strings.Fields(strings.TrimSpace(name))
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func writeAccountJSON(w http.ResponseWriter, status int, response accountResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}
