package immerle

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	chi "github.com/go-chi/chi/v5"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/integrations/listenbrainz"
	"github.com/immerle/immerle/internal/models"
)

// accountRoutes mounts the caller's account, other users' profiles, API tokens,
// device sessions and UI theme. Admin routes go on admin, which is already
// under /admin and admin-only.
func (h *Handler) accountRoutes(r, admin chi.Router) {
	r.Get("/me", h.handleAccount)
	r.Patch("/me", h.handleAccountUpdate)
	r.Put("/me/password", h.handleChangePassword)
	r.Get("/users", h.handleListPublicUsers)
	r.Get("/users/{username}", h.handleProfile)
	r.Get("/tokens", h.handleTokens)
	r.Post("/tokens", h.handleCreateToken)
	r.Delete("/tokens/{id}", h.handleRevokeToken)
	r.Get("/devices", h.handleDevices)
	r.Delete("/devices/{id}", h.handleRevokeDevice)
	r.Get("/theme", h.handleTheme)
	r.Patch("/theme", h.handleThemeUpdate)

	admin.Get("/users", h.handleListUsers)
	admin.Post("/users", h.handleCreateUser)
	admin.Get("/users/{username}", h.handleGetUser)
	admin.Patch("/users/{username}", h.handleUpdateUser)
	admin.Delete("/users/{username}", h.handleDeleteUser)
}

// validEmail is a light sanity check (not full RFC 5322): a bare address with no
// display-name part.
func validEmail(s string) bool {
	addr, err := mail.ParseAddress(s)
	return err == nil && addr.Address == s
}

// supportedLanguages are the UI languages the client ships translations for.
// "" is also accepted (clears the preference → client uses the device locale).
var supportedLanguages = map[string]bool{"en": true, "fr": true}

// accountView is the caller's own account (includes the private email).
type accountView struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
	IsAdmin     bool   `json:"isAdmin"`
	// Language is the UI language ("en", "fr"), empty to follow the device.
	Language string `json:"language"`
	// ListenBrainzToken is the caller's ListenBrainz API token, empty when
	// ListenBrainz scrobbling is off.
	ListenBrainzToken string `json:"listenBrainzToken"`
}

func toAccountView(u models.User) accountView {
	return accountView{
		ID: u.ID, Username: u.Username, DisplayName: u.DisplayName, Email: u.Email,
		IsAdmin: u.IsAdmin, Language: u.Language, ListenBrainzToken: u.ListenBrainzToken,
	}
}

// handleAccount returns the caller's own account settings. Unlike the public
// profile (/users/{username}), it exposes the private email.
//
// @Summary      Get your account
// @Description  Returns the authenticated user's own account, including the private email.
// @Tags         users
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  accountView
// @Failure      401  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /me [get]
func (h *Handler) handleAccount(w http.ResponseWriter, r *http.Request) {
	caller := userFrom(r.Context())
	user, err := h.Users.GetByID(r.Context(), caller.ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusOK, toAccountView(user))
}

// updateAccountRequest is a partial account update; pointer fields distinguish
// "omitted" (keep) from "" (clear).
type updateAccountRequest struct {
	DisplayName       *string `json:"displayName,omitempty" validate:"optional"`
	Email             *string `json:"email,omitempty" validate:"optional"`
	Language          *string `json:"language,omitempty" validate:"optional"`
	ListenBrainzToken *string `json:"listenBrainzToken,omitempty" validate:"optional"`
}

// validateTokenTimeout bounds the live ListenBrainz check so a slow/unreachable
// third party never hangs a PATCH /me request.
const validateTokenTimeout = 5 * time.Second

// handleAccountUpdate applies a partial update to the caller's own account.
//
// @Summary      Update your account
// @Description  Partial update — only fields present are changed. Lets a user set their display name, email and ListenBrainz token themselves.
// @Tags         users
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  updateAccountRequest  true  "Account fields to change"
// @Success      200  {object}  accountView
// @Failure      400  {object}  errorResponse
// @Failure      401  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /me [patch]
func (h *Handler) handleAccountUpdate(w http.ResponseWriter, r *http.Request) {
	caller := userFrom(r.Context())
	user, err := h.Users.GetByID(r.Context(), caller.ID)
	if err != nil {
		writeInternal(w, err)
		return
	}

	var req updateAccountRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.DisplayName != nil {
		user.DisplayName = core.NormalizeDisplayName(*req.DisplayName)
	}
	if req.Email != nil {
		email := strings.TrimSpace(*req.Email)
		if email != "" && !validEmail(email) {
			writeError(w, http.StatusBadRequest, "validation", "email must be a valid address like name@example.com")
			return
		}
		user.Email = email
	}
	if req.Language != nil {
		lang := strings.TrimSpace(*req.Language)
		if lang != "" && !supportedLanguages[lang] {
			writeError(w, http.StatusBadRequest, "validation", "language must be one of: en, fr")
			return
		}
		user.Language = lang
	}
	if req.ListenBrainzToken != nil {
		token := strings.TrimSpace(*req.ListenBrainzToken)
		if token != "" && h.ListenBrainz != nil {
			vctx, cancel := context.WithTimeout(r.Context(), validateTokenTimeout)
			_, verr := h.ListenBrainz.ValidateToken(vctx, token)
			cancel()
			if errors.Is(verr, listenbrainz.ErrInvalidToken) {
				writeError(w, http.StatusBadRequest, "validation", "ListenBrainz token was rejected — check it was copied correctly")
				return
			}
			// A network/timeout error talking to ListenBrainz itself (not an
			// explicit rejection) shouldn't block saving your own settings.
			if verr != nil && h.Logger != nil {
				h.Logger.Warn("listenbrainz: could not validate token, saving anyway", "error", verr)
			}
		}
		user.ListenBrainzToken = token
	}
	if err := h.Users.Update(r.Context(), user); err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusOK, toAccountView(user))
}
