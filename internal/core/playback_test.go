package core

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/testutil"
)

// fakeScrobbleEnqueuer records EnqueueScrobble calls instead of talking to a
// real external service.
type fakeScrobbleEnqueuer struct {
	calls int
}

func (f *fakeScrobbleEnqueuer) EnqueueScrobble(ctx context.Context, user models.User, track models.Track, at time.Time) {
	f.calls++
}

func TestScrobbleNotifiesScrobbleEnqueuer(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()

	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A", CreatedAt: time.Now()})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: time.Now()})
	trackID, err := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "T", AlbumID: albumID, ArtistID: artistID,
		Path: "/x.mp3", Duration: 200, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name       string
		submission bool
		enabled    bool
		wantCalls  int
	}{
		{"submitted + enabled: notifies", true, true, 1},
		{"not a submission: skipped", false, true, 0},
		{"scrobbling disabled: skipped", true, false, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := &fakeScrobbleEnqueuer{}
			svc := NewPlaybackService(store.Catalog, store.Annotations, store.Scrobbles, nil, nil, nil, fake)
			user := models.User{ID: uuid.NewString(), Username: "u", ScrobbleEnabled: c.enabled}
			svc.Scrobble(ctx, user, []string{trackID}, c.submission, time.Now())
			if fake.calls != c.wantCalls {
				t.Fatalf("EnqueueScrobble calls = %d, want %d", fake.calls, c.wantCalls)
			}
		})
	}
}

func TestScrobbleWithNilScrobbleEnqueuer(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()

	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A", CreatedAt: time.Now()})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: time.Now()})
	trackID, err := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "T", AlbumID: albumID, ArtistID: artistID,
		Path: "/y.mp3", Duration: 200, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// A nil scrobbleSync (the pre-existing default when ListenBrainz isn't
	// wired) must not panic.
	svc := NewPlaybackService(store.Catalog, store.Annotations, store.Scrobbles, nil, nil, nil, nil)
	user := models.User{ID: uuid.NewString(), Username: "u", ScrobbleEnabled: true}
	svc.Scrobble(ctx, user, []string{trackID}, true, time.Now())
}
