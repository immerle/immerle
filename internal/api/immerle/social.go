package immerle

import (
	"context"
	"net/http"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/importer"
	"github.com/immerle/immerle/internal/models"
)

// handleCapabilities reports the immerle features this instance supports so
// clients can detect support before using extension endpoints.
//
// @Summary      Capability discovery
// @Description  Unauthenticated. Lets clients detect supported immerle extensions and whether first-run setup is still needed.
// @Tags         discovery
// @Produce      json
// @Success      200  {object}  CapabilitiesDTO
// @Router       /capabilities [get]
func (h *Handler) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	federation := false
	if h.Federation != nil {
		federation = h.Federation.Enabled()
	}
	wrapped := h.wrappedEnabled() && h.Wrapped != nil
	needsSetup := false
	if h.Setup != nil {
		if initialized, err := h.Setup.IsInitialized(r.Context()); err == nil {
			needsSetup = !initialized
		}
	}
	smart := h.smartEnabled() && h.SmartPlaylists != nil
	radio := h.radioEnabled() && h.Radio != nil
	offline := h.offlineEnabled()
	hallOfFame := h.hallOfFameEnabled()
	concerts := h.concertsEnabled()
	writeResource(w, http.StatusOK, map[string]any{
		"server":          "immerle",
		"protocolVersion": ProtocolVersion,
		"capabilities": map[string]any{
			"setup":                  map[string]any{"version": 1, "needed": needsSetup},
			"profiles":               map[string]any{"version": 1, "selfEditable": []string{"displayName", "email"}},
			"libraryStats":           map[string]any{"version": 1, "fields": []string{"artists", "albums", "tracks", "totalSize", "totalDuration"}},
			"libraryAdmin":           map[string]any{"version": 1, "admin": true, "actions": []string{"list", "editMetadata", "editCover", "delete"}},
			"activityFeed":           map[string]any{"version": 1, "privacy": []string{"public", "private"}},
			"collaborativePlaylists": map[string]any{"version": 1},
			"publicPlaylists":        map[string]any{"version": 1, "subscribe": true},
			"playlistImport":         map[string]any{"version": 1, "sources": importer.Available()},
			"shares":                 map[string]any{"version": 1},
			"jam":                    map[string]any{"version": 1, "transports": []string{"sse"}},
			"apiTokens":              map[string]any{"version": 1},
			"devices":                map[string]any{"version": 1, "auth": "jwt"},
			"theme":                  map[string]any{"version": 1, "properties": []string{"accentColor"}},
			"dynamicProviders":       map[string]any{"version": 1, "kinds": []string{"http"}, "admin": true},
			"runtimeSettings":        map[string]any{"version": 1, "admin": true, "groups": []string{"providers", "scan", "cleanup", "federation", "import", "smartPlaylists", "radio", "wrapped", "offline"}},
			"federation":             map[string]any{"version": 1, "enabled": federation},
			"smartPlaylists":         map[string]any{"version": 1, "admin": true, "enabled": smart},
			"internetRadio":          map[string]any{"version": 1, "admin": true, "enabled": radio},
			"wrapped":                map[string]any{"version": 1, "admin": true, "enabled": wrapped},
			"offlineDownloads":       map[string]any{"version": 1, "admin": true, "enabled": offline},
			"hallOfFame":             map[string]any{"version": 1, "admin": true, "enabled": hallOfFame},
			"concertDiscovery":       map[string]any{"version": 1, "admin": true, "enabled": concerts},
		},
	})
}

// handleActivity returns activity events visible to the caller, honoring each
// author's privacy setting.
//
// @Summary      Activity feed
// @Tags         activity
// @Security     BearerAuth
// @Produce      json
// @Success      200  {array}  ActivityEventDTO
// @Failure      401  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /activity [get]
func (h *Handler) handleActivity(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	events, err := h.Activity.Feed(r.Context(), user.ID, 50)
	if err != nil {
		writeInternal(w, err)
		return
	}
	out := make([]activityEventView, 0, len(events))
	for _, e := range events {
		out = append(out, activityEventView{ActivityEvent: e, Item: h.activityItem(r.Context(), e)})
	}
	writeResource(w, http.StatusOK, out)
}

