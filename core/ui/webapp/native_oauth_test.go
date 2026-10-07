package webapp

import (
	"net/url"
	"testing"
)

func TestOAuthRedirectURLForDesktopAndNativePlatforms(t *testing.T) {
	pageURL, err := url.Parse("https://inovar.example/?old=1#old-token")
	if err != nil {
		t.Fatal(err)
	}
	flags := url.Values{"googleCalendar": {"1"}}
	if got, want := oauthRedirectURLFor(pageURL, flags, false), "https://inovar.example/?googleCalendar=1"; got != want {
		t.Errorf("web OAuth redirect = %q, want %q", got, want)
	}
	if got, want := oauthRedirectURLFor(pageURL, flags, true), "com.inovarapp.mobile://oauth-callback/?googleCalendar=1"; got != want {
		t.Errorf("native OAuth redirect = %q, want %q", got, want)
	}
	if got, want := desktopOAuthRedirectURL(flags), "inovarapp-desktop://oauth-callback/?googleCalendar=1"; got != want {
		t.Errorf("desktop OAuth redirect = %q, want %q", got, want)
	}
}

func TestIsNativeOAuthCallbackURLRequiresRegisteredSchemeAndHost(t *testing.T) {
	for _, raw := range []string{
		"com.inovarapp.mobile://oauth-callback/#access_token=token",
		"COM.INOVARAPP.MOBILE://OAUTH-CALLBACK/?googleContacts=1#access_token=token",
		"inovarapp-desktop://oauth-callback/?code=oauth-code",
	} {
		if !isNativeOAuthCallbackURL(raw) {
			t.Errorf("expected native OAuth callback to be accepted: %s", raw)
		}
	}
	for _, raw := range []string{
		"https://inovarapp.example/oauth-callback#access_token=token",
		"com.inovarapp.mobile://different-host/#access_token=token",
		"inovarapp-desktop://different-host/#access_token=token",
		"another-app://oauth-callback/#access_token=token",
		"not a URL",
	} {
		if isNativeOAuthCallbackURL(raw) {
			t.Errorf("unexpected OAuth callback accepted: %s", raw)
		}
	}
}
