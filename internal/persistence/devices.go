package persistence

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/immerle/immerle/internal/db"
	"github.com/immerle/immerle/internal/models"
)

// DeviceRepo persists device sessions (JWT jti registry + revocation + tracking).
type DeviceRepo struct{ *base }

const deviceColumns = `id, user_id, name, user_agent, created_at, last_seen_at, last_ip, expires_at, revoked`

func scanDevice(s rowScanner) (models.Device, error) {
	var d models.Device
	var created int64
	var lastSeen, expires sql.NullInt64
	var revoked int
	if err := s.Scan(&d.ID, &d.UserID, &d.Name, &d.UserAgent, &created, &lastSeen, &d.LastIP, &expires, &revoked); err != nil {
		return d, err
	}
	d.CreatedAt = db.FromMillis(created)
	d.LastSeenAt = db.TimePtr(lastSeen)
	d.ExpiresAt = db.TimePtr(expires)
	d.Revoked = revoked != 0
	return d, nil
}

// Create registers a device.
func (r *DeviceRepo) Create(ctx context.Context, d models.Device) error {
	_, err := r.exec(ctx, `INSERT INTO devices (`+deviceColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.UserID, d.Name, d.UserAgent, db.Millis(d.CreatedAt), db.NullMillis(d.LastSeenAt),
		d.LastIP, db.NullMillis(d.ExpiresAt), db.Bool(d.Revoked))
	return err
}

// Get returns a device by id (jti), regardless of revoked state.
func (r *DeviceRepo) Get(ctx context.Context, id string) (models.Device, error) {
	row := r.queryRow(ctx, `SELECT `+deviceColumns+` FROM devices WHERE id=?`, id)
	d, err := scanDevice(row)
	if errors.Is(err, sql.ErrNoRows) {
		return d, ErrNotFound
	}
	return d, err
}

// ListByUser returns a user's active (non-revoked) devices, most recent first.
func (r *DeviceRepo) ListByUser(ctx context.Context, userID string) ([]models.Device, error) {
	rows, err := r.query(ctx, `SELECT `+deviceColumns+` FROM devices WHERE user_id=? AND revoked=0 ORDER BY last_seen_at DESC, created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Device
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Revoke marks a device revoked (owner-scoped). Returns whether a row matched.
func (r *DeviceRepo) Revoke(ctx context.Context, id, userID string) (bool, error) {
	res, err := r.exec(ctx, `UPDATE devices SET revoked=1 WHERE id=? AND user_id=?`, id, userID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// TouchSeen records last-seen time and IP (best effort).
func (r *DeviceRepo) TouchSeen(ctx context.Context, id, ip string, at time.Time) error {
	_, err := r.exec(ctx, `UPDATE devices SET last_seen_at=?, last_ip=? WHERE id=?`, db.Millis(at), ip, id)
	return err
}
