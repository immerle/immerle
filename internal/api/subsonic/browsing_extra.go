package subsonic

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/immerle/immerle/internal/lyrics"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// toAlbumChild renders an album as a directory-style Child (used by file-based
// browsing and the v1 list/search endpoints).
func toAlbumChild(a models.Album, ann *models.Annotation) Child {
	c := Child{
		ID:       a.ID,
		Parent:   a.ArtistID,
		IsDir:    true,
		Title:    a.Name,
		Album:    a.Name,
		Artist:   a.ArtistName,
		CoverArt: coverIDForAlbum(a),
		Year:     a.Year,
		Genre:    a.Genre,
		Created:  formatTime(a.CreatedAt),
		AlbumID:  a.ID,
		ArtistID: a.ArtistID,
		Duration: a.Duration,
	}
	if ann != nil {
		c.Starred = starredStr(ann)
		c.UserRating = ann.Rating
	}
	return c
}

func (h *Handler) handleGetSongsByGenre(w http.ResponseWriter, r *http.Request) {
	genre := param(r, "genre")
	if genre == "" {
		writeError(w, r, ErrMissingParameter, "Required parameter genre is missing")
		return
	}
	user := userFrom(r.Context())
	tracks, err := h.library.SongsByGenre(r.Context(), user.ID, genre, intParam(r, "count", 10), intParam(r, "offset", 0))
	if err != nil {
		h.failInternal(w, r, err)
		return
	}
	resp := newResponse()
	resp.SongsByGenre = &Songs{Song: trackEntriesToChildren(tracks)}
	write(w, r, resp)
}

func (h *Handler) handleGetRandomSongs(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	tracks, err := h.library.RandomSongs(r.Context(), user.ID, intParam(r, "size", 10), param(r, "genre"), intParam(r, "fromYear", 0), intParam(r, "toYear", 0))
	if err != nil {
		h.failInternal(w, r, err)
		return
	}
	resp := newResponse()
	resp.RandomSongs = &Songs{Song: trackEntriesToChildren(tracks)}
	write(w, r, resp)
}

func (h *Handler) handleGetAlbumList(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	albums, err := h.library.AlbumList(r.Context(), buildAlbumListOptions(r, user.ID))
	if err != nil {
		h.failInternal(w, r, err)
		return
	}
	list := make([]Child, 0, len(albums))
	for _, e := range albums {
		list = append(list, toAlbumChild(e.Album, e.Annotation))
	}
	resp := newResponse()
	resp.AlbumList = &AlbumList{Album: list}
	write(w, r, resp)
}

func (h *Handler) handleGetMusicDirectory(w http.ResponseWriter, r *http.Request) {
	id := param(r, "id")
	if id == "" {
		writeError(w, r, ErrMissingParameter, "Required parameter id is missing")
		return
	}
	// Remote (provider) artist/album directories.
	if h.OnDemand != nil && h.remoteMusicDirectory(w, r, id) {
		return
	}
	user := userFrom(r.Context())
	ctx := r.Context()

	// Artist directory → albums as sub-directories.
	if artist, err := h.Catalog.GetArtist(ctx, id); err == nil {
		albums, _ := h.Catalog.ListAlbumsByArtist(ctx, id)
		albumAnn, _ := h.Annotations.AnnotationMap(ctx, user.ID, models.ItemAlbum)
		children := make([]Child, 0, len(albums))
		for _, a := range albums {
			children = append(children, toAlbumChild(a, annPtr(albumAnn, a.ID)))
		}
		resp := newResponse()
		resp.Directory = &Directory{ID: artist.ID, Name: artist.Name, Child: children}
		write(w, r, resp)
		return
	}

	// Album directory → songs (local + provider enrichment, same as getAlbum).
	if res, err := h.library.GetAlbum(ctx, user.ID, id); err == nil {
		resp := newResponse()
		resp.Directory = &Directory{
			ID:     res.Album.ID,
			Name:   res.Album.Name,
			Parent: res.Album.ArtistID,
			Child:  trackEntriesToChildren(res.Tracks),
		}
		write(w, r, resp)
		return
	}

	writeError(w, r, ErrDataNotFound, "Directory not found")
}

func (h *Handler) handleGetStarred(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	st := h.library.Starred(r.Context(), user.ID)
	out := &Starred{}
	for _, a := range st.Artists {
		out.Artist = append(out.Artist, ArtistItem{ID: a.ID, Name: a.Name})
	}
	for _, a := range st.Albums {
		out.Album = append(out.Album, toAlbumChild(a, nil))
	}
	for _, t := range st.Songs {
		out.Song = append(out.Song, toChild(t, nil))
	}
	resp := newResponse()
	resp.Starred = out
	write(w, r, resp)
}

