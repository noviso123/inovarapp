package domain

import (
	"regexp"
	"strconv"
	"strings"
)

var serviceObservationMarkers = regexp.MustCompile(`\[(?:GARANTIA_DIAS|DATA_CONCLUSAO|DATA_INICIO|DATA_CANCELAMENTO|MOTIVO_CANCELAMENTO|PROXIMO_RETORNO|MAO_OBRA|PECAS):[^\]]+\]|\[CHECKLIST:\{(?s:.*?)\}\]`)
var serviceHorizontalWhitespace = regexp.MustCompile(`[\t ]+`)
var serviceWarrantyText = regexp.MustCompile(`(?i)Garantia de \d+ dias\.?`)

// MergeServiceObservationMarkers replaces the free-form observation while
// preserving lifecycle, warranty, payment and checklist markers used by older
// service orders and their PDFs.
func MergeServiceObservationMarkers(replacement, existing string) string {
	markers := serviceObservationMarkers.FindAllString(existing, -1)
	parts := make([]string, 0, len(markers)+1)
	if replacement = strings.TrimSpace(replacement); replacement != "" {
		parts = append(parts, replacement)
	}
	parts = append(parts, markers...)
	return strings.TrimSpace(strings.Join(parts, " "))
}

// SetServiceDateMarker replaces one lifecycle date marker without changing the
// remaining notes or audit markers when an older database lacks date columns.
func SetServiceDateMarker(observations, name, date string) string {
	if name != "DATA_CONCLUSAO" && name != "DATA_CANCELAMENTO" && name != "DATA_INICIO" {
		return strings.TrimSpace(observations)
	}
	pattern := regexp.MustCompile(`\[` + regexp.QuoteMeta(name) + `:[^\]]+\]`)
	observations = strings.TrimSpace(serviceHorizontalWhitespace.ReplaceAllString(pattern.ReplaceAllString(observations, ""), " "))
	observations = regexp.MustCompile(`[ \t]{2,}`).ReplaceAllString(observations, " ")
	marker := "[" + name + ":" + strings.TrimSpace(date) + "]"
	if observations == "" {
		return marker
	}
	return observations + " " + marker
}

// SetServiceWarrantyMarker replaces the legacy warranty marker and sentence,
// preserving all other free-form notes and service markers.
func SetServiceWarrantyMarker(observations string, days int) string {
	if days < 0 {
		days = 0
	}
	marker := regexp.MustCompile(`\[GARANTIA_DIAS:[^\]]+\]`)
	observations = marker.ReplaceAllString(observations, "")
	observations = serviceWarrantyText.ReplaceAllString(observations, "")
	observations = strings.TrimSpace(serviceHorizontalWhitespace.ReplaceAllString(observations, " "))
	warranty := "[GARANTIA_DIAS:" + strconv.Itoa(days) + "] Garantia de " + strconv.Itoa(days) + " dias."
	if observations == "" {
		return warranty
	}
	return observations + " " + warranty
}
