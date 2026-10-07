package webapp

import "testing"

func TestPublicAppEnvironmentExposesOnlyBrowserSafePushConfiguration(t *testing.T) {
	t.Setenv("GOAPP_API_BASE_URL", "https://api.example.com")
	t.Setenv("SUPABASE_URL", "https://example.supabase.co")
	t.Setenv("SUPABASE_ANON_KEY", "public-anon")
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "private-service-role")
	t.Setenv("VAPID_PUBLIC_KEY", "public-vapid")
	t.Setenv("VAPID_PRIVATE_KEY", "private-vapid")
	values := publicSupabaseEnvironment()
	if values["VAPID_PUBLIC_KEY"] != "public-vapid" || values["SUPABASE_ANON_KEY"] != "public-anon" {
		t.Fatalf("browser-safe config=%v", values)
	}
	if values["API_BASE_URL"] != "https://api.example.com" {
		t.Fatalf("public API base URL missing from app runtime config: %v", values)
	}
	if _, ok := values["VAPID_PRIVATE_KEY"]; ok {
		t.Fatal("VAPID private key leaked to PWA configuration")
	}
	if _, ok := values["SUPABASE_SERVICE_ROLE_KEY"]; ok {
		t.Fatal("service-role key leaked to PWA configuration")
	}
}
