package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateExportedBundleRequiresCompleteGoShell(t *testing.T) {
	root := t.TempDir()
	files := map[string][]byte{
		"favicon.ico":             []byte("icon"),
		"index.html":              []byte(`<html><script src="/wasm_exec.js"></script><script src="/app.js"></script><link href="/web/inovar.css"><script src="/web/clean-auth-query.js"></script></html>`),
		"app.css":                 []byte("body{}"),
		"app.js":                  []byte("load web/app.wasm"),
		"wasm_exec.js":            []byte("runtime"),
		"manifest.webmanifest":    []byte("{}"),
		"app-worker.js":           []byte("worker"),
		"web/app.wasm":            {0x00, 0x61, 0x73, 0x6d},
		"web/inovar.css":          []byte("body{}"),
		"web/clean-auth-query.js": []byte("sanitize URL credentials"),
		"web/icon-192.png":        []byte("icon"),
		"web/icon-512.png":        []byte("icon"),
		"inovar-brand/INOVAR_SIGNATURE_GABRIEL.png":     []byte("signature"),
		"inovar-brand/INOVAR_SIGNATURE_GABRIEL_PDF.png": []byte("signature"),
		"downloads/index.html":                          []byte("downloads"),
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := validateExportedBundle(root); err != nil {
		t.Fatalf("valid bundle rejected: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "app.js"), []byte("react-dom"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateExportedBundle(root); err == nil {
		t.Fatal("React runtime marker accepted in Go-only frontend bundle")
	}
}

func TestCopyPublicAssetsIncludesBrandAndDownloadAssets(t *testing.T) {
	root := t.TempDir()
	output := filepath.Join(root, "dist")
	files := map[string][]byte{
		"favicon.ico": []byte("icon"),
		"inovar-brand/INOVAR_SIGNATURE_GABRIEL.png": []byte("signature"),
		"downloads/index.html":                      []byte("downloads"),
		"downloads/InovarApp-Android.apk":           []byte("apk"),
	}
	for name, content := range files {
		path := filepath.Join(root, "public", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := copyPublicAssets(root, output); err != nil {
		t.Fatalf("copy public assets: %v", err)
	}
	for name, want := range files {
		got, err := os.ReadFile(filepath.Join(output, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("read copied asset %s: %v", name, err)
		}
		if string(got) != string(want) {
			t.Errorf("asset %s = %q, want %q", name, got, want)
		}
	}
}

func TestWasmBuildEnvironmentOverridesExistingTargetValues(t *testing.T) {
	environment := wasmBuildEnvironment([]string{"PATH=bin", "GOOS=windows", "GOARCH=amd64", "OTHER=value"})
	var goos, goarch int
	for _, entry := range environment {
		if entry == "GOOS=js" {
			goos++
		}
		if entry == "GOARCH=wasm" {
			goarch++
		}
		if entry == "GOOS=windows" || entry == "GOARCH=amd64" {
			t.Fatalf("stale target environment retained: %q", entry)
		}
	}
	if goos != 1 || goarch != 1 {
		t.Fatalf("expected one js/wasm target, got environment %#v", environment)
	}
}
