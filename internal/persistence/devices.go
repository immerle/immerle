package persistence

import (
	"context"
	"database/sql"
	"errors"
	"time"

	melody "github.com/ermos/melody/v2"

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
	_, err := r.bexec(ctx, r.mel.NewInsert("devices").
		Set("id", d.ID).Set("user_id", d.UserID).Set("name", d.Name).Set("user_agent", d.UserAgent).
		Set("created_at", db.Millis(d.CreatedAt)).Set("last_seen_at", db.NullMillis(d.LastSeenAt)).
		Set("last_ip", d.LastIP).Set("expires_at", db.NullMillis(d.ExpiresAt)).Set("revoked", db.Bool(d.Revoked)))
	return err
}

// Get returns a device by id (jti), regardless of revoked state.
func (r *DeviceRepo) Get(ctx context.Context, id string) (models.Device, error) {
	row := r.bqueryRow(ctx, r.mel.New("devices").Select(deviceColumns).Where("id", "=", id))
	d, err := scanDevice(row)
	if errors.Is(err, sql.ErrNoRows) {
		return d, ErrNotFound
	}
	return d, err
}

// ListByUser returns a user's active (non-revoked) devices, most recent first.
func (r *DeviceRepo) ListByUser(ctx context.Context, userID string) ([]models.Device, error) {
	rows, err := r.bquery(ctx, r.mel.New("devices").Select(deviceColumns).
		Where("user_id", "=", userID).Where("revoked", "=", 0).
		OrderBy("last_seen_at", melody.Desc).OrderBy("created_at", melody.Desc))
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
	res, err := r.bexec(ctx, r.mel.NewUpdate("devices").Set("revoked", 1).
		Where("id", "=", id).Where("user_id", "=", userID))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// TouchSeen records last-seen time and IP (best effort).
func (r *DeviceRepo) TouchSeen(ctx context.Context, id, ip string, at time.Time) error {
	_, err := r.bexec(ctx, r.mel.NewUpdate("devices").
		Set("last_seen_at", db.Millis(at)).Set("last_ip", ip).Where("id", "=", id))
	return err
}
