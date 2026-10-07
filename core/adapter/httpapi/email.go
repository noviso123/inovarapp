package httpapi

import (
	"context"
	"encoding/json"
	"html/template"
	"net/http"
	"net/mail"
	"strings"

	mailadapter "inovarapp/core/adapter/email"
	"inovarapp/core/adapter/supabase"
)

type EmailHandler struct {
	Supabase *supabase.Client
	Sender   mailadapter.Sender
	Defaults mailadapter.Config
}

type emailRequest struct {
	Action  string `json:"acao"`
	To      string `json:"para"`
	Name    string `json:"nome"`
	Subject string `json:"assunto"`
	HTML    string `json:"html"`
	Type    string `json:"tipo"`
}

func (h EmailHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
		writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Somente a equipe Inovar dispara e-mails"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 512<<10)
	var input emailRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Solicitação inválida"})
		return
	}
	if input.Action != "enviar" && input.Action != "disparo" && input.Action != "testar" {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Ação inválida"})
		return
	}
	cfg := h.loadEmailConfig(r.Context())
	if !mailadapter.Configured(cfg) {
		writeResourceJSON(w, http.StatusOK, map[string]any{"configurado": false, "mensagem": "Conecte uma conta Google nas Configurações ou configure um canal de e-mail alternativo."})
		return
	}
	if h.Sender == nil {
		h.Sender = mailadapter.ConfiguredSender{}
	}
	switch input.Action {
	case "enviar":
		if !validEmail(input.To) || strings.TrimSpace(input.Subject) == "" || strings.TrimSpace(input.HTML) == "" {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Informe para, assunto e html"})
			return
		}
		if _, err := h.Sender.Send(r.Context(), cfg, mailadapter.Message{To: input.To, Subject: input.Subject, HTML: input.HTML}); err != nil {
			writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Falha no envio de e-mail", "detalhe": shortEmailError(err)})
			return
		}
		writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true, "canal": "gmail"})
	case "testar":
		to := strings.TrimSpace(input.To)
		if to == "" {
			to = "jhonatan.satiro1@gmail.com"
		}
		if !validEmail(to) {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Informe um e-mail válido para o teste"})
			return
		}
		name := strings.TrimSpace(input.Name)
		if name == "" {
			name = "Jhonatan"
		}
		err := h.sendTest(r.Context(), cfg, name, to)
		if err != nil {
			writeResourceJSON(w, http.StatusOK, map[string]any{"ok": false, "mensagem": "Falha ao enviar o teste: " + shortEmailError(err)})
			return
		}
		writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true, "canal": "gmail"})
	case "disparo":
		h.sendBulk(w, r, cfg, input)
	}
}

func (h EmailHandler) loadEmailConfig(ctx context.Context) mailadapter.Config {
	cfg := h.Defaults
	if h.Supabase != nil {
		if stored, err := (SettingsHandler{Supabase: h.Supabase}).loadSettings(ctx); err == nil {
			var username, password, googleAccount, googleRefreshToken string
			_ = json.Unmarshal(stored["email_gmail_user"], &username)
			_ = json.Unmarshal(stored["email_gmail_pass"], &password)
			_ = json.Unmarshal(stored["email_google_account"], &googleAccount)
			_ = json.Unmarshal(stored["email_google_refresh_token"], &googleRefreshToken)
			if strings.TrimSpace(googleRefreshToken) != "" {
				cfg.GoogleRefreshToken = googleRefreshToken
				if strings.TrimSpace(googleAccount) != "" {
					cfg.Username = googleAccount
				}
			}
			if strings.TrimSpace(username) != "" {
				if strings.TrimSpace(cfg.GoogleRefreshToken) == "" {
					cfg.Username = username
				}
			}
			if strings.TrimSpace(password) != "" {
				cfg.Password = password
			}
		}
	}
	return cfg
}

func (h EmailHandler) sendTest(ctx context.Context, cfg mailadapter.Config, name, to string) error {
	body, err := renderEmailTest(name)
	if err != nil {
		return err
	}
	_, err = h.Sender.Send(ctx, cfg, mailadapter.Message{To: to, Subject: "InovarApp — canal de e-mail ativo (Gmail)!", HTML: body})
	return err
}

