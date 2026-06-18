package immerle

import (
	"context"
	"net/http"
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

	bobToken := login(t, srv, "bob")

	// Before subscribing, bob's library does NOT contain Alice's public playlist.
	visible, _ := store.Playlists.ListVisible(ctx, bob.ID)
	if containsPlaylist(visible, pub.ID) {
		t.Fatal("public playlist must not appear in a non-subscriber's library")
	}

	// bob browses public playlists and sees it (not subscribed yet).
	status, pls := doArr(t, srv, http.MethodGet, "/playlists/public", bobToken, nil)
	if status != http.StatusOK {
		t.Fatalf("public list status %d", status)
	}
	if len(pls) != 1 {
		t.Fatalf("expected 1 public playlist, got %d", len(pls))
	}
	first, _ := pls[0].(map[string]any)
	if first["subscribed"] != false || first["name"] != "Alice Public" {
		t.Fatalf("unexpected public entry: %+v", first)
	}

	// bob subscribes.
	if code := doStatus(t, srv, http.MethodPut, "/playlists/"+pub.ID+"/subscription", bobToken, nil); code != http.StatusNoContent {
		t.Fatalf("subscribe failed: %d", code)
	}

	// Now it appears in bob's library (read-only).
	visible, _ = store.Playlists.ListVisible(ctx, bob.ID)
	if !containsPlaylist(visible, pub.ID) {
		t.Fatal("subscribed playlist should appear in bob's library")
	}

	// Subscribing to a private playlist is refused.
	if code := doStatus(t, srv, http.MethodPut, "/playlists/"+priv.ID+"/subscription", bobToken, nil); code != http.StatusForbidden {
		t.Fatalf("subscribing to a private playlist must be 403, got %d", code)
	}

	// Unsubscribe → removed from library.
	if code := doStatus(t, srv, http.MethodDelete, "/playlists/"+pub.ID+"/subscription", bobToken, nil); code != http.StatusNoContent {
		t.Fatalf("unsubscribe failed: %d", code)
	}
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
