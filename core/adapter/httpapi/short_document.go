package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"inovarapp/core/adapter/supabase"
)

var shortDocumentCode = regexp.MustCompile(`^[A-Za-z0-9_-]{10,20}$`)
var shortDocumentPath = regexp.MustCompile(`^os-orcamentos/[a-zA-Z0-9_ ./-]+$`)

// ShortDocumentHandler resolves public, opaque document links without exposing
// a storage path or a long-lived signed URL. The reference and target file stay
// in the private Supabase bucket.
type ShortDocumentHandler struct {
	Supabase *supabase.Client
	Now      func() time.Time
}

func (h ShortDocumentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if r.Method != http.MethodGet {
		http.Error(w, "Método não permitido.", http.StatusMethodNotAllowed)
		return
	}
	code := strings.TrimPrefix(r.URL.Path, "/d/")
	if strings.Contains(code, "/") || !shortDocumentCode.MatchString(code) {
		http.Error(w, "Documento não encontrado.", http.StatusNotFound)
		return
	}
	if h.Supabase == nil {
		http.Error(w, "Não foi possível abrir o documento agora. Tente novamente em instantes.", http.StatusServiceUnavailable)
		return
	}
	reference, err := h.Supabase.ServiceRequest(r.Context(), "/storage/v1/object/documentos-inovar/links/"+code+".json", supabase.RequestOptions{Method: http.MethodGet})
	if err != nil {
		http.Error(w, "Não foi possível abrir o documento agora. Tente novamente em instantes.", http.StatusServiceUnavailable)
		return
	}
	if reference.StatusCode != http.StatusOK {
		http.Error(w, "Este link de documento expirou. Solicite um novo à Inovar Refrigeração.", http.StatusGone)
		return
	}
	var item struct {
		Path    string  `json:"path"`
		Expires float64 `json:"expiraEm"`
	}
	if json.Unmarshal(reference.Body, &item) != nil || item.Path == "" || item.Expires <= 0 {
		http.Error(w, "Este link de documento expirou. Solicite um novo à Inovar Refrigeração.", http.StatusGone)
		return
	}
	if !shortDocumentPath.MatchString(item.Path) || strings.Contains(item.Path, "..") || strings.Contains(item.Path, "\\") {
		http.Error(w, "Documento não encontrado.", http.StatusNotFound)
		return
	}
	now := time.Now()
	if h.Now != nil {
		now = h.Now()
	}
	remaining := int((time.UnixMilli(int64(item.Expires)).Sub(now)).Seconds())
	if remaining <= 0 {
		http.Error(w, "Este link de documento expirou. Solicite um novo à Inovar Refrigeração.", http.StatusGone)
		return
	}
	if remaining < 60 {
		remaining = 60
	}
	target, err := h.Supabase.CreatePrivateSignedURL(r.Context(), item.Path, remaining)
	if err != nil {
		var transportErr *supabase.TransportError
		if errors.As(err, &transportErr) || errors.Is(err, supabase.ErrNotConfigured) {
			http.Error(w, "Não foi possível abrir o documento agora. Tente novamente em instantes.", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, "Documento não encontrado.", http.StatusNotFound)
		return
	}
	http.Redirect(w, r, target, http.StatusFound)
}
