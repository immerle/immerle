package federation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/config"
	"github.com/immerle/immerle/internal/federation/hub"
	"github.com/immerle/immerle/internal/federation/stream"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/outbox"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/testutil"
)

// fakeCovers serves the same bytes for any cover id.
type fakeCovers struct{ data []byte }

func (f fakeCovers) Get(_ context.Context, _ string, _ int, _ string) ([]byte, string, error) {
	return f.data, "image/jpeg", nil
}

// perIDCovers serves distinct bytes per cover id, for tests that need each
// tile to be individually addressable/distinguishable.
type perIDCovers struct{ byID map[string][]byte }

func (f perIDCovers) Get(_ context.Context, id string, _ int, _ string) ([]byte, string, error) {
	data, ok := f.byID[id]
	if !ok {
		return nil, "", errNoCovers
	}
	return data, "image/jpeg", nil
}

// solidJPEG encodes a tiny single-color JPEG, decodable by image.Decode (a
// real image, unlike the "JPEGDATA" placeholder used elsewhere in this file).
func solidJPEG(t *testing.T, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// syncStub records what the (fake) hub received: cover REST calls unchanged,
// but playlist push/delete now arrive as socket frames (RFC-socket-federation-
// client.md §7) instead of REST PUT/DELETE.
type syncStub struct {
	mu          sync.Mutex
	missing     [][]string
	uploaded    []string
	upsert      *stream.Frame
	upsertCount int
	deleted     string // externalId of the last playlist.delete frame
	replayReply *stream.Frame
	conn        *websocket.Conn // set once the client has connected
}

func (st *syncStub) lastUpsert() stream.Frame {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.upsert == nil {
		return stream.Frame{}
	}
	return *st.upsert
}

func (st *syncStub) numUpserts() int {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.upsertCount
}

func (st *syncStub) deletedID() string {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.deleted
}

func (st *syncStub) lastReplayReply() *stream.Frame {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.replayReply
}

// sendToClient pushes a frame from the fake hub down to the connected client,
// waiting for the connection to exist first.
func (st *syncStub) sendToClient(t *testing.T, f stream.Frame) {
	t.Helper()
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	var conn *websocket.Conn
	for conn == nil {
		if time.Now().After(deadline) {
			t.Fatal("client never connected")
		}
		st.mu.Lock()
		conn = st.conn
		st.mu.Unlock()
		if conn == nil {
			time.Sleep(time.Millisecond)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatal(err)
	}
}

func newSyncStub(t *testing.T) (*httptest.Server, *syncStub) {
	st := &syncStub{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/covers/missing", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Hashes []string `json:"hashes"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		st.mu.Lock()
		st.missing = append(st.missing, req.Hashes)
		st.mu.Unlock()
		// The hub has none of them yet → all missing.
		_ = json.NewEncoder(w).Encode(hub.PublicMissingCoversResponse{Missing: &req.Hashes})
	})
	mux.HandleFunc("/api/v1/covers/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			st.mu.Lock()
			st.uploaded = append(st.uploaded, strings.TrimPrefix(r.URL.Path, "/api/v1/covers/"))
			st.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		}
	})
	mux.HandleFunc("/api/v1/instances/me/data", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	})
	mux.HandleFunc("/api/v1/instances/me/stream", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		st.mu.Lock()
		st.conn = c
		st.mu.Unlock()
		for {
			_, data, err := c.Read(r.Context())
			if err != nil {
				return
			}
			var f stream.Frame
			if json.Unmarshal(data, &f) != nil {
				continue
			}
			st.mu.Lock()
			switch {
			case f.Type == stream.TypePlaylistUpsert && f.Target != "":
				frame := f
				st.replayReply = &frame
			case f.Type == stream.TypePlaylistUpsert:
				frame := f
				st.upsert = &frame
				st.upsertCount++
			case f.Type == stream.TypePlaylistDelete:
				st.deleted = f.ExternalID
			}
			st.mu.Unlock()
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, st
}

// startStream starts the federation socket against the stub and blocks until
// it's actually connected (so the test's Send calls don't race the dial).
func startStream(t *testing.T, fed *Service) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go fed.RunStream(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := fed.stream.Send(context.Background(), stream.Frame{Type: stream.TypeHeartbeat}); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("federation stream never connected")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestOutboxWorkerSyncsAndDeletesPublicPlaylist(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	store := testutil.NewStore(t)

	owner := models.User{ID: uuid.NewString(), Username: "admin", PasswordHash: "x", IsAdmin: true, CreatedAt: now}
	_ = store.Users.Create(ctx, owner)
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "Daft Punk", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Discovery", ArtistID: artistID, CreatedAt: now})
	trackID, _ := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "One More Time", AlbumID: albumID, ArtistID: artistID,
		Genre: "House", Year: 2001, Duration: 320, Path: "/p.mp3", CreatedAt: now, UpdatedAt: now,
	})

	plID := uuid.NewString()
	_ = store.Playlists.Create(ctx, models.Playlist{ID: plID, Name: "Summer Hits", OwnerID: owner.ID, Public: true, CreatedAt: now, UpdatedAt: now})
	_ = store.Playlists.ReplaceTracks(ctx, plID, []string{trackID}, owner.ID)

	srv, st := newSyncStub(t)
	cfg := config.FederationConfig{HubURL: srv.URL, InstanceID: "inst-1", PrivateKey: "iml_key", SyncPlaylists: true}
	fed := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, store.FeedCursors, nil, testLogger())
	startStream(t, fed)
	worker := outbox.NewWorker(store.Outbox, testLogger())
	s := NewPlaylistSyncer(fed, worker, store.PlaylistSync, store.CoverUploads, store.Playlists, fakeCovers{data: []byte("JPEGDATA")}, testLogger())
	job := persistence.OutboxJob{Kind: PlaylistSyncKind, DedupeKey: plID}

	// Upsert flow.
	if err := s.handle(ctx, job); err != nil {
		t.Fatal(err)
	}

	wantHash := func() string { s := sha256.Sum256([]byte("JPEGDATA")); return hex.EncodeToString(s[:]) }()
	upsert := st.lastUpsert()
	if upsert.ExternalID != plID {
		t.Fatalf("upsert externalId = %q, want %s", upsert.ExternalID, plID)
	}
	st.mu.Lock()
	if len(st.missing) != 1 || len(st.missing[0]) != 1 || st.missing[0][0] != wantHash {
		t.Fatalf("covers/missing not called with the cover hash: %+v", st.missing)
	}
	if len(st.uploaded) != 1 || st.uploaded[0] != wantHash {
		t.Fatalf("cover not uploaded: %+v", st.uploaded)
	}
	st.mu.Unlock()
	if !strings.Contains(string(upsert.Metadata), `"name":"Summer Hits"`) {
		t.Fatalf("unexpected metadata: %s", upsert.Metadata)
	}
	if !strings.Contains(string(upsert.Tracks), `"title":"One More Time"`) ||
		!strings.Contains(string(upsert.Tracks), `"cover":"/api/v1/covers/`+wantHash+`"`) ||
		!strings.Contains(string(upsert.Tracks), `"seconds":320`) {
		t.Fatalf("unexpected tracks: %s", upsert.Tracks)
	}

	// Sync state recorded.
	if h, _ := store.PlaylistSync.Hash(ctx, plID); h == "" {
		t.Fatal("playlist_sync hash not recorded")
	}
	if payload, version, _ := store.PlaylistSync.LastPayload(ctx, plID); payload == "" || version == "" {
		t.Fatal("playlist_sync resolved payload/version not recorded")
	}

	// Unchanged → handle again → no new upsert.
	prevCount := st.numUpserts()
	if err := s.handle(ctx, job); err != nil {
		t.Fatal(err)
	}
	if st.numUpserts() != prevCount {
		t.Fatal("unchanged playlist should not re-push")
	}

	// Disabling sync purges: it enqueues a delete job for every synced playlist.
	if err := s.PurgePlaylists(ctx); err != nil {
		t.Fatal(err)
	}
	pj, err := store.Outbox.ClaimNext(ctx, time.Now())
	if err != nil || pj.DedupeKey != plID {
		t.Fatalf("purge should enqueue a job for %s, got %+v err=%v", plID, pj, err)
	}

	// With sync turned off, the handler deletes the (still public) playlist.
	cfg.SyncPlaylists = false
	if err := s.handle(ctx, job); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for st.deletedID() != plID {
		if time.Now().After(deadline) {
			t.Fatalf("expected a playlist.delete for %s, got %q", plID, st.deletedID())
		}
		time.Sleep(time.Millisecond)
	}
}

