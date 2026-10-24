package immerle

import (
	"errors"
	"net/http"
	"time"

	chi "github.com/go-chi/chi/v5"

	"github.com/immerle/immerle/internal/integrations/bandcamp"
	"github.com/immerle/immerle/internal/models"
)

// purchasesRoutes mounts the caller's Bandcamp purchase import.
func (h *Handler) purchasesRoutes(r, _ chi.Router) {
	r.Get("/me/purchases/bandcamp", h.handleBandcampStatus)
	r.Post("/me/purchases/bandcamp/connect", h.handleBandcampConnect)
	r.Delete("/me/purchases/bandcamp", h.handleBandcampDisconnect)
	r.Get("/me/purchases/bandcamp/collection", h.handleBandcampCollection)
	r.Post("/me/purchases/bandcamp/items/{saleItemType}/{saleItemId}/import", h.handleBandcampImport)
	r.Get("/me/purchases/bandcamp/jobs", h.handleBandcampJobs)
}

// handleBandcampStatus reports the caller's Bandcamp connection state.
//
// @Summary      Bandcamp connection status
// @Description  Reports whether the caller has connected their Bandcamp account, and whether the stored cookie needs to be refreshed.
// @Tags         purchases
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  bandcampStatusView
// @Failure      401  {object}  errorResponse
// @Router       /me/purchases/bandcamp [get]
func (h *Handler) handleBandcampStatus(w http.ResponseWriter, r *http.Request) {
	if h.Purchases == nil {
		writeResource(w, http.StatusOK, bandcampStatusView{Connected: false})
		return
	}
	conn, connected, err := h.Purchases.Status(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	if !connected {
		writeResource(w, http.StatusOK, bandcampStatusView{Connected: false})
		return
	}
	dto := bandcampStatusView{Connected: true, FanID: conn.FanID, NeedsReconnect: conn.InvalidSince != nil}
	if conn.LastSyncedAt != nil {
		dto.LastSyncedAt = conn.LastSyncedAt.UTC().Format(time.RFC3339)
	}
	writeResource(w, http.StatusOK, dto)
}

// bandcampConnectRequest carries the user's pasted Bandcamp session cookie.
type bandcampConnectRequest struct {
	Cookie string `json:"cookie" validate:"optional"`
}

// handleBandcampConnect validates and stores the caller's Bandcamp session
// cookie, encrypted at rest. The cookie itself is never echoed back.
//
// @Summary      Connect a Bandcamp account
// @Description  Validates the pasted session cookie against Bandcamp and stores it (encrypted), replacing any previous connection.
// @Tags         purchases
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  bandcampConnectRequest  true  "Bandcamp session cookie"
// @Success      200  {object}  bandcampStatusView
// @Failure      400  {object}  errorResponse
// @Failure      401  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /me/purchases/bandcamp/connect [post]
func (h *Handler) handleBandcampConnect(w http.ResponseWriter, r *http.Request) {
	if h.Purchases == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Bandcamp import is not configured")
		return
	}
	var req bandcampConnectRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	fanID, err := h.Purchases.Connect(r.Context(), userFrom(r.Context()).ID, req.Cookie)
	if err != nil {
		if errors.Is(err, bandcamp.ErrInvalidCookie) {
			writeError(w, http.StatusBadRequest, "invalid_cookie", "this Bandcamp cookie is invalid or expired")
			return
		}
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusOK, bandcampStatusView{Connected: true, FanID: fanID})
}

