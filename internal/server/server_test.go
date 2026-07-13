package server

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRunShutsDownOnContextCancel(t *testing.T) {
	s := New("127.0.0.1:0", http.NotFoundHandler(), testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- s.Run(ctx) }()

	// Give the server a moment to start listening before cancelling.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run() = %v, want nil after graceful shutdown", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}
}

// TestRunSurvivesShutdownGraceTimeout covers the fix: http.Server.Shutdown
// never force-closes a still-open connection (e.g. a live SSE stream), it only
// waits — so if one outlasts shutdownGrace, Shutdown returns
// context.DeadlineExceeded. That's an expected outcome of an intentional
// shutdown, not an app failure, and must not surface as an error from Run.
func TestRunSurvivesShutdownGraceTimeout(t *testing.T) {
	old := shutdownGrace
	shutdownGrace = 20 * time.Millisecond
	defer func() { shutdownGrace = old }()

	// Reserve a free port up front: Run calls ListenAndServe, which resolves
	// ":0" internally without exposing the chosen port, so a real client
	// needs a concrete address to connect to.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	blocked := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blocked // simulates a connection still open past the grace period
	})
	s := New(addr, handler, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	errCh := runAsync(s, ctx)
	time.Sleep(50 * time.Millisecond) // let it start listening

	go func() {
		_, _ = http.Get("http://" + addr + "/")
	}()
	time.Sleep(50 * time.Millisecond) // let the request reach the handler
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run() = %v, want nil despite the shutdown grace timeout", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after the shutdown grace period elapsed")
	}
	close(blocked)
}

func TestRunReturnsListenError(t *testing.T) {
	// Occupy a port so the server fails to bind.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer ln.Close()

	s := New(ln.Addr().String(), http.NotFoundHandler(), testLogger())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	select {
	case err := <-runAsync(s, ctx):
		if err == nil {
			t.Fatal("Run() = nil, want an error for an address already in use")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after a listen failure")
	}
}

func runAsync(s *Server, ctx context.Context) <-chan error {
	errCh := make(chan error, 1)
	go func() { errCh <- s.Run(ctx) }()
	return errCh
}
