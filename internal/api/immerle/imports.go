package immerle

import (
	"errors"
	"net/http"

	"github.com/immerle/immerle/internal/persistence"
)

// handleImportSources lists the available import sources and whether each is
// configured (credentials present).
//
// @Summary      List import sources
// @Description  Lists the available playlist-import sources (e.g. spotify) and whether each is configured.
// @Tags         imports
// @Security     BearerAuth
// @Produce      json
// @Success      200  {array}  ImportSourceDTO
// @Failure      401  {object}  errorResponse
// @Router       /imports/sources [get]
func (h *Handler) handleImportSources(w http.ResponseWriter, r *http.Request) {
	if h.Imports == nil {
		writeResource(w, http.StatusOK, []any{})
		return
	}
	writeResource(w, http.StatusOK, h.Imports.Sources())
}

// handleImports lists the caller's imports (without items).
//
// @Summary      List imports
// @Description  Lists the caller's playlist imports (most recent first), without per-track items.
// @Tags         imports
// @Security     BearerAuth
// @Produce      json
// @Success      200  {array}  ImportDTO
// @Failure      401  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /imports [get]
func (h *Handler) handleImports(w http.ResponseWriter, r *http.Request) {
	if h.Imports == nil {
		writeResource(w, http.StatusOK, []any{})
		return
	}
	user := userFrom(r.Context())
	list, err := h.Imports.List(r.Context(), user.ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusOK, list)
}

// startImportRequest is the body for POST /imports.
type startImportRequest struct {
	Source string `json:"source"`
	Ref    string `json:"ref"`
}

// handleImportStart queues a playlist import.
//
// @Summary      Start a playlist import
// @Description  Queues an import of an external playlist (by source + reference). Returns the import job; poll GET /imports/{id} for progress. The import creates a new immerle playlist and resolves each source track against the on-demand content providers.
// @Tags         imports
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  startImportRequest  true  "Import source + reference"
// @Success      201  {object}  ImportDTO
// @Failure      400  {object}  errorResponse
// @Failure      401  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /imports [post]
func (h *Handler) handleImportStart(w http.ResponseWriter, r *http.Request) {
	if h.Imports == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "imports not available")
		return
	}
	var req startImportRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	user := userFrom(r.Context())
	im, err := h.Imports.Start(r.Context(), user.ID, req.Source, req.Ref)
	if err != nil {
		writeErrorParams(w, http.StatusBadRequest, "bad_request", err.Error(), map[string]any{"detail": err.Error()})
		return
	}
	writeResource(w, http.StatusCreated, im)
}

// resolveItemRequest is the body for resolving an import item.
type resolveItemRequest struct {
	Query string `json:"query"`
}

// handleImportItemResolve validates or modifies a not-yet-matched import item:
// it downloads a chosen track and adds it to the import's playlist.
//
// @Summary      Validate or modify an import item
// @Description  Resolves a doubtful/missing/failed import item: downloads a track and adds it to the import's playlist, flipping the item to "matched". With no `query`, it validates the flagged candidate as-is; with a `query`, it re-searches the content providers with that corrected text and uses the best result.
// @Tags         imports
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        id      path  string              true   "Import id"
// @Param        itemId  path  string              true   "Import item id"
// @Param        body    body  resolveItemRequest  false  "Optional corrected 'artist title' search"
// @Success      200  {object}  ImportItemDTO
// @Failure      400  {object}  errorResponse
// @Failure      401  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /imports/{id}/items/{itemId}/resolve [post]
func (h *Handler) handleImportItemResolve(w http.ResponseWriter, r *http.Request) {
	if h.Imports == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "imports not available")
		return
	}
	itemID := pathParam(r, "itemId")
	if itemID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "itemId is required")
		return
	}
	var req resolveItemRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	user := userFrom(r.Context())
	item, err := h.Imports.ResolveItem(r.Context(), user.ID, itemID, req.Query)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "import item not found")
			return
		}
		writeErrorParams(w, http.StatusBadRequest, "bad_request", err.Error(), map[string]any{"detail": err.Error()})
		return
	}
	writeResource(w, http.StatusOK, item)
}

// handleImportStatus returns one import with its per-track items (the progress
// view).
//
// @Summary      Import status
// @Description  Returns one import with its per-track items and their status (matched/doubtful/missing/failed) for a progress page.
// @Tags         imports
// @Security     BearerAuth
// @Produce      json
// @Param        id  path  string  true  "Import id"
// @Success      200  {object}  ImportDTO
// @Failure      401  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /imports/{id} [get]
func (h *Handler) handleImportStatus(w http.ResponseWriter, r *http.Request) {
	if h.Imports == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "imports not available")
		return
	}
	user := userFrom(r.Context())
	im, err := h.Imports.Get(r.Context(), user.ID, pathParam(r, "id"))
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "import not found")
			return
		}
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusOK, im)
}
