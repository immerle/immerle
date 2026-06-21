package federation

import (
	"context"
	"fmt"
	"time"

	"github.com/immerle/immerle/internal/federation/hub"
)

// ExternalPlaylist is a third-party playlist (e.g. Spotify) fetched through the
// hub. The hub holds the third-party credentials and resolves public playlists,
// so instances import without their own Spotify keys.
type ExternalPlaylist struct {
	Name   string
	Tracks []ExternalTrack
}

// ExternalTrack is one track of an ExternalPlaylist (metadata only).
type ExternalTrack struct {
	Title  string
	Artist string
	Album  string
}

// importPollInterval is the delay between hub job polls (a var so tests can
// shorten it); importPollTimeout bounds the whole wait.
var importPollInterval = 2 * time.Second

const importPollTimeout = 5 * time.Minute

// FetchExternalPlaylist imports a public playlist from an external source (only
// "spotify" today) through the hub. The hub processes imports as a lazy job
// (enqueue + poll) to stay within the third party's rate limits, so this:
//  1. enqueues the import → a job id;
//  2. polls the job until completed or failed.
//
// It works whenever the instance is registered with the hub, independent of the
// background-sync Enabled flag (an import is a distinct, user-initiated action).
func (s *Service) FetchExternalPlaylist(ctx context.Context, source, ref string) (ExternalPlaylist, error) {
	if !s.HubConfigured() {
		return ExternalPlaylist{}, fmt.Errorf("hub not configured (register the instance with the hub first)")
	}
	if source != "spotify" {
		return ExternalPlaylist{}, fmt.Errorf("hub import source %q not supported", source)
	}

	job, err := s.hub.SpotifyImport(ctx, s.auth(), ref)
	if err != nil {
		return ExternalPlaylist{}, err
	}
	if deref(job.JobId) == "" {
		return ExternalPlaylist{}, fmt.Errorf("hub returned no job id")
	}

	ticker := time.NewTicker(importPollInterval)
	defer ticker.Stop()
	timeout := time.NewTimer(importPollTimeout)
	defer timeout.Stop()
	for {
		switch deref(job.Status) {
		case "completed":
			return toExternal(job), nil
		case "failed":
			msg := deref(job.Error)
			if msg == "" {
				msg = "unknown error"
			}
			return ExternalPlaylist{}, fmt.Errorf("hub import failed: %s", msg)
		}
		select {
		case <-ctx.Done():
			return ExternalPlaylist{}, ctx.Err()
		case <-timeout.C:
			return ExternalPlaylist{}, fmt.Errorf("hub import did not complete within %s", importPollTimeout)
		case <-ticker.C:
		}
		if job, err = s.hub.SpotifyJob(ctx, s.auth(), deref(job.JobId)); err != nil {
			return ExternalPlaylist{}, err
		}
	}
}

// toExternal flattens a completed hub Spotify job into an ExternalPlaylist.
func toExternal(j hub.PublicSpotifyJobResponse) ExternalPlaylist {
	pl := ExternalPlaylist{}
	if j.Playlist != nil {
		pl.Name = deref(j.Playlist.Name)
	}
	if j.Tracks != nil {
		for _, t := range *j.Tracks {
			pl.Tracks = append(pl.Tracks, ExternalTrack{Title: deref(t.Title), Artist: deref(t.Artist), Album: deref(t.Album)})
		}
	}
	return pl
}
