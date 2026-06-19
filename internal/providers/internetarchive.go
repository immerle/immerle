package providers

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// InternetArchiveProvider serves freely-distributable audio from the Internet
// Archive (archive.org): public-domain recordings, Creative Commons works and
// artist-sanctioned live music. No credentials and no DRM.
type InternetArchiveProvider struct {
	baseURL  string
	maxItems int
	http     *http.Client
}

// NewInternetArchiveProvider builds an Internet Archive provider.
func NewInternetArchiveProvider(baseURL string, maxItems int) *InternetArchiveProvider {
	if baseURL == "" {
		baseURL = "https://archive.org"
	}
	if maxItems <= 0 {
		maxItems = 8
	}
	return &InternetArchiveProvider{
		baseURL:  strings.TrimRight(baseURL, "/"),
		maxItems: maxItems,
		http:     NewHTTPClient(60 * time.Second),
	}
}

func init() {
	RegisterFactory("internet-archive", func(cfg Config) (Provider, error) {
		maxItems, _ := strconv.Atoi(cfg.Param("max_items", "8"))
		// base_url is hardcoded in the constructor; not configurable.
		return NewInternetArchiveProvider("", maxItems), nil
	})
}

// Name implements Provider.
func (p *InternetArchiveProvider) Name() string { return "internet-archive" }

// MaxQuality implements Provider.
func (p *InternetArchiveProvider) MaxQuality() string { return "original (varies: mp3/flac/ogg)" }

const iaSep = "|"

type iaSearchResponse struct {
	Response struct {
		Docs []struct {
			Identifier string `json:"identifier"`
			Title      string `json:"title"`
			Creator    any    `json:"creator"`
		} `json:"docs"`
	} `json:"response"`
}

type iaMetadata struct {
	Metadata struct {
		Title   string `json:"title"`
		Creator any    `json:"creator"`
		Date    string `json:"date"`
	} `json:"metadata"`
	Files []iaFile `json:"files"`
}

type iaFile struct {
	Name   string `json:"name"`
	Format string `json:"format"`
	Title  string `json:"title"`
	Track  string `json:"track"`
	Album  string `json:"album"`
	Artist string `json:"artist"`
	Length string `json:"length"`
}

func firstString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		if len(t) > 0 {
			if s, ok := t[0].(string); ok {
				return s
			}
		}
	}
	return ""
}

func isAudioFormat(f iaFile) (suffix string, ok bool) {
	name := strings.ToLower(f.Name)
	switch {
	case strings.HasSuffix(name, ".mp3"):
		return "mp3", true
	case strings.HasSuffix(name, ".flac"):
		return "flac", true
	case strings.HasSuffix(name, ".ogg"), strings.HasSuffix(name, ".oga"):
		return "ogg", true
	}
	return "", false
}

// Search implements Provider by finding audio items and expanding each into its
// audio files (capped to keep the number of metadata calls bounded).
func (p *InternetArchiveProvider) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	if limit <= 0 {
		limit = 20
	}
	params := url.Values{}
	params.Set("q", query+" AND mediatype:(audio)")
	params.Add("fl[]", "identifier")
	params.Add("fl[]", "title")
	params.Add("fl[]", "creator")
	params.Set("rows", strconv.Itoa(p.maxItems))
	params.Set("page", "1")
	params.Set("output", "json")

	var sr iaSearchResponse
	if err := p.getJSON(ctx, p.baseURL+"/advancedsearch.php?"+params.Encode(), &sr); err != nil {
		return nil, err
	}

	var out []Result
	for _, doc := range sr.Response.Docs {
		md, err := p.metadata(ctx, doc.Identifier)
		if err != nil {
			continue
		}
		for _, f := range md.Files {
			suffix, ok := isAudioFormat(f)
			if !ok {
				continue
			}
			out = append(out, iaResult(doc.Identifier, f, md, suffix))
			if len(out) >= limit {
				return out, nil
			}
		}
	}
	return out, nil
}

func iaResult(identifier string, f iaFile, md *iaMetadata, suffix string) Result {
	title := cmp.Or(f.Title, strings.TrimSuffix(f.Name, "."+suffix))
	artist := cmp.Or(f.Artist, firstString(md.Metadata.Creator), "Unknown Artist")
	album := cmp.Or(f.Album, md.Metadata.Title, identifier)
	track, _ := strconv.Atoi(f.Track)
	year := 0
	if len(md.Metadata.Date) >= 4 {
		year, _ = strconv.Atoi(md.Metadata.Date[:4])
	}
	duration := 0
	if secs, err := strconv.ParseFloat(f.Length, 64); err == nil {
		duration = int(secs)
	}
	return Result{
		ProviderTrackID: identifier + iaSep + f.Name,
		Title:           title,
		Artist:          artist,
		Album:           album,
		AlbumArtist:     artist,
		TrackNo:         track,
		Year:            year,
		Duration:        duration,
		Suffix:          suffix,
	}
}

// Resolve implements Provider.
func (p *InternetArchiveProvider) Resolve(ctx context.Context, providerTrackID string) (Result, error) {
	identifier, filename, ok := splitIAID(providerTrackID)
	if !ok {
		return Result{}, fmt.Errorf("internet-archive: invalid track id %q", providerTrackID)
	}
	md, err := p.metadata(ctx, identifier)
	if err != nil {
		return Result{}, err
	}
	for _, f := range md.Files {
		if f.Name != filename {
			continue
		}
		suffix, _ := isAudioFormat(f)
		if suffix == "" {
			suffix = "mp3"
		}
		return iaResult(identifier, f, md, suffix), nil
	}
	return Result{}, fmt.Errorf("internet-archive: file %q not found in %q", filename, identifier)
}

// Download implements Provider.
func (p *InternetArchiveProvider) Download(ctx context.Context, providerTrackID string, w io.Writer) error {
	identifier, filename, ok := splitIAID(providerTrackID)
	if !ok {
		return fmt.Errorf("internet-archive: invalid track id %q", providerTrackID)
	}
	u := p.baseURL + "/download/" + url.PathEscape(identifier) + "/" + url.PathEscape(filename)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("internet-archive: download status %d", resp.StatusCode)
	}
	_, err = io.Copy(w, io.LimitReader(resp.Body, MaxDownloadBytes))
	return err
}

func (p *InternetArchiveProvider) metadata(ctx context.Context, identifier string) (*iaMetadata, error) {
	var md iaMetadata
	if err := p.getJSON(ctx, p.baseURL+"/metadata/"+url.PathEscape(identifier), &md); err != nil {
		return nil, err
	}
	return &md, nil
}

func (p *InternetArchiveProvider) getJSON(ctx context.Context, rawURL string, out any) error {
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
		return fmt.Errorf("internet-archive: status %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, MaxMetadataBytes)).Decode(out)
}

func splitIAID(id string) (identifier, filename string, ok bool) {
	i := strings.Index(id, iaSep)
	if i < 0 {
		return "", "", false
	}
	return id[:i], id[i+len(iaSep):], true
}