// activityEventView is an activity event plus resolved, human-readable details
// about the item it references (title, artist, cover, …).
type activityEventView struct {
	models.ActivityEvent
	Item map[string]any `json:"item,omitempty"`
}

// activityItem resolves display details for the item an event references. Returns
// nil when the item can't be resolved (e.g. a remote favorite never downloaded,
// or a deleted item) — the event still carries its ids.
func (h *Handler) activityItem(ctx context.Context, e models.ActivityEvent) map[string]any {
	switch e.ItemType {
	case models.ItemTrack:
		if h.Catalog == nil {
			return nil
		}
		id := e.ItemID
		// A remote favorite/listen resolves to its downloaded local copy.
		if core.IsRemoteID(id) && h.OnDemand != nil {
			if local, ok := h.OnDemand.LocalTrackIDForRemote(ctx, id); ok {
				id = local
			}
		}
		t, err := h.Catalog.GetTrack(ctx, id)
		if err != nil {
			return nil
		}
		cover := t.CoverArt
		if cover == "" {
			cover = t.AlbumID
		}
		return map[string]any{
			"title":    t.Title,
			"artist":   t.ArtistName,
			"album":    t.AlbumName,
			"albumId":  t.AlbumID,
			"artistId": t.ArtistID,
			"coverArt": cover,
			"duration": t.Duration,
		}
	case models.ItemAlbum:
		if h.Catalog == nil {
			return nil
		}
		a, err := h.Catalog.GetAlbum(ctx, e.ItemID)
		if err != nil {
			return nil
		}
		cover := a.CoverArt
		if cover == "" {
			cover = a.ID
		}
		return map[string]any{"title": a.Name, "artist": a.ArtistName, "artistId": a.ArtistID, "coverArt": cover, "year": a.Year}
	case models.ItemArtist:
		if h.Catalog == nil {
			return nil
		}
		a, err := h.Catalog.GetArtist(ctx, e.ItemID)
		if err != nil {
			return nil
		}
		return map[string]any{"title": a.Name}
	case models.ItemPlaylist:
		p, err := h.Playlists.Get(ctx, e.ItemID)
		if err != nil {
			return nil
		}
		return map[string]any{"title": p.Name}
	}
	return nil
}

// resolveProfileUser resolves the {username} path param used by profile and
// profile-adjacent routes (its Hall of Fame) to the target user — "me" or the
// caller's own username resolve without a lookup.
func (h *Handler) resolveProfileUser(r *http.Request) (models.User, error) {
	caller := userFrom(r.Context())
	username := pathParam(r, "username")
	if username == "me" || username == caller.Username {
		return caller, nil
	}
	return h.Users.GetByUsername(r.Context(), username)
}

