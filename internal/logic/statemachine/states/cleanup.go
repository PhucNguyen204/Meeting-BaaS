package states

import (
	"context"

	"go.uber.org/zap"

	sm "github.com/yourorg/meet-bot-go/internal/logic/statemachine"
	"github.com/yourorg/meet-bot-go/internal/pkg/logger"
)

// CleanupState finalizes the recording, uploads artifacts, and sends
// webhooks.
//
// Port reference: src/state-machine/states/cleanup-state.ts.
type CleanupState struct {
	Recorder  Recorder
	Uploader  Uploader
	Webhooker Webhooker
}

func (s *CleanupState) Name() sm.StateType { return sm.StateCleanup }

func (s *CleanupState) Execute(ctx context.Context, mc *sm.MeetingContext) (sm.Transition, error) {
	log := logger.FromContext(ctx).With(zap.String("state", "cleanup"))
	log.Info("cleanup starting",
		zap.String("end_reason", string(mc.GetEndReason())),
	)

	// 1. Stop recorder and finalize MP4.
	if s.Recorder != nil {
		if err := s.Recorder.Stop(ctx); err != nil {
			log.Error("recorder stop failed", zap.Error(err))
		} else {
			log.Info("recording finalized")
		}
	}

	// 2. Close the meeting gracefully.
	if mc.Provider != nil && mc.Page != nil {
		if err := mc.Provider.CloseMeeting(ctx, mc.Page); err != nil {
			log.Warn("close meeting failed", zap.Error(err))
		}
	}

	// 3. Upload to S3.
	if s.Uploader != nil {
		if err := s.Uploader.Upload(ctx, mc); err != nil {
			log.Error("upload failed", zap.Error(err))
		} else {
			log.Info("artifacts uploaded")
		}
	}

	// 4. Send webhook.
	if s.Webhooker != nil {
		if err := s.Webhooker.SendComplete(ctx, mc); err != nil {
			log.Error("webhook failed", zap.Error(err))
		} else {
			log.Info("webhook sent")
		}
	}

	// 5. Close browser.
	if mc.BrowserDriver != nil {
		if err := mc.BrowserDriver.Close(ctx); err != nil {
			log.Warn("browser close failed", zap.Error(err))
		}
	}

	log.Info("cleanup complete, terminating")
	return sm.Transition{Next: sm.StateTerminated}, nil
}
