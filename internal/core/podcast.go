package core

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/podcastsearch"
)

// Podcast lifecycle status values (Subsonic semantics).
const (
	podcastNew         = "new"
	podcastCompleted   = "completed"
	podcastError       = "error"
	podcastDownloading = "downloading"
	episodeSkipped     = "skipped" // discovered but not downloaded
)

// maxEpisodeBytes caps a downloaded episode file (500 MiB).
const maxEpisodeBytes = 500 << 20

// PodcastService manages podcast channels and episodes: subscribing to an RSS
// feed, refreshing it into episode rows, and downloading episode audio to disk
// for local streaming.
//
// ponytail: refresh and download run synchronously in the request. Fine for a
// handful of feeds; move to a background worker if channel/episode counts grow.
type PodcastService struct {
	repo      *persistence.PodcastRepo
	dir       string
	client    *http.Client
	providers []podcastsearch.Provider
	logger    *slog.Logger
}

// NewPodcastService builds the service; dir is where episode audio is stored.
func NewPodcastService(repo *persistence.PodcastRepo, dir string, logger *slog.Logger) *PodcastService {
	client := &http.Client{Timeout: 60 * time.Second}
	return &PodcastService{repo: repo, dir: dir, client: client, providers: podcastsearch.Builtins(client), logger: logger}
}

// Channels lists all subscribed channels; when withEpisodes is set each channel
// is populated with its episodes (newest first).
func (s *PodcastService) Channels(ctx context.Context, withEpisodes bool) ([]models.PodcastChannel, error) {
	chans, err := s.repo.ListChannels(ctx)
	if err != nil {
		return nil, err
	}
	if withEpisodes {
		for i := range chans {
			eps, err := s.repo.ListEpisodes(ctx, chans[i].ID)
			if err != nil {
				return nil, err
			}
			chans[i].Episodes = eps
		}
	}
	return chans, nil
}

// Channel returns one channel with its episodes.
func (s *PodcastService) Channel(ctx context.Context, id string) (models.PodcastChannel, error) {
	c, err := s.repo.GetChannel(ctx, id)
	if err != nil {
		return c, err
	}
	c.Episodes, err = s.repo.ListEpisodes(ctx, id)
	return c, err
}

// NewestEpisodes returns the most recent episodes across all channels.
func (s *PodcastService) NewestEpisodes(ctx context.Context, count int) ([]models.PodcastEpisode, error) {
	if count <= 0 {
		count = 20
	}
	return s.repo.NewestEpisodes(ctx, count)
}

// Episode returns a single episode by id.
func (s *PodcastService) Episode(ctx context.Context, id string) (models.PodcastEpisode, error) {
	return s.repo.GetEpisode(ctx, id)
}

// --- directory search across the built-in adapters ---

// ProviderStatus is one built-in adapter's admin view: its declared config
// fields plus the current enabled flag and (non-secret) stored values.
type ProviderStatus struct {
	Name        string                      `json:"name"`
	DisplayName string                      `json:"displayName"`
	Enabled     bool                        `json:"enabled"`
	Fields      []podcastsearch.ConfigField `json:"fields"`
	// Config echoes the stored values, with secret fields masked (the UI knows a
	// secret is set from `configured` but never receives the value back).
	Config     map[string]string `json:"config"`
	Configured bool              `json:"configured"`
}

// defaultEnabled lists adapters that ship enabled out of the box (no-auth ones).
var defaultEnabled = map[string]bool{"itunes": true}

// EnsureDefaults seeds the default-on providers on first boot. It only inserts a
// row when none exists for that provider, so an admin who later disables one is
// not overridden on the next restart.
func (s *PodcastService) EnsureDefaults(ctx context.Context) error {
	stored, err := s.repo.ProviderConfigs(ctx)
	if err != nil {
		return err
	}
	for _, p := range s.providers {
		if !defaultEnabled[p.Name()] {
			continue
		}
		if _, ok := stored[p.Name()]; ok {
			continue // admin already decided for this provider
		}
		if err := s.repo.SaveProviderConfig(ctx, models.PodcastProviderConfig{
			Name: p.Name(), Enabled: true, Config: map[string]string{}, UpdatedAt: time.Now(),
		}); err != nil {
			return err
		}
	}
	return nil
}

