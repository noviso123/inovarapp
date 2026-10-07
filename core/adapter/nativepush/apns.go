package nativepush

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
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

type APNsConfig struct {
	TeamID      string
	KeyID       string
	BundleID    string
	PrivateKey  string
	Environment string
}

type APNsSender struct {
	config    APNsConfig
	key       *ecdsa.PrivateKey
	client    *http.Client
	tokenMu   sync.Mutex
	access    string
	tokenTill time.Time
}

func NewAPNs(config APNsConfig, client *http.Client) (*APNsSender, error) {
	config.TeamID = strings.TrimSpace(config.TeamID)
	config.KeyID = strings.TrimSpace(config.KeyID)
	config.BundleID = strings.TrimSpace(config.BundleID)
	config.Environment = strings.ToLower(strings.TrimSpace(config.Environment))
	if config.BundleID == "" {
		config.BundleID = "com.inovarapp.mobile"
	}
	if config.Environment == "" {
		config.Environment = "production"
	}
	if config.Environment != "production" && config.Environment != "sandbox" {
		return nil, errors.New("APNs environment must be production or sandbox")
	}
	if config.TeamID == "" || config.KeyID == "" || config.PrivateKey == "" {
		return nil, errors.New("APNs configuration is missing team ID, key ID or private key")
	}
	block, _ := pem.Decode([]byte(config.PrivateKey))
	if block == nil {
		return nil, errors.New("APNs private key is not valid PEM")
	}
	var key *ecdsa.PrivateKey
	if parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		key, _ = parsed.(*ecdsa.PrivateKey)
	} else if parsed, sec1Err := x509.ParseECPrivateKey(block.Bytes); sec1Err == nil {
		key = parsed
	}
	if key == nil || key.Curve != elliptic.P256() {
		return nil, errors.New("APNs private key must be an elliptic curve P-256 key")
	}
	if client == nil {
		client = &http.Client{Timeout: 12 * time.Second}
	}
	return &APNsSender{config: config, key: key, client: client}, nil
}

func (s *APNsSender) Send(ctx context.Context, token, title, body, path string) error {
	if s == nil || s.key == nil {
		return errors.New("APNs is not configured")
	}
	if len(token) != 64 {
		return errors.New("APNs device token must be 64 hexadecimal characters")
	}
	if _, err := hex.DecodeString(token); err != nil {
		return errors.New("APNs device token must be 64 hexadecimal characters")
	}
	access, err := s.providerToken()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(path, "/") {
		path = "/"
	}
	data, err := json.Marshal(map[string]any{
		"aps": map[string]any{"alert": map[string]string{"title": title, "body": body}, "sound": "default"},
		"url": path,
	})
	if err != nil {
		return err
	}
	host := "api.push.apple.com"
	if s.config.Environment == "sandbox" {
		host = "api.sandbox.push.apple.com"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+host+"/3/device/"+url.PathEscape(token), bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("authorization", "bearer "+access)
	request.Header.Set("apns-topic", s.config.BundleID)
	request.Header.Set("apns-push-type", "alert")
	request.Header.Set("apns-priority", "10")
	request.Header.Set("content-type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 16<<10))
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return nil
	}
	if response.StatusCode == http.StatusGone && bytes.Contains(responseBody, []byte("Unregistered")) {
		return push.ErrUnregistered
	}
	return fmt.Errorf("Apple Push Notification service returned HTTP %d", response.StatusCode)
}

func (s *APNsSender) providerToken() (string, error) {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	if s.access != "" && time.Until(s.tokenTill) > time.Minute {
		return s.access, nil
	}
	encode := base64.RawURLEncoding.EncodeToString
	header, err := json.Marshal(map[string]string{"alg": "ES256", "kid": s.config.KeyID})
	if err != nil {
		return "", err
	}
	claims, err := json.Marshal(map[string]any{"iss": s.config.TeamID, "iat": time.Now().Unix()})
	if err != nil {
		return "", err
	}
	unsigned := encode(header) + "." + encode(claims)
	digest := sha256.Sum256([]byte(unsigned))
	r, signatureS, err := ecdsa.Sign(rand.Reader, s.key, digest[:])
	if err != nil {
		return "", err
	}
	rBytes, sBytes := r.FillBytes(make([]byte, 32)), signatureS.FillBytes(make([]byte, 32))
	signature := append(rBytes, sBytes...)
	s.access = unsigned + "." + encode(signature)
	s.tokenTill = time.Now().Add(50 * time.Minute)
	return s.access, nil
}
