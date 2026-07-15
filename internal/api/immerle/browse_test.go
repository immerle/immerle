package immerle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	chi "github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/config"
	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/scanner"
	"github.com/immerle/immerle/internal/stream"
	"github.com/immerle/immerle/internal/testutil"
)

// newBrowseEnv builds a handler wired with the catalog browse/mutation
// dependencies and a scanned fixture library, plus a logged-in admin token. The
// store is returned so tests can assert persisted state directly.
func newBrowseEnv(t *testing.T) (*httptest.Server, string, *persistence.Store) {
	t.Helper()
	if !testutil.FFmpegAvailable() {
		t.Skip("ffmpeg required")
	}
	store := testutil.NewStore(t)
	ctx := context.Background()
	auth, _ := core.NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	if _, err := auth.CreateUser(ctx, "admin", "adminpw", "", "", true); err != nil {
		t.Fatal(err)
	}

	lib := t.TempDir()
	gen := func(rel string, tags testutil.AudioTags) {
		p := filepath.Join(lib, rel)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		testutil.GenerateAudio(t, p, tags)
	}
	gen("Daft Punk/Discovery/01.mp3", testutil.AudioTags{Title: "One More Time", Artist: "Daft Punk", Album: "Discovery", Track: 1, Genre: "House", Year: 2001})
	gen("Daft Punk/Discovery/02.mp3", testutil.AudioTags{Title: "Aerodynamic", Artist: "Daft Punk", Album: "Discovery", Track: 2, Genre: "House", Year: 2001})
	gen("Miles Davis/Kind of Blue/01.mp3", testutil.AudioTags{Title: "So What", Artist: "Miles Davis", Album: "Kind of Blue", Track: 1, Genre: "Jazz", Year: 1959})

	coversDir := filepath.Join(t.TempDir(), "covers")
	scan := scanner.New(store.Catalog, store.Genres, scanner.NewExtractor("ffprobe"), coversDir, testutil.NewLogger())
	if _, err := scan.ScanPaths(ctx, []string{lib}); err != nil {
		t.Fatal(err)
	}

	onDemand := core.NewCatalogService(core.CatalogServiceConfig{
		Catalog: store.Catalog, Downloads: store.Downloads, Registry: core.NewProviderRegistry(), Logger: testutil.NewLogger(),
	})
	h := NewHandler(Deps{
		Auth:        auth,
		Users:       store.Users,
		Catalog:     store.Catalog,
		Annotations: store.Annotations,
		Genres:      store.Genres,
		Playlists:   store.Playlists,
		Scrobbles:   store.Scrobbles,
		PlayQueues:  store.PlayQueues,
		NowPlaying:  core.NewNowPlayingTracker(0),
		OnDemand:    onDemand,
		Streamer:    stream.NewStreamer(config.TranscodeConfig{FFmpegPath: "ffmpeg", CacheDir: filepath.Join(t.TempDir(), "tc")}, testutil.NewLogger()),
		Cover:       stream.NewCoverService(store.Catalog, coversDir),
		Shares:      store.Shares,
		BaseURL:     "https://music.example",
		SigningKey:  "test-media-secret",
		Logger:      testutil.NewLogger(),
	})
	mux := chi.NewRouter()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, login(t, srv, "admin"), store
}

// getJSON performs an authenticated GET and decodes the body into out.
func getJSON(t *testing.T, srv *httptest.Server, token, path string, out any) int {
	t.Helper()
	resp := do(t, srv, http.MethodGet, path, token, nil)
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func TestBrowseArtistsAndAlbums(t *testing.T) {
	srv, token, _ := newBrowseEnv(t)

	var list struct {
		Artists []artistView `json:"artists"`
	}
	if st := getJSON(t, srv, token, "/artists", &list); st != http.StatusOK {
		t.Fatalf("list artists: status %d", st)
	}
	if len(list.Artists) != 2 {
		t.Fatalf("expected 2 artists, got %d", len(list.Artists))
	}

	var daftID string
	for _, a := range list.Artists {
		if a.Name == "Daft Punk" {
			daftID = a.ID
		}
	}
	if daftID == "" {
		t.Fatal("Daft Punk not found")
	}

	var artist artistView
	if st := getJSON(t, srv, token, "/artists/"+daftID, &artist); st != http.StatusOK {
		t.Fatalf("get artist: status %d", st)
	}
	if len(artist.Albums) != 1 || artist.Albums[0].Name != "Discovery" {
		t.Fatalf("expected Discovery album, got %+v", artist.Albums)
	}

	var album albumView
	if st := getJSON(t, srv, token, "/albums/"+artist.Albums[0].ID, &album); st != http.StatusOK {
		t.Fatalf("get album: status %d", st)
	}
	if len(album.Tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(album.Tracks))
	}
}

