package immerle

import (
	"errors"
	"net/http"
	"time"

	"github.com/immerle/immerle/internal/bandcamp"
	"github.com/immerle/immerle/internal/models"
)

// handleBandcampStatus reports the caller's Bandcamp connection state.
//
// @Summary      Bandcamp connection status
// @Description  Reports whether the caller has connected their Bandcamp account, and whether the stored cookie needs to be refreshed.
// @Tags         purchases
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  BandcampStatusDTO
// @Failure      401  {object}  errorResponse
// @Router       /me/purchases/bandcamp [get]
func (h *Handler) handleBandcampStatus(w http.ResponseWriter, r *http.Request) {
	if h.Purchases == nil {
		writeResource(w, http.StatusOK, BandcampStatusDTO{Connected: false})
		return
	}
	conn, connected, err := h.Purchases.Status(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	if !connected {
		writeResource(w, http.StatusOK, BandcampStatusDTO{Connected: false})
		return
	}
	dto := BandcampStatusDTO{Connected: true, FanID: conn.FanID, NeedsReconnect: conn.InvalidSince != nil}
	if conn.LastSyncedAt != nil {
		dto.LastSyncedAt = conn.LastSyncedAt.UTC().Format(time.RFC3339)
	}
	writeResource(w, http.StatusOK, dto)
}

// bandcampConnectRequest carries the user's pasted Bandcamp session cookie.
type bandcampConnectRequest struct {
	Cookie string `json:"cookie"`
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
// @Success      200  {object}  BandcampStatusDTO
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
	writeResource(w, http.StatusOK, BandcampStatusDTO{Connected: true, FanID: fanID})
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
// @Success      200  {object}  BandcampCollectionDTO
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
	out := make([]BandcampCollectionItemDTO, 0, len(items))
	for _, it := range items {
		dto := BandcampCollectionItemDTO{
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
	writeResource(w, http.StatusOK, BandcampCollectionDTO{Items: out})
}

// bandcampImportRequest carries the display fields for the item being
// imported (the caller already has them from the collection listing) — the
// server re-derives everything else (redownload URL, format) at job time.
type bandcampImportRequest struct {
	ItemType   string `json:"itemType"`
	ArtistName string `json:"artistName"`
	ItemTitle  string `json:"itemTitle"`
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
// @Success      202  {object}  BandcampJobDTO
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
func toBandcampJobDTO(j models.BandcampImportJob) BandcampJobDTO {
	return BandcampJobDTO{
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
// @Success      200  {object}  BandcampJobsDTO
// @Failure      401  {object}  errorResponse
// @Router       /me/purchases/bandcamp/jobs [get]
func (h *Handler) handleBandcampJobs(w http.ResponseWriter, r *http.Request) {
	if h.Purchases == nil {
		writeResource(w, http.StatusOK, BandcampJobsDTO{})
		return
	}
	jobs, err := h.Purchases.ListJobs(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	out := make([]BandcampJobDTO, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, toBandcampJobDTO(j))
	}
	writeResource(w, http.StatusOK, BandcampJobsDTO{Jobs: out})
}
