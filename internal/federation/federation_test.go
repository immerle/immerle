package federation

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/testutil"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

// strptr is a tiny helper for building the generated hub DTOs (all-pointer).
func strptr(s string) *string { return &s }

// stubFeedPlaylist is one subscribed instance's playlist served by stubHub's
// feed endpoints (/instances/me/feed/playlists + /instances/{id}/playlists/{externalId}).
type stubFeedPlaylist struct {
	instanceID, instanceName, externalID, name, description, image string
	tracks                                                         []map[string]any
}

// stubHub is a minimal in-memory immerle-hub for testing the client.
func stubHub(t *testing.T, playlists []hub.PublicDistributionPlaylist, feed []stubFeedPlaylist) (*httptest.Server, *stubState) {
	state := &stubState{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/instances/register", func(w http.ResponseWriter, r *http.Request) {
		state.registered = true
		_ = json.NewEncoder(w).Encode(hub.PublicProfileResponse{Ok: boolptr(true)})
	})
	mux.HandleFunc("/api/v1/playlists", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(playlists)
	})
	mux.HandleFunc("/api/v1/instances/me/feed/playlists", func(w http.ResponseWriter, r *http.Request) {
		headers := make([]map[string]any, 0, len(feed))
		for _, f := range feed {
			headers = append(headers, map[string]any{
				"author":     map[string]any{"id": f.instanceID, "name": f.instanceName},
				"externalId": f.externalID, "name": f.name, "description": f.description,
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "playlists": headers, "hasMore": false})
	})
	mux.HandleFunc("/api/v1/instances/", func(w http.ResponseWriter, r *http.Request) {
		// Path shape: /api/v1/instances/{id}/playlists/{externalId}.
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/api/v1/instances/"), "/playlists/", 2)
		if len(parts) == 2 {
			for _, f := range feed {
				if f.instanceID == parts[0] && f.externalID == parts[1] {
					_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "playlist": map[string]any{
						"author":      map[string]any{"id": f.instanceID, "name": f.instanceName},
						"externalId":  f.externalID,
						"name":        f.name,
						"description": f.description,
						"image":       f.image,
						"tracks":      f.tracks,
					}})
					return
				}
			}
		}
		http.NotFound(w, r)
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
	svc := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, store.FeedCursors, nil, testLogger())

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
			{Mbid: strptr("mbid-absent"), Artist: strptr("Absent Artist"), Title: strptr("Absent")}, // no local match: kept unresolved
		},
	}}
	srv, state := stubHub(t, playlists, nil)

	cfg := config.FederationConfig{
		HubURL:     srv.URL,
		InstanceID: "inst-1",
		PrivateKey: "iml_key",
	}
	svc := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, store.FeedCursors, nil, testLogger())
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
	fed, err := store.Playlists.FindFederated(ctx, "", "editorial-1")
	if err != nil {
		t.Fatalf("federated playlist not created: %v", err)
	}
	if !fed.Federated {
		t.Fatal("playlist should be marked federated (read-only)")
	}
	// The mbid-matched track resolves immediately (no network call); the
	// unmatched one is kept, not dropped, as an unresolved stub carrying its
	// portable name — resolved lazily at play time (see ResolvePlaylistTrack).
	tracks, _ := store.Playlists.Tracks(ctx, fed.ID)
	if len(tracks) != 2 {
		t.Fatalf("expected both tracks kept (resolved + unresolved), got %d", len(tracks))
	}
	if tracks[0].ID != localID || tracks[0].Unresolved {
		t.Fatalf("expected the present track resolved to %q, got %+v", localID, tracks[0])
	}
	if !tracks[1].Unresolved || tracks[1].Title != "Absent" || tracks[1].ID != "" {
		t.Fatalf("expected the absent track kept unresolved, got %+v", tracks[1])
	}

	// Re-syncing must not duplicate the federated playlist. Federated rows
	// surface via ListPublic (discoverable/subscribable by anyone, including
	// the nominal owner) rather than ListVisible (owner_id never applies to
	// federated rows — see ListVisible's doc comment).
	if err := svc.SyncPlaylists(ctx); err != nil {
		t.Fatal(err)
	}
	discoverable, _ := store.Playlists.ListPublic(ctx, owner.ID)
	fedCount := 0
	for _, p := range discoverable {
		if p.Federated {
			fedCount++
		}
	}
	if fedCount != 1 {
		t.Fatalf("expected 1 federated playlist after re-sync, got %d", fedCount)
	}
}

