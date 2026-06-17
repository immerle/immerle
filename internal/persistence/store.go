// Package persistence implements repositories over the database. Each repository
// owns one aggregate; the Store groups them and shares the connection pool.
package persistence

import (
	"context"
	"database/sql"
	"errors"

	"github.com/immerle/immerle/internal/db"
)

// ErrNotFound is returned when a lookup matches no row.
var ErrNotFound = errors.New("not found")

// Store groups all repositories.
type Store struct {
	db *db.DB

	Users           *UserRepo
	Catalog         *CatalogRepo
	Genres          *GenreRepo
	Annotations     *AnnotationRepo
	Playlists       *PlaylistRepo
	PlayQueues      *PlayQueueRepo
	Scrobbles       *ScrobbleRepo
	Shares          *ShareRepo
	Friends         *FriendRepo
	Activity        *ActivityRepo
	Jam             *JamRepo
	Downloads       *DownloadRepo
	ProviderCache   *ProviderCacheRepo
	ProviderConfigs *ProviderConfigRepo
	APITokens       *APITokenRepo
	Devices         *DeviceRepo
	Imports         *ImportRepo
}

// New builds a Store over the given database.
func New(database *db.DB) *Store {
	base := &base{db: database}
	return &Store{
		db:              database,
		Users:           &UserRepo{base},
		Catalog:         &CatalogRepo{base},
		Genres:          &GenreRepo{base},
		Annotations:     &AnnotationRepo{base},
		Playlists:       &PlaylistRepo{base},
		PlayQueues:      &PlayQueueRepo{base},
		Scrobbles:       &ScrobbleRepo{base},
		Shares:          &ShareRepo{base},
		Friends:         &FriendRepo{base},
		Activity:        &ActivityRepo{base},
		Jam:             &JamRepo{base},
		Downloads:       &DownloadRepo{base},
		ProviderCache:   &ProviderCacheRepo{base},
		ProviderConfigs: &ProviderConfigRepo{base},
		APITokens:       &APITokenRepo{base},
		Devices:         &DeviceRepo{base},
		Imports:         &ImportRepo{base},
	}
}

// DB exposes the underlying database (for transactions/migrations).
func (s *Store) DB() *db.DB { return s.db }

// base is the shared, generic repository foundation. It centralizes query
// rebinding and a small set of execution helpers reused by all repositories.
type base struct {
	db *db.DB
}

func (b *base) exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return b.db.ExecContext(ctx, b.db.Rebind(query), args...)
}

func (b *base) query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return b.db.QueryContext(ctx, b.db.Rebind(query), args...)
}

func (b *base) queryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return b.db.QueryRowContext(ctx, b.db.Rebind(query), args...)
}

// withTx runs fn inside a transaction, committing on success and rolling back on
// error or panic.
func (b *base) withTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// rebind exposes dialect placeholder rewriting to repositories building queries
// that they run against a *sql.Tx (which does not pass through base.exec).
func (b *base) rebind(query string) string { return b.db.Rebind(query) }
