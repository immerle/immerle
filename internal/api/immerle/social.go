package immerle

import (
	"context"
	"net/http"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/services/importer"
)

// handleCapabilities reports the immerle features this instance supports so
// clients can detect support before using extension endpoints.
//
// @Summary      Capability discovery
// @Description  Unauthenticated. Lets clients detect supported immerle extensions and whether first-run setup is still needed.
// @Tags         discovery
// @Produce      json
// @Success      200  {object}  capabilitiesView
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
	bandcampImport := h.Purchases != nil
	lastFm := h.Settings != nil && h.Settings.Get().LastFm.Enabled
	sonicAnalysis := h.Sonic.Enabled()
	sonicRadio := h.Sonic.RadioEnabled()
	sonicAutoplay := h.Sonic.AutoplayEnabled()
	sonicMoods := h.Sonic.MoodsEnabled()
	sonicAutoDJ := h.Sonic.AutoDJEnabled()
	sonicDuplicates := h.Sonic.DuplicatesEnabled()
	writeResource(w, http.StatusOK, capabilitiesView{
		Server:          "immerle",
		ProtocolVersion: ProtocolVersion,
		Capabilities: map[string]capabilityView{
			"setup":                  {Version: 1, Needed: flag(needsSetup)},
			"profiles":               {Version: 1, SelfEditable: []string{"displayName", "email"}},
			"libraryStats":           {Version: 1, Fields: []string{"artists", "albums", "tracks", "totalSize", "totalDuration"}},
			"libraryAdmin":           {Version: 1, Admin: true, Actions: []string{"list", "editMetadata", "editCover", "delete"}},
			"activityFeed":           {Version: 1, Privacy: []string{"public", "private"}},
			"collaborativePlaylists": {Version: 1},
			"publicPlaylists":        {Version: 1, Subscribe: true},
			"playlistImport":         {Version: 1, Sources: importer.Available()},
			"shares":                 {Version: 1},
			"jam":                    {Version: 1, Transports: []string{"sse"}},
			"apiTokens":              {Version: 1},
			"devices":                {Version: 1, Auth: "jwt"},
			"theme":                  {Version: 1, Properties: []string{"accentColor"}},
			"dynamicProviders":       {Version: 1, Kinds: []string{"http"}, Admin: true},
			"runtimeSettings":        {Version: 1, Admin: true, Groups: []string{"providers", "scan", "cleanup", "federation", "import", "smartPlaylists", "radio", "wrapped", "offline", "sonic"}},
			"federation":             {Version: 1, Enabled: flag(federation)},
			"smartPlaylists":         {Version: 1, Admin: true, Enabled: flag(smart)},
			"internetRadio":          {Version: 1, Admin: true, Enabled: flag(radio)},
			"wrapped":                {Version: 1, Admin: true, Enabled: flag(wrapped)},
			"offlineDownloads":       {Version: 1, Admin: true, Enabled: flag(offline)},
			"hallOfFame":             {Version: 1, Admin: true, Enabled: flag(hallOfFame)},
			"concertDiscovery":       {Version: 1, Admin: true, Enabled: flag(concerts)},
			"bandcampImport":         {Version: 1, Enabled: flag(bandcampImport)},
			"lastFm":                 {Version: 1, Admin: true, Enabled: flag(lastFm)},
			"sonicAnalysis":          {Version: 1, Admin: true, Enabled: flag(sonicAnalysis)},
			"sonicRadio":             {Version: 1, Admin: true, Enabled: flag(sonicRadio)},
			"sonicAutoplay":          {Version: 1, Admin: true, Enabled: flag(sonicAutoplay)},
			"sonicMoods":             {Version: 1, Admin: true, Enabled: flag(sonicMoods)},
			"sonicAutoDj":            {Version: 1, Admin: true, Enabled: flag(sonicAutoDJ)},
			"sonicDuplicates":        {Version: 1, Admin: true, Enabled: flag(sonicDuplicates)},
		},
	})
}

// capabilitiesView is the capability-discovery payload. Clients mask every
// feature the server does not advertise.
type capabilitiesView struct {
	Server          string                    `json:"server"`
	ProtocolVersion string                    `json:"protocolVersion"`
	Capabilities    map[string]capabilityView `json:"capabilities" validate:"required"`
}

