package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"inovarapp/core/adapter/supabase"
)

var documentNameChars = regexp.MustCompile(`[^A-Za-z0-9_. -]+`)

// DocumentsHandler ports the upload and signed-link actions used for PDFs.
// Objects stay private in Supabase Storage; access is granted with short-lived
// signed URLs after authenticating a team member.
type DocumentsHandler struct{ Supabase *supabase.Client }

type documentRequest struct {
	Action      string `json:"acao"`
	Name        string `json:"nome"`
	Base64      string `json:"base64"`
	Path        string `json:"path"`
	Days        int    `json:"dias"`
	Folder      string `json:"pasta"`
	RefID       string `json:"refId"`
	ApplianceID string `json:"aparelhoId"`
	ServiceID   string `json:"serviceId"`
}

func (h DocumentsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	r.Body = http.MaxBytesReader(w, r.Body, 14<<20)
	var input documentRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Solicitação inválida"})
		return
	}
	if input.Action == "fotoperfil" || input.Action == "fotoperfil-url" || input.Action == "fotoperfil-remover" {
		h.profilePhotoAction(w, r, caller, input)
		return
	}
	if !isTeamRole(caller.Role) {
		writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Somente a equipe Inovar pode enviar documentos"})
		return
	}
	switch input.Action {
	case "upload":
		if input.Folder == "aparelho" {
			h.uploadAppliancePhoto(w, r, caller, input)
		} else if input.Folder == "fotos" {
			h.uploadServicePhoto(w, r, caller, input)
		} else {
			h.upload(w, r, input)
		}
	case "link":
		h.link(w, r, input)
	case "fotos-aparelho":
		h.listAppliancePhotos(w, r, caller, input.ApplianceID)
	case "fotos":
		h.listServicePhotos(w, r, caller, input.ServiceID)
	case "excluirfoto":
		if supabase.IsValidServicePhotoPath(strings.TrimSpace(input.Path)) {
			h.deleteServicePhoto(w, r, caller, input.Path)
		} else {
			h.deleteAppliancePhoto(w, r, caller, input.Path)
		}
	default:
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Ação inválida"})
	}
}

func (h DocumentsHandler) profilePhotoAction(w http.ResponseWriter, r *http.Request, caller supabase.Caller, input documentRequest) {
	userID := strings.TrimSpace(caller.UserID)
	if _, err := uuid.Parse(userID); err != nil {
		writeResourceJSON(w, http.StatusUnauthorized, map[string]string{"error": "Não autenticado"})
		return
	}
	objectPath := "fotos-perfil/" + userID + ".jpg"
	switch input.Action {
	case "fotoperfil":
		if len(input.Base64) > 900_000 {
			writeResourceJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "Foto muito grande (máx. ~650KB)"})
			return
		}
		matches := profilePhotoDataURL.FindStringSubmatch(input.Base64)
		if len(matches) != 3 {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Envie uma imagem (PNG/JPEG/WebP) em base64"})
			return
		}
		image, err := base64.StdEncoding.DecodeString(matches[2])
		if err != nil || len(image) == 0 {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Envie uma imagem (PNG/JPEG/WebP) em base64"})
			return
		}
		if err := h.Supabase.UpsertPrivateObject(r.Context(), objectPath, "image/jpeg", image); err != nil {
			writeResourceJSON(w, http.StatusInternalServerError, map[string]string{"error": "Falha ao salvar a foto"})
			return
		}
		writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true, "path": objectPath})
	case "fotoperfil-url":
		objects, err := h.Supabase.ListPrivateObjects(r.Context(), "fotos-perfil/", 200)
		if err != nil {
			writeResourceJSON(w, http.StatusOK, map[string]any{"ok": false, "url": nil})
			return
		}
		exists := false
		for _, object := range objects {
			if object.Name == objectPath || object.Name == userID+".jpg" {
				exists = true
				break
			}
		}
		if !exists {
			writeResourceJSON(w, http.StatusOK, map[string]any{"ok": false, "url": nil})
			return
		}
		photoURL, err := h.Supabase.CreatePrivateSignedURL(r.Context(), objectPath, 7*24*60*60)
		if err != nil {
			writeResourceJSON(w, http.StatusOK, map[string]any{"ok": false, "url": nil})
			return
		}
		writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true, "url": photoURL})
	case "fotoperfil-remover":
		if err := h.Supabase.DeletePrivateObject(r.Context(), objectPath); err != nil {
			writeResourceJSON(w, http.StatusInternalServerError, map[string]string{"error": "Falha ao remover a foto"})
			return
		}
		writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

