package immerle

import (
	"encoding/json"
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
// @Success      200  {object}  SetupStatusResponse
// @Router       /setup/status [get]
func (h *Handler) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	initialized, count, err := h.setupState(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                 true,
		"initialized":        initialized,
		"needsSetup":         !initialized,
		"userCount":          count,
		"setupTokenRequired": !initialized && h.Setup.TokenRequired(),
	})
}

// setupInitRequest is the body accepted by /setup/init (JSON or form).
type setupInitRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	SetupToken  string `json:"setupToken"`
}

// handleSetupInit creates the first admin account. It is public but self-locks
// once a user exists (409). Accepts JSON or form-encoded bodies.
//
// @Summary      Create the first administrator
// @Description  Unauthenticated, one-shot. Creates the initial admin — the only way to bootstrap an account (no config/env provisioning). Self-locks once any user exists.
// @Tags         setup
// @Accept       json
// @Produce      json
// @Param        body  body  SetupInitRequest  true  "Initial admin credentials"
// @Success      201  {object}  SetupInitResponse
// @Failure      400  {object}  ValidationErrorResponse  "validation"
// @Failure      401  {object}  ErrorResponse            "invalid_setup_token"
// @Failure      409  {object}  ErrorResponse            "already_initialized"
// @Router       /setup/init [post]
func (h *Handler) handleSetupInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorBody("method_not_allowed"))
		return
	}

	req := parseSetupInit(r)

	user, err := h.Setup.InitFirstAdmin(r.Context(), strings.TrimSpace(req.Username), req.Password, strings.TrimSpace(req.Email), req.DisplayName, req.SetupToken)
	switch {
	case err == nil:
		writeJSON(w, http.StatusCreated, map[string]any{
			"ok": true,
			"user": map[string]any{
				"id":          user.ID,
				"username":    user.Username,
				"displayName": user.DisplayName,
				"isAdmin":     true,
			},
		})
	case errors.Is(err, core.ErrAlreadyInitialized):
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": "already_initialized"})
	case errors.Is(err, core.ErrInvalidSetupToken):
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "invalid_setup_token"})
	default:
		var verr *core.ValidationError
		if errors.As(err, &verr) {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"ok":      false,
				"error":   "validation",
				"details": verr.Fields,
			})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
	}
}

func parseSetupInit(r *http.Request) setupInitRequest {
	var req setupInitRequest
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		_ = json.NewDecoder(r.Body).Decode(&req)
		return req
	}
	_ = r.ParseForm()
	req.Username = r.Form.Get("username")
	req.Password = r.Form.Get("password")
	req.Email = r.Form.Get("email")
	req.DisplayName = r.Form.Get("displayName")
	req.SetupToken = r.Form.Get("setupToken")
	return req
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
