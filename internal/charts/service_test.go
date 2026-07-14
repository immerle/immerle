package charts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/testutil"
)

// stubKworb serves a fixed chart response per slug (404 for any other),
// standing in for kworb-net-api's raw GitHub content.
func stubKworb(t *testing.T, bySlug map[string][]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		slug := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), "_weekly.json")
		entries, ok := bySlug[slug]
		if !ok {
			http.NotFound(w, r)
			return
		}
		chart := make([]kworbEntry, 0, len(entries))
		for _, e := range entries {
			chart = append(chart, kworbEntry{ArtistAndTitle: e})
		}
		_ = json.NewEncoder(w).Encode(kworbChart{Chart: chart})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestSyncNowMaterializesPublicPlaylists(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()

	owner := models.User{ID: uuid.NewString(), Username: "admin", PasswordHash: "x", IsAdmin: true}
	if err := store.Users.Create(ctx, owner); err != nil {
		t.Fatal(err)
	}

	srv := stubKworb(t, map[string][]string{
		"fr": {"PLK - Pocahontas", "Aya Nakamura - Sexy Nana (w/ La Rvfleuze)"},
	})

	svc := New(store.Playlists, srv.URL, t.TempDir(), nil, testutil.NewLogger())
	svc.charts = []Chart{{Slug: "fr", Name: "Top 50 France"}}
	svc.SetOwner(owner.ID)

	n, err := svc.SyncNow(ctx)
	if err != nil {
		t.Fatalf("SyncNow: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 chart synced, got %d", n)
	}

	p, err := store.Playlists.FindFederated(ctx, sourceInstanceID, "fr_weekly")
	if err != nil {
		t.Fatalf("playlist not created: %v", err)
	}
	if !p.Federated || !p.Public {
		t.Fatalf("expected a public, federated playlist, got %+v", p)
	}
	if p.Name != "Top 50 France" {
		t.Fatalf("name = %q, want %q", p.Name, "Top 50 France")
	}
	if p.CoverArt == "" {
		t.Fatal("expected a generated cover")
	}

	tracks, err := store.Playlists.Tracks(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(tracks))
	}
	if !tracks[0].Unresolved || tracks[0].ArtistName != "PLK" || tracks[0].Title != "Pocahontas" {
		t.Fatalf("track 0 = %+v", tracks[0])
	}
	if !tracks[1].Unresolved || tracks[1].ArtistName != "Aya Nakamura" || tracks[1].Title != "Sexy Nana" {
		t.Fatalf("track 1 = %+v", tracks[1])
	}

	// Re-syncing must update in place, not duplicate.
	if _, err := svc.SyncNow(ctx); err != nil {
		t.Fatal(err)
	}
	visible, err := store.Playlists.ListPublic(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, v := range visible {
		if v.SourceInstanceID == sourceInstanceID && v.SourceExternalID == "fr_weekly" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 playlist after re-sync, got %d", count)
	}
}

func TestSyncNowSkipsFailingChartButSyncsOthers(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()

	owner := models.User{ID: uuid.NewString(), Username: "admin", PasswordHash: "x", IsAdmin: true}
	if err := store.Users.Create(ctx, owner); err != nil {
		t.Fatal(err)
	}

	srv := stubKworb(t, map[string][]string{"fr": {"Artist - Title"}})

	svc := New(store.Playlists, srv.URL, t.TempDir(), nil, testutil.NewLogger())
	svc.charts = []Chart{
		{Slug: "does-not-exist", Name: "Broken"},
		{Slug: "fr", Name: "Top 50 France"},
	}
	svc.SetOwner(owner.ID)

	n, err := svc.SyncNow(ctx)
	if err != nil {
		t.Fatalf("SyncNow should not fail outright: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 chart synced (the other failing), got %d", n)
	}
}

func TestSplitArtistAndTitle(t *testing.T) {
	cases := []struct {
		in            string
		artist, title string
	}{
		{"PLK - Pocahontas", "PLK", "Pocahontas"},
		{"Aya Nakamura - Sexy Nana (w/ La Rvfleuze)", "Aya Nakamura", "Sexy Nana"},
		{"Artist - Song (W/ Someone)", "Artist", "Song"},
		{"Artist - Song - Remix", "Artist", "Song - Remix"},
		{"no separator here", "", ""},
	}
	for _, c := range cases {
		artist, title := splitArtistAndTitle(c.in)
		if artist != c.artist || title != c.title {
			t.Errorf("splitArtistAndTitle(%q) = (%q, %q), want (%q, %q)", c.in, artist, title, c.artist, c.title)
		}
	}
}
