package immerle

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/immerle/immerle/internal/models"
)

// hexColor matches a CSS hex colour: #RGB, #RRGGBB or #RRGGBBAA.
var hexColor = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)

// loadTheme reads and decodes a user's stored theme.
func (h *Handler) loadTheme(r *http.Request, userID string) (models.ThemeSettings, error) {
	var theme models.ThemeSettings
	raw, err := h.Users.GetTheme(r.Context(), userID)
	if err != nil {
		return theme, err
	}
	_ = json.Unmarshal([]byte(raw), &theme)
	return theme, nil
}

// handleTheme returns the caller's UI theme.
//
// @Summary      Get the UI theme
// @Tags         theme
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  ThemeDTO
// @Failure      401  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /theme [get]
func (h *Handler) handleTheme(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	theme, err := h.loadTheme(r, user.ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusOK, theme)
}

// updateThemeRequest is a partial theme update; pointer fields distinguish
// "omitted" (keep) from "" (clear).
type updateThemeRequest struct {
	AccentColor *string `json:"accentColor"`
}

// handleThemeUpdate applies a partial update to the caller's UI theme. Only the
// accent colour is supported; pass an empty accentColor to clear it.
//
// @Summary      Update the UI theme
// @Description  Partial update — omitted fields keep their stored value; pass an empty accentColor to clear it.
// @Tags         theme
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  updateThemeRequest  true  "Theme fields to change"
// @Success      200  {object}  ThemeDTO
// @Failure      400  {object}  errorResponse
// @Failure      401  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /theme [patch]
func (h *Handler) handleThemeUpdate(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	theme, err := h.loadTheme(r, user.ID)
	if err != nil {
		writeInternal(w, err)
		return
	}

	var req updateThemeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.AccentColor != nil {
		color := strings.TrimSpace(*req.AccentColor)
		if color != "" && !hexColor.MatchString(color) {
			writeError(w, http.StatusBadRequest, "validation", "accentColor must be a hex colour like #3b82f6")
			return
		}
		theme.AccentColor = color
	}

	encoded, err := json.Marshal(theme)
	if err != nil {
		writeInternal(w, err)
		return
	}
	if err := h.Users.SetTheme(r.Context(), user.ID, string(encoded)); err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusOK, theme)
}
