package immerle

import (
	"net/http"
	"net/mail"
	"strings"

	"github.com/immerle/immerle/internal/core"
)

// validEmail is a light sanity check (not full RFC 5322): a bare address with no
// display-name part.
func validEmail(s string) bool {
	addr, err := mail.ParseAddress(s)
	return err == nil && addr.Address == s
}

// handleAccount returns or updates the caller's own account settings. Unlike the
// public /profile, it exposes the private email and lets the user change their
// display name and email.
//
// @Summary      Get or update your account
// @Description  Reads (GET) or updates (POST) the authenticated user's own account. POST is a partial update — only fields present are changed. Lets a user set their display name and email themselves.
// @Tags         users
// @Produce      json
// @Param        u            query  string  true   "Subsonic username (or use a Bearer token)"
// @Param        p            query  string  false  "Subsonic password"
// @Param        c            query  string  true   "Client name"
// @Param        displayName  query  string  false  "POST only: free-text UI name (empty clears it)"
// @Param        email        query  string  false  "POST only: email address (empty clears it)"
// @Success      200  {object}  AccountResponse
// @Failure      400  {object}  ErrorResponse
// @Router       /account [get]
// @Router       /account [post]
func (h *Handler) handleAccount(w http.ResponseWriter, r *http.Request) {
	caller := userFrom(r.Context())
	// Reload to avoid persisting stale fields from the auth snapshot.
	user, err := h.Users.GetByID(r.Context(), caller.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}

	if r.Method == http.MethodPost {
		if _, ok := r.Form["displayName"]; ok {
			user.DisplayName = core.NormalizeDisplayName(r.Form.Get("displayName"))
		}
		if _, ok := r.Form["email"]; ok {
			email := strings.TrimSpace(r.Form.Get("email"))
			if email != "" && !validEmail(email) {
				writeJSON(w, http.StatusBadRequest, errorBody("email must be a valid address like name@example.com"))
				return
			}
			user.Email = email
		}
		if err := h.Users.Update(r.Context(), user); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
			return
		}
	}

	writeJSON(w, http.StatusOK, okBody(map[string]any{
		"user": map[string]any{
			"id":          user.ID,
			"username":    user.Username,
			"displayName": user.DisplayName,
			"email":       user.Email,
			"isAdmin":     user.IsAdmin,
		},
	}))
}
