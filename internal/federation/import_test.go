package federation

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/immerle/immerle/internal/config"
	"github.com/immerle/immerle/internal/testutil"
)

func TestFetchExternalPlaylist(t *testing.T) {
	// Speed up polling for the test.
	prev := importPollInterval
	importPollInterval = 2 * time.Millisecond
	defer func() { importPollInterval = prev }()

	var postBody, postAuth, postInstance string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/spotify/imports":
			b, _ := io.ReadAll(r.Body)
			postBody = string(b)
			postAuth = r.Header.Get("Authorization")
			postInstance = r.Header.Get("X-Instance-ID")
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"jobId":"job-1","status":"pending"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/spotify/imports/job-1":
			_, _ = w.Write([]byte(`{"jobId":"job-1","status":"completed","playlist":{"name":"My Mix","description":"d"},"tracks":[
				{"title":"Da Funk","artist":"Daft Punk","album":"Homework","duration":224},
				{"title":"Around the World","artist":"Daft Punk"}
			]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	store := testutil.NewStore(t)
	// Import works even with sync disabled, as long as the instance is registered.
	cfg := config.FederationConfig{HubURL: srv.URL, InstanceID: "inst-1", PrivateKey: "iml_key-1"}
	svc := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, store.FeedCursors, nil, testLogger())

	pl, err := svc.FetchExternalPlaylist(context.Background(), "spotify", "https://open.spotify.com/playlist/PL?si=x")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(postBody, `"playlist":"https://open.spotify.com/playlist/PL?si=x"`) {
		t.Fatalf("ref not forwarded in body: %s", postBody)
	}
	if postAuth != "Bearer iml_key-1" || postInstance != "inst-1" {
		t.Fatalf("hub auth headers wrong: auth=%q instance=%q", postAuth, postInstance)
	}
	if pl.Name != "My Mix" || len(pl.Tracks) != 2 {
		t.Fatalf("playlist decode wrong: %+v", pl)
	}
	if pl.Tracks[0].Title != "Da Funk" || pl.Tracks[0].Artist != "Daft Punk" {
		t.Fatalf("track 0 wrong: %+v", pl.Tracks[0])
	}
}

func TestFetchExternalPlaylistFailedJob(t *testing.T) {
	prev := importPollInterval
	importPollInterval = 2 * time.Millisecond
	defer func() { importPollInterval = prev }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"jobId":"job-1","status":"pending"}`))
			return
		}
		_, _ = w.Write([]byte(`{"jobId":"job-1","status":"failed","error":"playlist is private"}`))
	}))
	defer srv.Close()

	store := testutil.NewStore(t)
	cfg := config.FederationConfig{HubURL: srv.URL, InstanceID: "i", PrivateKey: "iml_k"}
	svc := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, store.FeedCursors, nil, testLogger())
	if _, err := svc.FetchExternalPlaylist(context.Background(), "spotify", "PL"); err == nil ||
		!strings.Contains(err.Error(), "playlist is private") {
		t.Fatalf("expected failed-job error, got %v", err)
	}
}

// On first Register (no private key yet) the instance bootstraps under the
// configured owner user id and persists the hub-issued id/sqid/private key.
func TestRegisterBootstrapsAndPersistsCredentials(t *testing.T) {
	var path, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		body = strings.TrimSpace(string(b))
		w.WriteHeader(http.StatusCreated)
		// Bootstrap returns the fixed UUID id, an editable sqid and the key (once).
		_, _ = w.Write([]byte(`{"ok":true,"id":"3f1c-uuid","sqid":"my-node","privateKey":"iml_secret","name":"My immerle"}`))
	}))
	defer srv.Close()

	store := testutil.NewStore(t)
	cfg := config.FederationConfig{HubURL: srv.URL, UserID: "user-1", InstanceName: "My immerle"}
	svc := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, store.FeedCursors, nil, testLogger())
	var saved Credentials
	svc.SetCredentialsSaver(func(_ context.Context, c Credentials) error { saved = c; return nil })

	if err := svc.Register(context.Background()); err != nil {
		t.Fatal(err)
	}
	if path != "/api/v1/instances" {
		t.Fatalf("bootstrap should hit /api/v1/instances, hit %q", path)
	}
	// Body carries the owner user id, desired name and version.
	if !strings.Contains(body, `"userId":"user-1"`) || !strings.Contains(body, `"version":"0.2.0"`) {
		t.Fatalf("unexpected bootstrap body: %s", body)
	}
	// The hub-issued credentials are persisted back.
	if saved.InstanceID != "3f1c-uuid" || saved.Sqid != "my-node" || saved.PrivateKey != "iml_secret" {
		t.Fatalf("credentials not persisted: %+v", saved)
	}
}

func TestFetchExternalPlaylistNoHub(t *testing.T) {
	store := testutil.NewStore(t)
	svc := New(func() config.FederationConfig { return config.FederationConfig{} }, store.Catalog, store.Playlists, store.Scrobbles, store.FeedCursors, nil, testLogger())
	if _, err := svc.FetchExternalPlaylist(context.Background(), "spotify", "PL"); err == nil {
		t.Fatal("expected error when no hub is configured")
	}
}
