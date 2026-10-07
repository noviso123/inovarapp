package webapp

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
)

var mobileStaticRoutes = []string{
	"/", "/app.css", "/app.js", "/wasm_exec.js", "/manifest.webmanifest", "/app-worker.js",
	"/web/app.wasm", "/web/inovar.css", "/web/clean-auth-query.js", "/web/favicon.svg",
	"/web/icon-192.png", "/web/icon-512.png", "/web/apple-touch-icon.png",
}

// ExportStatic materializes the Go UI handler for native WebViews such as
// Capacitor. Public settings are injected by newAppHandler; private keys are
// never part of this static export.
func ExportStatic(outputDirectory string) error {
	registerAppRoutes()
	handler := newAppHandler()
	for _, route := range mobileStaticRoutes {
		request := httptest.NewRequest(http.MethodGet, route, nil)
		request.Host = "localhost"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			return fmt.Errorf("export route %s: HTTP %d", route, response.Code)
		}
		name := "index.html"
		if route != "/" {
			name = strings.TrimPrefix(route, "/")
		}
		destination := filepath.Join(outputDirectory, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(destination, response.Body.Bytes(), 0o644); err != nil {
			return err
		}
	}
	return nil
}
