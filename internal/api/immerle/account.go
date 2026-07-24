package immerle

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/listenbrainz"
	"github.com/immerle/immerle/internal/models"
)

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
func accountView(u models.User) map[string]any {
	return map[string]any{
		"id":                u.ID,
		"username":          u.Username,
		"displayName":       u.DisplayName,
		"email":             u.Email,
		"isAdmin":           u.IsAdmin,
		"language":          u.Language,
		"listenBrainzToken": u.ListenBrainzToken,
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
// @Success      200  {object}  AccountDTO
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
	writeResource(w, http.StatusOK, accountView(user))
}

// updateAccountRequest is a partial account update; pointer fields distinguish
// "omitted" (keep) from "" (clear).
type updateAccountRequest struct {
	DisplayName       *string `json:"displayName"`
	Email             *string `json:"email"`
	Language          *string `json:"language"`
	ListenBrainzToken *string `json:"listenBrainzToken"`
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
// @Success      200  {object}  AccountDTO
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
	writeResource(w, http.StatusOK, accountView(user))
}
