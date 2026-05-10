package webhook

import (
	"context"
	"time"

	sm "github.com/yourorg/meet-bot-go/internal/logic/statemachine"
)

// EventType identifies webhook event types per MeetingBaaS API v2.
type EventType string

const (
	EventComplete EventType = "complete"
	EventFailed   EventType = "failed"
)

// WebhookPayload is the webhook body sent to the configured URL.
//
// Port reference: MeetingBaaS API v2 webhook schema.
type WebhookPayload struct {
	Event     EventType     `json:"event"`
	BotUUID   string        `json:"bot_id"`
	SessionID string        `json:"session_id"`
	Timestamp time.Time     `json:"timestamp"`
	Data      *WebhookData  `json:"data,omitempty"`
	Error     *WebhookError `json:"error,omitempty"`
}

// WebhookData carries recording metadata on successful completion.
type WebhookData struct {
	MP4URL        string        `json:"mp4_url,omitempty"`
	Duration      time.Duration `json:"duration_ms,omitempty"`
	Participants  []string      `json:"participants,omitempty"`
	EndReason     string        `json:"end_reason"`
}

// WebhookError carries error details on failure.
type WebhookError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// BotWebhooker implements the states.Webhooker interface using Sender.
type BotWebhooker struct {
	sender     *Sender
	webhookURL string
}

// NewBotWebhooker creates a Webhooker backed by a Sender.
func NewBotWebhooker(sender *Sender, webhookURL string) *BotWebhooker {
	return &BotWebhooker{sender: sender, webhookURL: webhookURL}
}

// SendComplete sends a success webhook.
func (w *BotWebhooker) SendComplete(ctx context.Context, mc *sm.MeetingContext) error {
	payload := WebhookPayload{
		Event:     EventComplete,
		BotUUID:   mc.Config.BotUUID,
		SessionID: mc.Config.SessionID,
		Timestamp: time.Now(),
		Data: &WebhookData{
			EndReason: string(mc.GetEndReason()),
		},
	}
	return w.sender.Send(ctx, w.webhookURL, payload)
}

// SendError sends a failure webhook.
func (w *BotWebhooker) SendError(ctx context.Context, mc *sm.MeetingContext) error {
	_, reason, msg := mc.GetError()
	payload := WebhookPayload{
		Event:     EventFailed,
		BotUUID:   mc.Config.BotUUID,
		SessionID: mc.Config.SessionID,
		Timestamp: time.Now(),
		Error: &WebhookError{
			Code:    string(reason),
			Message: msg,
		},
	}
	return w.sender.Send(ctx, w.webhookURL, payload)
}
