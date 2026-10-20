package sonic

import (
	"context"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/immerle/immerle/internal/persistence"
)

func TestDuplicatesGroupsIdenticalRecordings(t *testing.T) {
	vector := func() []float32 {
		v := make([]float32, 8+2*numMFCC)
		for d := range v {
			v[d] = float32(rand.NormFloat64())
		}
		return v
	}
	nudge := func(v []float32) []float32 { // a re-encode: nearly the same features
		out := slices.Clone(v)
		for d := range out {
			out[d] += float32(rand.NormFloat64() * 0.01)
		}
		return out
	}
	song, other := vector(), vector()
	rows := []persistence.SonicVector{
		{TrackID: "flac", Duration: 200, Vector: song},
		{TrackID: "mp3", Duration: 201, Vector: nudge(song)},
		{TrackID: "radio-edit", Duration: 180, Vector: nudge(song)}, // same sound, other length
		{TrackID: "other", Duration: 200, Vector: other},
		{TrackID: "unknown-length", Duration: 0, Vector: song},
	}
	for range 30 { // a library to normalize against
		rows = append(rows, persistence.SonicVector{TrackID: "x" + string(rune('a'+len(rows))), Duration: 100 + len(rows), Vector: vector()})
	}
	x := newIndex(nil)
	x.load(rows)
	x.built = time.Now()

	groups, err := x.duplicates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || !slices.Equal(groups[0], []string{"flac", "mp3"}) {
		t.Fatalf("got %v, want only [flac mp3]", groups)
	}
}
