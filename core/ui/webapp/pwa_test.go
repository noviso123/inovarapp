package webapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoAppPWAProvidesLocalManifestIconsAndOfflineShell(t *testing.T) {
	registerAppRoutes()
	handler := newAppHandler()
	manifestRequest := httptest.NewRequest(http.MethodGet, "/manifest.webmanifest", nil)
	manifestResponse := httptest.NewRecorder()
	handler.ServeHTTP(manifestResponse, manifestRequest)
	if manifestResponse.Code != http.StatusOK || !strings.Contains(manifestResponse.Header().Get("Content-Type"), "manifest+json") {
		t.Fatalf("manifest status=%d type=%q", manifestResponse.Code, manifestResponse.Header().Get("Content-Type"))
	}
	var manifest struct {
		Name      string `json:"name"`
		ShortName string `json:"short_name"`
		Display   string `json:"display"`
		StartURL  string `json:"start_url"`
		Theme     string `json:"theme_color"`
		Icons     []struct {
			Src     string `json:"src"`
			Purpose string `json:"purpose"`
		} `json:"icons"`
	}
	if err := json.Unmarshal(manifestResponse.Body.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Name != "InovarApp • Inovar Refrigeração" || manifest.ShortName != "InovarApp" || manifest.Display != "standalone" || manifest.StartURL != "/" || manifest.Theme != "#0B2D4E" {
		t.Fatalf("PWA manifest=%#v", manifest)
	}
	for _, want := range []string{"/web/icon-192.png", "/web/icon-512.png", "/web/favicon.svg"} {
		found := false
		for _, icon := range manifest.Icons {
			if icon.Src == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("manifest missing local icon %q: %#v", want, manifest.Icons)
		}
	}
	for _, path := range []string{"/web/icon-192.png", "/web/icon-512.png", "/web/favicon.svg", "/web/apple-touch-icon.png", "/web/inovar.css"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Errorf("PWA asset %s status=%d bytes=%d", path, response.Code, response.Body.Len())
		}
	}
	workerRequest := httptest.NewRequest(http.MethodGet, "/app-worker.js", nil)
	workerResponse := httptest.NewRecorder()
	handler.ServeHTTP(workerResponse, workerRequest)
	worker := workerResponse.Body.String()
	for _, resource := range []string{"/web/app.wasm", "/web/inovar.css", "/web/clean-auth-query.js", "/web/icon-192.png", "/web/icon-512.png", "/web/apple-touch-icon.png", "/web/favicon.svg"} {
		if !strings.Contains(worker, resource) {
			t.Errorf("offline worker does not precache %s", resource)
		}
	}
	if strings.Contains(worker, "/api/") || strings.Contains(worker, "cache.put(") {
		t.Fatal("offline worker must not persist API responses or customer data")
	}
	rootRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	rootResponse := httptest.NewRecorder()
	handler.ServeHTTP(rootResponse, rootRequest)
	if rootResponse.Code != http.StatusOK || !strings.Contains(rootResponse.Body.String(), `/manifest.webmanifest`) || !strings.Contains(rootResponse.Body.String(), `/app.js`) {
		t.Fatalf("Go PWA shell status=%d body missing runtime elements", rootResponse.Code)
	}
}

func TestPWAResourceHandlerCannotReadOutsideWebDirectory(t *testing.T) {
	root := t.TempDir()
	webRoot := filepath.Join(root, "web")
	if err := os.MkdirAll(webRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("PRIVATE_TEST_SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webRoot, "inovar.css"), []byte("body{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := newPWAResources(root)

	for _, path := range []string{"/web/inovar.css", "/web/../.env", "/web/%2e%2e/.env"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if strings.Contains(response.Body.String(), "PRIVATE_TEST_SECRET") {
			t.Fatalf("resource handler exposed a file outside web directory via %s", path)
		}
		if path == "/web/inovar.css" && (response.Code != http.StatusOK || response.Body.String() != "body{}") {
			t.Fatalf("web asset status=%d body=%q", response.Code, response.Body.String())
		}
	}
}

func TestPackagedPWAAssetsFallbackOutsideWorkspace(t *testing.T) {
	handler := newPWAResources(t.TempDir())
	for _, path := range []string{"/web/inovar.css", "/web/icon-192.png"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Errorf("embedded asset %s status=%d bytes=%d", path, response.Code, response.Body.Len())
		}
	}
}

func TestServerListenAddressSupportsAssignedPort(t *testing.T) {
	t.Setenv("PORT", "8787")
	if got := serverListenAddress(); got != ":8787" {
		t.Fatalf("listen address=%q", got)
	}
	t.Setenv("PORT", ":9123")
	if got := serverListenAddress(); got != ":9123" {
		t.Fatalf("listen address=%q", got)
	}
}

func TestFrontendVersionChangesWhenBundleChanges(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "web")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	wasm := filepath.Join(directory, "app.wasm")
	if err := os.WriteFile(wasm, []byte("first bundle"), 0o600); err != nil {
		t.Fatal(err)
	}
	first := frontendAssetVersion(root, "same-release-label")
	if first != frontendAssetVersion(root, "same-release-label") {
		t.Fatal("version must be stable for identical assets")
	}
	if err := os.WriteFile(wasm, []byte("updated bundle"), 0o600); err != nil {
		t.Fatal(err)
	}
	if first == frontendAssetVersion(root, "same-release-label") {
		t.Fatal("changed WASM must invalidate the offline cache")
	}
}