// handleSearch2 is the file-based (non-ID3) twin of handleSearch3: same merged
// local+remote search via the library service, rendered as directory-style
// children. (Album annotations are intentionally not surfaced here, matching the
// historical search2 shape.)
func (h *Handler) handleSearch2(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	res, err := h.library.Search(r.Context(), user.ID, param(r, "query"),
		intParam(r, "artistCount", 20), intParam(r, "albumCount", 20), intParam(r, "songCount", 20), 0)
	if err != nil {
		h.failInternal(w, r, err)
		return
	}

	out := &SearchResult2{}
	for _, a := range res.Artists {
		out.Artist = append(out.Artist, ArtistItem{ID: a.ID, Name: a.Name})
	}
	for _, a := range res.Albums {
		out.Album = append(out.Album, toAlbumChild(a, nil))
	}
	for _, t := range res.Tracks {
		out.Song = append(out.Song, toChild(t, annPtr(res.TrackAnnotations, t.ID)))
	}

	resp := newResponse()
	resp.SearchResult2 = out
	write(w, r, resp)
}

func (h *Handler) handleGetTopSongs(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	tracks := h.library.TopSongs(r.Context(), user.ID, param(r, "artist"), intParam(r, "count", 50))
	resp := newResponse()
	resp.TopSongs = &Songs{Song: trackEntriesToChildren(tracks)}
	write(w, r, resp)
}

func (h *Handler) handleGetSimilarSongs(w http.ResponseWriter, r *http.Request) {
	songs := h.similarSongs(r)
	resp := newResponse()
	resp.SimilarSongs = &SimilarSongs{Song: songs}
	write(w, r, resp)
}

func (h *Handler) handleGetSimilarSongs2(w http.ResponseWriter, r *http.Request) {
	songs := h.similarSongs(r)
	resp := newResponse()
	resp.SimilarSongs2 = &SimilarSongs2{Song: songs}
	write(w, r, resp)
}

func (h *Handler) similarSongs(r *http.Request) []Child {
	user := userFrom(r.Context())
	tracks := h.library.SimilarSongs(r.Context(), user.ID, param(r, "id"), intParam(r, "count", 50))
	return trackEntriesToChildren(tracks)
}

func (h *Handler) handleGetArtistInfo(w http.ResponseWriter, r *http.Request) {
	resp := newResponse()
	out := &ArtistInfo{}
	if a, err := h.Catalog.GetArtist(r.Context(), param(r, "id")); err == nil {
		out.ArtistInfoBase = h.artistInfoBase(r, a)
	}
	resp.ArtistInfo = out
	write(w, r, resp)
}

func (h *Handler) handleGetArtistInfo2(w http.ResponseWriter, r *http.Request) {
	resp := newResponse()
	out := &ArtistInfo2{}
	if a, err := h.Catalog.GetArtist(r.Context(), param(r, "id")); err == nil {
		out.ArtistInfoBase = h.artistInfoBase(r, a)
	}
	resp.ArtistInfo2 = out
	write(w, r, resp)
}

// artistInfoBase fills MBID and avatar image URLs (when an avatar was fetched by
// the enrichment service) for getArtistInfo/2.
func (h *Handler) artistInfoBase(r *http.Request, a models.Artist) ArtistInfoBase {
	base := ArtistInfoBase{MusicBrainzID: a.MBID}
	if a.CoverArt != "" {
		base.SmallImageURL = h.coverURL(r, a.CoverArt, 160)
		base.MediumImageURL = h.coverURL(r, a.CoverArt, 320)
		base.LargeImageURL = h.coverURL(r, a.CoverArt, 0)
	}
	return base
}

// coverURL builds an absolute getCoverArt URL, forwarding the caller's auth
// params so the returned link is directly usable. The cleartext password ("p")
// is deliberately omitted: these URLs end up cached/logged/proxied client-side,
// so token+salt ("t"/"s") auth is forwarded instead. Password-only callers get
// a link that still requires them to re-supply credentials.
func (h *Handler) coverURL(r *http.Request, coverArt string, size int) string {
	if coverArt == "" {
		return ""
	}
	q := url.Values{}
	for _, k := range []string{"u", "t", "s", "c", "v"} {
		if v := r.Form.Get(k); v != "" {
			q.Set(k, v)
		}
	}
	q.Set("id", coverArt)
	if size > 0 {
		q.Set("size", strconv.Itoa(size))
	}
	return strings.TrimRight(h.BaseURL, "/") + "/rest/getCoverArt?" + q.Encode()
}

func (h *Handler) handleGetAlbumInfo(w http.ResponseWriter, r *http.Request) {
	resp := newResponse()
	out := &AlbumInfo{}
	if a, err := h.Catalog.GetAlbum(r.Context(), param(r, "id")); err == nil {
		out.MusicBrainzID = a.MBID
	}
	resp.AlbumInfo = out
	write(w, r, resp)
}