var profilePhotoDataURL = regexp.MustCompile(`^data:image/(png|jpeg|jpg|webp);base64,([A-Za-z0-9+/]*={0,2})$`)

func (h DocumentsHandler) serviceVisible(r *http.Request, caller supabase.Caller, serviceID string) bool {
	if _, err := uuid.Parse(strings.TrimSpace(serviceID)); err != nil {
		return false
	}
	query := url.Values{"id": {"eq." + serviceID}, "select": {"id"}}
	result, err := h.Supabase.UserRequest(r.Context(), caller.Token, "/rest/v1/services?"+query.Encode(), supabase.RequestOptions{Method: http.MethodGet})
	if err != nil || result.StatusCode != http.StatusOK {
		return false
	}
	var rows []map[string]any
	return json.Unmarshal(result.Body, &rows) == nil && len(rows) == 1
}

func (h DocumentsHandler) uploadServicePhoto(w http.ResponseWriter, r *http.Request, caller supabase.Caller, input documentRequest) {
	serviceID := strings.TrimSpace(input.ServiceID)
	if serviceID == "" || !h.serviceVisible(r, caller, serviceID) {
		writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Ordem de serviço não disponível para esta conta"})
		return
	}
	header, encoded, found := strings.Cut(input.Base64, ",")
	if !found {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Envie uma imagem PNG, JPEG ou WebP"})
		return
	}
	contentType, extension := "", ""
	switch header {
	case "data:image/png;base64":
		contentType, extension = "image/png", "png"
	case "data:image/jpeg;base64", "data:image/jpg;base64":
		contentType, extension = "image/jpeg", "jpg"
	case "data:image/webp;base64":
		contentType, extension = "image/webp", "webp"
	default:
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Envie uma imagem PNG, JPEG ou WebP"})
		return
	}
	if len(input.Base64) > 10_500_000 {
		writeResourceJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "Arquivo muito grande (máx. ~10MB)"})
		return
	}
	contents, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(contents) == 0 {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Imagem base64 inválida"})
		return
	}
	name := strings.TrimSpace(documentNameChars.ReplaceAllString(input.Name, "_"))
	if name == "" {
		name = "foto"
	}
	if len(name) > 80 {
		name = name[:80]
	}
	name = strings.TrimSuffix(name, path.Ext(name))
	objectPath := fmt.Sprintf("fotos-os/%s/%s-%s.%s", serviceID, uuid.NewString(), name, extension)
	if err := h.Supabase.UploadPrivateObject(r.Context(), objectPath, contentType, contents); err != nil {
		writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Falha ao enviar foto ao Storage"})
		return
	}
	writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true, "path": objectPath})
}

func (h DocumentsHandler) listServicePhotos(w http.ResponseWriter, r *http.Request, caller supabase.Caller, serviceID string) {
	serviceID = strings.TrimSpace(serviceID)
	if !h.serviceVisible(r, caller, serviceID) {
		writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Ordem de serviço não disponível para esta conta"})
		return
	}
	prefix := "fotos-os/" + serviceID
	objects, err := h.Supabase.ListPrivateObjects(r.Context(), prefix, 60)
	if err != nil {
		writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível carregar as fotos da ordem de serviço"})
		return
	}
	type photo struct {
		Name      string `json:"nome"`
		Path      string `json:"caminho"`
		URL       string `json:"url"`
		CreatedAt string `json:"criadoEm,omitempty"`
	}
	photos := make([]photo, 0, len(objects))
	for _, object := range objects {
		name := object.Name
		if strings.HasPrefix(name, prefix+"/") {
			name = strings.TrimPrefix(name, prefix+"/")
		}
		if name == "" || strings.Contains(name, "/") {
			continue
		}
		objectPath := prefix + "/" + name
		if !supabase.IsValidServicePhotoPath(objectPath) {
			continue
		}
		signedURL, err := h.Supabase.CreatePrivateSignedURL(r.Context(), objectPath, 7*24*60*60)
		if err != nil {
			continue
		}
		photos = append(photos, photo{Name: name, Path: objectPath, URL: signedURL, CreatedAt: object.CreatedAt})
	}
	writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true, "fotos": photos})
}

