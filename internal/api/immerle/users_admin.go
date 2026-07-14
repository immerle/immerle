package immerle

import (
	"net/http"
	"strconv"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/models"
)

// This file exposes admin user management and self password change over the
// shared core.UserService (which enforces the authorization rules: admin-only
// for management, self-or-admin for reads/password).

// adminUserView is the REST representation of a user in the admin API.
type adminUserView struct {
	ID                string `json:"id"`
	Username          string `json:"username"`
	Email             string `json:"email,omitempty"`
	DisplayName       string `json:"displayName,omitempty"`
	Admin             bool   `json:"admin"`
	ScrobblingEnabled bool   `json:"scrobblingEnabled"`
}

func toAdminUserView(u models.User) adminUserView {
	return adminUserView{
		ID:                u.ID,
		Username:          u.Username,
		Email:             u.Email,
		DisplayName:       u.DisplayName,
		Admin:             u.IsAdmin,
		ScrobblingEnabled: u.ScrobbleEnabled,
	}
}

// handleListUsers lists every user (admin only).
//
// @Summary  List users
// @Tags     admin
// @Security BearerAuth
// @Produce  json
// @Success  200  {object}  map[string][]adminUserView
// @Failure  401  {object}  errorResponse
// @Failure  403  {object}  errorResponse
// @Router   /admin/users [get]
func (h *Handler) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := h.userSvc.ListUsers(r.Context(), userFrom(r.Context()))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	out := make([]adminUserView, 0, len(users))
	for _, u := range users {
		out = append(out, toAdminUserView(u))
	}
	writeResource(w, http.StatusOK, map[string]any{"users": out})
}

// handleGetUser returns a user by name (self or admin).
//
// @Summary  Get user
// @Tags     admin
// @Security BearerAuth
// @Produce  json
// @Param    username  path  string  true  "Username"
// @Success  200  {object}  adminUserView
// @Failure  401  {object}  errorResponse
// @Failure  403  {object}  errorResponse
// @Failure  404  {object}  errorResponse
// @Router   /admin/users/{username} [get]
func (h *Handler) handleGetUser(w http.ResponseWriter, r *http.Request) {
	u, err := h.userSvc.GetUser(r.Context(), userFrom(r.Context()), pathParam(r, "username"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeResource(w, http.StatusOK, toAdminUserView(u))
}

// adminUserCreateRequest is the body for POST /admin/users.
type adminUserCreateRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Admin       bool   `json:"admin"`
}

// handleCreateUser creates a user (admin only).
//
// @Summary  Create user
// @Tags     admin
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    body  body  adminUserCreateRequest  true  "User"
// @Success  201  {object}  adminUserView
// @Failure  400  {object}  errorResponse
// @Failure  401  {object}  errorResponse
// @Failure  403  {object}  errorResponse
// @Router   /admin/users [post]
func (h *Handler) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req adminUserCreateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Username == "" || req.Password == "" {
		writeValidation(w, []fieldError{{Field: "username", Message: "username and password are required"}})
		return
	}
	caller := userFrom(r.Context())
	if err := h.userSvc.CreateUser(r.Context(), caller, req.Username, req.Password, req.Email, req.DisplayName, req.Admin); err != nil {
		writeServiceError(w, err)
		return
	}
	u, err := h.userSvc.GetUser(r.Context(), caller, req.Username)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	h.Logger.Info("user created", "by", caller.Username, "username", req.Username, "admin", req.Admin)
	writeResource(w, http.StatusCreated, toAdminUserView(u))
}

// adminUserUpdateRequest is the body for PATCH /admin/users/{username}. nil
// fields are left unchanged; a non-empty password is applied.
type adminUserUpdateRequest struct {
	Email             *string `json:"email"`
	DisplayName       *string `json:"displayName"`
	Admin             *bool   `json:"admin"`
	ScrobblingEnabled *bool   `json:"scrobblingEnabled"`
	Password          string  `json:"password"`
}

// handleUpdateUser edits a user (admin only).
//
// @Summary  Update user
// @Tags     admin
// @Security BearerAuth
// @Accept   json
// @Param    username  path  string                  true  "Username"
// @Param    body      body  adminUserUpdateRequest  true  "Changes"
// @Success  204  "No Content"
// @Failure  401  {object}  errorResponse
// @Failure  403  {object}  errorResponse
// @Failure  404  {object}  errorResponse
// @Router   /admin/users/{username} [patch]
func (h *Handler) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	var req adminUserUpdateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	upd := core.UserUpdate{Email: req.Email, DisplayName: req.DisplayName, Password: req.Password}
	if req.Admin != nil {
		s := strconv.FormatBool(*req.Admin)
		upd.AdminRaw = &s
	}
	if req.ScrobblingEnabled != nil {
		s := strconv.FormatBool(*req.ScrobblingEnabled)
		upd.ScrobbleRaw = &s
	}
	if err := h.userSvc.UpdateUser(r.Context(), userFrom(r.Context()), pathParam(r, "username"), upd); err != nil {
		writeServiceError(w, err)
		return
	}
	writeResource(w, http.StatusNoContent, nil)
}

// handleDeleteUser removes a user (admin only).
//
// @Summary  Delete user
// @Tags     admin
// @Security BearerAuth
// @Param    username  path  string  true  "Username"
// @Success  204  "No Content"
// @Failure  401  {object}  errorResponse
// @Failure  403  {object}  errorResponse
// @Failure  404  {object}  errorResponse
// @Router   /admin/users/{username} [delete]
func (h *Handler) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	caller := userFrom(r.Context())
	username := pathParam(r, "username")
	if err := h.userSvc.DeleteUser(r.Context(), caller, username); err != nil {
		writeServiceError(w, err)
		return
	}
	h.Logger.Info("user deleted", "by", caller.Username, "username", username)
	writeResource(w, http.StatusNoContent, nil)
}

// passwordRequest is the body for PUT /me/password.
type passwordRequest struct {
	Password string `json:"password"`
}

// handleChangePassword changes the caller's own password.
//
// @Summary  Change own password
// @Tags     account
// @Security BearerAuth
// @Accept   json
// @Param    body  body  passwordRequest  true  "New password"
// @Success  204  "No Content"
// @Failure  400  {object}  errorResponse
// @Failure  401  {object}  errorResponse
// @Router   /me/password [put]
func (h *Handler) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req passwordRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Password == "" {
		writeValidation(w, []fieldError{{Field: "password", Message: "password is required"}})
		return
	}
	caller := userFrom(r.Context())
	if err := h.userSvc.ChangePassword(r.Context(), caller, caller.Username, req.Password); err != nil {
		writeServiceError(w, err)
		return
	}
	writeResource(w, http.StatusNoContent, nil)
}
