package meet

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/yourorg/meet-bot-go/internal/logic/browser"
)

// SendEntryMessage opens the Meet chat panel and posts msg.
//
// Port reference: src/meeting/meet/sendEntryMessage.ts.
//
// Outline:
//   1. Click chat toggle (SelChatToggleButton).
//   2. Wait for chat panel (SelChatPanel) to be visible.
//   3. Fill SelChatTextarea with msg.
//   4. Click SelChatSendButton (or press Enter).
//   5. Optionally click chat toggle again to close.
//
// Best-effort: the chat may be disabled by the host; in that case log a
// warning and return nil rather than failing the join.
//
// TODO(user): port the body, including timeouts and dialog handling.
func SendEntryMessage(ctx context.Context, page browser.Page, msg string) error {
	if page == nil {
		return fmt.Errorf("meet: nil page")
	}
	if msg == "" {
		return nil
	}
	log := zap.L().Named("meet.send_message").With(zap.String("msg", msg))
	log.Debug("posting entry message")
	_ = ctx
	return nil
}
