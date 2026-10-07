package webapp

import (
	"net/url"
	"strings"
)

const nativeOAuthScheme = "com.inovarapp.mobile"
const desktopOAuthScheme = "inovarapp-desktop"
const nativeOAuthHost = "oauth-callback"

func nativeOAuthRedirectURL(flags url.Values) string {
	return (&url.URL{Scheme: nativeOAuthScheme, Host: nativeOAuthHost, Path: "/", RawQuery: flags.Encode()}).String()
}

func oauthRedirectURLFor(pageURL *url.URL, flags url.Values, native bool) string {
	if native {
		return nativeOAuthRedirectURL(flags)
	}
	if pageURL == nil {
		return ""
	}
	redirect := *pageURL
	redirect.RawQuery = flags.Encode()
	redirect.Fragment = ""
	return redirect.String()
}

func desktopOAuthRedirectURL(flags url.Values) string {
	return (&url.URL{Scheme: desktopOAuthScheme, Host: nativeOAuthHost, Path: "/", RawQuery: flags.Encode()}).String()
}

func isNativeOAuthCallbackURL(raw string) bool {
	callback, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && isOAuthCallbackScheme(callback.Scheme) && strings.EqualFold(callback.Host, nativeOAuthHost)
}

func isDesktopOAuthCallbackURL(raw string) bool {
	callback, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && strings.EqualFold(callback.Scheme, desktopOAuthScheme) && strings.EqualFold(callback.Host, nativeOAuthHost)
}

func isOAuthCallbackScheme(scheme string) bool {
	return strings.EqualFold(scheme, nativeOAuthScheme) || strings.EqualFold(scheme, desktopOAuthScheme)
}
