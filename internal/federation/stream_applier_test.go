package federation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/config"
	"github.com/immerle/immerle/internal/federation/hub"
	"github.com/immerle/immerle/internal/federation/stream"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/testutil"
)

// subsStubHub serves only /api/v1/instances/me/subscriptions, returning the
// given instance ids as followed.
func subsStubHub(t *testing.T, followed ...string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		summaries := make([]hub.PublicInstanceSummary, 0, len(followed))
		for _, id := range followed {
			id := id
			summaries = append(summaries, hub.PublicInstanceSummary{Id: &id, Sqid: &id})
		}
		_ = json.NewEncoder(w).Encode(hub.PublicSubscriptionsResponse{Subscriptions: &summaries})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestResumeCursors(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewStore(t)
	srv := subsStubHub(t, "pub-1", "pub-2")

	cfg := config.FederationConfig{HubURL: srv.URL, InstanceID: "inst-1", PrivateKey: "key"}
	svc := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, store.FeedCursors, nil, testLogger())

	if err := store.FeedCursors.Set(ctx, "pub-1", "v5"); err != nil {
		t.Fatal(err)
	}

	cursors, err := svc.resumeCursors(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cursors["pub-1"] != "v5" {
		t.Fatalf("cursors[pub-1] = %q, want v5", cursors["pub-1"])
	}
	if cursors["pub-2"] != "" {
		t.Fatalf("cursors[pub-2] = %q, want empty (never seen)", cursors["pub-2"])
	}
}

func TestApplyStreamUpsertMaterializesAndAdvancesCursor(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewStore(t)
	now := time.Now()
	srv := subsStubHub(t, "pub-1")

	owner := models.User{ID: uuid.NewString(), Username: "admin", PasswordHash: "x", IsAdmin: true, CreatedAt: now}
	if err := store.Users.Create(ctx, owner); err != nil {
		t.Fatal(err)
	}

	cfg := config.FederationConfig{HubURL: srv.URL, InstanceID: "inst-1", PrivateKey: "key"}
	svc := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, store.FeedCursors, nil, testLogger())
	svc.SetOwner(owner.ID)

	metadata, _ := json.Marshal(map[string]string{"name": "From Pub 1", "description": "desc"})
	tracks, _ := json.Marshal([]map[string]string{{"mbid": "", "artist": "A", "title": "T"}})

	err := svc.applyStreamUpsert(ctx, stream.Frame{
		Type: stream.TypePlaylistUpsert, AuthorID: "pub-1", ExternalID: "ext-1",
		Version: "2026-07-13T10:00:00Z", Metadata: metadata, Tracks: tracks,
	})
	if err != nil {
		t.Fatal(err)
	}

	fed, err := store.Playlists.FindFederated(ctx, "pub-1", "ext-1")
	if err != nil {
		t.Fatalf("federated playlist not created: %v", err)
	}
	if fed.Name != "From Pub 1" || fed.Comment != "desc" {
		t.Fatalf("got name=%q comment=%q, want From Pub 1/desc", fed.Name, fed.Comment)
	}

	v, err := store.FeedCursors.Get(ctx, "pub-1")
	if err != nil || v != "2026-07-13T10:00:00Z" {
		t.Fatalf("cursor = %q err %v, want the applied version", v, err)
	}
}

func TestApplyStreamUpsertIgnoresUnsubscribedSource(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewStore(t)
	srv := subsStubHub(t) // no subscriptions at all

	cfg := config.FederationConfig{HubURL: srv.URL, InstanceID: "inst-1", PrivateKey: "key"}
	svc := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, store.FeedCursors, nil, testLogger())
	svc.SetOwner(uuid.NewString())

	if err := svc.applyStreamUpsert(ctx, stream.Frame{
		Type: stream.TypePlaylistUpsert, AuthorID: "pub-1", ExternalID: "ext-1", Version: "v1",
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Playlists.FindFederated(ctx, "pub-1", "ext-1"); err == nil {
		t.Fatal("expected no playlist materialized for an unsubscribed source")
	}
}

func TestApplyStreamDeleteRemovesMaterializedPlaylist(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewStore(t)
	now := time.Now()
	srv := subsStubHub(t, "pub-1")

	owner := models.User{ID: uuid.NewString(), Username: "admin", PasswordHash: "x", IsAdmin: true, CreatedAt: now}
	if err := store.Users.Create(ctx, owner); err != nil {
		t.Fatal(err)
	}

	cfg := config.FederationConfig{HubURL: srv.URL, InstanceID: "inst-1", PrivateKey: "key"}
	svc := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, store.FeedCursors, nil, testLogger())
	svc.SetOwner(owner.ID)

	if err := svc.applyStreamUpsert(ctx, stream.Frame{
		Type: stream.TypePlaylistUpsert, AuthorID: "pub-1", ExternalID: "ext-1", Version: "v1",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Playlists.FindFederated(ctx, "pub-1", "ext-1"); err != nil {
		t.Fatal("expected playlist to exist before delete")
	}

	if err := svc.applyStreamDelete(ctx, stream.Frame{Type: stream.TypePlaylistDelete, AuthorID: "pub-1", ExternalID: "ext-1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Playlists.FindFederated(ctx, "pub-1", "ext-1"); err == nil {
		t.Fatal("expected playlist to be gone after delete")
	}

	// Deleting again (already gone) is a no-op, not an error.
	if err := svc.applyStreamDelete(ctx, stream.Frame{Type: stream.TypePlaylistDelete, AuthorID: "pub-1", ExternalID: "ext-1"}); err != nil {
		t.Fatal(err)
	}
}

// TestUnlinkClosesStream covers RFC-socket-federation-client.md §10.3: Unlink
// must close the feed socket right away instead of leaving it open under
// credentials it just revoked until the next missed heartbeat.
func TestUnlinkClosesStream(t *testing.T) {
	ctx := context.Background()
	store := testutil.NewStore(t)
	srv, _ := newSyncStub(t)

	cfg := config.FederationConfig{HubURL: srv.URL, InstanceID: "inst-1", PrivateKey: "key"}
	svc := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, store.FeedCursors, nil, testLogger())
	startStream(t, svc)

	if err := svc.Unlink(ctx); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if errors.Is(svc.stream.Send(ctx, stream.Frame{Type: stream.TypeHeartbeat}), stream.ErrNotConnected) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("expected the stream to be disconnected right after Unlink")
		}
		time.Sleep(time.Millisecond)
	}
}
