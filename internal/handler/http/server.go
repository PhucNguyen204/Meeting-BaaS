// Package http hosts the in-bot control plane HTTP server.
//
//	GET  /healthz       -> liveness/readiness
//	GET  /readyz        -> alias
//	GET  /version       -> build metadata
//	GET  /status        -> runtime status
//	POST /stop_record   -> request graceful stop
//	POST /pause         -> pause recording
//	POST /resume        -> resume recording
//
// Port reference: src/server.ts.
package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"

	"github.com/yourorg/meet-bot-go/internal/pkg/logger"
)

// Stopper is the dependency the /stop_record handler calls into.
type Stopper interface {
	Stop(ctx context.Context, reason string) error
}

// Server bundles the http.Server with the chi router and dependencies.
type Server struct {
	addr           string
	log            *zap.Logger
	router         chi.Router
	stopper        Stopper
	statusProvider StatusProvider
	pauseResumer   PauseResumer

	httpSrv *http.Server
}

// New constructs a Server but does not start it. Call Start in a goroutine.
func New(addr string, log *zap.Logger, stopper Stopper) *Server {
	if log == nil {
		log = zap.NewNop()
	}
	s := &Server{
		addr:    addr,
		log:     log.Named("http"),
		router:  chi.NewRouter(),
		stopper: stopper,
	}
	s.routes()
	return s
}

// SetStatusProvider wires the status endpoint to the state machine.
func (s *Server) SetStatusProvider(sp StatusProvider) { s.statusProvider = sp }

// SetPauseResumer wires the pause/resume endpoints to the state machine.
func (s *Server) SetPauseResumer(pr PauseResumer) { s.pauseResumer = pr }

// routes registers all endpoints.
func (s *Server) routes() {
	r := s.router
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(s.zapLogger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", s.handleHealthz)
	r.Get("/readyz", s.handleHealthz)
	r.Get("/version", s.handleVersion)
	r.Get("/status", s.handleStatus)
	r.Post("/stop_record", s.handleStopRecord)
	r.Post("/pause", s.handlePause)
	r.Post("/resume", s.handleResume)
}

// Start blocks until the server stops or ctx is cancelled.
func (s *Server) Start(ctx context.Context) error {
	s.httpSrv = &http.Server{
		Addr:              s.addr,
		Handler:           s.router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		s.log.Info("http server listening", zap.String("addr", s.addr))
		if err := s.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpSrv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("http: shutdown: %w", err)
		}
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

// zapLogger is a chi-compatible middleware.
func (s *Server) zapLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		reqID := middleware.GetReqID(r.Context())

		ctx := logger.WithRequestID(r.Context(), reqID)
		ctx = logger.IntoContext(ctx, s.log)
		next.ServeHTTP(ww, r.WithContext(ctx))

		s.log.Info("http request",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.Int("status", ww.Status()),
			zap.Int("bytes", ww.BytesWritten()),
			zap.Duration("dur", time.Since(start)),
			zap.String("request_id", reqID),
			zap.String("remote", r.RemoteAddr),
		)
	})
}
