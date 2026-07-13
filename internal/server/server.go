// Package server runs the HTTP server with graceful shutdown.
package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

// shutdownGrace caps how long shutdown waits for in-flight requests. When nothing
// is in flight, Shutdown returns immediately — so Ctrl+C on an idle server exits
// instantly.
const shutdownGrace = 10 * time.Second

// Server wraps http.Server with graceful shutdown.
type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
}

// New builds a Server bound to address with the given handler.
func New(address string, handler http.Handler, logger *slog.Logger) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:              address,
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
			// No WriteTimeout on purpose: streaming responses are long-lived and it
			// would cut them off. IdleTimeout bounds idle keep-alive connections.
			IdleTimeout: 120 * time.Second,
		},
		logger: logger,
	}
}

// Run starts serving and blocks until ctx is cancelled, then shuts down (waiting
// at most shutdownGrace for active requests; instant when idle).
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("http server listening", "address", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		s.logger.Info("shutting down http server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		return s.httpServer.Shutdown(shutdownCtx)
	}
}