// handleBandcampDisconnect removes the caller's Bandcamp connection.
//
// @Summary      Disconnect Bandcamp
// @Tags         purchases
// @Security     BearerAuth
// @Success      204  "disconnected"
// @Failure      401  {object}  errorResponse
// @Router       /me/purchases/bandcamp [delete]
func (h *Handler) handleBandcampDisconnect(w http.ResponseWriter, r *http.Request) {
	if h.Purchases == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := h.Purchases.Disconnect(r.Context(), userFrom(r.Context()).ID); err != nil {
		writeInternal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleBandcampCollection lists the caller's Bandcamp purchases, live, each
// annotated with any existing import job for it.
//
// @Summary      List Bandcamp purchases
// @Description  Fetches the caller's purchase collection live from Bandcamp. Each item is annotated with its import job status, if any.
// @Tags         purchases
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  bandcampCollectionView
// @Failure      400  {object}  errorResponse
// @Failure      401  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /me/purchases/bandcamp/collection [get]
func (h *Handler) handleBandcampCollection(w http.ResponseWriter, r *http.Request) {
	if h.Purchases == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Bandcamp import is not configured")
		return
	}
	userID := userFrom(r.Context()).ID
	items, err := h.Purchases.ListCollection(r.Context(), userID)
	if err != nil {
		if errors.Is(err, bandcamp.ErrInvalidCookie) {
			writeError(w, http.StatusBadRequest, "invalid_cookie", "this Bandcamp cookie is invalid or expired — reconnect your account")
			return
		}
		writeInternal(w, err)
		return
	}
	jobs, err := h.Purchases.ListJobs(r.Context(), userID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	jobByKey := make(map[string]int, len(jobs))
	for i, j := range jobs {
		jobByKey[j.SaleItemType+j.SaleItemID] = i
	}
	out := make([]bandcampCollectionItemView, 0, len(items))
	for _, it := range items {
		dto := bandcampCollectionItemView{
			SaleItemType: it.SaleItemType,
			SaleItemID:   it.SaleItemID,
			ItemType:     it.ItemType,
			ArtistName:   it.ArtistName,
			ItemTitle:    it.ItemTitle,
			ArtURL:       it.ArtURL,
			Purchased:    it.Purchased.UTC().Format(time.RFC3339),
		}
		if i, ok := jobByKey[it.SaleItemType+it.SaleItemID]; ok {
			dto.JobStatus = string(jobs[i].Status)
			dto.JobID = jobs[i].ID
		}
		out = append(out, dto)
	}
	writeResource(w, http.StatusOK, bandcampCollectionView{Items: out})
}

// bandcampImportRequest carries the display fields for the item being
// imported (the caller already has them from the collection listing) — the
// server re-derives everything else (redownload URL, format) at job time.
type bandcampImportRequest struct {
	ItemType   string `json:"itemType" validate:"optional"`
	ArtistName string `json:"artistName" validate:"optional"`
	ItemTitle  string `json:"itemTitle" validate:"optional"`
}

// handleBandcampImport queues one purchased item for download+ingest.
// Idempotent: re-importing an already-queued/imported item returns the
// existing job.
//
// @Summary      Import a Bandcamp purchase
// @Tags         purchases
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        saleItemType  path  string  true  "Bandcamp sale item type"
// @Param        saleItemId    path  string  true  "Bandcamp sale item id"
// @Param        body  body  bandcampImportRequest  true  "Item display fields"
// @Success      202  {object}  bandcampJobView
// @Failure      400  {object}  errorResponse
// @Failure      401  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /me/purchases/bandcamp/items/{saleItemType}/{saleItemId}/import [post]
func (h *Handler) handleBandcampImport(w http.ResponseWriter, r *http.Request) {
	if h.Purchases == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Bandcamp import is not configured")
		return
	}
	var req bandcampImportRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	item := bandcamp.CollectionItem{
		SaleItemType: pathParam(r, "saleItemType"),
		SaleItemID:   pathParam(r, "saleItemId"),
		ItemType:     req.ItemType,
		ArtistName:   req.ArtistName,
		ItemTitle:    req.ItemTitle,
	}
	job, err := h.Purchases.EnqueueImport(r.Context(), userFrom(r.Context()).ID, item)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusAccepted, toBandcampJobDTO(job))
}

