package immerle

import (
	"errors"
	"net/http"

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
		writeError(w, http.StatusServiceUnavailable, "unavailable", "provider subsystem disabled (set providers.enabled)")
		return false
	}
	return true
}

// handleProviders lists configured on-demand providers.
//
// @Summary      List on-demand providers
// @Description  Admin only. Lists configured providers (built-in and dynamic) with their live status.
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Success      200  {array}  ProviderDTO
// @Failure      401  {object}  apiError
// @Failure      403  {object}  apiError
// @Failure      500  {object}  apiError
// @Failure      503  {object}  apiError
// @Router       /admin/providers [get]
func (h *Handler) handleProviders(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) || !h.providersAvailable(w) {
		return
	}
	list, err := h.Providers.List(r.Context())
	if err != nil {
		writeInternal(w, err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, p := range list {
		out = append(out, h.providerView(p))
	}
	writeResource(w, http.StatusOK, out)
}

// upsertProviderRequest is the body for POST /admin/providers.
type upsertProviderRequest struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Endpoint string `json:"endpoint"`
	Config   string `json:"config"`
	Enabled  *bool  `json:"enabled"`
}

// handleProviderUpsert creates or updates an on-demand provider.
//
// @Summary      Create or update an on-demand provider
// @Description  Admin only. A provider is content-neutral: a name, an HTTP endpoint and an opaque JSON config. Applied immediately — an enabled provider is registered live, a disabled one is removed.
// @Tags         admin
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  upsertProviderRequest  true  "Provider config"
// @Success      200  {object}  ProviderDTO
// @Failure      400  {object}  apiError
// @Failure      401  {object}  apiError
// @Failure      403  {object}  apiError
// @Failure      503  {object}  apiError
// @Router       /admin/providers [post]
func (h *Handler) handleProviderUpsert(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) || !h.providersAvailable(w) {
		return
	}
	var req upsertProviderRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	saved, err := h.Providers.Upsert(r.Context(), models.ProviderConfig{
		Name:     req.Name,
		Kind:     req.Kind,
		Endpoint: req.Endpoint,
		Config:   req.Config,
		Enabled:  enabled,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	h.Logger.Info("provider upserted", "provider", saved.Name, "enabled", saved.Enabled, "by", userFrom(r.Context()).Username)
	writeResource(w, http.StatusOK, h.providerView(saved))
}

// setEnabledRequest is the body for PUT /admin/providers/{name}/enabled.
type setEnabledRequest struct {
	Enabled *bool `json:"enabled"`
}

// handleProviderEnable enables or disables a provider.
//
// @Summary      Enable or disable a provider
// @Description  Admin only. Toggles a provider on or off; the change is applied to the live registry immediately.
// @Tags         admin
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        name  path  string             true  "Provider name"
// @Param        body  body  setEnabledRequest  true  "Enabled flag"
// @Success      200  {object}  ProviderDTO
// @Failure      400  {object}  apiError
// @Failure      401  {object}  apiError
// @Failure      403  {object}  apiError
// @Failure      404  {object}  apiError
// @Failure      503  {object}  apiError
// @Router       /admin/providers/{name}/enabled [put]
func (h *Handler) handleProviderEnable(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) || !h.providersAvailable(w) {
		return
	}
	var req setEnabledRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Enabled == nil {
		writeError(w, http.StatusBadRequest, "validation", "enabled is required")
		return
	}
	name := pathParam(r, "name")
	saved, err := h.Providers.SetEnabled(r.Context(), name, *req.Enabled)
	if err != nil {
		h.writeProviderError(w, err)
		return
	}
	h.Logger.Info("provider toggled", "provider", name, "enabled", *req.Enabled, "by", userFrom(r.Context()).Username)
	writeResource(w, http.StatusOK, h.providerView(saved))
}

// handleProviderDelete removes a provider.
//
// @Summary      Delete a provider
// @Description  Admin only. Removes a provider config and unregisters it.
// @Tags         admin
// @Security     BearerAuth
// @Param        name  path  string  true  "Provider name"
// @Success      204  "deleted"
// @Failure      400  {object}  apiError
// @Failure      401  {object}  apiError
// @Failure      403  {object}  apiError
// @Failure      404  {object}  apiError
// @Failure      503  {object}  apiError
// @Router       /admin/providers/{name} [delete]
func (h *Handler) handleProviderDelete(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) || !h.providersAvailable(w) {
		return
	}
	name := pathParam(r, "name")
	if err := h.Providers.Delete(r.Context(), name); err != nil {
		h.writeProviderError(w, err)
		return
	}
	h.Logger.Info("provider deleted", "provider", name, "by", userFrom(r.Context()).Username)
	writeResource(w, http.StatusNoContent, nil)
}

// reorderRequest is the body for PUT /admin/providers/order.
type reorderRequest struct {
	Order []string `json:"order"`
}

// handleProviderReorder sets the provider priority order.
//
// @Summary      Reorder providers
// @Description  Admin only. Sets the provider priority order (lower = higher priority). `order` lists every provider name, each exactly once. Order also decides which provider search falls back to when no explicit default is set.
// @Tags         admin
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  reorderRequest  true  "Provider names in the desired order"
// @Success      200  {array}  ProviderDTO
// @Failure      400  {object}  apiError
// @Failure      401  {object}  apiError
// @Failure      403  {object}  apiError
// @Failure      500  {object}  apiError
// @Failure      503  {object}  apiError
// @Router       /admin/providers/order [put]
func (h *Handler) handleProviderReorder(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) || !h.providersAvailable(w) {
		return
	}
	var req reorderRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.Providers.Reorder(r.Context(), req.Order); err != nil {
		h.writeProviderError(w, err)
		return
	}
	list, err := h.Providers.List(r.Context())
	if err != nil {
		writeInternal(w, err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, p := range list {
		out = append(out, h.providerView(p))
	}
	h.Logger.Info("providers reordered", "order", req.Order, "by", userFrom(r.Context()).Username)
	writeResource(w, http.StatusOK, out)
}

func (h *Handler) writeProviderError(w http.ResponseWriter, err error) {
	if errors.Is(err, persistence.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "provider not found")
		return
	}
	writeError(w, http.StatusBadRequest, "bad_request", err.Error())
}
