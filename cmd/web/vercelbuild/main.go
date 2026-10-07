// Command vercelbuild creates the shared Go/WASM frontend bundle for Vercel's
// static output directory. The existing Vercel API functions remain separate
// serverless handlers and are routed through api/go.go.
package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"inovarapp/core/ui/webapp"
)

func main() {
	root, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	wasmPath := filepath.Join(root, "web", "app.wasm")
	// Do not let a previous local or cached build shadow the current embedded
	// CSS and PWA assets during ExportStatic. Only app.wasm is generated here.
	if err := os.RemoveAll(filepath.Dir(wasmPath)); err != nil {
		log.Fatalf("clear stale web build assets: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(wasmPath), 0o755); err != nil {
		log.Fatal(err)
	}
	// The builder may receive an incomplete Git checkout. Vercel records the
	// release commit separately; Go must not require Git metadata to compile.
	// Go's content-addressed cache also tracks embedded assets, so avoid -a.
	command := exec.Command("go", "build", "-buildvcs=false", "-trimpath", "-ldflags=-s -w", "-o", wasmPath, "./cmd/web")
	command.Dir = root
	command.Env = wasmBuildEnvironment(os.Environ())
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		log.Fatalf("compile Go frontend to WebAssembly: %v", err)
	}
	if err := copyPackagedWebAssets(root, wasmPath); err != nil {
		log.Fatalf("prepare current PWA assets: %v", err)
	}

	output := os.Getenv("VERCEL_OUTPUT_DIR")
	if output == "" {
		output = filepath.Join(root, "dist-go")
	}
	if err := webapp.ExportStatic(output); err != nil {
		log.Fatalf("export Go frontend for Vercel: %v", err)
	}
	if err := copyPublicAssets(root, output); err != nil {
		log.Fatalf("copy public assets: %v", err)
	}
	if err := validateExportedBundle(output); err != nil {
		log.Fatalf("validate Go frontend bundle: %v", err)
	}
}

func copyPackagedWebAssets(root, wasmPath string) error {
	source := filepath.Join(root, "core", "ui", "webapp", "static", "web")
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Clean(path) == filepath.Clean(filepath.Join(source, "app.wasm")) {
			return nil
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(filepath.Dir(wasmPath), relative)
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, data, 0o644)
	})
}

func copyPublicAssets(root, output string) error {
	source := filepath.Join(root, "public")
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(output, relative)
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, data, 0o644)
	})
}

func wasmBuildEnvironment(current []string) []string {
	environment := make([]string, 0, len(current)+2)
	for _, entry := range current {
		key, _, _ := strings.Cut(entry, "=")
		if key != "GOOS" && key != "GOARCH" {
			environment = append(environment, entry)
		}
	}
	return append(environment, "GOOS=js", "GOARCH=wasm")
}

func validateExportedBundle(output string) error {
	for _, name := range []string{
		"index.html", "app.css", "app.js", "wasm_exec.js", "manifest.webmanifest", "app-worker.js",
		"web/app.wasm", "web/inovar.css", "web/clean-auth-query.js", "web/icon-192.png", "web/icon-512.png", "favicon.ico",
		"inovar-brand/INOVAR_SIGNATURE_GABRIEL.png", "inovar-brand/INOVAR_SIGNATURE_GABRIEL_PDF.png",
		"downloads/index.html",
	} {
		info, err := os.Stat(filepath.Join(output, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		if info.Size() == 0 {
			return fmt.Errorf("exported asset %s is empty", name)
		}
	}
	index, err := os.ReadFile(filepath.Join(output, "index.html"))
	if err != nil {
		return err
	}
	for _, reference := range []string{"/app.js", "/wasm_exec.js", "/web/inovar.css", "/web/clean-auth-query.js"} {
		if !bytes.Contains(index, []byte(reference)) {
			return fmt.Errorf("Go shell does not reference %s", reference)
		}
	}
	runtime, err := os.ReadFile(filepath.Join(output, "app.js"))
	if err != nil {
		return err
	}
	if !bytes.Contains(runtime, []byte("web/app.wasm")) || strings.Contains(strings.ToLower(string(runtime)), "react-dom") {
		return fmt.Errorf("runtime does not load the Go WASM bundle or contains a React runtime")
	}
	wasm, err := os.ReadFile(filepath.Join(output, "web", "app.wasm"))
	if err != nil {
		return err
	}
	if len(wasm) < 4 || !bytes.Equal(wasm[:4], []byte{0x00, 0x61, 0x73, 0x6d}) {
		return fmt.Errorf("web/app.wasm is not a WebAssembly module")
	}
	return nil
}

