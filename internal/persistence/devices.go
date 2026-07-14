package persistence

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"sync"
	"time"

	melody "github.com/ermos/melody/v2"

	"github.com/immerle/immerle/internal/db"
	"github.com/immerle/immerle/internal/models"
)

// DeviceRepo persists device sessions (JWT jti registry + revocation + tracking).
type DeviceRepo struct {
	*base
	// seenMu/pending buffer TouchSeen writes in memory instead of hitting the DB
	// on every authenticated request; FlushSeen persists them in one batch. Get/
	// ListByUser overlay pending entries so reads still see the freshest state.
	seenMu  sync.Mutex
	pending map[string]models.Device
}

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
	if err != nil {
		return d, err
	}
	return r.withPending(d), nil
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
		out = append(out, r.withPending(d))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// A pending (not yet flushed) touch can make a device fresher than what the
	// DB-ordered query above reflects; re-sort so "most recent first" still holds.
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].LastSeenAt, out[j].LastSeenAt
		switch {
		case a == nil && b == nil:
			return false
		case a == nil:
			return false
		case b == nil:
			return true
		default:
			return a.After(*b)
		}
	})
	return out, nil
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

// TouchSeen buffers a device's last-seen time/IP in memory rather than
// writing to the DB on every authenticated request. dev is the caller's
// current view of the device (already fetched via Get), so a flush can
// recreate its row from this snapshot if it's ever missing. Call FlushSeen
// (on shutdown) to persist buffered touches.
func (r *DeviceRepo) TouchSeen(_ context.Context, dev models.Device, ip string, at time.Time) error {
	dev.LastSeenAt = &at
	dev.LastIP = ip
	r.seenMu.Lock()
	if r.pending == nil {
		r.pending = make(map[string]models.Device)
	}
	r.pending[dev.ID] = dev
	r.seenMu.Unlock()
	return nil
}

// FlushSeen persists every buffered last-seen touch to the DB. A hard kill
// between flushes only loses those touches — a device just shows "offline" a
// little longer, self-healing on its next request.
func (r *DeviceRepo) FlushSeen(ctx context.Context) error {
	r.seenMu.Lock()
	pending := r.pending
	r.pending = nil
	r.seenMu.Unlock()

	for _, dev := range pending {
		res, err := r.bexec(ctx, r.mel.NewUpdate("devices").
			Set("last_seen_at", db.NullMillis(dev.LastSeenAt)).Set("last_ip", dev.LastIP).
			Where("id", "=", dev.ID))
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			// The row disappeared between the original Get and this flush (or
			// never existed) — recreate it from the cached snapshot.
			if err := r.Create(ctx, dev); err != nil {
				return err
			}
		}
	}
	return nil
}

// withPending overlays a not-yet-flushed touch onto a DB-scanned row, so reads
// reflect the freshest last-seen state even before the next FlushSeen.
func (r *DeviceRepo) withPending(d models.Device) models.Device {
	r.seenMu.Lock()
	p, ok := r.pending[d.ID]
	r.seenMu.Unlock()
	if ok {
		d.LastSeenAt = p.LastSeenAt
		d.LastIP = p.LastIP
	}
	return d
}
