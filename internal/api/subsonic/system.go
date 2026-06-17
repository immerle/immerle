package subsonic

import "net/http"

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
	resp := newResponse()
	_, _, tracks, _ := h.Catalog.Stats(r.Context())
	resp.ScanStatus = &ScanStatus{Scanning: false, Count: tracks}
	write(w, r, resp)
}

func (h *Handler) handleStartScan(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) {
		return
	}
	if h.Scanner != nil && len(h.MusicFolderPaths) > 0 {
		go func() {
			_, _ = h.Scanner.ScanPaths(contextDetached(), h.MusicFolderPaths)
		}()
	}
	resp := newResponse()
	resp.ScanStatus = &ScanStatus{Scanning: true}
	write(w, r, resp)
}
