// Package concerts finds upcoming shows for each user's top-listened artists
// near the single, admin-chosen country for the instance, refreshed daily.
// Every configured source is searched on every sync (not stopped at the
// first match) — a global source finding something unrelated must not
// starve a country-specific one. See providers below for the source list.
package concerts

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/eventim"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/skiddle"
	"github.com/immerle/immerle/internal/ticketmaster"
)

const defaultInterval = 24 * time.Hour

// topArtistsWindow bounds how far back "top-listened" looks; topArtistsLimit
// bounds how many artists get searched per user per sync.
const topArtistsWindow = 180 * 24 * time.Hour
const topArtistsLimit = 10

// maxEventsPerArtist caps how many upcoming shows are kept per artist match,
// per source.
const maxEventsPerArtist = 3

// foundEvent unifies ticketmaster.Event, skiddle.Event, and eventim.Event
// (same shape, three packages) so the matching loop below doesn't care which
// source found it.
type foundEvent struct {
	id, name, url, venue, city string
	startTime                  time.Time
}

// searcher is implemented by *ticketmaster.Client, *skiddle.Client, and
// *eventim.Client (via small adapters below). Returning no events for an
// unconfigured/unmatched/out-of-coverage search is not an error — see each
// client's Search doc. country is an ISO 3166-1 alpha-2 code (e.g. "FR").
type searcher interface {
	Search(ctx context.Context, artist, country string, limit int) ([]foundEvent, error)
}

type ticketmasterSearcher struct{ c *ticketmaster.Client }

func (s ticketmasterSearcher) Search(ctx context.Context, artist, country string, limit int) ([]foundEvent, error) {
	events, err := s.c.Search(ctx, artist, country, limit)
	if err != nil {
		return nil, err
	}
	out := make([]foundEvent, len(events))
	for i, e := range events {
		out[i] = foundEvent{id: e.ID, name: e.Name, url: e.URL, venue: e.Venue, city: e.City, startTime: e.StartTime}
	}
	return out, nil
}

type skiddleSearcher struct{ c *skiddle.Client }

func (s skiddleSearcher) Search(ctx context.Context, artist, country string, limit int) ([]foundEvent, error) {
	events, err := s.c.Search(ctx, artist, country, limit)
	if err != nil {
		return nil, err
	}
	out := make([]foundEvent, len(events))
	for i, e := range events {
		out[i] = foundEvent{id: e.ID, name: e.Name, url: e.URL, venue: e.Venue, city: e.City, startTime: e.StartTime}
	}
	return out, nil
}

type eventimSearcher struct{ c *eventim.Client }

func (s eventimSearcher) Search(ctx context.Context, artist, country string, limit int) ([]foundEvent, error) {
	events, err := s.c.Search(ctx, artist, country, limit)
	if err != nil {
		return nil, err
	}
	out := make([]foundEvent, len(events))
	for i, e := range events {
		out[i] = foundEvent{id: e.ID, name: e.Name, url: e.URL, venue: e.Venue, city: e.City, startTime: e.StartTime}
	}
	return out, nil
}

// provider names one concert-discovery source. new builds a fresh searcher
// from the live config on every sync (settings are hot-reloadable, and these
// clients are cheap stateless wrappers).
type provider struct {
	name string
	new  func(cfg models.ConcertsRuntime) searcher
}

// defaultProviders lists every concert-discovery source. Adding a new one
// (global or country-specific — a source decides its own coverage inside
// Search, same as Eventim's France-only check) is a single entry here; no
// other code needs to change.
func defaultProviders() []provider {
	return []provider{
		{name: "ticketmaster", new: func(cfg models.ConcertsRuntime) searcher {
			return ticketmasterSearcher{ticketmaster.NewClient(cfg.TicketmasterAPIKey)}
		}},
		{name: "skiddle", new: func(cfg models.ConcertsRuntime) searcher {
			return skiddleSearcher{skiddle.NewClient(cfg.SkiddleAPIKey)}
		}},
		{name: "eventim", new: func(cfg models.ConcertsRuntime) searcher {
			return eventimSearcher{eventim.NewClient()}
		}},
	}
}

