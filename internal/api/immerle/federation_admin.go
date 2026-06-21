package immerle

import (
	"encoding/json"
	"net/http"

	"github.com/immerle/immerle/internal/federation"
)

// handleFederationSearch discovers other instances on the hub.
//
// @Summary      Discover instances on the hub
// @Description  Admin only. Searches the hub for other instances by exact sqid or name (the hub excludes this instance and revoked ones). The HTTP exchange runs server-side.
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Param        q  query  string  true  "Search query (sqid or name)"
// @Success      200  {object}  FederationInstancesDTO
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      502  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /admin/federation/instances [get]
func (h *Handler) handleFederationSearch(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.Federation == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "federation unavailable")
		return
	}
	results, err := h.Federation.SearchInstances(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, http.StatusBadGateway, "search_failed", err.Error())
		return
	}
	writeResource(w, http.StatusOK, map[string]any{"instances": nonNil(results)})
}

// handleFederationSubscriptions lists the instances this one follows.
//
// @Summary      List hub subscriptions
// @Description  Admin only. Returns the instances this one follows on the hub. The HTTP exchange runs server-side.
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  FederationSubscriptionsDTO
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      502  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /admin/federation/subscriptions [get]
func (h *Handler) handleFederationSubscriptions(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.Federation == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "federation unavailable")
		return
	}
	subs, err := h.Federation.Subscriptions(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "subscriptions_failed", err.Error())
		return
	}
	writeResource(w, http.StatusOK, map[string]any{"subscriptions": nonNil(subs)})
}

// handleFederationSubscribe follows a target instance.
//
// @Summary      Subscribe to an instance
// @Description  Admin only. Follows a target instance on the hub by instanceId (UUID) or sqid. The HTTP exchange runs server-side.
// @Tags         admin
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  SubscribeRequestDTO  true  "Target instance id or sqid"
// @Success      200  {object}  OkResponse
// @Failure      400  {object}  errorResponse
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Failure      502  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /admin/federation/subscriptions [post]
func (h *Handler) handleFederationSubscribe(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.Federation == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "federation unavailable")
		return
	}
	var body struct {
		InstanceID string `json:"instanceId"`
		Sqid       string `json:"sqid"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorParams(w, http.StatusBadRequest, "invalid_body", "invalid JSON: "+err.Error(), map[string]any{"detail": err.Error()})
		return
	}
	if err := h.Federation.Subscribe(r.Context(), body.InstanceID, body.Sqid); err != nil {
		writeError(w, http.StatusBadGateway, "subscribe_failed", err.Error())
		return
	}
	writeResource(w, http.StatusOK, map[string]any{"ok": true})
}

// handleFederationUnsubscribe stops following an instance.
//
// @Summary      Unsubscribe from an instance
// @Description  Admin only. Stops following the instance with the given hub id (UUID). The HTTP exchange runs server-side.
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  string  true  "Target instance id (UUID)"
// @Success      200  {object}  OkResponse
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Failure      502  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /admin/federation/subscriptions/{id} [delete]
func (h *Handler) handleFederationUnsubscribe(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.Federation == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "federation unavailable")
		return
	}
	if err := h.Federation.Unsubscribe(r.Context(), pathParam(r, "id")); err != nil {
		writeError(w, http.StatusBadGateway, "unsubscribe_failed", err.Error())
		return
	}
	writeResource(w, http.StatusOK, map[string]any{"ok": true})
}

// nonNil returns a non-nil slice so the JSON renders [] instead of null.
func nonNil(s []federation.InstanceSummary) []federation.InstanceSummary {
	if s == nil {
		return []federation.InstanceSummary{}
	}
	return s
}
