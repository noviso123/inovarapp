package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"inovarapp/core/adapter/supabase"
)

func TestDocumentsUploadAndLinkKeepPDFPrivateAndTeamOnly(t *testing.T) {
	pdfBytes := []byte("%PDF-1.7 fixture")
	var uploadedPath string
	var signedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/storage/v1/object/documentos-inovar/os-orcamentos/") {
			if r.Header.Get("Authorization") != "Bearer server-secret" || r.Header.Get("apikey") != "server-secret" || r.Header.Get("Content-Type") != "application/pdf" || r.Header.Get("x-upsert") != "false" {
				t.Errorf("unexpected upload headers: %v", r.Header)
			}
			body, _ := io.ReadAll(r.Body)
			if string(body) != string(pdfBytes) {
				t.Errorf("uploaded bytes=%q", body)
			}
			uploadedPath = strings.TrimPrefix(r.URL.Path, "/storage/v1/object/documentos-inovar/")
			w.WriteHeader(http.StatusOK)
			return
		}
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"user-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"TECNICO"}]`)
		case "/storage/v1/object/sign/documentos-inovar/" + uploadedPath:
			var body map[string]int
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["expiresIn"] != 30*24*60*60 {
				t.Errorf("expiresIn=%d", body["expiresIn"])
			}
			signedPath = r.URL.Path
			_, _ = io.WriteString(w, `{"signedURL":"/object/sign/documentos-inovar/example.pdf?token=signed"}`)
		default:
			t.Errorf("unexpected request: %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	handler := DocumentsHandler{Supabase: accountTestClient(t, server)}

	upload := httptest.NewRequest(http.MethodPost, "/api/documentos", strings.NewReader(`{"acao":"upload","nome":"OS João.pdf","base64":"data:application/pdf;base64,`+base64.StdEncoding.EncodeToString(pdfBytes)+`"}`))
	upload.Header.Set("Authorization", "Bearer user-token")
	uploadResponse := httptest.NewRecorder()
	handler.ServeHTTP(uploadResponse, upload)
	if uploadResponse.Code != http.StatusOK || uploadedPath == "" || !strings.HasSuffix(uploadedPath, "OS Jo_o.pdf") {
		t.Fatalf("upload status=%d path=%q body=%s", uploadResponse.Code, uploadedPath, uploadResponse.Body.String())
	}

	link := httptest.NewRequest(http.MethodPost, "/api/documentos", strings.NewReader(`{"acao":"link","path":"`+uploadedPath+`","dias":90}`))
	link.Header.Set("Authorization", "Bearer user-token")
	linkResponse := httptest.NewRecorder()
	handler.ServeHTTP(linkResponse, link)
	var linked map[string]any
	if err := json.Unmarshal(linkResponse.Body.Bytes(), &linked); err != nil {
		t.Fatal(err)
	}
	if linkResponse.Code != http.StatusOK || signedPath == "" || linked["url"] != server.URL+"/storage/v1/object/sign/documentos-inovar/example.pdf?token=signed" || linked["expiraEmDias"] != float64(30) {
		t.Fatalf("link status=%d path=%q result=%#v", linkResponse.Code, signedPath, linked)
	}
}

