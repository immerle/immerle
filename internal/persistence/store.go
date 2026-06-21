// Package persistence implements repositories over the database. Each repository
// owns one aggregate; the Store groups them and shares the connection pool.
package persistence

import (
	"context"
	"database/sql"
	"errors"

	melody "github.com/ermos/melody/v2"

	"github.com/immerle/immerle/internal/db"
)

// ErrNotFound is returned when a lookup matches no row.
var ErrNotFound = errors.New("not found")

// rowScanner is satisfied by both *sql.Row and *sql.Rows, so scan helpers work
// with either a single-row lookup or an iterated result set.
type rowScanner interface{ Scan(...any) error }

// Store groups all repositories.
type Store struct {
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
	ProviderConfigs *ProviderConfigRepo
	ProviderLogs    *ProviderLogRepo
	APITokens       *APITokenRepo
	Devices         *DeviceRepo
	Imports         *ImportRepo
	SmartPlaylists  *SmartPlaylistRepo
	Radio           *RadioRepo
	Wrapped         *WrappedRepo
	Podcasts        *PodcastRepo
	HubOutbox       *HubOutboxRepo
	PlaylistSync    *PlaylistSyncRepo
	CoverUploads    *CoverUploadRepo
}

// New builds a Store over the given database.
func New(database *db.DB) *Store {
	base := &base{db: database, mel: melody.With(melodyDialect(database.Dialect))}
	return &Store{
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
		ProviderConfigs: &ProviderConfigRepo{base},
		ProviderLogs:    &ProviderLogRepo{base},
		APITokens:       &APITokenRepo{base},
		Devices:         &DeviceRepo{base},
		Imports:         &ImportRepo{base},
		SmartPlaylists:  &SmartPlaylistRepo{base},
		Radio:           &RadioRepo{base},
		Wrapped:         &WrappedRepo{base},
		Podcasts:        &PodcastRepo{base},
		HubOutbox:       &HubOutboxRepo{base},
		PlaylistSync:    &PlaylistSyncRepo{base},
		CoverUploads:    &CoverUploadRepo{base},
	}
}

// melodyDialect maps the DB driver dialect to the melody SQL-builder dialect.
func melodyDialect(d string) melody.Dialect {
	if d == "postgres" {
		return melody.Postgres
	}
	return melody.SQLite
}

// base is the shared, generic repository foundation. It centralizes query
// rebinding and a small set of execution helpers reused by all repositories.
type base struct {
	db *db.DB
	// mel is the melody SQL-builder factory, pre-configured with the connection's
	// dialect so builders render native placeholders ($1.. for postgres, ?..
	// for sqlite). Builder output flows through the same exec/query helpers as the
	// remaining hand-written SQL: db.Rebind only rewrites "?", so already-native
	// builder SQL passes through unchanged.
	mel *melody.Melody
}

// sqlBuilder is satisfied by every melody builder (select/insert/update/delete);
// it lets the b* helpers run a built query without each caller unpacking Get().
type sqlBuilder interface {
	Get() (string, []any, error)
}

// bexec runs a builder as a statement (INSERT/UPDATE/DELETE).
func (b *base) bexec(ctx context.Context, sb sqlBuilder) (sql.Result, error) {
	q, args, err := sb.Get()
	if err != nil {
		return nil, err
	}
	return b.exec(ctx, q, args...)
}

// bquery runs a builder as a multi-row SELECT.
func (b *base) bquery(ctx context.Context, sb sqlBuilder) (*sql.Rows, error) {
	q, args, err := sb.Get()
	if err != nil {
		return nil, err
	}
	return b.query(ctx, q, args...)
}

// bqueryRow runs a builder as a single-row SELECT. A builder Get() error is a
// programming mistake (e.g. no columns), surfaced as a failed query so the
// caller's Scan returns it. ponytail: build-time builder errors are caught by
// the tests, not a runtime data path.
func (b *base) bqueryRow(ctx context.Context, sb sqlBuilder) *sql.Row {
	q, args, err := sb.Get()
	if err != nil {
		return b.queryRow(ctx, "SELECT /* builder error: "+err.Error()+" */ 1 WHERE 0=1")
	}
	return b.queryRow(ctx, q, args...)
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
