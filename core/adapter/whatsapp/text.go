package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type Config struct {
	ProprioURL     string
	ProprioToken   string
	ProprioSession string
}

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type Sender struct {
	HTTPClient HTTPDoer
}

// DocumentDelivery reports whether WhatsApp received the PDF as an attachment
// or whether the sender had to fall back to sending its signed link as text.
type DocumentDelivery struct {
	Attached bool
	LinkSent bool
}

var phoneDigits = regexp.MustCompile(`\D`)

func NormalizePhone(value string) (string, bool) {
	number := phoneDigits.ReplaceAllString(value, "")
	if strings.HasPrefix(number, "55") && len(number) >= 12 {
		return number, true
	}
	if len(number) >= 10 && len(number) <= 11 {
		return "55" + number, true
	}
	return number, len(number) >= 10
}

func (s Sender) proprioConfigured(config Config) bool {
	return strings.TrimSpace(config.ProprioURL) != "" && strings.TrimSpace(config.ProprioToken) != "" && strings.TrimSpace(config.ProprioSession) != ""
}

func (s Sender) proprioSession(config Config) string {
	session := strings.TrimSpace(config.ProprioSession)
	if session == "" {
		session = "inovar"
	}
	return session
}

func (s Sender) SendText(ctx context.Context, config Config, phone, message string) error {
	number, valid := NormalizePhone(phone)
	if !valid {
		return errors.New("telefone invalido")
	}
	if strings.TrimSpace(message) == "" {
		return errors.New("mensagem vazia")
	}
	if s.proprioConfigured(config) {
		return s.sendProprioText(ctx, config, number, message)
	}
	return errors.New("whatsapp nao configurado")
}

// SendDocument entrega um documento (PDF) via o sistema próprio de WhatsApp.
// O documento é baixado da URL assinada e reenviado como anexo multipart; se o
// upload falhar, preserva o fallback legado de enviar o link como texto.
func (s Sender) SendDocument(ctx context.Context, config Config, phone, message, documentURL, documentName string) error {
	_, err := s.SendDocumentWithResult(ctx, config, phone, message, documentURL, documentName)
	return err
}

func (s Sender) SendDocumentWithResult(ctx context.Context, config Config, phone, message, documentURL, documentName string) (DocumentDelivery, error) {
	number, valid := NormalizePhone(phone)
	if !valid {
		return DocumentDelivery{}, errors.New("telefone invalido")
	}
	if strings.TrimSpace(documentURL) == "" {
		return DocumentDelivery{}, errors.New("documento invalido")
	}
	if strings.TrimSpace(documentName) == "" {
		documentName = "Inovar.pdf"
	}
	if s.proprioConfigured(config) {
		if err := s.sendProprioDocument(ctx, config, number, message, documentURL, documentName); err == nil {
			return DocumentDelivery{Attached: true}, nil
		}
		fallback := strings.TrimSpace(message) + "\n\n📎 Documento: " + documentURL
		if err := s.sendProprioText(ctx, config, number, fallback); err != nil {
			return DocumentDelivery{}, err
		}
		return DocumentDelivery{LinkSent: true}, nil
	}
	return DocumentDelivery{}, errors.New("whatsapp nao configurado")
}

func (s Sender) sendProprioText(ctx context.Context, config Config, number, message string) error {
	base := strings.TrimRight(strings.TrimSpace(config.ProprioURL), "/")
	session := s.proprioSession(config)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/sessions/"+url.PathEscape(session)+"/messages", bytes.NewBuffer(mustJSON(map[string]string{
		"to":   number + "@s.whatsapp.net",
		"text": message,
	})))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.ProprioToken)
	response, err := s.client().Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errors.New("falha sistema proprio")
	}
	return nil
}

// downloadDocument baixa o documento da URL assinada (Supabase) para upload multipart.
func (s Sender) downloadDocument(ctx context.Context, documentURL string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, documentURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := s.client().Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, errors.New("falha ao baixar documento")
	}
	return io.ReadAll(io.LimitReader(response.Body, 20<<20))
}

func (s Sender) sendProprioDocument(ctx context.Context, config Config, number, message, documentURL, documentName string) error {
	content, err := s.downloadDocument(ctx, documentURL)
	if err != nil {
		return err
	}
	base := strings.TrimRight(strings.TrimSpace(config.ProprioURL), "/")
	session := s.proprioSession(config)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("to", number+"@s.whatsapp.net"); err != nil {
		return err
	}
	if err := writer.WriteField("caption", message); err != nil {
		return err
	}
	part, err := writer.CreateFormFile("file", documentName)
	if err != nil {
		return err
	}
	if _, err := part.Write(content); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/sessions/"+url.PathEscape(session)+"/messages", body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+config.ProprioToken)
	response, err := s.client().Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errors.New("falha sistema proprio documento")
	}
	return nil
}

func (s Sender) client() HTTPDoer {
	if s.HTTPClient != nil {
		return s.HTTPClient
	}
	return &http.Client{Timeout: 25 * time.Second}
}

func mustJSON(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}
