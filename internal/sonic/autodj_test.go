package sonic

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/testutil"
)

func TestKeyCostFollowsTheCamelotWheel(t *testing.T) {
	cases := []struct {
		a, b string
		want float64
	}{
		{"8A", "8A", 0},
		{"8A", "9A", 0},
		{"12A", "1A", 0}, // the wheel wraps
		{"8A", "8B", 0},  // relative major/minor
		{"8A", "10A", 0.5},
		{"8A", "3B", 1},
		{"8A", "", 0.5}, // unknown key
	}
	for _, c := range cases {
		if got := keyCost(c.a, c.b); got != c.want {
			t.Errorf("keyCost(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestTempoCostTreatsHalfAndDoubleAsMatching(t *testing.T) {
	if got := tempoCost(128, 64); got != 0 {
		t.Errorf("128 vs 64: %v", got)
	}
	if near, far := tempoCost(128, 130), tempoCost(128, 150); near >= far {
		t.Errorf("130 should be closer to 128 than 150: %v vs %v", near, far)
	}
}

// ramp is a set of tracks on a smooth tempo/energy gradient in one key, so
// the smoothest order is the gradient itself.
func ramp(n int) []persistence.SonicSummary {
	out := make([]persistence.SonicSummary, n)
	for i := range out {
		out[i] = persistence.SonicSummary{TrackID: fmt.Sprintf("t%02d", i), BPM: 100 + 2*float64(i), Energy: float64(i) / float64(n), Camelot: "8A"}
	}
	return out
}

func TestOrderForDJRebuildsTheGradient(t *testing.T) {
	tracks := ramp(20)
	want := make([]string, len(tracks))
	for i, tr := range tracks {
		want[i] = tr.TrackID
	}
	shuffled := slices.Clone(tracks)
	rand.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })

	if got := OrderForDJ(shuffled, false); !slices.Equal(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestOrderForDJKeepsTheTrackPlayingFirst(t *testing.T) {
	tracks := ramp(10)
	slices.Reverse(tracks) // t09 (the most energetic) is playing
	got := OrderForDJ(tracks, true)
	if got[0] != "t09" || len(got) != 10 {
		t.Errorf("got %v", got)
	}
	if got[1] != "t08" {
		t.Errorf("should continue with its closest neighbour, got %v", got)
	}
}

func TestAutoDJPlacesUnanalyzedTracksLast(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "B", ArtistID: artistID, CreatedAt: now})
	mk := func(bpm float64) string {
		id, err := store.Catalog.UpsertTrack(ctx, models.Track{ID: uuid.NewString(), Title: "x", AlbumID: albumID, ArtistID: artistID,
			Path: uuid.NewString(), CreatedAt: now, UpdatedAt: now})
		if err != nil {
			t.Fatal(err)
		}
		if bpm > 0 {
			if err := store.Sonic.Save(ctx, models.TrackSonic{TrackID: id, AlgoVersion: AlgoVersion, BPM: bpm, Energy: bpm / 200, Camelot: "8A", AnalyzedAt: now}); err != nil {
				t.Fatal(err)
			}
		}
		return id
	}
	fast, unknown, slow, mid := mk(140), mk(0), mk(100), mk(120)
	svc := NewService(store.Sonic, "ffmpeg", func() models.SonicRuntime { return models.SonicRuntime{} }, func() bool { return false },
		slog.New(slog.NewTextHandler(io.Discard, nil)))

	got, err := svc.AutoDJ(ctx, []string{fast, unknown, slow, mid}, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{slow, mid, fast, unknown}; !slices.Equal(got, want) {
		t.Errorf("got %v, want calmest first then the unanalyzed track", got)
	}

	// A pinned track that isn't analyzed stays first; a duplicate is kept.
	got, _ = svc.AutoDJ(ctx, []string{unknown, fast, slow, fast}, true)
	if len(got) != 4 || got[0] != unknown {
		t.Errorf("got %v", got)
	}
}
