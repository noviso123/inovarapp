//go:build bindings

package main

// Binding generation must stay in the Wails project directory so the CLI can
// read wails.json after it executes this temporary binary from the OS temp dir.
func setDesktopWorkingDirectory() error { return nil }
