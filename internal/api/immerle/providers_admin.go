package immerle

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// providerView serializes a provider config plus its live status for the API.
func (h *Handler) providerView(p models.ProviderConfig) map[string]any {
	return map[string]any{
		"name":      p.Name,
		"kind":      p.Kind,
		"endpoint":  p.Endpoint,
		"config":    p.Config,
		"enabled":   p.Enabled,
		"active":    h.Providers.Active(p.Name), // currently live in the registry
		"builtin":   p.Builtin(),
		"deletable": !p.Builtin(), // built-ins can be disabled but not removed
		"sortOrder": p.SortOrder,
		"createdAt": p.CreatedAt,
		"updatedAt": p.UpdatedAt,
	}
}

// providersAvailable guards endpoints on the provider subsystem being enabled.
func (h *Handler) providersAvailable(w http.ResponseWriter) bool {
	if h.Providers == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorBody("provider subsystem disabled (set providers.enabled)"))
		return false
	}
	return true
}

// handleProviders lists (GET) or creates/updates (POST) on-demand providers.
//
// @Summary      List or upsert on-demand providers
// @Description  Admin only. GET lists configured providers; POST creates or updates one. A provider is content-neutral: a name, an HTTP endpoint and an opaque JSON config. POST applies immediately — an enabled provider is registered live, a disabled one is removed.
// @Tags         admin
// @Produce      json
// @Param        u         query  string  true   "Subsonic username (or Bearer token)"
// @Param        p         query  string  false  "Subsonic password"
// @Param        c         query  string  true   "Client name"
// @Param        name      query  string  false  "POST: provider name (slug: a-z 0-9 - _)"
// @Param        endpoint  query  string  false  "POST: base http(s) URL of the external service"
// @Param        config    query  string  false  "POST: JSON config payload (default {})"
// @Param        enabled   query  bool    false  "POST: register it live (default true)"
// @Param        kind      query  string  false  "POST: provider kind (default http)"
// @Success      200  {object}  ProvidersResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      403  {object}  ErrorResponse
// @Router       /admin/providers [get]
// @Router       /admin/providers [post]
func (h *Handler) handleProviders(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) || !h.providersAvailable(w) {
		return
	}

	if r.Method == http.MethodPost {
		enabled := true
		if raw := r.Form.Get("enabled"); raw != "" {
			v, err := strconv.ParseBool(raw)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errorBody("enabled must be true or false"))
				return
			}
			enabled = v
		}
		cfg := models.ProviderConfig{
			Name:     r.Form.Get("name"),
			Kind:     r.Form.Get("kind"),
			Endpoint: r.Form.Get("endpoint"),
			Config:   r.Form.Get("config"),
			Enabled:  enabled,
		}
		saved, err := h.Providers.Upsert(r.Context(), cfg)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorBody(err.Error()))
			return
		}
		h.Logger.Info("provider upserted", "provider", saved.Name, "enabled", saved.Enabled, "by", userFrom(r.Context()).Username)
		writeJSON(w, http.StatusOK, okBody(map[string]any{"provider": h.providerView(saved)}))
		return
	}

	list, err := h.Providers.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, p := range list {
		out = append(out, h.providerView(p))
	}
	writeJSON(w, http.StatusOK, okBody(map[string]any{"providers": out}))
}

// handleProviderEnable enables or disables a provider.
//
// @Summary      Enable or disable a provider
// @Description  Admin only. Toggles a provider on or off; the change is applied to the live registry immediately.
// @Tags         admin
// @Produce      json
// @Param        u        query  string  true   "Subsonic username (or Bearer token)"
// @Param        p        query  string  false  "Subsonic password"
// @Param        c        query  string  true   "Client name"
// @Param        name     query  string  true   "Provider name"
// @Param        enabled  query  bool    true   "Enable (true) or disable (false)"
// @Success      200  {object}  ProviderResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /admin/providers/enable [post]
func (h *Handler) handleProviderEnable(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) || !h.providersAvailable(w) {
		return
	}
	name := r.Form.Get("name")
	enabled, err := strconv.ParseBool(r.Form.Get("enabled"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody("enabled must be true or false"))
		return
	}
	saved, err := h.Providers.SetEnabled(r.Context(), name, enabled)
	if err != nil {
		h.writeProviderError(w, err)
		return
	}
	h.Logger.Info("provider toggled", "provider", name, "enabled", enabled, "by", userFrom(r.Context()).Username)
	writeJSON(w, http.StatusOK, okBody(map[string]any{"provider": h.providerView(saved)}))
}

// handleProviderDelete removes a provider.
//
// @Summary      Delete a provider
// @Description  Admin only. Removes a provider config and unregisters it.
// @Tags         admin
// @Produce      json
// @Param        u     query  string  true   "Subsonic username (or Bearer token)"
// @Param        p     query  string  false  "Subsonic password"
// @Param        c     query  string  true   "Client name"
// @Param        name  query  string  true   "Provider name"
// @Success      200  {object}  OKResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /admin/providers/delete [post]
func (h *Handler) handleProviderDelete(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) || !h.providersAvailable(w) {
		return
	}
	if err := h.Providers.Delete(r.Context(), r.Form.Get("name")); err != nil {
		h.writeProviderError(w, err)
		return
	}
	h.Logger.Info("provider deleted", "provider", r.Form.Get("name"), "by", userFrom(r.Context()).Username)
	writeJSON(w, http.StatusOK, okBody(nil))
}

// handleProviderReorder sets the provider priority order.
//
// @Summary      Reorder providers
// @Description  Admin only. Sets the provider priority order (lower = higher priority). The `order` param is a comma-separated list of every provider name, each exactly once. Order also decides which provider search falls back to when no explicit default is set.
// @Tags         admin
// @Produce      json
// @Param        u      query  string  true   "Subsonic username (or Bearer token)"
// @Param        p      query  string  false  "Subsonic password"
// @Param        c      query  string  true   "Client name"
// @Param        order  query  string  true   "Comma-separated provider names in the desired order"
// @Success      200  {object}  ProvidersResponse
// @Failure      400  {object}  ErrorResponse
// @Router       /admin/providers/reorder [post]
func (h *Handler) handleProviderReorder(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) || !h.providersAvailable(w) {
		return
	}
	names := splitCSV(r.Form.Get("order"))
	if err := h.Providers.Reorder(r.Context(), names); err != nil {
		h.writeProviderError(w, err)
		return
	}
	list, err := h.Providers.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, p := range list {
		out = append(out, h.providerView(p))
	}
	h.Logger.Info("providers reordered", "order", names, "by", userFrom(r.Context()).Username)
	writeJSON(w, http.StatusOK, okBody(map[string]any{"providers": out}))
}

// splitCSV splits a comma-separated list, trimming spaces and dropping empties.
func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (h *Handler) writeProviderError(w http.ResponseWriter, err error) {
	if errors.Is(err, persistence.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, errorBody("provider not found"))
		return
	}
	writeJSON(w, http.StatusBadRequest, errorBody(err.Error()))
}
