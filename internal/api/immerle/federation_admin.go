package immerle

import (
	"encoding/json"
	"net/http"

	"github.com/immerle/immerle/internal/services/federation"
)

// handleFederationSearch discovers other instances on the hub.
//
// @Summary      Discover instances on the hub
// @Description  Admin only. Searches the hub for other instances by exact sqid or name (the hub excludes this instance and revoked ones). The HTTP exchange runs server-side.
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Param        q  query  string  true  "Search query (sqid or name)"
// @Success      200  {object}  instanceListView
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      502  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /admin/federation/instances [get]
func (h *Handler) handleFederationSearch(w http.ResponseWriter, r *http.Request) {
	if h.Federation == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "federation unavailable")
		return
	}
	results, err := h.Federation.SearchInstances(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, http.StatusBadGateway, "search_failed", err.Error())
		return
	}
	writeResource(w, http.StatusOK, instanceListView{Instances: toInstanceViews(results)})
}

// handleFederationSubscriptions lists the instances this one follows.
//
// @Summary      List hub subscriptions
// @Description  Admin only. Returns the instances this one follows on the hub. The HTTP exchange runs server-side.
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  subscriptionListView
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      502  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /admin/federation/subscriptions [get]
func (h *Handler) handleFederationSubscriptions(w http.ResponseWriter, r *http.Request) {
	if h.Federation == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "federation unavailable")
		return
	}
	subs, err := h.Federation.Subscriptions(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "subscriptions_failed", err.Error())
		return
	}
	writeResource(w, http.StatusOK, subscriptionListView{Subscriptions: toInstanceViews(subs)})
}

// handleFederationSubscribe follows a target instance.
//
// @Summary      Subscribe to an instance
// @Description  Admin only. Follows a target instance on the hub by instanceId (UUID) or sqid. The HTTP exchange runs server-side.
// @Tags         admin
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  subscribeRequest  true  "Target instance id or sqid"
// @Success      200  {object}  okView
// @Failure      400  {object}  errorResponse
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Failure      502  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /admin/federation/subscriptions [post]
func (h *Handler) handleFederationSubscribe(w http.ResponseWriter, r *http.Request) {
	if h.Federation == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "federation unavailable")
		return
	}
	var body subscribeRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorParams(w, http.StatusBadRequest, "invalid_body", "invalid JSON: "+err.Error(), map[string]any{"detail": err.Error()})
		return
	}
	if err := h.Federation.Subscribe(r.Context(), body.InstanceID, body.Sqid); err != nil {
		writeError(w, http.StatusBadGateway, "subscribe_failed", err.Error())
		return
	}
	writeResource(w, http.StatusOK, okView{Ok: true})
}

// handleFederationUnsubscribe stops following an instance.
//
// @Summary      Unsubscribe from an instance
// @Description  Admin only. Stops following the instance with the given hub id (UUID). The HTTP exchange runs server-side.
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  string  true  "Target instance id (UUID)"
// @Success      200  {object}  okView
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Failure      502  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /admin/federation/subscriptions/{id} [delete]
func (h *Handler) handleFederationUnsubscribe(w http.ResponseWriter, r *http.Request) {
	if h.Federation == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "federation unavailable")
		return
	}
	if err := h.Federation.Unsubscribe(r.Context(), pathParam(r, "id")); err != nil {
		writeError(w, http.StatusBadGateway, "unsubscribe_failed", err.Error())
		return
	}
	writeResource(w, http.StatusOK, okView{Ok: true})
}

// instanceView is a federation hub instance (search result or subscription).
type instanceView struct {
	ID         string `json:"id"`
	Sqid       string `json:"sqid"`
	Name       string `json:"name"`
	Region     string `json:"region"`
	LastSeenAt string `json:"lastSeenAt,omitempty" validate:"optional"`
}

type instanceListView struct {
	Instances []instanceView `json:"instances" validate:"required"`
}

type subscriptionListView struct {
	Subscriptions []instanceView `json:"subscriptions" validate:"required"`
}

// subscribeRequest targets an instance by id or by sqid.
type subscribeRequest struct {
	InstanceID string `json:"instanceId" validate:"optional"`
	Sqid       string `json:"sqid" validate:"optional"`
}

// okView acknowledges a federation action.
type okView struct {
	Ok bool `json:"ok"`
}

// toInstanceViews maps hub instances, never returning a nil slice.
func toInstanceViews(in []federation.InstanceSummary) []instanceView {
	out := make([]instanceView, 0, len(in))
	for _, s := range in {
		out = append(out, instanceView{ID: s.ID, Sqid: s.Sqid, Name: s.Name, Region: s.Region, LastSeenAt: s.LastSeenAt})
	}
	return out
}
