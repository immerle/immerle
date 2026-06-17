package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// ExternalPlaylist is a third-party playlist (e.g. Spotify) fetched through the
// hub. The hub holds the third-party credentials and resolves public playlists,
// so instances import without their own Spotify keys.
type ExternalPlaylist struct {
	Name        string
	Description string
	Tracks      []ExternalTrack
}

// ExternalTrack is one track of an ExternalPlaylist (metadata only).
type ExternalTrack struct {
	Title    string
	Artist   string
	Album    string
	ISRC     string
	Duration int
}

// importPollInterval is the delay between hub job polls (a var so tests can
// shorten it); importPollTimeout bounds the whole wait.
var importPollInterval = 2 * time.Second

const importPollTimeout = 5 * time.Minute

// FetchExternalPlaylist imports a public playlist from an external source (only
// "spotify" today) through the hub. The hub processes imports as a lazy job
// (enqueue + poll) to stay within the third party's rate limits, so this:
//  1. POSTs /api/v1/spotify/imports {"playlist": ref} → a job id;
//  2. polls GET /api/v1/spotify/imports/{id} until completed or failed.
//
// It works whenever a hub URL + keys are configured, independent of the
// background-sync Enabled flag (an import is a distinct, user-initiated action).
func (s *Service) FetchExternalPlaylist(ctx context.Context, source, ref string) (ExternalPlaylist, error) {
	if !s.HubConfigured() {
		return ExternalPlaylist{}, fmt.Errorf("hub not configured (set the hub URL, public key and private key)")
	}
	if source != "spotify" {
		return ExternalPlaylist{}, fmt.Errorf("hub import source %q not supported", source)
	}

	// DEBUG: surface the exact credentials sent to the hub so a 401 can be
	// diagnosed against the hub dashboard. Remove once the auth is sorted.
	c := s.cfg()
	s.logger.Warn("hub import debug — credentials sent",
		"hubUrl", c.HubURL,
		"publicKey (X-Instance-ID)", c.PublicKey,
		"privateKey (Bearer)", c.PrivateKey)

	raw, err := s.do(ctx, http.MethodPost, "/api/v1/spotify/imports", map[string]any{"playlist": ref})
	if err != nil {
		return ExternalPlaylist{}, err
	}
	var job spotifyImportJob
	if err := json.Unmarshal(raw, &job); err != nil {
		return ExternalPlaylist{}, fmt.Errorf("decode import job: %w", err)
	}
	if job.JobID == "" {
		return ExternalPlaylist{}, fmt.Errorf("hub returned no job id")
	}

	ticker := time.NewTicker(importPollInterval)
	defer ticker.Stop()
	timeout := time.NewTimer(importPollTimeout)
	defer timeout.Stop()
	for {
		switch job.Status {
		case "completed":
			return job.toExternal(), nil
		case "failed":
			if job.Error == "" {
				job.Error = "unknown error"
			}
			return ExternalPlaylist{}, fmt.Errorf("hub import failed: %s", job.Error)
		}
		select {
		case <-ctx.Done():
			return ExternalPlaylist{}, ctx.Err()
		case <-timeout.C:
			return ExternalPlaylist{}, fmt.Errorf("hub import did not complete within %s", importPollTimeout)
		case <-ticker.C:
		}
		raw, err := s.do(ctx, http.MethodGet, "/api/v1/spotify/imports/"+url.PathEscape(job.JobID), nil)
		if err != nil {
			return ExternalPlaylist{}, err
		}
		job = spotifyImportJob{}
		if err := json.Unmarshal(raw, &job); err != nil {
			return ExternalPlaylist{}, fmt.Errorf("decode import job: %w", err)
		}
	}
}

// spotifyImportJob mirrors the hub's job payload (POST/GET spotify/imports).
type spotifyImportJob struct {
	JobID    string `json:"jobId"`
	Status   string `json:"status"` // pending | in_progress | completed | failed
	Error    string `json:"error"`
	Playlist struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"playlist"`
	Tracks []struct {
		Artist   string `json:"artist"`
		Title    string `json:"title"`
		Album    string `json:"album"`
		Duration int    `json:"duration"`
	} `json:"tracks"`
}

func (j spotifyImportJob) toExternal() ExternalPlaylist {
	pl := ExternalPlaylist{Name: j.Playlist.Name, Description: j.Playlist.Description}
	for _, t := range j.Tracks {
		pl.Tracks = append(pl.Tracks, ExternalTrack{
			Title: t.Title, Artist: t.Artist, Album: t.Album, Duration: t.Duration,
		})
	}
	return pl
}
