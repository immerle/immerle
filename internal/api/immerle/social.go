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
// @Success      200  {object}  CapabilitiesResponse
// @Router       /capabilities [get]
func (h *Handler) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	federation := false
	if h.Federation != nil {
		federation = h.Federation.Enabled()
	}
	needsSetup := false
	if h.Setup != nil {
		if initialized, err := h.Setup.IsInitialized(r.Context()); err == nil {
			needsSetup = !initialized
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"server":          "immerle",
		"protocolVersion": ProtocolVersion,
		"capabilities": map[string]any{
			"setup":                  map[string]any{"version": 1, "needed": needsSetup},
			"friendships":            map[string]any{"version": 1},
			"profiles":               map[string]any{"version": 1, "selfEditable": []string{"displayName", "email"}},
			"libraryStats":           map[string]any{"version": 1, "fields": []string{"artists", "albums", "tracks", "totalSize", "totalDuration"}},
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
			"runtimeSettings":        map[string]any{"version": 1, "admin": true, "groups": []string{"providers", "scan", "cleanup", "federation", "import"}},
			"federation":             map[string]any{"version": 1, "enabled": federation},
		},
	})
}

// @Summary      List friends
// @Description  Returns the caller's accepted friends.
// @Tags         friends
// @Produce      json
// @Param        u  query  string  true   "Subsonic username"
// @Param        p  query  string  false  "Subsonic password (or use t+s token auth)"
// @Param        c  query  string  true   "Client name"
// @Success      200  {object}  FriendsResponse
// @Failure      401  {object}  ErrorResponse
// @Router       /friends [get]
func (h *Handler) handleFriends(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	ids, err := h.Friends.ListFriends(r.Context(), user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
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
	writeJSON(w, http.StatusOK, okBody(map[string]any{"friends": friends}))
}

// @Summary      Send a friend request
// @Tags         friends
// @Produce      json
// @Param        u         query  string  true   "Subsonic username"
// @Param        p         query  string  false  "Subsonic password (or t+s token auth)"
// @Param        c         query  string  true   "Client name"
// @Param        username  query  string  true   "Target username to befriend"
// @Success      200  {object}  OKResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /friends/request [post]
func (h *Handler) handleFriendRequest(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	target := r.Form.Get("username")
	if target == "" {
		writeJSON(w, http.StatusBadRequest, errorBody("username required"))
		return
	}
	friend, err := h.Users.GetByUsername(r.Context(), target)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorBody("user not found"))
		return
	}
	if friend.ID == user.ID {
		writeJSON(w, http.StatusBadRequest, errorBody("cannot befriend yourself"))
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
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, okBody(map[string]any{"requested": friend.Username}))
}

// @Summary      Accept a friend request
// @Tags         friends
// @Produce      json
// @Param        u         query  string  true   "Subsonic username"
// @Param        p         query  string  false  "Subsonic password (or t+s token auth)"
// @Param        c         query  string  true   "Client name"
// @Param        username  query  string  true   "Username of the requester to accept"
// @Success      200  {object}  OKResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /friends/accept [post]
func (h *Handler) handleFriendAccept(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	requester := r.Form.Get("username")
	requesterUser, err := h.Users.GetByUsername(r.Context(), requester)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorBody("user not found"))
		return
	}
	if err := h.Friends.Accept(r.Context(), requesterUser.ID, user.ID, uuid.NewString()); err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, errorBody("no pending request from this user"))
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, okBody(map[string]any{"accepted": requesterUser.Username}))
}

