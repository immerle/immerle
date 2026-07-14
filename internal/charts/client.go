package charts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// defaultBaseURL is kworb-net-api's raw GitHub content root for the Spotify
// chart data set (https://github.com/ermos/kworb-net-api/tree/main/data/spotify).
const defaultBaseURL = "https://raw.githubusercontent.com/ermos/kworb-net-api/main/data/spotify"

// maxChartResponseBytes caps an in-memory chart response — kworb chart files
// are small JSON (well under 100KB), this just guards against a hostile or
// broken redirect serving something huge.
const maxChartResponseBytes = 4 << 20 // 4 MiB

// client fetches chart JSON files. baseURL is overridable (tests point it at
// an httptest.Server) instead of a package-level default.
type client struct {
	baseURL string
	http    *http.Client
}

func newClient(baseURL string, hc *http.Client) *client {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &client{baseURL: strings.TrimRight(baseURL, "/"), http: hc}
}

// fetch retrieves and decodes "<slug>_weekly.json".
func (c *client) fetch(ctx context.Context, slug string) (kworbChart, error) {
	url := c.baseURL + "/" + slug + "_weekly.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return kworbChart{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return kworbChart{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return kworbChart{}, fmt.Errorf("charts: fetch %s: unexpected status %d", url, resp.StatusCode)
	}
	var out kworbChart
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxChartResponseBytes)).Decode(&out); err != nil {
		return kworbChart{}, fmt.Errorf("charts: decode %s: %w", url, err)
	}
	return out, nil
}
