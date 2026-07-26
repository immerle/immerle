package lastfm

import (
	"context"
	"fmt"
	"strings"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/reccobeats"
)

// Recommender resolves a similarity-based discovery mix from seed tracks
// (track.getSimilar), satisfying autoplaylists.Recommender the same way
// *reccobeats.Client does. Unlike ReccoBeats it needs the admin-configured
// API key/secret, read live so a key rotated from the admin UI applies
// without a restart -- same reasoning as Scrobbler.
type Recommender struct {
	client      *Client
	credentials func() models.LastFmRuntime
}

// NewRecommender builds a Recommender.
func NewRecommender(client *Client, credentials func() models.LastFmRuntime) *Recommender {
	return &Recommender{client: client, credentials: credentials}
}

// Recommend expands each seed via track.getSimilar, deduping candidates
// (case-insensitively, by artist+title) against each other, stopping once
// size results have been collected. A seed Last.fm has no similarity data
// for is skipped rather than failing the whole request -- best-effort, same
// as ReccoBeats' per-seed resolution.
func (r *Recommender) Recommend(ctx context.Context, seeds []reccobeats.Seed, size int) ([]reccobeats.Track, error) {
	cfg := r.credentials()
	if cfg.APIKey == "" || cfg.APISecret == "" {
		return nil, fmt.Errorf("lastfm: not configured")
	}
	seen := make(map[string]bool, len(seeds))
	for _, seed := range seeds {
		seen[dedupeKey(seed.Artist, seed.Title)] = true
	}
	var out []reccobeats.Track
	for _, seed := range seeds {
		if len(out) >= size {
			break
		}
		similar, err := r.client.GetSimilarTracks(ctx, cfg.APIKey, cfg.APISecret, seed.Artist, seed.Title, size)
		if err != nil {
			continue
		}
		for _, s := range similar {
			key := dedupeKey(s.Artist, s.Title)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, reccobeats.Track{Artist: s.Artist, Title: s.Title})
			if len(out) >= size {
				break
			}
		}
	}
	return out, nil
}

func dedupeKey(artist, title string) string {
	return strings.ToLower(artist) + "\x00" + strings.ToLower(title)
}