// @Summary      List pending friend requests
// @Tags         friends
// @Produce      json
// @Param        u  query  string  true   "Subsonic username"
// @Param        p  query  string  false  "Subsonic password (or t+s token auth)"
// @Param        c  query  string  true   "Client name"
// @Success      200  {object}  PendingFriendsResponse
// @Router       /friends/pending [get]
func (h *Handler) handleFriendPending(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	pending, err := h.Friends.ListPending(r.Context(), user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
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
	writeJSON(w, http.StatusOK, okBody(map[string]any{"pending": out}))
}

// @Summary      Activity feed
// @Description  Returns activity events visible to the caller, honoring each author's privacy setting.
// @Tags         activity
// @Produce      json
// @Param        u  query  string  true   "Subsonic username"
// @Param        p  query  string  false  "Subsonic password (or t+s token auth)"
// @Param        c  query  string  true   "Client name"
// @Success      200  {object}  ActivityResponse
// @Router       /activity [get]
func (h *Handler) handleActivity(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	events, err := h.Activity.Feed(r.Context(), user.ID, 50)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}
	out := make([]activityEventView, 0, len(events))
	for _, e := range events {
		out = append(out, activityEventView{ActivityEvent: e, Item: h.activityItem(r.Context(), e)})
	}
	writeJSON(w, http.StatusOK, okBody(map[string]any{"events": out}))
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

// handleProfile returns a user's public profile: identity (username, display
// name), their activity (as visible to the caller) and their public playlists.
//
// @Summary      User profile
// @Description  Returns a user's profile — identity, recent activity visible to the caller (honoring privacy), and their public playlists. Defaults to the caller when username is omitted.
// @Tags         users
// @Produce      json
// @Param        u         query  string  true   "Subsonic username"
// @Param        p         query  string  false  "Subsonic password (or t+s token auth)"
// @Param        c         query  string  true   "Client name"
// @Param        username  query  string  false  "Target username (defaults to the caller)"
// @Success      200  {object}  ProfileResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /profile [get]
func (h *Handler) handleProfile(w http.ResponseWriter, r *http.Request) {
	caller := userFrom(r.Context())
	username := r.Form.Get("username")
	target := caller
	if username != "" && username != caller.Username {
		u, err := h.Users.GetByUsername(r.Context(), username)
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorBody("user not found"))
			return
		}
		target = u
	}

	// Activity visible to the caller, enriched with item details.
	events, err := h.Activity.UserFeed(r.Context(), caller.ID, target.ID, 50)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}
	activity := make([]activityEventView, 0, len(events))
	for _, e := range events {
		activity = append(activity, activityEventView{ActivityEvent: e, Item: h.activityItem(r.Context(), e)})
	}

	// The target's public playlists.
	lists, err := h.Playlists.ListPublicByOwner(r.Context(), target.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
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

	writeJSON(w, http.StatusOK, okBody(map[string]any{
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
	}))
}

// handleAddCollaborator grants a user edit rights on a collaborative playlist.
//
// @Summary      Add a playlist collaborator
// @Description  Owner-only. Marks the playlist collaborative and grants edit rights to another user.
// @Tags         playlists
// @Produce      json
// @Param        u           query  string  true   "Subsonic username"
// @Param        p           query  string  false  "Subsonic password (or t+s token auth)"
// @Param        c           query  string  true   "Client name"
// @Param        playlistId  query  string  true   "Playlist id"
// @Param        username    query  string  true   "User to grant edit rights"
// @Success      200  {object}  OKResponse
// @Failure      403  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /playlists/collaborators [post]
func (h *Handler) handleAddCollaborator(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	playlistID := r.Form.Get("playlistId")
	username := r.Form.Get("username")

	p, err := h.Playlists.Get(r.Context(), playlistID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorBody("playlist not found"))
		return
	}
	if p.OwnerID != user.ID && !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, errorBody("only the owner can add collaborators"))
		return
	}
	collaborator, err := h.Users.GetByUsername(r.Context(), username)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorBody("user not found"))
		return
	}
	// Ensure the playlist is marked collaborative.
	if !p.Collaborative {
		p.Collaborative = true
		_ = h.Playlists.UpdateMeta(r.Context(), p)
	}
	if err := h.Playlists.AddCollaborator(r.Context(), playlistID, collaborator.ID); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, okBody(map[string]any{"collaborator": collaborator.Username}))
}
