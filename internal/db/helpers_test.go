package db

import (
	"testing"
	"time"
)

func TestMillisRoundTrip(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 30, 0, 0, time.FixedZone("CEST", 2*3600))
	ms := Millis(now)
	got := FromMillis(ms)
	if !got.Equal(now) {
		t.Errorf("round trip = %v, want %v", got, now)
	}
	if got.Location() != time.UTC {
		t.Errorf("FromMillis should return UTC, got %v", got.Location())
	}
}

func TestNullMillisAndTimePtr(t *testing.T) {
	if n := NullMillis(nil); n.Valid {
		t.Errorf("NullMillis(nil).Valid = true, want false")
	}
	if p := TimePtr(NullMillis(nil)); p != nil {
		t.Errorf("TimePtr(invalid) = %v, want nil", p)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	n := NullMillis(&now)
	if !n.Valid {
		t.Fatal("NullMillis(&now).Valid = false, want true")
	}
	p := TimePtr(n)
	if p == nil || !p.Equal(now) {
		t.Errorf("TimePtr round trip = %v, want %v", p, now)
	}
}

func TestBool(t *testing.T) {
	if got := Bool(true); got != 1 {
		t.Errorf("Bool(true) = %d, want 1", got)
	}
	if got := Bool(false); got != 0 {
		t.Errorf("Bool(false) = %d, want 0", got)
	}
}
