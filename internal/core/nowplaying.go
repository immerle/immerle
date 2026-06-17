package core

import (
	"sync"
	"time"

	"github.com/gossignol/gossignol/internal/models"
)

// NowPlayingTracker keeps an in-memory record of what each user is currently
// playing. Entries expire after a fixed window.
type NowPlayingTracker struct {
	mu      sync.RWMutex
	entries map[string]models.NowPlaying // keyed by user id
	ttl     time.Duration
}

// NewNowPlayingTracker builds a tracker with the given entry TTL.
func NewNowPlayingTracker(ttl time.Duration) *NowPlayingTracker {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &NowPlayingTracker{
		entries: make(map[string]models.NowPlaying),
		ttl:     ttl,
	}
}

// Set records that a user is playing a track now.
func (t *NowPlayingTracker) Set(userID, username, trackID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entries[userID] = models.NowPlaying{
		UserID:   userID,
		Username: username,
		TrackID:  trackID,
		At:       time.Now(),
	}
}

// List returns non-expired now-playing entries.
func (t *NowPlayingTracker) List() []models.NowPlaying {
	t.mu.RLock()
	defer t.mu.RUnlock()
	now := time.Now()
	var out []models.NowPlaying
	for _, e := range t.entries {
		if now.Sub(e.At) <= t.ttl {
			out = append(out, e)
		}
	}
	return out
}
