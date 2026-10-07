package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"inovarapp/core/adapter/push"
	"inovarapp/core/adapter/supabase"
	"inovarapp/core/adapter/webpush"
)

type PushNotifier interface {
	NotifyTeam(ctx context.Context, title, body string)
}

// TeamPushNotifier sends a best-effort alert to the authenticated app's team
// peers after customer-originated state changes.
type TeamPushNotifier struct {
	Supabase      *supabase.Client
	WebSender     webpush.Sender
	AndroidSender AndroidPushSender
	IOSSender     IOSPushSender
}

func (n TeamPushNotifier) NotifyTeam(ctx context.Context, title, body string) {
	if n.Supabase == nil {
		return
	}
	webConfigured := strings.TrimSpace(n.WebSender.PublicKey) != "" && strings.TrimSpace(n.WebSender.PrivateKey) != ""
	if !webConfigured && n.AndroidSender == nil && n.IOSSender == nil {
		return
	}
	users, err := n.Supabase.ListTeamProfileIDs(ctx)
	if err != nil {
		return
	}
	payload := webpush.Payload{Title: truncateUTF8(title, 120), Body: truncateUTF8(body, 400), URL: "/", Path: "/", Tag: "inovar-" + time.Now().UTC().Format("20060102150405.000000000")}
	if payload.Title == "" {
		payload.Title = "InovarApp"
	}
	for _, userID := range users {
		if webConfigured {
			if subscriptions, err := n.Supabase.ListPushSubscriptions(ctx, userID); err == nil {
				for _, item := range subscriptions {
					var subscription webpush.Subscription
					if json.Unmarshal(item.Data, &subscription) != nil {
						continue
					}
					if err := n.WebSender.Send(ctx, subscription, payload); errors.Is(err, webpush.ErrSubscriptionExpired) {
						_ = n.Supabase.DeletePushSubscription(ctx, item.ObjectPath)
					}
				}
			}
		}
		if n.AndroidSender == nil && n.IOSSender == nil {
			continue
		}
		tokens, err := n.Supabase.ListNativePushTokens(ctx, userID)
		if err != nil {
			continue
		}
		for _, token := range tokens {
			var sendErr error
			switch token.Platform {
			case "android":
				if n.AndroidSender != nil {
					sendErr = n.AndroidSender.SendAndroid(ctx, token.Token, payload.Title, payload.Body, payload.Path)
				}
			case "ios":
				if n.IOSSender != nil {
					sendErr = n.IOSSender.Send(ctx, token.Token, payload.Title, payload.Body, payload.Path)
				}
			}
			if errors.Is(sendErr, push.ErrUnregistered) {
				_ = n.Supabase.DeletePushSubscription(ctx, token.ObjectPath)
			}
		}
	}
}
