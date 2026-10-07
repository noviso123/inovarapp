package webapp

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseServerAddress(t *testing.T) {
	for _, test := range []struct{ port, want string }{
		{"", ":8080"}, {"8081", ":8081"}, {":9090", ":9090"},
	} {
		if got := ParseServerAddress(test.port); got != test.want {
			t.Errorf("ParseServerAddress(%q) = %q, want %q", test.port, got, test.want)
		}
	}
}

func TestDesktopAssetProxyKeepsAPIOnTheSharedGoServer(t *testing.T) {
	baseURL, shutdown, err := StartLocalServer()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = shutdown() }()

	proxy := LocalHandler(baseURL)
	recorder := httptest.NewRecorder()
	proxy.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/whatsapp", nil))
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("proxied API status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	if !strings.HasPrefix(recorder.Header().Get("Content-Type"), "application/json") || !strings.Contains(recorder.Body.String(), "Método não permitido") {
		t.Fatalf("desktop proxy did not reach JSON Go API handler: headers=%v body=%q", recorder.Header(), recorder.Body.String())
	}
}

func TestDesktopLocalServerServesGoUIAndAssets(t *testing.T) {
	baseURL, shutdown, err := StartLocalServer()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = shutdown() }()

	for _, path := range []string{"/", "/web/inovar.css"} {
		response, err := http.Get(baseURL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil {
			t.Fatalf("read %s: %v", path, readErr)
		}
		if response.StatusCode != http.StatusOK || len(body) == 0 {
			t.Errorf("GET %s status=%d bytes=%d", path, response.StatusCode, len(body))
		}
	}
}
