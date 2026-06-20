package immerle

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/importer"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
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
	writeResource(w, http.StatusOK, map[string]any{
		"server":          "immerle",
		"protocolVersion": ProtocolVersion,
		"capabilities": map[string]any{
			"setup":                  map[string]any{"version": 1, "needed": needsSetup},
			"friendships":            map[string]any{"version": 1},
			"profiles":               map[string]any{"version": 1, "selfEditable": []string{"displayName", "email"}},
			"libraryStats":           map[string]any{"version": 1, "fields": []string{"artists", "albums", "tracks", "totalSize", "totalDuration"}},
			"libraryAdmin":           map[string]any{"version": 1, "admin": true, "actions": []string{"list", "editMetadata", "editCover", "delete"}},
			"activityFeed":           map[string]any{"version": 1, "privacy": []string{"public", "friends", "private"}},
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
		},
	})
}

// handleFriends returns the caller's accepted friends.
//
// @Summary      List friends
// @Tags         friends
// @Security     BearerAuth
// @Produce      json
// @Success      200  {array}  FriendDTO
// @Failure      401  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /friends [get]
func (h *Handler) handleFriends(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	ids, err := h.Friends.ListFriends(r.Context(), user.ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	friends := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		u, err := h.Users.GetByID(r.Context(), id)
		if err != nil {
			continue
		}
		friends = append(friends, map[string]any{"id": u.ID, "username": u.Username, "displayName": u.DisplayName})
	}
	writeResource(w, http.StatusOK, friends)
}

// friendRequestBody is the body for POST /friends/requests.
type friendRequestBody struct {
	Username string `json:"username"`
}

// handleFriendRequest sends a friend request to another user.
//
// @Summary      Send a friend request
// @Tags         friends
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  friendRequestBody  true  "Target username"
// @Success      201  {object}  apiError  "created"
// @Failure      400  {object}  errorResponse
// @Failure      401  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /friends/requests [post]
func (h *Handler) handleFriendRequest(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	var req friendRequestBody
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Username == "" {
		writeError(w, http.StatusBadRequest, "validation", "username required")
		return
	}
	friend, err := h.Users.GetByUsername(r.Context(), req.Username)
	if err != nil {
		writeErrorParams(w, http.StatusNotFound, "not_found", "user not found", map[string]any{"username": req.Username})
		return
	}
	if friend.ID == user.ID {
		writeError(w, http.StatusBadRequest, "validation", "cannot befriend yourself")
		return
	}
	now := time.Now()
	err = h.Friends.Request(r.Context(), models.Friendship{
		ID:        uuid.NewString(),
		UserID:    user.ID,
		FriendID:  friend.ID,
		Status:    models.FriendPending,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusCreated, map[string]any{"requested": friend.Username})
}

// handleFriendAccept accepts an incoming friend request from {username}.
//
// @Summary      Accept a friend request
// @Tags         friends
// @Security     BearerAuth
// @Produce      json
// @Param        username  path  string  true  "Username of the requester to accept"
// @Success      200  {object}  FriendDTO
// @Failure      401  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /friends/requests/{username}/accept [post]
func (h *Handler) handleFriendAccept(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	requesterUser, err := h.Users.GetByUsername(r.Context(), pathParam(r, "username"))
	if err != nil {
		writeErrorParams(w, http.StatusNotFound, "not_found", "user not found", map[string]any{"username": pathParam(r, "username")})
		return
	}
	if err := h.Friends.Accept(r.Context(), requesterUser.ID, user.ID, uuid.NewString()); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			writeErrorParams(w, http.StatusNotFound, "not_found", "no pending request from this user", map[string]any{"username": requesterUser.Username})
			return
		}
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusOK, map[string]any{
		"id":          requesterUser.ID,
		"username":    requesterUser.Username,
		"displayName": requesterUser.DisplayName,
	})
}

// handleFriendPending lists the caller's incoming friend requests.
//
// @Summary      List pending friend requests
// @Tags         friends
// @Security     BearerAuth
// @Produce      json
// @Success      200  {array}  PendingFriendDTO
// @Failure      401  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /friends/requests [get]
func (h *Handler) handleFriendPending(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	pending, err := h.Friends.ListPending(r.Context(), user.ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	out := make([]map[string]any, 0, len(pending))
	for _, f := range pending {
		u, err := h.Users.GetByID(r.Context(), f.UserID)
		if err != nil {
			continue
		}
		out = append(out, map[string]any{"id": u.ID, "username": u.Username, "displayName": u.DisplayName, "since": f.CreatedAt})
	}
	writeResource(w, http.StatusOK, out)
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

// handleProfile returns a user's public profile: identity, the activity visible
// to the caller, and the user's public playlists. The path segment "me" resolves
// to the caller.
//
// @Summary      User profile
// @Description  Returns a user's profile — identity, recent activity visible to the caller (honoring privacy), and their public playlists. Use "me" for the caller.
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
	username := pathParam(r, "username")
	target := caller
	if username != "me" && username != caller.Username {
		u, err := h.Users.GetByUsername(r.Context(), username)
		if err != nil {
			writeErrorParams(w, http.StatusNotFound, "not_found", "user not found", map[string]any{"username": username})
			return
		}
		target = u
	}

	// Activity visible to the caller, enriched with item details.
	events, err := h.Activity.UserFeed(r.Context(), caller.ID, target.ID, 50)
	if err != nil {
		writeInternal(w, err)
		return
	}
	activity := make([]activityEventView, 0, len(events))
	for _, e := range events {
		activity = append(activity, activityEventView{ActivityEvent: e, Item: h.activityItem(r.Context(), e)})
	}

	// The target's public playlists.
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
			"coverArts": p.CoverArts,
		})
	}

	isSelf := target.ID == caller.ID
	isFriend := false
	if !isSelf {
		isFriend, _ = h.Friends.AreFriends(r.Context(), caller.ID, target.ID)
	}

	writeResource(w, http.StatusOK, map[string]any{
		"user": map[string]any{
			"id":          target.ID,
			"username":    target.Username,
			"displayName": target.DisplayName,
			"isAdmin":     target.IsAdmin,
		},
		"isSelf":    isSelf,
		"isFriend":  isFriend,
		"activity":  activity,
		"playlists": playlists,
	})
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
	if p.OwnerID != user.ID && !user.IsAdmin {
		writeError(w, http.StatusForbidden, "forbidden", "only the owner can add collaborators")
		return
	}
	collaborator, err := h.Users.GetByUsername(r.Context(), req.Username)
	if err != nil {
		writeErrorParams(w, http.StatusNotFound, "not_found", "user not found", map[string]any{"username": req.Username})
		return
	}
	// Ensure the playlist is marked collaborative.
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
