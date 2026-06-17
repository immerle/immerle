package persistence_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gossignol/gossignol/internal/models"
	"github.com/gossignol/gossignol/internal/testutil"
)

func TestCollaborativePlaylistConcurrentAppendNoLoss(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	// Owner + collaborator.
	owner := models.User{ID: uuid.NewString(), Username: "owner", PasswordHash: "x", CreatedAt: now}
	collab := models.User{ID: uuid.NewString(), Username: "collab", PasswordHash: "x", CreatedAt: now}
	if err := store.Users.Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if err := store.Users.Create(ctx, collab); err != nil {
		t.Fatal(err)
	}

	// Seed an artist + album so tracks satisfy FKs.
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: now})

	const perUser = 20
	trackIDs := make([]string, 0, perUser*2)
	for i := 0; i < perUser*2; i++ {
		id, err := store.Catalog.UpsertTrack(ctx, models.Track{
			ID: uuid.NewString(), Title: "t", AlbumID: albumID, ArtistID: artistID,
			Path: uuid.NewString(), CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			t.Fatal(err)
		}
		trackIDs = append(trackIDs, id)
	}

	// Collaborative playlist.
	pl := models.Playlist{ID: uuid.NewString(), Name: "Shared", OwnerID: owner.ID, Collaborative: true, CreatedAt: now, UpdatedAt: now}
	if err := store.Playlists.Create(ctx, pl); err != nil {
		t.Fatal(err)
	}
	_ = store.Playlists.AddCollaborator(ctx, pl.ID, collab.ID)

	// Two users append concurrently; AppendTracks serializes via a transaction
	// computing MAX(position)+1, so no positions collide and nothing is lost.
	var wg sync.WaitGroup
	wg.Add(2)
	appendAll := func(user string, ids []string) {
		defer wg.Done()
		for _, id := range ids {
			if err := store.Playlists.AppendTracks(ctx, pl.ID, []string{id}, user); err != nil {
				t.Errorf("append: %v", err)
				return
			}
		}
	}
	go appendAll(owner.ID, trackIDs[:perUser])
	go appendAll(collab.ID, trackIDs[perUser:])
	wg.Wait()

	got, err := store.Playlists.Tracks(ctx, pl.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != perUser*2 {
		t.Fatalf("expected %d tracks after concurrent append, got %d (data lost)", perUser*2, len(got))
	}
	// All ids present exactly once.
	seen := map[string]int{}
	for _, tr := range got {
		seen[tr.ID]++
	}
	for _, id := range trackIDs {
		if seen[id] != 1 {
			t.Fatalf("track %s appears %d times (expected 1)", id, seen[id])
		}
	}
}