// Providers lists every built-in adapter merged with its stored config, for the
// admin enable/disable + credentials screen.
func (s *PodcastService) Providers(ctx context.Context) ([]ProviderStatus, error) {
	stored, err := s.repo.ProviderConfigs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ProviderStatus, 0, len(s.providers))
	for _, p := range s.providers {
		cfg := stored[p.Name()]
		fields := p.ConfigFields()
		safe := map[string]string{}
		configured := true
		for _, f := range fields {
			v := cfg.Config[f.Key]
			if f.Required && v == "" {
				configured = false
			}
			if f.Secret {
				continue // never echo secrets back to the client
			}
			safe[f.Key] = v
		}
		out = append(out, ProviderStatus{
			Name: p.Name(), DisplayName: p.DisplayName(), Enabled: cfg.Enabled,
			Fields: fields, Config: safe, Configured: configured,
		})
	}
	return out, nil
}

// SetProvider enables/disables an adapter and stores its credentials. Empty
// values in config leave the stored secret untouched (so the admin can toggle
// enabled without re-entering an API secret the UI never received back).
func (s *PodcastService) SetProvider(ctx context.Context, name string, enabled bool, config map[string]string) error {
	var prov podcastsearch.Provider
	for _, p := range s.providers {
		if p.Name() == name {
			prov = p
			break
		}
	}
	if prov == nil {
		return persistence.ErrNotFound
	}
	stored, err := s.repo.ProviderConfigs(ctx)
	if err != nil {
		return err
	}
	merged := map[string]string{}
	for k, v := range stored[name].Config {
		merged[k] = v
	}
	for k, v := range config {
		if v != "" {
			merged[k] = v
		}
	}
	if enabled {
		for _, f := range prov.ConfigFields() {
			if f.Required && merged[f.Key] == "" {
				return fmt.Errorf("%s requires %s", prov.DisplayName(), f.Label)
			}
		}
	}
	return s.repo.SaveProviderConfig(ctx, models.PodcastProviderConfig{
		Name: name, Enabled: enabled, Config: merged, UpdatedAt: time.Now(),
	})
}

// Search queries every enabled adapter and returns the merged, feed-deduplicated
// results. A failing adapter is logged and skipped, not fatal.
func (s *PodcastService) Search(ctx context.Context, query string) ([]podcastsearch.Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("search query is required")
	}
	stored, err := s.repo.ProviderConfigs(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []podcastsearch.Result{}
	for _, p := range s.providers {
		cfg := stored[p.Name()]
		if !cfg.Enabled {
			continue
		}
		results, err := p.Search(ctx, query, cfg.Config)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("podcast search failed", "provider", p.Name(), "error", err)
			}
			continue
		}
		for _, r := range results {
			if seen[r.FeedURL] {
				continue
			}
			seen[r.FeedURL] = true
			out = append(out, r)
		}
	}
	return out, nil
}