func TestPlaylistSyncComposesMosaicWhenNoCustomCover(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	store := testutil.NewStore(t)

	owner := models.User{ID: uuid.NewString(), Username: "admin", PasswordHash: "x", IsAdmin: true, CreatedAt: now}
	_ = store.Users.Create(ctx, owner)
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: now})
	trackA, _ := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "A", AlbumID: albumID, ArtistID: artistID, CoverArt: "cover-a",
		Path: "/a.mp3", CreatedAt: now, UpdatedAt: now,
	})
	trackB, _ := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "B", AlbumID: albumID, ArtistID: artistID, CoverArt: "cover-b",
		Path: "/b.mp3", CreatedAt: now, UpdatedAt: now,
	})

	plID := uuid.NewString()
	_ = store.Playlists.Create(ctx, models.Playlist{ID: plID, Name: "Mosaic Test", OwnerID: owner.ID, Public: true, CreatedAt: now, UpdatedAt: now})
	_ = store.Playlists.ReplaceTracks(ctx, plID, []string{trackA, trackB}, owner.ID)

	srv, st := newSyncStub(t)
	cfg := config.FederationConfig{HubURL: srv.URL, InstanceID: "inst-1", PrivateKey: "iml_key", SyncPlaylists: true}
	fed := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, store.FeedCursors, nil, testLogger())
	startStream(t, fed)
	worker := outbox.NewWorker(store.Outbox, testLogger())
	covers := perIDCovers{byID: map[string][]byte{
		"cover-a": solidJPEG(t, color.RGBA{R: 255, A: 255}),
		"cover-b": solidJPEG(t, color.RGBA{B: 255, A: 255}),
	}}
	s := NewPlaylistSyncer(fed, worker, store.PlaylistSync, store.CoverUploads, store.Playlists, covers, testLogger())

	if err := s.handle(ctx, persistence.OutboxJob{Kind: PlaylistSyncKind, DedupeKey: plID}); err != nil {
		t.Fatal(err)
	}

	image := st.lastUpsert().Image
	if !strings.HasPrefix(image, "/api/v1/covers/") {
		t.Fatalf("expected a composed mosaic cover url, got %q", image)
	}
	hash := strings.TrimPrefix(image, "/api/v1/covers/")
	st.mu.Lock()
	defer st.mu.Unlock()
	found := false
	for _, h := range st.uploaded {
		if h == hash {
			found = true
		}
	}
	if !found {
		t.Fatalf("mosaic %s was not uploaded, uploaded=%v", hash, st.uploaded)
	}
}

