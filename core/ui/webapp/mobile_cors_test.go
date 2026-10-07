package webapp

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMobileCORSAllowsCapacitorOriginsOnly(t *testing.T) {
	called := false
	handler := mobileCORSMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called = true; w.WriteHeader(http.StatusOK) }))
	for _, test := range []struct {
		origin     string
		method     string
		wantStatus int
		wantOrigin string
	}{
		{"https://localhost", http.MethodOptions, http.StatusNoContent, "https://localhost"},
		{"capacitor://localhost", http.MethodGet, http.StatusOK, "capacitor://localhost"},
		{"https://evil.example", http.MethodOptions, http.StatusOK, ""},
	} {
		called = false
		request := httptest.NewRequest(test.method, "/api/servicos", nil)
		request.Header.Set("Origin", test.origin)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.wantStatus || response.Header().Get("Access-Control-Allow-Origin") != test.wantOrigin {
			t.Errorf("origin=%s status=%d allow=%q", test.origin, response.Code, response.Header().Get("Access-Control-Allow-Origin"))
		}
		if test.wantOrigin != "" && response.Header().Get("Access-Control-Expose-Headers") != "Content-Disposition, X-Service-Order-File, X-Service-Order-URL, X-Service-Order-WhatsApp" {
			t.Errorf("origin=%s exposed headers=%q", test.origin, response.Header().Get("Access-Control-Expose-Headers"))
		}
		if test.method == http.MethodOptions && test.origin == "https://localhost" && called {
			t.Fatal("preflight should end before API handler")
		}
	}
}
