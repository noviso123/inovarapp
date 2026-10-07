package email

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	googleTokenURL = "https://oauth2.googleapis.com/token"
	gmailSendURL   = "https://gmail.googleapis.com/gmail/v1/users/me/messages/send"
)

type GoogleAPISender struct {
	Client   *http.Client
	TokenURL string
	SendURL  string
}

type cachedGoogleAccessToken struct {
	value     string
	expiresAt time.Time
}

var googleTokenCache = struct {
	sync.Mutex
	items map[string]cachedGoogleAccessToken
}{items: make(map[string]cachedGoogleAccessToken)}

func (s GoogleAPISender) Send(ctx context.Context, config Config, message Message) (string, error) {
	config.Username = strings.TrimSpace(config.Username)
	if strings.TrimSpace(config.GoogleRefreshToken) == "" || strings.TrimSpace(config.GoogleClientID) == "" || strings.TrimSpace(config.GoogleClientSecret) == "" {
		return "", errors.New("conexão OAuth do Gmail incompleta")
	}
	if strings.ContainsAny(message.To+message.Subject, "\r\n") || strings.TrimSpace(message.HTML) == "" {
		return "", errors.New("mensagem inválida")
	}
	to, err := mail.ParseAddress(message.To)
	if err != nil || to.Address != message.To {
		return "", errors.New("e-mail do destinatário inválido")
	}
	from, err := mail.ParseAddress(config.Username)
	if err != nil || from.Address != config.Username {
		return "", errors.New("conta Google conectada inválida")
	}
	raw, err := gmailMIME(from, to, message)
	if err != nil {
		return "", err
	}
	token, err := s.accessToken(ctx, config)
	if err != nil {
		return "", err
	}
	messageID, status, err := s.sendRaw(ctx, token, raw)
	if err != nil {
		return "", err
	}
	if status == http.StatusUnauthorized {
		invalidateGoogleAccessToken(config.GoogleRefreshToken)
		token, err = s.accessToken(ctx, config)
		if err == nil {
			messageID, status, err = s.sendRaw(ctx, token, raw)
		}
	}
	if err != nil {
		return "", err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return "", fmt.Errorf("Gmail API recusou o envio (HTTP %d); confirme se Gmail API e a permissão de envio estão habilitadas", status)
	}
	return messageID, nil
}

func gmailMIME(from, to *mail.Address, message Message) ([]byte, error) {
	var raw bytes.Buffer
	_, _ = fmt.Fprintf(&raw, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n",
		(&mail.Address{Name: "Inovar Refrigeração", Address: from.Address}).String(),
		to.String(), mime.QEncoding.Encode("UTF-8", message.Subject))
	encoded := quotedprintable.NewWriter(&raw)
	if _, err := encoded.Write([]byte(message.HTML + "\r\n")); err != nil {
		_ = encoded.Close()
		return nil, err
	}
	if err := encoded.Close(); err != nil {
		return nil, err
	}
	return raw.Bytes(), nil
}

func (s GoogleAPISender) accessToken(ctx context.Context, config Config) (string, error) {
	key := googleRefreshTokenKey(config.GoogleRefreshToken)
	googleTokenCache.Lock()
	defer googleTokenCache.Unlock()
	if cached, ok := googleTokenCache.items[key]; ok && time.Until(cached.expiresAt) > time.Minute {
		return cached.value, nil
	}
	now := time.Now()
	for cachedKey, value := range googleTokenCache.items {
		if !value.expiresAt.After(now) {
			delete(googleTokenCache.items, cachedKey)
		}
	}
	tokenURL := s.TokenURL
	if tokenURL == "" {
		tokenURL = googleTokenURL
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {config.GoogleRefreshToken},
		"client_id":     {config.GoogleClientID},
		"client_secret": {config.GoogleClientSecret},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := s.client().Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 2048))
		return "", fmt.Errorf("Google não renovou a autorização do Gmail (HTTP %d); reconecte a conta", response.StatusCode)
	}
	var tokenResponse struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&tokenResponse); err != nil || strings.TrimSpace(tokenResponse.AccessToken) == "" {
		return "", errors.New("Google não retornou um token de acesso válido")
	}
	if tokenResponse.ExpiresIn <= 0 {
		tokenResponse.ExpiresIn = 3600
	}
	googleTokenCache.items[key] = cachedGoogleAccessToken{value: tokenResponse.AccessToken, expiresAt: time.Now().Add(time.Duration(tokenResponse.ExpiresIn) * time.Second)}
	return tokenResponse.AccessToken, nil
}

func (s GoogleAPISender) sendRaw(ctx context.Context, accessToken string, raw []byte) (string, int, error) {
	sendURL := s.SendURL
	if sendURL == "" {
		sendURL = gmailSendURL
	}
	payload, err := json.Marshal(map[string]string{"raw": base64.RawURLEncoding.EncodeToString(raw)})
	if err != nil {
		return "", 0, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, sendURL, bytes.NewReader(payload))
	if err != nil {
		return "", 0, err
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client().Do(request)
	if err != nil {
		return "", 0, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 2048))
		return "", response.StatusCode, nil
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		return "", response.StatusCode, err
	}
	return result.ID, response.StatusCode, nil
}

func (s GoogleAPISender) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func googleRefreshTokenKey(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func invalidateGoogleAccessToken(refreshToken string) {
	googleTokenCache.Lock()
	delete(googleTokenCache.items, googleRefreshTokenKey(refreshToken))
	googleTokenCache.Unlock()
}
