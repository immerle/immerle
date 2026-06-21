package federation

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/config"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/testutil"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

// stubHub is a minimal in-memory immerle-hub for testing the client.
func stubHub(t *testing.T, playlists []hubPlaylist) (*httptest.Server, *stubState) {
	state := &stubState{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/instances/register", func(w http.ResponseWriter, r *http.Request) {
		state.registered = true
		w.WriteHeader(http.StatusOK)
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

	playlists := []hubPlaylist{{
		ID:      "editorial-1",
		Name:    "Hub Picks",
		Comment: "Editorial",
		Tracks: []hubTrack{
			{MBID: "mbid-present", Artist: "Present Artist", Title: "Present"},
			{MBID: "mbid-absent", Artist: "Absent Artist", Title: "Absent"}, // not resolvable (no resolver)
		},
	}}
	srv, state := stubHub(t, playlists)

	cfg := config.FederationConfig{
		Enabled:    true,
		HubURL:     srv.URL,
		UserID:     "user-1",
		InstanceID: "inst-1",
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
	cfg := config.FederationConfig{Enabled: true, HubURL: srv.URL, UserID: "user-1", InstanceID: "inst-1", ExportScrobbles: true}
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
