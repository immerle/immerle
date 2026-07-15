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

	svc := New(store.Catalog, store.Genres, store.Wrapped, store.Annotations, store.Users, store.Playlists, testutil.NewLogger())
	svc.SetOwner(owner.ID)

	n, err := svc.SyncNow(ctx)
	if err != nil {
		t.Fatalf("SyncNow: %v", err)
	}
	if n != 3 { // Rock + 1990s + the owner's own "Aléatoire" random mix
		t.Fatalf("expected 3 auto-playlists synced, got %d", n)
	}

	rock, err := store.Playlists.FindFederated(ctx, sourceGenre, "Rock")
	if err != nil {
		t.Fatalf("Rock playlist not created: %v", err)
	}
	if !rock.Public || !rock.Federated {
		t.Fatalf("expected a public, federated playlist, got %+v", rock)
	}
	if want := models.GeneratorCoverID(coverParams("Rock", musicNoteIcon)); rock.CoverArt != want {
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

// TestSyncNowMaterializesPersonalListsAsPrivatePlaylists covers the personal
// lists (top-month, forgotten favorites, and the activity-independent random
// mix): real playlists (not a live-computed view), owned by their user,
// private (not searchable/visible to anyone else), and deliberately not
// subscribed — found via GET /me/custom-playlists, not the owner's library.
func TestSyncNowMaterializesPersonalListsAsPrivatePlaylists(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	admin := models.User{ID: uuid.NewString(), Username: "admin", PasswordHash: "x", IsAdmin: true}
	if err := store.Users.Create(ctx, admin); err != nil {
		t.Fatal(err)
	}
	listener := models.User{ID: uuid.NewString(), Username: "listener", PasswordHash: "x"}
	if err := store.Users.Create(ctx, listener); err != nil {
		t.Fatal(err)
	}
	idle := models.User{ID: uuid.NewString(), Username: "idle", PasswordHash: "x"}
	if err := store.Users.Create(ctx, idle); err != nil {
		t.Fatal(err)
	}

	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: now})
	newTrack := func(title string) string {
		id, _ := store.Catalog.UpsertTrack(ctx, models.Track{ID: uuid.NewString(), Title: title, AlbumID: albumID, ArtistID: artistID, Path: uuid.NewString(), CreatedAt: now, UpdatedAt: now})
		return id
	}
	playedThisMonth := newTrack("Played This Month")
	forgotten := newTrack("Forgotten Favorite")

	if err := store.Scrobbles.Insert(ctx, models.Scrobble{ID: uuid.NewString(), UserID: listener.ID, TrackID: playedThisMonth, PlayedAt: now, Submitted: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.Annotations.SetStarred(ctx, listener.ID, models.ItemTrack, forgotten, true); err != nil {
		t.Fatal(err)
	}

	svc := New(store.Catalog, store.Genres, store.Wrapped, store.Annotations, store.Users, store.Playlists, testutil.NewLogger())
	svc.SetOwner(admin.ID)

	if _, err := svc.SyncNow(ctx); err != nil {
		t.Fatalf("SyncNow: %v", err)
	}

	topMonth, err := store.Playlists.FindFederated(ctx, SourceTopMonth, listener.ID)
	if err != nil {
		t.Fatalf("top-month playlist not created for listener: %v", err)
	}
	if topMonth.Public {
		t.Fatalf("personal list must be private, got %+v", topMonth)
	}
	if !topMonth.Federated {
		t.Fatalf("personal list must be federated (read-only), got %+v", topMonth)
	}
	if topMonth.OwnerID != listener.ID {
		t.Fatalf("owner = %q, want the listener", topMonth.OwnerID)
	}
	tracks, err := store.Playlists.Tracks(ctx, topMonth.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 1 || tracks[0].ID != playedThisMonth {
		t.Fatalf("expected just the track played this month, got %+v", tracks)
	}

	// Deliberately NOT subscribed: it must not depend on the subscribe/"like"
	// mechanism (unsubscribing must never lose access to your own personal
	// list — see GET /me/custom-playlists, which looks it up directly instead).
	visible, err := store.Playlists.ListVisible(ctx, listener.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range visible {
		if v.ID == topMonth.ID {
			t.Fatalf("personal list must not rely on a subscription to be found, got it in ListVisible: %+v", v)
		}
	}

	// Forgotten favorites: the never-played starred track.
	forgottenList, err := store.Playlists.FindFederated(ctx, SourceForgotten, listener.ID)
	if err != nil {
		t.Fatalf("forgotten-favorites playlist not created: %v", err)
	}
	forgottenTracks, err := store.Playlists.Tracks(ctx, forgottenList.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(forgottenTracks) != 1 || forgottenTracks[0].ID != forgotten {
		t.Fatalf("expected just the forgotten favorite, got %+v", forgottenTracks)
	}

	// The idle user has no listening history at all: no top-month/forgotten
	// lists (those depend on activity) — but still gets a random mix, since
	// "Aléatoire" only depends on the catalog having tracks, not on the user.
	if _, err := store.Playlists.FindFederated(ctx, SourceTopMonth, idle.ID); err == nil {
		t.Fatal("an inactive user must not get a top-month playlist")
	}
	idleRandom, err := store.Playlists.FindFederated(ctx, SourceRandom, idle.ID)
	if err != nil {
		t.Fatalf("random playlist not created for the idle user: %v", err)
	}
	if idleRandom.Public || !idleRandom.Federated || idleRandom.OwnerID != idle.ID {
		t.Fatalf("expected a private, federated, idle-owned random playlist, got %+v", idleRandom)
	}
	randomTracks, err := store.Playlists.Tracks(ctx, idleRandom.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(randomTracks) != 2 { // only 2 tracks exist in the whole catalog
		t.Fatalf("expected all 2 catalog tracks in the random mix, got %d", len(randomTracks))
	}
}

// TestSyncNowMaterializesWeeklyTrendingChart covers the shared (public,
// single-instance) community chart: most-scrobbled tracks across every user
// in the last trendingWindowDays, not any one user's own history.
func TestSyncNowMaterializesWeeklyTrendingChart(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	owner := models.User{ID: uuid.NewString(), Username: "admin", PasswordHash: "x", IsAdmin: true}
	if err := store.Users.Create(ctx, owner); err != nil {
		t.Fatal(err)
	}
	listener := models.User{ID: uuid.NewString(), Username: "listener", PasswordHash: "x"}
	if err := store.Users.Create(ctx, listener); err != nil {
		t.Fatal(err)
	}

	seedTracks(t, store, minTracks, "", 0)
	tracks, err := store.Catalog.RandomTracks(ctx, minTracks, "", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, tr := range tracks {
		if err := store.Scrobbles.Insert(ctx, models.Scrobble{ID: uuid.NewString(), UserID: listener.ID, TrackID: tr.ID, PlayedAt: now, Submitted: true}); err != nil {
			t.Fatal(err)
		}
	}

	svc := New(store.Catalog, store.Genres, store.Wrapped, store.Annotations, store.Users, store.Playlists, testutil.NewLogger())
	svc.SetOwner(owner.ID)

	if _, err := svc.SyncNow(ctx); err != nil {
		t.Fatalf("SyncNow: %v", err)
	}

	trending, err := store.Playlists.FindFederated(ctx, sourceTrending, trendingExternalID)
	if err != nil {
		t.Fatalf("trending playlist not created: %v", err)
	}
	if !trending.Public || !trending.Federated {
		t.Fatalf("expected a public, federated playlist, got %+v", trending)
	}
	trendingTracks, err := store.Playlists.Tracks(ctx, trending.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(trendingTracks) != minTracks {
		t.Fatalf("expected %d tracks in the trending chart, got %d", minTracks, len(trendingTracks))
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
