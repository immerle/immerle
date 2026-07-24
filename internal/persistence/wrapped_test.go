package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/testutil"
)

func TestWrappedAggregatesByYear(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	user := models.User{ID: uuid.NewString(), Username: "u", PasswordHash: "x", CreatedAt: now}
	if err := store.Users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Daft Punk", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Discovery", ArtistID: artistID, CreatedAt: now})
	hit, _ := store.Catalog.UpsertTrack(ctx, models.Track{ID: uuid.NewString(), Title: "One More Time", AlbumID: albumID, ArtistID: artistID, Genre: "House", Duration: 320, Path: uuid.NewString(), CreatedAt: now, UpdatedAt: now})
	other, _ := store.Catalog.UpsertTrack(ctx, models.Track{ID: uuid.NewString(), Title: "Aerodynamic", AlbumID: albumID, ArtistID: artistID, Genre: "House", Duration: 200, Path: uuid.NewString(), CreatedAt: now, UpdatedAt: now})

	scrobble := func(trackID string, at time.Time) {
		if err := store.Scrobbles.Insert(ctx, models.Scrobble{ID: uuid.NewString(), UserID: user.ID, TrackID: trackID, PlayedAt: at, Submitted: true}); err != nil {
			t.Fatal(err)
		}
	}
	// 3 plays of "hit" and 1 of "other" in 2024 (Jan + Mar); 1 play in 2023 (must be excluded).
	jan2024 := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	mar2024 := time.Date(2024, 3, 2, 9, 0, 0, 0, time.UTC)
	scrobble(hit, jan2024)
	scrobble(hit, jan2024)
	scrobble(hit, mar2024)
	scrobble(other, mar2024)
	scrobble(hit, time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC))

	w, err := store.Wrapped.Wrapped(ctx, user.ID, 2024)
	if err != nil {
		t.Fatal(err)
	}
	if w.TotalPlays != 4 {
		t.Fatalf("TotalPlays = %d, want 4", w.TotalPlays)
	}
	if w.TotalSeconds != 320*3+200 {
		t.Fatalf("TotalSeconds = %d, want %d", w.TotalSeconds, 320*3+200)
	}
	if w.ByMonth[0] != 2 || w.ByMonth[2] != 2 {
		t.Fatalf("ByMonth Jan=%d Mar=%d, want 2 and 2", w.ByMonth[0], w.ByMonth[2])
	}
	if len(w.TopTracks) != 2 || w.TopTracks[0].ID != hit || w.TopTracks[0].Plays != 3 {
		t.Fatalf("TopTracks[0] = %+v, want hit with 3 plays", w.TopTracks)
	}
	if w.TopTracks[0].Artist != "Daft Punk" {
		t.Fatalf("artist = %q", w.TopTracks[0].Artist)
	}
	if len(w.TopArtists) != 1 || w.TopArtists[0].Plays != 4 {
		t.Fatalf("TopArtists = %+v, want one artist with 4 plays", w.TopArtists)
	}
	if len(w.TopGenres) != 1 || w.TopGenres[0].Name != "House" || w.TopGenres[0].Plays != 4 {
		t.Fatalf("TopGenres = %+v, want House with 4 plays", w.TopGenres)
	}
}

