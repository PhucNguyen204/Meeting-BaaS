package states

import (
	"context"

	"go.uber.org/zap"

	"github.com/PhucNguyen204/Meeting-BaaS/internal/pkg/logger"
	sm "github.com/PhucNguyen204/Meeting-BaaS/internal/usecase/bot"
)

// ErrorState logs the error, takes a diagnostic snapshot, and transitions
// to cleanup.
//
// Port reference: src/state-machine/states/error-state.ts.
type ErrorState struct {
	Webhooker Webhooker
}

func (s *ErrorState) Name() sm.StateType { return sm.StateError }

func (s *ErrorState) Execute(ctx context.Context, mc *sm.MeetingContext) (sm.Transition, error) {
	log := logger.FromContext(ctx).With(zap.String("state", "error"))

	hasErr, reason, msg := mc.GetError()
	if hasErr {
		log.Error("error state entered",
			zap.String("end_reason", string(reason)),
			zap.String("message", msg),
		)
	} else {
		log.Warn("error state entered with no error context")
	}

	// Send error webhook if configured.
	if s.Webhooker != nil {
		if err := s.Webhooker.SendError(ctx, mc); err != nil {
			log.Warn("error webhook failed", zap.Error(err))
		}
	}

	return sm.Transition{Next: sm.StateCleanup}, nil
}
