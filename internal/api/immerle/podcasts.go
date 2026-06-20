package immerle

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

type channelView struct {
	ID          string        `json:"id"`
	URL         string        `json:"url"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	ImageURL    string        `json:"imageUrl"`
	Status      string        `json:"status"`
	Error       string        `json:"error,omitempty"`
	Episodes    []episodeView `json:"episodes,omitempty"`
}

type episodeView struct {
	ID          string    `json:"id"`
	ChannelID   string    `json:"channelId"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	PublishDate time.Time `json:"publishDate"`
	Duration    int       `json:"duration"`
	Size        int64     `json:"size"`
	Suffix      string    `json:"suffix"`
	ContentType string    `json:"contentType"`
	Status      string    `json:"status"`
	// Streamable is true once the episode audio has been downloaded.
	Streamable bool `json:"streamable"`
}

func toEpisodeView(e models.PodcastEpisode) episodeView {
	return episodeView{
		ID: e.ID, ChannelID: e.ChannelID, Title: e.Title, Description: e.Description,
		PublishDate: e.PublishDate, Duration: e.Duration, Size: e.Size, Suffix: e.Suffix,
		ContentType: e.ContentType, Status: e.Status, Streamable: e.Status == "completed",
	}
}

func toChannelView(c models.PodcastChannel) channelView {
	v := channelView{
		ID: c.ID, URL: c.URL, Title: c.Title, Description: c.Description,
		ImageURL: c.CoverArt, Status: c.Status, Error: c.Error,
	}
	for _, e := range c.Episodes {
		v.Episodes = append(v.Episodes, toEpisodeView(e))
	}
	return v
}

func (h *Handler) podcastsReady(w http.ResponseWriter) bool {
	if h.Podcasts == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "podcasts not available")
		return false
	}
	return true
}

// handlePodcastList lists subscribed channels with their episodes.
//
// @Summary      List podcast channels
// @Tags         podcasts
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Router       /podcasts [get]
func (h *Handler) handlePodcastList(w http.ResponseWriter, r *http.Request) {
	if !h.podcastsReady(w) {
		return
	}
	chans, err := h.Podcasts.Channels(r.Context(), true)
	if err != nil {
		writeInternal(w, err)
		return
	}
	views := make([]channelView, 0, len(chans))
	for _, c := range chans {
		views = append(views, toChannelView(c))
	}
	writeResource(w, http.StatusOK, map[string]any{"channels": views})
}

// handlePodcastGet returns one channel with its episodes.
//
// @Summary      Get a podcast channel
// @Tags         podcasts
// @Security     BearerAuth
// @Param        id  path  string  true  "Channel id"
// @Success      200  {object}  channelView
// @Failure      404  {object}  errorResponse
// @Router       /podcasts/{id} [get]
func (h *Handler) handlePodcastGet(w http.ResponseWriter, r *http.Request) {
	if !h.podcastsReady(w) {
		return
	}
	ch, err := h.Podcasts.Channel(r.Context(), pathParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "channel not found")
		return
	}
	writeResource(w, http.StatusOK, toChannelView(ch))
}

// handlePodcastNewest returns the most recent episodes across all channels.
//
// @Summary      Newest podcast episodes
// @Tags         podcasts
// @Security     BearerAuth
// @Success      200  {object}  map[string]interface{}
// @Router       /podcasts/episodes/newest [get]
func (h *Handler) handlePodcastNewest(w http.ResponseWriter, r *http.Request) {
	if !h.podcastsReady(w) {
		return
	}
	count := 20
	if n, err := strconv.Atoi(r.URL.Query().Get("count")); err == nil && n > 0 {
		count = n
	}
	eps, err := h.Podcasts.NewestEpisodes(r.Context(), count)
	if err != nil {
		writeInternal(w, err)
		return
	}
	views := make([]episodeView, 0, len(eps))
	for _, e := range eps {
		views = append(views, toEpisodeView(e))
	}
	writeResource(w, http.StatusOK, map[string]any{"episodes": views})
}

// handlePodcastEpisode returns one episode.
//
// @Summary      Get a podcast episode
// @Tags         podcasts
// @Security     BearerAuth
// @Param        id  path  string  true  "Episode id"
// @Success      200  {object}  episodeView
// @Failure      404  {object}  errorResponse
// @Router       /podcasts/episodes/{id} [get]
func (h *Handler) handlePodcastEpisode(w http.ResponseWriter, r *http.Request) {
	if !h.podcastsReady(w) {
		return
	}
	ep, err := h.Podcasts.Episode(r.Context(), pathParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "episode not found")
		return
	}
	writeResource(w, http.StatusOK, toEpisodeView(ep))
}

// handlePodcastStream serves a downloaded episode's audio file.
//
// @Summary      Stream a podcast episode
// @Tags         podcasts
// @Security     BearerAuth
// @Param        id  path  string  true  "Episode id"
// @Success      200
// @Failure      404  {object}  errorResponse
// @Router       /podcasts/episodes/{id}/stream [get]
func (h *Handler) handlePodcastStream(w http.ResponseWriter, r *http.Request) {
	if !h.podcastsReady(w) {
		return
	}
	ep, err := h.Podcasts.Episode(r.Context(), pathParam(r, "id"))
	if err != nil || ep.MediaPath == "" || ep.Status != "completed" {
		writeError(w, http.StatusNotFound, "not_found", "episode not downloaded")
		return
	}
	http.ServeFile(w, r, ep.MediaPath)
}

