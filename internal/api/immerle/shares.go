package immerle

import (
	"net/http"
	"strings"
	"time"

	"github.com/immerle/immerle/internal/core"
)

// This file exposes share links over the shared core.ShareService. Ownership is
// enforced by the repository (updates/deletes are scoped to the caller).

// shareView is the REST representation of a share link resolved into its tracks.
type shareView struct {
	ID          string     `json:"id"`
	URL         string     `json:"url"`
	Description string     `json:"description,omitempty"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	ViewCount   int        `json:"viewCount"`
	Entries     []songView `json:"entries"`
}

// shareURL builds the absolute public link for a share secret.
func (h *Handler) shareURL(secret string) string {
	return strings.TrimRight(h.BaseURL, "/") + "/share/" + secret
}

func (h *Handler) toShareView(swe core.ShareWithEntries) shareView {
	s := swe.Share
	v := shareView{
		ID:          s.ID,
		URL:         h.shareURL(s.Secret),
		Description: s.Description,
		ExpiresAt:   s.ExpiresAt,
		CreatedAt:   s.CreatedAt,
		ViewCount:   s.ViewCount,
		Entries:     make([]songView, 0, len(swe.Entries)),
	}
	for _, t := range swe.Entries {
		v.Entries = append(v.Entries, toSongView(t))
	}
	return v
}

// shareExpiry converts an optional epoch-millis value to a time pointer.
func shareExpiry(ms int64) *time.Time {
	if ms <= 0 {
		return nil
	}
	t := time.UnixMilli(ms)
	return &t
}

// handleListShares lists the caller's share links.
//
// @Summary  List shares
// @Tags     shares
// @Security BearerAuth
// @Produce  json
// @Success  200  {object}  map[string][]shareView
// @Failure  401  {object}  errorResponse
// @Router   /shares [get]
func (h *Handler) handleListShares(w http.ResponseWriter, r *http.Request) {
	shares, err := h.shareSvc.List(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	out := make([]shareView, 0, len(shares))
	for _, swe := range shares {
		out = append(out, h.toShareView(swe))
	}
	writeResource(w, http.StatusOK, map[string]any{"shares": out})
}

// shareCreateRequest is the body for POST /shares. expiresAt is epoch millis
// (omit or <=0 for no expiry).
type shareCreateRequest struct {
	ItemID      string `json:"itemId"`
	Description string `json:"description"`
	ExpiresAt   int64  `json:"expiresAt"`
}

// handleCreateShare creates a share link for a track, album or playlist.
//
// @Summary  Create share
// @Tags     shares
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    body  body  shareCreateRequest  true  "Share"
// @Success  201  {object}  shareView
// @Failure  400  {object}  errorResponse
// @Failure  401  {object}  errorResponse
// @Router   /shares [post]
func (h *Handler) handleCreateShare(w http.ResponseWriter, r *http.Request) {
	var req shareCreateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.ItemID == "" {
		writeValidation(w, []fieldError{{Field: "itemId", Message: "itemId is required"}})
		return
	}
	swe, err := h.shareSvc.Create(r.Context(), userFrom(r.Context()).ID, req.ItemID, req.Description, shareExpiry(req.ExpiresAt))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeResource(w, http.StatusCreated, h.toShareView(swe))
}

// shareUpdateRequest is the body for PATCH /shares/{id}. It replaces the
// description and expiry (omit expiresAt or set <=0 to clear it).
type shareUpdateRequest struct {
	Description string `json:"description"`
	ExpiresAt   int64  `json:"expiresAt"`
}

// handleUpdateShare updates a share's description and expiry.
//
// @Summary  Update share
// @Tags     shares
// @Security BearerAuth
// @Accept   json
// @Param    id    path  string              true  "Share id"
// @Param    body  body  shareUpdateRequest  true  "Changes"
// @Success  204  "No Content"
// @Failure  401  {object}  errorResponse
// @Failure  404  {object}  errorResponse
// @Router   /shares/{id} [patch]
func (h *Handler) handleUpdateShare(w http.ResponseWriter, r *http.Request) {
	var req shareUpdateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.shareSvc.Update(r.Context(), pathParam(r, "id"), userFrom(r.Context()).ID, req.Description, shareExpiry(req.ExpiresAt)); err != nil {
		writeServiceError(w, err)
		return
	}
	writeResource(w, http.StatusNoContent, nil)
}

// handleDeleteShare removes a share link.
//
// @Summary  Delete share
// @Tags     shares
// @Security BearerAuth
// @Param    id   path  string  true  "Share id"
// @Success  204  "No Content"
// @Failure  401  {object}  errorResponse
// @Router   /shares/{id} [delete]
func (h *Handler) handleDeleteShare(w http.ResponseWriter, r *http.Request) {
	if err := h.shareSvc.Delete(r.Context(), pathParam(r, "id"), userFrom(r.Context()).ID); err != nil {
		writeServiceError(w, err)
		return
	}
	writeResource(w, http.StatusNoContent, nil)
}