func (h *Handler) handleGetLyrics(w http.ResponseWriter, r *http.Request) {
	// Legacy getLyrics keys off artist+title, which we do not index. ponytail:
	// wire a title lookup here if a client actually needs the legacy endpoint;
	// modern clients use getLyricsBySongId below.
	resp := newResponse()
	resp.Lyrics = &Lyrics{Artist: param(r, "artist"), Title: param(r, "title")}
	write(w, r, resp)
}

func (h *Handler) handleGetLyricsBySongID(w http.ResponseWriter, r *http.Request) {
	resp := newResponse()
	out := &LyricsList{}
	if t, err := h.Catalog.GetTrack(r.Context(), param(r, "id")); err == nil && t.Lyrics != "" {
		out.StructuredLyrics = []StructuredLyrics{parseStructuredLyrics(t.Lyrics)}
	}
	resp.LyricsList = out
	write(w, r, resp)
}

// parseStructuredLyrics maps a parsed lyrics document (shared with the native
// REST API) into the OpenSubsonic shape.
func parseStructuredLyrics(raw string) StructuredLyrics {
	doc := lyrics.Parse(raw)
	out := StructuredLyrics{Lang: "xxx", Synced: doc.Synced}
	for _, l := range doc.Lines {
		line := LyricLine{Value: l.Text}
		if doc.Synced {
			line.Start = l.StartMs
		}
		out.Line = append(out.Line, line)
	}
	return out
}

// ---- empty-but-valid stubs for sections we do not implement ----

func (h *Handler) handleGetVideos(w http.ResponseWriter, r *http.Request) {
	resp := newResponse()
	resp.Videos = &Videos{}
	write(w, r, resp)
}

func (h *Handler) handleGetBookmarks(w http.ResponseWriter, r *http.Request) {
	resp := newResponse()
	resp.Bookmarks = &Bookmarks{}
	write(w, r, resp)
}

// radioEnabled reports whether the internet-radio feature is on (default on
// when the toggle is unavailable, e.g. in tests).
func (h *Handler) radioEnabled() bool {
	return h.Settings == nil || h.Settings.RadioEnabled()
}

func (h *Handler) handleGetInternetRadioStations(w http.ResponseWriter, r *http.Request) {
	resp := newResponse()
	resp.InternetRadioStations = &InternetRadioStations{}
	if h.radioEnabled() && h.Radio != nil {
		stations, err := h.Radio.List(r.Context())
		if err != nil {
			h.failInternal(w, r, err)
			return
		}
		for _, s := range stations {
			resp.InternetRadioStations.InternetRadioStation = append(resp.InternetRadioStations.InternetRadioStation, InternetRadioStation{
				ID: s.ID, Name: s.Name, StreamURL: s.StreamURL, HomePageURL: s.HomepageURL,
			})
		}
	}
	write(w, r, resp)
}

// handleCreateInternetRadioStation adds a station (admin only).
func (h *Handler) handleCreateInternetRadioStation(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	if h.Radio == nil {
		writeError(w, r, ErrGeneric, "radio not available")
		return
	}
	streamURL := param(r, "streamUrl")
	name := param(r, "name")
	if streamURL == "" || name == "" {
		writeError(w, r, ErrMissingParameter, "streamUrl and name are required")
		return
	}
	now := time.Now()
	st := models.RadioStation{
		ID: persistence.NewStationID(), Name: name, StreamURL: streamURL,
		HomepageURL: param(r, "homepageUrl"), CreatedAt: now, UpdatedAt: now,
	}
	if err := h.Radio.Create(r.Context(), st); err != nil {
		h.failInternal(w, r, err)
		return
	}
	writeOK(w, r)
}

// handleUpdateInternetRadioStation edits a station (admin only).
func (h *Handler) handleUpdateInternetRadioStation(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	if h.Radio == nil {
		writeError(w, r, ErrGeneric, "radio not available")
		return
	}
	id := param(r, "id")
	st, err := h.Radio.Get(r.Context(), id)
	if err != nil {
		writeError(w, r, ErrDataNotFound, "Station not found")
		return
	}
	if v := param(r, "name"); v != "" {
		st.Name = v
	}
	if v := param(r, "streamUrl"); v != "" {
		st.StreamURL = v
	}
	st.HomepageURL = param(r, "homepageUrl")
	st.UpdatedAt = time.Now()
	if err := h.Radio.Update(r.Context(), st); err != nil {
		h.failInternal(w, r, err)
		return
	}
	writeOK(w, r)
}

// handleDeleteInternetRadioStation removes a custom station (admin only;
// built-ins can't be deleted).
func (h *Handler) handleDeleteInternetRadioStation(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	if h.Radio == nil {
		writeError(w, r, ErrGeneric, "radio not available")
		return
	}
	if err := h.Radio.Delete(r.Context(), param(r, "id")); err != nil {
		h.failInternal(w, r, err)
		return
	}
	writeOK(w, r)
}

func (h *Handler) handleGetChatMessages(w http.ResponseWriter, r *http.Request) {
	resp := newResponse()
	resp.ChatMessages = &ChatMessages{}
	write(w, r, resp)
}
