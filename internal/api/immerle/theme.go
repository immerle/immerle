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

// handleTheme returns or updates the caller's UI theme.
//
// @Summary      Get or update the UI theme
// @Description  Reads (GET) or updates (POST) the caller's per-account theme. Only the accent colour is supported for now. POST applies a partial update — omitted fields keep their stored value; pass an empty accentColor to clear it.
// @Tags         theme
// @Produce      json
// @Param        u            query  string  true   "Subsonic username (or use a Bearer token)"
// @Param        p            query  string  false  "Subsonic password"
// @Param        c            query  string  true   "Client name"
// @Param        accentColor  query  string  false  "POST only: CSS hex colour, e.g. #3b82f6 (empty string clears it)"
// @Success      200  {object}  ThemeResponse
// @Failure      400  {object}  ErrorResponse
// @Router       /theme [get]
// @Router       /theme [post]
func (h *Handler) handleTheme(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	theme, err := h.loadTheme(r, user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}

	if r.Method == http.MethodPost {
		// Partial update: only fields present in the form are changed.
		if _, ok := r.Form["accentColor"]; ok {
			color := strings.TrimSpace(r.Form.Get("accentColor"))
			if color != "" && !hexColor.MatchString(color) {
				writeJSON(w, http.StatusBadRequest, errorBody("accentColor must be a hex colour like #3b82f6"))
				return
			}
			theme.AccentColor = color
		}
		encoded, err := json.Marshal(theme)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
			return
		}
		if err := h.Users.SetTheme(r.Context(), user.ID, string(encoded)); err != nil {
			writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
			return
		}
	}

	writeJSON(w, http.StatusOK, okBody(map[string]any{"theme": theme}))
}
