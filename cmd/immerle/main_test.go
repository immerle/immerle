package main

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"
)

// TestAwaitShutdownSignalCancelsThenForceExits covers the fix: a first SIGINT
// starts graceful shutdown (cancels ctx), a second one — sent while still
// waiting — forces an immediate exit instead of being silently swallowed.
func TestAwaitShutdownSignalCancelsThenForceExits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	forced := make(chan struct{})
	awaitShutdownSignal(cancel, func() { close(forced) })

	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("send first SIGINT: %v", err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("first signal did not cancel the context")
	}

	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("send second SIGINT: %v", err)
	}
	select {
	case <-forced:
	case <-time.After(time.Second):
		t.Fatal("second signal did not trigger the force-exit callback")
	}
}
