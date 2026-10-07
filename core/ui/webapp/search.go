package webapp

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// searchMatches treats each query word as a live substring filter, ignores
// accents and punctuation, and keeps phone/CEP searches usable with or without
// formatting characters.
func searchMatches(query string, fields ...string) bool {
	query = normalizeSearchText(query)
	if query == "" {
		return true
	}

	var values strings.Builder
	var digits strings.Builder
	for _, field := range fields {
		values.WriteByte(' ')
		values.WriteString(normalizeSearchText(field))
		digits.WriteByte(' ')
		for _, r := range field {
			if r >= '0' && r <= '9' {
				digits.WriteRune(r)
			}
		}
	}
	haystack := values.String()

	for _, term := range strings.Fields(query) {
		if onlyDigits(term) {
			if !strings.Contains(digits.String(), term) {
				return false
			}
			continue
		}
		if !strings.Contains(haystack, term) {
			return false
		}
	}
	return true
}

func normalizeSearchText(value string) string {
	value = norm.NFD.String(strings.ToLower(strings.TrimSpace(value)))
	var normalized strings.Builder
	for _, r := range value {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			normalized.WriteRune(r)
		} else {
			normalized.WriteByte(' ')
		}
	}
	return strings.Join(strings.Fields(normalized.String()), " ")
}

func onlyDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
