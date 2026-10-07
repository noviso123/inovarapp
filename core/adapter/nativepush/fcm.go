// Package nativepush sends Android alerts through FCM HTTP v1 and iOS alerts
// through APNs HTTP/2 using platform-specific credentials and tokens.
package nativepush

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"inovarapp/core/adapter/push"
)

const fcmScope = "https://www.googleapis.com/auth/firebase.messaging"

type ServiceAccount struct {
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

type Sender struct {
	account   ServiceAccount
	key       *rsa.PrivateKey
	client    *http.Client
	tokenMu   sync.Mutex
	access    string
	tokenTill time.Time
}

func New(credentials []byte, client *http.Client) (*Sender, error) {
	var account ServiceAccount
	if err := json.Unmarshal(credentials, &account); err != nil {
		return nil, fmt.Errorf("decode Firebase service account: %w", err)
	}
	if account.ProjectID == "" || account.ClientEmail == "" || account.PrivateKey == "" {
		return nil, errors.New("Firebase service account is missing project_id, client_email or private_key")
	}
	if account.TokenURI == "" {
		account.TokenURI = "https://oauth2.googleapis.com/token"
	}
	tokenURL, err := url.Parse(account.TokenURI)
	if err != nil || tokenURL.Scheme != "https" || tokenURL.Host == "" || tokenURL.User != nil {
		return nil, errors.New("Firebase token_uri must be an HTTPS URL")
	}
	block, _ := pem.Decode([]byte(account.PrivateKey))
	if block == nil {
		return nil, errors.New("Firebase private_key is not valid PEM")
	}
	var key *rsa.PrivateKey
	if parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		key, _ = parsed.(*rsa.PrivateKey)
	} else if parsed, pkcs1Err := x509.ParsePKCS1PrivateKey(block.Bytes); pkcs1Err == nil {
		key = parsed
	}
	if key == nil {
		return nil, errors.New("Firebase private_key must be an RSA private key")
	}
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	return &Sender{account: account, key: key, client: client}, nil
}

func (s *Sender) Configured() bool { return s != nil && s.key != nil }

func (s *Sender) SendAndroid(ctx context.Context, token, title, body, path string) error {
	if !s.Configured() {
		return errors.New("Firebase Cloud Messaging is not configured")
	}
	access, err := s.bearerToken(ctx)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(path, "/") {
		path = "/"
	}
	payload := map[string]any{"message": map[string]any{
		"token":        token,
		"notification": map[string]string{"title": title, "body": body},
		"data":         map[string]string{"url": path},
	}}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint := "https://fcm.googleapis.com/v1/projects/" + url.PathEscape(s.account.ProjectID) + "/messages:send"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+access)
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 16<<10))
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}
	if response.StatusCode == http.StatusNotFound && bytes.Contains(responseBody, []byte("UNREGISTERED")) {
		return push.ErrUnregistered
	}
	return fmt.Errorf("Firebase Cloud Messaging returned HTTP %d", response.StatusCode)
}

func (s *Sender) bearerToken(ctx context.Context) (string, error) {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	if s.access != "" && time.Until(s.tokenTill) > time.Minute {
		return s.access, nil
	}
	now := time.Now()
	claims, err := json.Marshal(map[string]any{
		"iss": s.account.ClientEmail, "scope": fcmScope,
		"aud": s.account.TokenURI, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	})
	if err != nil {
		return "", err
	}
	encode := base64.RawURLEncoding.EncodeToString
	header := encode([]byte(`{"alg":"RS256","typ":"JWT"}`))
	unsigned := header + "." + encode(claims)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	assertion := unsigned + "." + encode(signature)
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.account.TokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := s.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 16<<10))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("Firebase OAuth token endpoint returned HTTP %d", response.StatusCode)
	}
	var token struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(responseBody, &token); err != nil || token.AccessToken == "" || token.ExpiresIn <= 0 {
		return "", errors.New("Firebase OAuth token response is invalid")
	}
	s.access, s.tokenTill = token.AccessToken, now.Add(time.Duration(token.ExpiresIn)*time.Second)
	return s.access, nil
}