func TestPlaylistSyncKeepsCustomCoverOverMosaic(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	store := testutil.NewStore(t)

	owner := models.User{ID: uuid.NewString(), Username: "admin2", PasswordHash: "x", IsAdmin: true, CreatedAt: now}
	_ = store.Users.Create(ctx, owner)
	artistID, _ := store.Catalog.UpsertArtist(ctx, models.Artist{ID: uuid.NewString(), Name: "A", CreatedAt: now})
	albumID, _ := store.Catalog.UpsertAlbum(ctx, models.Album{ID: uuid.NewString(), Name: "Al", ArtistID: artistID, CreatedAt: now})
	trackID, _ := store.Catalog.UpsertTrack(ctx, models.Track{
		ID: uuid.NewString(), Title: "A", AlbumID: albumID, ArtistID: artistID, CoverArt: "cover-a",
		Path: "/a.mp3", CreatedAt: now, UpdatedAt: now,
	})

	plID := uuid.NewString()
	_ = store.Playlists.Create(ctx, models.Playlist{ID: plID, Name: "Custom Cover Test", OwnerID: owner.ID, Public: true, CreatedAt: now, UpdatedAt: now})
	_ = store.Playlists.SetCover(ctx, plID, "custom-cover")
	_ = store.Playlists.ReplaceTracks(ctx, plID, []string{trackID}, owner.ID)

	srv, st := newSyncStub(t)
	cfg := config.FederationConfig{HubURL: srv.URL, InstanceID: "inst-1", PrivateKey: "iml_key", SyncPlaylists: true}
	fed := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, store.FeedCursors, nil, testLogger())
	startStream(t, fed)
	worker := outbox.NewWorker(store.Outbox, testLogger())
	covers := perIDCovers{byID: map[string][]byte{
		"custom-cover": solidJPEG(t, color.RGBA{G: 255, A: 255}),
		"cover-a":      solidJPEG(t, color.RGBA{R: 255, A: 255}),
	}}
	s := NewPlaylistSyncer(fed, worker, store.PlaylistSync, store.CoverUploads, store.Playlists, covers, testLogger())

	if err := s.handle(ctx, persistence.OutboxJob{Kind: PlaylistSyncKind, DedupeKey: plID}); err != nil {
		t.Fatal(err)
	}

	wantHash := func() string {
		sum := sha256.Sum256(covers.byID["custom-cover"])
		return hex.EncodeToString(sum[:])
	}()
	if image := st.lastUpsert().Image; image != "/api/v1/covers/"+wantHash {
		t.Fatalf("expected the custom cover, not a mosaic, got image=%q", image)
	}
}

