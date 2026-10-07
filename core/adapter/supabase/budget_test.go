package supabase

import (
	"encoding/json"
	"strings"
	"testing"

	"inovarapp/core/domain"
)

func TestMapSupabaseBudgetToLocalReadsMetadataAndDatabaseColumns(t *testing.T) {
	row := SupabaseBudget{
		ID: "budget-1", Number: "2026-0042", ClientID: "client-1", ServiceID: "service-1",
		Date: "2026-10-03", ValidUntil: "2026-10-18", ServiceType: "Limpeza de Ar",
		LaborValue: 200, MaterialsValue: 50, TotalValue: 225, Conditions: "PIX",
		Status:      "APROVADO",
		Description: `{"numero":"2026-0042","clientName":"João Silva","clientPhone":"27999999999","clientAddress":"Serra","clientDocument":"123","equipmentName":"LG Split","applianceDesc":"LG 12.000 BTUs","items":[{"id":"item-1","description":"Limpeza de Ar","quantity":1,"unitPrice":250,"totalPrice":250,"category":"servico"}],"discount":25,"totalValue":250,"executionTime":"2h","warrantyTerms":"90 dias","notes":"Confirmar horário","assinatura":"data:image/png;base64,abc","assinatura_em":"2026-10-03T10:00:00Z","pago":true,"pago_em":"2026-10-03T11:00:00Z","valor_recebido":225,"applianceId":"appliance-1"}`,
	}
	got := MapSupabaseBudgetToLocal(row)
	if got.Number != row.Number || got.ClientID != row.ClientID || got.ClientName != "João Silva" || got.ApplianceDescription != "LG 12.000 BTUs" {
		t.Fatalf("identity or UTF-8 metadata changed: %#v", got)
	}
	if got.Status != domain.BudgetApproved || got.ValidUntil != row.ValidUntil || got.TotalValue != 250 || got.FinalValue != 225 || got.Discount != 25 {
		t.Fatalf("budget totals/status changed: %#v", got)
	}
	if len(got.Items) != 1 || got.Items[0].Category != domain.BudgetItemService || got.ServiceID == nil || *got.ServiceID != "service-1" {
		t.Fatalf("items or service link changed: %#v", got)
	}
	if got.Signature == nil || *got.Signature != "data:image/png;base64,abc" || got.Paid == nil || !*got.Paid || got.AmountReceived == nil || *got.AmountReceived != 225 {
		t.Fatalf("signature/payment metadata changed: %#v", got)
	}
}

func TestMapSupabaseBudgetToLocalUsesLegacyFallbacks(t *testing.T) {
	row := SupabaseBudget{ID: "budget-2", ClientID: "client-2", Date: "2026-10-03", Description: "Descrição antiga", LaborValue: 100, MaterialsValue: 30, TotalValue: 125, Status: "ENVIADO"}
	got := MapSupabaseBudgetToLocal(row)
	if got.ClientName != "Cliente Inovar" || got.ValidUntil != row.Date || got.TotalValue != 130 || got.FinalValue != 125 || got.PaymentConditions != "A combinar" || got.ExecutionTime != "A combinar" {
		t.Fatalf("legacy defaults changed: %#v", got)
	}
	if got.WarrantyTerms != "Não informado — parametrização não registrada neste orçamento" || len(got.Items) != 1 || got.Items[0].Description != "Descrição antiga" || got.Items[0].TotalPrice != 125 {
		t.Fatalf("legacy description fallback changed: %#v", got)
	}
	if got.Status != domain.BudgetPending {
		t.Fatalf("unknown database status = %q, want pending", got.Status)
	}
	empty := MapSupabaseBudgetToLocal(SupabaseBudget{ID: "budget-empty"})
	if empty.Items == nil || len(empty.Items) != 0 {
		t.Fatalf("missing description should produce an empty JSON list, got %#v", empty.Items)
	}
}

func TestMapLocalBudgetToSupabasePreservesWriteContract(t *testing.T) {
	budget := domain.BudgetEstimate{
		ClientName: "João Silva", ClientPhone: "27999999999", ClientAddress: "",
		EquipmentName: "LG Split", ApplianceDescription: "LG 12.000 BTUs", Date: "2026-10-03",
		ValidUntil: "2026-10-18", Items: []domain.BudgetItem{
			{ID: "i1", Description: "Serviço", Quantity: 1, UnitPrice: 200, TotalPrice: 200, Category: domain.BudgetItemService},
			{ID: "i2", Description: "Peça", Quantity: 2, UnitPrice: 25, TotalPrice: 50, Category: domain.BudgetItemPart},
		},
		TotalValue: 250, Discount: 25, FinalValue: 225, PaymentConditions: "PIX",
		ExecutionTime: "2h", WarrantyTerms: "90 dias", Notes: "",
	}
	got, err := MapLocalBudgetToSupabase(budget, "2026-0042", "client-1", "Gabriel")
	if err != nil {
		t.Fatal(err)
	}
	if got.Number != "2026-0042" || got.ClientID != "client-1" || got.LaborValue != 200 || got.MaterialsValue != 50 || got.TotalValue != 225 || got.Status != "ENVIADO" || got.ServiceType != "Serviço" {
		t.Fatalf("database columns changed: %#v", got)
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal([]byte(got.Description), &metadata); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"numero", "clientName", "clientPhone", "clientAddress", "clientDocument", "equipmentName", "applianceDesc", "items", "discount", "totalValue", "executionTime", "warrantyTerms", "notes", "assinatura", "assinatura_em", "pago", "pago_em", "valor_recebido", "applianceId"} {
		if _, ok := metadata[key]; !ok {
			t.Errorf("metadata missing %q: %s", key, got.Description)
		}
	}
	if string(metadata["assinatura"]) != "null" || string(metadata["assinatura_em"]) != "null" || string(metadata["valor_recebido"]) != "null" || string(metadata["applianceId"]) != "null" {
		t.Errorf("nullable metadata did not retain nulls: %s", got.Description)
	}
	if _, ok := metadata["service_id"]; ok {
		t.Errorf("absent service id should be omitted: %s", got.Description)
	}
	if !strings.Contains(got.Description, "João Silva") || !strings.Contains(got.Description, "Peça") {
		t.Errorf("JSON metadata lost UTF-8: %s", got.Description)
	}
}

func TestTruncateUTF16DoesNotSplitUTF8Characters(t *testing.T) {
	value := strings.Repeat("á", 119) + "😀" + "x"
	got := truncateUTF16(value, 120)
	if got != strings.Repeat("á", 119) {
		t.Fatalf("truncateUTF16 returned invalid boundary: length=%d", len([]rune(got)))
	}
}
