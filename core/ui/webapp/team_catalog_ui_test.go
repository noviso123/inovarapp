package webapp

import (
	"testing"

	"inovarapp/core/domain"
)

func TestApplyTeamCatalogChangeAddsAndEditsEntries(t *testing.T) {
	data := domain.DadosTipoServico{Name: "Higienização", Price: 320, Items: []string{"Limpeza técnica"}}
	profile, err := applyTeamCatalogChange(domain.TechnicianProfile{}, "", data)
	if err != nil || len(profile.CustomServiceTypes) != 1 || profile.CustomServiceTypes[0].Name != data.Name {
		t.Fatalf("add profile=%+v err=%v", profile, err)
	}

	profile, err = applyTeamCatalogChange(profile, "custom:0", domain.DadosTipoServico{Name: "Higienização Plus", Price: 400})
	if err != nil || len(profile.CustomServiceTypes) != 1 || profile.CustomServiceTypes[0].Name != "Higienização Plus" {
		t.Fatalf("custom edit profile=%+v err=%v", profile, err)
	}

	profile, err = applyTeamCatalogChange(profile, "fixo:Limpeza de Ar", domain.DadosTipoServico{Name: "Limpeza Premium", Price: 450})
	if err != nil || len(profile.EditedFixedServiceTypes) != 1 || profile.EditedFixedServiceTypes[0].Type != "Limpeza de Ar" {
		t.Fatalf("fixed edit profile=%+v err=%v", profile, err)
	}
	entries := domain.BuildServiceCatalog(&profile)
	found := false
	for _, entry := range entries {
		if entry.Key == "fixo:Limpeza de Ar" {
			found = entry.Name == "Limpeza Premium" && entry.Price == 450
		}
	}
	if !found {
		t.Fatalf("fixed catalog edit not reflected: %+v", entries)
	}
}

func TestRemoveTeamCatalogEntryRemovesCustomAndFixedTypes(t *testing.T) {
	profile := domain.TechnicianProfile{CustomServiceTypes: []domain.CustomServiceType{{Name: "Higienização"}}}
	profile, name, err := removeTeamCatalogEntry(profile, "custom:0")
	if err != nil || name != "Higienização" || len(profile.CustomServiceTypes) != 0 {
		t.Fatalf("custom remove profile=%+v name=%q err=%v", profile, name, err)
	}
	profile, _, err = removeTeamCatalogEntry(profile, "fixo:Limpeza de Ar")
	if err != nil || !containsCatalogValue(profile.RemovedFixedServiceTypes, "Limpeza de Ar") {
		t.Fatalf("fixed remove profile=%+v err=%v", profile, err)
	}
	profile, _, err = removeTeamCatalogEntry(profile, "fixo:Limpeza de Ar")
	if err != nil || len(profile.RemovedFixedServiceTypes) != 1 {
		t.Fatalf("fixed remove should be idempotent, profile=%+v err=%v", profile, err)
	}
}

func TestTeamCatalogChangeRejectsStaleKeys(t *testing.T) {
	if _, err := applyTeamCatalogChange(domain.TechnicianProfile{}, "custom:3", domain.DadosTipoServico{Name: "x"}); err == nil {
		t.Fatal("expected stale edit key rejection")
	}
	if _, _, err := removeTeamCatalogEntry(domain.TechnicianProfile{}, "custom:0"); err == nil {
		t.Fatal("expected stale delete key rejection")
	}
}

func TestTeamServiceCatalogCardIncludesCustomAndEditedFixedEntries(t *testing.T) {
	profile := domain.TechnicianProfile{
		CustomServiceTypes:      []domain.CustomServiceType{{Name: "Higienização Premium", Price: 480, Description: "Limpeza ampliada", AverageTime: "2 horas", DefaultWarranty: "120 dias", Badge: "Premium", Items: []string{"Serpentina", "Dreno"}}},
		EditedFixedServiceTypes: []domain.EditedFixedServiceType{{Type: string(domain.ServiceCleaning), DadosTipoServico: domain.DadosTipoServico{Name: "Limpeza Comercial", Price: 520, Description: "Plano comercial", AverageTime: "3 horas", DefaultWarranty: "180 dias", Badge: "Empresa", Items: []string{"PMOC"}}}},
	}
	entries := domain.BuildServiceCatalog(&profile)
	var custom, fixed *domain.CatalogEntry
	for i := range entries {
		if entries[i].Key == "custom:0" {
			custom = &entries[i]
		}
		if entries[i].Key == "fixo:"+string(domain.ServiceCleaning) {
			fixed = &entries[i]
		}
	}
	if custom == nil || fixed == nil {
		t.Fatalf("missing custom/fixed entries: %+v", entries)
	}
	customCard, customPrice := teamServiceCatalogCard(*custom)
	if customCard.Title != "Higienização Premium" || customCard.Description != "Limpeza ampliada" || customCard.AverageTime != "2 horas" || customCard.DefaultWarranty != "120 dias" || customCard.Badge != "Premium" || len(customCard.IncludedItems) != 2 || customPrice != "R$ 480,00" {
		t.Fatalf("custom card=%+v price=%q", customCard, customPrice)
	}
	fixedCard, fixedPrice := teamServiceCatalogCard(*fixed)
	if fixedCard.Title != "Limpeza Comercial" || fixedCard.Description != "Plano comercial" || fixedCard.AverageTime != "3 horas" || fixedCard.DefaultWarranty != "180 dias" || fixedCard.Badge != "Empresa" || len(fixedCard.IncludedItems) != 1 || fixedPrice != "R$ 520,00" {
		t.Fatalf("fixed card=%+v price=%q", fixedCard, fixedPrice)
	}
}

func TestNewTeamCatalogFormPrefillsFixedServiceDefaults(t *testing.T) {
	entry := domain.BuildServiceCatalog(nil)[0]
	form := newTeamCatalogForm(&entry)
	if form.Name != entry.Name || form.Description != entry.Card.Description || form.AverageTime != entry.Card.AverageTime || form.Warranty != entry.Card.DefaultWarranty || form.Badge != entry.Card.Badge || form.Items == "" {
		t.Fatalf("fixed form did not prefill defaults: %+v", form)
	}
}

func TestTeamCompletionCatalogKeepsFixedTypeAndCustomDisplayNameSeparate(t *testing.T) {
	entries := domain.BuildServiceCatalog(&domain.TechnicianProfile{
		EditedFixedServiceTypes: []domain.EditedFixedServiceType{{Type: string(domain.ServiceCleaning), DadosTipoServico: domain.DadosTipoServico{Name: "Limpeza Comercial"}}},
		CustomServiceTypes:      []domain.CustomServiceType{{Name: "Higienização Premium"}},
	})
	if got := teamCompletionCatalogValue(entries[0]); got != string(domain.ServiceCleaning) {
		t.Fatalf("fixed option value=%q, want canonical checklist type %q", got, domain.ServiceCleaning)
	}
	custom := entries[len(entries)-1]
	if got := teamCompletionCatalogValue(custom); got != "Higienização Premium" {
		t.Fatalf("custom option value=%q", got)
	}
}
