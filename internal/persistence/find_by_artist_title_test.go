package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/testutil"
)

// TestFindByArtistTitleDeprioritizesAlternateVersions covers a real bug: a
// library can legitimately contain both the original and an alternate
// version (e.g. a "Gospel" cover) of the same song under the same
// artist+title. FindByArtistTitle runs before any remote provider search in
// track auto-resolution, so if it picks the alternate version, the caller
// never even reaches the remote-match scoring that already deprioritizes
// alternate versions — this must apply the same disambiguation itself.
func TestFindByArtistTitleDeprioritizesAlternateVersions(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	artistID, err := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Mauvais djo", CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	albumID, err := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Single", ArtistID: artistID, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}

	// Insert the alternate (Gospel) version first, so a naive "first row"
	// pick would return it.
	if _, err := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "Pilé", AlbumID: albumID, ArtistID: artistID,
		AlbumName: "Pilé (Gospel Version)", Path: "/music/pile-gospel.mp3", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "Pilé", AlbumID: albumID, ArtistID: artistID,
		AlbumName: "Single", Path: "/music/pile-original.mp3", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	track, found, err := store.Catalog.FindByArtistTitle(ctx, "Mauvais djo", "Pilé")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected a match")
	}
	if track.AlbumName != "Single" {
		t.Fatalf("expected the original (non-Gospel) track, got %+v", track)
	}
}
