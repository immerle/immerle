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

func TestSmartPlaylistSonicRules(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	user := models.User{ID: uuid.NewString(), Username: "u", PasswordHash: "x", CreatedAt: now}
	if err := store.Users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Artist", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Album", ArtistID: artistID, CreatedAt: now})
	mk := func(title string, tagBPM int, sonic *models.TrackSonic) string {
		id, err := store.Catalog.UpsertTrack(ctx, models.Track{ID: uuid.NewString(), Title: title, AlbumID: albumID, ArtistID: artistID,
			BPM: tagBPM, Path: uuid.NewString(), CreatedAt: now, UpdatedAt: now})
		if err != nil {
			t.Fatal(err)
		}
		if sonic != nil {
			sonic.TrackID, sonic.AlgoVersion, sonic.AnalyzedAt = id, 1, now
			if err := store.Sonic.Save(ctx, *sonic); err != nil {
				t.Fatal(err)
			}
		}
		return id
	}
	banger := mk("Banger", 0, &models.TrackSonic{BPM: 127.6, Energy: 0.91, Danceability: 0.8, Camelot: "8A", Mode: "minor"})
	ballad := mk("Ballad", 72, &models.TrackSonic{BPM: 140, Energy: 0.2, Danceability: 0.3, Camelot: "8B", Mode: "major"})
	mk("Not analyzed", 0, nil)

	only := func(field, op, value, want string) {
		t.Helper()
		got, err := store.SmartPlaylists.Evaluate(ctx, models.SmartRules{Conditions: []models.SmartCondition{{Field: field, Op: op, Value: value}}}, user.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != want {
			t.Errorf("%s %s %s: got %d tracks, want only %s", field, op, value, len(got), want)
		}
	}
	only("energy", "gte", "90", banger)
	only("danceability", "lte", "30", ballad)
	only("camelot", "is", "8A", banger)
	only("mode", "is", "major", ballad)
	only("bpm", "gte", "128", banger) // rounded analyzed tempo, no tag
	only("bpm", "lte", "100", ballad) // the tag (72) wins over the analysis (140)
}
