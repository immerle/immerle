package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	melody "github.com/ermos/melody/v2"

	"github.com/immerle/immerle/internal/db"
	"github.com/immerle/immerle/internal/models"
)

// ---- Play queues ----

// PlayQueueRepo persists per-user server-side play queues.
type PlayQueueRepo struct{ *base }

// Save stores the user's queue.
func (r *PlayQueueRepo) Save(ctx context.Context, q models.PlayQueue) error {
	ids := strings.Join(q.TrackIDs, ",")
	entriesJSON, err := json.Marshal(q.Entries)
	if err != nil {
		return err
	}
	_, err = r.bexec(ctx, r.mel.NewInsert("play_queues").
		Set("user_id", q.UserID).
		Set("track_ids", ids).UpdateDuplicateKey().
		Set("entries_json", string(entriesJSON)).UpdateDuplicateKey().
		Set("current", q.Current).UpdateDuplicateKey().
		Set("position_ms", q.PositionMs).UpdateDuplicateKey().
		Set("playing", db.Bool(q.Playing)).UpdateDuplicateKey().
		Set("changed_by", q.ChangedBy).UpdateDuplicateKey().
		Set("changed_at", db.Millis(q.ChangedAt)).UpdateDuplicateKey().
		OnConflict("user_id"))
	return err
}

// Get returns the user's queue, or ErrNotFound.
func (r *PlayQueueRepo) Get(ctx context.Context, userID string) (models.PlayQueue, error) {
	var q models.PlayQueue
	var ids, entriesJSON string
	var changedAt int64
	var playing int
	var targetChangedAt sql.NullInt64
	err := r.bqueryRow(ctx, r.mel.New("play_queues").
		Select("user_id", "track_ids", "entries_json", "current", "position_ms", "playing", "changed_by", "changed_at", "target_device_id", "target_changed_at").
		Where("user_id", "=", userID)).Scan(&q.UserID, &ids, &entriesJSON, &q.Current, &q.PositionMs, &playing, &q.ChangedBy, &changedAt, &q.TargetDeviceID, &targetChangedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return q, ErrNotFound
	}
	if err != nil {
		return q, err
	}
	if ids != "" {
		q.TrackIDs = strings.Split(ids, ",")
	}
	if entriesJSON != "" {
		_ = json.Unmarshal([]byte(entriesJSON), &q.Entries)
	}
	q.Playing = playing != 0
	q.ChangedAt = db.FromMillis(changedAt)
	q.TargetChangedAt = db.TimePtr(targetChangedAt)
	return q, nil
}

// SetTarget assigns (or clears, with an empty deviceID) the device that
// should be the sole active player for the user's queue. Independent of
// Save so a normal playback-position sync never clobbers it.
func (r *PlayQueueRepo) SetTarget(ctx context.Context, userID, deviceID string) error {
	_, err := r.bexec(ctx, r.mel.NewInsert("play_queues").
		Set("user_id", userID).
		Set("changed_at", db.Millis(time.Now())).
		Set("target_device_id", deviceID).UpdateDuplicateKey().
		Set("target_changed_at", db.Millis(time.Now())).UpdateDuplicateKey().
		OnConflict("user_id"))
	return err
}

// ---- Scrobbles ----

// ScrobbleRepo persists scrobble submissions.
type ScrobbleRepo struct{ *base }

// Insert records a scrobble.
func (r *ScrobbleRepo) Insert(ctx context.Context, s models.Scrobble) error {
	_, err := r.bexec(ctx, r.mel.NewInsert("scrobbles").
		Set("id", s.ID).Set("user_id", s.UserID).Set("track_id", s.TrackID).
		Set("played_at", db.Millis(s.PlayedAt)).Set("submitted", db.Bool(s.Submitted)).Set("exported", db.Bool(s.Exported)))
	return err
}

