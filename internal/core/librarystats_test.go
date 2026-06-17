package core

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gossignol/gossignol/internal/models"
	"github.com/gossignol/gossignol/internal/testutil"
)

func TestLibraryStatsCachesTotals(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	svc := NewLibraryStatsService(store.Catalog, testutil.NewLogger())

	// Empty library → zero snapshot.
	if got := svc.Get(); got.Tracks != 0 || got.TotalSize != 0 {
		t.Fatalf("expected empty snapshot, got %+v", got)
	}

	// Seed two tracks with known sizes/durations.
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A", CreatedAt: time.Now()})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: time.Now()})
	for i, sz := range []int64{1000, 2500} {
		_, err := store.Catalog.UpsertTrack(ctx, models.Track{
			ID: uuid.NewString(), Title: "T", AlbumID: albumID, ArtistID: artistID,
			Path: "/x" + string(rune('a'+i)) + ".mp3", Size: sz, Duration: 100 + i,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	stats, err := svc.Refresh(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Tracks != 2 || stats.TotalSize != 3500 {
		t.Fatalf("refresh wrong: %+v", stats)
	}
	if stats.TotalDuration != 201 {
		t.Fatalf("expected total duration 201, got %d", stats.TotalDuration)
	}
	if stats.UpdatedAt.IsZero() {
		t.Fatal("updatedAt should be set")
	}

	// Cached snapshot matches the last refresh without re-querying.
	if got := svc.Get(); got.TotalSize != 3500 || got.Tracks != 2 {
		t.Fatalf("cache mismatch: %+v", got)
	}
}
