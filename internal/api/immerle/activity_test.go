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

	// Seed a track.
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Daft Punk", CreatedAt: time.Now()})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Discovery", ArtistID: artistID, CreatedAt: time.Now()})
	trackID, _ := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "One More Time", AlbumID: albumID, ArtistID: artistID,
		Path: "/x.mp3", Duration: 320, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	// Record an activity event referencing it.
	activitySvc := core.NewActivityService(store.Activity, store.Friends, store.Users)
	if err := activitySvc.Record(ctx, user, "favorite", models.ItemTrack, trackID); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(Deps{
		Auth: auth, Users: store.Users, Friends: store.Friends,
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

	// Alice records a public activity event.
	activitySvc := core.NewActivityService(store.Activity, store.Friends, store.Users)
	alicePublic := alice
	alicePublic.ActivityPrivacy = "public"
	if err := activitySvc.Record(ctx, alicePublic, "favorite", models.ItemArtist, uuid.NewString()); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(Deps{
		Auth: auth, Users: store.Users, Friends: store.Friends,
		Activity: activitySvc, Playlists: store.Playlists, Catalog: store.Catalog, Logger: testutil.NewLogger(),
	})
	mux := chi.NewRouter()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Bob views Alice's profile.
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
}
