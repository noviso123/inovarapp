package webapp

import (
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
	"inovarapp/core/domain"
)

func TestTeamSettingsFormMapsExistingTechnicianProfile(t *testing.T) {
	signature := "/inovar-brand/signature.png"
	profile := domain.TechnicianProfile{
		Name: "Gabriel", BusinessName: "Inovar Refrigeração", CNPJ: "36.020.014/0001-14", Address: "Serra",
		Phone: "27999999999", PIXKey: "pix@example.com", PIXType: domain.PIXEmail,
		DefaultPrice: 275.5, DefaultReturnMonths: 6, DefaultWarrantyDays: 90, Signature: &signature,
	}
	form := newTeamSettingsForm(profile)
	if form.Name != profile.Name || form.BusinessName != profile.BusinessName || form.CNPJ != profile.CNPJ || form.Address != profile.Address || form.Phone != profile.Phone {
		t.Fatalf("identity fields were not mapped: %+v", form)
	}
	if form.PIXKey != profile.PIXKey || form.PIXType != string(profile.PIXType) || form.DefaultPrice != "275.5" || form.ReturnMonths != "6" || form.WarrantyDays != "90" {
		t.Fatalf("billing/default fields were not mapped: %+v", form)
	}
}

func TestMergeTechnicianProfileRetainsReadOnlySignature(t *testing.T) {
	signature := "/inovar-brand/signature.png"
	current := domain.TechnicianProfile{Signature: &signature, Name: "Before"}
	returned := domain.TechnicianProfile{Name: "After"}
	merged := mergeTechnicianProfile(current, returned)
	if merged.Name != "After" || merged.Signature == nil || *merged.Signature != signature {
		t.Fatalf("read-only signature or returned values lost: %+v", merged)
	}
}

func TestTeamSettingsDialogRequiresProfessionalNameAndWhatsApp(t *testing.T) {
	page := &serviceCatalogPage{teamSettingsForm: &teamSettingsForm{PIXType: "email"}}
	markup := app.HTMLString(page.teamSettingsDialog())
	for _, label := range []string{"Seu Nome Profissional *", "WhatsApp de Contato"} {
		if !strings.Contains(markup, label) {
			t.Fatalf("settings form missing %q: %s", label, markup)
		}
	}
	if strings.Count(markup, "required") < 2 {
		t.Fatalf("name and phone inputs must both be required: %s", markup)
	}
}

func TestTeamWhatsAppSettingsHidesConnectActionsWhenBackendIsNotConfigured(t *testing.T) {
	page := &serviceCatalogPage{
		teamWhatsAppStatus: map[string]any{"configurado": false, "conectado": false},
		teamWhatsAppNotice: "Serviço WhatsApp não configurado",
	}
	markup := app.HTMLString(page.teamWhatsAppSettings())
	for _, action := range []string{"Conectar pelo QR Code", "Conectar por código"} {
		if strings.Contains(markup, action) {
			t.Fatalf("unavailable backend must not offer %q: %s", action, markup)
		}
	}
	if !strings.Contains(markup, "Conexão indisponível") {
		t.Fatalf("missing unavailable state in settings: %s", markup)
	}
}

func TestTeamWhatsAppSettingsDoesNotShowUnavailableErrorWhileChecking(t *testing.T) {
	page := &serviceCatalogPage{
		teamWhatsAppBusy:   true,
		teamWhatsAppStatus: map[string]any{"configurado": false, "conectado": false},
		teamWhatsAppNotice: "Verificando conexão do WhatsApp...",
	}
	markup := app.HTMLString(page.teamWhatsAppSettings())
	if !strings.Contains(markup, "Verificando conexão...") || !strings.Contains(markup, "Verificando conexão do WhatsApp...") {
		t.Fatalf("settings must show the in-progress state: %s", markup)
	}
	if strings.Contains(markup, "Conexão indisponível") {
		t.Fatalf("a pending status request must not appear as an unavailable backend: %s", markup)
	}
}

func TestTeamWhatsAppSettingsOffersPairingAfterBackendIsConfigured(t *testing.T) {
	page := &serviceCatalogPage{teamWhatsAppStatus: map[string]any{"configurado": true, "conectado": false}}
	markup := app.HTMLString(page.teamWhatsAppSettings())
	for _, action := range []string{"Conectar pelo QR Code", "Conectar por código", "Número da conta WhatsApp"} {
		if !strings.Contains(markup, action) {
			t.Fatalf("configured backend UI missing %q: %s", action, markup)
		}
	}
}

func TestTeamWhatsAppSettingsShowsPhoneCodeAndCopyAction(t *testing.T) {
	page := &serviceCatalogPage{
		teamWhatsAppStatus:  map[string]any{"configurado": true, "conectado": false},
		teamWhatsAppPairing: "ABCD-1234",
	}
	markup := app.HTMLString(page.teamWhatsAppSettings())
	for _, expected := range []string{"ABCD-1234", "Copiar código", "Conectar com número de telefone"} {
		if !strings.Contains(markup, expected) {
			t.Fatalf("phone pairing UI missing %q: %s", expected, markup)
		}
	}
}
