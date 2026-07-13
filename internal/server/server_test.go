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
