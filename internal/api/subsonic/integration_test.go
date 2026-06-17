package subsonic

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/immerle/immerle/internal/config"
	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/scanner"
	"github.com/immerle/immerle/internal/stream"
	"github.com/immerle/immerle/internal/testutil"
)

type testEnv struct {
	server *httptest.Server
	store  *persistence.Store
}

// jsonResponse is a minimal decoder for the subsonic-response envelope.
type jsonResponse struct {
	Response Response `json:"subsonic-response"`
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	if !testutil.FFmpegAvailable() {
		t.Skip("ffmpeg required for integration test")
	}
	store := testutil.NewStore(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx := context.Background()

	auth, err := core.NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.CreateUser(ctx, "admin", "admin", "", "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.CreateUser(ctx, "bob", "bobpass", "", "", false); err != nil {
		t.Fatal(err)
	}

	// Build a fixture library and scan it.
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
	scan := scanner.New(store.Catalog, store.Genres, scanner.NewExtractor("ffprobe"), coversDir, logger)
	if _, err := scan.ScanPaths(ctx, []string{lib}); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(Deps{
		Auth:             auth,
		Catalog:          store.Catalog,
		Genres:           store.Genres,
		Annotations:      store.Annotations,
		Playlists:        store.Playlists,
		PlayQueues:       store.PlayQueues,
		Scrobbles:        store.Scrobbles,
		Shares:           store.Shares,
		Users:            store.Users,
		Cover:            stream.NewCoverService(store.Catalog, coversDir),
		Streamer:         stream.NewStreamer(config.TranscodeConfig{FFmpegPath: "ffmpeg", CacheDir: filepath.Join(t.TempDir(), "tc")}, logger),
		NowPlaying:       core.NewNowPlayingTracker(0),
		Logger:           logger,
		MusicFolderPaths: []string{lib},
	})
	mux := http.NewServeMux()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &testEnv{server: srv, store: store}
}

// get performs an authenticated JSON request and decodes the response envelope.
func (e *testEnv) get(t *testing.T, user, pass, endpoint string, params map[string]string) Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, e.server.URL+"/rest/"+endpoint, nil)
	q := req.URL.Query()
	q.Set("u", user)
	q.Set("p", pass)
	q.Set("c", "test")
	q.Set("v", "1.16.1")
	q.Set("f", "json")
	for k, v := range params {
		q.Set(k, v)
	}
	req.URL.RawQuery = q.Encode()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var jr jsonResponse
	if err := json.NewDecoder(resp.Body).Decode(&jr); err != nil {
		t.Fatalf("decode %s: %v", endpoint, err)
	}
	return jr.Response
}

func TestBrowsingAndSearch(t *testing.T) {
	e := newTestEnv(t)

	// getArtists returns both artists.
	r := e.get(t, "admin", "admin", "getArtists", nil)
	count := 0
	for _, idx := range r.Artists.Index {
		count += len(idx.Artist)
	}
	if count != 2 {
		t.Fatalf("expected 2 artists, got %d", count)
	}

	// getAlbumList2 returns 2 albums.
	r = e.get(t, "admin", "admin", "getAlbumList2", map[string]string{"type": "alphabeticalByName", "size": "10"})
	if len(r.AlbumList2.Album) != 2 {
		t.Fatalf("expected 2 albums, got %d", len(r.AlbumList2.Album))
	}

	// search3 finds the track.
	r = e.get(t, "admin", "admin", "search3", map[string]string{"query": "So What"})
	if len(r.SearchResult3.Song) != 1 || r.SearchResult3.Song[0].Title != "So What" {
		t.Fatalf("search3 failed: %+v", r.SearchResult3.Song)
	}

	// getGenres returns the genres.
	r = e.get(t, "admin", "admin", "getGenres", nil)
	if len(r.Genres.Genre) < 2 {
		t.Fatalf("expected >=2 genres, got %d", len(r.Genres.Genre))
	}
}

