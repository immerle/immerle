package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/immerle/immerle/internal/db"
	"github.com/immerle/immerle/internal/models"
)

// BandcampConnectionRepo persists each user's link to their personal Bandcamp
// account (see migration 00045). One row per user.
type BandcampConnectionRepo struct{ *base }

func scanBandcampConnection(s rowScanner) (models.BandcampConnection, error) {
	var c models.BandcampConnection
	var connectedAt int64
	var lastSyncedAt, invalidSince sql.NullInt64
	if err := s.Scan(&c.UserID, &c.FanID, &c.IdentityEnc, &connectedAt, &lastSyncedAt, &invalidSince); err != nil {
		return c, err
	}
	c.ConnectedAt = db.FromMillis(connectedAt)
	c.LastSyncedAt = db.TimePtr(lastSyncedAt)
	c.InvalidSince = db.TimePtr(invalidSince)
	return c, nil
}

// Upsert connects (or reconnects) a user's Bandcamp account, overwriting any
// previous fan id/cookie and clearing invalid_since.
func (r *BandcampConnectionRepo) Upsert(ctx context.Context, c models.BandcampConnection) error {
	_, err := r.exec(ctx, `INSERT INTO bandcamp_connections (user_id, fan_id, identity_enc, connected_at, last_synced_at, invalid_since)
		VALUES (?, ?, ?, ?, NULL, NULL)
		ON CONFLICT (user_id) DO UPDATE SET fan_id=excluded.fan_id, identity_enc=excluded.identity_enc,
			connected_at=excluded.connected_at, last_synced_at=NULL, invalid_since=NULL`,
		c.UserID, c.FanID, c.IdentityEnc, db.Millis(c.ConnectedAt))
	return err
}

// Get returns a user's Bandcamp connection, or ErrNotFound if not connected.
func (r *BandcampConnectionRepo) Get(ctx context.Context, userID string) (models.BandcampConnection, error) {
	row := r.queryRow(ctx, `SELECT user_id, fan_id, identity_enc, connected_at, last_synced_at, invalid_since
		FROM bandcamp_connections WHERE user_id=?`, userID)
	c, err := scanBandcampConnection(row)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// Delete disconnects a user's Bandcamp account.
func (r *BandcampConnectionRepo) Delete(ctx context.Context, userID string) error {
	_, err := r.exec(ctx, `DELETE FROM bandcamp_connections WHERE user_id=?`, userID)
	return err
}

// TouchSynced records a successful collection fetch.
func (r *BandcampConnectionRepo) TouchSynced(ctx context.Context, userID string, at time.Time) error {
	_, err := r.exec(ctx, `UPDATE bandcamp_connections SET last_synced_at=? WHERE user_id=?`, db.Millis(at), userID)
	return err
}

// MarkInvalid records that the stored cookie no longer works, so the user can
// be prompted to reconnect instead of the worker retrying forever.
func (r *BandcampConnectionRepo) MarkInvalid(ctx context.Context, userID string, at time.Time) error {
	_, err := r.exec(ctx, `UPDATE bandcamp_connections SET invalid_since=? WHERE user_id=?`, db.Millis(at), userID)
	return err
}

// BandcampImportRepo persists the Bandcamp purchase-import job queue (see
// migration 00045).
type BandcampImportRepo struct{ *base }

const bandcampJobColumns = `id, user_id, sale_item_type, sale_item_id, item_type, artist_name, item_title, format, status, track_ids, error, attempts, created_at, updated_at`

func scanBandcampJob(s rowScanner) (models.BandcampImportJob, error) {
	var j models.BandcampImportJob
	var status, trackIDs string
	var created, updated int64
	if err := s.Scan(&j.ID, &j.UserID, &j.SaleItemType, &j.SaleItemID, &j.ItemType, &j.ArtistName, &j.ItemTitle,
		&j.Format, &status, &trackIDs, &j.Error, &j.Attempts, &created, &updated); err != nil {
		return j, err
	}
	j.Status = models.BandcampJobStatus(status)
	if trackIDs != "" {
		_ = json.Unmarshal([]byte(trackIDs), &j.TrackIDs)
	}
	j.CreatedAt = db.FromMillis(created)
	j.UpdatedAt = db.FromMillis(updated)
	return j, nil
}

// Enqueue inserts a job, or returns the existing job for the same purchased
// item (idempotent — re-clicking "import" doesn't create a duplicate).
// created reports whether a new row was actually inserted.
func (r *BandcampImportRepo) Enqueue(ctx context.Context, j models.BandcampImportJob) (models.BandcampImportJob, bool, error) {
	existing, err := r.GetByKey(ctx, j.UserID, j.SaleItemType, j.SaleItemID)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return j, false, err
	}
	_, err = r.exec(ctx, `INSERT INTO bandcamp_import_jobs
		(id, user_id, sale_item_type, sale_item_id, item_type, artist_name, item_title, format, status, track_ids, error, attempts, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '', '', 0, ?, ?)`,
		j.ID, j.UserID, j.SaleItemType, j.SaleItemID, j.ItemType, j.ArtistName, j.ItemTitle, j.Format, string(j.Status),
		db.Millis(j.CreatedAt), db.Millis(j.UpdatedAt))
	if err != nil {
		return j, false, err
	}
	return j, true, nil
}

