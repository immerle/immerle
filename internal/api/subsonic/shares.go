package subsonic

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/immerle/immerle/internal/models"
)

func (h *Handler) shareURL(secret string) string {
	base := strings.TrimRight(h.BaseURL, "/")
	return base + "/share/" + secret
}

func (h *Handler) toShare(r *http.Request, s models.Share) Share {
	out := Share{
		ID:          s.ID,
		URL:         h.shareURL(s.Secret),
		Description: s.Description,
		Username:    userFrom(r.Context()).Username,
		Created:     formatTime(s.CreatedAt),
		VisitCount:  s.ViewCount,
	}
	if s.ExpiresAt != nil {
		out.Expires = formatTime(*s.ExpiresAt)
	}
	// Resolve the shared entry into entries.
	switch s.ItemType {
	case models.ItemTrack:
		if t, err := h.Catalog.GetTrack(r.Context(), s.ItemID); err == nil {
			out.Entry = append(out.Entry, toChild(t, nil))
		}
	case models.ItemAlbum:
		if tracks, err := h.Catalog.ListTracksByAlbum(r.Context(), s.ItemID); err == nil {
			for _, t := range tracks {
				out.Entry = append(out.Entry, toChild(t, nil))
			}
		}
	case models.ItemPlaylist:
		if tracks, err := h.Playlists.Tracks(r.Context(), s.ItemID); err == nil {
			for _, t := range tracks {
				out.Entry = append(out.Entry, toChild(t, nil))
			}
		}
	}
	return out
}

func (h *Handler) handleGetShares(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	shares, err := h.Shares.ListByUser(r.Context(), user.ID)
	if err != nil {
		h.failInternal(w, r, err)
		return
	}
	resp := newResponse()
	out := &Shares{}
	for _, s := range shares {
		out.Share = append(out.Share, h.toShare(r, s))
	}
	resp.Shares = out
	write(w, r, resp)
}

func (h *Handler) handleCreateShare(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	ids := r.Form["id"]
	if len(ids) == 0 {
		writeError(w, r, ErrMissingParameter, "Required parameter id is missing")
		return
	}
	id := ids[0]
	itemType := h.classifyItem(r, id)

	share := models.Share{
		ID:          newID(),
		UserID:      user.ID,
		ItemType:    itemType,
		ItemID:      id,
		Secret:      randomSecret(),
		Description: param(r, "description"),
		CreatedAt:   time.Now(),
	}
	if exp := intParam(r, "expires", 0); exp > 0 {
		t := time.UnixMilli(int64(exp))
		share.ExpiresAt = &t
	}
	if err := h.Shares.Create(r.Context(), share); err != nil {
		h.failInternal(w, r, err)
		return
	}
	resp := newResponse()
	resp.Shares = &Shares{Share: []Share{h.toShare(r, share)}}
	write(w, r, resp)
}

func (h *Handler) handleUpdateShare(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	id := param(r, "id")
	if id == "" {
		writeError(w, r, ErrMissingParameter, "Required parameter id is missing")
		return
	}
	var expires *time.Time
	if exp := intParam(r, "expires", 0); exp > 0 {
		t := time.UnixMilli(int64(exp))
		expires = &t
	}
	if err := h.Shares.Update(r.Context(), id, user.ID, param(r, "description"), expires); err != nil {
		h.failInternal(w, r, err)
		return
	}
	writeOK(w, r)
}

func (h *Handler) handleDeleteShare(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	id := param(r, "id")
	if err := h.Shares.Delete(r.Context(), id, user.ID); err != nil {
		h.failInternal(w, r, err)
		return
	}
	writeOK(w, r)
}

// classifyItem determines whether an id is a track, album or playlist.
func (h *Handler) classifyItem(r *http.Request, id string) models.ItemType {
	if _, err := h.Catalog.GetAlbum(r.Context(), id); err == nil {
		return models.ItemAlbum
	}
	if _, err := h.Playlists.Get(r.Context(), id); err == nil {
		return models.ItemPlaylist
	}
	return models.ItemTrack
}

func randomSecret() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