func TestPerUserAnnotationsAreIsolated(t *testing.T) {
	e := newTestEnv(t)
	// find a song id via search
	r := e.get(t, "admin", "admin", "search3", map[string]string{"query": "One More Time"})
	if len(r.SearchResult3.Song) == 0 {
		t.Fatal("song not found")
	}
	songID := r.SearchResult3.Song[0].ID

	// admin stars it; bob does not.
	e.get(t, "admin", "admin", "star", map[string]string{"id": songID})

	adminStarred := e.get(t, "admin", "admin", "getStarred2", nil)
	if len(adminStarred.Starred2.Song) != 1 {
		t.Fatalf("admin should have 1 starred song, got %d", len(adminStarred.Starred2.Song))
	}
	bobStarred := e.get(t, "bob", "bobpass", "getStarred2", nil)
	if len(bobStarred.Starred2.Song) != 0 {
		t.Fatalf("bob should have 0 starred songs, got %d", len(bobStarred.Starred2.Song))
	}
}

func TestScrobbleIncrementsPlayCount(t *testing.T) {
	e := newTestEnv(t)
	r := e.get(t, "bob", "bobpass", "search3", map[string]string{"query": "Aerodynamic"})
	songID := r.SearchResult3.Song[0].ID

	e.get(t, "bob", "bobpass", "scrobble", map[string]string{"id": songID, "submission": "true"})
	e.get(t, "bob", "bobpass", "scrobble", map[string]string{"id": songID, "submission": "true"})

	got := e.get(t, "bob", "bobpass", "getSong", map[string]string{"id": songID})
	if got.Song == nil || got.Song.PlayCount != 2 {
		t.Fatalf("expected play count 2, got %+v", got.Song)
	}
}

func TestPlaylistsAndPlayQueueSync(t *testing.T) {
	e := newTestEnv(t)
	r := e.get(t, "bob", "bobpass", "search3", map[string]string{"query": "So What"})
	songID := r.SearchResult3.Song[0].ID

	// create playlist
	created := e.get(t, "bob", "bobpass", "createPlaylist", map[string]string{"name": "Mine", "songId": songID})
	if created.Playlist == nil || created.Playlist.SongCount != 1 {
		t.Fatalf("playlist not created: %+v", created.Playlist)
	}

	// bob sees it, admin does not (it is private).
	bobLists := e.get(t, "bob", "bobpass", "getPlaylists", nil)
	if len(bobLists.Playlists.Playlist) != 1 {
		t.Fatalf("bob should see 1 playlist, got %d", len(bobLists.Playlists.Playlist))
	}

	// play queue syncs across "devices" (same user, two requests).
	e.get(t, "bob", "bobpass", "savePlayQueue", map[string]string{"id": songID, "current": songID, "position": "4200"})
	q := e.get(t, "bob", "bobpass", "getPlayQueue", nil)
	if q.PlayQueue == nil || q.PlayQueue.Current != songID || q.PlayQueue.Position != 4200 {
		t.Fatalf("play queue did not sync: %+v", q.PlayQueue)
	}
}

func TestGetArtistIncludeSongs(t *testing.T) {
	e := newTestEnv(t)

	// Locate Daft Punk's id.
	artists := e.get(t, "admin", "admin", "getArtists", nil)
	var daftID string
	for _, idx := range artists.Artists.Index {
		for _, a := range idx.Artist {
			if a.Name == "Daft Punk" {
				daftID = a.ID
			}
		}
	}
	if daftID == "" {
		t.Fatal("Daft Punk not found")
	}

	// Without includeSongs: albums carry no inline songs.
	r := e.get(t, "admin", "admin", "getArtist", map[string]string{"id": daftID})
	if len(r.Artist.Album) == 0 || len(r.Artist.Album[0].Song) != 0 {
		t.Fatalf("default getArtist must not inline songs, got %+v", r.Artist.Album)
	}

	// With includeSongs: the album's songs are inlined.
	r = e.get(t, "admin", "admin", "getArtist", map[string]string{"id": daftID, "includeSongs": "true"})
	if len(r.Artist.Album) == 0 || len(r.Artist.Album[0].Song) != 2 {
		t.Fatalf("includeSongs should inline the 2 Discovery tracks, got %+v", r.Artist.Album)
	}
}