// TestFederationSyncFeedHandlesSameNameAcrossInstances covers the bug this
// feature was built for: a subscribed instance's public playlists must
// appear locally, and two different instances publishing a same-named
// playlist must not collapse into one (the old dedupe-by-name behavior).
func TestFederationSyncFeedHandlesSameNameAcrossInstances(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	owner := models.User{ID: uuid.NewString(), Username: "admin2", PasswordHash: "x", IsAdmin: true, CreatedAt: now}
	_ = store.Users.Create(ctx, owner)

	feed := []stubFeedPlaylist{
		{instanceID: "inst-a", instanceName: "A", externalID: "ext-1", name: "Road Trip", description: "from A"},
		{instanceID: "inst-b", instanceName: "B", externalID: "ext-1", name: "Road Trip", description: "from B"},
	}
	srv, _ := stubHub(t, nil, feed)

	cfg := config.FederationConfig{HubURL: srv.URL, InstanceID: "self", PrivateKey: "iml_key"}
	svc := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, store.FeedCursors, nil, testLogger())
	svc.SetOwner(owner.ID)

	if err := svc.SyncPlaylists(ctx); err != nil {
		t.Fatal(err)
	}

	a, err := store.Playlists.FindFederated(ctx, "inst-a", "ext-1")
	if err != nil {
		t.Fatalf("instance A's playlist not materialized: %v", err)
	}
	b, err := store.Playlists.FindFederated(ctx, "inst-b", "ext-1")
	if err != nil {
		t.Fatalf("instance B's playlist not materialized: %v", err)
	}
	if a.ID == b.ID {
		t.Fatal("same-named playlists from different instances collapsed into one")
	}
	if a.Comment != "from A" || b.Comment != "from B" {
		t.Fatalf("wrong comment: a=%q b=%q", a.Comment, b.Comment)
	}

	// Re-syncing must not duplicate either.
	if err := svc.SyncPlaylists(ctx); err != nil {
		t.Fatal(err)
	}
	discoverable, _ := store.Playlists.ListPublic(ctx, owner.ID)
	fedCount := 0
	for _, p := range discoverable {
		if p.Federated {
			fedCount++
		}
	}
	if fedCount != 2 {
		t.Fatalf("expected 2 federated playlists after re-sync, got %d", fedCount)
	}
}

// fakeProviderResolver mimics the on-demand content providers:
// ResolveBestRemoteMatch finds a candidate by artist/title (an undownloaded
// "remote:" track, as the real on-demand catalog service returns).
// autoDownload defaults to false, so existing tests keep asserting
// ResolvePlaylistTrack never triggers a download/persist unless a test opts in.
type fakeProviderResolver struct {
	searched     []string
	autoDownload bool
	resolved     models.Track // returned by Resolve when autoDownload is true
}

func (f *fakeProviderResolver) ResolveBestRemoteMatch(_ context.Context, artist, title string) (models.Track, bool) {
	query := strings.TrimSpace(artist + " " + title)
	f.searched = append(f.searched, query)
	if strings.Contains(query, "Nobody Knows") {
		return models.Track{ID: "remote:fake:1", Title: "Unlisted Track", ArtistName: "Nobody Knows", Remote: true}, true
	}
	return models.Track{}, false
}

func (f *fakeProviderResolver) Resolve(context.Context, string, string) (models.Track, bool, string, error) {
	if !f.autoDownload {
		panic("not used: auto-download-on-play is off in this test")
	}
	return f.resolved, true, "", nil
}

