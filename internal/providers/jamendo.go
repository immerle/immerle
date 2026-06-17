package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// JamendoProvider is an on-demand provider backed by Jamendo, a catalog of
// Creative-Commons-licensed music with a documented public API and free,
// authorized track downloads. A free client_id is required (no user account or
// DRM involved). See https://developer.jamendo.com/.
type JamendoProvider struct {
	clientID    string
	audioFormat string // jamendo audioformat: mp31, mp32 (default), ogg, flac
	baseURL     string
	http        *http.Client
}

// NewJamendoProvider builds a Jamendo provider.
func NewJamendoProvider(clientID, audioFormat, baseURL string) *JamendoProvider {
	if audioFormat == "" {
		audioFormat = "mp32"
	}
	if baseURL == "" {
		baseURL = "https://api.jamendo.com/v3.0"
	}
	return &JamendoProvider{
		clientID:    clientID,
		audioFormat: audioFormat,
		baseURL:     baseURL,
		http:        &http.Client{Timeout: 60 * time.Second},
	}
}

func init() {
	RegisterFactory("jamendo", func(settings map[string]string) (Provider, error) {
		clientID := setting(settings, "client_id", "")
		if clientID == "" {
			return nil, fmt.Errorf("jamendo: client_id is required (free key from developer.jamendo.com)")
		}
		return NewJamendoProvider(clientID, setting(settings, "audioformat", "mp32"), setting(settings, "base_url", "")), nil
	})
}

// Name implements Provider.
func (p *JamendoProvider) Name() string { return "jamendo" }

// MaxQuality implements Provider.
func (p *JamendoProvider) MaxQuality() string {
	switch p.audioFormat {
	case "flac":
		return "flac (lossless)"
	case "ogg":
		return "ogg vorbis"
	case "mp31":
		return "mp3 96k"
	default:
		return "mp3 VBR"
	}
}

// jamendoTrack is the subset of a Jamendo track we consume.
type jamendoTrack struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ArtistName    string `json:"artist_name"`
	AlbumName     string `json:"album_name"`
	Position      int    `json:"position"`
	ReleaseDate   string `json:"releasedate"`
	Duration      int    `json:"duration"`
	AudioDownload string `json:"audiodownload"`
	MusicInfo     struct {
		Tags struct {
			Genres []string `json:"genres"`
		} `json:"tags"`
	} `json:"musicinfo"`
}

type jamendoResponse struct {
	Headers struct {
		Status string `json:"status"`
		Error  string `json:"error_message"`
	} `json:"headers"`
	Results []jamendoTrack `json:"results"`
}

func (t jamendoTrack) toResult(format string) Result {
	suffix := "mp3"
	switch format {
	case "flac":
		suffix = "flac"
	case "ogg":
		suffix = "ogg"
	}
	genre := ""
	if len(t.MusicInfo.Tags.Genres) > 0 {
		genre = t.MusicInfo.Tags.Genres[0]
	}
	year := 0
	if len(t.ReleaseDate) >= 4 {
		year, _ = strconv.Atoi(t.ReleaseDate[:4])
	}
	return Result{
		ProviderTrackID: t.ID,
		Title:           t.Name,
		Artist:          t.ArtistName,
		Album:           t.AlbumName,
		AlbumArtist:     t.ArtistName,
		TrackNo:         t.Position,
		Year:            year,
		Duration:        t.Duration,
		Genre:           genre,
		Suffix:          suffix,
	}
}

func (p *JamendoProvider) query(ctx context.Context, params url.Values) (*jamendoResponse, error) {
	params.Set("client_id", p.clientID)
	params.Set("format", "json")
	params.Set("audioformat", p.audioFormat)
	u := p.baseURL + "/tracks/?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("jamendo: status %d", resp.StatusCode)
	}
	var out jamendoResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Headers.Status != "" && out.Headers.Status != "success" {
		return nil, fmt.Errorf("jamendo: %s", out.Headers.Error)
	}
	return &out, nil
}

// Search implements Provider.
func (p *JamendoProvider) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	if limit <= 0 {
		limit = 20
	}
	params := url.Values{}
	params.Set("search", query)
	params.Set("limit", strconv.Itoa(limit))
	params.Set("include", "musicinfo")
	params.Set("audiodownload_allowed", "true") // only downloadable tracks
	resp, err := p.query(ctx, params)
	if err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(resp.Results))
	for _, t := range resp.Results {
		if t.AudioDownload == "" {
			continue
		}
		out = append(out, t.toResult(p.audioFormat))
	}
	return out, nil
}

// Resolve implements Provider.
func (p *JamendoProvider) Resolve(ctx context.Context, providerTrackID string) (Result, error) {
	params := url.Values{}
	params.Set("id", providerTrackID)
	params.Set("include", "musicinfo")
	resp, err := p.query(ctx, params)
	if err != nil {
		return Result{}, err
	}
	if len(resp.Results) == 0 {
		return Result{}, fmt.Errorf("jamendo: track %q not found", providerTrackID)
	}
	return resp.Results[0].toResult(p.audioFormat), nil
}

// Download implements Provider by fetching the track's authorized download URL.
func (p *JamendoProvider) Download(ctx context.Context, providerTrackID string, w io.Writer) error {
	params := url.Values{}
	params.Set("id", providerTrackID)
	resp, err := p.query(ctx, params)
	if err != nil {
		return err
	}
	if len(resp.Results) == 0 || resp.Results[0].AudioDownload == "" {
		return fmt.Errorf("jamendo: no download URL for track %q", providerTrackID)
	}
	return p.fetch(ctx, resp.Results[0].AudioDownload, w)
}

func (p *JamendoProvider) fetch(ctx context.Context, rawURL string, w io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("jamendo: download status %d", resp.StatusCode)
	}
	_, err = io.Copy(w, resp.Body)
	return err
}
