package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const desktopOAuthScheme = "inovarapp-desktop"

type DesktopApp struct {
	mu         sync.Mutex
	ctx        context.Context
	pendingURL string
}

func (a *DesktopApp) startup(ctx context.Context) {
	a.mu.Lock()
	a.ctx = ctx
	a.mu.Unlock()
	if screens, err := runtime.ScreenGetAll(ctx); err == nil {
		for _, screen := range screens {
			if !screen.IsCurrent {
				continue
			}
			width, height, minWidth, minHeight := desktopWindowSize(screen.Size.Width, screen.Size.Height)
			runtime.WindowSetMinSize(ctx, minWidth, minHeight)
			runtime.WindowSetSize(ctx, width, height)
			runtime.WindowCenter(ctx)
			break
		}
	} else {
		runtime.LogWarning(ctx, "Não foi possível ajustar o tamanho da janela ao monitor: "+err.Error())
	}
	if err := runtime.InitializeNotifications(ctx); err != nil {
		runtime.LogWarning(ctx, "Notificações nativas indisponíveis: "+err.Error())
	}
}

func (a *DesktopApp) domReady(ctx context.Context) {
	if raw := desktopOAuthArgument(os.Args[1:]); raw != "" {
		a.setPendingOAuthURL(raw)
	}
	if raw := a.takePendingOAuthURL(); raw != "" {
		runtime.EventsEmit(ctx, "desktop:oauth-url", raw)
	}
}

func (a *DesktopApp) OpenExternal(raw string) error {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return errors.New("somente URLs HTTPS podem ser abertas fora do aplicativo")
	}
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx == nil {
		return errors.New("aplicativo ainda não está pronto")
	}
	runtime.BrowserOpenURL(ctx, parsed.String())
	return nil
}

func (a *DesktopApp) Notify(title, body string) error {
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx == nil {
		return errors.New("aplicativo ainda não está pronto")
	}
	if !runtime.IsNotificationAvailable(ctx) {
		return errors.New("notificações não são suportadas neste sistema")
	}
	return runtime.SendNotification(ctx, runtime.NotificationOptions{ID: uuid.NewString(), Title: title, Body: body})
}

func (a *DesktopApp) SaveFile(filename, encoded string) (string, error) {
	if len(encoded) > 140<<20 {
		return "", errors.New("arquivo excede o limite de 100 MB")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", errors.New("conteúdo de arquivo inválido")
	}
	if len(data) > 100<<20 {
		return "", errors.New("arquivo excede o limite de 100 MB")
	}
	filename = filepath.Base(strings.TrimSpace(filename))
	if filename == "." || filename == "" || strings.ContainsAny(filename, "\\/\x00") {
		return "", errors.New("nome de arquivo inválido")
	}
	filters := []runtime.FileFilter{}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".pdf":
		filters = append(filters, runtime.FileFilter{DisplayName: "Documento PDF (*.pdf)", Pattern: "*.pdf"})
	case ".json":
		filters = append(filters, runtime.FileFilter{DisplayName: "Backup JSON (*.json)", Pattern: "*.json"})
	default:
		return "", errors.New("tipo de arquivo não permitido")
	}
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx == nil {
		return "", errors.New("aplicativo ainda não está pronto")
	}
	path, err := runtime.SaveFileDialog(ctx, runtime.SaveDialogOptions{Title: "Salvar arquivo", DefaultFilename: filename, Filters: filters})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", errors.New("salvamento cancelado")
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("não foi possível salvar o arquivo: %w", err)
	}
	return path, nil
}

func (a *DesktopApp) PendingOAuthURL() string { return a.takePendingOAuthURL() }

func (a *DesktopApp) setPendingOAuthURL(raw string) {
	if !isDesktopOAuthURL(raw) {
		return
	}
	a.mu.Lock()
	a.pendingURL = raw
	ctx := a.ctx
	a.mu.Unlock()
	if ctx != nil {
		runtime.EventsEmit(ctx, "desktop:oauth-url", raw)
	}
}

func (a *DesktopApp) takePendingOAuthURL() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	raw := a.pendingURL
	a.pendingURL = ""
	return raw
}

func desktopOAuthArgument(args []string) string {
	for _, arg := range args {
		if isDesktopOAuthURL(arg) {
			return arg
		}
	}
	return ""
}

func isDesktopOAuthURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && strings.EqualFold(parsed.Scheme, desktopOAuthScheme) && strings.EqualFold(parsed.Host, "oauth-callback")
}
