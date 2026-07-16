package main

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"
)

// TestAwaitShutdownSignalCancelsThenForceExits verifies the first SIGINT
// cancels ctx and a second one forces an immediate exit.
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
