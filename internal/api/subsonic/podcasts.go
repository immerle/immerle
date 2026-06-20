package subsonic

import (
	"net/http"

	"github.com/immerle/immerle/internal/models"
)

// toPodcastEpisode maps a domain episode to its Subsonic wire form. The stream
// id is only exposed once the episode has been downloaded (otherwise there is
// nothing to stream).
func toPodcastEpisode(e models.PodcastEpisode) PodcastEpisode {
	out := PodcastEpisode{
		Child: Child{
			ID: e.ID, Parent: e.ChannelID, IsDir: false, Title: e.Title, Type: "podcast",
			Size: e.Size, ContentType: e.ContentType, Suffix: e.Suffix, Duration: e.Duration, BitRate: e.BitRate,
			Created: formatTime(e.CreatedAt),
		},
		ChannelID:   e.ChannelID,
		Description: e.Description,
		Status:      e.Status,
		PublishDate: formatTime(e.PublishDate),
	}
	if e.Status == "completed" {
		out.StreamID = e.ID
	}
	return out
}

func toPodcastChannel(c models.PodcastChannel, includeEpisodes bool) PodcastChannel {
	out := PodcastChannel{
		ID: c.ID, URL: c.URL, Title: c.Title, Description: c.Description,
		OriginalImageURL: c.CoverArt, Status: c.Status, ErrorMessage: c.Error,
	}
	if includeEpisodes {
		for _, e := range c.Episodes {
			out.Episode = append(out.Episode, toPodcastEpisode(e))
		}
	}
	return out
}

func (h *Handler) handleGetPodcasts(w http.ResponseWriter, r *http.Request) {
	if h.Podcasts == nil {
		writeError(w, r, ErrGeneric, "podcasts not available")
		return
	}
	includeEpisodes := boolParam(r, "includeEpisodes", true)
	resp := newResponse()
	out := &Podcasts{}
	if id := param(r, "id"); id != "" {
		ch, err := h.Podcasts.Channel(r.Context(), id)
		if err != nil {
			h.writeServiceError(w, r, err, "Channel not found")
			return
		}
		out.Channel = []PodcastChannel{toPodcastChannel(ch, includeEpisodes)}
	} else {
		chans, err := h.Podcasts.Channels(r.Context(), includeEpisodes)
		if err != nil {
			h.failInternal(w, r, err)
			return
		}
		for _, ch := range chans {
			out.Channel = append(out.Channel, toPodcastChannel(ch, includeEpisodes))
		}
	}
	resp.Podcasts = out
	write(w, r, resp)
}

func (h *Handler) handleGetNewestPodcasts(w http.ResponseWriter, r *http.Request) {
	if h.Podcasts == nil {
		writeError(w, r, ErrGeneric, "podcasts not available")
		return
	}
	eps, err := h.Podcasts.NewestEpisodes(r.Context(), intParam(r, "count", 20))
	if err != nil {
		h.failInternal(w, r, err)
		return
	}
	resp := newResponse()
	out := &NewestPodcasts{}
	for _, e := range eps {
		out.Episode = append(out.Episode, toPodcastEpisode(e))
	}
	resp.NewestPodcasts = out
	write(w, r, resp)
}

func (h *Handler) handleGetPodcastEpisode(w http.ResponseWriter, r *http.Request) {
	if h.Podcasts == nil {
		writeError(w, r, ErrGeneric, "podcasts not available")
		return
	}
	ep, err := h.Podcasts.Episode(r.Context(), param(r, "id"))
	if err != nil {
		h.writeServiceError(w, r, err, "Episode not found")
		return
	}
	resp := newResponse()
	episode := toPodcastEpisode(ep)
	resp.PodcastEpisode = &episode
	write(w, r, resp)
}

// handleRefreshPodcasts re-fetches every channel feed (admin only).
func (h *Handler) handleRefreshPodcasts(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) || h.Podcasts == nil {
		if h.Podcasts == nil {
			writeError(w, r, ErrGeneric, "podcasts not available")
		}
		return
	}
	if err := h.Podcasts.RefreshAll(r.Context()); err != nil {
		h.failInternal(w, r, err)
		return
	}
	writeOK(w, r)
}

// handleCreatePodcastChannel subscribes to a feed URL (admin only).
func (h *Handler) handleCreatePodcastChannel(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) || h.Podcasts == nil {
		if h.Podcasts == nil {
			writeError(w, r, ErrGeneric, "podcasts not available")
		}
		return
	}
	url := param(r, "url")
	if url == "" {
		writeError(w, r, ErrMissingParameter, "Required parameter url is missing")
		return
	}
	if _, err := h.Podcasts.CreateChannel(r.Context(), url); err != nil {
		h.failInternal(w, r, err)
		return
	}
	writeOK(w, r)
}

// handleDeletePodcastChannel removes a channel (admin only).
func (h *Handler) handleDeletePodcastChannel(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) || h.Podcasts == nil {
		if h.Podcasts == nil {
			writeError(w, r, ErrGeneric, "podcasts not available")
		}
		return
	}
	if err := h.Podcasts.DeleteChannel(r.Context(), param(r, "id")); err != nil {
		h.failInternal(w, r, err)
		return
	}
	writeOK(w, r)
}

// handleDeletePodcastEpisode removes a single episode (admin only).
func (h *Handler) handleDeletePodcastEpisode(w http.ResponseWriter, r *http.Request) {
	if !requireAdmin(w, r) || h.Podcasts == nil {
		if h.Podcasts == nil {
			writeError(w, r, ErrGeneric, "podcasts not available")
		}
		return
	}
	if err := h.Podcasts.DeleteEpisode(r.Context(), param(r, "id")); err != nil {
		h.writeServiceError(w, r, err, "Episode not found")
		return
	}
	writeOK(w, r)
}

// handleDownloadPodcastEpisode fetches an episode's audio to disk so it becomes
// streamable (any authenticated user, matching Subsonic's podcast role).
func (h *Handler) handleDownloadPodcastEpisode(w http.ResponseWriter, r *http.Request) {
	if h.Podcasts == nil {
		writeError(w, r, ErrGeneric, "podcasts not available")
		return
	}
	if err := h.Podcasts.DownloadEpisode(r.Context(), param(r, "id")); err != nil {
		h.writeServiceError(w, r, err, "Episode not found")
		return
	}
	writeOK(w, r)
}
