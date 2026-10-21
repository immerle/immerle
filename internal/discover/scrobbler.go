package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/outbox"
	"github.com/immerle/immerle/internal/persistence"
)

// ScrobbleKind and ForgetKind are the outbox job kinds handled by Scrobbler.
const (
	ScrobbleKind = "discover_scrobble"
	ForgetKind   = "discover_forget"
)

// Scrobbler submits plays to discover through the outbox (fire-and-forget,
// retried with backoff). Implements core.ScrobbleEnqueuer.
type Scrobbler struct {
	client *Client
	worker *outbox.Worker
	logger *slog.Logger
}

// NewScrobbler builds the scrobbler and registers its handler on worker.
func NewScrobbler(client *Client, worker *outbox.Worker, logger *slog.Logger) *Scrobbler {
	s := &Scrobbler{client: client, worker: worker, logger: logger}
	worker.Register(ScrobbleKind, s.handle)
	worker.Register(ForgetKind, s.handleForget)
	return s
}

type scrobblePayload struct {
	UserID string `json:"userId"`
	Play   Play   `json:"play"`
}

// EnqueueScrobble is a no-op unless discover scrobbling is on and the user
// has scrobbling enabled.
func (s *Scrobbler) EnqueueScrobble(ctx context.Context, user models.User, track models.Track, at time.Time) {
	if !user.ScrobbleEnabled || !s.client.ScrobbleEnabled() {
		return
	}
	play := Play{Artist: track.ArtistName, Title: track.Title, Album: track.AlbumName, PlayedAt: at.UTC().Format(time.RFC3339)}
	if track.MBID != "" {
		play.IDs = []string{"mbid:" + track.MBID}
	}
	payload, err := json.Marshal(scrobblePayload{UserID: user.ID, Play: play})
	if err != nil {
		s.logger.Warn("discover: marshal scrobble payload failed", "error", err)
		return
	}
	if err := s.worker.Enqueue(ctx, ScrobbleKind, "", string(payload)); err != nil {
		s.logger.Warn("discover: enqueue scrobble failed", "error", err)
	}
}

// handle drops the job when scrobbling was switched off since it was queued.
func (s *Scrobbler) handle(ctx context.Context, job persistence.OutboxJob) error {
	if !s.client.ScrobbleEnabled() {
		return nil
	}
	var p scrobblePayload
	if err := json.Unmarshal([]byte(job.Payload), &p); err != nil {
		return fmt.Errorf("discover: bad job payload: %w", err)
	}
	return s.client.Scrobble(ctx, p.UserID, []Play{p.Play})
}

// ForgetUser queues the deletion of a deleted user's discover data. Implements
// core.UserForgetter; queued so an unreachable discover is retried.
func (s *Scrobbler) ForgetUser(ctx context.Context, userID string) {
	if err := s.worker.Enqueue(ctx, ForgetKind, userID, userID); err != nil {
		s.logger.Warn("discover: enqueue forget failed", "error", err)
	}
}

func (s *Scrobbler) handleForget(ctx context.Context, job persistence.OutboxJob) error {
	return s.client.ForgetListener(ctx, job.Payload)
}
