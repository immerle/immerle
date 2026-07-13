package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Client is a thin, typed client for the immerle-hub instance API (the /api/v1
// routes). It wraps the generated wire types with one method per instance call.
type Client struct {
	// baseURL resolves the hub root live on each call, so an operator changing
	// the configured hub URL at runtime takes effect without a restart.
	baseURL func() string
	http    *http.Client
}

// maxHubResponseBytes caps an in-memory hub API response, so a hostile or buggy
// hub cannot exhaust memory with an unbounded body. Hub payloads are small JSON.
const maxHubResponseBytes = 8 << 20 // 8 MiB

// New builds a hub client. baseURL resolves the hub root live (e.g.
// https://hub.immerle.com); hc may be nil (a default client is used). Redirects
// are restricted to the hub's own host to prevent SSRF via a hostile redirect.
func New(baseURL func() string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{}
	}
	c := &Client{baseURL: baseURL, http: hc}
	hc.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
		base, err := url.Parse(c.base())
		if err != nil || !strings.EqualFold(req.URL.Hostname(), base.Hostname()) {
			return fmt.Errorf("hub: refusing redirect to disallowed host %q", req.URL.Hostname())
		}
		return nil
	}
	return c
}

// base returns the current hub root with any trailing slash trimmed.
func (c *Client) base() string { return strings.TrimRight(c.baseURL(), "/") }

// Auth carries the per-instance credentials sent on authenticated calls: the
// private key as a Bearer token and the instance UUID as the X-Instance-ID
// header (both are required by the hub on every /api/v1 call except bootstrap).
type Auth struct {
	InstanceID string
	PrivateKey string
}

// Bootstrap self-registers an instance under the owner identified by req.UserId
// (no auth). The response carries the assigned id (UUID), sqid handle and the
// private key (shown once) — persist all three.
func (c *Client) Bootstrap(ctx context.Context, req PublicBootstrapRequest) (PublicBootstrapResponse, error) {
	var out PublicBootstrapResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/instances", Auth{}, req, &out)
	return out, err
}

// Register records a heartbeat and the reported version, returning the profile.
func (c *Client) Register(ctx context.Context, a Auth, version string) (PublicProfileResponse, error) {
	var out PublicProfileResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/instances/register", a, PublicRegisterRequest{Version: &version}, &out)
	return out, err
}

// Me returns this instance's current profile (the hub is the source of truth
// for the name and sqid handle).
func (c *Client) Me(ctx context.Context, a Auth) (PublicProfileResponse, error) {
	var out PublicProfileResponse
	err := c.do(ctx, http.MethodGet, "/api/v1/instances/me", a, nil, &out)
	return out, err
}

// DeleteData deletes this instance's data on the hub (GDPR / unlink).
func (c *Client) DeleteData(ctx context.Context, a Auth) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/instances/me/data", a, nil, nil)
}

// UpdateInstance changes this instance's editable fields (name, sqid handle,
// opt-in). The hub validates sqid uniqueness (409 surfaces as an error).
func (c *Client) UpdateInstance(ctx context.Context, a Auth, req PublicUpdateInstanceRequest) (PublicProfileResponse, error) {
	var out PublicProfileResponse
	err := c.do(ctx, http.MethodPatch, "/api/v1/instances/me", a, req, &out)
	return out, err
}

// ListPlaylists fetches the distributed (editorial/recommendation) playlists.
func (c *Client) ListPlaylists(ctx context.Context, a Auth, region string) ([]PublicDistributionPlaylist, error) {
	path := "/api/v1/playlists"
	if region != "" {
		path += "?region=" + url.QueryEscape(region)
	}
	var out []PublicDistributionPlaylist
	err := c.do(ctx, http.MethodGet, path, a, nil, &out)
	return out, err
}

// FeedPlaylists returns one page of playlist headers from the instances this
// one is subscribed to, ordered by update time ascending. Pass after ==
// resp.NextUpdatedAfter to page forward while resp.HasMore is true; "" fetches
// from the start.
func (c *Client) FeedPlaylists(ctx context.Context, a Auth, after string) (PublicFeedResponse, error) {
	path := "/api/v1/instances/me/feed/playlists"
	if after != "" {
		path += "?updatedAfter=" + url.QueryEscape(after)
	}
	var out PublicFeedResponse
	err := c.do(ctx, http.MethodGet, path, a, nil, &out)
	return out, err
}

