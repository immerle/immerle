package persistence

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/immerle/immerle/internal/db"
	"github.com/immerle/immerle/internal/models"
)

// ImportRepo persists playlist-import jobs and their per-track items.
type ImportRepo struct{ *base }

const importColumns = `id, user_id, source, source_ref, source_playlist_name, playlist_id, status,
	total, matched, doubtful, missing, failed, error, created_at, updated_at`

func scanImport(s rowScanner) (models.Import, error) {
	var im models.Import
	var status string
	var playlistID sql.NullString
	var created, updated int64
	if err := s.Scan(&im.ID, &im.UserID, &im.Source, &im.SourceRef, &im.SourcePlaylistName, &playlistID, &status,
		&im.Total, &im.Matched, &im.Doubtful, &im.Missing, &im.Failed, &im.Error, &created, &updated); err != nil {
		return im, err
	}
	im.PlaylistID = playlistID.String
	im.Status = models.ImportStatus(status)
	im.CreatedAt = db.FromMillis(created)
	im.UpdatedAt = db.FromMillis(updated)
	return im, nil
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Create inserts a new import job.
func (r *ImportRepo) Create(ctx context.Context, im models.Import) error {
	_, err := r.exec(ctx, `INSERT INTO imports (`+importColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		im.ID, im.UserID, im.Source, im.SourceRef, im.SourcePlaylistName, nullStr(im.PlaylistID), string(im.Status),
		im.Total, im.Matched, im.Doubtful, im.Missing, im.Failed, im.Error,
		db.Millis(im.CreatedAt), db.Millis(im.UpdatedAt))
	return err
}

// Update writes the mutable fields of an import job (status, counts, playlist
// link, error, source playlist name, total).
func (r *ImportRepo) Update(ctx context.Context, im models.Import) error {
	_, err := r.exec(ctx, `UPDATE imports SET source_playlist_name=?, playlist_id=?, status=?,
		total=?, matched=?, doubtful=?, missing=?, failed=?, error=?, updated_at=? WHERE id=?`,
		im.SourcePlaylistName, nullStr(im.PlaylistID), string(im.Status),
		im.Total, im.Matched, im.Doubtful, im.Missing, im.Failed, im.Error,
		db.Millis(time.Now()), im.ID)
	return err
}

// Get returns an import job (without items).
func (r *ImportRepo) Get(ctx context.Context, id string) (models.Import, error) {
	row := r.queryRow(ctx, `SELECT `+importColumns+` FROM imports WHERE id=?`, id)
	im, err := scanImport(row)
	if errors.Is(err, sql.ErrNoRows) {
		return im, ErrNotFound
	}
	return im, err
}

// ListByUser returns a user's imports, most recent first.
func (r *ImportRepo) ListByUser(ctx context.Context, userID string, limit int) ([]models.Import, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.query(ctx, `SELECT `+importColumns+` FROM imports WHERE user_id=? ORDER BY created_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Import
	for rows.Next() {
		im, err := scanImport(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, im)
	}
	return out, rows.Err()
}

// ClaimNext atomically claims the oldest queued import and marks it running.
// Returns ErrNotFound when none are queued.
func (r *ImportRepo) ClaimNext(ctx context.Context) (models.Import, error) {
	var claimed models.Import
	err := r.withTx(ctx, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, r.rebind(`SELECT `+importColumns+` FROM imports
			WHERE status='queued' ORDER BY created_at LIMIT 1`))
		im, err := scanImport(row)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, r.rebind(`UPDATE imports SET status='running', updated_at=? WHERE id=?`),
			db.Millis(time.Now()), im.ID); err != nil {
			return err
		}
		im.Status = models.ImportRunning
		claimed = im
		return nil
	})
	return claimed, err
}

// RequeueStale resets imports stuck in 'running' (e.g. after a crash) to queued.
func (r *ImportRepo) RequeueStale(ctx context.Context) error {
	_, err := r.exec(ctx, `UPDATE imports SET status='queued', updated_at=? WHERE status='running'`, db.Millis(time.Now()))
	return err
}

const importItemColumns = `id, import_id, position, source_title, source_artist, source_album, status,
	matched_track_id, resolved_title, resolved_artist, confidence, note, candidate_id, candidate_cover_art, created_at, updated_at`

func scanImportItem(s rowScanner) (models.ImportItem, error) {
	var it models.ImportItem
	var status string
	var created, updated int64
	if err := s.Scan(&it.ID, &it.ImportID, &it.Position, &it.SourceTitle, &it.SourceArtist, &it.SourceAlbum, &status,
		&it.MatchedTrackID, &it.ResolvedTitle, &it.ResolvedArtist, &it.Confidence, &it.Note, &it.CandidateID, &it.CandidateCoverArt, &created, &updated); err != nil {
		return it, err
	}
	it.Status = models.ImportItemStatus(status)
	it.CreatedAt = db.FromMillis(created)
	it.UpdatedAt = db.FromMillis(updated)
	return it, nil
}

// GetItem returns a single import item by id.
func (r *ImportRepo) GetItem(ctx context.Context, id string) (models.ImportItem, error) {
	row := r.queryRow(ctx, `SELECT `+importItemColumns+` FROM import_items WHERE id=?`, id)
	it, err := scanImportItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return it, ErrNotFound
	}
	return it, err
}

// InsertItems bulk-inserts import items in one transaction.
func (r *ImportRepo) InsertItems(ctx context.Context, items []models.ImportItem) error {
	if len(items) == 0 {
		return nil
	}
	return r.withTx(ctx, func(tx *sql.Tx) error {
		for _, it := range items {
			if _, err := tx.ExecContext(ctx, r.rebind(`INSERT INTO import_items (`+importItemColumns+`)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
				it.ID, it.ImportID, it.Position, it.SourceTitle, it.SourceArtist, it.SourceAlbum, string(it.Status),
				it.MatchedTrackID, it.ResolvedTitle, it.ResolvedArtist, it.Confidence, it.Note, it.CandidateID, it.CandidateCoverArt,
				db.Millis(it.CreatedAt), db.Millis(it.UpdatedAt)); err != nil {
				return err
			}
		}
		return nil
	})
}

// UpdateItem writes the outcome of resolving one item.
func (r *ImportRepo) UpdateItem(ctx context.Context, it models.ImportItem) error {
	_, err := r.exec(ctx, `UPDATE import_items SET status=?, matched_track_id=?, resolved_title=?,
		resolved_artist=?, confidence=?, note=?, candidate_id=?, candidate_cover_art=?, updated_at=? WHERE id=?`,
		string(it.Status), it.MatchedTrackID, it.ResolvedTitle, it.ResolvedArtist, it.Confidence, it.Note, it.CandidateID, it.CandidateCoverArt,
		db.Millis(time.Now()), it.ID)
	return err
}

// ListItems returns an import's items in order.
func (r *ImportRepo) ListItems(ctx context.Context, importID string) ([]models.ImportItem, error) {
	rows, err := r.query(ctx, `SELECT `+importItemColumns+` FROM import_items WHERE import_id=? ORDER BY position`, importID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ImportItem
	for rows.Next() {
		it, err := scanImportItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
