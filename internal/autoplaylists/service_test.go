package autoplaylists

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/testutil"
)

// seedTracks creates n tracks under one artist/album, each tagged with genre
// (if non-empty) and year (if non-zero) — enough fixture data to cross (or
// stay under) minTracks for a genre/decade auto-playlist.
func seedTracks(t *testing.T, store *persistence.Store, n int, genre string, year int) {
	t.Helper()
	ctx := context.Background()
	now := time.Now()
	artistID, err := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Artist " + uuid.NewString(), CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	albumID, err := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Album", ArtistID: artistID, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		_, err := store.Catalog.UpsertTrack(ctx, models.Track{
			ID: uuid.NewString(), Title: uuid.NewString(), AlbumID: albumID, ArtistID: artistID,
			Genre: genre, Year: year, Path: uuid.NewString(), CreatedAt: now, UpdatedAt: now,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestSyncNowMaterializesGenreAndDecadePlaylists(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()

	owner := models.User{ID: uuid.NewString(), Username: "admin", PasswordHash: "x", IsAdmin: true}
	if err := store.Users.Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Genres.Upsert(ctx, uuid.NewString(), "Rock"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Genres.Upsert(ctx, uuid.NewString(), "Jazz"); err != nil {
		t.Fatal(err)
	}
	seedTracks(t, store, minTracks, "Rock", 1995)   // enough for both a genre and a decade playlist
	seedTracks(t, store, minTracks-1, "Jazz", 1960) // below threshold for either

	svc := New(store.Catalog, store.Genres, store.Playlists, testutil.NewLogger())
	svc.SetOwner(owner.ID)

	n, err := svc.SyncNow(ctx)
	if err != nil {
		t.Fatalf("SyncNow: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 auto-playlists synced (Rock + 1990s), got %d", n)
	}

	rock, err := store.Playlists.FindFederated(ctx, sourceGenre, "Rock")
	if err != nil {
		t.Fatalf("Rock playlist not created: %v", err)
	}
	if !rock.Public || !rock.Federated {
		t.Fatalf("expected a public, federated playlist, got %+v", rock)
	}
	if want := models.GeneratorCoverID(coverParams("Rock")); rock.CoverArt != want {
		t.Fatalf("coverArt = %q, want %q", rock.CoverArt, want)
	}
	tracks, err := store.Playlists.Tracks(ctx, rock.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != minTracks {
		t.Fatalf("expected %d tracks in the Rock playlist, got %d", minTracks, len(tracks))
	}

	decade, err := store.Playlists.FindFederated(ctx, sourceDecade, "1990s")
	if err != nil {
		t.Fatalf("1990s playlist not created: %v", err)
	}
	if !decade.Public || !decade.Federated {
		t.Fatalf("expected a public, federated playlist, got %+v", decade)
	}

	if _, err := store.Playlists.FindFederated(ctx, sourceGenre, "Jazz"); err == nil {
		t.Fatal("Jazz has too few tracks and must not get a playlist")
	}
	if _, err := store.Playlists.FindFederated(ctx, sourceDecade, "1960s"); err == nil {
		t.Fatal("the 1960s has too few tracks and must not get a playlist")
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
		if v.SourceInstanceID == sourceGenre && v.SourceExternalID == "Rock" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 Rock playlist after re-sync, got %d", count)
	}
}

func TestDecadesSpansThe1950sToTheCurrentDecade(t *testing.T) {
	d := decades()
	if len(d) == 0 || d[0].from != 1950 || d[0].label != "1950s" {
		t.Fatalf("expected to start at the 1950s, got %+v", d[0])
	}
	currentDecade := (time.Now().Year() / 10) * 10
	last := d[len(d)-1]
	if last.from != currentDecade {
		t.Fatalf("expected to end at the current decade (%d), got %+v", currentDecade, last)
	}
}
