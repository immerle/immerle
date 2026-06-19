// Package radio holds the curated built-in internet radio stations, organized by
// country. Each country has its own review-friendly stations.json plus a covers/
// folder of logo images; everything is embedded into the binary at build time,
// so adding a station (or a country) is a one-folder change and the logos ship
// with the server (no runtime hotlinking).
package radio

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/immerle/immerle/internal/models"
)

//go:embed fr es gb us ch int
var radioFS embed.FS

// countries is the fixed display order of the built-in country groups.
var countries = []string{"fr", "es", "gb", "us", "ch", "int"}

// seedStation mirrors one entry of a country's stations.json. Verified is
// documentation only (false flags streams not yet confirmed reachable); Logo is
// a filename relative to the country's covers/ folder.
type seedStation struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	StreamURL   string `json:"streamUrl"`
	HomepageURL string `json:"homepageUrl"`
	Logo        string `json:"logo"`
	Verified    *bool  `json:"verified"`
}

// coverRefPrefix marks a CoverArt value that points at an embedded logo file
// (vs. an external URL set on a custom station).
const coverRefPrefix = "embed:"

// Builtins returns the curated stations across all countries, in country order
// then file order. It panics on a malformed embedded file: the JSON ships in the
// binary, so a parse error is a build/release bug — failing fast surfaces it at
// startup rather than silently serving no stations.
func Builtins() []models.RadioStation {
	var out []models.RadioStation
	for _, cc := range countries {
		data, err := radioFS.ReadFile(cc + "/stations.json")
		if err != nil {
			panic(fmt.Sprintf("radio: missing embedded %s/stations.json: %v", cc, err))
		}
		var seeds []seedStation
		if err := json.Unmarshal(data, &seeds); err != nil {
			panic(fmt.Sprintf("radio: invalid %s/stations.json: %v", cc, err))
		}
		for _, s := range seeds {
			cover := ""
			if s.Logo != "" {
				cover = coverRefPrefix + cc + "/covers/" + s.Logo
			}
			out = append(out, models.RadioStation{
				ID:          s.ID,
				Name:        s.Name,
				StreamURL:   s.StreamURL,
				HomepageURL: s.HomepageURL,
				Country:     cc,
				CoverArt:    cover,
			})
		}
	}
	return out
}

// CoverFile resolves an "embed:<path>" CoverArt reference to its embedded image
// bytes and content type. Returns ok=false for non-embedded refs (external URLs).
func CoverFile(ref string) (data []byte, contentType string, ok bool) {
	if !strings.HasPrefix(ref, coverRefPrefix) {
		return nil, "", false
	}
	path := strings.TrimPrefix(ref, coverRefPrefix)
	b, err := radioFS.ReadFile(path)
	if err != nil {
		return nil, "", false
	}
	// Built-in covers are standardized as 64x64 WebP.
	return b, "image/webp", true
}
