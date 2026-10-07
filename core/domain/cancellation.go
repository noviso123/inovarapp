package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type ServiceCancellation struct {
	Date         string
	Reason       string
	Observations string
}

var cancellationMarkerCleanup = regexp.MustCompile(`\[(DATA_CANCELAMENTO|MOTIVO_CANCELAMENTO):[^\]]+\]`)

// BuildServiceCancellation centralizes the lifecycle markers used by older
// databases that do not yet have dedicated cancellation columns.
func BuildServiceCancellation(reason, observations string, now time.Time) (ServiceCancellation, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" || now.IsZero() {
		return ServiceCancellation{}, fmt.Errorf("motivo e data do cancelamento são obrigatórios")
	}
	date := now.Format("2006-01-02")
	clean := strings.TrimSpace(cancellationMarkerCleanup.ReplaceAllString(observations, ""))
	marker := fmt.Sprintf("[DATA_CANCELAMENTO:%s] [MOTIVO_CANCELAMENTO:%s]", date, strings.ReplaceAll(reason, "]", ")"))
	if clean != "" {
		clean += " "
	}
	return ServiceCancellation{Date: date, Reason: reason, Observations: clean + marker}, nil
}
