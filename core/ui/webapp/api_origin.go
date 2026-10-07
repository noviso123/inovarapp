package webapp

import (
	"net/url"
	"strings"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

func apiBaseURL() string {
	return resolveAPIBaseURL(app.Getenv("API_BASE_URL"), app.Window().URL())
}

func apiEndpoint(path string) string {
	base := apiBaseURL()
	if base == "" {
		return ""
	}
	return base + "/" + strings.TrimLeft(path, "/")
}

func resolveAPIBaseURL(configured string, pageURL *url.URL) string {
	configured = strings.TrimRight(strings.TrimSpace(configured), "/")
	if configured != "" {
		parsed, err := url.Parse(configured)
		if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return ""
		}
		if parsed.Scheme == "https" || parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1") {
			return configured
		}
		return ""
	}
	if pageURL == nil || pageURL.Scheme == "" || pageURL.Host == "" {
		return ""
	}
	return pageURL.Scheme + "://" + pageURL.Host
}
