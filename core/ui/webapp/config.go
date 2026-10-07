package webapp

import (
	"bufio"
	"os"
	"strconv"
	"strings"

	"inovarapp/core/adapter/supabase"
	"inovarapp/core/adapter/whatsapp"
)

// publicSupabaseEnvironment exposes browser-safe configuration only. Server
// credentials, including service-role and VAPID private keys, never reach PWA HTML.
func publicSupabaseEnvironment() map[string]string {
	values := map[string]string{}
	if value := configuredValue("GOAPP_API_BASE_URL", "API_BASE_URL", "VITE_API_BASE_URL"); value != "" {
		values["API_BASE_URL"] = value
	}
	if value := configuredValue("SUPABASE_URL", "VITE_SUPABASE_URL"); value != "" {
		values["SUPABASE_URL"] = value
	}
	if value := configuredValue("SUPABASE_ANON_KEY", "SUPABASE_PUBLISHABLE_KEY", "VITE_SUPABASE_ANON_KEY"); value != "" {
		values["SUPABASE_ANON_KEY"] = value
	}
	if value := configuredValue("VAPID_PUBLIC_KEY"); value != "" {
		values["VAPID_PUBLIC_KEY"] = value
	}
	return values
}

// serverSupabaseClient can read local .env files for development. Only its
// public URL and anon key reach app.Handler.Env; the service-role value stays
// inside this server-side client.
func serverSupabaseClient() (*supabase.Client, error) {
	return supabase.New(supabase.Config{
		URL:            configuredValue("SUPABASE_URL", "VITE_SUPABASE_URL"),
		AnonKey:        configuredValue("SUPABASE_ANON_KEY", "SUPABASE_PUBLISHABLE_KEY", "VITE_SUPABASE_ANON_KEY"),
		ServiceRoleKey: configuredValue("SUPABASE_SERVICE_ROLE_KEY", "SUPABASE_SECRET_KEY"),
	})
}

func serverWhatsAppConfig() whatsapp.Config {
	session := configuredValue("WHATSAPP_OWN_SESSION")
	if session == "" {
		session = "inovar"
	}
	return whatsapp.Config{
		ProprioURL:     configuredValue("WHATSAPP_OWN_URL"),
		ProprioToken:   configuredValue("WHATSAPP_OWN_TOKEN"),
		ProprioSession: session,
	}
}

func configuredInt(name string, fallback int) int {
	value := configuredValue(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func configuredValue(names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value
		}
	}
	for _, filename := range []string{".env.local", ".env"} {
		for _, name := range names {
			if value := readDotEnvValue(filename, name); value != "" {
				return value
			}
		}
	}
	return ""
}

func readDotEnvValue(filename, wanted string) string {
	file, err := os.Open(filename)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(name) != wanted {
			continue
		}
		return dotenvScalar(strings.TrimSpace(value))
	}
	return ""
}

func dotenvScalar(value string) string {
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		if decoded, err := strconv.Unquote(value); err == nil {
			return decoded
		}
		return value[1 : len(value)-1]
	}
	if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
		return value[1 : len(value)-1]
	}
	if index := strings.Index(value, " #"); index >= 0 {
		value = strings.TrimSpace(value[:index])
	}
	return value
}

