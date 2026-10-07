package webapp

import (
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

func TestWhatsAppQueuePanelExplainsScheduledMessagesAndRetention(t *testing.T) {
	markup := app.HTMLString((&serviceCatalogPage{teamWhatsAppQueueLoaded: true}).teamWhatsAppQueuePanel())
	for _, expected := range []string{
		"Agendamentos e fila do WhatsApp",
		"mensagens enviadas ficam na fila por 3 dias",
		"Pendentes e falhas não são apagadas",
		"Consulta concluída: fila vazia",
	} {
		if !strings.Contains(markup, expected) {
			t.Errorf("WhatsApp queue panel missing %q: %s", expected, markup)
		}
	}
}

func TestWhatsAppQueueFailureDoesNotLookLikeSuccessfulEmptyQueue(t *testing.T) {
	markup := app.HTMLString((&serviceCatalogPage{teamWhatsAppQueueError: "Falha na consulta"}).teamWhatsAppQueuePanel())
	if !strings.Contains(markup, "Não foi possível confirmar o estado atual da fila") {
		t.Fatal("queue failure must be visible")
	}
	for _, misleading := range []string{"Consulta concluída: fila vazia", "team-whatsapp-queue__metrics"} {
		if strings.Contains(markup, misleading) {
			t.Fatalf("failed request incorrectly displays %q", misleading)
		}
	}
}