// FeedTrack is one track inside a subscribed instance's playlist, decoded from
// the free-form JSON the owning instance synced (per the hub's
// SyncTrackExample convention: any JSON is accepted, only these fields are
// used for resolution).
type FeedTrack struct {
	Mbid   string `json:"mbid"`
	Artist string `json:"artist"`
	Title  string `json:"title"`
}

// FeedPlaylistDetail is one subscribed instance's full playlist, as returned by
// GetFeedPlaylist.
type FeedPlaylistDetail struct {
	InstanceID   string
	InstanceName string
	ExternalID   string
	Name         string
	Description  string
	Image        string
	Tracks       []FeedTrack
}

// GetFeedPlaylist returns one full playlist (with tracks) owned by instanceID,
// an instance this one is subscribed to (404 if not subscribed). Hand-decoded
// rather than via the generated PublicInstancePlaylistDTO: the hub's
// swaggertype:"object" annotation on the tracks field (actually a JSON array)
// makes oapi-codegen type it as map[string]interface{}, which fails to decode.
func (c *Client) GetFeedPlaylist(ctx context.Context, a Auth, instanceID, externalID string) (FeedPlaylistDetail, error) {
	var wire struct {
		Playlist struct {
			Author struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"author"`
			ExternalID  string          `json:"externalId"`
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Image       string          `json:"image"`
			Tracks      json.RawMessage `json:"tracks"`
		} `json:"playlist"`
	}
	path := "/api/v1/instances/" + url.PathEscape(instanceID) + "/playlists/" + url.PathEscape(externalID)
	if err := c.do(ctx, http.MethodGet, path, a, nil, &wire); err != nil {
		return FeedPlaylistDetail{}, err
	}
	var tracks []FeedTrack
	if len(wire.Playlist.Tracks) > 0 {
		_ = json.Unmarshal(wire.Playlist.Tracks, &tracks) // best-effort; malformed tracks are simply dropped
	}
	return FeedPlaylistDetail{
		InstanceID: wire.Playlist.Author.ID, InstanceName: wire.Playlist.Author.Name,
		ExternalID: wire.Playlist.ExternalID, Name: wire.Playlist.Name, Description: wire.Playlist.Description,
		Image: wire.Playlist.Image, Tracks: tracks,
	}, nil
}

// SearchInstances finds other instances by exact sqid or name (ILIKE); the hub
// excludes the caller and revoked instances. Returns summaries (no secrets).
func (c *Client) SearchInstances(ctx context.Context, a Auth, q string) (PublicSearchResponse, error) {
	var out PublicSearchResponse
	err := c.do(ctx, http.MethodGet, "/api/v1/instances/search?q="+url.QueryEscape(q), a, nil, &out)
	return out, err
}

// Subscriptions lists the instances this one follows.
func (c *Client) Subscriptions(ctx context.Context, a Auth) (PublicSubscriptionsResponse, error) {
	var out PublicSubscriptionsResponse
	err := c.do(ctx, http.MethodGet, "/api/v1/instances/me/subscriptions", a, nil, &out)
	return out, err
}

// Subscribe follows a target instance (by instanceId UUID or sqid). Idempotent;
// the hub rejects self-subscription (400) and unknown/revoked targets (404).
func (c *Client) Subscribe(ctx context.Context, a Auth, req PublicSubscribeRequest) (PublicSubscriptionStateResponse, error) {
	var out PublicSubscriptionStateResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/instances/me/subscriptions", a, req, &out)
	return out, err
}

// Unsubscribe stops following the instance with the given id (UUID).
func (c *Client) Unsubscribe(ctx context.Context, a Auth, id string) (PublicSubscriptionStateResponse, error) {
	var out PublicSubscriptionStateResponse
	err := c.do(ctx, http.MethodDelete, "/api/v1/instances/me/subscriptions/"+url.PathEscape(id), a, nil, &out)
	return out, err
}

// IngestScrobbles pushes anonymized aggregated scrobble counts (opt-in only).
func (c *Client) IngestScrobbles(ctx context.Context, a Auth, req PublicScrobblesRequest) (PublicIngestResultResponse, error) {
	var out PublicIngestResultResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/scrobbles", a, req, &out)
	return out, err
}

// SpotifyImport enqueues a Spotify public-playlist import job.
func (c *Client) SpotifyImport(ctx context.Context, a Auth, playlist string) (PublicSpotifyJobResponse, error) {
	var out PublicSpotifyJobResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/spotify/imports", a, PublicSpotifyImportRequest{Playlist: &playlist}, &out)
	return out, err
}

// SpotifyJob polls a Spotify import job by id.
func (c *Client) SpotifyJob(ctx context.Context, a Auth, id string) (PublicSpotifyJobResponse, error) {
	var out PublicSpotifyJobResponse
	err := c.do(ctx, http.MethodGet, "/api/v1/spotify/imports/"+url.PathEscape(id), a, nil, &out)
	return out, err
}

// SyncPlaylist upserts a public playlist on the hub under externalId (the local
// playlist id, the idempotency key). body is the marshaled sync payload (name,
// description, image, metadata object, tracks array).
func (c *Client) SyncPlaylist(ctx context.Context, a Auth, externalID string, body any) error {
	return c.do(ctx, http.MethodPut, "/api/v1/instances/me/playlists/"+url.PathEscape(externalID), a, body, nil)
}

// DeletePlaylist removes a synced playlist from the hub. A 404 (never synced) is
// surfaced as an *HTTPError so the caller can treat it as already-gone.
func (c *Client) DeletePlaylist(ctx context.Context, a Auth, externalID string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/instances/me/playlists/"+url.PathEscape(externalID), a, nil, nil)
}

// MissingCovers returns which of the candidate cover hashes the hub does NOT yet
// have (so only those need uploading). Max 1000 hashes per call.
func (c *Client) MissingCovers(ctx context.Context, a Auth, hashes []string) ([]string, error) {
	var out PublicMissingCoversResponse
	if err := c.do(ctx, http.MethodPost, "/api/v1/covers/missing", a, PublicMissingCoversRequest{Hashes: &hashes}, &out); err != nil {
		return nil, err
	}
	if out.Missing == nil {
		return nil, nil
	}
	return *out.Missing, nil
}

// UploadCover uploads raw cover bytes addressed by their sha256 hash (idempotent;
// the hub verifies sha256(bytes)==hash). contentType is image/jpeg|png|webp|gif.
func (c *Client) UploadCover(ctx context.Context, a Auth, hash, contentType string, data []byte) error {
	return c.doRaw(ctx, http.MethodPut, "/api/v1/covers/"+url.PathEscape(hash), a, contentType, data, nil)
}

// HTTPError is a non-2xx response from the hub. Callers can inspect Status to
// react (e.g. 429 → back off harder, 404 → treat a delete as already-gone).
type HTTPError struct {
	Status  int
	Method  string
	Path    string
	Message string // the hub's error string when present
}

func (e *HTTPError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("hub %s %s: %s (status %d)", e.Method, e.Path, e.Message, e.Status)
	}
	return fmt.Sprintf("hub %s %s: status %d", e.Method, e.Path, e.Status)
}

// do performs a JSON request, attaching auth headers when set, and decodes the
// 2xx body into out (nil to ignore). Non-2xx responses become an *HTTPError.
func (c *Client) do(ctx context.Context, method, path string, a Auth, body, out any) error {
	var raw []byte
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		raw = buf
	}
	return c.doRaw(ctx, method, path, a, "application/json", raw, out)
}

// doRaw performs a request with a pre-marshaled body and an explicit content type
// (used for JSON via do, and for octet-stream cover uploads). out may be nil.
func (c *Client) doRaw(ctx context.Context, method, path string, a Auth, contentType string, body []byte, out any) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base()+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", contentType)
	}
	if a.PrivateKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.PrivateKey)
		req.Header.Set("X-Instance-ID", a.InstanceID)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, maxHubResponseBytes))
	if resp.StatusCode >= 300 {
		return &HTTPError{Status: resp.StatusCode, Method: method, Path: path, Message: errorMessage(data)}
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode hub response: %w", err)
		}
	}
	return nil
}

// instanceKeyContextKey shims a type the generated types.gen.go references for
// the undefined "InstanceKey" security scheme (the hub spec's spotify routes
// declare it but it is not in components.securitySchemes — a swaggo artifact).
// Remove once the hub spec defines or drops InstanceKey.
type instanceKeyContextKey string

// errorMessage extracts the hub's error string from a non-2xx body, if shaped
// like httpx.ErrorResponse.
func errorMessage(data []byte) string {
	var e HttpxErrorResponse
	if json.Unmarshal(data, &e) == nil && e.Error != nil {
		return *e.Error
	}
	return ""
}
