package concerts

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/testutil"
)

// fakeSearcher returns canned events per artist name, and records every call
// so tests can assert whether Skiddle was even tried.
type fakeSearcher struct {
	events map[string][]foundEvent
	calls  []string
}

func (f *fakeSearcher) Search(_ context.Context, artist, _ string, _ int) ([]foundEvent, error) {
	f.calls = append(f.calls, artist)
	return f.events[artist], nil
}

func newTestService(t *testing.T, cfg models.ConcertsRuntime, tm, sk *fakeSearcher) (*Service, *persistence.Store) {
	t.Helper()
	store := testutil.NewStore(t)
	svc := New(store.Users, store.Wrapped, store.Concerts, func() models.ConcertsRuntime { return cfg }, testutil.NewLogger())
	svc.newTicketmaster = func(string) searcher { return tm }
	svc.newSkiddle = func(string) searcher { return sk }
	return svc, store
}

// seedListener creates a user with one scrobble for a track by artistName, so
// WrappedRepo.TopArtists finds them.
func seedListener(t *testing.T, store *persistence.Store, artistName string) models.User {
	t.Helper()
	ctx := context.Background()
	now := time.Now()
	u := models.User{ID: uuid.NewString(), Username: uuid.NewString(), PasswordHash: "x", CreatedAt: now}
	if err := store.Users.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	artistID, err := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: artistName, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	albumID, err := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	trackID, err := store.Catalog.UpsertTrack(ctx, models.Track{ID: uuid.NewString(), Title: "T", AlbumID: albumID, ArtistID: artistID, Path: uuid.NewString(), CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	// A minute in the past, not now: TopArtists' window end is an independently
	// captured time.Now() (in SyncNow, called after this returns) — under load,
	// two time.Now() calls close enough together can land on the same
	// millisecond, and the query's played_at<end is a strict less-than.
	if err := store.Scrobbles.Insert(ctx, models.Scrobble{ID: uuid.NewString(), UserID: u.ID, TrackID: trackID, PlayedAt: now.Add(-time.Minute), Submitted: true}); err != nil {
		t.Fatal(err)
	}
	return u
}

func TestSyncNowDisabledIsNoOp(t *testing.T) {
	tm, sk := &fakeSearcher{}, &fakeSearcher{}
	svc, store := newTestService(t, models.ConcertsRuntime{Enabled: false, Country: "FR"}, tm, sk)
	seedListener(t, store, "Daft Punk")

	synced, err := svc.SyncNow(context.Background())
	if err != nil || synced != 0 {
		t.Fatalf("SyncNow(disabled) = %d, %v, want 0, nil", synced, err)
	}
	if len(tm.calls) != 0 || len(sk.calls) != 0 {
		t.Fatal("SyncNow(disabled) called a search client — it must be a pure no-op")
	}
}

func TestSyncNowNoCountryIsNoOp(t *testing.T) {
	tm, sk := &fakeSearcher{events: map[string][]foundEvent{"Daft Punk": {{id: "1", name: "Show", startTime: time.Now().Add(24 * time.Hour)}}}}, &fakeSearcher{}
	svc, store := newTestService(t, models.ConcertsRuntime{Enabled: true, Country: ""}, tm, sk)
	seedListener(t, store, "Daft Punk")

	synced, err := svc.SyncNow(context.Background())
	if err != nil || synced != 0 {
		t.Fatalf("SyncNow = %d, %v, want 0, nil (no country configured)", synced, err)
	}
	if len(tm.calls) != 0 {
		t.Fatal("SyncNow(no country) called a search client — it must be a pure no-op")
	}
}

func TestSyncNowPrefersTicketmasterAndSkipsSkiddleWhenItFindsSomething(t *testing.T) {
	tm := &fakeSearcher{events: map[string][]foundEvent{
		"Daft Punk": {{id: "tm-1", name: "TM Show", startTime: time.Now().Add(24 * time.Hour)}},
	}}
	sk := &fakeSearcher{}
	svc, store := newTestService(t, models.ConcertsRuntime{Enabled: true, Country: "FR"}, tm, sk)
	user := seedListener(t, store, "Daft Punk")

	synced, err := svc.SyncNow(context.Background())
	if err != nil || synced != 1 {
		t.Fatalf("SyncNow = %d, %v, want 1, nil", synced, err)
	}
	if len(sk.calls) != 0 {
		t.Fatal("Skiddle was called even though Ticketmaster already found a match")
	}
	list, err := store.Concerts.ListActive(context.Background(), user.ID, time.Now(), 10)
	if err != nil || len(list) != 1 || list[0].Source != "ticketmaster" {
		t.Fatalf("ListActive = %+v, err=%v, want one ticketmaster match", list, err)
	}
}

func TestSyncNowFallsBackToSkiddleWhenTicketmasterFindsNothing(t *testing.T) {
	tm := &fakeSearcher{events: map[string][]foundEvent{}} // no match for anything
	sk := &fakeSearcher{events: map[string][]foundEvent{
		"Daft Punk": {{id: "sk-1", name: "SK Show", startTime: time.Now().Add(24 * time.Hour)}},
	}}
	svc, store := newTestService(t, models.ConcertsRuntime{Enabled: true, Country: "FR"}, tm, sk)
	user := seedListener(t, store, "Daft Punk")

	synced, err := svc.SyncNow(context.Background())
	if err != nil || synced != 1 {
		t.Fatalf("SyncNow = %d, %v, want 1, nil", synced, err)
	}
	list, err := store.Concerts.ListActive(context.Background(), user.ID, time.Now(), 10)
	if err != nil || len(list) != 1 || list[0].Source != "skiddle" {
		t.Fatalf("ListActive = %+v, err=%v, want one skiddle match", list, err)
	}
}

func TestSyncNowSkipsPastEvents(t *testing.T) {
	tm := &fakeSearcher{events: map[string][]foundEvent{
		"Daft Punk": {{id: "tm-1", name: "Already happened", startTime: time.Now().Add(-24 * time.Hour)}},
	}}
	sk := &fakeSearcher{}
	svc, store := newTestService(t, models.ConcertsRuntime{Enabled: true, Country: "FR"}, tm, sk)
	seedListener(t, store, "Daft Punk")

	synced, err := svc.SyncNow(context.Background())
	if err != nil || synced != 0 {
		t.Fatalf("SyncNow = %d, %v, want 0, nil (event already happened)", synced, err)
	}
}

// TestSyncNowSearchesEveryUser covers that, unlike the old per-user-city
// design, a single instance-wide country applies to every user's own
// top-listened artists.
func TestSyncNowSearchesEveryUser(t *testing.T) {
	tm := &fakeSearcher{events: map[string][]foundEvent{
		"Daft Punk": {{id: "tm-1", name: "Show A", startTime: time.Now().Add(24 * time.Hour)}},
		"Jay-Z":     {{id: "tm-2", name: "Show B", startTime: time.Now().Add(48 * time.Hour)}},
	}}
	sk := &fakeSearcher{}
	svc, store := newTestService(t, models.ConcertsRuntime{Enabled: true, Country: "FR"}, tm, sk)
	userA := seedListener(t, store, "Daft Punk")
	userB := seedListener(t, store, "Jay-Z")

	synced, err := svc.SyncNow(context.Background())
	if err != nil || synced != 2 {
		t.Fatalf("SyncNow = %d, %v, want 2, nil", synced, err)
	}
	listA, err := store.Concerts.ListActive(context.Background(), userA.ID, time.Now(), 10)
	if err != nil || len(listA) != 1 {
		t.Fatalf("userA ListActive = %+v, err=%v, want 1 match", listA, err)
	}
	listB, err := store.Concerts.ListActive(context.Background(), userB.ID, time.Now(), 10)
	if err != nil || len(listB) != 1 {
		t.Fatalf("userB ListActive = %+v, err=%v, want 1 match", listB, err)
	}
}
