package domain

import "context"

// WebhookEvent represents an event sent to the control plane.
type WebhookEvent struct {
	Type      string         `json:"type"`
	Timestamp int64          `json:"timestamp"`
	Payload   map[string]any `json:"payload"`
}

// WebhookSender is responsible for reliably delivering events.
type WebhookSender interface {
	Send(ctx context.Context, event WebhookEvent) error
}