func TestBrowseAuthAndNotFound(t *testing.T) {
	srv, token, _ := newBrowseEnv(t)

	if st := doStatus(t, srv, http.MethodGet, "/artists", "", nil); st != http.StatusUnauthorized {
		t.Fatalf("no token: expected 401, got %d", st)
	}
	if st := doStatus(t, srv, http.MethodGet, "/artists", "bogus", nil); st != http.StatusUnauthorized {
		t.Fatalf("bogus token: expected 401, got %d", st)
	}
	if st := doStatus(t, srv, http.MethodGet, "/artists/does-not-exist", token, nil); st != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", st)
	}
}

func TestBrowseListsSearchAndSong(t *testing.T) {
	srv, token, _ := newBrowseEnv(t)

	// Album list (alphabetical) returns both albums.
	var albums struct {
		Albums []albumView `json:"albums"`
	}
	if st := getJSON(t, srv, token, "/albums?type=alphabeticalByName&size=10", &albums); st != http.StatusOK {
		t.Fatalf("list albums: status %d", st)
	}
	if len(albums.Albums) != 2 {
		t.Fatalf("expected 2 albums, got %d", len(albums.Albums))
	}

	// Genres include House and Jazz.
	var genres struct {
		Genres []struct {
			Name string `json:"name"`
		} `json:"genres"`
	}
	if st := getJSON(t, srv, token, "/genres", &genres); st != http.StatusOK {
		t.Fatalf("genres: status %d", st)
	}
	if len(genres.Genres) < 2 {
		t.Fatalf("expected >=2 genres, got %d", len(genres.Genres))
	}

	// Search finds the track and the single resource resolves it.
	var search searchView
	if st := getJSON(t, srv, token, "/search?q=So+What", &search); st != http.StatusOK {
		t.Fatalf("search: status %d", st)
	}
	if len(search.Songs()) != 1 || search.Songs()[0].Title != "So What" {
		t.Fatalf("search songs: %+v", search.Songs())
	}

	var song songView
	if st := getJSON(t, srv, token, "/songs/"+search.Songs()[0].ID, &song); st != http.StatusOK {
		t.Fatalf("get song: status %d", st)
	}
	if song.Title != "So What" {
		t.Fatalf("song title = %q", song.Title)
	}
}

// TestSearchIsOneRelevanceRankedListIncludingPublicPlaylists covers the
// unified search: public playlists are searchable alongside artists/albums/
// songs, and the response is one list ranked by relevance to the query
// (exact match first) rather than grouped by type.
func TestSearchIsOneRelevanceRankedListIncludingPublicPlaylists(t *testing.T) {
	srv, token, store := newBrowseEnv(t)
	ctx := context.Background()

	// A public playlist owned by someone else than the searching admin (a
	// self-owned public playlist is excluded from search, same as /playlists/public).
	other := models.User{ID: uuid.NewString(), Username: "other", PasswordHash: "x", CreatedAt: time.Now()}
	if err := store.Users.Create(ctx, other); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	pub := models.Playlist{ID: uuid.NewString(), Name: "Discovery Deep Cuts", OwnerID: other.ID, Public: true, CreatedAt: now, UpdatedAt: now}
	if err := store.Playlists.Create(ctx, pub); err != nil {
		t.Fatal(err)
	}
	priv := models.Playlist{ID: uuid.NewString(), Name: "Discovery Private Mix", OwnerID: other.ID, Public: false, CreatedAt: now, UpdatedAt: now}
	if err := store.Playlists.Create(ctx, priv); err != nil {
		t.Fatal(err)
	}

	// "Discovery" matches: the Daft Punk album (exact name match) and the
	// public playlist (substring match) — but not the private one.
	var search searchView
	if st := getJSON(t, srv, token, "/search?q=Discovery", &search); st != http.StatusOK {
		t.Fatalf("search: status %d", st)
	}
	if len(search.Results) < 2 {
		t.Fatalf("expected at least an album and a playlist hit, got %+v", search.Results)
	}
	if search.Results[0].Type != "album" || search.Results[0].Album == nil || search.Results[0].Album.Name != "Discovery" {
		t.Fatalf("expected the exact-match album to rank first, got %+v", search.Results[0])
	}
	foundPlaylist, foundPrivate := false, false
	for _, r := range search.Results {
		if r.Type == "playlist" && r.Playlist != nil {
			if r.Playlist.Name == pub.Name {
				foundPlaylist = true
			}
			if r.Playlist.Name == priv.Name {
				foundPrivate = true
			}
		}
	}
	if !foundPlaylist {
		t.Fatalf("expected the public playlist in results, got %+v", search.Results)
	}
	if foundPrivate {
		t.Fatalf("private playlist must not be searchable, got %+v", search.Results)
	}
}

