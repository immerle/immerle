// Package concerts finds upcoming shows for each user's top-listened artists
// near their city, refreshed daily. Ticketmaster is searched first; Skiddle
// (a rougher, keyword-only match) is only tried when Ticketmaster has no key
// configured or found nothing for that artist.
package concerts

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

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

// maxEventsPerArtist caps how many upcoming shows are kept per artist match.
const maxEventsPerArtist = 3

// foundEvent unifies ticketmaster.Event and skiddle.Event (same shape, two
// packages) so the matching loop below doesn't care which source found it.
type foundEvent struct {
	id, name, url, venue, city string
	startTime                  time.Time
}

// searcher is implemented by *ticketmaster.Client and *skiddle.Client (via
// small adapters below). Returning no events for an unconfigured/unmatched
// search is not an error — see both clients' Search doc.
type searcher interface {
	Search(ctx context.Context, artist, city string, limit int) ([]foundEvent, error)
}

type ticketmasterSearcher struct{ c *ticketmaster.Client }

func (s ticketmasterSearcher) Search(ctx context.Context, artist, city string, limit int) ([]foundEvent, error) {
	events, err := s.c.Search(ctx, artist, city, limit)
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

func (s skiddleSearcher) Search(ctx context.Context, artist, city string, limit int) ([]foundEvent, error) {
	events, err := s.c.Search(ctx, artist, city, limit)
	if err != nil {
		return nil, err
	}
	out := make([]foundEvent, len(events))
	for i, e := range events {
		out[i] = foundEvent{id: e.ID, name: e.Name, url: e.URL, venue: e.Venue, city: e.City, startTime: e.StartTime}
	}
	return out, nil
}

// Service syncs concert matches for every user with a city set.
type Service struct {
	users    *persistence.UserRepo
	wrapped  *persistence.WrappedRepo
	concerts *persistence.ConcertRepo
	settings func() models.ConcertsRuntime
	interval time.Duration
	logger   *slog.Logger

	// newTicketmaster/newSkiddle build a fresh searcher from the live API key on
	// every sync (settings are hot-reloadable, and these clients are cheap
	// stateless wrappers) — overridden with fakes in tests.
	newTicketmaster func(apiKey string) searcher
	newSkiddle      func(apiKey string) searcher
}

// New builds a Service. settings supplies the live, hot-reloadable
// Enabled/API-key configuration (typically SettingsService.ConcertsConfig).
func New(users *persistence.UserRepo, wrapped *persistence.WrappedRepo, concerts *persistence.ConcertRepo,
	settings func() models.ConcertsRuntime, logger *slog.Logger) *Service {
	return &Service{
		users: users, wrapped: wrapped, concerts: concerts, settings: settings,
		interval: defaultInterval, logger: logger,
		newTicketmaster: func(apiKey string) searcher { return ticketmasterSearcher{ticketmaster.NewClient(apiKey)} },
		newSkiddle:      func(apiKey string) searcher { return skiddleSearcher{skiddle.NewClient(apiKey)} },
	}
}

// SyncNow searches every user-with-a-city's top-listened artists for nearby
// upcoming shows and upserts any new matches, returning how many were newly
// added (one user or artist failing is logged and skipped, not fatal).
// A no-op (0, nil) when the feature is disabled.
func (s *Service) SyncNow(ctx context.Context) (int, error) {
	cfg := s.settings()
	if !cfg.Enabled {
		return 0, nil
	}
	tm := s.newTicketmaster(cfg.TicketmasterAPIKey)
	sk := s.newSkiddle(cfg.SkiddleAPIKey)

	users, err := s.users.List(ctx)
	if err != nil {
		return 0, err
	}
	now := time.Now()
	windowStart := now.Add(-topArtistsWindow)

	synced := 0
	for _, u := range users {
		if u.City == "" {
			continue
		}
		artists, err := s.wrapped.TopArtists(ctx, u.ID, windowStart.UnixMilli(), now.UnixMilli(), topArtistsLimit)
		if err != nil {
			s.logger.Warn("concerts: top artists failed", "user", u.ID, "error", err)
			continue
		}
		for _, artist := range artists {
			synced += s.syncArtist(ctx, tm, sk, u.ID, u.City, artist.Name, now)
		}
	}
	return synced, nil
}

// syncArtist searches one artist for one user (Ticketmaster first, Skiddle
// only if that found nothing) and upserts any new, still-upcoming matches.
func (s *Service) syncArtist(ctx context.Context, tm, sk searcher, userID, city, artist string, now time.Time) int {
	events, err := tm.Search(ctx, artist, city, maxEventsPerArtist)
	source := "ticketmaster"
	if err != nil {
		s.logger.Warn("concerts: ticketmaster search failed", "artist", artist, "error", err)
		events = nil
	}
	if len(events) == 0 {
		events, err = sk.Search(ctx, artist, city, maxEventsPerArtist)
		source = "skiddle"
		if err != nil {
			s.logger.Warn("concerts: skiddle search failed", "artist", artist, "error", err)
			return 0
		}
	}
	synced := 0
	for _, e := range events {
		if e.id == "" || e.startTime.Before(now) {
			continue
		}
		created, err := s.concerts.Upsert(ctx, models.Concert{
			ID: uuid.NewString(), UserID: userID, Source: source, SourceEventID: e.id,
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
