package immerle

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/immerle/immerle/internal/models"
)

// maxActiveConcerts bounds how many upcoming matches GET /me/concerts
// returns — plenty for a "next up" banner without unbounded growth.
const maxActiveConcerts = 20

// concertsEnabled reports whether concert discovery is on (defaults to off
// when settings are unavailable, e.g. in tests — unlike the other toggleable
// features, this one needs an API key to be useful, so "off" is the safe
// default here rather than "on").
func (h *Handler) concertsEnabled() bool {
	return h.Settings != nil && h.Settings.ConcertsEnabled()
}

// handleMyConcerts returns the caller's upcoming, non-dismissed concert
// matches, soonest first.
//
// @Summary      Your upcoming concert matches
// @Description  Concert discovery matches your top-listened artists (see your account's city) against Ticketmaster/Skiddle, refreshed daily. Returns upcoming, non-dismissed matches, soonest first. Empty (not an error) when the feature is disabled, no city is set, or nothing matched yet.
// @Tags         concerts
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  ConcertsDTO
// @Failure      401  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /me/concerts [get]
func (h *Handler) handleMyConcerts(w http.ResponseWriter, r *http.Request) {
	if !h.concertsEnabled() || h.Concerts == nil {
		writeResource(w, http.StatusOK, map[string]any{"concerts": []models.Concert{}})
		return
	}
	list, err := h.Concerts.ListActive(r.Context(), userFrom(r.Context()).ID, time.Now(), maxActiveConcerts)
	if err != nil {
		writeInternal(w, err)
		return
	}
	if list == nil {
		list = []models.Concert{}
	}
	writeResource(w, http.StatusOK, map[string]any{"concerts": list})
}

// handleDismissConcert closes a concert match for the caller — it stays
// dismissed permanently, even after the next daily sync.
//
// @Summary      Dismiss a concert match
// @Tags         concerts
// @Security     BearerAuth
// @Param        id  path  string  true  "Concert id"
// @Success      204  "dismissed"
// @Failure      401  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /me/concerts/{id}/dismiss [put]
func (h *Handler) handleDismissConcert(w http.ResponseWriter, r *http.Request) {
	if h.Concerts == nil {
		writeError(w, http.StatusNotFound, "not_found", "concert not found")
		return
	}
	found, err := h.Concerts.Dismiss(r.Context(), userFrom(r.Context()).ID, pathParam(r, "id"))
	if err != nil {
		writeInternal(w, err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "concert not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// concertsStatus is the admin view of concert-discovery config. The API keys
// themselves are write-only (see redactSettings) — this only reports whether
// one is set, never its value.
func (h *Handler) concertsStatus() ConcertsStatusDTO {
	cfg := models.ConcertsRuntime{}
	if h.Settings != nil {
		cfg = h.Settings.ConcertsConfig()
	}
	return ConcertsStatusDTO{
		Enabled:                cfg.Enabled,
		Country:                cfg.Country,
		TicketmasterConfigured: cfg.TicketmasterAPIKey != "",
		SkiddleConfigured:      cfg.SkiddleAPIKey != "",
	}
}

// handleConcertsAdmin reports the concert-discovery feature state.
//
// @Summary  Get the concert-discovery feature state
// @Tags     admin
// @Security BearerAuth
// @Produce  json
// @Success  200  {object}  ConcertsStatusDTO
// @Router   /admin/concerts [get]
func (h *Handler) handleConcertsAdmin(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	writeResource(w, http.StatusOK, h.concertsStatus())
}

// concertsUpdateRequest is a partial update of concert-discovery settings;
// pointer fields distinguish "omitted" (keep current) from "" (clear).
type concertsUpdateRequest struct {
	Enabled *bool `json:"enabled"`
	// Country is an ISO 3166-1 alpha-2 code (e.g. "FR") from the admin UI's
	// fixed dropdown — the single instance-wide location concert discovery
	// searches near (there is no per-user location).
	Country            *string `json:"country"`
	TicketmasterAPIKey *string `json:"ticketmasterApiKey"`
	SkiddleAPIKey      *string `json:"skiddleApiKey"`
}

// handleConcertsUpdate changes concert-discovery settings: enable/disable and
// set the Ticketmaster/Skiddle API keys (hot-reloaded, no restart needed).
//
// @Summary  Update concert-discovery settings
// @Description  Admin only. Partial update — only fields present are changed. API keys are write-only: the response never echoes them back.
// @Tags     admin
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    body  body  concertsUpdateRequest  true  "Fields to change"
// @Success  200  {object}  ConcertsStatusDTO
// @Failure  400  {object}  errorResponse
// @Router   /admin/concerts [put]
func (h *Handler) handleConcertsUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.Settings == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "settings not available")
		return
	}
	var req concertsUpdateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	current := h.Settings.Get()
	next := current
	if req.Enabled != nil {
		next.Concerts.Enabled = *req.Enabled
	}
	if req.Country != nil {
		next.Concerts.Country = strings.ToUpper(strings.TrimSpace(*req.Country))
	}
	if req.TicketmasterAPIKey != nil {
		next.Concerts.TicketmasterAPIKey = *req.TicketmasterAPIKey
	}
	if req.SkiddleAPIKey != nil {
		next.Concerts.SkiddleAPIKey = *req.SkiddleAPIKey
	}
	if _, _, err := h.Settings.Update(next); err != nil {
		writeInternal(w, err)
		return
	}
	h.Logger.Info("concert discovery settings updated", "enabled", next.Concerts.Enabled, "country", next.Concerts.Country, "by", userFrom(r.Context()).Username)
	// A changed country invalidates every user's previous (unmatched) search —
	// give an immediate result instead of making everyone wait for the next
	// daily sync.
	if h.ConcertsSync != nil && next.Concerts.Enabled && next.Concerts.Country != current.Concerts.Country {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			if _, err := h.ConcertsSync.SyncNow(ctx); err != nil {
				h.Logger.Warn("concerts: sync on country change failed", "error", err)
			}
		}()
	}
	writeResource(w, http.StatusOK, h.concertsStatus())
}

// handleConcertsSync triggers an immediate concert-discovery sync, regardless
// of the daily schedule.
//
// @Summary      Sync concert discovery now
// @Description  Admin only. Searches every user-with-a-city's top-listened artists for nearby upcoming shows immediately, returning how many new matches were found.
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Success      201  {object}  ChartsSyncDTO
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /admin/concerts/sync [post]
func (h *Handler) handleConcertsSync(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.ConcertsSync == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "concert sync not available")
		return
	}
	synced, err := h.ConcertsSync.SyncNow(r.Context())
	if err != nil {
		writeInternal(w, err)
		return
	}
	h.Logger.Info("concerts synced on demand", "synced", synced, "by", userFrom(r.Context()).Username)
	writeResource(w, http.StatusCreated, map[string]any{"synced": synced})
}
