package webapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportStaticCreatesGoAppShellAndWASM(t *testing.T) {
	if _, err := os.Stat(filepath.Join(webRoot(), "web", "app.wasm")); err != nil {
		t.Skip("compile the Go WebAssembly app before testing static export")
	}
	output := t.TempDir()
	t.Setenv("GOAPP_API_BASE_URL", "https://api.example.com")
	if err := ExportStatic(output); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"index.html", "app.css", "app.js", "wasm_exec.js", "manifest.webmanifest", "app-worker.js", "web/app.wasm", "web/inovar.css", "web/clean-auth-query.js"} {
		data, err := os.ReadFile(filepath.Join(output, filepath.FromSlash(path)))
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("exported file %s is empty", path)
		}
	}
	index, err := os.ReadFile(filepath.Join(output, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range []string{"/app.css", "/app.js", "/wasm_exec.js", "/manifest.webmanifest", "/web/inovar.css", "/web/clean-auth-query.js"} {
		if !strings.Contains(string(index), resource) {
			t.Errorf("exported HTML does not reference expected resource %s", resource)
		}
	}
	runtime, err := os.ReadFile(filepath.Join(output, "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(runtime), "https://api.example.com") {
		t.Error("public API base URL was not injected into the app runtime")
	}
	if !strings.Contains(string(runtime), "/web/app.wasm") {
		t.Error("Go runtime does not request the exported WASM bundle")
	}
}