func (h DocumentsHandler) deleteServicePhoto(w http.ResponseWriter, r *http.Request, caller supabase.Caller, objectPath string) {
	objectPath = strings.TrimSpace(objectPath)
	if !supabase.IsValidServicePhotoPath(objectPath) {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Caminho de foto inválido"})
		return
	}
	parts := strings.Split(objectPath, "/")
	if !h.serviceVisible(r, caller, parts[1]) {
		writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Ordem de serviço não disponível para esta conta"})
		return
	}
	if err := h.Supabase.DeletePrivateObject(r.Context(), objectPath); err != nil {
		writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível excluir a foto"})
		return
	}
	writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h DocumentsHandler) applianceVisible(r *http.Request, caller supabase.Caller, applianceID string) bool {
	if _, err := uuid.Parse(strings.TrimSpace(applianceID)); err != nil {
		return false
	}
	query := url.Values{"id": {"eq." + applianceID}, "select": {"id"}}
	result, err := h.Supabase.UserRequest(r.Context(), caller.Token, "/rest/v1/air_conditioners?"+query.Encode(), supabase.RequestOptions{Method: http.MethodGet})
	if err != nil || result.StatusCode != http.StatusOK {
		return false
	}
	var rows []map[string]any
	return json.Unmarshal(result.Body, &rows) == nil && len(rows) == 1
}

func (h DocumentsHandler) uploadAppliancePhoto(w http.ResponseWriter, r *http.Request, caller supabase.Caller, input documentRequest) {
	applianceID := strings.TrimSpace(input.RefID)
	if applianceID == "" || !h.applianceVisible(r, caller, applianceID) {
		writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Aparelho não disponível para esta conta"})
		return
	}
	header, encoded, found := strings.Cut(input.Base64, ",")
	if !found {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Envie uma imagem PNG, JPEG ou WebP"})
		return
	}
	contentType, extension := "", ""
	switch header {
	case "data:image/png;base64":
		contentType, extension = "image/png", "png"
	case "data:image/jpeg;base64", "data:image/jpg;base64":
		contentType, extension = "image/jpeg", "jpg"
	case "data:image/webp;base64":
		contentType, extension = "image/webp", "webp"
	default:
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Envie uma imagem PNG, JPEG ou WebP"})
		return
	}
	if len(input.Base64) > 10_500_000 {
		writeResourceJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "Arquivo muito grande (máx. ~10MB)"})
		return
	}
	contents, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(contents) == 0 {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Imagem base64 inválida"})
		return
	}
	name := strings.TrimSpace(documentNameChars.ReplaceAllString(input.Name, "_"))
	if name == "" {
		name = "foto"
	}
	if len(name) > 80 {
		name = name[:80]
	}
	name = strings.TrimSuffix(name, path.Ext(name))
	objectPath := fmt.Sprintf("fotos-aparelho/%s/%s-%s.%s", applianceID, uuid.NewString(), name, extension)
	if err := h.Supabase.UploadPrivateObject(r.Context(), objectPath, contentType, contents); err != nil {
		writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Falha ao enviar foto ao Storage"})
		return
	}
	writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true, "path": objectPath})
}

func (h DocumentsHandler) listAppliancePhotos(w http.ResponseWriter, r *http.Request, caller supabase.Caller, applianceID string) {
	applianceID = strings.TrimSpace(applianceID)
	if !h.applianceVisible(r, caller, applianceID) {
		writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Aparelho não disponível para esta conta"})
		return
	}
	prefix := "fotos-aparelho/" + applianceID
	objects, err := h.Supabase.ListPrivateObjects(r.Context(), prefix, 100)
	if err != nil {
		writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível carregar as fotos do aparelho"})
		return
	}
	type photo struct {
		Name      string `json:"nome"`
		Path      string `json:"caminho"`
		URL       string `json:"url"`
		CreatedAt string `json:"criadoEm,omitempty"`
	}
	photos := make([]photo, 0, len(objects))
	for _, object := range objects {
		if object.Name == "" || strings.Contains(object.Name, "/") {
			continue
		}
		objectPath := prefix + "/" + object.Name
		if !supabase.IsValidAppliancePhotoPath(objectPath) {
			continue
		}
		url, err := h.Supabase.CreatePrivateSignedURL(r.Context(), objectPath, 7*24*60*60)
		if err != nil {
			continue
		}
		photos = append(photos, photo{Name: object.Name, Path: objectPath, URL: url, CreatedAt: object.CreatedAt})
	}
	writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true, "fotos": photos})
}

