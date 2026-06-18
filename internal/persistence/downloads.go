package persistence

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/immerle/immerle/internal/db"
	"github.com/immerle/immerle/internal/models"
)

// DownloadRepo persists the on-demand download job queue.
type DownloadRepo struct{ *base }

const downloadColumns = `id, user_id, provider, provider_track_id, query, status, track_id, error, attempts, created_at, updated_at`

func scanDownload(s rowScanner) (models.DownloadJob, error) {
	var j models.DownloadJob
	var status string
	var created, updated int64
	if err := s.Scan(&j.ID, &j.UserID, &j.Provider, &j.ProviderTrackID, &j.Query, &status, &j.TrackID, &j.Error, &j.Attempts, &created, &updated); err != nil {
		return j, err
	}
	j.Status = models.DownloadStatus(status)
	j.CreatedAt = db.FromMillis(created)
	j.UpdatedAt = db.FromMillis(updated)
	return j, nil
}

// Enqueue inserts a job, or returns the existing job for the same provider track
// (idempotent — strict dedup at the queue level).
func (r *DownloadRepo) Enqueue(ctx context.Context, j models.DownloadJob) (models.DownloadJob, error) {
	existing, err := r.GetByProviderTrack(ctx, j.Provider, j.ProviderTrackID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return j, err
	}
	_, err = r.exec(ctx, `INSERT INTO download_jobs (`+downloadColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		j.ID, j.UserID, j.Provider, j.ProviderTrackID, j.Query, string(j.Status), j.TrackID, j.Error, j.Attempts,
		db.Millis(j.CreatedAt), db.Millis(j.UpdatedAt))
	if err != nil {
		return j, err
	}
	return j, nil
}

// Get returns a job by id.
func (r *DownloadRepo) Get(ctx context.Context, id string) (models.DownloadJob, error) {
	row := r.queryRow(ctx, `SELECT `+downloadColumns+` FROM download_jobs WHERE id=?`, id)
	j, err := scanDownload(row)
	if errors.Is(err, sql.ErrNoRows) {
		return j, ErrNotFound
	}
	return j, err
}

// GetByProviderTrack returns a job for a provider track, if any.
func (r *DownloadRepo) GetByProviderTrack(ctx context.Context, provider, providerTrackID string) (models.DownloadJob, error) {
	row := r.queryRow(ctx, `SELECT `+downloadColumns+` FROM download_jobs WHERE provider=? AND provider_track_id=?`, provider, providerTrackID)
	j, err := scanDownload(row)
	if errors.Is(err, sql.ErrNoRows) {
		return j, ErrNotFound
	}
	return j, err
}

// ClaimNext atomically claims the oldest queued job and marks it running.
// Returns ErrNotFound when the queue is empty.
func (r *DownloadRepo) ClaimNext(ctx context.Context) (models.DownloadJob, error) {
	var claimed models.DownloadJob
	err := r.withTx(ctx, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, r.rebind(`SELECT `+downloadColumns+` FROM download_jobs
			WHERE status='queued' ORDER BY created_at LIMIT 1`))
		j, err := scanDownload(row)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, r.rebind(`UPDATE download_jobs SET status='running', attempts=attempts+1, updated_at=? WHERE id=?`),
			db.Millis(time.Now()), j.ID)
		if err != nil {
			return err
		}
		j.Status = models.DownloadRunning
		j.Attempts++
		claimed = j
		return nil
	})
	return claimed, err
}

// Complete marks a job completed and links the resulting track.
func (r *DownloadRepo) Complete(ctx context.Context, id, trackID string) error {
	_, err := r.exec(ctx, `UPDATE download_jobs SET status='completed', track_id=?, error='', updated_at=? WHERE id=?`,
		trackID, db.Millis(time.Now()), id)
	return err
}

// Fail marks a job failed (or re-queues it for retry if under the attempt cap).
func (r *DownloadRepo) Fail(ctx context.Context, id, errMsg string, requeue bool) error {
	status := "failed"
	if requeue {
		status = "queued"
	}
	_, err := r.exec(ctx, `UPDATE download_jobs SET status=?, error=?, updated_at=? WHERE id=?`,
		status, errMsg, db.Millis(time.Now()), id)
	return err
}

// RequeueStale resets jobs stuck in 'running' (e.g. after a crash) back to queued.
func (r *DownloadRepo) RequeueStale(ctx context.Context) error {
	_, err := r.exec(ctx, `UPDATE download_jobs SET status='queued', updated_at=? WHERE status='running'`, db.Millis(time.Now()))
	return err
}

// DeleteByTrack removes any download jobs that produced the given track (used
// when a downloaded track is evicted, so a later play re-downloads cleanly).
func (r *DownloadRepo) DeleteByTrack(ctx context.Context, trackID string) error {
	_, err := r.exec(ctx, `DELETE FROM download_jobs WHERE track_id=?`, trackID)
	return err
}

// ListByUser returns a user's jobs, most recent first.
func (r *DownloadRepo) ListByUser(ctx context.Context, userID string, limit int) ([]models.DownloadJob, error) {
	rows, err := r.query(ctx, `SELECT `+downloadColumns+` FROM download_jobs WHERE user_id=? ORDER BY created_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.DownloadJob
	for rows.Next() {
		j, err := scanDownload(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
