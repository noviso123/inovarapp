package domain

import (
	"strings"
	"testing"
	"time"
)

func TestBuildServiceCancellationPreservesNotesAndReplacesOldMarkers(t *testing.T) {
	got, err := BuildServiceCancellation("Cancelado pelo técnico", "Cliente pediu retorno [DATA_CANCELAMENTO:2026-01-01] [MOTIVO_CANCELAMENTO:antigo]", time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if got.Date != "2026-10-03" || got.Reason != "Cancelado pelo técnico" || !strings.HasPrefix(got.Observations, "Cliente pediu retorno ") {
		t.Fatalf("cancellation=%#v", got)
	}
	if strings.Count(got.Observations, "DATA_CANCELAMENTO:") != 1 || !strings.Contains(got.Observations, "MOTIVO_CANCELAMENTO:Cancelado pelo técnico") {
		t.Fatalf("markers=%q", got.Observations)
	}
}

func TestBuildServiceCancellationRejectsMissingReason(t *testing.T) {
	if _, err := BuildServiceCancellation(" ", "", time.Now()); err == nil {
		t.Fatal("empty reason accepted")
	}
}