// capabilityView is one advertised feature: every capability has a version,
// the other fields are set only by the features they describe.
type capabilityView struct {
	Version int `json:"version"`
	// Enabled is the live on/off state of an admin-togglable feature (or of a
	// configured integration); absent for the always-on ones.
	Enabled *bool `json:"enabled,omitempty" validate:"optional"`
	// Admin marks a feature with admin-only controls.
	Admin bool `json:"admin,omitempty" validate:"optional"`
	// Needed (setup) is true until the first administrator is provisioned.
	Needed       *bool    `json:"needed,omitempty" validate:"optional"`
	Subscribe    bool     `json:"subscribe,omitempty" validate:"optional"`
	Auth         string   `json:"auth,omitempty" validate:"optional"`
	SelfEditable []string `json:"selfEditable,omitempty" validate:"optional"`
	Fields       []string `json:"fields,omitempty" validate:"optional"`
	Actions      []string `json:"actions,omitempty" validate:"optional"`
	Privacy      []string `json:"privacy,omitempty" validate:"optional"`
	Sources      []string `json:"sources,omitempty" validate:"optional"`
	Transports   []string `json:"transports,omitempty" validate:"optional"`
	Properties   []string `json:"properties,omitempty" validate:"optional"`
	Kinds        []string `json:"kinds,omitempty" validate:"optional"`
	Groups       []string `json:"groups,omitempty" validate:"optional"`
}

func flag(b bool) *bool { return &b }

// handleActivity returns activity events visible to the caller, honoring each
// author's privacy setting.
//
// @Summary      Activity feed
// @Tags         activity
// @Security     BearerAuth
// @Produce      json
// @Success      200  {array}  activityEventView
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
	Item *activityItemView `json:"item,omitempty" validate:"optional"`
}

// activityItemView is the display detail of the item an event references:
// a title for every type, plus the fields that type has (artist for an album,
// album and duration for a track, ...).
type activityItemView struct {
	Title    string `json:"title"`
	Artist   string `json:"artist,omitempty" validate:"optional"`
	Album    string `json:"album,omitempty" validate:"optional"`
	AlbumID  string `json:"albumId,omitempty" validate:"optional"`
	ArtistID string `json:"artistId,omitempty" validate:"optional"`
	CoverArt string `json:"coverArt,omitempty" validate:"optional"`
	Duration int    `json:"duration,omitempty" validate:"optional"`
	Year     int    `json:"year,omitempty" validate:"optional"`
}

// activityItem resolves display details for the item an event references. Returns
// nil when the item can't be resolved (e.g. a remote favorite never downloaded,
// or a deleted item) — the event still carries its ids.
func (h *Handler) activityItem(ctx context.Context, e models.ActivityEvent) *activityItemView {
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
		return &activityItemView{
			Title: t.Title, Artist: t.ArtistName, Album: t.AlbumName, AlbumID: t.AlbumID,
			ArtistID: t.ArtistID, CoverArt: cover, Duration: t.Duration,
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
		return &activityItemView{Title: a.Name, Artist: a.ArtistName, ArtistID: a.ArtistID, CoverArt: cover, Year: a.Year}
	case models.ItemArtist:
		if h.Catalog == nil {
			return nil
		}
		a, err := h.Catalog.GetArtist(ctx, e.ItemID)
		if err != nil {
			return nil
		}
		return &activityItemView{Title: a.Name}
	case models.ItemPlaylist:
		p, err := h.Playlists.Get(ctx, e.ItemID)
		if err != nil {
			return nil
		}
		return &activityItemView{Title: p.Name}
	}
	return nil
}

// handleListPublicUsers lists every user on the server, minimally (id,
// username, displayName) -- a member directory so any authenticated caller
// can find someone's profile, unlike GET /admin/users which is admin-only and
// exposes the full account (email, admin flag, scrobbling setting).
//
// @Summary      List users
// @Description  Returns every user's public identity (id, username, display name) so any authenticated caller can browse to a profile.
// @Tags         users
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  userListView
// @Failure      401  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /users [get]
func (h *Handler) handleListPublicUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.Services.Users.ListPublicUsers(r.Context())
	if err != nil {
		writeInternal(w, err)
		return
	}
	out := make([]userSummaryView, 0, len(users))
	for _, u := range users {
		out = append(out, userSummaryView{ID: u.ID, Username: u.Username, DisplayName: u.DisplayName})
	}
	writeResource(w, http.StatusOK, userListView{Users: out})
}

