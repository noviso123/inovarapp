package main

import (
	"log"
	"os"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"inovarapp/core/ui/webapp"
)

var bundledSupabaseURL string
var bundledSupabaseAnonKey string
var bundledAPIBaseURL string

func main() {
	if value := strings.TrimSpace(bundledSupabaseURL); value != "" {
		_ = os.Setenv("SUPABASE_URL", value)
	}
	if value := strings.TrimSpace(bundledSupabaseAnonKey); value != "" {
		_ = os.Setenv("SUPABASE_ANON_KEY", value)
	}
	if value := strings.TrimSpace(bundledAPIBaseURL); value != "" {
		_ = os.Setenv("GOAPP_API_BASE_URL", value)
	}
	if err := setDesktopWorkingDirectory(); err != nil {
		log.Fatalf("não foi possível abrir a pasta do aplicativo: %v", err)
	}
	desktop := &DesktopApp{}
	address, shutdown, err := webapp.StartLocalServer()
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = shutdown() }()
	if err := wails.Run(&options.App{
		Title: "InovarApp • Inovar Refrigeração", Width: 1180, Height: 760,
		MinWidth: 600, MinHeight: 400,
		AssetServer: &assetserver.Options{Handler: webapp.LocalHandler(address)},
		Bind:        []interface{}{desktop}, OnStartup: desktop.startup, OnDomReady: desktop.domReady,
		Mac: &mac.Options{OnUrlOpen: desktop.setPendingOAuthURL},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "com.inovarapp.desktop",
			OnSecondInstanceLaunch: func(data options.SecondInstanceData) { desktop.setPendingOAuthURL(desktopOAuthArgument(data.Args)) },
		},
	}); err != nil {
		log.Fatal(err)
	}
}
