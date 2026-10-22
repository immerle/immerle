package db

import (
	"database/sql"
	"time"
)

// Millis converts a time to unix milliseconds for storage.
func Millis(t time.Time) int64 {
	return t.UTC().UnixMilli()
}

// FromMillis converts stored unix milliseconds back to a UTC time.
func FromMillis(ms int64) time.Time {
	return time.UnixMilli(ms).UTC()
}

// NullMillis stores an optional time. A nil pointer becomes SQL NULL.
func NullMillis(t *time.Time) sql.NullInt64 {
	if t == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: Millis(*t), Valid: true}
}

// TimePtr converts a nullable stored timestamp back to a *time.Time.
func TimePtr(n sql.NullInt64) *time.Time {
	if !n.Valid {
		return nil
	}
	t := FromMillis(n.Int64)
	return &t
}

// Bool converts a Go bool to the integer representation used in storage.
func Bool(b bool) int {
	if b {
		return 1
	}
	return 0
}
