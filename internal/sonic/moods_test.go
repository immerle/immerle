package sonic

import (
	"fmt"
	"testing"

	"github.com/immerle/immerle/internal/persistence"
)

// library spreads 100 tracks evenly over energy, danceability and tempo,
// 10 artists, every other track in a minor key.
func library() (map[string]persistence.SonicSummary, []persistence.SonicSummary) {
	byID := map[string]persistence.SonicSummary{}
	var all []persistence.SonicSummary
	for i := range 100 {
		mode := "major"
		if i%2 == 0 {
			mode = "minor"
		}
		t := persistence.SonicSummary{
			TrackID: fmt.Sprintf("t%02d", i), ArtistID: fmt.Sprintf("a%d", i%10),
			Energy: float64(i) / 100, Danceability: float64((i*37)%100) / 100, BPM: 70 + float64(i), Mode: mode,
		}
		byID[t.TrackID] = t
		all = append(all, t)
	}
	return byID, all
}

func TestPickMoodsFollowsTheRules(t *testing.T) {
	byID, all := library()
	picks := PickMoods(all, 20)

	check := func(m Mood, ok func(persistence.SonicSummary) bool) {
		t.Helper()
		if len(picks[m]) == 0 {
			t.Errorf("%s: empty", m)
		}
		for _, id := range picks[m] {
			if !ok(byID[id]) {
				t.Errorf("%s: unexpected track %+v", m, byID[id])
			}
		}
	}
	check(MoodCalm, func(s persistence.SonicSummary) bool { return s.Energy <= 0.3 })
	check(MoodEnergetic, func(s persistence.SonicSummary) bool { return s.Energy >= 0.69 && s.BPM >= 115 })
	check(MoodMelancholy, func(s persistence.SonicSummary) bool { return s.Mode == "minor" && s.Energy <= 0.5 })
	check(MoodWorkout, func(s persistence.SonicSummary) bool { return s.BPM >= 120 && s.BPM <= 160 })

	// The calmest track leads the calm playlist.
	if picks[MoodCalm][0] != "t00" {
		t.Errorf("calm should start with the lowest energy, got %v", picks[MoodCalm])
	}
}

func TestPickMoodsCapsArtistsAndSize(t *testing.T) {
	_, all := library()
	for m, ids := range PickMoods(all, 5) {
		if len(ids) > 5 {
			t.Errorf("%s: %d tracks", m, len(ids))
		}
		perArtist := map[string]int{}
		for _, id := range ids {
			perArtist[id[len(id)-1:]]++ // artist = last digit of the track id
		}
		for a, n := range perArtist {
			if n > moodMaxPerArtist {
				t.Errorf("%s: artist %s has %d tracks", m, a, n)
			}
		}
	}
}

func TestPickMoodsEmptyLibrary(t *testing.T) {
	for m, ids := range PickMoods(nil, 20) {
		if len(ids) != 0 {
			t.Errorf("%s: %v", m, ids)
		}
	}
}
