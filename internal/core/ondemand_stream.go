package core

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/gossignol/gossignol/internal/models"
	"github.com/gossignol/gossignol/internal/providers"
)

// PendingDownload is a remote track that is not yet local and must be fetched.
// It carries everything needed to stream-and-save it without a second lookup.
type PendingDownload struct {
	prov   providers.Provider
	ptid   string
	userID string
	// Meta is the resolved provider metadata (title/artist/album/suffix/...).
	Meta providers.Result
}

// Suffix returns the audio file extension the provider will deliver.
func (p *PendingDownload) Suffix() string {
	if p.Meta.Suffix != "" {
		return p.Meta.Suffix
	}
	return "mp3"
}

// PrepareStream resolves a track id for playback WITHOUT downloading. It returns
// either an existing local track (local=true) or a PendingDownload to stream and
// save (local=false). For remote ids it runs the same dedup as Resolve (by MBID,
// then by a previously completed download job); local ids are returned as-is.
//
// This is the fast-path peek used by the streaming endpoint so a first listen can
// be served progressively instead of waiting for the whole download to land.
func (s *CatalogService) PrepareStream(ctx context.Context, userID, id string) (models.Track, bool, *PendingDownload, error) {
	st := s.state
	if !IsRemoteID(id) {
		t, err := st.catalog.GetTrack(ctx, id)
		return t, err == nil, nil, err
	}

	provName, ptid, ok := decodeRemoteID(id)
	if !ok {
		return models.Track{}, false, nil, fmt.Errorf("invalid remote id")
	}
	prov, ok := st.registry.Get(provName)
	if !ok {
		return models.Track{}, false, nil, fmt.Errorf("unknown provider %q", provName)
	}

	meta, err := prov.Resolve(ctx, ptid)
	if err != nil {
		return models.Track{}, false, nil, err
	}

	// Already in the library (strict dedup by MBID).
	if meta.MBID != "" {
		if lid, exists, _ := st.catalog.TrackExistsByMBIDOrHash(ctx, meta.MBID, ""); exists {
			t, err := st.catalog.GetTrack(ctx, lid)
			return t, true, nil, err
		}
	}
	// Already downloaded earlier via this provider track.
	if job, err := st.downloads.GetByProviderTrack(ctx, prov.Name(), ptid); err == nil &&
		job.Status == models.DownloadCompleted && job.TrackID != "" {
		if t, err := st.catalog.GetTrack(ctx, job.TrackID); err == nil {
			return t, true, nil, nil
		}
	}

	return models.Track{}, false, &PendingDownload{prov: prov, ptid: ptid, userID: userID, Meta: meta}, nil
}

// LocalTrackIDForRemote returns the local track id a remote provider track was
// downloaded to (via a completed download job), if any. Read-only: it never
// downloads. Used to reflect a downloaded track's like/rating/play state when it
// is still listed under its remote id (e.g. on a provider album page).
func (s *CatalogService) LocalTrackIDForRemote(ctx context.Context, remoteID string) (string, bool) {
	if s == nil || s.state == nil || !IsRemoteID(remoteID) {
		return "", false
	}
	provName, ptid, ok := decodeRemoteID(remoteID)
	if !ok {
		return "", false
	}
	job, err := s.state.downloads.GetByProviderTrack(ctx, provName, ptid)
	if err == nil && job.Status == models.DownloadCompleted && job.TrackID != "" {
		return job.TrackID, true
	}
	return "", false
}

// StreamPending streams a pending track's bytes to w while teeing a copy to disk,
// then ingests that copy in the background so later plays are served locally (and
// transcoded as requested). The first listen is therefore instant: bytes flow to
// the client as they arrive from the provider, with no prior full-file buffering.
func (s *CatalogService) StreamPending(ctx context.Context, pd *PendingDownload, w io.Writer) error {
	suffix := pd.Suffix()
	dest := s.destPath(pd.Meta, suffix)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}

	// A request-unique temp avoids clobbering when two clients start the same
	// not-yet-local track at once; only one finalize wins (singleflight below).
	tmp := dest + ".part-" + uuid.NewString()
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	// Tee: provider stream → client AND disk simultaneously.
	dlErr := pd.prov.Download(ctx, pd.ptid, io.MultiWriter(w, f))
	closeErr := f.Close()
	if dlErr != nil {
		_ = os.Remove(tmp) // client likely disconnected; response already started
		return dlErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}

	// Ingest off the request path so the chunked response terminates immediately
	// (the client isn't held open after the last audio byte). Detached context so
	// it completes even if the client disconnects right away.
	go s.finalizeStreamed(context.WithoutCancel(ctx), pd, tmp, dest)
	return nil
}

// finalizeStreamed embeds tags, ingests the saved file and records a completed
// download job so the track becomes local. Deduplicated with the worker/resolve
// paths via singleflight; if another path already produced the track, the temp
// file is discarded.
func (s *CatalogService) finalizeStreamed(ctx context.Context, pd *PendingDownload, tmp, dest string) {
	st := s.state
	key := "job:" + pd.prov.Name() + ":" + pd.ptid
	_, _, _ = st.group.Do(key, func() (any, error) {
		if job, err := st.downloads.GetByProviderTrack(ctx, pd.prov.Name(), pd.ptid); err == nil &&
			job.Status == models.DownloadCompleted && job.TrackID != "" {
			_ = os.Remove(tmp)
			return job.TrackID, nil
		}
		if err := s.embedTags(ctx, tmp, dest, pd.Meta); err != nil {
			_ = os.Remove(tmp)
			st.logger.Warn("finalize: embed tags failed", "track", pd.ptid, "error", err)
			return nil, err
		}
		s.saveSidecarCover(ctx, pd.Meta, dest)
		if err := st.scanner.ScanFile(ctx, dest); err != nil {
			st.logger.Warn("finalize: scan failed", "track", pd.ptid, "error", err)
			return nil, err
		}
		id, err := s.trackIDForPath(ctx, dest)
		if err != nil {
			st.logger.Warn("finalize: track not found after scan", "track", pd.ptid, "error", err)
			return nil, err
		}
		now := time.Now()
		job, err := st.downloads.Enqueue(ctx, models.DownloadJob{
			ID: uuid.NewString(), UserID: pd.userID, Provider: pd.prov.Name(), ProviderTrackID: pd.ptid,
			Query: pd.Meta.Title, Status: models.DownloadQueued, CreatedAt: now, UpdatedAt: now,
		})
		if err == nil {
			_ = st.downloads.Complete(ctx, job.ID, id)
		}
		st.logger.Info("streamed track ingested", "track", id, "provider", pd.prov.Name())
		return id, nil
	})
}
