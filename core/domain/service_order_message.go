package domain

import (
	"fmt"
	"strings"
	"time"
)

// BuildServiceOrderReceiptMessage keeps the WhatsApp receipt text used by the
// legacy service-order modal, including its formatting and empty-date fallback.
func BuildServiceOrderReceiptMessage(client Client, appliance Appliance, record MaintenanceRecord, profile TechnicianProfile) string {
	firstName := client.Name
	if parts := strings.Fields(firstName); len(parts) > 0 {
		firstName = parts[0]
	}
	date := serviceOrderMessageDate(record.Date)
	returnDate := serviceOrderMessageDate(record.ReturnDate)
	warranty := ""
	if record.WarrantyDays != 0 {
		warranty = fmt.Sprintf("Garantia de %d dias inclusa.", record.WarrantyDays)
	}
	message := fmt.Sprintf("Olá *%s*! Segue o comprovante do serviço realizado:\n\n❄️ *ORDEM DE SERVIÇO & COMPROVANTE*\n👤 *Cliente:* %s\n📍 *Local:* %s (%s %s BTUs)\n🔧 *Serviço:* %s\n📅 *Data:* %s\n💰 *Valor:* R$ %.2f (%s)\n", firstName, client.Name, appliance.Room, appliance.Brand, appliance.CapacityBTU, record.ServiceType, date, record.Price, record.PaymentMethod)
	if profile.PIXKey != "" {
		message += fmt.Sprintf("🔑 *Chave PIX:* %s (%s)\n", profile.PIXKey, strings.ToUpper(string(profile.PIXType)))
	}
	message += fmt.Sprintf("🛡️ %s\n🔄 *Próximo retorno recomendado:* %s\n\nMuito obrigado pela confiança! Qualquer dúvida estou à disposição.\n*%s* - %s", warranty, returnDate, profile.Name, profile.BusinessName)
	return message
}

func serviceOrderMessageDate(value string) string {
	if len(value) < 10 {
		return "—"
	}
	parsed, err := time.Parse("2006-01-02", value[:10])
	if err != nil {
		return "—"
	}
	return parsed.Format("02/01/2006")
}