// toBandcampJobDTO renders a Bandcamp import job for the API.
func toBandcampJobDTO(j models.BandcampImportJob) bandcampJobView {
	return bandcampJobView{
		ID:           j.ID,
		SaleItemType: j.SaleItemType,
		SaleItemID:   j.SaleItemID,
		ItemType:     j.ItemType,
		ArtistName:   j.ArtistName,
		ItemTitle:    j.ItemTitle,
		Format:       j.Format,
		Status:       string(j.Status),
		TrackIDs:     j.TrackIDs,
		Error:        j.Error,
		Attempts:     j.Attempts,
		CreatedAt:    j.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:    j.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// handleBandcampJobs lists the caller's Bandcamp import jobs, most recent first.
//
// @Summary      List Bandcamp import jobs
// @Tags         purchases
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  bandcampJobsView
// @Failure      401  {object}  errorResponse
// @Router       /me/purchases/bandcamp/jobs [get]
func (h *Handler) handleBandcampJobs(w http.ResponseWriter, r *http.Request) {
	if h.Purchases == nil {
		writeResource(w, http.StatusOK, bandcampJobsView{})
		return
	}
	jobs, err := h.Purchases.ListJobs(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	out := make([]bandcampJobView, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, toBandcampJobDTO(j))
	}
	writeResource(w, http.StatusOK, bandcampJobsView{Jobs: out})
}

// bandcampStatusView is the caller's Bandcamp connection state. The cookie
// itself is write-only — this never echoes it back.
type bandcampStatusView struct {
	Connected      bool   `json:"connected"`
	FanID          string `json:"fanId,omitempty" validate:"optional"`
	LastSyncedAt   string `json:"lastSyncedAt,omitempty" example:"2026-07-18T21:42:00Z" validate:"optional"`
	NeedsReconnect bool   `json:"needsReconnect,omitempty" validate:"optional"`
}

// bandcampCollectionItemView is one purchased Bandcamp item, annotated with any
// existing import job for it.
type bandcampCollectionItemView struct {
	SaleItemType string `json:"saleItemType" example:"p"`
	SaleItemID   string `json:"saleItemId" example:"123456789"`
	ItemType     string `json:"itemType" example:"album" enums:"album,track"`
	ArtistName   string `json:"artistName"`
	ItemTitle    string `json:"itemTitle"`
	ArtURL       string `json:"artUrl,omitempty" validate:"optional"`
	Purchased    string `json:"purchased" example:"2021-01-01T10:00:00Z"`
	JobStatus    string `json:"jobStatus,omitempty" example:"completed" enums:"queued,running,completed,failed" validate:"optional"`
	JobID        string `json:"jobId,omitempty" validate:"optional"`
}

// bandcampCollectionView wraps the caller's live Bandcamp purchase collection.
type bandcampCollectionView struct {
	Items []bandcampCollectionItemView `json:"items,omitempty" validate:"optional"`
}

// bandcampJobView is one queued/running/completed/failed Bandcamp import.
type bandcampJobView struct {
	ID           string   `json:"id"`
	SaleItemType string   `json:"saleItemType"`
	SaleItemID   string   `json:"saleItemId"`
	ItemType     string   `json:"itemType" example:"album" enums:"album,track"`
	ArtistName   string   `json:"artistName"`
	ItemTitle    string   `json:"itemTitle"`
	Format       string   `json:"format,omitempty" example:"flac" validate:"optional"`
	Status       string   `json:"status" example:"completed" enums:"queued,running,completed,failed"`
	TrackIDs     []string `json:"trackIds,omitempty" validate:"optional"`
	Error        string   `json:"error,omitempty" validate:"optional"`
	Attempts     int      `json:"attempts"`
	CreatedAt    string   `json:"createdAt" example:"2026-07-18T21:42:00Z"`
	UpdatedAt    string   `json:"updatedAt" example:"2026-07-18T21:43:00Z"`
}

// bandcampJobsView wraps the caller's Bandcamp import job history.
type bandcampJobsView struct {
	Jobs []bandcampJobView `json:"jobs,omitempty" validate:"optional"`
}
