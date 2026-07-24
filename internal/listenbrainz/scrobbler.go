package listenbrainz

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
const ScrobbleKind = "listenbrainz_scrobble"

// rateLimitRetry is the retry delay after ListenBrainz rate-limits us (429).
const rateLimitRetry = time.Minute

// Scrobbler is the outbox consumer that submits plays to ListenBrainz for
// users who've set a personal API token. It registers itself as a handler on
// a generic outbox.Worker and implements core.ScrobbleEnqueuer, the interface
// PlaybackService calls on every submitted scrobble.
type Scrobbler struct {
	client *Client
	worker *outbox.Worker
	logger *slog.Logger
}

// NewScrobbler builds the scrobbler and registers its handler on worker for
// ScrobbleKind.
func NewScrobbler(client *Client, worker *outbox.Worker, logger *slog.Logger) *Scrobbler {
	s := &Scrobbler{client: client, worker: worker, logger: logger}
	worker.Register(ScrobbleKind, s.handle)
	return s
}

// scrobblePayload is the outbox job payload -- everything the handler needs,
// so it never has to look the user back up (their token might change or
// clear before the job runs; scrobbling with the token as it was at play
// time is the right call either way).
type scrobblePayload struct {
	Token          string `json:"token"`
	Artist         string `json:"artist"`
	Track          string `json:"track"`
	Release        string `json:"release,omitempty"`
	DurationMs     int    `json:"durationMs,omitempty"`
	RecordingMBID  string `json:"recordingMbid,omitempty"`
	ISRC           string `json:"isrc,omitempty"`
	ListenedAtUnix int64  `json:"listenedAt"`
}

// EnqueueScrobble implements core.ScrobbleEnqueuer: no-op unless the user has
// scrobbling enabled and a ListenBrainz token set. Fire-and-forget (backed by
// the outbox), so a slow/unreachable ListenBrainz never blocks the caller.
func (s *Scrobbler) EnqueueScrobble(ctx context.Context, user models.User, track models.Track, at time.Time) {
	if !user.ScrobbleEnabled || user.ListenBrainzToken == "" {
		return
	}
	payload, err := json.Marshal(scrobblePayload{
		Token:          user.ListenBrainzToken,
		Artist:         track.ArtistName,
		Track:          track.Title,
		Release:        track.AlbumName,
		DurationMs:     track.Duration * 1000,
		RecordingMBID:  track.MBID,
		ISRC:           track.ISRC,
		ListenedAtUnix: at.Unix(),
	})
	if err != nil {
		s.logger.Warn("listenbrainz: marshal scrobble payload failed", "error", err)
		return
	}
	// Empty dedupe key: unlike playlist sync, every play must submit
	// independently -- collapsing would silently drop repeat plays of the
	// same track.
	if err := s.worker.Enqueue(ctx, ScrobbleKind, "", string(payload)); err != nil {
		s.logger.Warn("listenbrainz: enqueue scrobble failed", "error", err)
	}
}

// handle is the outbox.Handler: submits the listen, mapping a rate limit to
// an explicit retry delay. Any other failure (including an invalid/revoked
// token) gets the worker's default exponential backoff and eventually parks
// after its max-attempts cap -- there's no separate "permanent failure"
// signal to give it here.
func (s *Scrobbler) handle(ctx context.Context, job persistence.OutboxJob) error {
	var p scrobblePayload
	if err := json.Unmarshal([]byte(job.Payload), &p); err != nil {
		return fmt.Errorf("listenbrainz: bad job payload: %w", err)
	}
	err := s.client.SubmitListen(ctx, p.Token, Listen{
		ListenedAt:    time.Unix(p.ListenedAtUnix, 0),
		Artist:        p.Artist,
		Track:         p.Track,
		Release:       p.Release,
		DurationMs:    p.DurationMs,
		RecordingMBID: p.RecordingMBID,
		ISRC:          p.ISRC,
	})
	if errors.Is(err, ErrRateLimited) {
		return outbox.RetryAfter(rateLimitRetry, err)
	}
	return err
}
