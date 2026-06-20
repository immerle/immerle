package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/testutil"
)

// An album with no cover of its own falls back to its first track's cover
// (ordered by disc/track number); none available stays empty for the placeholder.
func TestGetAlbumCoverFallsBackToFirstTrack(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Artist", CreatedAt: now})

	mkAlbum := func() string {
		id, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: uuid.NewString(), ArtistID: artistID, CreatedAt: now})
		return id
	}
	mkTrack := func(albumID, cover string, disc, no int) {
		if _, err := store.Catalog.UpsertTrack(ctx, models.Track{
			ID: uuid.NewString(), Title: uuid.NewString(), AlbumID: albumID, ArtistID: artistID,
			DiscNo: disc, TrackNo: no, CoverArt: cover, Path: uuid.NewString(), CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Album with no cover; track 2 has a cover, track 1 (lower number) does not.
	a := mkAlbum()
	mkTrack(a, "", 1, 1)
	mkTrack(a, "cover-of-track-2", 1, 2)
	if got, _ := store.Catalog.GetAlbum(ctx, a); got.CoverArt != "cover-of-track-2" {
		t.Fatalf("expected fallback to first track with a cover, got %q", got.CoverArt)
	}

	// Album with its own cover keeps it, ignoring track covers.
	b, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: uuid.NewString(), ArtistID: artistID, CoverArt: "album-cover", CreatedAt: now})
	mkTrack(b, "track-cover", 1, 1)
	if got, _ := store.Catalog.GetAlbum(ctx, b); got.CoverArt != "album-cover" {
		t.Fatalf("expected album's own cover, got %q", got.CoverArt)
	}

	// No covers anywhere stays empty (frontend renders the placeholder).
	c := mkAlbum()
	mkTrack(c, "", 1, 1)
	if got, _ := store.Catalog.GetAlbum(ctx, c); got.CoverArt != "" {
		t.Fatalf("expected empty cover, got %q", got.CoverArt)
	}
}