func (f *fakeProviderResolver) AutoDownloadOnPlay() bool { return f.autoDownload }

// TestFederationSyncKeepsTracksUnresolvedAndForwardsCover covers the two gaps
// found after the feed pull started working: sync must not eagerly hit
// provider search over the network — a feed track with no local mbid match is
// kept as an unresolved entry (name intact) instead of being dropped — and the
// playlist's cover must be forwarded (as a fetchable remote-cover id, hub-
// relative URLs resolved against the configured hub).
func TestFederationSyncKeepsTracksUnresolvedAndForwardsCover(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	owner := models.User{ID: uuid.NewString(), Username: "admin3", PasswordHash: "x", IsAdmin: true, CreatedAt: now}
	_ = store.Users.Create(ctx, owner)

	feed := []stubFeedPlaylist{{
		instanceID: "inst-c", instanceName: "C", externalID: "ext-9",
		name: "Discoveries", description: "from C", image: "/api/v1/covers/deadbeef",
		tracks: []map[string]any{
			{"artist": "Nobody Knows", "title": "Unlisted Track"}, // no mbid: stays unresolved at sync
		},
	}}
	srv, _ := stubHub(t, nil, feed)

	resolver := &fakeProviderResolver{}
	cfg := config.FederationConfig{HubURL: srv.URL, InstanceID: "self", PrivateKey: "iml_key"}
	svc := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, store.FeedCursors, resolver, testLogger())
	svc.SetOwner(owner.ID)

	if err := svc.SyncPlaylists(ctx); err != nil {
		t.Fatal(err)
	}

	p, err := store.Playlists.FindFederated(ctx, "inst-c", "ext-9")
	if err != nil {
		t.Fatalf("playlist not materialized: %v", err)
	}

	tracks, _ := store.Playlists.Tracks(ctx, p.ID)
	if len(tracks) != 1 || !tracks[0].Unresolved || tracks[0].ID != "" {
		t.Fatalf("expected the track kept unresolved (no network at sync), got %+v", tracks)
	}
	if len(resolver.searched) != 0 {
		t.Fatalf("sync must not search providers, got %v", resolver.searched)
	}

	wantCoverURL := srv.URL + "/api/v1/covers/deadbeef"
	gotURL, ok := models.DecodeRemoteCoverID(p.CoverArt)
	if !ok || gotURL != wantCoverURL {
		t.Fatalf("cover not forwarded: coverArt=%q decoded=%q ok=%v want=%q", p.CoverArt, gotURL, ok, wantCoverURL)
	}

	// Playing it (ResolvePlaylistTrack) is where provider search happens.
	resolved, err := svc.ResolvePlaylistTrack(ctx, p.ID, 0)
	if err != nil {
		t.Fatalf("ResolvePlaylistTrack: %v", err)
	}
	if resolved.ID != "remote:fake:1" {
		t.Fatalf("expected the provider's remote track, got %+v", resolved)
	}
	if len(resolver.searched) != 1 || !strings.Contains(resolver.searched[0], "Nobody Knows") {
		t.Fatalf("provider wasn't searched with the track's name: %v", resolver.searched)
	}

	// A track without any local/provider match resolves to ErrUnresolvable.
	feed[0].tracks = []map[string]any{{"artist": "Truly Unknown", "title": "Nothing"}}
	if err := svc.SyncPlaylists(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ResolvePlaylistTrack(ctx, p.ID, 0); !errors.Is(err, ErrUnresolvable) {
		t.Fatalf("expected ErrUnresolvable, got %v", err)
	}
}

