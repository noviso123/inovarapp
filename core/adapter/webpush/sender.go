// Package webpush implements the browser Web Push protocol with the Go
// standard library. VAPID secrets are intended to remain server-side.
package webpush

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var ErrSubscriptionExpired = errors.New("web push subscription expired")

type Subscription struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256DH string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

type Payload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
	Path  string `json:"path"`
	Tag   string `json:"tag,omitempty"`
}

type Sender struct {
	PublicKey  string
	PrivateKey string
	Subject    string
	HTTPClient *http.Client
	AllowHTTP  bool
}

func ValidateSubscription(subscription Subscription) error {
	endpoint, err := url.Parse(subscription.Endpoint)
	if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" || endpoint.User != nil || endpoint.Fragment != "" {
		return errors.New("invalid web push endpoint")
	}
	public, err := base64.RawURLEncoding.DecodeString(subscription.Keys.P256DH)
	if err != nil || len(public) != 65 || public[0] != 4 {
		return errors.New("invalid Web Push p256dh key")
	}
	if _, err := ecdh.P256().NewPublicKey(public); err != nil {
		return errors.New("invalid Web Push p256dh key")
	}
	auth, err := base64.RawURLEncoding.DecodeString(subscription.Keys.Auth)
	if err != nil || len(auth) != 16 {
		return errors.New("invalid Web Push authentication secret")
	}
	return nil
}

func (s Sender) Send(ctx context.Context, subscription Subscription, payload Payload) error {
	endpoint, err := url.Parse(subscription.Endpoint)
	if err != nil || endpoint.Host == "" || endpoint.User != nil || endpoint.Fragment != "" || (endpoint.Scheme != "https" && !(s.AllowHTTP && endpoint.Scheme == "http")) {
		return errors.New("invalid web push endpoint")
	}
	if !s.AllowHTTP {
		if err := validatePublicEndpoint(ctx, endpoint.Hostname()); err != nil {
			return err
		}
	}
	if payload.Path == "" {
		payload.Path = payload.URL
	}
	if !strings.HasPrefix(payload.Path, "/") {
		payload.Path = "/"
	}
	if payload.URL == "" {
		payload.URL = payload.Path
	}
	body, err := encrypt(subscription, payload)
	if err != nil {
		return err
	}
	authorization, publicKey, err := s.authorization(endpoint)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("TTL", "86400")
	request.Header.Set("Urgency", "high")
	request.Header.Set("Content-Encoding", "aes128gcm")
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("Authorization", authorization)
	request.Header.Set("Crypto-Key", "p256ecdsa="+publicKey)
	client := s.HTTPClient
	if client == nil {
		client = securePushHTTPClient()
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("send web push: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode == http.StatusGone || response.StatusCode == http.StatusNotFound {
		return ErrSubscriptionExpired
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("web push service returned status %d", response.StatusCode)
	}
	return nil
}

func validatePublicEndpoint(ctx context.Context, host string) error {
	if ip, err := netip.ParseAddr(host); err == nil {
		if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			return errors.New("web push endpoint must use a public network address")
		}
		return nil
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") || strings.HasSuffix(strings.ToLower(host), ".local") {
		return errors.New("web push endpoint must use a public network address")
	}
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addresses) == 0 {
		return errors.New("could not resolve web push endpoint")
	}
	for _, address := range addresses {
		if !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() {
			return errors.New("web push endpoint must use a public network address")
		}
	}
	return nil
}

func securePushHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
					return nil, errors.New("web push endpoint resolved to a private address")
				}
			}
			if len(ips) == 0 {
				return nil, errors.New("web push endpoint has no address")
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
	}
	return &http.Client{Transport: transport, Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (s Sender) authorization(endpoint *url.URL) (string, string, error) {
	privateBytes, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(s.PrivateKey))
	if err != nil || len(privateBytes) != 32 {
		return "", "", errors.New("VAPID private key is not configured correctly")
	}
	d := new(big.Int).SetBytes(privateBytes)
	curve := elliptic.P256()
	if d.Sign() <= 0 || d.Cmp(curve.Params().N) >= 0 {
		return "", "", errors.New("VAPID private key is not configured correctly")
	}
	x, y := curve.ScalarBaseMult(privateBytes)
	key := &ecdsa.PrivateKey{PublicKey: ecdsa.PublicKey{Curve: curve, X: x, Y: y}, D: d}
	public := base64.RawURLEncoding.EncodeToString(elliptic.Marshal(curve, x, y))
	if s.PublicKey != "" && strings.TrimSpace(s.PublicKey) != public {
		return "", "", errors.New("VAPID public and private keys do not match")
	}
	subject := strings.TrimSpace(s.Subject)
	if subject == "" {
		subject = "mailto:contato@inovarapp.vercel.app"
	}
	header, _ := json.Marshal(map[string]string{"typ": "JWT", "alg": "ES256"})
	claims, _ := json.Marshal(map[string]any{"aud": endpoint.Scheme + "://" + endpoint.Host, "exp": time.Now().Add(12 * time.Hour).Unix(), "sub": subject})
	message := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(message))
	r, ss, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", "", err
	}
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	ss.FillBytes(signature[32:])
	return "vapid t=" + message + "." + base64.RawURLEncoding.EncodeToString(signature) + ", k=" + public, public, nil
}

func encrypt(subscription Subscription, payload Payload) ([]byte, error) {
	clientPublic, err := base64.RawURLEncoding.DecodeString(subscription.Keys.P256DH)
	if err != nil || len(clientPublic) != 65 || clientPublic[0] != 4 {
		return nil, errors.New("invalid Web Push p256dh key")
	}
	auth, err := base64.RawURLEncoding.DecodeString(subscription.Keys.Auth)
	if err != nil || len(auth) != 16 {
		return nil, errors.New("invalid Web Push authentication secret")
	}
	curve := ecdh.P256()
	recipient, err := curve.NewPublicKey(clientPublic)
	if err != nil {
		return nil, errors.New("invalid Web Push recipient key")
	}
	server, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	shared, err := server.ECDH(recipient)
	if err != nil {
		return nil, err
	}
	serverPublic := server.PublicKey().Bytes()
	info := append([]byte("WebPush: info\x00"), clientPublic...)
	info = append(info, serverPublic...)
	inputKey, err := hkdf.Key(sha256.New, shared, auth, string(info), 32)
	if err != nil {
		return nil, err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	prk, err := hkdf.Extract(sha256.New, inputKey, salt)
	if err != nil {
		return nil, err
	}
	key, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}
	plain, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	plain = append(plain, 2)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nil, nonce, plain, nil)
	result := make([]byte, 0, 16+4+1+len(serverPublic)+len(ciphertext))
	result = append(result, salt...)
	recordSize := make([]byte, 4)
	binary.BigEndian.PutUint32(recordSize, 4096)
	result = append(result, recordSize...)
	result = append(result, byte(len(serverPublic)))
	result = append(result, serverPublic...)
	result = append(result, ciphertext...)
	return result, nil
}