// TestPlaylistSyncerAnswersReplayRequest covers RFC-socket-federation-client.md
// §6: a replay.request for a subscriber whose cursor is behind our last pushed
// version gets a unicast playlist.upsert reply (Target set), replayed from the
// stored resolved payload — no recomputation.
func TestPlaylistSyncerAnswersReplayRequest(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	store := testutil.NewStore(t)

	owner := models.User{ID: uuid.NewString(), Username: "admin", PasswordHash: "x", IsAdmin: true, CreatedAt: now}
	_ = store.Users.Create(ctx, owner)
	plID := uuid.NewString()
	_ = store.Playlists.Create(ctx, models.Playlist{ID: plID, Name: "Replay Me", OwnerID: owner.ID, Public: true, CreatedAt: now, UpdatedAt: now})

	srv, st := newSyncStub(t)
	cfg := config.FederationConfig{HubURL: srv.URL, InstanceID: "inst-1", PrivateKey: "iml_key", SyncPlaylists: true}
	fed := New(func() config.FederationConfig { return cfg }, store.Catalog, store.Playlists, store.Scrobbles, store.FeedCursors, nil, testLogger())
	startStream(t, fed)
	worker := outbox.NewWorker(store.Outbox, testLogger())
	NewPlaylistSyncer(fed, worker, store.PlaylistSync, store.CoverUploads, store.Playlists, fakeCovers{data: []byte("JPEGDATA")}, testLogger())

	// Directly seed the sync state, as if this playlist had already been pushed
	// once (no need to go through the outbox for this test).
	version := "2026-07-13T10:00:00Z"
	resolved, _ := json.Marshal(syncPayload{Name: "Replay Me", Tracks: []syncTrack{}})
	if err := store.PlaylistSync.SetPayload(ctx, plID, string(resolved), version); err != nil {
		t.Fatal(err)
	}

	st.sendToClient(t, stream.Frame{Type: stream.TypeReplayRequest, ForSubscriberID: "sub-1", SinceVersion: ""})

	deadline := time.Now().Add(2 * time.Second)
	for st.lastReplayReply() == nil {
		if time.Now().After(deadline) {
			t.Fatal("no replay reply received")
		}
		time.Sleep(time.Millisecond)
	}
	reply := st.lastReplayReply()
	if reply.ExternalID != plID || reply.Target != "sub-1" || reply.Version != version {
		t.Fatalf("got reply %+v, want externalId=%s target=sub-1 version=%s", reply, plID, version)
	}

	// A subscriber whose cursor is already at (or past) our version gets no
	// replay for this playlist.
	st.mu.Lock()
	st.replayReply = nil
	st.mu.Unlock()
	st.sendToClient(t, stream.Frame{Type: stream.TypeReplayRequest, ForSubscriberID: "sub-2", SinceVersion: version})
	time.Sleep(50 * time.Millisecond)
	if st.lastReplayReply() != nil {
		t.Fatalf("expected no replay for an up-to-date subscriber, got %+v", st.lastReplayReply())
	}
}