// CreateChannel subscribes to a feed URL and refreshes it immediately so its
// metadata and episode list are populated before the call returns.
func (s *PodcastService) CreateChannel(ctx context.Context, url string) (models.PodcastChannel, error) {
	url = strings.TrimSpace(url)
	if !strings.HasPrefix(url, "http") {
		return models.PodcastChannel{}, fmt.Errorf("podcast feed url must be http(s)")
	}
	now := time.Now()
	ch := models.PodcastChannel{ID: persistence.NewPodcastID(), URL: url, Status: podcastNew, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.CreateChannel(ctx, ch); err != nil {
		return ch, err
	}
	s.refresh(ctx, &ch)
	return s.repo.GetChannel(ctx, ch.ID)
}

// DeleteChannel removes a channel, its episodes and any downloaded files.
func (s *PodcastService) DeleteChannel(ctx context.Context, id string) error {
	eps, err := s.repo.ListEpisodes(ctx, id)
	if err != nil {
		return err
	}
	for _, e := range eps {
		if e.MediaPath != "" {
			_ = os.Remove(e.MediaPath)
		}
	}
	return s.repo.DeleteChannel(ctx, id)
}

// DeleteEpisode removes an episode and its downloaded file.
func (s *PodcastService) DeleteEpisode(ctx context.Context, id string) error {
	e, err := s.repo.GetEpisode(ctx, id)
	if err != nil {
		return err
	}
	if e.MediaPath != "" {
		_ = os.Remove(e.MediaPath)
	}
	return s.repo.DeleteEpisode(ctx, id)
}

// RefreshAll re-fetches every channel's feed, importing new episodes.
func (s *PodcastService) RefreshAll(ctx context.Context) error {
	chans, err := s.repo.ListChannels(ctx)
	if err != nil {
		return err
	}
	for i := range chans {
		s.refresh(ctx, &chans[i])
	}
	return nil
}

// refresh fetches the channel feed and imports any not-yet-seen episodes,
// updating the channel's status/metadata. Errors are recorded on the channel
// (status "error") rather than returned, so one bad feed doesn't abort a batch.
func (s *PodcastService) refresh(ctx context.Context, ch *models.PodcastChannel) {
	feed, err := s.fetchFeed(ctx, ch.URL)
	ch.UpdatedAt = time.Now()
	if err != nil {
		ch.Status = podcastError
		ch.Error = err.Error()
		_ = s.repo.UpdateChannel(ctx, *ch)
		if s.logger != nil {
			s.logger.Warn("podcast refresh failed", "url", ch.URL, "error", err)
		}
		return
	}
	if feed.Channel.Title != "" {
		ch.Title = feed.Channel.Title
	}
	ch.Description = feed.Channel.Description
	if img := feed.image(); img != "" {
		ch.CoverArt = img
	}
	ch.Status = podcastCompleted
	ch.Error = ""
	_ = s.repo.UpdateChannel(ctx, *ch)

	for _, item := range feed.Channel.Items {
		if item.Enclosure.URL == "" {
			continue // not a media episode
		}
		guid := item.GUID
		if guid == "" {
			guid = item.Enclosure.URL
		}
		exists, err := s.repo.EpisodeExists(ctx, ch.ID, guid)
		if err != nil || exists {
			continue
		}
		now := time.Now()
		ep := models.PodcastEpisode{
			ID: persistence.NewPodcastID(), ChannelID: ch.ID, GUID: guid,
			Title: item.Title, Description: item.Description, PublishDate: parsePubDate(item.PubDate),
			Duration: parseDuration(item.Duration), Size: item.Enclosure.Length,
			ContentType: item.Enclosure.Type, Suffix: suffixFor(item.Enclosure.URL, item.Enclosure.Type),
			StreamURL: item.Enclosure.URL, Status: episodeSkipped, CreatedAt: now, UpdatedAt: now,
		}
		if err := s.repo.CreateEpisode(ctx, ep); err != nil && s.logger != nil {
			s.logger.Warn("podcast episode insert failed", "title", ep.Title, "error", err)
		}
	}
}

// DownloadEpisode fetches an episode's audio to disk, making it streamable.
func (s *PodcastService) DownloadEpisode(ctx context.Context, id string) error {
	ep, err := s.repo.GetEpisode(ctx, id)
	if err != nil {
		return err
	}
	if ep.Status == podcastCompleted && ep.MediaPath != "" {
		if _, err := os.Stat(ep.MediaPath); err == nil {
			return nil // already downloaded
		}
	}
	ep.Status = podcastDownloading
	ep.UpdatedAt = time.Now()
	_ = s.repo.UpdateEpisode(ctx, ep)

	path := filepath.Join(s.dir, ep.ID+"."+ep.Suffix)
	size, err := s.download(ctx, ep.StreamURL, path)
	if err != nil {
		ep.Status = podcastError
		ep.UpdatedAt = time.Now()
		_ = s.repo.UpdateEpisode(ctx, ep)
		return err
	}
	ep.MediaPath = path
	ep.Size = size
	ep.Status = podcastCompleted
	ep.UpdatedAt = time.Now()
	return s.repo.UpdateEpisode(ctx, ep)
}

// download streams url to path, enforcing the size cap. Returns the byte count.
func (s *PodcastService) download(ctx context.Context, url, path string) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "immerle")
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("episode fetch %s: %s", url, resp.Status)
	}
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	n, err := io.Copy(f, io.LimitReader(resp.Body, maxEpisodeBytes))
	if err != nil {
		_ = os.Remove(path)
		return 0, err
	}
	return n, nil
}

