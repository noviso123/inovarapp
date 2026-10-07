package webapp

import (
	"net/url"
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

func TestSignupFormRequiresWhatsAppAndMatchesLegacyPostalLookupUI(t *testing.T) {
	page := &serviceCatalogPage{authMode: "signup", signupPostalLoading: true}
	markup := app.HTMLString(page.authModal())
	for _, want := range []string{`type="tel"`, `required`, `inputmode="numeric"`, `class="auth-cep-spinner"`, `aria-label="Buscando CEP"`} {
		if !strings.Contains(markup, want) {
			t.Errorf("signup form missing %q: %s", want, markup)
		}
	}
	page.signupPostalLoading = false
	markup = app.HTMLString(page.authModal())
	if strings.Contains(markup, `class="auth-cep-spinner"`) {
		t.Fatalf("CEP spinner rendered while idle: %s", markup)
	}
}

func TestAuthRedirectErrorReadsQueryAndFragment(t *testing.T) {
	for _, raw := range []struct {
		url  string
		want string
	}{
		{"https://inovar.example/?error=access_denied&error_description=Login+cancelado", "Login cancelado"},
		{"https://inovar.example/#error=access_denied&error_description=Permiss%C3%A3o+negada", "Permissão negada"},
		{"https://inovar.example/?error=access_denied", "access_denied"},
	} {
		parsed, err := url.Parse(raw.url)
		if err != nil {
			t.Fatal(err)
		}
		if got := authRedirectError(parsed); got != raw.want {
			t.Errorf("authRedirectError(%q) = %q, want %q", raw.url, got, raw.want)
		}
	}
	if got := authRedirectError(nil); got != "" {
		t.Fatalf("nil URL error = %q", got)
	}
}

func TestStripCredentialQueryRemovesLoginDetailsAndPreservesOAuthState(t *testing.T) {
	pageURL, err := url.Parse("https://inovar.example/?email=user%40example.com&password=secret&googleCalendar=1&recovery=1")
	if err != nil {
		t.Fatal(err)
	}
	if !stripCredentialQuery(pageURL) {
		t.Fatal("stripCredentialQuery did not report removed credentials")
	}
	query := pageURL.Query()
	for _, key := range []string{"email", "password", "senha"} {
		if query.Has(key) {
			t.Errorf("credential query parameter %q remains", key)
		}
	}
	if query.Get("googleCalendar") != "1" || query.Get("recovery") != "1" {
		t.Fatalf("non-credential OAuth state was not preserved: %v", query)
	}
	if stripCredentialQuery(pageURL) {
		t.Fatal("second sanitization unexpectedly changed the URL")
	}
}
