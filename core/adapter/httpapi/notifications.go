package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"inovarapp/core/adapter/push"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/adapter/webpush"
)

type NotificationsHandler struct {
	Supabase      *supabase.Client
	Sender        webpush.Sender
	AndroidSender AndroidPushSender
	IOSSender     IOSPushSender
}

type AndroidPushSender interface {
	SendAndroid(ctx context.Context, token, title, body, path string) error
}

type IOSPushSender interface {
	Send(ctx context.Context, token, title, body, path string) error
}

func (h NotificationsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeResourceJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "Método não permitido"})
		return
	}
	if h.Supabase == nil {
		writeResourceJSON(w, http.StatusInternalServerError, map[string]string{"error": "Backend não configurado"})
		return
	}
	caller, err := h.Supabase.AuthenticateCaller(r.Context(), supabase.BearerToken(r.Header.Get("Authorization")))
	if err != nil {
		writeResourceJSON(w, http.StatusUnauthorized, map[string]string{"error": "Entre na sua conta para ativar notificações."})
		return
	}
	defer r.Body.Close()
	var input struct {
		Action       string          `json:"acao"`
		Subscription json.RawMessage `json:"subscription"`
		Platform     string          `json:"platform"`
		Token        string          `json:"token"`
		Endpoint     string          `json:"endpoint"`
		Title        string          `json:"titulo"`
		Text         string          `json:"texto"`
		URL          string          `json:"url"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&input); err != nil {
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Solicitação inválida."})
		return
	}
	switch input.Action {
	case "inscrever-nativo":
		if err := h.Supabase.StoreNativePushToken(r.Context(), caller.UserID, input.Platform, input.Token); err != nil {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Token de notificação nativa inválido."})
			return
		}
		platform := strings.ToLower(strings.TrimSpace(input.Platform))
		deliveryConfigured := (platform == "android" && h.AndroidSender != nil) || (platform == "ios" && h.IOSSender != nil)
		writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true, "registered": true, "deliveryConfigured": deliveryConfigured})
	case "remover-nativo":
		if err := h.Supabase.DeleteNativePushToken(r.Context(), caller.UserID, input.Platform, input.Token); err != nil {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Não foi possível remover o registro nativo de notificações."})
			return
		}
		writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true})
	case "inscrever":
		var subscription webpush.Subscription
		if json.Unmarshal(input.Subscription, &subscription) != nil || webpush.ValidateSubscription(subscription) != nil {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Inscrição de notificação inválida."})
			return
		}
		if err := h.Supabase.StorePushSubscription(r.Context(), caller.UserID, input.Subscription); err != nil {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Não foi possível registrar este dispositivo para notificações."})
			return
		}
		writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true})
	case "estado", "remover":
		var subscription webpush.Subscription
		if json.Unmarshal(input.Subscription, &subscription) != nil || subscription.Endpoint == "" {
			writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Inscrição de notificação inválida."})
			return
		}
		if input.Action == "remover" {
			if err := h.Supabase.DeletePushSubscriptionForEndpoint(r.Context(), caller.UserID, subscription.Endpoint); err != nil {
				writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível remover a inscrição de notificações."})
				return
			}
			writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}
		active, err := h.Supabase.PushSubscriptionExists(r.Context(), caller.UserID, subscription.Endpoint)
		if err != nil {
			writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível consultar a inscrição de notificações."})
			return
		}
		writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true, "active": active})
	case "enviar":
		payload := webpush.Payload{Title: truncateUTF8(input.Title, 120), Body: truncateUTF8(input.Text, 400), URL: input.URL, Path: input.URL, Tag: "inovar-" + strconv.FormatInt(time.Now().UnixMilli(), 10)}
		if payload.Title == "" {
			payload.Title = "InovarApp"
		}
		if payload.Body == "" {
			payload.Body = "Há uma atualização no aplicativo."
		}
		if !strings.HasPrefix(payload.Path, "/") {
			payload.Path = "/"
			payload.URL = "/"
		}
		sent, expired, nativeSent, nativeExpired := 0, 0, 0, 0
		webPushConfigured := strings.TrimSpace(h.Sender.PrivateKey) != "" && strings.TrimSpace(h.Sender.PublicKey) != ""
		if webPushConfigured {
			subscriptions, err := h.Supabase.ListPushSubscriptions(r.Context(), caller.UserID)
			if err != nil {
				writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível enviar a notificação agora."})
				return
			}
			for _, item := range subscriptions {
				var subscription webpush.Subscription
				if json.Unmarshal(item.Data, &subscription) != nil {
					continue
				}
				if err := h.Sender.Send(r.Context(), subscription, payload); err == nil {
					sent++
				} else if err == webpush.ErrSubscriptionExpired {
					if h.Supabase.DeletePushSubscription(r.Context(), item.ObjectPath) == nil {
						expired++
					}
				}
			}
		}
		if h.AndroidSender != nil || h.IOSSender != nil {
			tokens, err := h.Supabase.ListNativePushTokens(r.Context(), caller.UserID)
			if err != nil {
				writeResourceJSON(w, http.StatusBadGateway, map[string]string{"error": "Não foi possível enviar a notificação nativa agora."})
				return
			}
			for _, token := range tokens {
				var sendErr error
				switch token.Platform {
				case "android":
					if h.AndroidSender == nil {
						continue
					}
					sendErr = h.AndroidSender.SendAndroid(r.Context(), token.Token, payload.Title, payload.Body, payload.Path)
				case "ios":
					if h.IOSSender == nil {
						continue
					}
					sendErr = h.IOSSender.Send(r.Context(), token.Token, payload.Title, payload.Body, payload.Path)
				default:
					continue
				}
				if sendErr == nil {
					nativeSent++
				} else if errors.Is(sendErr, push.ErrUnregistered) {
					if h.Supabase.DeletePushSubscription(r.Context(), token.ObjectPath) == nil {
						nativeExpired++
					}
				}
			}
		}
		writeResourceJSON(w, http.StatusOK, map[string]any{"ok": true, "sent": sent, "expired": expired, "nativeSent": nativeSent, "nativeExpired": nativeExpired, "skipped": !webPushConfigured && h.AndroidSender == nil && h.IOSSender == nil})
	default:
		writeResourceJSON(w, http.StatusBadRequest, map[string]string{"error": "Ação inválida."})
	}
}

func truncateUTF8(value string, maxRunes int) string {
	value = strings.ToValidUTF8(value, "�")
	runes := []rune(value)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes])
	}
	return value
}