type emailRecipient struct {
	Name    string `json:"nome"`
	Profile *struct {
		Email string `json:"email"`
	} `json:"profiles"`
}
type emailResult struct {
	Customer string `json:"cliente"`
	Email    string `json:"email"`
	OK       bool   `json:"ok"`
	Reason   string `json:"motivo,omitempty"`
}

func (h EmailHandler) sendBulk(w http.ResponseWriter, r *http.Request, cfg mailadapter.Config, input emailRequest) {
	result, err := h.Supabase.ServiceRequest(r.Context(), "/rest/v1/customers?select=id,nome,profile_id,profiles!customers_profile_id_fkey(email)", supabase.RequestOptions{Method: http.MethodGet})
	if err != nil || result.StatusCode < 200 || result.StatusCode >= 300 {
		writeResourceJSON(w, http.StatusInternalServerError, map[string]string{"error": "Não foi possível carregar os clientes"})
		return
	}
	var recipients []emailRecipient
	if json.Unmarshal(result.Body, &recipients) != nil {
		writeResourceJSON(w, http.StatusInternalServerError, map[string]string{"error": "Não foi possível carregar os clientes"})
		return
	}
	custom := strings.TrimSpace(input.Subject) != "" || strings.TrimSpace(input.HTML) != ""
	if custom && (strings.TrimSpace(input.Subject) == "" || strings.TrimSpace(input.HTML) == "") {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Informe assunto e html"})
		return
	}
	subject := input.Subject
	if subject == "" {
		subject = "InovarApp — e-mail de teste"
	}
	results := make([]emailResult, 0, len(recipients))
	sent := 0
	for _, recipient := range recipients {
		if recipient.Profile == nil || strings.TrimSpace(recipient.Profile.Email) == "" {
			continue
		}
		body := input.HTML
		if body == "" {
			name := strings.Fields(strings.TrimSpace(recipient.Name))
			first := "Cliente"
			if len(name) > 0 {
				first = name[0]
			}
			html, err := renderEmailTest(first)
			if err != nil {
				continue
			}
			body = html
		}
		_, sendErr := h.Sender.Send(r.Context(), cfg, mailadapter.Message{To: recipient.Profile.Email, Subject: subject, HTML: body})
		item := emailResult{Customer: recipient.Name, Email: recipient.Profile.Email, OK: sendErr == nil}
		if sendErr == nil {
			sent++
		} else {
			item.Reason = shortEmailError(sendErr)
		}
		results = append(results, item)
	}
	typeLabel := "personalizado"
	if input.Type == "teste" || (!custom) {
		typeLabel = "teste"
	}
	writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true, "canal": "gmail", "tipo": typeLabel, "totalClientesComEmail": len(results), "enviados": sent, "falhas": len(results) - sent, "resultados": results})
}

func renderEmailTest(name string) (string, error) {
	first := strings.Fields(strings.TrimSpace(name))
	if len(first) == 0 {
		first = []string{"Cliente"}
	}
	t, err := template.New("email").Parse(`<div style="font-family:sans-serif;max-width:560px;margin:auto;border:1px solid #e2e8f0;border-radius:12px;overflow:hidden"><div style="background:#0B2D4E;padding:20px 24px"><span style="color:#FFC61E;font-weight:800;font-size:20px">❄️ InovarApp</span></div><div style="padding:24px"><h2 style="color:#0B2D4E;margin-top:0">Olá, {{.}}!</h2><p>Este é um <b>e-mail de teste do sistema de envios</b> da Inovar Refrigeração.</p><p>Por este canal você receberá: <b>alertas de manutenção preventiva</b>, <b>orçamentos</b> e <b>comunicados</b>.</p><p style="color:#64748b;font-size:13px">Se recebeu este e-mail no spam, marque como "não é spam" para receber os próximos normalmente.</p></div><div style="background:#f8fafc;padding:14px 24px;color:#94a3b8;font-size:12px">Inovar Refrigeração — enviado automaticamente pelo InovarApp</div></div>`)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	err = t.Execute(&out, first[0])
	return out.String(), err
}
func validEmail(value string) bool {
	value = strings.TrimSpace(value)
	address, err := mail.ParseAddress(value)
	return err == nil && address.Address == value
}
func shortEmailError(err error) string {
	value := strings.ReplaceAll(strings.ReplaceAll(err.Error(), "\r", " "), "\n", " ")
	if len(value) > 120 {
		value = value[:120]
	}
	return value
}
