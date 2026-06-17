package immerle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
)

func TestPublicPlaylistSubscriptionFlow(t *testing.T) {
	srv, store := newEnv(t)
	ctx := context.Background()

	alice, _ := store.Users.GetByUsername(ctx, "alice")
	bob, _ := store.Users.GetByUsername(ctx, "bob")

	// Alice owns a public playlist and a private one.
	now := time.Now()
	pub := models.Playlist{ID: uuid.NewString(), Name: "Alice Public", OwnerID: alice.ID, Public: true, CreatedAt: now, UpdatedAt: now}
	priv := models.Playlist{ID: uuid.NewString(), Name: "Alice Private", OwnerID: alice.ID, Public: false, CreatedAt: now, UpdatedAt: now}
	_ = store.Playlists.Create(ctx, pub)
	_ = store.Playlists.Create(ctx, priv)

	// Before subscribing, bob's library does NOT contain Alice's public playlist.
	visible, _ := store.Playlists.ListVisible(ctx, bob.ID)
	if containsPlaylist(visible, pub.ID) {
		t.Fatal("public playlist must not appear in a non-subscriber's library")
	}

	// bob browses public playlists and sees it (not subscribed yet).
	pubList := postForm(t, srv, "/playlists/public", creds("bob"))
	pls, _ := pubList["playlists"].([]any)
	if len(pls) != 1 {
		t.Fatalf("expected 1 public playlist, got %d", len(pls))
	}
	first, _ := pls[0].(map[string]any)
	if first["subscribed"] != false || first["name"] != "Alice Public" {
		t.Fatalf("unexpected public entry: %+v", first)
	}

	// bob subscribes.
	sv := creds("bob")
	sv.Set("playlistId", pub.ID)
	if r := postForm(t, srv, "/playlists/subscribe", sv); r["ok"] != true {
		t.Fatalf("subscribe failed: %+v", r)
	}

	// Now it appears in bob's library (read-only).
	visible, _ = store.Playlists.ListVisible(ctx, bob.ID)
	if !containsPlaylist(visible, pub.ID) {
		t.Fatal("subscribed playlist should appear in bob's library")
	}

	// Subscribing to a private playlist is refused.
	pv := creds("bob")
	pv.Set("playlistId", priv.ID)
	if code := postFormStatus(t, srv, "/playlists/subscribe", pv); code != http.StatusForbidden {
		t.Fatalf("subscribing to a private playlist must be 403, got %d", code)
	}

	// Unsubscribe → removed from library.
	uv := creds("bob")
	uv.Set("playlistId", pub.ID)
	postForm(t, srv, "/playlists/unsubscribe", uv)
	visible, _ = store.Playlists.ListVisible(ctx, bob.ID)
	if containsPlaylist(visible, pub.ID) {
		t.Fatal("after unsubscribe the playlist must leave bob's library")
	}
}

func containsPlaylist(pls []models.Playlist, id string) bool {
	for _, p := range pls {
		if p.ID == id {
			return true
		}
	}
	return false
}

// postFormStatus is like postForm but returns the HTTP status code.
func postFormStatus(t *testing.T, srv *httptest.Server, path string, v url.Values) int {
	t.Helper()
	resp, err := http.PostForm(srv.URL+path, v)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var discard map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&discard)
	return resp.StatusCode
}