func TestAdditionalBrowsingEndpoints(t *testing.T) {
	e := newTestEnv(t)

	// getSongsByGenre: House has 2 songs (Daft Punk/Discovery).
	r := e.get(t, "admin", "admin", "getSongsByGenre", map[string]string{"genre": "House"})
	if r.SongsByGenre == nil || len(r.SongsByGenre.Song) != 2 {
		t.Fatalf("getSongsByGenre House expected 2, got %+v", r.SongsByGenre)
	}

	// getRandomSongs: returns all 3 fixtures when size >= 3.
	r = e.get(t, "admin", "admin", "getRandomSongs", map[string]string{"size": "10"})
	if r.RandomSongs == nil || len(r.RandomSongs.Song) != 3 {
		t.Fatalf("getRandomSongs expected 3, got %+v", r.RandomSongs)
	}

	// getAlbumList (v1, non-ID3): 2 albums as directory entries.
	r = e.get(t, "admin", "admin", "getAlbumList", map[string]string{"type": "alphabeticalByName"})
	if r.AlbumList == nil || len(r.AlbumList.Album) != 2 {
		t.Fatalf("getAlbumList expected 2, got %+v", r.AlbumList)
	}
	if !r.AlbumList.Album[0].IsDir {
		t.Fatal("getAlbumList albums should be directories")
	}

	// getMusicDirectory on an artist → albums; on an album → songs.
	artists := e.get(t, "admin", "admin", "getArtists", nil)
	var daftID string
	for _, idx := range artists.Artists.Index {
		for _, a := range idx.Artist {
			if a.Name == "Daft Punk" {
				daftID = a.ID
			}
		}
	}
	if daftID == "" {
		t.Fatal("Daft Punk not found")
	}
	dir := e.get(t, "admin", "admin", "getMusicDirectory", map[string]string{"id": daftID})
	if dir.Directory == nil || len(dir.Directory.Child) != 1 {
		t.Fatalf("artist directory expected 1 album, got %+v", dir.Directory)
	}
	albumID := dir.Directory.Child[0].ID
	dir = e.get(t, "admin", "admin", "getMusicDirectory", map[string]string{"id": albumID})
	if dir.Directory == nil || len(dir.Directory.Child) != 2 {
		t.Fatalf("album directory expected 2 songs, got %+v", dir.Directory)
	}

	// search2 (non-ID3): finds the Daft Punk artist.
	r = e.get(t, "admin", "admin", "search2", map[string]string{"query": "Daft"})
	if r.SearchResult2 == nil || len(r.SearchResult2.Artist) != 1 {
		t.Fatalf("search2 expected 1 artist, got %+v", r.SearchResult2)
	}

	// Stub endpoints return valid empty payloads (clients must not error).
	if e.get(t, "admin", "admin", "getVideos", nil).Videos == nil {
		t.Fatal("getVideos should return an (empty) videos element")
	}
	if e.get(t, "admin", "admin", "getInternetRadioStations", nil).InternetRadioStations == nil {
		t.Fatal("getInternetRadioStations should return an (empty) element")
	}
}

