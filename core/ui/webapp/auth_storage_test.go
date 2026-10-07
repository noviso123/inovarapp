package webapp

import "testing"

func TestSupabaseSessionStorageKeyMatchesJSClientDefault(t *testing.T) {
	if got, want := supabaseSessionStorageKeyForURL("https://ycpswioserctavijhnre.supabase.co"), "sb-ycpswioserctavijhnre-auth-token"; got != want {
		t.Fatalf("session storage key = %q, want %q", got, want)
	}
}

func TestSupabaseSessionStorageKeyHandlesURLPathAndInvalidConfig(t *testing.T) {
	if got, want := supabaseSessionStorageKeyForURL("https://project.example.test/custom/path"), "sb-project-auth-token"; got != want {
		t.Fatalf("session storage key with path = %q, want %q", got, want)
	}
	if got := supabaseSessionStorageKeyForURL("://bad-url"); got != legacySupabaseSessionStorageKey {
		t.Fatalf("invalid URL storage key = %q, want legacy fallback %q", got, legacySupabaseSessionStorageKey)
	}
}

func TestRecoverySubmitLabelShowsPendingState(t *testing.T) {
	if got := recoverySubmitLabel(true); got != "Salvando..." {
		t.Fatalf("busy label = %q, want Salvando...", got)
	}
	if got := recoverySubmitLabel(false); got != "Salvar nova senha" {
		t.Fatalf("idle label = %q, want Salvar nova senha", got)
	}
}
