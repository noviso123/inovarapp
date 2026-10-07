package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestVercelConfigurationShipsGoWASMAndRoutesGoAPIs(t *testing.T) {
	data, err := os.ReadFile("../vercel.json")
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		BuildCommand    string `json:"buildCommand"`
		OutputDirectory string `json:"outputDirectory"`
		Rewrites        []struct {
			Source      string `json:"source"`
			Destination string `json:"destination"`
		} `json:"rewrites"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if config.BuildCommand != "npm run build" || config.OutputDirectory != "dist-go" {
		t.Fatalf("Vercel must run the production build into dist-go, got command=%q output=%q", config.BuildCommand, config.OutputDirectory)
	}
	buildScript, err := os.ReadFile("../scripts/vercel-build.mjs")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(buildScript), "./cmd/web/vercelbuild") {
		t.Fatalf("production build script must compile the Go/WASM frontend: %s", buildScript)
	}
	var routesFrontend, routesAPI bool
	for _, rewrite := range config.Rewrites {
		if strings.HasPrefix(rewrite.Destination, "/api/go?") && rewrite.Source == "/d/:code" {
			routesFrontend = true
		}
		if strings.HasPrefix(rewrite.Destination, "/api/go?") && rewrite.Source == "/api/:path*" {
			routesAPI = true
		}
	}
	if !routesFrontend || !routesAPI {
		t.Fatalf("Vercel rewrite must route short docs and all APIs through Go: %#v", config.Rewrites)
	}
}

func TestVercelUploadExcludesLegacyNodeFunctionsThatShadowGoRewrites(t *testing.T) {
	data, err := os.ReadFile("../.vercelignore")
	if err != nil {
		t.Fatal(err)
	}
	ignore := string(data)
	for _, pattern := range []string{"/api/*.js", "/api/**/*.js"} {
		if !strings.Contains(ignore, pattern) {
			t.Fatalf("Vercel upload must exclude legacy Node functions using %q; rewrites otherwise lose to filesystem routes", pattern)
		}
	}
	if _, err := os.Stat("go.go"); err != nil {
		t.Fatalf("Go dispatcher must remain in api/: %v", err)
	}
}

func TestVercelGoDispatcherPreservesMobileCORSAndAPIPath(t *testing.T) {
	request := httptest.NewRequest(http.MethodOptions, "/api/go?__go_path=%2Fapi%2Fservicos", nil)
	request.Header.Set("Origin", "https://localhost")
	response := httptest.NewRecorder()
	Handler(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("preflight status=%d body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Access-Control-Allow-Origin") != "https://localhost" {
		t.Fatalf("mobile CORS header=%q", response.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestVercelGoDispatcherMobilePreflightProbeReachesGoAPI(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/go?__go_path=%2Fapi%2Fwhatsapp", nil)
	response := httptest.NewRecorder()
	Handler(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /api/whatsapp status=%d body=%s, want handler method rejection", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("GET /api/whatsapp content type=%q, want JSON from the Go handler", contentType)
	}
}

func TestVercelGoDispatcherRejectsPathsOutsideAPIRoot(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/go?__go_path=%2Findex.html", nil)
	response := httptest.NewRecorder()
	Handler(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("path traversal/escape status=%d", response.Code)
	}
}
