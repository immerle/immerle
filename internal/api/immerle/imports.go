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
// @Produce      json
// @Param        u  query  string  true   "Subsonic username"
// @Param        p  query  string  false  "Subsonic password (or t+s token auth)"
// @Param        c  query  string  true   "Client name"
// @Success      200  {object}  ImportSourcesResponse
// @Router       /imports/sources [get]
func (h *Handler) handleImportSources(w http.ResponseWriter, r *http.Request) {
	if h.Imports == nil {
		writeJSON(w, http.StatusOK, okBody(map[string]any{"sources": []any{}}))
		return
	}
	writeJSON(w, http.StatusOK, okBody(map[string]any{"sources": h.Imports.Sources()}))
}

// handleImportStart queues a playlist import.
//
// @Summary      Start a playlist import
// @Description  Queues an import of an external playlist (by source + reference). Returns the import job; poll /imports/status for progress. The import creates a new immerle playlist and resolves each source track against the on-demand content providers.
// @Tags         imports
// @Produce      json
// @Param        u       query  string  true   "Subsonic username"
// @Param        p       query  string  false  "Subsonic password (or t+s token auth)"
// @Param        c       query  string  true   "Client name"
// @Param        source  query  string  true   "Import source name (e.g. spotify)"
// @Param        ref     query  string  true   "Source playlist id or URL"
// @Success      200  {object}  ImportResponse
// @Failure      400  {object}  ErrorResponse
// @Router       /imports/start [post]
func (h *Handler) handleImportStart(w http.ResponseWriter, r *http.Request) {
	if h.Imports == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorBody("imports not available"))
		return
	}
	user := userFrom(r.Context())
	source := r.Form.Get("source")
	ref := r.Form.Get("ref")
	im, err := h.Imports.Start(r.Context(), user.ID, source, ref)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, okBody(map[string]any{"import": im}))
}

// handleImports lists the caller's imports (without items).
//
// @Summary      List imports
// @Description  Lists the caller's playlist imports (most recent first), without per-track items.
// @Tags         imports
// @Produce      json
// @Param        u  query  string  true   "Subsonic username"
// @Param        p  query  string  false  "Subsonic password (or t+s token auth)"
// @Param        c  query  string  true   "Client name"
// @Success      200  {object}  ImportsResponse
// @Router       /imports [get]
func (h *Handler) handleImports(w http.ResponseWriter, r *http.Request) {
	if h.Imports == nil {
		writeJSON(w, http.StatusOK, okBody(map[string]any{"imports": []any{}}))
		return
	}
	user := userFrom(r.Context())
	list, err := h.Imports.List(r.Context(), user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, okBody(map[string]any{"imports": list}))
}

// handleImportItemResolve validates or modifies a not-yet-matched import item:
// it downloads a chosen track and adds it to the import's playlist.
//
// @Summary      Validate or modify an import item
// @Description  Resolves a doubtful/missing/failed import item: downloads a track and adds it to the import's playlist, flipping the item to "matched". With no `query`, it validates the flagged candidate as-is; with a `query`, it re-searches the content providers with that corrected text and uses the best result.
// @Tags         imports
// @Produce      json
// @Param        u       query  string  true   "Subsonic username"
// @Param        p       query  string  false  "Subsonic password (or t+s token auth)"
// @Param        c       query  string  true   "Client name"
// @Param        itemId  query  string  true   "Import item id"
// @Param        query   query  string  false  "Corrected 'artist title' search (omit to validate the flagged candidate)"
// @Success      200  {object}  ImportItemResponse
// @Failure      400  {object}  ErrorResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /imports/items/resolve [post]
func (h *Handler) handleImportItemResolve(w http.ResponseWriter, r *http.Request) {
	if h.Imports == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorBody("imports not available"))
		return
	}
	user := userFrom(r.Context())
	itemID := r.Form.Get("itemId")
	if itemID == "" {
		writeJSON(w, http.StatusBadRequest, errorBody("itemId is required"))
		return
	}
	item, err := h.Imports.ResolveItem(r.Context(), user.ID, itemID, r.Form.Get("query"))
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, errorBody("import item not found"))
			return
		}
		writeJSON(w, http.StatusBadRequest, errorBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, okBody(map[string]any{"item": item}))
}

// handleImportStatus returns one import with its per-track items (the progress
// view).
//
// @Summary      Import status
// @Description  Returns one import with its per-track items and their status (matched/doubtful/missing/failed) for a progress page.
// @Tags         imports
// @Produce      json
// @Param        u   query  string  true   "Subsonic username"
// @Param        p   query  string  false  "Subsonic password (or t+s token auth)"
// @Param        c   query  string  true   "Client name"
// @Param        id  query  string  true   "Import id"
// @Success      200  {object}  ImportResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /imports/status [get]
func (h *Handler) handleImportStatus(w http.ResponseWriter, r *http.Request) {
	if h.Imports == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorBody("imports not available"))
		return
	}
	user := userFrom(r.Context())
	id := r.Form.Get("id")
	im, err := h.Imports.Get(r.Context(), user.ID, id)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, errorBody("import not found"))
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, okBody(map[string]any{"import": im}))
}
