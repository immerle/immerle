package subsonic

import (
	"net/http"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/models"
)

func toUser(u models.User) User {
	return User{
		Username:          u.Username,
		Email:             u.Email,
		DisplayName:       u.DisplayName,
		AdminRole:         u.IsAdmin,
		SettingsRole:      u.IsAdmin,
		DownloadRole:      true,
		UploadRole:        false,
		PlaylistRole:      true,
		CoverArtRole:      true,
		CommentRole:       true,
		PodcastRole:       false,
		StreamRole:        true,
		JukeboxRole:       false,
		ShareRole:         true,
		ScrobblingEnabled: u.ScrobbleEnabled,
	}
}

func (h *Handler) handleGetUser(w http.ResponseWriter, r *http.Request) {
	caller := userFrom(r.Context())
	username := param(r, "username")
	if username == "" {
		username = caller.Username
	}
	// Non-admins may only query themselves.
	if username != caller.Username && !caller.IsAdmin {
		writeError(w, r, ErrUnauthorizedAction, "Not authorized")
		return
	}
	u, err := h.Users.GetByUsername(r.Context(), username)
	if err != nil {
		writeError(w, r, ErrDataNotFound, "User not found")
		return
	}
	resp := newResponse()
	out := toUser(u)
	resp.User = &out
	write(w, r, resp)
}

func (h *Handler) handleGetUsers(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	users, err := h.Users.List(r.Context())
	if err != nil {
		writeError(w, r, ErrGeneric, err.Error())
		return
	}
	resp := newResponse()
	out := &Users{}
	for _, u := range users {
		out.User = append(out.User, toUser(u))
	}
	resp.Users = out
	write(w, r, resp)
}

func (h *Handler) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	username := param(r, "username")
	password := param(r, "password")
	if username == "" || password == "" {
		writeError(w, r, ErrMissingParameter, "username and password are required")
		return
	}
	admin := boolParam(r, "adminRole", false)
	email := param(r, "email")
	displayName := param(r, "displayName")
	if _, err := h.Auth.CreateUser(r.Context(), username, decodeEncParam(password), email, displayName, admin); err != nil {
		writeError(w, r, ErrGeneric, err.Error())
		return
	}
	writeOK(w, r)
}

func (h *Handler) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	username := param(r, "username")
	u, err := h.Users.GetByUsername(r.Context(), username)
	if err != nil {
		writeError(w, r, ErrDataNotFound, "User not found")
		return
	}
	if _, ok := r.Form["email"]; ok {
		u.Email = param(r, "email")
	}
	if _, ok := r.Form["displayName"]; ok {
		u.DisplayName = core.NormalizeDisplayName(param(r, "displayName"))
	}
	if _, ok := r.Form["adminRole"]; ok {
		u.IsAdmin = boolParam(r, "adminRole", u.IsAdmin)
	}
	if _, ok := r.Form["scrobblingEnabled"]; ok {
		u.ScrobbleEnabled = boolParam(r, "scrobblingEnabled", u.ScrobbleEnabled)
	}
	if err := h.Users.Update(r.Context(), u); err != nil {
		writeError(w, r, ErrGeneric, err.Error())
		return
	}
	if pw := param(r, "password"); pw != "" {
		_ = h.Auth.SetPassword(r.Context(), u.ID, decodeEncParam(pw))
	}
	writeOK(w, r)
}

func (h *Handler) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	username := param(r, "username")
	u, err := h.Users.GetByUsername(r.Context(), username)
	if err != nil {
		writeError(w, r, ErrDataNotFound, "User not found")
		return
	}
	if err := h.Users.Delete(r.Context(), u.ID); err != nil {
		writeError(w, r, ErrGeneric, err.Error())
		return
	}
	writeOK(w, r)
}

func (h *Handler) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	caller := userFrom(r.Context())
	username := param(r, "username")
	password := param(r, "password")
	if password == "" {
		writeError(w, r, ErrMissingParameter, "password is required")
		return
	}
	if username != caller.Username && !caller.IsAdmin {
		writeError(w, r, ErrUnauthorizedAction, "Not authorized")
		return
	}
	u, err := h.Users.GetByUsername(r.Context(), username)
	if err != nil {
		writeError(w, r, ErrDataNotFound, "User not found")
		return
	}
	if err := h.Auth.SetPassword(r.Context(), u.ID, decodeEncParam(password)); err != nil {
		writeError(w, r, ErrGeneric, err.Error())
		return
	}
	writeOK(w, r)
}
