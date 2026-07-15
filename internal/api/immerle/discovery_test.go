package immerle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	chi "github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/testutil"
)

// newDiscoveryEnv wires just what the personal discovery lists need (no
// ffmpeg/audio scanning — tracks are inserted directly, like the other
// persistence-level tests) plus a logged-in admin token.
func newDiscoveryEnv(t *testing.T) (*httptest.Server, string, *persistence.Store) {
	t.Helper()
	store := testutil.NewStore(t)
	ctx := context.Background()
	auth, _ := core.NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	if _, err := auth.CreateUser(ctx, "admin", "adminpw", "", "", true); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(Deps{
		Auth:        auth,
		Users:       store.Users,
		Catalog:     store.Catalog,
		Annotations: store.Annotations,
		Wrapped:     store.Wrapped,
		Playlists:   store.Playlists,
		Logger:      testutil.NewLogger(),
	})
	mux := chi.NewRouter()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, login(t, srv, "admin"), store
}

func TestTopTracksThisMonth(t *testing.T) {
	srv, token, store := newDiscoveryEnv(t)
	ctx := context.Background()
	now := time.Now()

	admin, _ := store.Users.GetByUsername(ctx, "admin")
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: now})
	newTrack := func(title string) string {
		id, _ := store.Catalog.UpsertTrack(ctx, models.Track{ID: uuid.NewString(), Title: title, AlbumID: albumID, ArtistID: artistID, Path: uuid.NewString(), CreatedAt: now, UpdatedAt: now})
		return id
	}
	popular := newTrack("Popular This Month")
	oldHit := newTrack("Played Last Month")

	scrobble := func(trackID string, at time.Time) {
		if err := store.Scrobbles.Insert(ctx, models.Scrobble{ID: uuid.NewString(), UserID: admin.ID, TrackID: trackID, PlayedAt: at, Submitted: true}); err != nil {
			t.Fatal(err)
		}
	}
	thisMonth := time.Date(now.Year(), now.Month(), 5, 12, 0, 0, 0, time.UTC)
	if thisMonth.After(now) {
		thisMonth = now
	}
	scrobble(popular, thisMonth)
	scrobble(popular, thisMonth)
	scrobble(oldHit, now.AddDate(0, -2, 0))

	var out struct {
		Songs []songView `json:"songs"`
	}
	if st := getJSON(t, srv, token, "/me/top-tracks", &out); st != http.StatusOK {
		t.Fatalf("status %d", st)
	}
	if len(out.Songs) != 1 || out.Songs[0].ID != popular {
		t.Fatalf("expected only this month's popular track, got %+v", out.Songs)
	}
}

func TestForgottenFavoritesEndpoint(t *testing.T) {
	srv, token, store := newDiscoveryEnv(t)
	ctx := context.Background()
	now := time.Now()

	admin, _ := store.Users.GetByUsername(ctx, "admin")
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: now})
	newTrack := func(title string) string {
		id, _ := store.Catalog.UpsertTrack(ctx, models.Track{ID: uuid.NewString(), Title: title, AlbumID: albumID, ArtistID: artistID, Path: uuid.NewString(), CreatedAt: now, UpdatedAt: now})
		return id
	}
	forgotten := newTrack("Forgotten Favorite")
	active := newTrack("Active Favorite")

	if err := store.Annotations.SetStarred(ctx, admin.ID, models.ItemTrack, forgotten, true); err != nil {
		t.Fatal(err)
	}
	if err := store.Annotations.SetStarred(ctx, admin.ID, models.ItemTrack, active, true); err != nil {
		t.Fatal(err)
	}
	if err := store.Annotations.IncrementPlay(ctx, admin.ID, models.ItemTrack, active, now.AddDate(0, 0, -1)); err != nil {
		t.Fatal(err)
	}

	var out struct {
		Songs []songView `json:"songs"`
	}
	if st := getJSON(t, srv, token, "/me/forgotten-favorites", &out); st != http.StatusOK {
		t.Fatalf("status %d", st)
	}
	if len(out.Songs) != 1 || out.Songs[0].ID != forgotten {
		t.Fatalf("expected only the never-played favorite, got %+v", out.Songs)
	}
}
