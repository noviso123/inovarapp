package supabase

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

// StoreGoogleConnection encrypts a per-user Google token envelope before writing it
// to the private bucket. The service-role key never leaves the server.
func (c *Client) StoreGoogleConnection(ctx context.Context, userID string, value any) error {
	if c == nil || strings.TrimSpace(c.serviceRoleKey) == "" {
		return ErrNotConfigured
	}
	objectPath := "config/google/" + userID + ".json"
	if !validGoogleConnectionPath(objectPath) {
		return errors.New("invalid Google connection path")
	}
	plain, err := json.Marshal(value)
	if err != nil {
		return err
	}
	key := sha256.Sum256([]byte("inovar-google-v1:" + c.serviceRoleKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	iv := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(iv); err != nil {
		return err
	}
	encrypted := gcm.Seal(nil, iv, plain, nil)
	// Preserve the legacy AES-GCM envelope so the React client can still read
	// stored tokens during the platform migration.
	tagStart := len(encrypted) - gcm.Overhead()
	envelope, err := json.Marshal(map[string]string{
		"iv":   base64.StdEncoding.EncodeToString(iv),
		"tag":  base64.StdEncoding.EncodeToString(encrypted[tagStart:]),
		"data": base64.StdEncoding.EncodeToString(encrypted[:tagStart]),
	})
	if err != nil {
		return err
	}
	target, err := c.storageObjectURL("object", "documentos-inovar", objectPath)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(envelope))
	if err != nil {
		return err
	}
	request.Header.Set("apikey", c.serviceRoleKey)
	request.Header.Set("Authorization", "Bearer "+c.serviceRoleKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-upsert", "true")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("store Google connection returned status %d", response.StatusCode)
	}
	return nil
}

// LoadGoogleConnection retrieves and decrypts the caller's connection envelope.
func (c *Client) LoadGoogleConnection(ctx context.Context, userID string, target any) (bool, error) {
	if c == nil || strings.TrimSpace(c.serviceRoleKey) == "" {
		return false, ErrNotConfigured
	}
	objectPath := "config/google/" + userID + ".json"
	if !validGoogleConnectionPath(objectPath) {
		return false, errors.New("invalid Google connection path")
	}
	url, err := c.storageObjectURL("object", "documentos-inovar", objectPath)
	if err != nil {
		return false, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	request.Header.Set("apikey", c.serviceRoleKey)
	request.Header.Set("Authorization", "Bearer "+c.serviceRoleKey)
	response, err := c.httpClient.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusBadRequest {
		return false, nil
	}
	if response.StatusCode != http.StatusOK {
		return false, fmt.Errorf("load Google connection returned status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return false, err
	}
	var envelope struct {
		IV   string `json:"iv"`
		Tag  string `json:"tag"`
		Data string `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return false, err
	}
	iv, err := decodeConnectionBase64(envelope.IV)
	if err != nil {
		return false, err
	}
	data, err := decodeConnectionBase64(envelope.Data)
	if err != nil {
		return false, err
	}
	if envelope.Tag != "" {
		tag, decodeErr := decodeConnectionBase64(envelope.Tag)
		if decodeErr != nil {
			return false, decodeErr
		}
		data = append(data, tag...)
	}
	key := sha256.Sum256([]byte("inovar-google-v1:" + c.serviceRoleKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return false, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return false, err
	}
	plain, err := gcm.Open(nil, iv, data, nil)
	if err != nil {
		return false, errors.New("invalid Google connection envelope")
	}
	if err := json.Unmarshal(plain, target); err != nil {
		return false, err
	}
	return true, nil
}

func validGoogleConnectionPath(path string) bool {
	if strings.Contains(path, "\\") || strings.Contains(path, "..") {
		return false
	}
	parts := strings.Split(path, "/")
	if len(parts) != 3 || parts[0] != "config" || parts[1] != "google" || !strings.HasSuffix(parts[2], ".json") {
		return false
	}
	_, err := uuid.Parse(strings.TrimSuffix(parts[2], ".json"))
	return err == nil
}

func decodeConnectionBase64(value string) ([]byte, error) {
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		decoded, err := encoding.DecodeString(value)
		if err == nil {
			return decoded, nil
		}
	}
	return nil, errors.New("invalid Google connection envelope encoding")
}

type PrivateStorageObject struct {
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

func (c *Client) ListPrivateObjects(ctx context.Context, prefix string, limit int) ([]PrivateStorageObject, error) {
	if c == nil || strings.TrimSpace(c.serviceRoleKey) == "" {
		return nil, ErrNotConfigured
	}
	if !validAppliancePhotoPrefix(prefix) && !validServicePhotoPrefix(prefix) && !validProfilePhotoPrefix(prefix) && !validPushPrefix(prefix) {
		return nil, errors.New("invalid private storage prefix")
	}
	if limit < 1 || limit > 200 {
		limit = 200
	}
	result, err := c.ServiceRequest(ctx, "/storage/v1/object/list/documentos-inovar", RequestOptions{Method: http.MethodPost, Body: map[string]any{"prefix": prefix, "limit": limit, "sortBy": map[string]string{"column": "created_at", "order": "desc"}}})
	if err != nil {
		return nil, err
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		return nil, fmt.Errorf("list storage objects returned status %d", result.StatusCode)
	}
	var objects []PrivateStorageObject
	if err := json.Unmarshal(result.Body, &objects); err != nil {
		return nil, err
	}
	return objects, nil
}

func (c *Client) DeletePrivateObject(ctx context.Context, objectPath string) error {
	if c == nil || strings.TrimSpace(c.serviceRoleKey) == "" {
		return ErrNotConfigured
	}
	if !validPrivateObjectPath(objectPath) || (!validAppliancePhotoObjectPath(objectPath) && !validServicePhotoObjectPath(objectPath) && !validProfilePhotoObjectPath(objectPath) && !validPushObjectPath(objectPath)) {
		return errors.New("invalid private object path")
	}
	target, err := c.storageObjectURL("object", "documentos-inovar", objectPath)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, target, nil)
	if err != nil {
		return err
	}
	request.Header.Set("apikey", c.serviceRoleKey)
	request.Header.Set("Authorization", "Bearer "+c.serviceRoleKey)
	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusNotFound {
		return fmt.Errorf("delete storage object returned status %d", response.StatusCode)
	}
	return nil
}

// UploadPrivateObject writes a server-generated file to the private app bucket.
// Callers must authenticate and authorize before using the service-role boundary.
func (c *Client) UploadPrivateObject(ctx context.Context, objectPath, contentType string, data []byte) error {
	return c.uploadPrivateObject(ctx, objectPath, contentType, data, false)
}

// UpsertPrivateObject replaces an existing object in the private app bucket.
// Callers must authenticate and authorize before using the service-role boundary.
func (c *Client) UpsertPrivateObject(ctx context.Context, objectPath, contentType string, data []byte) error {
	return c.uploadPrivateObject(ctx, objectPath, contentType, data, true)
}

func (c *Client) uploadPrivateObject(ctx context.Context, objectPath, contentType string, data []byte, upsert bool) error {
	if c == nil || strings.TrimSpace(c.serviceRoleKey) == "" {
		return ErrNotConfigured
	}
	if !validPrivateObjectPath(objectPath) || strings.TrimSpace(contentType) == "" || len(data) == 0 {
		return errors.New("invalid storage object")
	}
	target, err := c.storageObjectURL("object", "documentos-inovar", objectPath)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("apikey", c.serviceRoleKey)
	request.Header.Set("Authorization", "Bearer "+c.serviceRoleKey)
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("x-upsert", fmt.Sprintf("%t", upsert))
	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("upload storage object: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("upload storage object returned status %d", response.StatusCode)
	}
	return nil
}

// CreatePrivateSignedURL creates a temporary download URL for an object in the
// app's private Storage bucket. Expiration is clamped to 1..30 days.
func (c *Client) CreatePrivateSignedURL(ctx context.Context, objectPath string, expiresInSeconds int) (string, error) {
	if c == nil || strings.TrimSpace(c.serviceRoleKey) == "" {
		return "", ErrNotConfigured
	}
	if !validPrivateObjectPath(objectPath) {
		return "", errors.New("invalid storage object path")
	}
	if expiresInSeconds < 1 {
		expiresInSeconds = 1
	}
	if expiresInSeconds > 30*24*60*60 {
		expiresInSeconds = 30 * 24 * 60 * 60
	}
	resourcePath, err := c.storageResourcePath("object", "sign", "documentos-inovar", objectPath)
	if err != nil {
		return "", err
	}
	result, err := c.ServiceRequest(ctx, resourcePath, RequestOptions{
		Method: http.MethodPost,
		Body:   map[string]int{"expiresIn": expiresInSeconds},
	})
	if err != nil {
		return "", err
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		return "", fmt.Errorf("create storage signed url returned status %d", result.StatusCode)
	}
	var payload struct {
		SignedURL string `json:"signedURL"`
	}
	if err := json.Unmarshal(result.Body, &payload); err != nil || strings.TrimSpace(payload.SignedURL) == "" {
		return "", errors.New("storage did not return a signed url")
	}
	parsed, parseErr := url.Parse(payload.SignedURL)
	if parseErr != nil || parsed.Fragment != "" {
		return "", errors.New("storage returned an invalid signed url")
	}
	if parsed.IsAbs() {
		if parsed.Host != c.baseURL.Host || parsed.Scheme != c.baseURL.Scheme {
			return "", errors.New("storage returned a signed url from an unexpected origin")
		}
		return parsed.String(), nil
	}
	base := *c.baseURL
	if parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/object/sign/") {
		return "", errors.New("storage returned an invalid signed url path")
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/storage/v1" + parsed.Path
	base.RawPath = ""
	base.RawQuery = parsed.RawQuery
	return base.String(), nil
}

func (c *Client) storageObjectURL(parts ...string) (string, error) {
	resourcePath, err := c.storageResourcePath(parts...)
	if err != nil {
		return "", err
	}
	target, err := c.endpoint(resourcePath)
	if err != nil {
		return "", err
	}
	return target.String(), nil
}

func (c *Client) storageResourcePath(parts ...string) (string, error) {
	if len(parts) == 0 {
		return "", errors.New("missing storage path")
	}
	encoded := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			return "", errors.New("invalid storage path segment")
		}
		for _, segment := range strings.Split(part, "/") {
			if segment == "" || segment == "." || segment == ".." {
				return "", errors.New("invalid storage path segment")
			}
			encoded = append(encoded, url.PathEscape(segment))
		}
	}
	return "/storage/v1/" + strings.Join(encoded, "/"), nil
}

func validPrivateObjectPath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" || strings.Contains(path, "\\") || strings.Contains(path, "..") {
		return false
	}
	parts := strings.Split(path, "/")
	return len(parts) >= 2 && (parts[0] == "os-orcamentos" || parts[0] == "fotos-os" || parts[0] == "fotos-aparelho" || validProfilePhotoObjectPath(path) || validPushObjectPath(path) || validNativePushObjectPath(path))
}

func validPushPrefix(prefix string) bool {
	parts := strings.Split(strings.TrimSuffix(strings.TrimSpace(prefix), "/"), "/")
	if len(parts) != 2 || (parts[0] != "push" && parts[0] != "push-native") {
		return false
	}
	_, err := uuid.Parse(parts[1])
	return err == nil
}

func validProfilePhotoPrefix(prefix string) bool {
	return strings.TrimSuffix(strings.TrimSpace(prefix), "/") == "fotos-perfil"
}

func validProfilePhotoObjectPath(objectPath string) bool {
	parts := strings.Split(strings.TrimSpace(objectPath), "/")
	if len(parts) != 2 || parts[0] != "fotos-perfil" {
		return false
	}
	userID := strings.TrimSuffix(parts[1], ".jpg")
	if userID == parts[1] || parts[1] != userID+".jpg" {
		return false
	}
	_, err := uuid.Parse(userID)
	return err == nil
}

func IsValidProfilePhotoPath(objectPath string) bool {
	return validProfilePhotoObjectPath(objectPath)
}

func validPushObjectPath(path string) bool {
	parts := strings.Split(strings.TrimSpace(path), "/")
	if len(parts) != 3 || parts[0] != "push" || !strings.HasSuffix(parts[2], ".json") {
		return false
	}
	if _, err := uuid.Parse(parts[1]); err != nil {
		return false
	}
	name := strings.TrimSuffix(parts[2], ".json")
	if len(name) != 64 {
		return false
	}
	_, err := hex.DecodeString(name)
	return err == nil
}

func validNativePushObjectPath(path string) bool {
	parts := strings.Split(strings.TrimSpace(path), "/")
	if len(parts) != 3 || parts[0] != "push-native" || !strings.HasSuffix(parts[2], ".json") {
		return false
	}
	if _, err := uuid.Parse(parts[1]); err != nil {
		return false
	}
	name := strings.TrimSuffix(parts[2], ".json")
	if len(name) != 64 {
		return false
	}
	_, err := hex.DecodeString(name)
	return err == nil
}

func validAppliancePhotoPrefix(prefix string) bool {
	parts := strings.Split(strings.TrimSpace(prefix), "/")
	if len(parts) != 2 || parts[0] != "fotos-aparelho" || parts[1] == "" {
		return false
	}
	if _, err := uuid.Parse(parts[1]); err != nil {
		return false
	}
	return true
}

func validServicePhotoPrefix(prefix string) bool {
	parts := strings.Split(strings.TrimSuffix(strings.TrimSpace(prefix), "/"), "/")
	if len(parts) != 2 || parts[0] != "fotos-os" {
		return false
	}
	_, err := uuid.Parse(parts[1])
	return err == nil
}

func validAppliancePhotoObjectPath(objectPath string) bool {
	parts := strings.Split(strings.TrimSpace(objectPath), "/")
	if len(parts) != 3 || parts[0] != "fotos-aparelho" || parts[2] == "" || strings.ContainsAny(parts[2], "\\:") {
		return false
	}
	if _, err := uuid.Parse(parts[1]); err != nil {
		return false
	}
	return true
}

func validServicePhotoObjectPath(objectPath string) bool {
	parts := strings.Split(strings.TrimSpace(objectPath), "/")
	if len(parts) != 3 || parts[0] != "fotos-os" || parts[2] == "" || strings.ContainsAny(parts[2], "\\:") {
		return false
	}
	if _, err := uuid.Parse(parts[1]); err != nil {
		return false
	}
	return true
}

func IsValidAppliancePhotoPath(objectPath string) bool {
	return validAppliancePhotoObjectPath(objectPath)
}

func IsValidServicePhotoPath(objectPath string) bool {
	return validServicePhotoObjectPath(objectPath)
}