// Service syncs concert matches for every user, near the single instance-wide
// country configured by an admin.
type Service struct {
	users     *persistence.UserRepo
	wrapped   *persistence.WrappedRepo
	concerts  *persistence.ConcertRepo
	settings  func() models.ConcertsRuntime
	interval  time.Duration
	logger    *slog.Logger
	providers []provider
}

// New builds a Service. settings supplies the live, hot-reloadable
// Enabled/Country/API-key configuration (typically SettingsService.ConcertsConfig).
func New(users *persistence.UserRepo, wrapped *persistence.WrappedRepo, concerts *persistence.ConcertRepo,
	settings func() models.ConcertsRuntime, logger *slog.Logger) *Service {
	return &Service{
		users: users, wrapped: wrapped, concerts: concerts, settings: settings,
		interval: defaultInterval, logger: logger,
		providers: defaultProviders(),
	}
}

// SyncNow searches every user's top-listened artists for upcoming shows in
// the configured country, across every source, and upserts any new matches,
// returning how many were newly added (one user/artist/source failing is
// logged and skipped, not fatal). A no-op (0, nil) when the feature is
// disabled or no country is set.
func (s *Service) SyncNow(ctx context.Context) (int, error) {
	cfg := s.settings()
	if !cfg.Enabled || cfg.Country == "" {
		return 0, nil
	}
	searchers := make([]struct {
		name string
		s    searcher
	}, len(s.providers))
	for i, p := range s.providers {
		searchers[i].name = p.name
		searchers[i].s = p.new(cfg)
	}

	users, err := s.users.List(ctx)
	if err != nil {
		return 0, err
	}
	now := time.Now()
	windowStart := now.Add(-topArtistsWindow)
	synced := 0
	for _, u := range users {
		artists, err := s.wrapped.TopArtists(ctx, u.ID, windowStart.UnixMilli(), now.UnixMilli(), topArtistsLimit)
		if err != nil {
			s.logger.Warn("concerts: top artists failed", "user", u.ID, "error", err)
			continue
		}
		for _, artist := range artists {
			for _, src := range searchers {
				synced += s.syncArtistFromSource(ctx, src.s, src.name, u.ID, cfg.Country, artist.Name, now)
			}
		}
	}
	return synced, nil
}

// syncArtistFromSource searches one artist for one user against a single
// source and upserts any new, still-upcoming matches.
func (s *Service) syncArtistFromSource(ctx context.Context, src searcher, sourceName, userID, country, artist string, now time.Time) int {
	events, err := src.Search(ctx, artist, country, maxEventsPerArtist)
	if err != nil {
		s.logger.Warn("concerts: search failed", "source", sourceName, "artist", artist, "error", err)
		return 0
	}
	synced := 0
	for _, e := range events {
		if e.id == "" || e.startTime.Before(now) {
			continue
		}
		created, err := s.concerts.Upsert(ctx, models.Concert{
			ID: uuid.NewString(), UserID: userID, Source: sourceName, SourceEventID: e.id,
			ArtistName: artist, EventName: e.name, Venue: e.venue, City: e.city,
			StartTime: e.startTime, URL: e.url,
		})
		if err != nil {
			s.logger.Warn("concerts: upsert failed", "artist", artist, "error", err)
			continue
		}
		if created {
			synced++
		}
	}
	return synced
}

// Run syncs on the configured interval until ctx is cancelled — once
// immediately, then every interval (daily by default).
func (s *Service) Run(ctx context.Context) {
	if _, err := s.SyncNow(ctx); err != nil {
		s.logger.Warn("concerts: initial sync failed", "error", err)
	}
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := s.SyncNow(ctx); err != nil {
				s.logger.Warn("concerts: sync failed", "error", err)
			}
		}
	}
}