func (h DocumentsHandler) deleteAppliancePhoto(w http.ResponseWriter, r *http.Request, caller supabase.Caller, objectPath string) {
	objectPath = strings.TrimSpace(objectPath)
	if !supabase.IsValidAppliancePhotoPath(objectPath) {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Caminho de foto inválido"})
		return
	}
	parts := strings.Split(objectPath, "/")
	if !h.applianceVisible(r, caller, parts[1]) {
		writeResourceJSON(w, http.StatusForbidden, map[string]string{"error": "Aparelho não disponível para esta conta"})
		return
	}
	if err := h.Supabase.DeletePrivateObject(r.Context(), objectPath); err != nil {
		writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível excluir a foto"})
		return
	}
	writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h DocumentsHandler) upload(w http.ResponseWriter, r *http.Request, input documentRequest) {
	header, encoded, found := strings.Cut(input.Base64, ",")
	if !found {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Envie um PDF ou imagem em base64"})
		return
	}
	contentType, extension := "", ""
	switch header {
	case "data:application/pdf;base64":
		contentType, extension = "application/pdf", "pdf"
	case "data:image/png;base64":
		contentType, extension = "image/png", "png"
	case "data:image/jpeg;base64", "data:image/jpg;base64":
		contentType, extension = "image/jpeg", "jpg"
	case "data:image/webp;base64":
		contentType, extension = "image/webp", "webp"
	default:
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Envie um PDF ou imagem (PNG/JPEG/WebP) em base64"})
		return
	}
	if len(input.Base64) > 10_500_000 {
		writeResourceJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "Arquivo muito grande (máx. ~10MB)"})
		return
	}
	contents, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(contents) == 0 {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Arquivo base64 inválido"})
		return
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = "documento"
	}
	name = strings.TrimSpace(documentNameChars.ReplaceAllString(name, "_"))
	if len(name) > 80 {
		name = name[:80]
	}
	if ext := strings.ToLower(path.Ext(name)); ext == ".pdf" || ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".webp" {
		name = strings.TrimSuffix(name, path.Ext(name))
	}
	objectPath := fmt.Sprintf("os-orcamentos/%s/%s-%s.%s", time.Now().UTC().Format("2006-01-02"), uuid.NewString(), name, extension)
	if err := h.Supabase.UploadPrivateObject(r.Context(), objectPath, contentType, contents); err != nil {
		writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Falha ao enviar arquivo ao Storage"})
		return
	}
	writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true, "path": objectPath})
}

func (h DocumentsHandler) link(w http.ResponseWriter, r *http.Request, input documentRequest) {
	objectPath := strings.TrimSpace(input.Path)
	if !validServiceOrderDocumentPath(objectPath) {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Caminho inválido"})
		return
	}
	days := input.Days
	if days < 1 {
		days = 7
	}
	if days > 30 {
		days = 30
	}
	signedURL, err := h.Supabase.CreatePrivateSignedURL(r.Context(), objectPath, days*24*60*60)
	if err != nil {
		writeResourceJSON(w, http.StatusNotFound, map[string]string{"error": "Documento não encontrado ou expirado"})
		return
	}
	writeResourceJSON(w, http.StatusOK, map[string]any{
		"ok": true, "url": signedURL,
		"shortUrl": nil, "expiraEmDias": days,
	})
}

func validServiceOrderDocumentPath(value string) bool {
	if strings.Contains(value, "..") || !strings.HasPrefix(value, "os-orcamentos/") {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || strings.ContainsRune("_ .-/", character)) {
			return false
		}
	}
	return strings.HasSuffix(strings.ToLower(value), ".pdf")
}
