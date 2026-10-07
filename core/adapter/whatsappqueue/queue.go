// Package whatsappqueue persists WhatsApp work in Supabase and retries it from
// a server-side cron worker. Provider acceptance is recorded as sent; this is
// an at-least-once queue because the current provider API has no idempotency
// token or end-device delivery receipt.
package whatsappqueue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"inovarapp/core/adapter/supabase"
	"inovarapp/core/adapter/whatsapp"
)

const tablePath = "/rest/v1/whatsapp_message_queue"

var ErrNotConfigured = errors.New("fila WhatsApp não configurada no Supabase")

type Message struct {
	ID                  string         `json:"id"`
	IdempotencyKey      string         `json:"idempotency_key"`
	EventType           string         `json:"event_type"`
	RecipientPhone      string         `json:"recipient_phone"`
	MessageText         string         `json:"message_text"`
	DocumentURL         string         `json:"document_url,omitempty"`
	DocumentStoragePath string         `json:"document_storage_path,omitempty"`
	DocumentName        string         `json:"document_name,omitempty"`
	ScheduledAt         time.Time      `json:"scheduled_at"`
	NextAttemptAt       time.Time      `json:"next_attempt_at"`
	ExpiresAt           *time.Time     `json:"expires_at,omitempty"`
	Status              string         `json:"status"`
	AttemptCount        int            `json:"attempt_count"`
	LastError           string         `json:"last_error,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
	SentAt              *time.Time     `json:"sent_at,omitempty"`
	Sensitive           bool           `json:"sensitive"`
	SourceEntityType    string         `json:"source_entity_type,omitempty"`
	SourceEntityID      string         `json:"source_entity_id,omitempty"`
	Metadata            map[string]any `json:"metadata,omitempty"`
}

type EnqueueInput struct {
	IdempotencyKey      string
	EventType           string
	RecipientPhone      string
	MessageText         string
	DocumentURL         string
	DocumentStoragePath string
	DocumentName        string
	ScheduledAt         time.Time
	ExpiresAt           *time.Time
	Sensitive           bool
	SourceEntityType    string
	SourceEntityID      string
	Metadata            map[string]any
}

// Enqueue stores work using an idempotency key. Repeated cron runs or client
// retries therefore return success without creating duplicate queue rows.
func Enqueue(ctx context.Context, db *supabase.Client, input EnqueueInput) error {
	if db == nil || db.RequireServiceRole() != nil {
		return ErrNotConfigured
	}
	key := strings.TrimSpace(input.IdempotencyKey)
	phone, valid := whatsapp.NormalizePhone(input.RecipientPhone)
	if key == "" || len(key) > 240 || !valid || strings.TrimSpace(input.MessageText) == "" && strings.TrimSpace(input.DocumentURL) == "" && strings.TrimSpace(input.DocumentStoragePath) == "" {
		return errors.New("dados da mensagem WhatsApp inválidos")
	}
	if input.DocumentURL != "" && input.DocumentStoragePath == "" {
		storagePath, ok := PrivateStoragePath(db, input.DocumentURL)
		if !ok {
			return errors.New("o documento precisa estar em um link assinado do armazenamento privado do Supabase")
		}
		input.DocumentStoragePath, input.DocumentURL = storagePath, ""
	}
	scheduled := input.ScheduledAt
	if scheduled.IsZero() {
		scheduled = time.Now().UTC()
	}
	metadata := input.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	payload := map[string]any{
		"idempotency_key":       key,
		"event_type":            strings.TrimSpace(input.EventType),
		"recipient_phone":       phone,
		"message_text":          input.MessageText,
		"document_url":          optionalString(input.DocumentURL),
		"document_storage_path": optionalString(input.DocumentStoragePath),
		"document_name":         optionalString(input.DocumentName),
		"scheduled_at":          scheduled.UTC().Format(time.RFC3339Nano),
		"next_attempt_at":       scheduled.UTC().Format(time.RFC3339Nano),
		"expires_at":            input.ExpiresAt,
		"status":                "pendente",
		"sensitive":             input.Sensitive,
		"source_entity_type":    optionalString(input.SourceEntityType),
		"source_entity_id":      optionalString(input.SourceEntityID),
		"metadata":              metadata,
	}
	path := tablePath + "?on_conflict=idempotency_key"
	var requestBody any = payload
	if input.EventType == "lembrete_manutencao_recorrente" {
		path = "/rest/v1/rpc/schedule_maintenance_whatsapp"
		requestBody = map[string]any{"p_payload": payload}
	}
	result, err := db.ServiceRequest(ctx, path, supabase.RequestOptions{
		Method: http.MethodPost, Body: requestBody,
		Prefer: "resolution=ignore-duplicates,return=minimal",
	})
	if err != nil {
		return err
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		return fmt.Errorf("Supabase recusou a fila WhatsApp (status %d)", result.StatusCode)
	}
	return nil
}

// PrivateStoragePath validates and extracts an object path from a signed URL
// belonging to this Supabase project's private documentos-inovar bucket.
func PrivateStoragePath(db *supabase.Client, signedURL string) (string, bool) {
	if db == nil {
		return "", false
	}
	base, err := url.Parse(db.BaseURL())
	if err != nil {
		return "", false
	}
	parsed, err := url.Parse(strings.TrimSpace(signedURL))
	if err != nil || parsed.Scheme != "https" && parsed.Scheme != "http" || !strings.EqualFold(parsed.Host, base.Host) {
		return "", false
	}
	const prefix = "/storage/v1/object/sign/documentos-inovar/"
	if !strings.HasPrefix(parsed.EscapedPath(), prefix) {
		return "", false
	}
	path, err := url.PathUnescape(strings.TrimPrefix(parsed.EscapedPath(), prefix))
	if err != nil || strings.TrimSpace(path) == "" || strings.Contains(path, "..") || strings.HasPrefix(path, "/") {
		return "", false
	}
	return path, true
}

func optionalString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

// CancelPendingBySource prevents reminders or documents tied to a cancelled,
// deleted, or rescheduled record from being delivered later.
func CancelPendingBySource(ctx context.Context, db *supabase.Client, sourceType, sourceID, reason string) error {
	if db == nil || db.RequireServiceRole() != nil {
		return ErrNotConfigured
	}
	if strings.TrimSpace(sourceType) == "" || strings.TrimSpace(sourceID) == "" {
		return nil
	}
	query := url.Values{
		"source_entity_type": {"eq." + sourceType},
		"source_entity_id":   {"eq." + sourceID},
		"status":             {"eq.pendente"},
	}
	result, err := db.ServiceRequest(ctx, tablePath+"?"+query.Encode(), supabase.RequestOptions{
		Method: http.MethodPatch,
		Body: map[string]any{
			"status": "cancelado", "last_error": reason,
			"message_text": "", "updated_at": time.Now().UTC(),
		}, Prefer: "return=minimal",
	})
	if err != nil {
		return err
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		return fmt.Errorf("não foi possível cancelar avisos pendentes (status %d)", result.StatusCode)
	}
	return nil
}

type ProcessResult struct {
	Claimed              int  `json:"claimed"`
	Skipped              int  `json:"skipped"`
	Sent                 int  `json:"sent"`
	Retried              int  `json:"retried"`
	Expired              int  `json:"expired"`
	WaitingForConnection bool `json:"waiting_for_connection"`
}

// ProcessDue claims messages atomically via a PostgreSQL RPC. A lease allows a
// later cron run to recover work if a serverless invocation is interrupted.
func ProcessDue(ctx context.Context, db *supabase.Client, sender whatsapp.Sender, config whatsapp.Config, now time.Time, limit int) (ProcessResult, error) {
	if db == nil || db.RequireServiceRole() != nil {
		return ProcessResult{}, ErrNotConfigured
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if limit < 1 || limit > 100 {
		limit = 25
	}
	if err := expireDue(ctx, db, now); err != nil {
		return ProcessResult{}, err
	}
	if !configured(config) || !providerConnected(ctx, config) {
		return ProcessResult{WaitingForConnection: true}, nil
	}
	result, err := db.ServiceRequest(ctx, "/rest/v1/rpc/claim_whatsapp_message_queue", supabase.RequestOptions{
		Method: http.MethodPost,
		Body:   map[string]any{"batch_size": limit, "lease_seconds": 150},
	})
	if err != nil {
		return ProcessResult{}, err
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		return ProcessResult{}, fmt.Errorf("não foi possível reservar mensagens da fila (status %d)", result.StatusCode)
	}
	var messages []Message
	if err := json.Unmarshal(result.Body, &messages); err != nil {
		return ProcessResult{}, fmt.Errorf("resposta inválida ao reservar mensagens: %w", err)
	}
	processed := ProcessResult{Claimed: len(messages)}
	if len(messages) == 0 {
		return processed, nil
	}
	for _, message := range messages {
		if message.Status != "processando" {
			continue
		}
		if message.ExpiresAt != nil && !message.ExpiresAt.After(now) {
			if _, err := recordAttempt(ctx, db, message, "falha", "mensagem expirou antes do envio", now, now, time.Time{}); err != nil {
				return processed, err
			}
			processed.Expired++
			continue
		}
		attemptAt := now.UTC()
		valid, deliveryError := validateReminder(ctx, db, message)
		if deliveryError == nil && !valid {
			processed.Skipped++
			continue
		}
		if deliveryError == nil {
			deliveryError = sendMessage(ctx, db, sender, config, message)
		}
		if deliveryError == nil {
			finishedAt := time.Now().UTC()
			status, err := recordAttempt(ctx, db, message, "enviado", "", attemptAt, finishedAt, time.Time{})
			if err != nil {
				return processed, err
			}
			if status == "enviado" {
				processed.Sent++
			}
			continue
		}
		failure := safeError(deliveryError)
		retryAt := now.UTC().Add(retryDelay(message.AttemptCount))
		status, err := recordAttempt(ctx, db, message, "falha", failure, attemptAt, time.Now().UTC(), retryAt)
		if err != nil {
			return processed, err
		}
		if status == "pendente" {
			processed.Retried++
		} else if status == "expirado" {
			processed.Expired++
		}
	}
	return processed, nil
}

func validateReminder(ctx context.Context, db *supabase.Client, message Message) (bool, error) {
	switch message.EventType {
	case "lembrete_manutencao_recorrente", "lembrete_agendamento_vespera", "lembrete_agendamento_uma_hora":
	default:
		return true, nil
	}
	result, err := db.ServiceRequest(ctx, "/rest/v1/rpc/validate_whatsapp_reminder", supabase.RequestOptions{
		Method: http.MethodPost,
		Body:   map[string]any{"p_message_id": message.ID, "p_attempt_number": message.AttemptCount},
	})
	if err != nil || result.StatusCode < 200 || result.StatusCode >= 300 {
		return false, errors.New("não foi possível validar o lembrete antes do envio; será tentado novamente")
	}
	var valid bool
	if json.Unmarshal(result.Body, &valid) != nil {
		return false, errors.New("resposta inválida ao validar o lembrete; será tentado novamente")
	}
	return valid, nil
}

func sendMessage(ctx context.Context, db *supabase.Client, sender whatsapp.Sender, config whatsapp.Config, message Message) error {
	text := message.MessageText
	if message.DocumentStoragePath != "" {
		signedURL, err := db.CreatePrivateSignedURL(ctx, message.DocumentStoragePath, 30*24*60*60)
		if err != nil {
			return fmt.Errorf("não foi possível renovar o link do documento: %w", err)
		}
		_, err = sender.SendDocumentWithResult(ctx, config, message.RecipientPhone, text, signedURL, message.DocumentName)
		return err
	}
	if message.DocumentURL != "" {
		_, err := sender.SendDocumentWithResult(ctx, config, message.RecipientPhone, text, message.DocumentURL, message.DocumentName)
		return err
	}
	return sender.SendText(ctx, config, message.RecipientPhone, text)
}

func configured(config whatsapp.Config) bool {
	return strings.TrimSpace(config.ProprioURL) != "" && strings.TrimSpace(config.ProprioToken) != "" && strings.TrimSpace(config.ProprioSession) != ""
}

func providerConnected(ctx context.Context, config whatsapp.Config) bool {
	base, err := url.Parse(strings.TrimRight(strings.TrimSpace(config.ProprioURL), "/"))
	if err != nil || base.Host == "" {
		return false
	}
	base.Path += "/v1/sessions/" + url.PathEscape(strings.TrimSpace(config.ProprioSession))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return false
	}
	request.Header.Set("Authorization", "Bearer "+config.ProprioToken)
	client := &http.Client{Timeout: 8 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false
	}
	var state struct {
		Status    string `json:"status"`
		Connected bool   `json:"connected"`
	}
	if json.NewDecoder(response.Body).Decode(&state) != nil {
		return false
	}
	return state.Connected || strings.EqualFold(strings.TrimSpace(state.Status), "connected")
}

func expireDue(ctx context.Context, db *supabase.Client, now time.Time) error {
	query := url.Values{"status": {"eq.pendente"}, "expires_at": {"lte." + now.UTC().Format(time.RFC3339Nano)}}
	result, err := db.ServiceRequest(ctx, tablePath+"?"+query.Encode(), supabase.RequestOptions{
		Method: http.MethodPatch,
		Body:   map[string]any{"status": "expirado", "message_text": "", "last_error": "mensagem expirada antes do envio", "updated_at": now.UTC()},
	})
	if err != nil {
		return err
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		return fmt.Errorf("não foi possível expirar mensagens vencidas (status %d)", result.StatusCode)
	}
	return nil
}

func recordAttempt(ctx context.Context, db *supabase.Client, message Message, attemptStatus, failure string, started, finished, retryAt time.Time) (string, error) {
	result, err := db.ServiceRequest(ctx, "/rest/v1/rpc/finish_whatsapp_message_attempt", supabase.RequestOptions{
		Method: http.MethodPost,
		Body: map[string]any{
			"p_message_id": message.ID, "p_attempt_number": message.AttemptCount,
			"p_attempt_status": attemptStatus, "p_error_message": optionalString(failure),
			"p_started_at": started.UTC(), "p_finished_at": finished.UTC(),
			"p_retry_at": optionalTime(retryAt),
		},
	})
	if err != nil {
		return "", err
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		return "", fmt.Errorf("não foi possível registrar o resultado da mensagem (status %d)", result.StatusCode)
	}
	var status string
	if err := json.Unmarshal(result.Body, &status); err != nil || status == "" {
		return "", errors.New("resposta inválida ao registrar o resultado da mensagem")
	}
	return status, nil
}

func optionalTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

func retryDelay(attempt int) time.Duration {
	minutes := 1
	if attempt > 1 {
		minutes = 1 << min(attempt-1, 10)
	}
	if minutes > 15 {
		minutes = 15
	}
	return time.Duration(minutes) * time.Minute
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.Join(strings.Fields(err.Error()), " ")
	if len(message) > 500 {
		message = message[:500]
	}
	return message
}
