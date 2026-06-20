package immerle

import (
	"net/http"
	"testing"
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

	// Save then read back.
	if st := doStatus(t, srv, http.MethodPut, "/play-queue", token, map[string]any{
		"ids": []string{id}, "current": id, "position": 4200,
	}); st != http.StatusNoContent {
		t.Fatalf("save queue: status %d", st)
	}
	var q playQueueView
	if st := getJSON(t, srv, token, "/play-queue", &q); st != http.StatusOK {
		t.Fatalf("get queue: status %d", st)
	}
	if q.Current != id || q.Position != 4200 || len(q.Entries) != 1 {
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
