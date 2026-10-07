package supabase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

type PushSubscriptionRecord struct {
	ObjectPath string
	Data       json.RawMessage
}

type NativePushTokenRecord struct {
	ObjectPath string
	Platform   string
	Token      string
}

func (c *Client) StorePushSubscription(ctx context.Context, userID string, subscription json.RawMessage) error {
	var wire struct {
		Endpoint string `json:"endpoint"`
	}
	if json.Unmarshal(subscription, &wire) != nil || !validPushEndpoint(wire.Endpoint) {
		return errors.New("invalid push endpoint")
	}
	path, err := pushSubscriptionPath(userID, wire.Endpoint)
	if err != nil {
		return err
	}
	data, err := json.Marshal(map[string]any{"subscription": json.RawMessage(subscription), "updatedAt": time.Now().UTC().Format(time.RFC3339Nano)})
	if err != nil {
		return err
	}
	return c.UpsertPrivateObject(ctx, path, "application/json", data)
}

func (c *Client) DeleteNativePushToken(ctx context.Context, userID, platform, token string) error {
	if _, err := uuid.Parse(userID); err != nil {
		return errors.New("invalid push subscription owner")
	}
	platform = strings.ToLower(strings.TrimSpace(platform))
	if platform != "ios" && platform != "android" {
		return errors.New("invalid native push platform")
	}
	if strings.ContainsAny(token, "\r\n\x00") {
		return errors.New("invalid native push token")
	}
	token = strings.TrimSpace(token)
	if len(token) < 16 || len(token) > 4096 {
		return errors.New("invalid native push token")
	}
	digest := sha256.Sum256([]byte(platform + ":" + token))
	return c.DeletePushSubscription(ctx, "push-native/"+userID+"/"+hex.EncodeToString(digest[:])+".json")
}

// StoreNativePushToken stores an FCM/APNs token separately from Web Push
// subscriptions so the VAPID sender never mistakes a native token for an endpoint.
func (c *Client) StoreNativePushToken(ctx context.Context, userID, platform, token string) error {
	if _, err := uuid.Parse(userID); err != nil {
		return errors.New("invalid push subscription owner")
	}
	platform = strings.ToLower(strings.TrimSpace(platform))
	if platform != "ios" && platform != "android" {
		return errors.New("invalid native push platform")
	}
	if strings.ContainsAny(token, "\r\n\x00") {
		return errors.New("invalid native push token")
	}
	token = strings.TrimSpace(token)
	if len(token) < 16 || len(token) > 4096 {
		return errors.New("invalid native push token")
	}
	digest := sha256.Sum256([]byte(platform + ":" + token))
	path := "push-native/" + userID + "/" + hex.EncodeToString(digest[:]) + ".json"
	data, err := json.Marshal(map[string]any{"platform": platform, "token": token, "updatedAt": time.Now().UTC().Format(time.RFC3339Nano)})
	if err != nil {
		return err
	}
	return c.UpsertPrivateObject(ctx, path, "application/json", data)
}

func (c *Client) PushSubscriptionExists(ctx context.Context, userID, endpoint string) (bool, error) {
	path, err := pushSubscriptionPath(userID, endpoint)
	if err != nil {
		return false, err
	}
	resourcePath, err := c.storageResourcePath("object", "documentos-inovar", path)
	if err != nil {
		return false, err
	}
	result, err := c.ServiceRequest(ctx, resourcePath, RequestOptions{Method: http.MethodGet})
	if err != nil {
		return false, err
	}
	if result.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if result.StatusCode != http.StatusOK {
		return false, fmt.Errorf("read push subscription returned status %d", result.StatusCode)
	}
	return true, nil
}

func (c *Client) DeletePushSubscriptionForEndpoint(ctx context.Context, userID, endpoint string) error {
	path, err := pushSubscriptionPath(userID, endpoint)
	if err != nil {
		return err
	}
	return c.DeletePushSubscription(ctx, path)
}

func pushSubscriptionPath(userID, endpoint string) (string, error) {
	if _, err := uuid.Parse(userID); err != nil {
		return "", errors.New("invalid push subscription owner")
	}
	if !validPushEndpoint(endpoint) {
		return "", errors.New("invalid push endpoint")
	}
	digest := sha256.Sum256([]byte(endpoint))
	return "push/" + userID + "/" + hex.EncodeToString(digest[:]) + ".json", nil
}

