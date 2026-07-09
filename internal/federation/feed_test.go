package federation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/config"
	"github.com/immerle/immerle/internal/federation/hub"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/testutil"
)

func TestSyncFeedMaterializesSubscribedPlaylists(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	// The virtual system account that owns feed playlists.
	system := models.User{ID: "system", Username: "system", DisplayName: "System", CreatedAt: now}
	_ = store.Users.Create(ctx, system)

	// A local track the remote playlist references by MBID.
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Artist", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Album", ArtistID: artistID, CreatedAt: now})
	trackID, _ := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "Song", AlbumID: albumID, ArtistID: artistID,
		Path: "/music/song.mp3", MBID: "mbid-1", CreatedAt: now, UpdatedAt: now,
	})

	detailHits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/instances/me/feed/playlists":
			// Single page (no nextUpdatedAfter / hasMore=false).
			_ = json.NewEncoder(w).Encode(hub.PublicFeedResponse{
				Ok: boolptr(true),
				Playlists: &[]hub.PublicFeedPlaylistDTO{{
					ExternalId: strptr("ext-1"),
					Name:       strptr("Their Mix"),
					UpdatedAt:  strptr("2026-06-22T10:00:00Z"),
					Author:     &hub.PublicAuthorDTO{Id: strptr("inst-remote"), Name: strptr("Remote")},
				}},
			})
		case r.URL.Path == "/api/v1/instances/inst-remote/playlists/ext-1":
			detailHits++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"playlist": map[string]any{
					"externalId":  "ext-1",
					"name":        "Their Mix",
					"description": "from afar",
					"tracks": []map[string]any{
						{"mbid": "mbid-1", "artist": "Artist", "title": "Song"},
						{"mbid": "mbid-absent", "artist": "Nope", "title": "Gone"}, // unresolvable
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := config.FederationConfig{HubURL: srv.URL, InstanceID: "inst-1", PrivateKey: "iml_key"}
	svc := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, nil, testLogger())
	svc.SetFeedOwnerResolver(func(context.Context) (string, error) { return "system", nil })

	if err := svc.SyncFeed(ctx); err != nil {
		t.Fatal(err)
	}

	localID := uuid.NewSHA1(feedNamespace, []byte("inst-remote/ext-1")).String()
	p, err := store.Playlists.Get(ctx, localID)
	if err != nil {
		t.Fatalf("feed playlist not created: %v", err)
	}
	if !p.Public || !p.Federated || p.OwnerID != "system" || p.Name != "Their Mix" {
		t.Fatalf("unexpected playlist: %+v", p)
	}
	tracks, _ := store.Playlists.Tracks(ctx, localID)
	if len(tracks) != 1 || tracks[0].ID != trackID {
		t.Fatalf("expected the resolvable track only, got %d", len(tracks))
	}

	// Re-sync with the same updatedAt must skip the detail fetch (dedup).
	if err := svc.SyncFeed(ctx); err != nil {
		t.Fatal(err)
	}
	if detailHits != 1 {
		t.Fatalf("expected 1 detail fetch (unchanged playlist skipped), got %d", detailHits)
	}
}
