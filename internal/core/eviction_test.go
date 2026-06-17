package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/testutil"
)

// seedProviderTrack creates a track on disk with a completed download job (i.e.
// "added by a provider") and returns its id + path.
func seedProviderTrack(t *testing.T, store *persistence.Store, dir, title string) (string, string) {
	t.Helper()
	ctx := context.Background()
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A", CreatedAt: time.Now()})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: time.Now()})
	path := filepath.Join(dir, title+".mp3")
	if err := os.WriteFile(path, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	id, _ := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: title, AlbumID: albumID, ArtistID: artistID, Path: path,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	// Mark it as provider-downloaded via a completed job.
	_, _ = store.Downloads.Enqueue(ctx, models.DownloadJob{
		ID: uuid.NewString(), Provider: "deezer", ProviderTrackID: "p-" + title,
		Status: models.DownloadQueued, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	job, _ := store.Downloads.GetByProviderTrack(ctx, "deezer", "p-"+title)
	_ = store.Downloads.Complete(ctx, job.ID, id)
	return id, path
}

func TestEvictionRemovesOnlyUnusedProviderTracks(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	dir := t.TempDir()

	u := models.User{ID: uuid.NewString(), Username: "u", PasswordHash: "x", CreatedAt: time.Now()}
	_ = store.Users.Create(ctx, u)

	// 1) Unused provider track → should be evicted.
	unusedID, unusedPath := seedProviderTrack(t, store, dir, "unused")

	// 2) Provider track played recently → kept.
	playedID, playedPath := seedProviderTrack(t, store, dir, "played")
	_ = store.Annotations.IncrementPlay(ctx, u.ID, models.ItemTrack, playedID, time.Now())

	// 3) Provider track starred → kept.
	starredID, starredPath := seedProviderTrack(t, store, dir, "starred")
	_ = store.Annotations.SetStarred(ctx, u.ID, models.ItemTrack, starredID, true)

	// 4) Provider track in a playlist → kept.
	inPlaylistID, inPlaylistPath := seedProviderTrack(t, store, dir, "inplaylist")
	pl := models.Playlist{ID: uuid.NewString(), Name: "P", OwnerID: u.ID, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	_ = store.Playlists.Create(ctx, pl)
	_ = store.Playlists.ReplaceTracks(ctx, pl.ID, []string{inPlaylistID}, u.ID)

	// 5) Manually-added track (no download job), unused → must NOT be evicted.
	manualArtist, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "M", CreatedAt: time.Now()})
	manualAlbum, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "MAl", ArtistID: manualArtist, CreatedAt: time.Now()})
	manualPath := filepath.Join(dir, "manual.mp3")
	_ = os.WriteFile(manualPath, []byte("audio"), 0o644)
	manualID, _ := store.Catalog.UpsertTrack(ctx, models.Track{ID: uuid.NewString(), Title: "manual", AlbumID: manualAlbum, ArtistID: manualArtist, Path: manualPath, CreatedAt: time.Now(), UpdatedAt: time.Now()})

	evictor := NewEvictor(store.Catalog, store.Downloads, func() bool { return true }, func() time.Duration { return 30 * 24 * time.Hour }, time.Hour, testutil.NewLogger())
	removed, err := evictor.Sweep(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("expected exactly 1 eviction, got %d", removed)
	}

	// Unused provider track: file + row gone.
	if _, err := os.Stat(unusedPath); !os.IsNotExist(err) {
		t.Fatal("unused provider file should be deleted")
	}
	if _, err := store.Catalog.GetTrack(ctx, unusedID); err == nil {
		t.Fatal("unused provider track row should be deleted")
	}

	// Everything else survives.
	for _, keep := range []struct {
		id, path string
	}{{playedID, playedPath}, {starredID, starredPath}, {inPlaylistID, inPlaylistPath}, {manualID, manualPath}} {
		if _, err := os.Stat(keep.path); err != nil {
			t.Fatalf("track %s file should be kept: %v", keep.id, err)
		}
		if _, err := store.Catalog.GetTrack(ctx, keep.id); err != nil {
			t.Fatalf("track %s row should be kept: %v", keep.id, err)
		}
	}
}

func TestEvictionRespectsRetentionWindow(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	dir := t.TempDir()
	u := models.User{ID: uuid.NewString(), Username: "u", PasswordHash: "x", CreatedAt: time.Now()}
	_ = store.Users.Create(ctx, u)

	id, path := seedProviderTrack(t, store, dir, "old")
	// Played 40 days ago — outside a 30-day window.
	_ = store.Annotations.IncrementPlay(ctx, u.ID, models.ItemTrack, id, time.Now().Add(-40*24*time.Hour))

	evictor := NewEvictor(store.Catalog, store.Downloads, func() bool { return true }, func() time.Duration { return 30 * 24 * time.Hour }, time.Hour, testutil.NewLogger())
	removed, _ := evictor.Sweep(ctx)
	if removed != 1 {
		t.Fatalf("a track last played 40d ago should be evicted, removed=%d", removed)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("file should be gone")
	}

	// A fresh play (within the window) keeps it.
	id2, path2 := seedProviderTrack(t, store, dir, "recent")
	_ = store.Annotations.IncrementPlay(ctx, u.ID, models.ItemTrack, id2, time.Now().Add(-10*24*time.Hour))
	removed, _ = evictor.Sweep(ctx)
	if removed != 0 {
		t.Fatalf("a track played 10d ago should be kept, removed=%d", removed)
	}
	if _, err := os.Stat(path2); err != nil {
		t.Fatal("recently played file should be kept")
	}
}
