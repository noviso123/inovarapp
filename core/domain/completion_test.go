package domain

import (
	"strings"
	"testing"
	"time"
)

func TestBuildServiceCompletionKeepsLegacyMarkersAndClampsReturnDate(t *testing.T) {
	now := time.Date(2026, time.January, 31, 15, 30, 0, 0, time.FixedZone("BRT", -3*60*60))
	checklist := map[string]any{"filtrosLavados": true, "correnteAmperes": "4.2 A"}
	completion, err := BuildServiceCompletion("Limpeza de Ar", "PIX", checklist, 250, 35.5, 90, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if completion.Date != "2026-01-31" || completion.ReturnDate != "2026-02-28" || completion.WarrantyDays != 90 || completion.Total != 285.5 {
		t.Fatalf("completion=%+v", completion)
	}
	for _, marker := range []string{
		"[GARANTIA_DIAS:90]", "Pagamento: PIX.", "[PROXIMO_RETORNO:2026-02-28]",
		"[CHECKLIST:{\"correnteAmperes\":\"4.2 A\",\"filtrosLavados\":true}]", "[MAO_OBRA:250]", "[PECAS:35.5]",
	} {
		if !strings.Contains(completion.Observations, marker) {
			t.Errorf("missing legacy marker %q in %q", marker, completion.Observations)
		}
	}
}

func TestBuildServiceCompletionRejectsMissingRequiredFieldsAndClampsWarranty(t *testing.T) {
	now := time.Date(2026, time.October, 3, 12, 0, 0, 0, time.UTC)
	if _, err := BuildServiceCompletion("", "PIX", map[string]any{}, 1, 0, 1, 6, now); err != ErrInvalidServiceCompletion {
		t.Fatalf("missing type error=%v", err)
	}
	completion, err := BuildServiceCompletion("Avaliação Técnica", "A Faturar", map[string]any{}, 12, 0, -4, 6, now)
	if err != nil {
		t.Fatal(err)
	}
	if completion.WarrantyDays != 0 || !strings.Contains(completion.Observations, "[GARANTIA_DIAS:0]") {
		t.Fatalf("warranty was not clamped: %+v", completion)
	}
}
