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

	// bob opens it directly (discover → playlist detail) before subscribing:
	// viewable (it's public) and its `subscribed` flag must be false.
	status, view := doMap(t, srv, http.MethodGet, "/playlists/"+pub.ID, bobToken, nil)
	if status != http.StatusOK {
		t.Fatalf("get playlist status %d", status)
	}
	if view["subscribed"] != false {
		t.Fatalf("expected subscribed=false before subscribing, got %+v", view["subscribed"])
	}

	// bob subscribes.
	if code := doStatus(t, srv, http.MethodPut, "/playlists/"+pub.ID+"/subscription", bobToken, nil); code != http.StatusNoContent {
		t.Fatalf("subscribe failed: %d", code)
	}

	// Now it appears in bob's library (read-only) and the detail view reflects it.
	visible, _ = store.Playlists.ListVisible(ctx, bob.ID)
	if !containsPlaylist(visible, pub.ID) {
		t.Fatal("subscribed playlist should appear in bob's library")
	}
	if _, view = doMap(t, srv, http.MethodGet, "/playlists/"+pub.ID, bobToken, nil); view["subscribed"] != true {
		t.Fatalf("expected subscribed=true after subscribing, got %+v", view["subscribed"])
	}

	// Subscribing to a private playlist is refused.
	if code := doStatus(t, srv, http.MethodPut, "/playlists/"+priv.ID+"/subscription", bobToken, nil); code != http.StatusForbidden {
		t.Fatalf("subscribing to a private playlist must be 403, got %d", code)
	}

	// Unsubscribe → removed from library, detail view flips back.
	if code := doStatus(t, srv, http.MethodDelete, "/playlists/"+pub.ID+"/subscription", bobToken, nil); code != http.StatusNoContent {
		t.Fatalf("unsubscribe failed: %d", code)
	}
	visible, _ = store.Playlists.ListVisible(ctx, bob.ID)
	if containsPlaylist(visible, pub.ID) {
		t.Fatal("after unsubscribe the playlist must leave bob's library")
	}
	if _, view = doMap(t, srv, http.MethodGet, "/playlists/"+pub.ID, bobToken, nil); view["subscribed"] != false {
		t.Fatalf("expected subscribed=false after unsubscribing, got %+v", view["subscribed"])
	}
}

// TestFederatedPlaylistSubscribableByNominalOwner covers a real bug: a
// federated playlist's owner_id is just an internal attribution (whichever
// admin the sync process picked, to satisfy the FK) — it must not block that
// same account from subscribing to it, the only way a federated playlist ever
// joins anyone's library (see ListVisible).
func TestFederatedPlaylistSubscribableByNominalOwner(t *testing.T) {
	srv, store := newEnv(t)
	ctx := context.Background()

	alice, _ := store.Users.GetByUsername(ctx, "alice")
	now := time.Now()
	fed := models.Playlist{ID: uuid.NewString(), Name: "Hub Picks", OwnerID: alice.ID, Public: true, Federated: true, CreatedAt: now, UpdatedAt: now}
	_ = store.Playlists.Create(ctx, fed)

	aliceToken := login(t, srv, "alice")

	// Not visible by default, even to its nominal owner.
	visible, _ := store.Playlists.ListVisible(ctx, alice.ID)
	if containsPlaylist(visible, fed.ID) {
		t.Fatal("a federated playlist must not appear via owner_id alone")
	}

	// But it must be subscribable by that same account.
	if code := doStatus(t, srv, http.MethodPut, "/playlists/"+fed.ID+"/subscription", aliceToken, nil); code != http.StatusNoContent {
		t.Fatalf("subscribing to one's own nominally-owned federated playlist: %d", code)
	}
	visible, _ = store.Playlists.ListVisible(ctx, alice.ID)
	if !containsPlaylist(visible, fed.ID) {
		t.Fatal("subscribed federated playlist should now appear in the library")
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