func (c *Client) ListPushSubscriptions(ctx context.Context, userID string) ([]PushSubscriptionRecord, error) {
	if _, err := uuid.Parse(userID); err != nil {
		return nil, errors.New("invalid push subscription owner")
	}
	prefix := "push/" + userID
	objects, err := c.ListPrivateObjects(ctx, prefix+"/", 100)
	if err != nil {
		return nil, err
	}
	result := make([]PushSubscriptionRecord, 0, len(objects))
	for _, object := range objects {
		name := object.Name
		if strings.Contains(name, "/") {
			name = name[strings.LastIndex(name, "/")+1:]
		}
		path := prefix + "/" + name
		if !validPushObjectPath(path) {
			continue
		}
		resourcePath, err := c.storageResourcePath("object", "documentos-inovar", path)
		if err != nil {
			continue
		}
		stored, err := c.ServiceRequest(ctx, resourcePath, RequestOptions{Method: http.MethodGet})
		if err != nil {
			return nil, err
		}
		if stored.StatusCode == http.StatusNotFound {
			continue
		}
		if stored.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("read push subscription returned status %d", stored.StatusCode)
		}
		var value struct {
			Subscription json.RawMessage `json:"subscription"`
		}
		if json.Unmarshal(stored.Body, &value) != nil || len(value.Subscription) == 0 {
			continue
		}
		result = append(result, PushSubscriptionRecord{ObjectPath: path, Data: value.Subscription})
	}
	return result, nil
}

func (c *Client) ListNativePushTokens(ctx context.Context, userID string) ([]NativePushTokenRecord, error) {
	if _, err := uuid.Parse(userID); err != nil {
		return nil, errors.New("invalid push subscription owner")
	}
	prefix := "push-native/" + userID
	objects, err := c.ListPrivateObjects(ctx, prefix+"/", 100)
	if err != nil {
		return nil, err
	}
	result := make([]NativePushTokenRecord, 0, len(objects))
	for _, object := range objects {
		name := object.Name
		if strings.Contains(name, "/") {
			name = name[strings.LastIndex(name, "/")+1:]
		}
		path := prefix + "/" + name
		if !validNativePushObjectPath(path) {
			continue
		}
		resourcePath, err := c.storageResourcePath("object", "documentos-inovar", path)
		if err != nil {
			continue
		}
		stored, err := c.ServiceRequest(ctx, resourcePath, RequestOptions{Method: http.MethodGet})
		if err != nil {
			return nil, err
		}
		if stored.StatusCode == http.StatusNotFound {
			continue
		}
		if stored.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("read native push token returned status %d", stored.StatusCode)
		}
		var value struct {
			Platform string `json:"platform"`
			Token    string `json:"token"`
		}
		if json.Unmarshal(stored.Body, &value) != nil {
			continue
		}
		value.Platform = strings.ToLower(strings.TrimSpace(value.Platform))
		value.Token = strings.TrimSpace(value.Token)
		if (value.Platform != "android" && value.Platform != "ios") || len(value.Token) < 16 || len(value.Token) > 4096 || strings.ContainsAny(value.Token, "\r\n\x00") {
			continue
		}
		result = append(result, NativePushTokenRecord{ObjectPath: path, Platform: value.Platform, Token: value.Token})
	}
	return result, nil
}

func (c *Client) DeletePushSubscription(ctx context.Context, objectPath string) error {
	if !validPushObjectPath(objectPath) && !validNativePushObjectPath(objectPath) {
		return errors.New("invalid push subscription path")
	}
	target, err := c.storageObjectURL("object", "documentos-inovar", objectPath)
	if err != nil {
		return err
	}
	parsed, err := url.Parse(target)
	if err != nil {
		return err
	}
	resourcePath := parsed.EscapedPath()
	if parsed.RawQuery != "" {
		resourcePath += "?" + parsed.RawQuery
	}
	result, err := c.ServiceRequest(ctx, resourcePath, RequestOptions{Method: http.MethodDelete})
	if err != nil {
		return err
	}
	if result.StatusCode != http.StatusOK && result.StatusCode != http.StatusNotFound {
		return fmt.Errorf("delete push subscription returned status %d", result.StatusCode)
	}
	return nil
}

func validPushEndpoint(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil && parsed.Fragment == ""
}