// --- feed parsing (stdlib encoding/xml; minimal RSS + iTunes namespace) ---

type rssFeed struct {
	Channel struct {
		Title       string `xml:"title"`
		Description string `xml:"description"`
		// Both the RSS <image><url> and <itunes:image href> share the local name
		// "image"; encoding/xml matches on local name, so collect both kinds here.
		Images []struct {
			URL  string `xml:"url"`
			Href string `xml:"href,attr"`
		} `xml:"image"`
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

// image returns the channel artwork from whichever <image> form the feed used.
func (f *rssFeed) image() string {
	for _, img := range f.Channel.Images {
		if img.Href != "" {
			return img.Href
		}
		if img.URL != "" {
			return img.URL
		}
	}
	return ""
}

type rssItem struct {
	Title       string `xml:"title"`
	Description string `xml:"description"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	Duration    string `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd duration"`
	Enclosure   struct {
		URL    string `xml:"url,attr"`
		Length int64  `xml:"length,attr"`
		Type   string `xml:"type,attr"`
	} `xml:"enclosure"`
}

func (s *PodcastService) fetchFeed(ctx context.Context, url string) (*rssFeed, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "immerle")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("feed fetch %s: %s", url, resp.Status)
	}
	var feed rssFeed
	dec := xml.NewDecoder(io.LimitReader(resp.Body, 16<<20))
	// ponytail: pass non-UTF-8 charsets through unchanged (UTF-8 feeds are the
	// norm). Add golang.org/x/net/html/charset if Latin-1 feeds mangle accents.
	dec.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil }
	if err := dec.Decode(&feed); err != nil {
		return nil, fmt.Errorf("parse feed: %w", err)
	}
	return &feed, nil
}

// parsePubDate parses an RFC-822 RSS date, tolerating the common variants.
func parsePubDate(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, "Mon, 2 Jan 2006 15:04:05 -0700", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// parseDuration parses an iTunes duration: "HH:MM:SS", "MM:SS" or plain seconds.
func parseDuration(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if !strings.Contains(s, ":") {
		n, _ := strconv.Atoi(s)
		return n
	}
	parts := strings.Split(s, ":")
	total := 0
	for _, p := range parts {
		n, _ := strconv.Atoi(p)
		total = total*60 + n
	}
	return total
}

// suffixFor derives a file extension from the enclosure URL or MIME type.
func suffixFor(url, mime string) string {
	if ext := strings.TrimPrefix(filepath.Ext(strings.Split(url, "?")[0]), "."); ext != "" && len(ext) <= 4 {
		return strings.ToLower(ext)
	}
	switch mime {
	case "audio/mpeg":
		return "mp3"
	case "audio/mp4", "audio/x-m4a":
		return "m4a"
	case "audio/ogg":
		return "ogg"
	default:
		return "mp3"
	}
}