// userSummaryView is a user's public identity.
type userSummaryView struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName,omitempty" validate:"optional"`
}

type userListView struct {
	Users []userSummaryView `json:"users" validate:"required"`
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
// @Success      200  {object}  profileView
// @Failure      401  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /users/{username} [get]
func (h *Handler) handleProfile(w http.ResponseWriter, r *http.Request) {
	caller := userFrom(r.Context())
	target, err := h.resolveProfileUser(r)
	if err != nil {
		if !isNotFound(err) {
			writeInternal(w, err)
			return
		}
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
	playlists := make([]profilePlaylistView, 0, len(lists))
	for _, p := range lists {
		playlists = append(playlists, profilePlaylistView{
			ID: p.ID, Name: p.Name, Comment: p.Comment, SongCount: p.SongCount,
			Duration: p.Duration, CoverArt: p.CoverArt, CoverArts: p.CoverArts,
		})
	}

	isSelf := target.ID == caller.ID

	stats := profileStatsView{Playlists: len(playlists)}
	if h.Wrapped != nil {
		if plays, seconds, err := h.Wrapped.Totals(r.Context(), target.ID); err == nil {
			stats.Plays = plays
			stats.ListenSeconds = seconds
		}
	}

	resp := profileView{
		User:      profileUserView{ID: target.ID, Username: target.Username, DisplayName: target.DisplayName, IsAdmin: target.IsAdmin},
		IsSelf:    isSelf,
		Activity:  activity,
		Playlists: playlists,
		Stats:     stats,
	}

	// The top of the target's Hall of Fame, omitted entirely when it's empty
	// (rather than shipping an empty section the profile page would hide anyway).
	if h.HallOfFame != nil && h.hallOfFameEnabled() {
		if d, err := h.Services.HallOfFame.Get(r.Context(), target.ID); err == nil && len(d.Entries) > 0 {
			top := d.Entries
			if len(top) > 3 {
				top = top[:3]
			}
			resp.HallOfFame = &profileHallOfFameView{Top: hallOfFameEntriesToSongViews(top), Total: len(d.Entries)}
		}
	}

	writeResource(w, http.StatusOK, resp)
}

// profileView is a user's public profile.
type profileView struct {
	User      profileUserView       `json:"user"`
	IsSelf    bool                  `json:"isSelf"`
	Activity  []activityEventView   `json:"activity" validate:"required"`
	Playlists []profilePlaylistView `json:"playlists" validate:"required"`
	Stats     profileStatsView      `json:"stats"`
	// HallOfFame is the top 3 of the user's Hall of Fame, absent when empty.
	HallOfFame *profileHallOfFameView `json:"hallOfFame,omitempty" validate:"optional"`
}

type profileUserView struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	IsAdmin     bool   `json:"isAdmin"`
}

// profilePlaylistView is one of the user's public playlists.
type profilePlaylistView struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Comment   string   `json:"comment"`
	SongCount int      `json:"songCount"`
	Duration  int      `json:"duration"`
	CoverArt  string   `json:"coverArt"`
	CoverArts []string `json:"coverArts,omitempty" validate:"optional"`
}

// profileStatsView is the profile's all-time listening totals.
type profileStatsView struct {
	Plays         int   `json:"plays"`
	ListenSeconds int64 `json:"listenSeconds"`
	Playlists     int   `json:"playlists"`
}

type profileHallOfFameView struct {
	Top   []songView `json:"top" validate:"required"`
	Total int        `json:"total"`
}

// addCollaboratorBody is the body for POST /playlists/{id}/collaborators.
type addCollaboratorBody struct {
	Username string `json:"username" validate:"optional"`
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
// @Success      201  {object}  collaboratorView  "added"
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
		writeLookupError(w, err, "playlist not found")
		return
	}
	if p.Federated || (p.OwnerID != user.ID && !user.IsAdmin) {
		writeError(w, http.StatusForbidden, "forbidden", "only the owner can add collaborators")
		return
	}
	collaborator, err := h.Users.GetByUsername(r.Context(), req.Username)
	if err != nil {
		if !isNotFound(err) {
			writeInternal(w, err)
			return
		}
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
	writeResource(w, http.StatusCreated, collaboratorView{Collaborator: collaborator.Username})
}

// collaboratorView names the user just granted edit rights.
type collaboratorView struct {
	Collaborator string `json:"collaborator"`
}
