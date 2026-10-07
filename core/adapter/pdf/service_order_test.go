package pdf

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"inovarapp/core/domain"
)

func TestGenerateServiceOrderPreservesUTF8AndBrandedPDF(t *testing.T) {
	model := "Inverter 12.000 BTU"
	address := "Rua João de Barro, Vitória"
	falseValue := false
	labor, parts := 210.0, 35.5
	data := ServiceOrderData{
		Client:    domain.Client{ID: "client-1", Name: "João Silva", Address: &address},
		Appliance: domain.Appliance{ID: "appliance-1", Brand: "Consul", Model: &model, CapacityBTU: "12000", Room: "Quarto"},
		Record: domain.MaintenanceRecord{
			ID: "service-12345678", Date: "2026-10-03", CompletionDate: "2026-10-03", ReturnDate: "2027-04-03",
			ServiceType: domain.ServiceCleaning, Price: 245.5, LaborPrice: &labor, PartsPrice: &parts,
			PartsUsed: "Bandeja e filtro", PaymentMethod: domain.PaymentPIX, WarrantyDays: 90,
			Checklist: &domain.ChecklistData{FiltersWashed: &falseValue, ThermalDeltaT: "9 °C"},
		},
		Profile: domain.TechnicianProfile{Name: "Gabriel", PIXKey: "pix@example.com", PIXType: domain.PIXEmail},
	}

	output, err := GenerateServiceOrder(data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(output, []byte("%PDF-")) {
		t.Fatalf("expected PDF signature, got %q", output[:minInt(len(output), 8)])
	}
	if len(output) < 300_000 {
		t.Fatalf("PDF too small to include the embedded brand art and font: %d bytes", len(output))
	}
	if !strings.Contains(string(output), "/FontFile2") || !strings.Contains(string(output), "/Subtype /Image") {
		t.Fatal("generated PDF is missing the embedded Unicode font or brand images")
	}
	if previewPath := os.Getenv("INOVAR_PDF_PREVIEW"); previewPath != "" {
		if err := os.WriteFile(previewPath, output, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFormatMoneyUsesBrazilianThousandsAndDecimals(t *testing.T) {
	if got := formatMoney(12345.6); got != "R$ 12.345,60" {
		t.Fatalf("formatMoney()=%q", got)
	}
}

func TestServiceOrderMaterialsPrintsDescriptiveParts(t *testing.T) {
	if got := serviceOrderMaterials(35.5, "Filtro antibacteriano"); got != "Peças/materiais: R$ 35,50 (Filtro antibacteriano)" {
		t.Fatalf("materials line=%q", got)
	}
	if got := serviceOrderMaterials(0, "Filtro antibacteriano"); got != "Materiais/insumos: Inclusos" {
		t.Fatalf("zero cost materials line=%q", got)
	}
}

func TestServiceChecklistPreservesLegacyMeasurementsAndFalseChecks(t *testing.T) {
	no := false
	for _, test := range []struct {
		name        string
		serviceType domain.ServiceType
		checklist   domain.ChecklistData
		partsUsed   string
		want        []string
	}{
		{
			name: "cleaning", serviceType: domain.ServiceCleaning,
			checklist: domain.ChecklistData{FiltersWashed: &no, DrainUnclogged: &no, ThermalDeltaT: "8 °C", GasPressurePSI: "118", CurrentAmps: "3.4"},
			want:      []string{"[ ] Filtros lavados", "[ ] Dreno desobstruído", "Salto térmico: 8 °C", "Pressão/corrente: 118 / 3.4"},
		},
		{
			name: "installation", serviceType: domain.ServiceInstallation,
			checklist: domain.ChecklistData{BracketLeveled: &no, VacuumMicrons: "420", NitrogenTest: &no, ValvesReleased: &no},
			want:      []string{"[ ] Fixação e nivelamento", "[X] Vácuo: 420", "[ ] Teste de estanqueidade", "[ ] Válvulas liberadas"},
		},
		{
			name: "corrective", serviceType: domain.ServiceCorrective,
			checklist: domain.ChecklistData{TechnicalDiagnosis: "Sensor aberto", CapacitorTested: "35 µF", PartsReplaced: "Sensor", CurrentAmps: "2.1"}, partsUsed: "Sensor",
			want: []string{"Diagnóstico: Sensor aberto", "Peças: Sensor", "Componente: 35 µF", "Corrente: 2.1"},
		},
		{
			name: "refrigerant", serviceType: domain.ServiceRefrigerant,
			checklist: domain.ChecklistData{GasAddedGrams: "550 g", GasPressurePSI: "120", CurrentAmps: "4.2"},
			want:      []string{"Fluido adicionado: 550 g", "Teste de vazamento realizado", "Pressão: 120", "Corrente: 4.2"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			left, right := serviceChecklist(domain.MaintenanceRecord{ServiceType: test.serviceType, PartsUsed: test.partsUsed, Checklist: &test.checklist})
			all := strings.Join(append(left, right...), "\n")
			for _, want := range test.want {
				if !strings.Contains(all, want) {
					t.Errorf("PDF checklist missing %q in:\n%s", want, all)
				}
			}
		})
	}
}

func TestBuildBudgetPDFIncludesBrandUtf8AndLongItemList(t *testing.T) {
	items := make([]domain.BudgetItem, 0, 36)
	for index := 0; index < 36; index++ {
		items = append(items, domain.BudgetItem{
			ID: "item", Description: "Higienização técnica do evaporador e serpentina — etapa número um",
			Quantity: 1, UnitPrice: 185.5, TotalPrice: 185.5, Category: domain.BudgetItemService,
		})
	}
	output, err := BuildBudgetPDF(domain.BudgetEstimate{
		ID: "budget-123", Number: "2026-0042", ClientName: "João da Silva", ClientPhone: "(27) 99999-0000",
		ClientAddress: "Rua João de Barro, Vitória — ES", EquipmentName: "LG Dual Inverter 12.000 BTUs",
		Date: "2026-10-03", ValidUntil: "2026-10-18", Items: items,
		TotalValue: 6678, Discount: 10, FinalValue: 6668, WarrantyTerms: "90 dias para mão de obra",
	}, domain.TechnicianProfile{CNPJ: "00.000.000/0001-00"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(output, []byte("%PDF-")) {
		t.Fatalf("expected PDF signature, got %q", output[:minInt(len(output), 8)])
	}
	if len(output) < 300_000 || !bytes.Contains(output, []byte("/FontFile2")) || !bytes.Contains(output, []byte("/Subtype /Image")) {
		t.Fatalf("PDF missing embedded Unicode font or brand imagery: %d bytes", len(output))
	}
	if previewPath := os.Getenv("INOVAR_BUDGET_PDF_PREVIEW"); previewPath != "" {
		if err := os.MkdirAll(filepath.Dir(previewPath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(previewPath, output, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBudgetValidityDaysUsesCivilDates(t *testing.T) {
	if days := budgetValidityDays("2026-03-01", "2026-03-31"); days != 30 {
		t.Fatalf("validity=%d want 30", days)
	}
	if days := budgetValidityDays("invalid", "2026-03-31"); days != 0 {
		t.Fatalf("invalid dates validity=%d want 0", days)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