func TestCustomerCanUploadSignAndRemoveOnlyTheirProfilePhoto(t *testing.T) {
	const userID = "11111111-1111-4111-8111-111111111111"
	const objectPath = "fotos-perfil/" + userID + ".jpg"
	imageBytes := []byte{0xff, 0xd8, 0xff, 0x00, 0x01}
	var actions []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"`+userID+`"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"`+userID+`","tipo":"CLIENTE"}]`)
		case "/storage/v1/object/documentos-inovar/" + objectPath:
			if r.Header.Get("Authorization") != "Bearer server-secret" || r.Header.Get("apikey") != "server-secret" {
				t.Errorf("profile photo storage request did not use server credentials: %v", r.Header)
			}
			switch r.Method {
			case http.MethodPost:
				if r.Header.Get("x-upsert") != "true" || r.Header.Get("Content-Type") != "image/jpeg" {
					t.Errorf("upload headers upsert=%q content-type=%q", r.Header.Get("x-upsert"), r.Header.Get("Content-Type"))
				}
				body, _ := io.ReadAll(r.Body)
				if string(body) != string(imageBytes) {
					t.Errorf("uploaded bytes=%v", body)
				}
				actions = append(actions, "upload")
				w.WriteHeader(http.StatusOK)
			case http.MethodDelete:
				actions = append(actions, "remove")
				w.WriteHeader(http.StatusNotFound)
			default:
				t.Errorf("unexpected storage method %s", r.Method)
				http.Error(w, "unexpected", http.StatusMethodNotAllowed)
			}
		case "/storage/v1/object/list/documentos-inovar":
			var body struct {
				Prefix string `json:"prefix"`
				Limit  int    `json:"limit"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Prefix != "fotos-perfil/" || body.Limit != 200 {
				t.Errorf("list request=%#v err=%v", body, err)
			}
			actions = append(actions, "list")
			_, _ = io.WriteString(w, `[{"name":"`+objectPath+`"}]`)
		case "/storage/v1/object/sign/documentos-inovar/" + objectPath:
			var body struct {
				ExpiresIn int `json:"expiresIn"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ExpiresIn != 604800 {
				t.Errorf("signed URL request=%#v err=%v", body, err)
			}
			actions = append(actions, "sign")
			_, _ = io.WriteString(w, `{"signedURL":"/object/sign/documentos-inovar/`+objectPath+`?token=signed"}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	h := DocumentsHandler{Supabase: accountTestClient(t, server)}
	call := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/api/documentos", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer customer-token")
		response := httptest.NewRecorder()
		h.ServeHTTP(response, request)
		return response
	}

	upload := call(`{"acao":"fotoperfil","base64":"data:image/png;base64,` + base64.StdEncoding.EncodeToString(imageBytes) + `"}`)
	if upload.Code != http.StatusOK || !strings.Contains(upload.Body.String(), `"ok":true`) {
		t.Fatalf("upload response=%d %s", upload.Code, upload.Body.String())
	}
	photoURL := call(`{"acao":"fotoperfil-url"}`)
	if photoURL.Code != http.StatusOK || !strings.Contains(photoURL.Body.String(), "/storage/v1/object/sign/documentos-inovar/"+objectPath) {
		t.Fatalf("photo URL response=%d %s", photoURL.Code, photoURL.Body.String())
	}
	removed := call(`{"acao":"fotoperfil-remover"}`)
	if removed.Code != http.StatusOK || !strings.Contains(removed.Body.String(), `"ok":true`) {
		t.Fatalf("remove response=%d %s", removed.Code, removed.Body.String())
	}
	if strings.Join(actions, ",") != "upload,list,sign,remove" {
		t.Fatalf("storage actions = %v", actions)
	}
}