// TestFederationResolvePersistsProviderHitWhenAutoDownloadEnabled covers the
// on-tap resolve of a federated entry with no mbid: once the admin's
// auto-download-on-play setting is on, a provider-search hit gets downloaded
// in the background and its (real, local) track id is written back to the
// playlist entry — and a later resync must not wipe that out just because the
// track still carries no mbid of its own.
func TestFederationResolvePersistsProviderHitWhenAutoDownloadEnabled(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	owner := models.User{ID: uuid.NewString(), Username: "admin4", PasswordHash: "x", IsAdmin: true, CreatedAt: now}
	_ = store.Users.Create(ctx, owner)

	// The track the provider "downloads" to, once resolved — named distinctly
	// from the feed track below so it isn't found by the local artist+title
	// fallback (this test covers the no-local-match, provider-search path).
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Downloaded Copy Artist", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: now})
	downloadedID, _ := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "Downloaded Copy Title", AlbumID: albumID, ArtistID: artistID,
		Path: "/music/unlisted.mp3", CreatedAt: now, UpdatedAt: now,
	})

	feed := []stubFeedPlaylist{{
		instanceID: "inst-d", instanceName: "D", externalID: "ext-10",
		name: "Discoveries",
		tracks: []map[string]any{
			{"artist": "Nobody Knows", "title": "Unlisted Track"}, // no mbid: stays unresolved at sync
		},
	}}
	srv, _ := stubHub(t, nil, feed)

	resolver := &fakeProviderResolver{autoDownload: true, resolved: models.Track{ID: downloadedID}}
	cfg := config.FederationConfig{HubURL: srv.URL, InstanceID: "self", PrivateKey: "iml_key"}
	svc := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, store.FeedCursors, resolver, testLogger())
	svc.SetOwner(owner.ID)

	if err := svc.SyncPlaylists(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := store.Playlists.FindFederated(ctx, "inst-d", "ext-10")
	if err != nil {
		t.Fatalf("playlist not materialized: %v", err)
	}

	resolved, err := svc.ResolvePlaylistTrack(ctx, p.ID, 0)
	if err != nil {
		t.Fatalf("ResolvePlaylistTrack: %v", err)
	}
	if resolved.ID != "remote:fake:1" {
		t.Fatalf("expected the transparent, immediate remote hit, got %+v", resolved)
	}

	// The download+persist happens in the background; poll for it.
	deadline := time.Now().Add(2 * time.Second)
	for {
		ref, err := store.Playlists.TrackRef(ctx, p.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		if ref.TrackID == downloadedID {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("track_id was never persisted, got ref=%+v", ref)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// A resync of the same (still mbid-less) track must not wipe the resolve
	// out just because it can't be re-derived by mbid.
	if err := svc.SyncPlaylists(ctx); err != nil {
		t.Fatal(err)
	}
	ref, err := store.Playlists.TrackRef(ctx, p.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if ref.TrackID != downloadedID {
		t.Fatalf("resync overwrote the resolved track_id: got %+v, want %q", ref, downloadedID)
	}
}

// TestResolvePlaylistTrackFindsLocalCopyByArtistTitle covers the bug: a track
// already in the local catalog (e.g. from another playlist) with no mbid, or
// an mbid that doesn't match the federated entry's, was never found — the old
// code checked mbid only, then jumped straight to a remote provider search.
func TestResolvePlaylistTrackFindsLocalCopyByArtistTitle(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	owner := models.User{ID: uuid.NewString(), Username: "admin5", PasswordHash: "x", IsAdmin: true, CreatedAt: now}
	_ = store.Users.Create(ctx, owner)

	// Already in the local catalog (e.g. uploaded manually into another
	// playlist), tagged with no mbid at all.
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Nicki Minaj", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: now})
	localID, _ := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "Rap Barbie", AlbumID: albumID, ArtistID: artistID,
		Path: "/music/rap-barbie.mp3", CreatedAt: now, UpdatedAt: now,
	})

	feed := []stubFeedPlaylist{{
		instanceID: "inst-e", instanceName: "E", externalID: "ext-11",
		name: "Shared",
		tracks: []map[string]any{
			{"artist": "Nicki Minaj", "title": "Rap Barbie"}, // no mbid from the remote instance either
		},
	}}
	srv, _ := stubHub(t, nil, feed)

	resolver := &fakeProviderResolver{} // never matches "Nicki Minaj" — proves it isn't reached
	cfg := config.FederationConfig{HubURL: srv.URL, InstanceID: "self", PrivateKey: "iml_key"}
	svc := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, store.FeedCursors, resolver, testLogger())
	svc.SetOwner(owner.ID)

	if err := svc.SyncPlaylists(ctx); err != nil {
		t.Fatal(err)
	}
	p, err := store.Playlists.FindFederated(ctx, "inst-e", "ext-11")
	if err != nil {
		t.Fatalf("playlist not materialized: %v", err)
	}

	resolved, err := svc.ResolvePlaylistTrack(ctx, p.ID, 0)
	if err != nil {
		t.Fatalf("ResolvePlaylistTrack: %v (should have found the local copy)", err)
	}
	if resolved.ID != localID {
		t.Fatalf("expected the existing local track %q, got %+v", localID, resolved)
	}
	if len(resolver.searched) != 0 {
		t.Fatalf("should never reach the provider once a local artist+title match exists, got %v", resolver.searched)
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

	srv, state := stubHub(t, nil, nil)
	cfg := config.FederationConfig{HubURL: srv.URL, InstanceID: "inst-1", PrivateKey: "iml_key", ExportScrobbles: true}
	svc := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, store.FeedCursors, nil, testLogger())

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

// TestUnsubscribeDropsUnkeptFederatedPlaylists covers the fix: unfollowing an
// instance must remove its materialized playlists locally, except any a user
// subscribed to (kept in their library) — those survive the unfollow.
func TestUnsubscribeDropsUnkeptFederatedPlaylists(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	owner := models.User{ID: uuid.NewString(), Username: "admin3", PasswordHash: "x", IsAdmin: true, CreatedAt: now}
	_ = store.Users.Create(ctx, owner)
	user := models.User{ID: uuid.NewString(), Username: "u3", PasswordHash: "x", CreatedAt: now}
	_ = store.Users.Create(ctx, user)

	kept := models.Playlist{ID: uuid.NewString(), Name: "Kept", OwnerID: owner.ID, Federated: true, SourceInstanceID: "inst-x", SourceExternalID: "ext-kept", CreatedAt: now, UpdatedAt: now}
	dropped := models.Playlist{ID: uuid.NewString(), Name: "Dropped", OwnerID: owner.ID, Federated: true, SourceInstanceID: "inst-x", SourceExternalID: "ext-dropped", CreatedAt: now, UpdatedAt: now}
	if err := store.Playlists.Create(ctx, kept); err != nil {
		t.Fatal(err)
	}
	if err := store.Playlists.Create(ctx, dropped); err != nil {
		t.Fatal(err)
	}
	if err := store.Playlists.Subscribe(ctx, kept.ID, user.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.FeedCursors.Set(ctx, "inst-x", "v42"); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(hub.PublicSubscriptionStateResponse{Ok: boolptr(true), Subscribed: boolptr(false)})
	}))
	defer srv.Close()
	cfg := config.FederationConfig{HubURL: srv.URL, InstanceID: "self", PrivateKey: "iml_key"}
	svc := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, store.FeedCursors, nil, testLogger())

	if err := svc.Unsubscribe(ctx, "inst-x"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Playlists.Get(ctx, kept.ID); err != nil {
		t.Fatalf("kept (subscribed) playlist should survive unfollow: %v", err)
	}
	if _, err := store.Playlists.Get(ctx, dropped.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("unsubscribed playlist should be dropped on unfollow, got: %v", err)
	}
	if v, err := store.FeedCursors.Get(ctx, "inst-x"); err != nil || v != "" {
		t.Fatalf("feed cursor should be cleared on unfollow (so a resubscribe fully catches up), got %q, %v", v, err)
	}
}
