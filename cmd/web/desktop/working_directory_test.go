//go:build !bindings

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDesktopWorkingDirectoryFindsProjectRootForPackagedBuild(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "web"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "web", "inovar.css"), []byte("body{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	executableDir := filepath.Join(root, "dist", "desktop", "bin")
	if err := os.MkdirAll(executableDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := desktopWorkingDirectory(executableDir); got != root {
		t.Fatalf("desktopWorkingDirectory=%q want %q", got, root)
	}
}

func TestDesktopWorkingDirectoryFallsBackToExecutableDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if got := desktopWorkingDirectory(dir); got != dir {
		t.Fatalf("desktopWorkingDirectory=%q want %q", got, dir)
	}
}
