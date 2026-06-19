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
	u, err := h.userSvc.GetUser(r.Context(), userFrom(r.Context()), param(r, "username"))
	if err != nil {
		h.writeServiceError(w, r, err, "User not found")
		return
	}
	resp := newResponse()
	out := toUser(u)
	resp.User = &out
	write(w, r, resp)
}

func (h *Handler) handleGetUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.userSvc.ListUsers(r.Context(), userFrom(r.Context()))
	if err != nil {
		h.writeServiceError(w, r, err, "User not found")
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
	username := param(r, "username")
	password := param(r, "password")
	if username == "" || password == "" {
		writeError(w, r, ErrMissingParameter, "username and password are required")
		return
	}
	err := h.userSvc.CreateUser(r.Context(), userFrom(r.Context()), username, decodeEncParam(password),
		param(r, "email"), param(r, "displayName"), boolParam(r, "adminRole", false))
	if err != nil {
		h.writeServiceError(w, r, err, "User not found")
		return
	}
	writeOK(w, r)
}

func (h *Handler) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	upd := core.UserUpdate{}
	if _, ok := r.Form["email"]; ok {
		v := param(r, "email")
		upd.Email = &v
	}
	if _, ok := r.Form["displayName"]; ok {
		v := param(r, "displayName")
		upd.DisplayName = &v
	}
	if _, ok := r.Form["adminRole"]; ok {
		v := param(r, "adminRole")
		upd.AdminRaw = &v
	}
	if _, ok := r.Form["scrobblingEnabled"]; ok {
		v := param(r, "scrobblingEnabled")
		upd.ScrobbleRaw = &v
	}
	if pw := param(r, "password"); pw != "" {
		upd.Password = decodeEncParam(pw)
	}
	if err := h.userSvc.UpdateUser(r.Context(), userFrom(r.Context()), param(r, "username"), upd); err != nil {
		h.writeServiceError(w, r, err, "User not found")
		return
	}
	writeOK(w, r)
}

func (h *Handler) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	if err := h.userSvc.DeleteUser(r.Context(), userFrom(r.Context()), param(r, "username")); err != nil {
		h.writeServiceError(w, r, err, "User not found")
		return
	}
	writeOK(w, r)
}

func (h *Handler) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	password := param(r, "password")
	if password == "" {
		writeError(w, r, ErrMissingParameter, "password is required")
		return
	}
	if err := h.userSvc.ChangePassword(r.Context(), userFrom(r.Context()), param(r, "username"), decodeEncParam(password)); err != nil {
		h.writeServiceError(w, r, err, "User not found")
		return
	}
	writeOK(w, r)
}
