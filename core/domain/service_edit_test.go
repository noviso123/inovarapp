package domain

import (
	"strings"
	"testing"
)

func TestMergeServiceObservationMarkersKeepsChecklistAndWarranty(t *testing.T) {
	got := MergeServiceObservationMarkers("texto atualizado", `texto antigo [GARANTIA_DIAS:90] [PROXIMO_RETORNO:2027-04-03] [CHECKLIST:{"itens":{"filtros":true}}] [MAO_OBRA:200]`)
	for _, want := range []string{"texto atualizado", "[GARANTIA_DIAS:90]", "[PROXIMO_RETORNO:2027-04-03]", `[CHECKLIST:{"itens":{"filtros":true}}]`, "[MAO_OBRA:200]"} {
		if !strings.Contains(got, want) {
			t.Fatalf("merged observations %q missing %q", got, want)
		}
	}
	if strings.Contains(got, "texto antigo") {
		t.Fatalf("free text was not replaced: %q", got)
	}
}

func TestSetServiceWarrantyMarkerReplacesWarrantyAndKeepsOtherMarkers(t *testing.T) {
	input := `Nota do técnico. [GARANTIA_DIAS:90] Garantia de 90 dias. [CHECKLIST:{"filtrosLavados":true}] [PROXIMO_RETORNO:2027-04-03]`
	got := SetServiceWarrantyMarker(input, 180)
	for _, want := range []string{"Nota do técnico.", "[GARANTIA_DIAS:180] Garantia de 180 dias.", `[CHECKLIST:{"filtrosLavados":true}]`, "[PROXIMO_RETORNO:2027-04-03]"} {
		if !strings.Contains(got, want) {
			t.Errorf("SetServiceWarrantyMarker dropped %q from %q", want, got)
		}
	}
	if strings.Contains(got, "[GARANTIA_DIAS:90]") || strings.Contains(got, "Garantia de 90 dias") {
		t.Fatalf("old warranty marker was retained: %q", got)
	}
}

func TestSetServiceWarrantyMarkerClampsNegativeDays(t *testing.T) {
	if got := SetServiceWarrantyMarker("", -5); got != "[GARANTIA_DIAS:0] Garantia de 0 dias." {
		t.Fatalf("got %q", got)
	}
}

func TestSetServiceDateMarkerReplacesOnlyTargetMarker(t *testing.T) {
	got := SetServiceDateMarker("observação [DATA_CONCLUSAO:2026-01-01] [GARANTIA_DIAS:90]", "DATA_CONCLUSAO", "2026-10-03")
	want := "observação [GARANTIA_DIAS:90] [DATA_CONCLUSAO:2026-10-03]"
	if got != want {
		t.Fatalf("marker=%q want %q", got, want)
	}
}
