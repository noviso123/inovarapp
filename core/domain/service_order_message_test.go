package domain

import (
	"strings"
	"testing"
)

func TestBuildServiceOrderReceiptMessageMatchesLegacyTextAndFallbacks(t *testing.T) {
	got := BuildServiceOrderReceiptMessage(
		Client{Name: "João Silva"},
		Appliance{Room: "Sala", Brand: "LG", CapacityBTU: "12000"},
		MaintenanceRecord{Date: "2026-10-04", ServiceType: ServiceCleaning, Price: 250, PaymentMethod: PaymentPIX, WarrantyDays: 90},
		TechnicianProfile{Name: "Gabriel", BusinessName: "Inovar Refrigeração"},
	)
	want := "Olá *João*! Segue o comprovante do serviço realizado:\n\n❄️ *ORDEM DE SERVIÇO & COMPROVANTE*\n👤 *Cliente:* João Silva\n📍 *Local:* Sala (LG 12000 BTUs)\n🔧 *Serviço:* Limpeza de Ar\n📅 *Data:* 04/10/2026\n💰 *Valor:* R$ 250.00 (PIX)\n🛡️ Garantia de 90 dias inclusa.\n🔄 *Próximo retorno recomendado:* —\n\nMuito obrigado pela confiança! Qualquer dúvida estou à disposição.\n*Gabriel* - Inovar Refrigeração"
	if got != want {
		t.Fatalf("receipt message mismatch:\n got: %q\nwant: %q", got, want)
	}
}

func TestBuildServiceOrderReceiptMessageAddsPIXAndHandlesMissingDates(t *testing.T) {
	got := BuildServiceOrderReceiptMessage(Client{Name: "Ana"}, Appliance{}, MaintenanceRecord{ServiceType: ServiceOther}, TechnicianProfile{PIXKey: "ana@example.test", PIXType: PIXEmail})
	for _, want := range []string{"📅 *Data:* —", "🔑 *Chave PIX:* ana@example.test (EMAIL)", "🛡️ \n🔄 *Próximo retorno recomendado:* —"} {
		if !strings.Contains(got, want) {
			t.Errorf("message %q missing %q", got, want)
		}
	}
}
