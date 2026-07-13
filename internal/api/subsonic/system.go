package subsonic

import (
	"context"
	"net/http"
)

func (h *Handler) handlePing(w http.ResponseWriter, r *http.Request) {
	writeOK(w, r)
}

func (h *Handler) handleGetLicense(w http.ResponseWriter, r *http.Request) {
	resp := newResponse()
	resp.License = &License{Valid: true}
	write(w, r, resp)
}

func (h *Handler) handleGetOpenSubsonicExtensions(w http.ResponseWriter, r *http.Request) {
	resp := newResponse()
	resp.OpenSubsonicExtensions = []OpenSubsonicExtension{
		{Name: "transcodeOffset", Versions: []int{1}},
		{Name: "formPost", Versions: []int{1}},
		{Name: "songLyrics", Versions: []int{1}},
	}
	write(w, r, resp)
}

func (h *Handler) handleGetScanStatus(w http.ResponseWriter, r *http.Request) {
	_, _, tracks, err := h.Catalog.Stats(r.Context())
	if err != nil {
		h.failInternal(w, r, err)
		return
	}
	resp := newResponse()
	resp.ScanStatus = &ScanStatus{Scanning: h.Scanner != nil && h.Scanner.Scanning(), Count: tracks}
	write(w, r, resp)
}

func (h *Handler) handleStartScan(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	started := h.Scanner != nil && len(h.MusicFolderPaths) > 0
	if started {
		go func() {
			// Detached background context: the scan must outlive this request.
			_, _ = h.Scanner.ScanPaths(context.Background(), h.MusicFolderPaths)
		}()
	}
	resp := newResponse()
	resp.ScanStatus = &ScanStatus{Scanning: started}
	write(w, r, resp)
}
