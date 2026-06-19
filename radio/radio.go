// Package radio holds the curated built-in internet radio stations. The list
// lives in an editable, review-friendly stations.json that is embedded into the
// binary at build time, so adding/editing a station is a one-file JSON change.
package radio

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/immerle/immerle/internal/models"
)

//go:embed stations.json
var stationsJSON []byte

// seedStation mirrors one entry of stations.json. Country is documentation only
// (used to group the file by region); it is not persisted.
type seedStation struct {
	ID          string `json:"id"`
	Country     string `json:"country"`
	Name        string `json:"name"`
	StreamURL   string `json:"streamUrl"`
	HomepageURL string `json:"homepageUrl"`
}

// Builtins returns the curated built-in stations parsed from the embedded JSON.
// It panics on a malformed file: the JSON ships inside the binary, so a parse
// error is a build/release bug, not a runtime condition — failing fast at
// startup surfaces it immediately rather than silently serving no stations.
func Builtins() []models.RadioStation {
	var seeds []seedStation
	if err := json.Unmarshal(stationsJSON, &seeds); err != nil {
		panic(fmt.Sprintf("radio: invalid embedded stations.json: %v", err))
	}
	out := make([]models.RadioStation, 0, len(seeds))
	for _, s := range seeds {
		out = append(out, models.RadioStation{
			ID:          s.ID,
			Name:        s.Name,
			StreamURL:   s.StreamURL,
			HomepageURL: s.HomepageURL,
		})
	}
	return out
}
