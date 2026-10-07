package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/testutil"
)

// A track deleted between picking its id and writing the playlist (evicted,
// pruned, deleted by hand) must be skipped, not fail the whole replace.
func TestReplaceTracksSkipsVanishedTrack(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	owner := models.User{ID: uuid.NewString(), Username: "owner", PasswordHash: "x", CreatedAt: now}
	if err := store.Users.Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: now})
	var ids []string
	for i := 0; i < 3; i++ {
		id, err := store.Catalog.UpsertTrack(ctx, models.Track{
			ID: uuid.NewString(), Title: "t", AlbumID: albumID, ArtistID: artistID,
			Path: uuid.NewString(), CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := store.Catalog.DeleteTrack(ctx, ids[1]); err != nil {
		t.Fatal(err)
	}

	pl := models.Playlist{ID: uuid.NewString(), Name: "P", OwnerID: owner.ID, CreatedAt: now, UpdatedAt: now}
	if err := store.Playlists.Create(ctx, pl); err != nil {
		t.Fatal(err)
	}
	if err := store.Playlists.ReplaceTracks(ctx, pl.ID, ids, owner.ID); err != nil {
		t.Fatalf("replace with a vanished track should not fail: %v", err)
	}
	tracks, err := store.Playlists.Tracks(ctx, pl.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 2 || tracks[0].ID != ids[0] || tracks[1].ID != ids[2] {
		t.Fatalf("expected [%s %s], got %+v", ids[0], ids[2], tracks)
	}

	fed := models.Playlist{ID: uuid.NewString(), Name: "F", OwnerID: owner.ID, CreatedAt: now, UpdatedAt: now}
	if err := store.Playlists.Create(ctx, fed); err != nil {
		t.Fatal(err)
	}
	err = store.Playlists.ReplaceFederatedTracks(ctx, fed.ID, []persistence.FederatedTrackRef{
		{TrackID: ids[0], Artist: "A", Title: "t"},
		{TrackID: ids[1], Artist: "A", Title: "gone"},
	})
	if err != nil {
		t.Fatalf("federated replace with a vanished track should not fail: %v", err)
	}
	fedTracks, err := store.Playlists.Tracks(ctx, fed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(fedTracks) != 2 || fedTracks[0].ID != ids[0] || !fedTracks[1].Unresolved {
		t.Fatalf("expected the vanished entry kept as unresolved, got %+v", fedTracks)
	}
}
