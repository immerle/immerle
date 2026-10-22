package discover

import (
	"slices"
	"testing"
	"time"

	"github.com/immerle/immerle/internal/models"
)

func TestNewPlayAlwaysCarriesAnID(t *testing.T) {
	at := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	if got := newPlay(models.Track{ID: "t1", ArtistName: "Ninho", Title: "Destin"}, at).IDs; !slices.Equal(got, []string{"local:t1"}) {
		t.Fatalf("ids without mbid = %v, want [local:t1]", got)
	}
	if got := newPlay(models.Track{ID: "t1", MBID: "m1"}, at).IDs; !slices.Equal(got, []string{"local:t1", "mbid:m1"}) {
		t.Fatalf("ids with mbid = %v", got)
	}
}
