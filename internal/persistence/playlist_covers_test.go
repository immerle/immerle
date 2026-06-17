package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gossignol/gossignol/internal/models"
	"github.com/gossignol/gossignol/internal/testutil"
)

func TestPlaylistCoverArtsMosaic(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	owner := models.User{ID: uuid.NewString(), Username: "owner", PasswordHash: "x", CreatedAt: now}
	if err := store.Users.Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: now})

	// 6 tracks: the first has no own cover (falls back to album id), the rest do.
	ids := make([]string, 0, 6)
	covers := make([]string, 0, 6)
	for i := 0; i < 6; i++ {
		cover := ""
		if i > 0 {
			cover = "cover" + uuid.NewString()
		}
		id, err := store.Catalog.UpsertTrack(ctx, models.Track{
			ID: uuid.NewString(), Title: "t", AlbumID: albumID, ArtistID: artistID,
			Path: uuid.NewString(), CoverArt: cover, CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
		if cover == "" {
			covers = append(covers, albumID) // fallback to album id
		} else {
			covers = append(covers, cover)
		}
	}

	pl := models.Playlist{ID: uuid.NewString(), Name: "Mix", OwnerID: owner.ID, Public: true, CreatedAt: now, UpdatedAt: now}
	if err := store.Playlists.Create(ctx, pl); err != nil {
		t.Fatal(err)
	}
	if err := store.Playlists.ReplaceTracks(ctx, pl.ID, ids, owner.ID); err != nil {
		t.Fatal(err)
	}

	// Get → first 4 covers in playlist order, album-id fallback applied.
	got, err := store.Playlists.Get(ctx, pl.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := covers[:4]
	if len(got.CoverArts) != 4 {
		t.Fatalf("expected 4 mosaic covers, got %d: %v", len(got.CoverArts), got.CoverArts)
	}
	for i := range want {
		if got.CoverArts[i] != want[i] {
			t.Fatalf("cover %d = %q, want %q (full %v)", i, got.CoverArts[i], want[i], got.CoverArts)
		}
	}

	// List path returns the same mosaic.
	lists, err := store.Playlists.ListPublicByOwner(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 1 || len(lists[0].CoverArts) != 4 || lists[0].CoverArts[0] != want[0] {
		t.Fatalf("list mosaic wrong: %+v", lists)
	}

	// An empty playlist exposes no covers.
	empty := models.Playlist{ID: uuid.NewString(), Name: "Empty", OwnerID: owner.ID, Public: true, CreatedAt: now, UpdatedAt: now}
	if err := store.Playlists.Create(ctx, empty); err != nil {
		t.Fatal(err)
	}
	eg, _ := store.Playlists.Get(ctx, empty.ID)
	if len(eg.CoverArts) != 0 {
		t.Fatalf("empty playlist should have no covers, got %v", eg.CoverArts)
	}
}
