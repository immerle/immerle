package immerle

import (
	"bufio"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPlayQueueAndNowPlaying(t *testing.T) {
	srv, token, _ := newBrowseEnv(t)

	// Locate a track id.
	var search searchView
	if st := getJSON(t, srv, token, "/search?q=So+What", &search); st != http.StatusOK || len(search.Songs) == 0 {
		t.Fatalf("search: status %d, songs %d", st, len(search.Songs))
	}
	id := search.Songs[0].ID

	// No queue saved yet → empty queue.
	var empty playQueueView
	if st := getJSON(t, srv, token, "/play-queue", &empty); st != http.StatusOK {
		t.Fatalf("get empty queue: status %d", st)
	}
	if len(empty.Entries) != 0 {
		t.Fatalf("expected empty queue, got %d entries", len(empty.Entries))
	}

	// Save then read back — including the playing flag, which a spectator
	// device uses to push a remote play/pause/skip command (see
	// TestPlaybackTargets and ui/src/audio/store.ts's pollPlayQueue).
	if st := doStatus(t, srv, http.MethodPut, "/play-queue", token, map[string]any{
		"ids": []string{id}, "current": id, "position": 4200, "playing": true,
	}); st != http.StatusNoContent {
		t.Fatalf("save queue: status %d", st)
	}
	var q playQueueView
	if st := getJSON(t, srv, token, "/play-queue", &q); st != http.StatusOK {
		t.Fatalf("get queue: status %d", st)
	}
	if q.Current != id || q.Position != 4200 || !q.Playing || len(q.Entries) != 1 {
		t.Fatalf("queue not persisted: %+v", q)
	}

	// A scrobble surfaces the track in the now-playing feed.
	if st := doStatus(t, srv, http.MethodPost, "/scrobbles", token, map[string]any{"ids": []string{id}, "submission": false}); st != http.StatusNoContent {
		t.Fatalf("scrobble: status %d", st)
	}
	var np struct {
		NowPlaying []nowPlayingView `json:"nowPlaying"`
	}
	if st := getJSON(t, srv, token, "/now-playing", &np); st != http.StatusOK {
		t.Fatalf("now-playing: status %d", st)
	}
	if len(np.NowPlaying) != 1 || np.NowPlaying[0].Song.ID != id {
		t.Fatalf("now-playing feed: %+v", np.NowPlaying)
	}
}

// TestPlaybackTargets covers the "cast to device" feature: setting/clearing
// the active-playback device on the saved queue, and listing candidate
// targets — which must include only device-kind tokens (app logins) that
// have actually been used, not manually-created personal/CLI tokens.
func TestPlaybackTargets(t *testing.T) {
	srv, token, _ := newBrowseEnv(t)

	if st := doStatus(t, srv, http.MethodPut, "/play-queue/target", token, map[string]any{"deviceId": "some-device"}); st != http.StatusNoContent {
		t.Fatalf("set target: status %d", st)
	}
	var q playQueueView
	if st := getJSON(t, srv, token, "/play-queue", &q); st != http.StatusOK {
		t.Fatalf("get queue: status %d", st)
	}
	if q.TargetDeviceID != "some-device" {
		t.Fatalf("target not persisted: %+v", q)
	}

	if st := doStatus(t, srv, http.MethodPut, "/play-queue/target", token, map[string]any{"deviceId": ""}); st != http.StatusNoContent {
		t.Fatalf("clear target: status %d", st)
	}
	// Fresh struct: TargetDeviceID has `omitempty`, so a cleared value is
	// absent from the response JSON and decoding into the same `q` from
	// above would just leave its stale non-empty value in place.
	var cleared playQueueView
	if st := getJSON(t, srv, token, "/play-queue", &cleared); st != http.StatusOK {
		t.Fatalf("get queue: status %d", st)
	}
	if cleared.TargetDeviceID != "" {
		t.Fatalf("target not cleared: %+v", cleared)
	}

	// A device-kind token, once used, shows up as a playback target; a
	// manually-created one never does.
	deviceStatus, deviceBody := doMap(t, srv, http.MethodPost, "/tokens", token, map[string]any{"name": "phone", "device": true})
	if deviceStatus != http.StatusCreated {
		t.Fatalf("create device token: status %d", deviceStatus)
	}
	deviceSecret, _ := deviceBody["token"].(string)
	if cliStatus, _ := doMap(t, srv, http.MethodPost, "/tokens", token, map[string]any{"name": "cli-script"}); cliStatus != http.StatusCreated {
		t.Fatalf("create cli token: status %d", cliStatus)
	}

	// Exercise the device token once so it counts as "recently used".
	if st := doStatus(t, srv, http.MethodGet, "/artists", deviceSecret, nil); st != http.StatusOK {
		t.Fatalf("authenticate as device token: status %d", st)
	}

	var targets []playbackTargetView
	if st := getJSON(t, srv, token, "/play-queue/targets", &targets); st != http.StatusOK {
		t.Fatalf("list targets: status %d", st)
	}
	if len(targets) != 1 || targets[0].Name != "phone" {
		t.Fatalf("expected only the used device token, got %+v", targets)
	}
}

// TestPlayQueueSSEKeepsClientsSynced covers the real-time channel behind
// cross-device sync: a connected client gets the current snapshot
// immediately, then a fresh event the moment another client saves a change —
// no polling delay. See ui/src/audio/store.ts (web uses this; native falls
// back to polling since React Native has no EventSource).
func TestPlayQueueSSEKeepsClientsSynced(t *testing.T) {
	srv, token, _ := newBrowseEnv(t)

	var search searchView
	if st := getJSON(t, srv, token, "/search?q=So+What", &search); st != http.StatusOK || len(search.Songs) == 0 {
		t.Fatalf("search: status %d, songs %d", st, len(search.Songs))
	}
	id := search.Songs[0].ID

	resp := do(t, srv, http.MethodGet, "/play-queue/events", token, nil)
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("expected SSE content type, got %q", ct)
	}
	reader := bufio.NewReader(resp.Body)
	readSSEData(t, reader) // initial (empty, nothing saved yet) snapshot

	if st := doStatus(t, srv, http.MethodPut, "/play-queue", token, map[string]any{
		"ids": []string{id}, "current": id, "position": 4200, "playing": true,
	}); st != http.StatusNoContent {
		t.Fatalf("save queue: status %d", st)
	}

	done := make(chan map[string]any, 1)
	go func() { done <- readSSEData(t, reader) }()
	select {
	case data := <-done:
		if data["current"] != id || data["playing"] != true {
			t.Errorf("not synced via SSE: %+v", data)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("did not receive SSE play-queue update")
	}
}
