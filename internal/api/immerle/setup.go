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
// @Success      200  {object}  setupStatusView
// @Failure      500  {object}  errorResponse
// @Router       /setup [get]
func (h *Handler) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	initialized, count, err := h.setupState(r)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusOK, setupStatusView{
		Initialized:        initialized,
		NeedsSetup:         !initialized,
		UserCount:          count,
		SetupTokenRequired: !initialized && h.Setup.TokenRequired(),
	})
}

// setupStatusView reports the first-run setup state.
type setupStatusView struct {
	Initialized bool `json:"initialized"`
	NeedsSetup  bool `json:"needsSetup"`
	UserCount   int  `json:"userCount"`
	// SetupTokenRequired is true when POST /setup needs the token printed in
	// the server logs.
	SetupTokenRequired bool `json:"setupTokenRequired"`
}

// setupAdminView is the administrator POST /setup just created.
type setupAdminView struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	IsAdmin     bool   `json:"isAdmin"`
}

// setupInitRequest is the body accepted by POST /setup.
type setupInitRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	Email       string `json:"email" validate:"optional"`
	DisplayName string `json:"displayName" validate:"optional"`
	// SetupToken is required only when GET /setup says setupTokenRequired.
	SetupToken string `json:"setupToken" validate:"optional"`
}

// handleSetupInit creates the first admin account. It is public but self-locks
// once a user exists (409).
//
// @Summary      Create the first administrator
// @Description  Unauthenticated, one-shot. Creates the initial admin. Self-locks once any user exists (also true when the admin was instead bootstrapped from ADMIN_USERNAME/ADMIN_PASSWORD at startup).
// @Tags         setup
// @Accept       json
// @Produce      json
// @Param        body  body  setupInitRequest  true  "Initial admin credentials"
// @Success      201  {object}  setupAdminView
// @Failure      400  {object}  errorResponse  "validation"
// @Failure      401  {object}  errorResponse  "invalid_setup_token"
// @Failure      409  {object}  errorResponse  "already_initialized"
// @Failure      500  {object}  errorResponse
// @Router       /setup [post]
func (h *Handler) handleSetupInit(w http.ResponseWriter, r *http.Request) {
	var req setupInitRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	user, err := h.Setup.InitFirstAdmin(r.Context(), strings.TrimSpace(req.Username), req.Password, strings.TrimSpace(req.Email), req.DisplayName, req.SetupToken)
	switch {
	case err == nil:
		h.Logger.Info("first admin created via setup", "username", user.Username)
		writeResource(w, http.StatusCreated, setupAdminView{ID: user.ID, Username: user.Username, DisplayName: user.DisplayName, IsAdmin: true})
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
