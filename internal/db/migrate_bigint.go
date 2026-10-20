package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// The SQL migrations declare integers as INTEGER, which SQLite stores as 64-bit
// but Postgres maps to int4: millisecond timestamps overflow it. This widens
// every int4 column to bigint on Postgres and is a no-op on SQLite.
func init() {
	goose.AddNamedMigrationNoTxContext("00049_postgres_bigint.go", upPostgresBigint, nil)
}

func upPostgresBigint(ctx context.Context, sqlDB *sql.DB) error {
	if _, ok := sqlDB.Driver().(*stdlib.Driver); !ok {
		return nil
	}
	rows, err := sqlDB.QueryContext(ctx, `
		SELECT table_name, column_name FROM information_schema.columns
		WHERE table_schema = current_schema() AND data_type = 'integer'
		  AND table_name <> 'goose_db_version'`)
	if err != nil {
		return fmt.Errorf("list int4 columns: %w", err)
	}
	var stmts []string
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			_ = rows.Close()
			return err
		}
		stmts = append(stmts, fmt.Sprintf("ALTER TABLE %q ALTER COLUMN %q TYPE BIGINT", table, column))
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", stmt, err)
		}
	}
	return tx.Commit()
}
