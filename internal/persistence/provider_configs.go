package persistence

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/immerle/immerle/internal/db"
	"github.com/immerle/immerle/internal/models"
)

// ProviderConfigRepo persists admin-managed, runtime-configurable providers.
type ProviderConfigRepo struct{ *base }

const providerConfigColumns = `name, kind, endpoint, config, enabled, sort_order, created_at, updated_at`

func scanProviderConfig(s rowScanner) (models.ProviderConfig, error) {
	var p models.ProviderConfig
	var enabled int
	var created, updated int64
	if err := s.Scan(&p.Name, &p.Kind, &p.Endpoint, &p.Config, &enabled, &p.SortOrder, &created, &updated); err != nil {
		return p, err
	}
	p.Enabled = enabled != 0
	p.CreatedAt = db.FromMillis(created)
	p.UpdatedAt = db.FromMillis(updated)
	return p, nil
}

// List returns all provider configs ordered by sort order then name.
func (r *ProviderConfigRepo) List(ctx context.Context) ([]models.ProviderConfig, error) {
	rows, err := r.query(ctx, `SELECT `+providerConfigColumns+` FROM provider_configs ORDER BY sort_order, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ProviderConfig
	for rows.Next() {
		p, err := scanProviderConfig(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Get returns a provider config by name.
func (r *ProviderConfigRepo) Get(ctx context.Context, name string) (models.ProviderConfig, error) {
	row := r.queryRow(ctx, `SELECT `+providerConfigColumns+` FROM provider_configs WHERE name=?`, name)
	p, err := scanProviderConfig(row)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// Upsert inserts or updates a provider config, preserving created_at on update.
func (r *ProviderConfigRepo) Upsert(ctx context.Context, p models.ProviderConfig) error {
	now := time.Now()
	_, err := r.exec(ctx, `INSERT INTO provider_configs (`+providerConfigColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			kind=excluded.kind,
			endpoint=excluded.endpoint,
			config=excluded.config,
			enabled=excluded.enabled,
			sort_order=excluded.sort_order,
			updated_at=excluded.updated_at`,
		p.Name, p.Kind, p.Endpoint, p.Config, db.Bool(p.Enabled), p.SortOrder, db.Millis(now), db.Millis(now))
	return err
}

// SetEnabled toggles a provider's enabled flag.
func (r *ProviderConfigRepo) SetEnabled(ctx context.Context, name string, enabled bool) error {
	res, err := r.exec(ctx, `UPDATE provider_configs SET enabled=?, updated_at=? WHERE name=?`,
		db.Bool(enabled), db.Millis(time.Now()), name)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetOrder sets a provider's sort order.
func (r *ProviderConfigRepo) SetOrder(ctx context.Context, name string, order int) error {
	res, err := r.exec(ctx, `UPDATE provider_configs SET sort_order=?, updated_at=? WHERE name=?`,
		order, db.Millis(time.Now()), name)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a provider config.
func (r *ProviderConfigRepo) Delete(ctx context.Context, name string) error {
	res, err := r.exec(ctx, `DELETE FROM provider_configs WHERE name=?`, name)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}