// Get returns a job by id.
func (r *BandcampImportRepo) Get(ctx context.Context, id string) (models.BandcampImportJob, error) {
	row := r.queryRow(ctx, `SELECT `+bandcampJobColumns+` FROM bandcamp_import_jobs WHERE id=?`, id)
	j, err := scanBandcampJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return j, ErrNotFound
	}
	return j, err
}

// GetByKey returns a user's job for a purchased item, if any.
func (r *BandcampImportRepo) GetByKey(ctx context.Context, userID, saleItemType, saleItemID string) (models.BandcampImportJob, error) {
	row := r.queryRow(ctx, `SELECT `+bandcampJobColumns+` FROM bandcamp_import_jobs
		WHERE user_id=? AND sale_item_type=? AND sale_item_id=?`, userID, saleItemType, saleItemID)
	j, err := scanBandcampJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return j, ErrNotFound
	}
	return j, err
}

// ListByUser returns a user's jobs, most recent first.
func (r *BandcampImportRepo) ListByUser(ctx context.Context, userID string, limit int) ([]models.BandcampImportJob, error) {
	rows, err := r.query(ctx, `SELECT `+bandcampJobColumns+` FROM bandcamp_import_jobs
		WHERE user_id=? ORDER BY created_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.BandcampImportJob
	for rows.Next() {
		j, err := scanBandcampJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// ClaimNext atomically claims the oldest queued job, marking it running. Runs
// in a transaction since the increment (attempts=attempts+1) is column-relative.
func (r *BandcampImportRepo) ClaimNext(ctx context.Context) (models.BandcampImportJob, error) {
	var claimed models.BandcampImportJob
	err := r.withTx(ctx, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, r.rebind(`SELECT `+bandcampJobColumns+` FROM bandcamp_import_jobs
			WHERE status='queued' ORDER BY created_at LIMIT 1`))
		j, err := scanBandcampJob(row)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, r.rebind(`UPDATE bandcamp_import_jobs SET status='running', attempts=attempts+1, updated_at=? WHERE id=?`),
			db.Millis(time.Now()), j.ID)
		if err != nil {
			return err
		}
		j.Status = models.BandcampRunning
		j.Attempts++
		claimed = j
		return nil
	})
	return claimed, err
}

// Complete marks a job completed and records the resulting track ids.
func (r *BandcampImportRepo) Complete(ctx context.Context, id string, trackIDs []string) error {
	encoded, err := json.Marshal(trackIDs)
	if err != nil {
		return err
	}
	_, err = r.exec(ctx, `UPDATE bandcamp_import_jobs SET status='completed', track_ids=?, error='', updated_at=? WHERE id=?`,
		string(encoded), db.Millis(time.Now()), id)
	return err
}

// Fail marks a job failed (or re-queues it for retry if under the attempt cap).
func (r *BandcampImportRepo) Fail(ctx context.Context, id, errMsg string, requeue bool) error {
	status := "failed"
	if requeue {
		status = "queued"
	}
	_, err := r.exec(ctx, `UPDATE bandcamp_import_jobs SET status=?, error=?, updated_at=? WHERE id=?`,
		status, errMsg, db.Millis(time.Now()), id)
	return err
}

// RequeueStale resets jobs stuck in 'running' (e.g. after a crash) back to queued.
func (r *BandcampImportRepo) RequeueStale(ctx context.Context) error {
	_, err := r.exec(ctx, `UPDATE bandcamp_import_jobs SET status='queued', updated_at=? WHERE status='running'`, db.Millis(time.Now()))
	return err
}
