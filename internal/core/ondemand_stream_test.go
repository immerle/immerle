package core

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestProgressiveStreamServesThenIngests(t *testing.T) {
	svc, store, _ := newOnDemand(t) // skips when ffmpeg is unavailable
	ctx := context.Background()

	remote, err := svc.RemoteSearch(ctx, "Remote", 10)
	if err != nil || len(remote) != 1 {
		t.Fatalf("expected 1 remote result, got %d (err %v)", len(remote), err)
	}
	id := remote[0].ID

	// Not local yet → a pending download is returned (no blocking download).
	_, local, pending, err := svc.PrepareStream(ctx, "user-1", id)
	if err != nil {
		t.Fatal(err)
	}
	if local || pending == nil {
		t.Fatalf("expected a pending download, got local=%v pending=%v", local, pending)
	}

	// Streaming tees bytes to the writer immediately.
	var buf bytes.Buffer
	if err := svc.StreamPending(ctx, pending, &buf); err != nil {
		t.Fatal(err)
	}
	if buf.Len() == 0 {
		t.Fatal("no bytes streamed to the client")
	}

	// The saved copy is ingested in the background → the track becomes local.
	if !eventually(t, func() bool {
		_, l, _, _ := svc.PrepareStream(ctx, "user-1", id)
		return l
	}) {
		t.Fatal("track never became local after progressive stream")
	}

	if _, _, tracks, _ := store.Catalog.Stats(ctx); tracks != 1 {
		t.Fatalf("expected exactly 1 local track, got %d", tracks)
	}

	// A second stream now resolves locally (no pending download).
	_, local2, pending2, _ := svc.PrepareStream(ctx, "user-1", id)
	if !local2 || pending2 != nil {
		t.Fatalf("second access should be local, got local=%v pending=%v", local2, pending2)
	}
}

func eventually(t *testing.T, cond func() bool) bool {
	t.Helper()
	for i := 0; i < 100; i++ {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}
