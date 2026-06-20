package immerle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	chi "github.com/go-chi/chi/v5"

	"github.com/immerle/immerle/internal/config"
	"github.com/immerle/immerle/internal/core"
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
		Streamer:    stream.NewStreamer(config.TranscodeConfig{FFmpegPath: "ffmpeg", CacheDir: filepath.Join(t.TempDir(), "tc")}, testutil.NewLogger()),
		Cover:       stream.NewCoverService(store.Catalog, coversDir),
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
	if len(search.Songs) != 1 || search.Songs[0].Title != "So What" {
		t.Fatalf("search songs: %+v", search.Songs)
	}

	var song songView
	if st := getJSON(t, srv, token, "/songs/"+search.Songs[0].ID, &song); st != http.StatusOK {
		t.Fatalf("get song: status %d", st)
	}
	if song.Title != "So What" {
		t.Fatalf("song title = %q", song.Title)
	}
}