// handleProfile returns a user's public profile: identity, the activity visible
// to the caller, the user's public playlists, all-time listening stats, and
// (when non-empty) the top of their Hall of Fame. The path segment "me"
// resolves to the caller.
//
// @Summary      User profile
// @Description  Returns a user's profile — identity, recent activity visible to the caller (honoring privacy), their public playlists, all-time listening stats, and the top of their Hall of Fame (omitted when empty). Use "me" for the caller.
// @Tags         users
// @Security     BearerAuth
// @Produce      json
// @Param        username  path  string  true  "Target username, or 'me' for the caller"
// @Success      200  {object}  ProfileDTO
// @Failure      401  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /users/{username} [get]
func (h *Handler) handleProfile(w http.ResponseWriter, r *http.Request) {
	caller := userFrom(r.Context())
	target, err := h.resolveProfileUser(r)
	if err != nil {
		writeErrorParams(w, http.StatusNotFound, "not_found", "user not found", map[string]any{"username": pathParam(r, "username")})
		return
	}

	events, err := h.Activity.UserFeed(r.Context(), target.ID, 50)
	if err != nil {
		writeInternal(w, err)
		return
	}
	activity := make([]activityEventView, 0, len(events))
	for _, e := range events {
		activity = append(activity, activityEventView{ActivityEvent: e, Item: h.activityItem(r.Context(), e)})
	}

	lists, err := h.Playlists.ListPublicByOwner(r.Context(), target.ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	playlists := make([]map[string]any, 0, len(lists))
	for _, p := range lists {
		playlists = append(playlists, map[string]any{
			"id":        p.ID,
			"name":      p.Name,
			"comment":   p.Comment,
			"songCount": p.SongCount,
			"duration":  p.Duration,
			"coverArt":  p.CoverArt,
			"coverArts": p.CoverArts,
		})
	}

	isSelf := target.ID == caller.ID

	stats := ProfileStatsDTO{Playlists: len(playlists)}
	if h.Wrapped != nil {
		if plays, seconds, err := h.Wrapped.Totals(r.Context(), target.ID); err == nil {
			stats.Plays = plays
			stats.ListenSeconds = seconds
		}
	}

	resp := map[string]any{
		"user": map[string]any{
			"id":          target.ID,
			"username":    target.Username,
			"displayName": target.DisplayName,
			"isAdmin":     target.IsAdmin,
		},
		"isSelf":    isSelf,
		"activity":  activity,
		"playlists": playlists,
		"stats":     stats,
	}

	// The top of the target's Hall of Fame, omitted entirely when it's empty
	// (rather than shipping an empty section the profile page would hide anyway).
	if h.HallOfFame != nil && h.hallOfFameEnabled() {
		if d, err := h.hallOfFameSvc.Get(r.Context(), target.ID); err == nil && len(d.Entries) > 0 {
			top := d.Entries
			if len(top) > 3 {
				top = top[:3]
			}
			resp["hallOfFame"] = map[string]any{
				"top":   hallOfFameEntriesToSongViews(top),
				"total": len(d.Entries),
			}
		}
	}

	writeResource(w, http.StatusOK, resp)
}

// addCollaboratorBody is the body for POST /playlists/{id}/collaborators.
type addCollaboratorBody struct {
	Username string `json:"username"`
}

// handleAddCollaborator grants a user edit rights on a collaborative playlist.
//
// @Summary      Add a playlist collaborator
// @Description  Owner-only. Marks the playlist collaborative and grants edit rights to another user.
// @Tags         playlists
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id    path  string               true  "Playlist id"
// @Param        body  body  addCollaboratorBody  true  "User to grant edit rights"
// @Success      201  {object}  apiError  "added"
// @Failure      400  {object}  errorResponse
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /playlists/{id}/collaborators [post]
func (h *Handler) handleAddCollaborator(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	playlistID := pathParam(r, "id")
	var req addCollaboratorBody
	if !decodeJSON(w, r, &req) {
		return
	}

	p, err := h.Playlists.Get(r.Context(), playlistID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "playlist not found")
		return
	}
	if p.Federated || (p.OwnerID != user.ID && !user.IsAdmin) {
		writeError(w, http.StatusForbidden, "forbidden", "only the owner can add collaborators")
		return
	}
	collaborator, err := h.Users.GetByUsername(r.Context(), req.Username)
	if err != nil {
		writeErrorParams(w, http.StatusNotFound, "not_found", "user not found", map[string]any{"username": req.Username})
		return
	}
	if !p.Collaborative {
		p.Collaborative = true
		_ = h.Playlists.UpdateMeta(r.Context(), p)
	}
	if err := h.Playlists.AddCollaborator(r.Context(), playlistID, collaborator.ID); err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusCreated, map[string]any{"collaborator": collaborator.Username})
}
