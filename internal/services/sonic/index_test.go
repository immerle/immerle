package sonic

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/immerle/immerle/internal/persistence"
)

// clusteredIndex builds an index of three well-separated clusters ("calm",
// "loud", "fast"), 6 tracks each, spread over 3 artists per cluster.
func clusteredIndex() *index {
	centers := map[string][]float32{}
	for c, name := range []string{"calm", "loud", "fast"} {
		v := make([]float32, 8+2*numMFCC)
		for d := range v {
			v[d] = float32(rand.NormFloat64())
		}
		v[c] += 20 // push each cluster far along its own axis
		centers[name] = v
	}
	var rows []persistence.SonicVector
	for name, center := range centers {
		for i := range 6 {
			v := slices.Clone(center)
			for d := range v {
				v[d] += float32(rand.NormFloat64() * 0.1)
			}
			rows = append(rows, persistence.SonicVector{
				TrackID:  fmt.Sprintf("%s-%d", name, i),
				ArtistID: fmt.Sprintf("%s-artist-%d", name, i%3),
				Vector:   v,
			})
		}
	}
	x := newIndex(nil)
	x.load(rows)
	x.built = time.Now()
	return x
}

func TestSimilarStaysInTheSeedCluster(t *testing.T) {
	x := clusteredIndex()
	ids, ok, err := x.similar(context.Background(), []string{"calm-0"}, []string{"calm-1"}, 4)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if len(ids) != 4 {
		t.Fatalf("want 4 tracks, got %v", ids)
	}
	for _, id := range ids {
		if !strings.HasPrefix(id, "calm-") || id == "calm-0" || id == "calm-1" {
			t.Errorf("unexpected track %s in %v", id, ids)
		}
	}
}

func TestSimilarCapsTracksPerArtist(t *testing.T) {
	x := clusteredIndex()
	// 5 calm tracks remain, 2 per artist at most except when topping up.
	ids, _, _ := x.similar(context.Background(), []string{"calm-0"}, nil, 3)
	perArtist := map[string]int{}
	for _, id := range ids {
		perArtist[x.artists[x.pos[id]]]++
	}
	for a, n := range perArtist {
		if n > maxPerArtist {
			t.Errorf("artist %s has %d tracks in %v", a, n, ids)
		}
	}
}

func TestSimilarUnknownSeedFallsBack(t *testing.T) {
	x := clusteredIndex()
	if _, ok, err := x.similar(context.Background(), []string{"not-analyzed"}, nil, 5); ok || err != nil {
		t.Fatalf("an unanalyzed seed should report ok=false, got ok=%v err=%v", ok, err)
	}
}
