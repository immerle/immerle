package subsonic

import (
	"context"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	chi "github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/api/httputil"
	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/scanner"
	"github.com/immerle/immerle/internal/stream"
)

// Deps holds the dependencies of the Subsonic handler. Optional fields (OnDemand)
// may be nil when the feature is disabled.
type Deps struct {
	Auth        *core.AuthService
	Catalog     *persistence.CatalogRepo
	Genres      *persistence.GenreRepo
	Annotations *persistence.AnnotationRepo
	Playlists   *persistence.PlaylistRepo
	PlayQueues  *persistence.PlayQueueRepo
	Scrobbles   *persistence.ScrobbleRepo
	Shares      *persistence.ShareRepo
	Users       *persistence.UserRepo
	Cover       *stream.CoverService
	Streamer    *stream.Streamer
	NowPlaying  *core.NowPlayingTracker
	Scanner     *scanner.Scanner
	OnDemand    *core.CatalogService
	Activity    *core.ActivityService
	// MusicFolderPaths are the configured library roots, exposed as music folders.
	MusicFolderPaths []string
	// BaseURL is used to build absolute share links.
	BaseURL string
	Logger  *slog.Logger
}

// Handler implements the Subsonic REST API.
type Handler struct {
	Deps
}

// NewHandler builds a Subsonic handler.
func NewHandler(d Deps) *Handler {
	return &Handler{Deps: d}
}

type ctxKey int

const userKey ctxKey = iota

// Register mounts all Subsonic endpoints on mux under /rest/.
func (h *Handler) Register(mux chi.Router) {
	endpoints := map[string]http.HandlerFunc{
		"ping":                      h.handlePing,
		"getLicense":                h.handleGetLicense,
		"getOpenSubsonicExtensions": h.handleGetOpenSubsonicExtensions,
		"getScanStatus":             h.handleGetScanStatus,
		"startScan":                 h.handleStartScan,
		"getMusicFolders":           h.handleGetMusicFolders,
		"getIndexes":                h.handleGetIndexes,
		"getArtists":                h.handleGetArtists,
		"getArtist":                 h.handleGetArtist,
		"getAlbum":                  h.handleGetAlbum,
		"getAlbumList":              h.handleGetAlbumList,
		"getAlbumList2":             h.handleGetAlbumList2,
		"getSong":                   h.handleGetSong,
		"getGenres":                 h.handleGetGenres,
		"getMusicDirectory":         h.handleGetMusicDirectory,
		"getSongsByGenre":           h.handleGetSongsByGenre,
		"getRandomSongs":            h.handleGetRandomSongs,
		"getStarred":                h.handleGetStarred,
		"getTopSongs":               h.handleGetTopSongs,
		"getSimilarSongs":           h.handleGetSimilarSongs,
		"getSimilarSongs2":          h.handleGetSimilarSongs2,
		"getArtistInfo":             h.handleGetArtistInfo,
		"getArtistInfo2":            h.handleGetArtistInfo2,
		"getAlbumInfo":              h.handleGetAlbumInfo,
		"getAlbumInfo2":             h.handleGetAlbumInfo,
		"getLyrics":                 h.handleGetLyrics,
		"getLyricsBySongId":         h.handleGetLyricsBySongID,
		"getVideos":                 h.handleGetVideos,
		"getBookmarks":              h.handleGetBookmarks,
		"getInternetRadioStations":  h.handleGetInternetRadioStations,
		"getChatMessages":           h.handleGetChatMessages,
		"search":                    h.handleSearch2,
		"search2":                   h.handleSearch2,
		"search3":                   h.handleSearch3,
		"getCoverArt":               h.handleGetCoverArt,
		"stream":                    h.handleStream,
		"download":                  h.handleDownload,
		"scrobble":                  h.handleScrobble,
		"getNowPlaying":             h.handleGetNowPlaying,
		"star":                      h.handleStar,
		"unstar":                    h.handleUnstar,
		"setRating":                 h.handleSetRating,
		"getStarred2":               h.handleGetStarred2,
		"getPlaylists":              h.handleGetPlaylists,
		"getPlaylist":               h.handleGetPlaylist,
		"createPlaylist":            h.handleCreatePlaylist,
		"updatePlaylist":            h.handleUpdatePlaylist,
		"deletePlaylist":            h.handleDeletePlaylist,
		"getPlayQueue":              h.handleGetPlayQueue,
		"savePlayQueue":             h.handleSavePlayQueue,
		"getUser":                   h.handleGetUser,
		"getUsers":                  h.handleGetUsers,
		"createUser":                h.handleCreateUser,
		"updateUser":                h.handleUpdateUser,
		"deleteUser":                h.handleDeleteUser,
		"changePassword":            h.handleChangePassword,
		"getShares":                 h.handleGetShares,
		"createShare":               h.handleCreateShare,
		"updateShare":               h.handleUpdateShare,
		"deleteShare":               h.handleDeleteShare,
	}
	// All Subsonic endpoints are authenticated; the group middleware applies it
	// once for the whole set.
	mux.Group(func(r chi.Router) {
		r.Use(h.authMiddleware)
		for name, fn := range endpoints {
			r.Handle("/rest/"+name, fn)
			r.Handle("/rest/"+name+".view", fn)
		}
	})
}

// maxFormBytes caps a request body parsed as form params, so an unbounded POST
// body can't exhaust memory.
const maxFormBytes = 1 << 20 // 1 MiB

// authMiddleware wraps a handler with Subsonic authentication, injecting the
// user into the context. On failure it answers a Subsonic error and stops.
func (h *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
		if err := r.ParseForm(); err != nil {
			writeError(w, r, ErrGeneric, "Invalid request")
			return
		}
		creds := core.Credentials{
			Username:  r.Form.Get("u"),
			Password:  r.Form.Get("p"),
			Token:     r.Form.Get("t"),
			Salt:      r.Form.Get("s"),
			APIToken:  httputil.APITokenFromRequest(r),
			RemoteIP:  httputil.ClientIP(r),
			UserAgent: r.UserAgent(),
		}
		user, err := h.Auth.Authenticate(r.Context(), creds)
		if err != nil {
			writeError(w, r, ErrWrongCredentials, "Wrong username or password")
			return
		}
		ctx := context.WithValue(r.Context(), userKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// userFrom returns the authenticated user from the request context.
func userFrom(ctx context.Context) models.User {
	u, _ := ctx.Value(userKey).(models.User)
	return u
}

// ---- parameter helpers ----

func param(r *http.Request, name string) string {
	return r.Form.Get(name)
}

func intParam(r *http.Request, name string, def int) int {
	v := r.Form.Get(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func boolParam(r *http.Request, name string, def bool) bool {
	v := r.Form.Get(name)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

// requireAdmin writes an error and returns false if the user is not an admin.
func requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !userFrom(r.Context()).IsAdmin {
		writeError(w, r, ErrUnauthorizedAction, "User is not authorized for this operation")
		return false
	}
	return true
}

// notFound is a convenience for translating repo ErrNotFound.
func isNotFound(err error) bool {
	return errors.Is(err, persistence.ErrNotFound)
}

// newID generates a unique identifier for new entities.
func newID() string { return uuid.NewString() }

// decodeEncParam decodes a Subsonic "enc:<hex>" encoded password value.
func decodeEncParam(p string) string {
	if raw, ok := strings.CutPrefix(p, "enc:"); ok {
		if dec, err := hex.DecodeString(raw); err == nil {
			return string(dec)
		}
	}
	return p
}
