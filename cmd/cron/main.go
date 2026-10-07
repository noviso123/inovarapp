package main

import (
	"bufio"
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"inovarapp/core/adapter/alertcron"
	mailadapter "inovarapp/core/adapter/email"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/adapter/webpush"
	"inovarapp/core/adapter/whatsapp"
)

func main() {
	loadLocalEnv()
	client, err := supabase.FromEnv()
	if err != nil {
		log.Fatalf("configuração do Supabase inválida: %v", err)
	}
	if err := client.RequireServiceRole(); err != nil {
		log.Fatal("SUPABASE_SERVICE_ROLE_KEY é obrigatório para o worker de cron")
	}
	location, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		location = time.FixedZone("BRT", -3*60*60)
	}
	worker := alertcron.Handler{
		Supabase:         client,
		Months:           envInt("ALERTA_MESES", 6),
		MaxLateDays:      envInt("ALERTA_MAX_ATRASO_DIAS", 180),
		WhatsAppDefaults: whatsapp.Config{ProprioURL: env("WHATSAPP_OWN_URL"), ProprioToken: env("WHATSAPP_OWN_TOKEN"), ProprioSession: envDefault("WHATSAPP_OWN_SESSION", "inovar")},
		EmailDefaults:    mailadapter.Config{APIKey: env("EMAIL_API_KEY"), From: env("EMAIL_FROM"), Username: env("EMAIL_GMAIL_USER"), Password: env("EMAIL_GMAIL_PASS")},
		PushDefaults:     webpush.Sender{PublicKey: env("VAPID_PUBLIC_KEY"), PrivateKey: env("VAPID_PRIVATE_KEY"), Subject: env("VAPID_SUBJECT")},
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Printf("worker Go de alertas iniciado; diário às 09:00 e push de agenda a cada hora (%s)", location)
	err = worker.RunScheduled(ctx, time.Now, location, func(job string, result map[string]any, err error) {
		if err != nil {
			log.Printf("cron %s falhou: %v", job, err)
			return
		}
		log.Printf("cron %s concluiu (ciclos=%v agenda=%v push=%v)", job, result["ciclosVencidos"], result["lembretesAgenda"], result["lembretesUmaHora"])
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func env(name string) string { return strings.TrimSpace(os.Getenv(name)) }
func envDefault(name, fallback string) string {
	if value := env(name); value != "" {
		return value
	}
	return fallback
}
func envInt(name string, fallback int) int {
	value := env(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func loadLocalEnv() {
	for _, filename := range []string{".env.local", ".env"} {
		file, err := os.Open(filename)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(scanner.Text()), "export "))
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			name, value, ok := strings.Cut(line, "=")
			name, value = strings.TrimSpace(name), strings.TrimSpace(value)
			if !ok || name == "" || env(name) != "" {
				continue
			}
			if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
				if decoded, decodeErr := strconv.Unquote(value); decodeErr == nil {
					value = decoded
				} else {
					value = value[1 : len(value)-1]
				}
			} else if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
				value = value[1 : len(value)-1]
			}
			_ = os.Setenv(name, value)
		}
		_ = file.Close()
	}
}
