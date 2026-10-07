package main

import (
	"encoding/base64"
	"testing"
)

func TestDesktopOAuthURLValidationAndCommandLineExtraction(t *testing.T) {
	valid := "inovarapp-desktop://oauth-callback/?code=abc"
	if !isDesktopOAuthURL(valid) {
		t.Fatal("valid desktop callback rejected")
	}
	if got := desktopOAuthArgument([]string{"--other", valid}); got != valid {
		t.Fatalf("callback argument=%q want=%q", got, valid)
	}
	for _, raw := range []string{
		"https://example.test/oauth-callback",
		"inovarapp-desktop://wrong-host/?code=abc",
		"com.inovarapp.mobile://oauth-callback/?code=abc",
	} {
		if isDesktopOAuthURL(raw) {
			t.Errorf("unexpected callback accepted: %q", raw)
		}
	}
}

func TestDesktopSaveFileRejectsInvalidDataAndFileTypes(t *testing.T) {
	app := &DesktopApp{}
	validBytes := base64.StdEncoding.EncodeToString([]byte("data"))
	for _, test := range []struct{ name, contents string }{
		{"backup.exe", validBytes},
		{"backup.json", "not-base64"},
		{"../secret.pdf", validBytes},
	} {
		if _, err := app.SaveFile(test.name, test.contents); err == nil {
			t.Errorf("SaveFile(%q) unexpectedly passed validation", test.name)
		}
	}
	if _, err := app.SaveFile("backup.json", validBytes); err == nil {
		t.Fatal("valid file should stop at the missing Wails runtime, before any write")
	}
}

func TestDesktopOpenExternalOnlyAcceptsHTTPS(t *testing.T) {
	app := &DesktopApp{}
	for _, raw := range []string{"javascript:alert(1)", "file:///secret", "http://insecure.test"} {
		if err := app.OpenExternal(raw); err == nil {
			t.Errorf("external URL %q unexpectedly accepted", raw)
		}
	}
	if err := app.OpenExternal("https://accounts.google.com/o/oauth2/auth"); err == nil {
		t.Fatal("OpenExternal should require a live Wails context for an accepted HTTPS URL")
	}
}