// Unexported returns scrobbles not yet pushed to the hub.
func (r *ScrobbleRepo) Unexported(ctx context.Context, limit int) ([]models.Scrobble, error) {
	rows, err := r.bquery(ctx, r.mel.New("scrobbles").
		Select("id", "user_id", "track_id", "played_at", "submitted", "exported").
		Where("exported", "=", 0).Where("submitted", "=", 1).OrderBy("played_at", melody.Asc).Limit(limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Scrobble
	for rows.Next() {
		var s models.Scrobble
		var playedAt int64
		var submitted, exported int
		if err := rows.Scan(&s.ID, &s.UserID, &s.TrackID, &playedAt, &submitted, &exported); err != nil {
			return nil, err
		}
		s.PlayedAt = db.FromMillis(playedAt)
		s.Submitted = submitted != 0
		s.Exported = exported != 0
		out = append(out, s)
	}
	return out, rows.Err()
}

// MarkExported flags scrobbles as exported to the hub.
func (r *ScrobbleRepo) MarkExported(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	// Single statement so the flag flips atomically — a mid-batch failure must
	// not leave some scrobbles marked exported and others not.
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	_, err := r.bexec(ctx, r.mel.NewUpdate("scrobbles").Set("exported", 1).Where("id", "IN", args...))
	return err
}

// ---- Shares ----

// ShareRepo persists public share links.
type ShareRepo struct{ *base }

// Create inserts a share.
func (r *ShareRepo) Create(ctx context.Context, s models.Share) error {
	_, err := r.bexec(ctx, r.mel.NewInsert("shares").
		Set("id", s.ID).Set("user_id", s.UserID).Set("item_type", string(s.ItemType)).Set("item_id", s.ItemID).
		Set("secret", s.Secret).Set("description", s.Description).Set("expires_at", db.NullMillis(s.ExpiresAt)).
		Set("created_at", db.Millis(s.CreatedAt)).Set("view_count", 0))
	return err
}

// Delete removes a share owned by user.
func (r *ShareRepo) Delete(ctx context.Context, id, userID string) error {
	_, err := r.bexec(ctx, r.mel.NewDelete("shares").Where("id", "=", id).Where("user_id", "=", userID))
	return err
}

func scanShare(s rowScanner) (models.Share, error) {
	var sh models.Share
	var itemType string
	var expires sql.NullInt64
	var created int64
	if err := s.Scan(&sh.ID, &sh.UserID, &itemType, &sh.ItemID, &sh.Secret, &sh.Description, &expires, &created, &sh.ViewCount); err != nil {
		return sh, err
	}
	sh.ItemType = models.ItemType(itemType)
	sh.ExpiresAt = db.TimePtr(expires)
	sh.CreatedAt = db.FromMillis(created)
	return sh, nil
}

const shareColumns = `id, user_id, item_type, item_id, secret, description, expires_at, created_at, view_count`

// GetBySecret returns a share by its public secret.
func (r *ShareRepo) GetBySecret(ctx context.Context, secret string) (models.Share, error) {
	row := r.bqueryRow(ctx, r.mel.New("shares").Select(shareColumns).Where("secret", "=", secret))
	sh, err := scanShare(row)
	if errors.Is(err, sql.ErrNoRows) {
		return sh, ErrNotFound
	}
	return sh, err
}

// ListByUser returns a user's shares.
func (r *ShareRepo) ListByUser(ctx context.Context, userID string) ([]models.Share, error) {
	rows, err := r.bquery(ctx, r.mel.New("shares").Select(shareColumns).Where("user_id", "=", userID).OrderBy("created_at", melody.Desc))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Share
	for rows.Next() {
		sh, err := scanShare(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sh)
	}
	return out, rows.Err()
}

// IncrementViews bumps a share's view counter.
func (r *ShareRepo) IncrementViews(ctx context.Context, id string) error {
	_, err := r.bexec(ctx, r.mel.NewUpdate("shares").
		SetRaw("view_count", "view_count + 1").Where("id", "=", id))
	return err
}

// Update changes a share's description and expiry (owner-scoped).
func (r *ShareRepo) Update(ctx context.Context, id, userID, description string, expiresAt *time.Time) error {
	_, err := r.bexec(ctx, r.mel.NewUpdate("shares").
		Set("description", description).Set("expires_at", db.NullMillis(expiresAt)).
		Where("id", "=", id).Where("user_id", "=", userID))
	return err
}
