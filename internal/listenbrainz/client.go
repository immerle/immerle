// Package listenbrainz submits plays to ListenBrainz (listenbrainz.org) on
// behalf of a user, using their personal API token.
package listenbrainz

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// defaultBaseURL is ListenBrainz's API root.
const defaultBaseURL = "https://api.listenbrainz.org"

// maxResponseBytes caps an in-memory response body -- ListenBrainz's own
// responses are small JSON, this just guards against a hostile/broken reply.
const maxResponseBytes = 1 << 20

// ErrInvalidToken means ListenBrainz rejected the token (validate-token
// answered valid:false).
var ErrInvalidToken = errors.New("listenbrainz: invalid token")

// ErrRateLimited means ListenBrainz answered 429 to submit-listens.
var ErrRateLimited = errors.New("listenbrainz: rate limited")

// Client talks to the ListenBrainz API. baseURL is overridable (tests point
// it at an httptest.Server) instead of a package-level default.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient builds a client against the real ListenBrainz API. hc is
// optional (nil uses a client with a sane default timeout).
func NewClient(hc *http.Client) *Client {
	return newClient(defaultBaseURL, hc)
}

func newClient(baseURL string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: hc}
}

// ValidateToken checks a user's personal API token, returning their
// ListenBrainz username on success. Returns ErrInvalidToken when the token is
// well-formed but rejected.
func (c *Client) ValidateToken(ctx context.Context, token string) (username string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/1/validate-token", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Token "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("listenbrainz: validate-token: unexpected status %d", resp.StatusCode)
	}

	var out struct {
		Valid    bool   `json:"valid"`
		UserName string `json:"user_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&out); err != nil {
		return "", fmt.Errorf("listenbrainz: decode validate-token: %w", err)
	}
	if !out.Valid {
		return "", ErrInvalidToken
	}
	return out.UserName, nil
}

// Listen is one play, ready to submit.
type Listen struct {
	ListenedAt    time.Time
	Artist        string
	Track         string
	Release       string
	DurationMs    int
	RecordingMBID string
	ISRC          string
}

// submit-listens request shapes, per
// https://listenbrainz.readthedocs.io/en/latest/users/api/core.html#post--1-submit-listens
type submitListensRequest struct {
	ListenType string          `json:"listen_type"`
	Payload    []listenPayload `json:"payload"`
}

type listenPayload struct {
	ListenedAt    int64         `json:"listened_at"`
	TrackMetadata trackMetadata `json:"track_metadata"`
}

type trackMetadata struct {
	ArtistName     string         `json:"artist_name"`
	TrackName      string         `json:"track_name"`
	ReleaseName    string         `json:"release_name,omitempty"`
	AdditionalInfo additionalInfo `json:"additional_info"`
}

type additionalInfo struct {
	DurationMs    int    `json:"duration_ms,omitempty"`
	RecordingMBID string `json:"recording_mbid,omitempty"`
	ISRC          string `json:"isrc,omitempty"`
}

// SubmitListen submits a single completed play.
func (c *Client) SubmitListen(ctx context.Context, token string, l Listen) error {
	body, err := json.Marshal(submitListensRequest{
		ListenType: "single",
		Payload: []listenPayload{{
			ListenedAt: l.ListenedAt.Unix(),
			TrackMetadata: trackMetadata{
				ArtistName:  l.Artist,
				TrackName:   l.Track,
				ReleaseName: l.Release,
				AdditionalInfo: additionalInfo{
					DurationMs:    l.DurationMs,
					RecordingMBID: l.RecordingMBID,
					ISRC:          l.ISRC,
				},
			},
		}},
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/1/submit-listens", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Token "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return ErrRateLimited
	}
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		return fmt.Errorf("listenbrainz: submit-listens: status %d: %s", resp.StatusCode, msg)
	}
	return nil
}