// TestWrappedRepoTopTracksWithCustomWindowAndLimit covers TopTracks used
// directly (not through Wrapped's fixed year+chartLimit) — the query a
// personal "top tracks this month"/"on repeat" list is built on.
func TestWrappedRepoTopTracksWithCustomWindowAndLimit(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	user := models.User{ID: uuid.NewString(), Username: "u2", PasswordHash: "x", CreatedAt: now}
	if err := store.Users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: now})
	newTrack := func(title string) string {
		id, _ := store.Catalog.UpsertTrack(ctx, models.Track{ID: uuid.NewString(), Title: title, AlbumID: albumID, ArtistID: artistID, Path: uuid.NewString(), CreatedAt: now, UpdatedAt: now})
		return id
	}
	a, b, c := newTrack("A"), newTrack("B"), newTrack("C")

	scrobble := func(trackID string, at time.Time) {
		if err := store.Scrobbles.Insert(ctx, models.Scrobble{ID: uuid.NewString(), UserID: user.ID, TrackID: trackID, PlayedAt: at, Submitted: true}); err != nil {
			t.Fatal(err)
		}
	}
	within := time.Date(2024, 6, 10, 0, 0, 0, 0, time.UTC)
	scrobble(a, within)
	scrobble(a, within)
	scrobble(b, within)
	scrobble(c, time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)) // outside the window below

	start := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	end := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC).UnixMilli()

	top, err := store.Wrapped.TopTracks(ctx, user.ID, start, end, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 1 || top[0].ID != a || top[0].Plays != 2 {
		t.Fatalf("TopTracks(limit=1) = %+v, want just track a with 2 plays", top)
	}

	top, err = store.Wrapped.TopTracks(ctx, user.ID, start, end, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 2 {
		t.Fatalf("TopTracks(limit=10) = %+v, want 2 tracks (c is outside the window)", top)
	}
}

// TestWrappedRepoTopArtistsWithCustomWindowAndLimit covers TopArtists (used by
// internal/concerts to pick which artists to search for nearby shows) — same
// window/limit shape as TopTracks above, but grouped by artist.
func TestWrappedRepoTopArtistsWithCustomWindowAndLimit(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	user := models.User{ID: uuid.NewString(), Username: "u3", PasswordHash: "x", CreatedAt: now}
	if err := store.Users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	newArtistTrack := func(artistName string) string {
		artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: artistName, CreatedAt: now})
		albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: now})
		id, _ := store.Catalog.UpsertTrack(ctx, models.Track{ID: uuid.NewString(), Title: "T", AlbumID: albumID, ArtistID: artistID, Path: uuid.NewString(), CreatedAt: now, UpdatedAt: now})
		return id
	}
	trackA, trackB, trackC := newArtistTrack("Artist A"), newArtistTrack("Artist B"), newArtistTrack("Artist C")

	scrobble := func(trackID string, at time.Time) {
		if err := store.Scrobbles.Insert(ctx, models.Scrobble{ID: uuid.NewString(), UserID: user.ID, TrackID: trackID, PlayedAt: at, Submitted: true}); err != nil {
			t.Fatal(err)
		}
	}
	within := time.Date(2024, 6, 10, 0, 0, 0, 0, time.UTC)
	scrobble(trackA, within)
	scrobble(trackA, within)
	scrobble(trackB, within)
	scrobble(trackC, time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)) // outside the window below

	start := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	end := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC).UnixMilli()

	top, err := store.Wrapped.TopArtists(ctx, user.ID, start, end, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 1 || top[0].Name != "Artist A" || top[0].Plays != 2 {
		t.Fatalf("TopArtists(limit=1) = %+v, want just Artist A with 2 plays", top)
	}

	top, err = store.Wrapped.TopArtists(ctx, user.ID, start, end, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 2 {
		t.Fatalf("TopArtists(limit=10) = %+v, want 2 artists (C is outside the window)", top)
	}
}

// TestWrappedRepoTotalsIsAllTime covers Totals used by the profile page's stat
// row: unlike Wrapped/TopTracks it has no year/window bound, and it must
// ignore unsubmitted scrobbles the same way Wrapped does.
func TestWrappedRepoTotalsIsAllTime(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	user := models.User{ID: uuid.NewString(), Username: "u3", PasswordHash: "x", CreatedAt: now}
	if err := store.Users.Create(ctx, user); err != nil {
		t.Fatal(err)
	}
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: now})
	track, _ := store.Catalog.UpsertTrack(ctx, models.Track{ID: uuid.NewString(), Title: "T", AlbumID: albumID, ArtistID: artistID, Duration: 200, Path: uuid.NewString(), CreatedAt: now, UpdatedAt: now})

	// Two submitted plays years apart (Totals must span both, unlike Wrapped's
	// single-year window) plus one unsubmitted play that must not count.
	if err := store.Scrobbles.Insert(ctx, models.Scrobble{ID: uuid.NewString(), UserID: user.ID, TrackID: track, PlayedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), Submitted: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.Scrobbles.Insert(ctx, models.Scrobble{ID: uuid.NewString(), UserID: user.ID, TrackID: track, PlayedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Submitted: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.Scrobbles.Insert(ctx, models.Scrobble{ID: uuid.NewString(), UserID: user.ID, TrackID: track, PlayedAt: now, Submitted: false}); err != nil {
		t.Fatal(err)
	}

	plays, seconds, err := store.Wrapped.Totals(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if plays != 2 {
		t.Fatalf("plays = %d, want 2 (unsubmitted scrobble must not count)", plays)
	}
	if seconds != 400 {
		t.Fatalf("seconds = %d, want 400 (2 plays x 200s)", seconds)
	}
}