// handlePodcastDownload fetches an episode's audio so it becomes streamable.
//
// @Summary      Download a podcast episode
// @Tags         podcasts
// @Security     BearerAuth
// @Param        id  path  string  true  "Episode id"
// @Success      202  {object}  episodeView
// @Failure      404  {object}  errorResponse
// @Router       /podcasts/episodes/{id}/download [post]
func (h *Handler) handlePodcastDownload(w http.ResponseWriter, r *http.Request) {
	if !h.podcastsReady(w) {
		return
	}
	id := pathParam(r, "id")
	if err := h.Podcasts.DownloadEpisode(r.Context(), id); err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "not_found", "episode not found")
			return
		}
		writeInternal(w, err)
		return
	}
	ep, err := h.Podcasts.Episode(r.Context(), id)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusOK, toEpisodeView(ep))
}

// podcastCreateRequest is the admin subscribe body.
type podcastCreateRequest struct {
	URL string `json:"url"`
}

// handlePodcastCreate subscribes to a feed URL (admin only).
//
// @Summary      Subscribe to a podcast feed
// @Tags         admin
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  podcastCreateRequest  true  "Feed URL"
// @Success      201  {object}  channelView
// @Failure      400  {object}  errorResponse
// @Router       /admin/podcasts [post]
func (h *Handler) handlePodcastCreate(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) || !h.podcastsReady(w) {
		return
	}
	var req podcastCreateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ch, err := h.Podcasts.CreateChannel(r.Context(), req.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation", err.Error())
		return
	}
	writeResource(w, http.StatusCreated, toChannelView(ch))
}

// handlePodcastRefresh re-fetches every channel feed (admin only).
//
// @Summary      Refresh all podcast feeds
// @Tags         admin
// @Security     BearerAuth
// @Success      204
// @Router       /admin/podcasts/refresh [post]
func (h *Handler) handlePodcastRefresh(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) || !h.podcastsReady(w) {
		return
	}
	if err := h.Podcasts.RefreshAll(r.Context()); err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusNoContent, nil)
}

// handlePodcastDeleteChannel removes a channel and its episodes (admin only).
//
// @Summary      Delete a podcast channel
// @Tags         admin
// @Security     BearerAuth
// @Param        id  path  string  true  "Channel id"
// @Success      204
// @Router       /admin/podcasts/{id} [delete]
func (h *Handler) handlePodcastDeleteChannel(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) || !h.podcastsReady(w) {
		return
	}
	if err := h.Podcasts.DeleteChannel(r.Context(), pathParam(r, "id")); err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusNoContent, nil)
}

// handlePodcastDeleteEpisode removes a single episode (admin only).
//
// @Summary      Delete a podcast episode
// @Tags         admin
// @Security     BearerAuth
// @Param        id  path  string  true  "Episode id"
// @Success      204
// @Router       /admin/podcasts/episodes/{id} [delete]
func (h *Handler) handlePodcastDeleteEpisode(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) || !h.podcastsReady(w) {
		return
	}
	if err := h.Podcasts.DeleteEpisode(r.Context(), pathParam(r, "id")); err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "not_found", "episode not found")
			return
		}
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusNoContent, nil)
}

// --- directory search providers (admin) ---

// handlePodcastProviders lists the built-in directory adapters with their config
// fields and enabled state, for the admin enable/disable screen.
//
// @Summary      List podcast directory providers
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  map[string]interface{}
// @Router       /admin/podcasts/providers [get]
func (h *Handler) handlePodcastProviders(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) || !h.podcastsReady(w) {
		return
	}
	provs, err := h.Podcasts.Providers(r.Context())
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusOK, map[string]any{"providers": provs})
}

// podcastProviderRequest toggles a provider and (optionally) sets its credentials.
type podcastProviderRequest struct {
	Enabled bool              `json:"enabled"`
	Config  map[string]string `json:"config"`
}

// handlePodcastProviderUpdate enables/disables a directory adapter and stores
// its per-source credentials (admin only).
//
// @Summary      Configure a podcast directory provider
// @Tags         admin
// @Security     BearerAuth
// @Accept       json
// @Param        name  path  string                   true  "Provider name"
// @Param        body  body  podcastProviderRequest   true  "Enabled + config"
// @Success      204
// @Failure      400  {object}  errorResponse
// @Router       /admin/podcasts/providers/{name} [put]
func (h *Handler) handlePodcastProviderUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) || !h.podcastsReady(w) {
		return
	}
	var req podcastProviderRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := h.Podcasts.SetProvider(r.Context(), pathParam(r, "name"), req.Enabled, req.Config); err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "not_found", "provider not found")
			return
		}
		writeError(w, http.StatusBadRequest, "validation", err.Error())
		return
	}
	writeResource(w, http.StatusNoContent, nil)
}

// handlePodcastSearch searches the enabled directory providers for feeds to
// subscribe to (admin only — it is the discovery side of subscribing).
//
// @Summary      Search podcast directories
// @Tags         admin
// @Security     BearerAuth
// @Param        q  query  string  true  "Search terms"
// @Success      200  {object}  map[string]interface{}
// @Router       /admin/podcasts/search [get]
func (h *Handler) handlePodcastSearch(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) || !h.podcastsReady(w) {
		return
	}
	results, err := h.Podcasts.Search(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "validation", err.Error())
		return
	}
	writeResource(w, http.StatusOK, map[string]any{"results": results})
}

// isNotFound reports a repo ErrNotFound.
func isNotFound(err error) bool { return errors.Is(err, persistence.ErrNotFound) }
