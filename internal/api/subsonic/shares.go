package subsonic

import (
	"net/http"
	"strings"
	"time"

	"github.com/immerle/immerle/internal/core"
)

func (h *Handler) shareURL(secret string) string {
	base := strings.TrimRight(h.BaseURL, "/")
	return base + "/share/" + secret
}

func (h *Handler) toShare(r *http.Request, swe core.ShareWithEntries) Share {
	s := swe.Share
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
	for _, t := range swe.Entries {
		out.Entry = append(out.Entry, toChild(t, nil))
	}
	return out
}

// shareExpiry reads the optional Subsonic "expires" param (epoch millis) into a
// time pointer; absent or non-positive means no expiry.
func shareExpiry(r *http.Request) *time.Time {
	if exp := intParam(r, "expires", 0); exp > 0 {
		t := time.UnixMilli(int64(exp))
		return &t
	}
	return nil
}

func (h *Handler) handleGetShares(w http.ResponseWriter, r *http.Request) {
	shares, err := h.shareSvc.List(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		h.failInternal(w, r, err)
		return
	}
	resp := newResponse()
	out := &Shares{}
	for _, swe := range shares {
		out.Share = append(out.Share, h.toShare(r, swe))
	}
	resp.Shares = out
	write(w, r, resp)
}

func (h *Handler) handleCreateShare(w http.ResponseWriter, r *http.Request) {
	ids := r.Form["id"]
	if len(ids) == 0 {
		writeError(w, r, ErrMissingParameter, "Required parameter id is missing")
		return
	}
	swe, err := h.shareSvc.Create(r.Context(), userFrom(r.Context()).ID, ids[0], param(r, "description"), shareExpiry(r))
	if err != nil {
		h.failInternal(w, r, err)
		return
	}
	resp := newResponse()
	resp.Shares = &Shares{Share: []Share{h.toShare(r, swe)}}
	write(w, r, resp)
}

func (h *Handler) handleUpdateShare(w http.ResponseWriter, r *http.Request) {
	id := param(r, "id")
	if id == "" {
		writeError(w, r, ErrMissingParameter, "Required parameter id is missing")
		return
	}
	if err := h.shareSvc.Update(r.Context(), id, userFrom(r.Context()).ID, param(r, "description"), shareExpiry(r)); err != nil {
		h.failInternal(w, r, err)
		return
	}
	writeOK(w, r)
}

func (h *Handler) handleDeleteShare(w http.ResponseWriter, r *http.Request) {
	if err := h.shareSvc.Delete(r.Context(), param(r, "id"), userFrom(r.Context()).ID); err != nil {
		h.failInternal(w, r, err)
		return
	}
	writeOK(w, r)
}
