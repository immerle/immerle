package federation

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/config"
	"github.com/immerle/immerle/internal/federation/hub"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/testutil"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

// strptr is a tiny helper for building the generated hub DTOs (all-pointer).
func strptr(s string) *string { return &s }

// stubHub is a minimal in-memory immerle-hub for testing the client.
func stubHub(t *testing.T, playlists []hub.PublicDistributionPlaylist) (*httptest.Server, *stubState) {
	state := &stubState{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/instances/register", func(w http.ResponseWriter, r *http.Request) {
		state.registered = true
		_ = json.NewEncoder(w).Encode(hub.PublicProfileResponse{Ok: boolptr(true)})
	})
	mux.HandleFunc("/api/v1/playlists", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(playlists)
	})
	mux.HandleFunc("/api/v1/scrobbles", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Aggregates []map[string]any `json:"aggregates"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		state.scrobbleBatches = append(state.scrobbleBatches, body.Aggregates)
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, state
}

func boolptr(b bool) *bool { return &b }

func TestFederationDiscoveryAndSubscriptions(t *testing.T) {
	ctx := context.Background()
	var searchQ, subBody, deletedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/instances/search":
			searchQ = r.URL.Query().Get("q")
			_ = json.NewEncoder(w).Encode(hub.PublicSearchResponse{Instances: &[]hub.PublicInstanceSummary{
				{Id: strptr("uuid-2"), Sqid: strptr("other-node"), Name: strptr("Other")},
			}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/instances/me/subscriptions":
			_ = json.NewEncoder(w).Encode(hub.PublicSubscriptionsResponse{Subscriptions: &[]hub.PublicInstanceSummary{
				{Id: strptr("uuid-3"), Sqid: strptr("followed"), Name: strptr("Followed")},
			}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/instances/me/subscriptions":
			b, _ := io.ReadAll(r.Body)
			subBody = string(b)
			_ = json.NewEncoder(w).Encode(hub.PublicSubscriptionStateResponse{Ok: boolptr(true), Subscribed: boolptr(true)})
		case r.Method == http.MethodDelete:
			deletedPath = r.URL.Path
			_ = json.NewEncoder(w).Encode(hub.PublicSubscriptionStateResponse{Ok: boolptr(true), Subscribed: boolptr(false)})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	store := testutil.NewStore(t)
	cfg := config.FederationConfig{HubURL: srv.URL, InstanceID: "uuid-1", PrivateKey: "iml_key"}
	svc := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, nil, testLogger())

	found, err := svc.SearchInstances(ctx, "other")
	if err != nil || len(found) != 1 || found[0].Sqid != "other-node" {
		t.Fatalf("search: %v %+v", err, found)
	}
	if searchQ != "other" {
		t.Fatalf("query not forwarded: %q", searchQ)
	}

	if err := svc.Subscribe(ctx, "uuid-2", ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(subBody, `"instanceId":"uuid-2"`) {
		t.Fatalf("subscribe body wrong: %s", subBody)
	}

	subs, err := svc.Subscriptions(ctx)
	if err != nil || len(subs) != 1 || subs[0].Sqid != "followed" {
		t.Fatalf("subscriptions: %v %+v", err, subs)
	}

	if err := svc.Unsubscribe(ctx, "uuid-3"); err != nil {
		t.Fatal(err)
	}
	if deletedPath != "/api/v1/instances/me/subscriptions/uuid-3" {
		t.Fatalf("unsubscribe path wrong: %q", deletedPath)
	}
}

type stubState struct {
	registered      bool
	scrobbleBatches [][]map[string]any
}

func TestFederationSyncMaterializesReadOnlyPlaylist(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	// A local track that the editorial playlist references by MBID.
	owner := models.User{ID: uuid.NewString(), Username: "admin", PasswordHash: "x", IsAdmin: true, CreatedAt: now}
	_ = store.Users.Create(ctx, owner)
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Present Artist", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Album", ArtistID: artistID, CreatedAt: now})
	localID, _ := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "Present", AlbumID: albumID, ArtistID: artistID,
		Path: "/music/present.mp3", MBID: "mbid-present", CreatedAt: now, UpdatedAt: now,
	})

	playlists := []hub.PublicDistributionPlaylist{{
		Id:      strptr("editorial-1"),
		Name:    strptr("Hub Picks"),
		Comment: strptr("Editorial"),
		Tracks: &[]hub.PublicDistributionTrack{
			{Mbid: strptr("mbid-present"), Artist: strptr("Present Artist"), Title: strptr("Present")},
			{Mbid: strptr("mbid-absent"), Artist: strptr("Absent Artist"), Title: strptr("Absent")}, // not resolvable (no resolver)
		},
	}}
	srv, state := stubHub(t, playlists)

	cfg := config.FederationConfig{
		HubURL:     srv.URL,
		InstanceID: "inst-1",
		PrivateKey: "iml_key",
	}
	svc := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, nil, testLogger())
	svc.SetOwner(owner.ID)

	if err := svc.Register(ctx); err != nil {
		t.Fatal(err)
	}
	if !state.registered {
		t.Fatal("hub did not record registration")
	}

	if err := svc.SyncPlaylists(ctx); err != nil {
		t.Fatal(err)
	}

	// A federated, read-only playlist should now exist with the resolvable track.
	fed, err := store.Playlists.FindFederated(ctx, "Hub Picks")
	if err != nil {
		t.Fatalf("federated playlist not created: %v", err)
	}
	if !fed.Federated {
		t.Fatal("playlist should be marked federated (read-only)")
	}
	tracks, _ := store.Playlists.Tracks(ctx, fed.ID)
	if len(tracks) != 1 || tracks[0].ID != localID {
		t.Fatalf("expected the present track resolved, got %d tracks", len(tracks))
	}

	// Re-syncing must not duplicate the federated playlist.
	if err := svc.SyncPlaylists(ctx); err != nil {
		t.Fatal(err)
	}
	visible, _ := store.Playlists.ListVisible(ctx, owner.ID)
	fedCount := 0
	for _, p := range visible {
		if p.Federated {
			fedCount++
		}
	}
	if fedCount != 1 {
		t.Fatalf("expected 1 federated playlist after re-sync, got %d", fedCount)
	}
}

func TestFederationExportsAnonymizedScrobbles(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	user := models.User{ID: uuid.NewString(), Username: "u", PasswordHash: "x", CreatedAt: now}
	_ = store.Users.Create(ctx, user)
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: now})
	trackID, _ := store.Catalog.UpsertTrack(ctx, models.Track{ID: uuid.NewString(), Title: "t", AlbumID: albumID, ArtistID: artistID, Path: "/p.mp3", CreatedAt: now, UpdatedAt: now})

	for i := 0; i < 3; i++ {
		_ = store.Scrobbles.Insert(ctx, models.Scrobble{ID: uuid.NewString(), UserID: user.ID, TrackID: trackID, PlayedAt: now, Submitted: true})
	}

	srv, state := stubHub(t, nil)
	cfg := config.FederationConfig{HubURL: srv.URL, InstanceID: "inst-1", PrivateKey: "iml_key", ExportScrobbles: true}
	svc := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, nil, testLogger())

	if err := svc.ExportScrobbles(ctx); err != nil {
		t.Fatal(err)
	}
	if len(state.scrobbleBatches) != 1 || len(state.scrobbleBatches[0]) != 1 {
		t.Fatalf("expected one aggregate batch with one track, got %+v", state.scrobbleBatches)
	}
	agg := state.scrobbleBatches[0][0]
	// No PII: the payload carries a hash and a count, never the raw track/user id.
	if _, hasHash := agg["trackHash"]; !hasHash {
		t.Fatal("aggregate missing trackHash")
	}
	if agg["trackHash"] == trackID {
		t.Fatal("raw track id leaked to hub")
	}
	if cnt, _ := agg["count"].(float64); cnt != 3 {
		t.Fatalf("expected count 3, got %v", agg["count"])
	}

	// Exported scrobbles are marked, so a second export sends nothing.
	if err := svc.ExportScrobbles(ctx); err != nil {
		t.Fatal(err)
	}
	if len(state.scrobbleBatches) != 1 {
		t.Fatal("scrobbles were exported twice")
	}
}
