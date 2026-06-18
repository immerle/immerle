package immerle

import (
	"net/http"
	"net/mail"
	"strings"

	"github.com/immerle/immerle/internal/core"
	"github.com/immerle/immerle/internal/models"
)

// validEmail is a light sanity check (not full RFC 5322): a bare address with no
// display-name part.
func validEmail(s string) bool {
	addr, err := mail.ParseAddress(s)
	return err == nil && addr.Address == s
}

// accountView is the caller's own account (includes the private email).
func accountView(u models.User) map[string]any {
	return map[string]any{
		"id":          u.ID,
		"username":    u.Username,
		"displayName": u.DisplayName,
		"email":       u.Email,
		"isAdmin":     u.IsAdmin,
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
	DisplayName *string `json:"displayName"`
	Email       *string `json:"email"`
}

// handleAccountUpdate applies a partial update to the caller's own account.
//
// @Summary      Update your account
// @Description  Partial update — only fields present are changed. Lets a user set their display name and email themselves.
// @Tags         users
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  updateAccountRequest  true  "Account fields to change"
// @Success      200  {object}  AccountDTO
// @Failure      400  {object}  apiError
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
	if err := h.Users.Update(r.Context(), user); err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusOK, accountView(user))
}
