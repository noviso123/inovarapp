package supabase

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestGoogleConnectionStorageKeepsLegacyEnvelopeCompatible(t *testing.T) {
	const userID, serviceRole = "11111111-1111-4111-8111-111111111111", "server-role"
	legacyEnvelope := makeLegacyGoogleEnvelope(t, serviceRole, []byte(`{"accessToken":"old-access","refreshToken":"old-refresh"}`))
	var stored []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/storage/v1/object/documentos-inovar/config/google/"+userID+".json" {
			t.Errorf("Google connection storage path=%q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer "+serviceRole {
			t.Errorf("missing server authorization")
		}
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write(legacyEnvelope)
		case http.MethodPost:
			stored, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected method %s", r.Method)
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	client, err := New(Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: serviceRole})
	if err != nil {
		t.Fatal(err)
	}
	var connection struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	loaded, err := client.LoadGoogleConnection(t.Context(), userID, &connection)
	if err != nil || !loaded || connection.AccessToken != "old-access" || connection.RefreshToken != "old-refresh" {
		t.Fatalf("legacy connection=%#v loaded=%t err=%v", connection, loaded, err)
	}
	if err := client.StoreGoogleConnection(t.Context(), userID, connection); err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		IV   string `json:"iv"`
		Tag  string `json:"tag"`
		Data string `json:"data"`
	}
	if err := json.Unmarshal(stored, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.IV == "" || envelope.Tag == "" || envelope.Data == "" {
		t.Fatalf("stored envelope does not match legacy shape: %s", stored)
	}
	plain := openLegacyGoogleEnvelope(t, serviceRole, envelope)
	var restored map[string]string
	if err := json.Unmarshal(plain, &restored); err != nil || restored["accessToken"] != "old-access" {
		t.Fatalf("stored credentials=%s err=%v", plain, err)
	}
}

func makeLegacyGoogleEnvelope(t *testing.T, serviceRole string, plain []byte) []byte {
	t.Helper()
	key := sha256.Sum256([]byte("inovar-google-v1:" + serviceRole))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	iv := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(iv); err != nil {
		t.Fatal(err)
	}
	sealed := gcm.Seal(nil, iv, plain, nil)
	split := len(sealed) - gcm.Overhead()
	envelope, err := json.Marshal(map[string]string{
		"iv":   base64.StdEncoding.EncodeToString(iv),
		"tag":  base64.StdEncoding.EncodeToString(sealed[split:]),
		"data": base64.StdEncoding.EncodeToString(sealed[:split]),
	})
	if err != nil {
		t.Fatal(err)
	}
	return envelope
}

func openLegacyGoogleEnvelope(t *testing.T, serviceRole string, envelope struct {
	IV   string `json:"iv"`
	Tag  string `json:"tag"`
	Data string `json:"data"`
}) []byte {
	t.Helper()
	key := sha256.Sum256([]byte("inovar-google-v1:" + serviceRole))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	iv, err := base64.StdEncoding.DecodeString(envelope.IV)
	if err != nil {
		t.Fatal(err)
	}
	data, err := base64.StdEncoding.DecodeString(envelope.Data)
	if err != nil {
		t.Fatal(err)
	}
	tag, err := base64.StdEncoding.DecodeString(envelope.Tag)
	if err != nil {
		t.Fatal(err)
	}
	returnValue, err := gcm.Open(nil, iv, append(data, tag...), nil)
	if err != nil {
		t.Fatal(err)
	}
	return returnValue
}

func TestUploadPrivateObjectAndCreateSignedURLUseServiceRoleAndPrivateBucket(t *testing.T) {
	var uploadedPath, uploadAuth, contentType string
	var expiry int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("apikey") != "server-role" || r.Header.Get("Authorization") != "Bearer server-role" {
			t.Errorf("service role headers missing: %v", r.Header)
		}
		switch r.URL.Path {
		case "/storage/v1/object/documentos-inovar/os-orcamentos/2026/os-1.pdf":
			uploadedPath, uploadAuth, contentType = r.URL.Path, r.Header.Get("x-upsert"), r.Header.Get("Content-Type")
			body, err := io.ReadAll(r.Body)
			if err != nil || string(body) != "pdf bytes" {
				t.Errorf("upload body=%q err=%v", body, err)
			}
			w.WriteHeader(http.StatusOK)
		case "/storage/v1/object/sign/documentos-inovar/os-orcamentos/2026/os-1.pdf":
			var payload struct {
				ExpiresIn int `json:"expiresIn"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			expiry = payload.ExpiresIn
			_, _ = io.WriteString(w, `{"signedURL":"/object/sign/documentos-inovar/os-orcamentos/2026/os-1.pdf?token=signed"}`)
		default:
			t.Errorf("unexpected storage path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := New(Config{URL: server.URL, AnonKey: "anon", ServiceRoleKey: "server-role"})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.UploadPrivateObject(t.Context(), "os-orcamentos/2026/os-1.pdf", "application/pdf", []byte("pdf bytes")); err != nil {
		t.Fatal(err)
	}
	link, err := client.CreatePrivateSignedURL(t.Context(), "os-orcamentos/2026/os-1.pdf", 40*24*60*60)
	if err != nil {
		t.Fatal(err)
	}
	if uploadedPath == "" || uploadAuth != "false" || contentType != "application/pdf" || expiry != 30*24*60*60 {
		t.Fatalf("upload path=%q upsert=%q contentType=%q expiry=%d", uploadedPath, uploadAuth, contentType, expiry)
	}
	parsedLink, parseErr := url.Parse(link)
	if parseErr != nil || parsedLink.Path != "/storage/v1/object/sign/documentos-inovar/os-orcamentos/2026/os-1.pdf" || parsedLink.Query().Get("token") != "signed" {
		t.Fatalf("signed link=%q", link)
	}
}

func TestStorageObjectPathsRejectTraversalAndUnapprovedFolders(t *testing.T) {
	for _, path := range []string{"", "config/tecnico.json", "os-orcamentos/../private.pdf", "fotos-os\\private.png"} {
		if validPrivateObjectPath(path) {
			t.Errorf("unexpectedly allowed object path %q", path)
		}
	}
	valid := "fotos-os/22222222-2222-4222-8222-222222222222/foto.jpg"
	if !IsValidServicePhotoPath(valid) || !validServicePhotoPrefix("fotos-os/22222222-2222-4222-8222-222222222222") {
		t.Fatal("valid service photo paths/prefix were rejected")
	}
	for _, path := range []string{
		"fotos-os/not-a-uuid/foto.jpg",
		"fotos-os/22222222-2222-4222-8222-222222222222/../private.jpg",
		"fotos-aparelho/22222222-2222-4222-8222-222222222222/foto.jpg",
	} {
		if IsValidServicePhotoPath(path) {
			t.Errorf("unexpectedly allowed service photo path %q", path)
		}
	}
	profilePath := "fotos-perfil/22222222-2222-4222-8222-222222222222.jpg"
	if !IsValidProfilePhotoPath(profilePath) || !validProfilePhotoPrefix("fotos-perfil/") || !validPrivateObjectPath(profilePath) {
		t.Fatal("valid profile photo path/prefix were rejected")
	}
	for _, path := range []string{
		"fotos-perfil/not-a-uuid.jpg",
		"fotos-perfil/22222222-2222-4222-8222-222222222222/other.jpg",
		"fotos-perfil/22222222-2222-4222-8222-222222222222.jpg/../private.jpg",
	} {
		if IsValidProfilePhotoPath(path) || validPrivateObjectPath(path) {
			t.Errorf("unexpectedly allowed profile photo path %q", path)
		}
	}
}
