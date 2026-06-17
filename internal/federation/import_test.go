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
	// Import works even with sync disabled, as long as the hub URL + keys are set.
	cfg := config.FederationConfig{Enabled: false, HubURL: srv.URL, PublicKey: "inst-1", PrivateKey: "key-1"}
	svc := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, nil, testLogger())

	pl, err := svc.FetchExternalPlaylist(context.Background(), "spotify", "https://open.spotify.com/playlist/PL?si=x")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(postBody, `"playlist":"https://open.spotify.com/playlist/PL?si=x"`) {
		t.Fatalf("ref not forwarded in body: %s", postBody)
	}
	if postAuth != "Bearer key-1" || postInstance != "inst-1" {
		t.Fatalf("hub auth headers wrong: auth=%q instance=%q", postAuth, postInstance)
	}
	if pl.Name != "My Mix" || len(pl.Tracks) != 2 {
		t.Fatalf("playlist decode wrong: %+v", pl)
	}
	if pl.Tracks[0].Title != "Da Funk" || pl.Tracks[0].Duration != 224 {
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
	cfg := config.FederationConfig{HubURL: srv.URL, PublicKey: "i", PrivateKey: "k"}
	svc := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, nil, testLogger())
	if _, err := svc.FetchExternalPlaylist(context.Background(), "spotify", "PL"); err == nil ||
		!strings.Contains(err.Error(), "playlist is private") {
		t.Fatalf("expected failed-job error, got %v", err)
	}
}

func TestRegisterSendsKeysAndVersion(t *testing.T) {
	var auth, instance, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		instance = r.Header.Get("X-Instance-ID")
		b, _ := io.ReadAll(r.Body)
		body = strings.TrimSpace(string(b))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := testutil.NewStore(t)
	cfg := config.FederationConfig{Enabled: true, HubURL: srv.URL, PublicKey: "pub-1", PrivateKey: "priv-1"}
	svc := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, nil, testLogger())

	if err := svc.Register(context.Background()); err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer priv-1" {
		t.Fatalf("private key not sent as Bearer: %q", auth)
	}
	if instance != "pub-1" {
		t.Fatalf("public key not sent as X-Instance-ID: %q", instance)
	}
	// Body carries only the version (identity is in the headers).
	if !strings.Contains(body, `"version":"0.2.0"`) || strings.Contains(body, "instanceId") {
		t.Fatalf("unexpected register body: %s", body)
	}
}

func TestFetchExternalPlaylistNoHub(t *testing.T) {
	store := testutil.NewStore(t)
	svc := New(func() config.FederationConfig { return config.FederationConfig{} }, store.Catalog, store.Playlists, store.Scrobbles, nil, testLogger())
	if _, err := svc.FetchExternalPlaylist(context.Background(), "spotify", "PL"); err == nil {
		t.Fatal("expected error when no hub is configured")
	}
}