func TestPlayStatsDriveAlbumLists(t *testing.T) {
	e := newTestEnv(t)

	// Find a song in "Kind of Blue" and scrobble it 3 times as bob.
	r := e.get(t, "bob", "bobpass", "search3", map[string]string{"query": "So What"})
	songID := r.SearchResult3.Song[0].ID
	for i := 0; i < 3; i++ {
		e.get(t, "bob", "bobpass", "scrobble", map[string]string{"id": songID, "submission": "true"})
	}
	// And one Daft Punk song once.
	r = e.get(t, "bob", "bobpass", "search3", map[string]string{"query": "Aerodynamic"})
	e.get(t, "bob", "bobpass", "scrobble", map[string]string{"id": r.SearchResult3.Song[0].ID, "submission": "true"})

	// frequent: most-played album first → "Kind of Blue".
	freq := e.get(t, "bob", "bobpass", "getAlbumList2", map[string]string{"type": "frequent"})
	if len(freq.AlbumList2.Album) == 0 || freq.AlbumList2.Album[0].Name != "Kind of Blue" {
		t.Fatalf("frequent should rank Kind of Blue first, got %+v", albumNames(freq.AlbumList2.Album))
	}

	// recent: recently played albums present (both played).
	recent := e.get(t, "bob", "bobpass", "getAlbumList2", map[string]string{"type": "recent"})
	if len(recent.AlbumList2.Album) != 2 {
		t.Fatalf("recent should list the 2 played albums, got %d", len(recent.AlbumList2.Album))
	}

	// A user with no plays sees an empty frequent list (per-user stats).
	adminFreq := e.get(t, "admin", "admin", "getAlbumList2", map[string]string{"type": "frequent"})
	if len(adminFreq.AlbumList2.Album) != 0 {
		t.Fatalf("admin has no plays → frequent must be empty, got %d", len(adminFreq.AlbumList2.Album))
	}

	// getTopSongs for Miles Davis ranks the most-played track first.
	top := e.get(t, "bob", "bobpass", "getTopSongs", map[string]string{"artist": "Miles Davis"})
	if top.TopSongs == nil || len(top.TopSongs.Song) == 0 || top.TopSongs.Song[0].Title != "So What" {
		t.Fatalf("top songs should rank 'So What' first, got %+v", top.TopSongs)
	}
}

func albumNames(albums []AlbumID3) []string {
	out := make([]string, len(albums))
	for i, a := range albums {
		out[i] = a.Name
	}
	return out
}

func TestAPITokenAuthenticatesRequests(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	// Mint a token for bob directly via the auth service.
	auth, _ := core.NewAuthService(e.store.Users, e.store.APITokens, e.store.Devices, "secret")
	bob, _ := e.store.Users.GetByUsername(ctx, "bob")
	secret, _, err := auth.CreateAPIToken(ctx, bob.ID, "test", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Use the token (Authorization: Bearer) instead of u/p — should authenticate.
	req, _ := http.NewRequest(http.MethodGet, e.server.URL+"/rest/getArtists?c=test&v=1.16.1&f=json", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var jr jsonResponse
	_ = json.NewDecoder(resp.Body).Decode(&jr)
	if jr.Response.Status != "ok" || jr.Response.Artists == nil {
		t.Fatalf("bearer-token auth failed: %+v", jr.Response)
	}

	// Same via the apiKey query param.
	jr = jsonResponse{}
	resp2, err := http.Get(e.server.URL + "/rest/getArtists?c=test&f=json&apiKey=" + secret)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	_ = json.NewDecoder(resp2.Body).Decode(&jr)
	if jr.Response.Status != "ok" {
		t.Fatalf("apiKey auth failed: %+v", jr.Response)
	}

	// A bogus token is rejected.
	req3, _ := http.NewRequest(http.MethodGet, e.server.URL+"/rest/getArtists?c=test&f=json", nil)
	req3.Header.Set("Authorization", "Bearer gsk_bogus")
	resp3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	jr = jsonResponse{}
	_ = json.NewDecoder(resp3.Body).Decode(&jr)
	if jr.Response.Status != "failed" {
		t.Fatalf("bogus token should be rejected, got %+v", jr.Response)
	}
}

func TestStreamServesAudioWithRange(t *testing.T) {
	e := newTestEnv(t)
	r := e.get(t, "admin", "admin", "search3", map[string]string{"query": "So What"})
	songID := r.SearchResult3.Song[0].ID

	// Range request should yield 206 Partial Content.
	req, _ := http.NewRequest(http.MethodGet, e.server.URL+"/rest/stream", nil)
	q := req.URL.Query()
	q.Set("u", "admin")
	q.Set("p", "admin")
	q.Set("c", "test")
	q.Set("id", songID)
	q.Set("format", "raw")
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Range", "bytes=0-99")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("expected 206 for range request, got %d", resp.StatusCode)
	}
}
