package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/testutil"
)

func TestSonicPendingTracksLifecycle(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	artistID, err := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Artist", CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	albumID, err := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Album", ArtistID: artistID, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	track := models.Track{ID: uuid.NewString(), Title: "Song", AlbumID: albumID, ArtistID: artistID,
		Path: "/music/song.flac", FileHash: "h1", Duration: 200, CreatedAt: now, UpdatedAt: now}
	if track.ID, err = store.Catalog.UpsertTrack(ctx, track); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Catalog.UpsertTrack(ctx, models.Track{ID: uuid.NewString(), Title: "Remote", AlbumID: albumID,
		ArtistID: artistID, Remote: true, Provider: "jamendo", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	pendingIDs := func(version int) []string {
		t.Helper()
		tracks, err := store.Sonic.PendingTracks(ctx, version, 10)
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, tr := range tracks {
			ids = append(ids, tr.ID)
		}
		return ids
	}

	if got := pendingIDs(1); len(got) != 1 || got[0] != track.ID {
		t.Fatalf("only the local track should be pending, got %v", got)
	}

	vec := []float32{0.5, -1.25, 3}
	if err := store.Sonic.Save(ctx, models.TrackSonic{TrackID: track.ID, FileHash: "h1", AlgoVersion: 1,
		BPM: 128, Camelot: "8A", Vector: vec, AnalyzedAt: now}); err != nil {
		t.Fatal(err)
	}
	if got := pendingIDs(1); len(got) != 0 {
		t.Fatalf("analyzed track still pending: %v", got)
	}
	if got := pendingIDs(2); len(got) != 1 {
		t.Fatalf("a newer algorithm should re-analyze, got %v", got)
	}
	saved, err := store.Sonic.Get(ctx, track.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.BPM != 128 || saved.Camelot != "8A" || len(saved.Vector) != 3 || saved.Vector[1] != -1.25 {
		t.Fatalf("round-trip mismatch: %+v", saved)
	}

	// A failed analysis is not retried, but counted.
	if err := store.Sonic.Save(ctx, models.TrackSonic{TrackID: track.ID, FileHash: "h1", AlgoVersion: 1,
		Error: "ffmpeg: boom", AnalyzedAt: now}); err != nil {
		t.Fatal(err)
	}
	if got := pendingIDs(1); len(got) != 0 {
		t.Fatalf("failed track retried: %v", got)
	}
	if st, err := store.Sonic.Stats(ctx, 1); err != nil || st.Total != 1 || st.Analyzed != 0 || st.Failed != 1 {
		t.Fatalf("stats: %+v, %v", st, err)
	}

	// A replaced file is analyzed again.
	track.FileHash = "h2"
	if _, err := store.Catalog.UpsertTrack(ctx, track); err != nil {
		t.Fatal(err)
	}
	if got := pendingIDs(1); len(got) != 1 {
		t.Fatalf("changed file should be pending, got %v", got)
	}
}
