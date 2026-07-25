package lastfm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/outbox"
	"github.com/immerle/immerle/internal/persistence"
)

// ScrobbleKind is the outbox job kind handled by Scrobbler.
const ScrobbleKind = "lastfm_scrobble"

// rateLimitRetry is the retry delay after Last.fm rate-limits us.
const rateLimitRetry = time.Minute

// Scrobbler is the outbox consumer that submits plays to Last.fm for users
// who've connected their account. It registers itself as a handler on a
// generic outbox.Worker and implements core.ScrobbleEnqueuer.
type Scrobbler struct {
	client *Client
	// credentials reads the admin-configured API key/secret live (read fresh
	// on every job, not baked in at construction) so a key rotated from the
	// admin UI applies without a restart -- same reasoning as
	// SettingsService.ConcertsConfig.
	credentials func() models.LastFmRuntime
	worker      *outbox.Worker
	logger      *slog.Logger
}

// NewScrobbler builds the scrobbler and registers its handler on worker for
// ScrobbleKind.
func NewScrobbler(client *Client, credentials func() models.LastFmRuntime, worker *outbox.Worker, logger *slog.Logger) *Scrobbler {
	s := &Scrobbler{client: client, credentials: credentials, worker: worker, logger: logger}
	worker.Register(ScrobbleKind, s.handle)
	return s
}

// scrobblePayload is the outbox job payload. It carries the user's own
// session key (which might change or clear before the job runs -- scrobbling
// with the key as it was at play time is the right call either way), but not
// the app-level API key/secret, which is read live at handle time.
type scrobblePayload struct {
	SessionKey     string `json:"sessionKey"`
	Artist         string `json:"artist"`
	Track          string `json:"track"`
	Release        string `json:"release,omitempty"`
	DurationMs     int    `json:"durationMs,omitempty"`
	ListenedAtUnix int64  `json:"listenedAt"`
}

// EnqueueScrobble implements core.ScrobbleEnqueuer: no-op unless the user has
// scrobbling enabled and has connected a Last.fm account. Fire-and-forget
// (backed by the outbox), so a slow/unreachable Last.fm never blocks the
// caller.
func (s *Scrobbler) EnqueueScrobble(ctx context.Context, user models.User, track models.Track, at time.Time) {
	if !user.ScrobbleEnabled || user.LastFmSessionKey == "" {
		return
	}
	payload, err := json.Marshal(scrobblePayload{
		SessionKey:     user.LastFmSessionKey,
		Artist:         track.ArtistName,
		Track:          track.Title,
		Release:        track.AlbumName,
		DurationMs:     track.Duration * 1000,
		ListenedAtUnix: at.Unix(),
	})
	if err != nil {
		s.logger.Warn("lastfm: marshal scrobble payload failed", "error", err)
		return
	}
	// Empty dedupe key: every play must submit independently, same reasoning
	// as the ListenBrainz scrobbler.
	if err := s.worker.Enqueue(ctx, ScrobbleKind, "", string(payload)); err != nil {
		s.logger.Warn("lastfm: enqueue scrobble failed", "error", err)
	}
}

// handle is the outbox.Handler: submits the listen, mapping a rate limit to
// an explicit retry delay. Any other failure (including an invalid/revoked
// session, or the admin not having configured Last.fm) gets the worker's
// default exponential backoff and eventually parks after its max-attempts cap.
func (s *Scrobbler) handle(ctx context.Context, job persistence.OutboxJob) error {
	var p scrobblePayload
	if err := json.Unmarshal([]byte(job.Payload), &p); err != nil {
		return fmt.Errorf("lastfm: bad job payload: %w", err)
	}
	cfg := s.credentials()
	if cfg.APIKey == "" || cfg.APISecret == "" {
		return fmt.Errorf("lastfm: not configured")
	}
	err := s.client.Scrobble(ctx, cfg.APIKey, cfg.APISecret, p.SessionKey, Listen{
		ListenedAt: time.Unix(p.ListenedAtUnix, 0),
		Artist:     p.Artist,
		Track:      p.Track,
		Release:    p.Release,
		DurationMs: p.DurationMs,
	})
	if errors.Is(err, ErrRateLimited) {
		return outbox.RetryAfter(rateLimitRetry, err)
	}
	return err
}
