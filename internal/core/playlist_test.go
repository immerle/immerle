package core

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/testutil"
)

// TestFederatedPlaylistNotMutableByNominalOwner covers a real bug: a federated
// playlist is attributed to a nominal local owner (whichever admin the sync
// process picked) purely so the row satisfies the owner_id FK — that
// attribution must never grant real ownership. The nominal owner must not be
// able to delete, re-cover or add collaborators to it; they can only
// unsubscribe, like anyone else.
func TestFederatedPlaylistNotMutableByNominalOwner(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	owner := models.User{ID: uuid.NewString(), Username: "admin", PasswordHash: "x", IsAdmin: true, CreatedAt: now}
	if err := store.Users.Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	p := models.Playlist{
		ID: uuid.NewString(), Name: "Hub Picks", OwnerID: owner.ID, Public: true, Federated: true,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Playlists.Create(ctx, p); err != nil {
		t.Fatal(err)
	}

	svc := NewPlaylistService(store.Playlists, store.Annotations, nil, nil, nil)

	// Not deletable by the nominal owner: not subscribed, so forbidden outright.
	if err := svc.Delete(ctx, owner, p.ID); err != ErrForbidden {
		t.Fatalf("expected ErrForbidden deleting a federated playlist, got %v", err)
	}
	if _, err := store.Playlists.Get(ctx, p.ID); err != nil {
		t.Fatalf("federated playlist should still exist: %v", err)
	}

	// Not re-coverable.
	if _, err := svc.CoverTarget(ctx, owner, p.ID); err != ErrForbidden {
		t.Fatalf("expected ErrForbidden setting cover on a federated playlist, got %v", err)
	}

	// Once subscribed, Delete unsubscribes instead of deleting the row.
	if err := store.Playlists.Subscribe(ctx, p.ID, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, owner, p.ID); err != nil {
		t.Fatalf("unsubscribe-via-delete should succeed once subscribed: %v", err)
	}
	if _, err := store.Playlists.Get(ctx, p.ID); err != nil {
		t.Fatalf("federated playlist should still exist after unsubscribing: %v", err)
	}
	if subscribed, _ := store.Playlists.IsSubscribed(ctx, p.ID, owner.ID); subscribed {
		t.Fatal("expected the subscription to be gone")
	}
}

// TestPlaylistAddResolvesRemoteTrack covers a real bug: a track fetched
// on-the-fly from an on-demand provider (a "remote:" id, not yet a row in
// `tracks`) couldn't be added to a playlist — playlist_tracks has a foreign
// key on track_id, so the insert failed deep in the persistence layer.
// Create/Replace/Update must resolve (download) such ids first, the same way
// favoriting already does.
func TestPlaylistAddResolvesRemoteTrack(t *testing.T) {
	onDemand, store, _ := newOnDemand(t)
	ctx := context.Background()
	now := time.Now()

	owner := models.User{ID: uuid.NewString(), Username: "owner", PasswordHash: "x", CreatedAt: now}
	if err := store.Users.Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	svc := NewPlaylistService(store.Playlists, store.Annotations, nil, nil, onDemand)

	remote, err := onDemand.RemoteSearch(ctx, "Remote", 10)
	if err != nil || len(remote) != 1 {
		t.Fatalf("remote search: %v %+v", err, remote)
	}
	remoteID := remote[0].ID
	if !IsRemoteID(remoteID) {
		t.Fatalf("expected a remote id, got %q", remoteID)
	}

	// Create, seeded with a remote track: it must be resolved (downloaded),
	// not left dangling or silently dropped.
	d, err := svc.Create(ctx, owner, "My Mix", []string{remoteID})
	if err != nil {
		t.Fatalf("create with a remote track: %v", err)
	}
	if len(d.Tracks) != 1 || IsRemoteID(d.Tracks[0].Track.ID) || d.Tracks[0].Track.ID == "" {
		t.Fatalf("expected the track resolved to a real local id, got %+v", d.Tracks)
	}

	// Update (the playlist-menu "add to playlist" path) resolves too. The
	// track is already downloaded now, so this also covers the dedup path
	// (second resolve of the same remote id must not re-download).
	if err := svc.Update(ctx, owner, d.Playlist.ID, PlaylistMetaUpdate{}, []string{remoteID}, nil); err != nil {
		t.Fatalf("update (add) with a remote track: %v", err)
	}
	tracks, _ := store.Playlists.Tracks(ctx, d.Playlist.ID)
	if len(tracks) != 2 {
		t.Fatalf("expected 2 tracks after appending, got %d", len(tracks))
	}
	for _, tr := range tracks {
		if tr.Unresolved || tr.ID == "" {
			t.Fatalf("track not resolved: %+v", tr)
		}
	}
}

// TestPlaylistCreateSurfacesUnresolvableTrackError covers the other half of
// the same bug: Create used to swallow ReplaceTracks' error
// (`_ = s.playlists.ReplaceTracks(...)`), so seeding a playlist with an
// unresolvable track silently returned a playlist that looked created but was
// empty. It must now error.
func TestPlaylistCreateSurfacesUnresolvableTrackError(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	owner := models.User{ID: uuid.NewString(), Username: "owner2", PasswordHash: "x", CreatedAt: now}
	if err := store.Users.Create(ctx, owner); err != nil {
		t.Fatal(err)
	}

	// No on-demand resolver wired: a remote track id can never be resolved.
	svc := NewPlaylistService(store.Playlists, store.Annotations, nil, nil, nil)
	if _, err := svc.Create(ctx, owner, "Broken", []string{"remote:fake:1"}); err == nil {
		t.Fatal("expected an error adding an unresolvable remote track")
	}
}
