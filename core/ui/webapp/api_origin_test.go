package webapp

import (
	"net/url"
	"testing"
)

func TestResolveAPIBaseURL(t *testing.T) {
	page, _ := url.Parse("capacitor://localhost/")
	cases := []struct {
		configured string
		page       *url.URL
		want       string
	}{
		{"https://api.example.com/", page, "https://api.example.com"},
		{"", page, "capacitor://localhost"},
		{"http://localhost:8080", page, "http://localhost:8080"},
		{"http://api.example.com", page, ""},
		{"https://user:pass@example.com", page, ""},
		{"", nil, ""},
	}
	for _, test := range cases {
		if got := resolveAPIBaseURL(test.configured, test.page); got != test.want {
			t.Errorf("resolveAPIBaseURL(%q) = %q, want %q", test.configured, got, test.want)
		}
	}
}
