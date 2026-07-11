package immerle

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/federation"
	"github.com/immerle/immerle/internal/models"
)

// This file exposes the personal playlist CRUD over the shared
// core.PlaylistService — the same logic (and view/edit permissions) the Subsonic
// playlist endpoints use. Public/collaborative extensions live in playlists.go.

// playlistView is the REST representation of a playlist. Tracks is populated on
// the single-playlist resource and on create/replace responses.
type playlistView struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Comment       string `json:"comment,omitempty"`
	Owner         string `json:"owner"`
	Public        bool   `json:"public"`
	Collaborative bool   `json:"collaborative"`
	// Federated marks a read-only playlist synced from the hub: its `owner` is
	// only an internal attribution, never real ownership — clients must not
	// offer edit/delete/cover controls for it, only subscribe/unsubscribe.
	Federated bool `json:"federated"`
	// Subscribed reports whether the caller has favorited this playlist (see
	// PUT/DELETE .../subscription). Only computed on the single-playlist
	// resource (handleGetPlaylist) — false elsewhere.
	Subscribed bool       `json:"subscribed"`
	SongCount  int        `json:"songCount"`
	Duration   int        `json:"duration"`
	CoverArt   string     `json:"coverArt,omitempty"`
	CoverArts  []string   `json:"coverArts,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	ChangedAt  time.Time  `json:"changedAt"`
	Tracks     []songView `json:"tracks,omitempty"`
}

func toPlaylistView(p models.Playlist, tracks []songView) playlistView {
	return playlistView{
		ID:            p.ID,
		Name:          p.Name,
		Comment:       p.Comment,
		Owner:         p.OwnerName,
		Public:        p.Public,
		Collaborative: p.Collaborative,
		Federated:     p.Federated,
		SongCount:     p.SongCount,
		Duration:      p.Duration,
		CoverArt:      p.CoverArt,
		CoverArts:     p.CoverArts,
		CreatedAt:     p.CreatedAt,
		ChangedAt:     p.UpdatedAt,
		Tracks:        tracks,
	}
}

func detailToView(d core.PlaylistDetail) playlistView {
	return toPlaylistView(d.Playlist, trackEntriesToSongViews(d.Tracks))
}

// handleListPlaylists lists the playlists visible to the caller.
//
// @Summary  List playlists
// @Description  Returns the playlists the caller owns, subscribes to or collaborates on.
// @Tags     playlists
// @Security BearerAuth
// @Produce  json
// @Success  200  {object}  map[string][]playlistView
// @Failure  401  {object}  errorResponse
// @Router   /playlists [get]
func (h *Handler) handleListPlaylists(w http.ResponseWriter, r *http.Request) {
	lists, err := h.playlistSvc.List(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	out := make([]playlistView, 0, len(lists))
	for _, p := range lists {
		out = append(out, toPlaylistView(p, nil))
	}
	writeResource(w, http.StatusOK, map[string]any{"playlists": out})
}

// handleGetPlaylist returns a playlist with its tracks.
//
// @Summary  Get playlist
// @Tags     playlists
// @Security BearerAuth
// @Produce  json
// @Param    id   path  string  true  "Playlist id"
// @Success  200  {object}  playlistView
// @Failure  401  {object}  errorResponse
// @Failure  403  {object}  errorResponse
// @Failure  404  {object}  errorResponse
// @Router   /playlists/{id} [get]
func (h *Handler) handleGetPlaylist(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	d, err := h.playlistSvc.Get(r.Context(), user, pathParam(r, "id"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	view := detailToView(d)
	view.Subscribed, _ = h.Playlists.IsSubscribed(r.Context(), d.Playlist.ID, user.ID)
	writeResource(w, http.StatusOK, view)
}

// playlistCreateRequest is the body for POST /playlists.
type playlistCreateRequest struct {
	Name string   `json:"name"`
	IDs  []string `json:"ids"`
}

// handleCreatePlaylist creates a playlist owned by the caller.
//
// @Summary  Create playlist
// @Tags     playlists
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    body  body  playlistCreateRequest  true  "Playlist"
// @Success  201  {object}  playlistView
// @Failure  400  {object}  errorResponse
// @Failure  401  {object}  errorResponse
// @Router   /playlists [post]
func (h *Handler) handleCreatePlaylist(w http.ResponseWriter, r *http.Request) {
	var req playlistCreateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeValidation(w, []fieldError{{Field: "name", Message: "name is required"}})
		return
	}
	d, err := h.playlistSvc.Create(r.Context(), userFrom(r.Context()), req.Name, req.IDs)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeResource(w, http.StatusCreated, detailToView(d))
}

// playlistUpdateRequest is the body for PATCH /playlists/{id}. nil fields are
// left unchanged.
type playlistUpdateRequest struct {
	Name          *string  `json:"name"`
	Comment       *string  `json:"comment"`
	Public        *bool    `json:"public"`
	AddIDs        []string `json:"addIds"`
	RemoveIndexes []int    `json:"removeIndexes"`
}

// handleUpdatePlaylist edits a playlist's metadata and tracks.
//
// @Summary  Update playlist
// @Description  Edits metadata and appends/removes tracks. Owner/admin/collaborator only.
// @Tags     playlists
// @Security BearerAuth
// @Accept   json
// @Param    id    path  string                 true  "Playlist id"
// @Param    body  body  playlistUpdateRequest  true  "Changes"
// @Success  204  "No Content"
// @Failure  401  {object}  errorResponse
// @Failure  403  {object}  errorResponse
// @Failure  404  {object}  errorResponse
// @Router   /playlists/{id} [patch]
func (h *Handler) handleUpdatePlaylist(w http.ResponseWriter, r *http.Request) {
	var req playlistUpdateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	meta := core.PlaylistMetaUpdate{Comment: req.Comment}
	if req.Name != nil {
		meta.Name = *req.Name
	}
	if req.Public != nil {
		s := strconv.FormatBool(*req.Public)
		meta.PublicRaw = &s
	}
	if err := h.playlistSvc.Update(r.Context(), userFrom(r.Context()), pathParam(r, "id"), meta, req.AddIDs, req.RemoveIndexes); err != nil {
		writeServiceError(w, err)
		return
	}
	writeResource(w, http.StatusNoContent, nil)
}

// playlistTracksRequest is the body for PUT /playlists/{id}/tracks.
type playlistTracksRequest struct {
	IDs []string `json:"ids"`
}

// handleReplacePlaylistTracks overwrites a playlist's tracklist.
//
// @Summary  Replace playlist tracks
// @Tags     playlists
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    id    path  string                 true  "Playlist id"
// @Param    body  body  playlistTracksRequest  true  "Track ids"
// @Success  200  {object}  playlistView
// @Failure  401  {object}  errorResponse
// @Failure  403  {object}  errorResponse
// @Failure  404  {object}  errorResponse
// @Router   /playlists/{id}/tracks [put]
func (h *Handler) handleReplacePlaylistTracks(w http.ResponseWriter, r *http.Request) {
	var req playlistTracksRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	d, err := h.playlistSvc.Replace(r.Context(), userFrom(r.Context()), pathParam(r, "id"), req.IDs)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeResource(w, http.StatusOK, detailToView(d))
}

// handleResolvePlaylistTrack resolves one federated-playlist entry (identified
// by its position) to a playable track, lazily, at the moment the caller wants
// to play it: a local catalog lookup first, then — if portable-id resolution
// is enabled — a provider search. The result may be a remote (not-yet-
// downloaded) track, streamed progressively like any other on-demand result.
//
// @Summary      Resolve a federated playlist track for playback
// @Description  Resolves an unresolved federated-playlist entry (checks the local catalog first, then the on-demand providers if enabled). 404 if it can't be resolved.
// @Tags         playlists
// @Security     BearerAuth
// @Produce      json
// @Param        id        path  string  true  "Playlist id"
// @Param        position  path  int     true  "Track position (0-based)"
// @Success      200  {object}  songView
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Router       /playlists/{id}/tracks/{position}/resolve [post]
func (h *Handler) handleResolvePlaylistTrack(w http.ResponseWriter, r *http.Request) {
	playlistID := pathParam(r, "id")
	if _, err := h.playlistSvc.Get(r.Context(), userFrom(r.Context()), playlistID); err != nil {
		writeServiceError(w, err)
		return
	}
	position, err := strconv.Atoi(pathParam(r, "position"))
	if err != nil || position < 0 {
		writeError(w, http.StatusBadRequest, "invalid_position", "invalid track position")
		return
	}
	if h.Federation == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "federation unavailable")
		return
	}
	track, err := h.Federation.ResolvePlaylistTrack(r.Context(), playlistID, position)
	if err != nil {
		if errors.Is(err, federation.ErrUnresolvable) {
			writeError(w, http.StatusNotFound, "unresolvable", "track could not be resolved")
			return
		}
		writeServiceError(w, err)
		return
	}
	writeResource(w, http.StatusOK, toSongView(track))
}

// handleDeletePlaylist deletes a playlist (owner/admin); a non-owner is
// unsubscribed instead.
//
// @Summary  Delete playlist
// @Tags     playlists
// @Security BearerAuth
// @Param    id   path  string  true  "Playlist id"
// @Success  204  "No Content"
// @Failure  401  {object}  errorResponse
// @Failure  403  {object}  errorResponse
// @Failure  404  {object}  errorResponse
// @Router   /playlists/{id} [delete]
func (h *Handler) handleDeletePlaylist(w http.ResponseWriter, r *http.Request) {
	if err := h.playlistSvc.Delete(r.Context(), userFrom(r.Context()), pathParam(r, "id")); err != nil {
		writeServiceError(w, err)
		return
	}
	writeResource(w, http.StatusNoContent, nil)
}
