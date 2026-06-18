package immerle

import (
	"errors"
	"net/http"
	"strings"

	"github.com/immerle/immerle/internal/core"
)

// handleSetupStatus reports whether the server needs first-run setup. It is
// public so client apps can show an onboarding screen.
//
// @Summary      First-run setup status
// @Description  Unauthenticated. Reports whether the server still needs its first admin and whether a setup token is required.
// @Tags         setup
// @Produce      json
// @Success      200  {object}  SetupStatusDTO
// @Router       /setup [get]
func (h *Handler) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	initialized, count, err := h.setupState(r)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusOK, map[string]any{
		"initialized":        initialized,
		"needsSetup":         !initialized,
		"userCount":          count,
		"setupTokenRequired": !initialized && h.Setup.TokenRequired(),
	})
}

// setupInitRequest is the body accepted by POST /setup.
type setupInitRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	SetupToken  string `json:"setupToken"`
}

// handleSetupInit creates the first admin account. It is public but self-locks
// once a user exists (409).
//
// @Summary      Create the first administrator
// @Description  Unauthenticated, one-shot. Creates the initial admin — the only way to bootstrap an account (no config/env provisioning). Self-locks once any user exists.
// @Tags         setup
// @Accept       json
// @Produce      json
// @Param        body  body  SetupInitRequest  true  "Initial admin credentials"
// @Success      201  {object}  UserDTO
// @Failure      400  {object}  apiError  "validation"
// @Failure      401  {object}  apiError  "invalid_setup_token"
// @Failure      409  {object}  apiError  "already_initialized"
// @Router       /setup [post]
func (h *Handler) handleSetupInit(w http.ResponseWriter, r *http.Request) {
	var req setupInitRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	user, err := h.Setup.InitFirstAdmin(r.Context(), strings.TrimSpace(req.Username), req.Password, strings.TrimSpace(req.Email), req.DisplayName, req.SetupToken)
	switch {
	case err == nil:
		writeResource(w, http.StatusCreated, map[string]any{
			"id":          user.ID,
			"username":    user.Username,
			"displayName": user.DisplayName,
			"isAdmin":     true,
		})
	case errors.Is(err, core.ErrAlreadyInitialized):
		writeError(w, http.StatusConflict, "already_initialized", "server is already initialized")
	case errors.Is(err, core.ErrInvalidSetupToken):
		writeError(w, http.StatusUnauthorized, "invalid_setup_token", "invalid setup token")
	default:
		var verr *core.ValidationError
		if errors.As(err, &verr) {
			fields := make([]fieldError, 0, len(verr.Fields))
			for _, f := range verr.Fields {
				fields = append(fields, fieldError{Field: f.Field, Message: f.Message})
			}
			writeValidation(w, fields)
			return
		}
		writeInternal(w, err)
	}
}

// setupState returns whether the server is initialized and its user count.
func (h *Handler) setupState(r *http.Request) (bool, int, error) {
	initialized, err := h.Setup.IsInitialized(r.Context())
	if err != nil {
		return false, 0, err
	}
	count := 0
	if c, err := h.Users.Count(r.Context()); err == nil {
		count = c
	}
	return initialized, count, nil
}
