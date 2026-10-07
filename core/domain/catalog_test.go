package domain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFixedCatalogPreservesTypesCopyAndVisualData(t *testing.T) {
	wantTypes := []ServiceType{
		ServiceCleaning, ServiceInstallation, ServiceCorrective,
		ServiceRefrigerant, ServicePreventive, ServiceTechnicalAssessment,
	}
	wantPrices := []float64{280, 1300, 250, 305, 0, 110}
	entries := BuildServiceCatalog(nil)
	if len(entries) != len(wantTypes) {
		t.Fatalf("catalog has %d entries; want %d", len(entries), len(wantTypes))
	}
	for i, entry := range entries {
		if entry.FixedType != wantTypes[i] || entry.Name != string(wantTypes[i]) || entry.Price != wantPrices[i] {
			t.Errorf("entry %d = %#v", i, entry)
		}
		if entry.Card == nil || len(entry.Card.IncludedItems) == 0 {
			t.Errorf("entry %d lost card details", i)
		}
	}
	if entries[0].Card.Title != "Limpeza de Ar Completa" || entries[0].Card.Color != "emerald" || entries[0].Card.Icon != "sparkles" {
		t.Errorf("first card lost display metadata: %#v", entries[0].Card)
	}
	entries[0].Card.IncludedItems[0] = "mutação externa"
	if BuildServiceCatalog(nil)[0].Card.IncludedItems[0] != "Lavagem de filtros e carenagem plástica" {
		t.Fatal("catalog result mutated the shared fixed catalog")
	}
}

func TestBuildServiceCatalogAppliesRemovedEditedAndCustomTypes(t *testing.T) {
	profile := &TechnicianProfile{
		RemovedFixedServiceTypes: []string{"Avaliação Técnica"},
		EditedFixedServiceTypes: []EditedFixedServiceType{{
			Type: string(ServiceCleaning),
			DadosTipoServico: DadosTipoServico{
				Name: "Limpeza Premium", Price: 350, Description: "Descrição editada",
				AverageTime: "2h", DefaultWarranty: "120 dias", Badge: "Personalizado",
				Items: []string{"Etapa 1", "Etapa 2"},
			},
		}},
		CustomServiceTypes: []CustomServiceType{{
			Name: "Instalação de duto", Price: 725, Description: "Tipo customizado", Items: []string{"Medição"},
		}},
	}
	entries := BuildServiceCatalog(profile)
	if len(entries) != 6 { // cinco fixos disponíveis e um tipo customizado
		t.Fatalf("catalog has %d entries; want 6", len(entries))
	}
	if entries[0].Key != "fixo:Limpeza de Ar" || entries[0].Name != "Limpeza Premium" || !entries[0].Edited || entries[0].Price != 350 {
		t.Errorf("fixed edit not applied: %#v", entries[0])
	}
	if entries[0].Description != "Descrição editada" || entries[0].AverageTime != "2h" || entries[0].DefaultWarranty != "120 dias" || len(entries[0].Items) != 2 {
		t.Errorf("edited metadata not preserved: %#v", entries[0])
	}
	last := entries[len(entries)-1]
	if last.Key != "custom:0" || last.Name != "Instalação de duto" || last.Price != 725 || last.Card != nil {
		t.Errorf("custom entry not preserved: %#v", last)
	}
}

func TestExtractReferencePriceUsesFirstNumber(t *testing.T) {
	cases := map[string]float64{
		"R$ 280":                   280,
		"R$ 250 - 625 + peças":     250,
		"Contratos / Sob Consulta": 0,
		"R$ 1.250,50":              1250,
		"R$ 1.300":                 1300,
	}
	for reference, want := range cases {
		if got := ExtractReferencePrice(reference); got != want {
			t.Errorf("ExtractReferencePrice(%q) = %v, want %v", reference, got, want)
		}
	}
}

func TestFixedCatalogCopyMatchesReactSourceContract(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "..", "src", "services", "catalogo.ts"))
	if err != nil {
		t.Fatalf("read React catalog source: %v", err)
	}
	legacy := string(source)
	for _, entry := range BuildServiceCatalog(nil) {
		card := entry.Card
		if card == nil {
			t.Fatalf("fixed service %q has no card", entry.Name)
		}
		for label, value := range map[string]string{
			"type": string(card.Type), "title": card.Title, "subtitle": card.Subtitle,
			"description": card.Description, "average time": card.AverageTime,
			"warranty": card.DefaultWarranty, "reference price": card.ReferencePrice,
			"badge": card.Badge, "accent color": card.AccentColor,
			"gradient": card.GradientClass, "color": card.Color,
		} {
			if !strings.Contains(legacy, "'"+value+"'") && !strings.Contains(legacy, "\""+value+"\"") {
				t.Errorf("React catalog is missing %s %q for service %q", label, value, entry.Name)
			}
		}
		for _, item := range card.IncludedItems {
			if !strings.Contains(legacy, "'"+item+"'") && !strings.Contains(legacy, "\""+item+"\"") {
				t.Errorf("React catalog is missing included item %q for service %q", item, entry.Name)
			}
		}
	}
}
