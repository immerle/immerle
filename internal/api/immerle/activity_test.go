package immerle

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	chi "github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/testutil"
)

func TestActivityFeedEnrichesItems(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	auth, _ := core.NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	if _, err := auth.CreateUser(ctx, "alice", "alicepw", "", "Alice W", false); err != nil {
		t.Fatal(err)
	}
	user, _ := store.Users.GetByUsername(ctx, "alice")

	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Daft Punk", CreatedAt: time.Now()})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Discovery", ArtistID: artistID, CreatedAt: time.Now()})
	trackID, _ := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "One More Time", AlbumID: albumID, ArtistID: artistID,
		Path: "/x.mp3", Duration: 320, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	activitySvc := core.NewActivityService(store.Activity)
	if err := activitySvc.Record(ctx, user, "favorite", models.ItemTrack, trackID); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(Deps{
		Auth: auth, Users: store.Users,
		Activity: activitySvc, Catalog: store.Catalog, Logger: testutil.NewLogger(),
	})
	mux := chi.NewRouter()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	token := login(t, srv, "alice")
	status, events := doArr(t, srv, http.MethodGet, "/activity", token, nil)
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0].(map[string]any)
	item, _ := ev["item"].(map[string]any)
	if item == nil {
		t.Fatalf("event has no enriched item: %+v", ev)
	}
	if item["title"] != "One More Time" {
		t.Fatalf("expected title 'One More Time', got %v", item["title"])
	}
	if item["artist"] != "Daft Punk" {
		t.Fatalf("expected artist 'Daft Punk', got %v", item["artist"])
	}
	if item["albumId"] != albumID {
		t.Fatalf("expected albumId %q, got %v", albumID, item["albumId"])
	}
	// The original ids are still present for actions.
	if ev["itemId"] != trackID || ev["type"] != "favorite" {
		t.Fatalf("base event fields wrong: %+v", ev)
	}
	// The author's display name is surfaced on the event.
	if ev["displayName"] != "Alice W" {
		t.Fatalf("expected displayName 'Alice W', got %v", ev["displayName"])
	}
}

