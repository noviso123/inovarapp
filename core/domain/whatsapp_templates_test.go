package domain

import (
	"regexp"
	"strings"
	"testing"
)

func TestWhatsAppTemplateCatalogIsCompleteAndHasUniqueKeys(t *testing.T) {
	templates := WhatsAppTemplates()
	if len(templates) != 20 {
		t.Fatalf("template count=%d", len(templates))
	}
	seen := make(map[string]bool, len(templates))
	placeholderPattern := regexp.MustCompile(`\{\{\w+\}\}`)
	values := map[string]string{"cliente": "Maria", "email": "maria@example.com", "senha": "123456", "empresa": "Inovar", "numero": "1", "os": "OS-1", "servico": "Limpeza", "data": "10/10/2026", "hora": "09:00", "valor": "R$ 250", "garantia": "90 dias", "app": AppPublicURL, "equipamento": "LG", "data_ultima": "01/01/2026", "situacao": "atraso", "meses": "6", "status": "CONCLUÍDO"}
	for _, model := range templates {
		if model.Key == "" || model.Title == "" || model.Default == "" || seen[model.Key] {
			t.Fatalf("invalid/duplicate WhatsApp template: %+v", model)
		}
		seen[model.Key] = true
		declared := make(map[string]bool, len(model.Placeholders))
		for _, placeholder := range model.Placeholders {
			declared[placeholder] = true
		}
		for _, placeholder := range placeholderPattern.FindAllString(model.Default, -1) {
			if !declared[placeholder] {
				t.Errorf("%s uses undeclared placeholder %s", model.Key, placeholder)
			}
		}
		preview := ApplyWhatsAppPlaceholders(model.Default, values)
		if strings.Contains(preview, "{{") {
			t.Errorf("%s preview has unexpanded placeholder: %s", model.Key, preview)
		}
	}
}

func TestResolveWhatsAppTemplateUsesCustomTextAndFallsBackForBlank(t *testing.T) {
	model, ok := WhatsAppTemplateByKey("status_cancelado")
	if !ok {
		t.Fatal("status_cancelado template missing")
	}
	if got := ResolveWhatsAppTemplate(map[string]string{"status_cancelado": " Personalizada {{cliente}} "}, model.Key); got != " Personalizada {{cliente}} " {
		t.Fatalf("custom text=%q", got)
	}
	if got := ResolveWhatsAppTemplate(map[string]string{"status_cancelado": "  "}, model.Key); got != model.Default {
		t.Fatalf("fallback=%q", got)
	}
	if got := ApplyWhatsAppPlaceholders("Olá {{cliente}} {{desconhecido}}!", map[string]string{"cliente": "João"}); got != "Olá João !" {
		t.Fatalf("placeholder output=%q", got)
	}
}