// TestSearchTypeParamScopesServerSide covers the `type` query param: it must
// filter server-side (the other types are never fetched at all — see
// searchCounts), not just hide them from an already-mixed response.
func TestSearchTypeParamScopesServerSide(t *testing.T) {
	srv, token, _ := newBrowseEnv(t)

	var albumOnly searchView
	if st := getJSON(t, srv, token, "/search?q=Discovery&type=album", &albumOnly); st != http.StatusOK {
		t.Fatalf("search: status %d", st)
	}
	if len(albumOnly.Results) == 0 {
		t.Fatal("expected at least the Discovery album")
	}
	for _, r := range albumOnly.Results {
		if r.Type != "album" {
			t.Fatalf("type=album must only return albums, got a %q hit: %+v", r.Type, r)
		}
	}

	var songOnly searchView
	if st := getJSON(t, srv, token, "/search?q=So+What&type=song", &songOnly); st != http.StatusOK {
		t.Fatalf("search: status %d", st)
	}
	if len(songOnly.Results) != 1 || songOnly.Results[0].Type != "song" {
		t.Fatalf("type=song must only return songs, got %+v", songOnly.Results)
	}

	var artistOnly searchView
	if st := getJSON(t, srv, token, "/search?q=So+What&type=artist", &artistOnly); st != http.StatusOK {
		t.Fatalf("search: status %d", st)
	}
	if len(artistOnly.Results) != 0 {
		t.Fatalf("type=artist for a song-only query must return nothing, got %+v", artistOnly.Results)
	}
}

// TestToAlbumViewFallsBackToAlbumIDForCoverArt covers a real bug: an album
// whose tracks only carry embedded (never cached) art has an empty
// models.Album.CoverArt, and without a fallback the API returned an empty
// coverArt — CoverArt.tsx's client-side guard then never even requests an
// image, so the album showed no cover anywhere (home screen, artist page,
// search…), while the same track played fine because toSongView already
// falls back a track's cover to its AlbumID. The cover service resolves an
// album id by extracting embedded/sidecar art live from one of its tracks
// (stream.CoverService.resolveOriginal), so falling back to the album's own
// id here recovers the same art — matching the Subsonic API's
// coverIDForAlbum in internal/api/subsonic/convert.go, which already does
// this.
func TestToAlbumViewFallsBackToAlbumIDForCoverArt(t *testing.T) {
	withCover := models.Album{ID: "album-1", Name: "Al", CoverArt: "cached-cover"}
	if got := toAlbumView(withCover, nil, nil).CoverArt; got != "cached-cover" {
		t.Fatalf("expected the cached cover to win, got %q", got)
	}

	noCover := models.Album{ID: "album-2", Name: "Al"}
	if got := toAlbumView(noCover, nil, nil).CoverArt; got != "album-2" {
		t.Fatalf("expected fallback to the album id, got %q", got)
	}
}

// TestSongLocalStatus covers the polling endpoint the player uses to upgrade
// a still-progressive-streaming track to the seekable local one once ready
// (see ui/src/audio/store.ts's seekTo, which no-ops seeks on a remote track
// otherwise). A remote id with no completed download reports local=false;
// once a download job completes — the same bookkeeping
// internal/core/ondemand_stream.go's finalizeStreamed performs — it reports
// the resolved local song.
func TestSongLocalStatus(t *testing.T) {
	srv, token, store := newBrowseEnv(t)
	ctx := context.Background()

	// Never downloaded → not local, no error.
	var status songLocalStatusView
	if st := getJSON(t, srv, token, "/songs/remote:basic:ptid-1/local", &status); st != http.StatusOK {
		t.Fatalf("status: %d", st)
	}
	if status.Local || status.Song != nil {
		t.Fatalf("expected not-local for a never-downloaded track, got %+v", status)
	}

	// A real local track from the fixture library, to resolve the remote id to.
	var search searchView
	if st := getJSON(t, srv, token, "/search?q=So+What", &search); st != http.StatusOK || len(search.Songs()) != 1 {
		t.Fatalf("search: status %d songs=%+v", st, search.Songs())
	}
	localTrackID := search.Songs()[0].ID

	// Simulate a completed background download without a real provider/ffmpeg
	// download run.
	job, err := store.Downloads.Enqueue(ctx, models.DownloadJob{
		ID: uuid.NewString(), UserID: "u1", Provider: "basic", ProviderTrackID: "ptid-1",
		Status: models.DownloadQueued, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Downloads.Complete(ctx, job.ID, localTrackID); err != nil {
		t.Fatal(err)
	}

	if st := getJSON(t, srv, token, "/songs/remote:basic:ptid-1/local", &status); st != http.StatusOK {
		t.Fatalf("status: %d", st)
	}
	if !status.Local || status.Song == nil || status.Song.ID != localTrackID {
		t.Fatalf("expected local=true with the resolved song, got %+v", status)
	}
}

// TestToSongViewExposesRemote covers the client-visible signal for a real
// bug: a not-yet-downloaded (on-demand provider) track streams progressively,
// which can't serve byte ranges — seeking it silently resets playback to
// 0:00. The client needs to know a song is still remote to disable seeking
// instead, so toSongView must expose models.Track.Remote (it's otherwise
// `json:"-"` and invisible to the API).
func TestToSongViewExposesRemote(t *testing.T) {
	if got := toSongView(models.Track{ID: "t1", Remote: true}).Remote; !got {
		t.Fatal("expected Remote to carry through to the song view")
	}
	if got := toSongView(models.Track{ID: "t2", Remote: false}).Remote; got {
		t.Fatal("expected Remote to stay false for a local track")
	}
}
