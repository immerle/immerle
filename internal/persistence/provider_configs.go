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
	rows, err := r.bquery(ctx, r.mel.New("provider_configs").Select(providerConfigColumns).
		OrderBy("sort_order", "").OrderBy("name", ""))
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
	row := r.bqueryRow(ctx, r.mel.New("provider_configs").Select(providerConfigColumns).Where("name", "=", name))
	p, err := scanProviderConfig(row)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// Upsert preserves created_at on conflict: every column except name and
// created_at is flagged UpdateDuplicateKey, so they stay out of the SET clause.
func (r *ProviderConfigRepo) Upsert(ctx context.Context, p models.ProviderConfig) error {
	now := db.Millis(time.Now())
	_, err := r.bexec(ctx, r.mel.NewInsert("provider_configs").
		Set("name", p.Name).
		Set("kind", p.Kind).UpdateDuplicateKey().
		Set("endpoint", p.Endpoint).UpdateDuplicateKey().
		Set("config", p.Config).UpdateDuplicateKey().
		Set("enabled", db.Bool(p.Enabled)).UpdateDuplicateKey().
		Set("sort_order", p.SortOrder).UpdateDuplicateKey().
		Set("created_at", now).
		Set("updated_at", now).UpdateDuplicateKey().
		OnConflict("name"))
	return err
}

// SetEnabled toggles a provider's enabled flag.
func (r *ProviderConfigRepo) SetEnabled(ctx context.Context, name string, enabled bool) error {
	res, err := r.bexec(ctx, r.mel.NewUpdate("provider_configs").
		Set("enabled", db.Bool(enabled)).Set("updated_at", db.Millis(time.Now())).Where("name", "=", name))
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
	res, err := r.bexec(ctx, r.mel.NewUpdate("provider_configs").
		Set("sort_order", order).Set("updated_at", db.Millis(time.Now())).Where("name", "=", name))
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
	res, err := r.bexec(ctx, r.mel.NewDelete("provider_configs").Where("name", "=", name))
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}