func TestDocumentsRejectCustomerAndUnsafeObjectPaths(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"user-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"user-1","tipo":"CLIENTE"}]`)
		default:
			t.Errorf("unexpected request: %s", r.URL.String())
		}
	}))
	defer server.Close()
	handler := DocumentsHandler{Supabase: accountTestClient(t, server)}
	request := httptest.NewRequest(http.MethodPost, "/api/documentos", strings.NewReader(`{"acao":"link","path":"os-orcamentos/../../private.pdf"}`))
	request.Header.Set("Authorization", "Bearer user-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("customer upload/link status=%d body=%s", response.Code, response.Body.String())
	}
	if validServiceOrderDocumentPath("os-orcamentos/2026/private.png") || validServiceOrderDocumentPath("fotos-os/a.pdf") || !validServiceOrderDocumentPath("os-orcamentos/2026/document.pdf") {
		t.Fatal("document path allowlist is incorrect")
	}
}
func TestAppliancePhotoListUploadAndDeleteStayInApplianceScope(t *testing.T) {
	applianceID := "11111111-1111-4111-8111-111111111111"
	var uploadedPath, deletedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		objectPrefix := "/storage/v1/object/documentos-inovar/fotos-aparelho/" + applianceID + "/"
		if strings.HasPrefix(r.URL.Path, objectPrefix) && (r.Method == http.MethodPost || r.Method == http.MethodDelete) {
			if r.Method == http.MethodPost {
				uploadedPath = strings.TrimPrefix(r.URL.Path, "/storage/v1/object/documentos-inovar/")
				body, _ := io.ReadAll(r.Body)
				if string(body) != "image bytes" {
					t.Errorf("upload body=%q", body)
				}
			} else {
				deletedPath = strings.TrimPrefix(r.URL.Path, "/storage/v1/object/documentos-inovar/")
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"staff-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"staff-1","tipo":"TECNICO"}]`)
		case "/rest/v1/air_conditioners":
			if r.URL.Query().Get("id") != "eq."+applianceID || r.Header.Get("Authorization") != "Bearer staff-token" {
				t.Errorf("appliance check query=%s auth=%q", r.URL.RawQuery, r.Header.Get("Authorization"))
			}
			_, _ = io.WriteString(w, `[{"id":"`+applianceID+`"}]`)
		case "/storage/v1/object/list/documentos-inovar":
			if r.Header.Get("Authorization") != "Bearer server-secret" {
				t.Errorf("list missing server role")
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["prefix"] != "fotos-aparelho/"+applianceID || body["limit"] != float64(100) {
				t.Errorf("list request=%#v", body)
			}
			_, _ = io.WriteString(w, `[{"name":"foto.jpg","created_at":"2026-10-03T12:00:00Z"},{"name":"pasta/foto.png"}]`)
		case "/storage/v1/object/sign/documentos-inovar/fotos-aparelho/" + applianceID + "/foto.jpg":
			_, _ = io.WriteString(w, `{"signedURL":"/object/sign/documentos-inovar/foto.jpg?token=photo"}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	h := DocumentsHandler{Supabase: accountTestClient(t, server)}
	call := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/documentos", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer staff-token")
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		return res
	}
	listed := call(`{"acao":"fotos-aparelho","aparelhoId":"` + applianceID + `"}`)
	var result struct {
		Photos []struct {
			Name string `json:"nome"`
			Path string `json:"caminho"`
			URL  string `json:"url"`
		} `json:"fotos"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if listed.Code != http.StatusOK || len(result.Photos) != 1 || result.Photos[0].Path != "fotos-aparelho/"+applianceID+"/foto.jpg" || !strings.Contains(result.Photos[0].URL, "token=photo") {
		t.Fatalf("list status=%d photos=%#v body=%s", listed.Code, result.Photos, listed.Body.String())
	}
	uploaded := call(`{"acao":"upload","pasta":"aparelho","refId":"` + applianceID + `","nome":"fachada.jpg","base64":"data:image/jpeg;base64,` + base64.StdEncoding.EncodeToString([]byte("image bytes")) + `"}`)
	if uploaded.Code != http.StatusOK || uploadedPath == "" || !strings.HasPrefix(uploadedPath, "fotos-aparelho/"+applianceID+"/") {
		t.Fatalf("upload status=%d path=%q body=%s", uploaded.Code, uploadedPath, uploaded.Body.String())
	}
	deleted := call(`{"acao":"excluirfoto","path":"fotos-aparelho/` + applianceID + `/foto.jpg"}`)
	if deleted.Code != http.StatusOK || deletedPath != "fotos-aparelho/"+applianceID+"/foto.jpg" {
		t.Fatalf("delete status=%d path=%q body=%s", deleted.Code, deletedPath, deleted.Body.String())
	}
}

func TestAppliancePhotoPathsRejectCrossFolderAndTraversal(t *testing.T) {
	for _, path := range []string{"fotos-aparelho/not-a-uuid/file.jpg", "fotos-aparelho/11111111-1111-4111-8111-111111111111/../secret.jpg", "fotos-os/11111111-1111-4111-8111-111111111111/photo.jpg"} {
		if supabase.IsValidAppliancePhotoPath(path) {
			t.Errorf("accepted unsafe path %q", path)
		}
	}
}

func TestServicePhotoListUploadAndDeleteRemainScopedToVisibleService(t *testing.T) {
	serviceID := "22222222-2222-4222-8222-222222222222"
	var uploadedPath, deletedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		objectPrefix := "/storage/v1/object/documentos-inovar/fotos-os/" + serviceID + "/"
		if strings.HasPrefix(r.URL.Path, objectPrefix) && (r.Method == http.MethodPost || r.Method == http.MethodDelete) {
			if r.Method == http.MethodPost {
				uploadedPath = strings.TrimPrefix(r.URL.Path, "/storage/v1/object/documentos-inovar/")
				body, _ := io.ReadAll(r.Body)
				if string(body) != "service image bytes" {
					t.Errorf("upload body=%q", body)
				}
			} else {
				deletedPath = strings.TrimPrefix(r.URL.Path, "/storage/v1/object/documentos-inovar/")
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"staff-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"staff-1","tipo":"TECNICO"}]`)
		case "/rest/v1/services":
			if r.URL.Query().Get("id") != "eq."+serviceID || r.Header.Get("Authorization") != "Bearer staff-token" {
				t.Errorf("service check query=%s auth=%q", r.URL.RawQuery, r.Header.Get("Authorization"))
			}
			_, _ = io.WriteString(w, `[{"id":"`+serviceID+`"}]`)
		case "/storage/v1/object/list/documentos-inovar":
			if r.Header.Get("Authorization") != "Bearer server-secret" {
				t.Errorf("list missing service role")
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["prefix"] != "fotos-os/"+serviceID || body["limit"] != float64(60) {
				t.Errorf("list request=%#v", body)
			}
			_, _ = io.WriteString(w, `[{"name":"foto.jpg","created_at":"2026-10-04T12:00:00Z"},{"name":"subdir/hidden.png"}]`)
		case "/storage/v1/object/sign/documentos-inovar/fotos-os/" + serviceID + "/foto.jpg":
			_, _ = io.WriteString(w, `{"signedURL":"/object/sign/documentos-inovar/foto.jpg?token=service-photo"}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	h := DocumentsHandler{Supabase: accountTestClient(t, server)}
	call := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/documentos", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer staff-token")
		res := httptest.NewRecorder()
		h.ServeHTTP(res, req)
		return res
	}
	listed := call(`{"acao":"fotos","serviceId":"` + serviceID + `"}`)
	var result struct {
		Photos []struct {
			Name string `json:"nome"`
			Path string `json:"caminho"`
			URL  string `json:"url"`
		} `json:"fotos"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if listed.Code != http.StatusOK || len(result.Photos) != 1 || result.Photos[0].Path != "fotos-os/"+serviceID+"/foto.jpg" || !strings.Contains(result.Photos[0].URL, "token=service-photo") {
		t.Fatalf("list status=%d photos=%#v body=%s", listed.Code, result.Photos, listed.Body.String())
	}

	uploaded := call(`{"acao":"upload","pasta":"fotos","serviceId":"` + serviceID + `","nome":"cliente 📷.jpg","base64":"data:image/jpeg;base64,` + base64.StdEncoding.EncodeToString([]byte("service image bytes")) + `"}`)
	if uploaded.Code != http.StatusOK || !strings.HasPrefix(uploadedPath, "fotos-os/"+serviceID+"/") || !strings.HasSuffix(uploadedPath, ".jpg") {
		t.Fatalf("upload status=%d path=%q body=%s", uploaded.Code, uploadedPath, uploaded.Body.String())
	}

	deleted := call(`{"acao":"excluirfoto","path":"fotos-os/` + serviceID + `/foto.jpg"}`)
	if deleted.Code != http.StatusOK || deletedPath != "fotos-os/"+serviceID+"/foto.jpg" {
		t.Fatalf("delete status=%d path=%q body=%s", deleted.Code, deletedPath, deleted.Body.String())
	}
}

func TestServicePhotoDeleteRejectsPathTraversalAndWrongOwner(t *testing.T) {
	serviceID := "33333333-3333-4333-8333-333333333333"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/user":
			_, _ = io.WriteString(w, `{"id":"staff-1"}`)
		case "/rest/v1/profiles":
			_, _ = io.WriteString(w, `[{"id":"staff-1","tipo":"TECNICO"}]`)
		case "/rest/v1/services":
			_, _ = io.WriteString(w, `[]`)
		default:
			t.Errorf("unexpected request %s", r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	h := DocumentsHandler{Supabase: accountTestClient(t, server)}
	for _, test := range []struct {
		path string
		want int
	}{
		{"fotos-os/" + serviceID + "/../private.jpg", http.StatusBadRequest},
		{"fotos-os/" + serviceID + "/other.jpg", http.StatusForbidden},
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/documentos", strings.NewReader(`{"acao":"excluirfoto","path":"`+test.path+`"}`))
		request.Header.Set("Authorization", "Bearer staff-token")
		response := httptest.NewRecorder()
		h.ServeHTTP(response, request)
		if response.Code != test.want {
			t.Errorf("path %q: status=%d want=%d body=%s", test.path, response.Code, test.want, response.Body.String())
		}
	}
}
