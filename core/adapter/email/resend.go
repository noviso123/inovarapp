package email

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

type ResendSender struct{ Client *http.Client }

func (s ResendSender) Send(ctx context.Context, config Config, message Message) (string, error) {
	if strings.TrimSpace(config.APIKey) == "" || strings.TrimSpace(config.From) == "" {
		return "", errors.New("Resend não configurado")
	}
	body, err := json.Marshal(map[string]any{"from": config.From, "to": []string{message.To}, "subject": message.Subject, "html": message.HTML})
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+config.APIKey)
	request.Header.Set("Content-Type", "application/json")
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 2048))
		return "", errors.New("falha no envio pelo Resend")
	}
	var result struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result)
	return result.ID, nil
}

// ConfiguredSender keeps the legacy cron's Resend preference and uses the
// Gmail SMTP configuration saved in technician settings as its fallback.
type ConfiguredSender struct {
	Resend Sender
	SMTP   Sender
	Google Sender
}

func (s ConfiguredSender) Send(ctx context.Context, config Config, message Message) (string, error) {
	if strings.TrimSpace(config.GoogleRefreshToken) != "" {
		sender := s.Google
		if sender == nil {
			sender = GoogleAPISender{}
		}
		return sender.Send(ctx, config, message)
	}
	if strings.TrimSpace(config.APIKey) != "" && strings.TrimSpace(config.From) != "" {
		sender := s.Resend
		if sender == nil {
			sender = ResendSender{}
		}
		return sender.Send(ctx, config, message)
	}
	sender := s.SMTP
	if sender == nil {
		sender = SMTPSender{}
	}
	return sender.Send(ctx, config, message)
}