func TestProfileEndpoint(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	auth, _ := core.NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	if _, err := auth.CreateUser(ctx, "alice", "alicepw", "", "Alice W", false); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.CreateUser(ctx, "bob", "bobpw", "", "Bob M", false); err != nil {
		t.Fatal(err)
	}
	alice, _ := store.Users.GetByUsername(ctx, "alice")

	// Alice has a public playlist and a private one; only the public shows.
	pub := models.Playlist{ID: uuid.NewString(), Name: "Public Mix", OwnerID: alice.ID, Public: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	priv := models.Playlist{ID: uuid.NewString(), Name: "Secret", OwnerID: alice.ID, Public: false, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := store.Playlists.Create(ctx, pub); err != nil {
		t.Fatal(err)
	}
	if err := store.Playlists.Create(ctx, priv); err != nil {
		t.Fatal(err)
	}

	activitySvc := core.NewActivityService(store.Activity)
	alicePublic := alice
	alicePublic.ActivityPrivacy = "public"
	if err := activitySvc.Record(ctx, alicePublic, "favorite", models.ItemArtist, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(Deps{
		Auth: auth, Users: store.Users,
		Activity: activitySvc, Playlists: store.Playlists, Catalog: store.Catalog, Logger: testutil.NewLogger(),
	})
	mux := chi.NewRouter()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	bobToken := login(t, srv, "bob")
	status, body := doMap(t, srv, http.MethodGet, "/users/alice", bobToken, nil)
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	u, _ := body["user"].(map[string]any)
	if u == nil || u["username"] != "alice" || u["displayName"] != "Alice W" {
		t.Fatalf("profile user wrong: %+v", body["user"])
	}
	if body["isSelf"] != false {
		t.Fatalf("expected isSelf=false, got %v", body["isSelf"])
	}
	playlists, _ := body["playlists"].([]any)
	if len(playlists) != 1 {
		t.Fatalf("expected 1 public playlist, got %d: %+v", len(playlists), body["playlists"])
	}
	if pl := playlists[0].(map[string]any); pl["name"] != "Public Mix" {
		t.Fatalf("expected 'Public Mix', got %v", pl["name"])
	}
	activity, _ := body["activity"].([]any)
	if len(activity) != 1 {
		t.Fatalf("expected 1 public activity event, got %d", len(activity))
	}
	if body["hallOfFame"] != nil {
		t.Fatalf("expected no hallOfFame key for a user with an empty Hall of Fame, got %+v", body["hallOfFame"])
	}
}

// TestProfileIncludesHallOfFameTop3 covers embedding another user's Hall of
// Fame top-3 (plus a total count) on their profile, and TestUserHallOfFame
// covers the "see all" endpoint the profile's top-3 links to.
func TestProfileIncludesHallOfFameTop3(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	auth, _ := core.NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	if _, err := auth.CreateUser(ctx, "alice", "alicepw", "", "Alice W", false); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.CreateUser(ctx, "bob", "bobpw", "", "Bob M", false); err != nil {
		t.Fatal(err)
	}
	alice, _ := store.Users.GetByUsername(ctx, "alice")

	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Daft Punk", CreatedAt: time.Now()})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Discovery", ArtistID: artistID, CreatedAt: time.Now()})
	trackIDs := make([]string, 4)
	for i := range trackIDs {
		id, err := store.Catalog.UpsertTrack(ctx, models.Track{
			ID: uuid.NewString(), Title: fmt.Sprintf("Track %d", i), AlbumID: albumID, ArtistID: artistID,
			Path: fmt.Sprintf("/%d.mp3", i), Duration: 200, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		})
		if err != nil {
			t.Fatal(err)
		}
		trackIDs[i] = id
	}
	hofSvc := core.NewHallOfFameService(store.HallOfFame, nil)
	if err := hofSvc.SetOrder(ctx, alice.ID, trackIDs); err != nil {
		t.Fatal(err)
	}

	// Two submitted plays (200s each) feed the profile's all-time stat row.
	for i := 0; i < 2; i++ {
		if err := store.Scrobbles.Insert(ctx, models.Scrobble{ID: uuid.NewString(), UserID: alice.ID, TrackID: trackIDs[0], PlayedAt: time.Now(), Submitted: true}); err != nil {
			t.Fatal(err)
		}
	}

	h := NewHandler(Deps{
		Auth: auth, Users: store.Users,
		Activity:   core.NewActivityService(store.Activity),
		Playlists:  store.Playlists,
		Catalog:    store.Catalog,
		HallOfFame: store.HallOfFame,
		Wrapped:    store.Wrapped,
		Logger:     testutil.NewLogger(),
	})
	mux := chi.NewRouter()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	bobToken := login(t, srv, "bob")
	status, body := doMap(t, srv, http.MethodGet, "/users/alice", bobToken, nil)
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	hof, _ := body["hallOfFame"].(map[string]any)
	if hof == nil {
		t.Fatalf("expected a hallOfFame section, got %+v", body)
	}
	if total, _ := hof["total"].(float64); total != 4 {
		t.Fatalf("expected total=4, got %v", hof["total"])
	}
	top, _ := hof["top"].([]any)
	if len(top) != 3 {
		t.Fatalf("expected only the top 3 tracks on the profile, got %d", len(top))
	}
	if first := top[0].(map[string]any); first["title"] != "Track 0" {
		t.Fatalf("expected 'Track 0' ranked first, got %v", first["title"])
	}

	// The "see all" endpoint returns the full ranked list, not just the top 3.
	status, full := doMap(t, srv, http.MethodGet, "/users/alice/hall-of-fame", bobToken, nil)
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	tracks, _ := full["tracks"].([]any)
	if len(tracks) != 4 {
		t.Fatalf("expected all 4 tracks, got %d", len(tracks))
	}

	// The stat row: 2 all-time plays x 200s each, plus the public playlist count.
	stats, _ := body["stats"].(map[string]any)
	if stats == nil {
		t.Fatalf("expected a stats section, got %+v", body)
	}
	if plays, _ := stats["plays"].(float64); plays != 2 {
		t.Fatalf("expected plays=2, got %v", stats["plays"])
	}
	if seconds, _ := stats["listenSeconds"].(float64); seconds != 400 {
		t.Fatalf("expected listenSeconds=400, got %v", stats["listenSeconds"])
	}
	if playlists, _ := stats["playlists"].(float64); playlists != 0 {
		t.Fatalf("expected playlists=0, got %v", stats["playlists"])
	}
}
