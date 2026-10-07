//go:build !bindings

package main

import (
	"os"
	"path/filepath"
)

func setDesktopWorkingDirectory() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return os.Chdir(desktopWorkingDirectory(filepath.Dir(executable)))
}

func desktopWorkingDirectory(executableDir string) string {
	for current := filepath.Clean(executableDir); ; current = filepath.Dir(current) {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(current, "web", "inovar.css")); err == nil {
				return current
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			return executableDir
		}
	}
}
