// Package federation implements the client side of immerle-hub integration
// (S7): instance registration, periodic editorial/recommendation playlist sync,
// portable-id resolution, and anonymized scrobble export. Everything here is
// opt-in and fully disable-able via configuration.
package federation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/config"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// instanceVersion is reported to the hub on register/heartbeat.
const instanceVersion = "0.2.0"

// Resolver turns a portable track identifier into a local track id, optionally
// downloading the track on demand when it is missing.
type Resolver interface {
	RemoteSearch(ctx context.Context, query string, limit int) ([]models.Track, error)
	Resolve(ctx context.Context, userID, trackID string) (models.Track, bool, string, error)
}

// Service is the federation client. Its configuration is read live (hot-
// reloadable): enabling/disabling, the hub URL/keys, the sync interval and the
// feature flags all take effect without a restart.
type Service struct {
	cfgFn     func() config.FederationConfig
	http      *http.Client
	catalog   *persistence.CatalogRepo
	playlists *persistence.PlaylistRepo
	scrobbles *persistence.ScrobbleRepo
	resolver  Resolver // optional (on-demand catalog); may be nil
	logger    *slog.Logger
	// ownerID is the cached nominal owner for federated (public, read-only)
	// playlists; ownerFn resolves it lazily when unset (so enabling federation
	// after first-run setup still finds an admin).
	ownerID string
	ownerFn func(context.Context) (string, error)
}

// cfg returns the current federation configuration (read live).
func (s *Service) cfg() config.FederationConfig { return s.cfgFn() }

// SetOwner pins the nominal owner for federated playlists (typically an admin).
func (s *Service) SetOwner(userID string) { s.ownerID = userID }

// SetOwnerResolver registers a lazy resolver for the federated-playlist owner,
// used when no owner is pinned (e.g. federation enabled after the admin exists).
func (s *Service) SetOwnerResolver(fn func(context.Context) (string, error)) { s.ownerFn = fn }

// New builds a federation Service. cfgFn supplies the live config; resolver may
// be nil to disable on-demand resolution of missing tracks.
func New(cfgFn func() config.FederationConfig, catalog *persistence.CatalogRepo, playlists *persistence.PlaylistRepo, scrobbles *persistence.ScrobbleRepo, resolver Resolver, logger *slog.Logger) *Service {
	return &Service{
		cfgFn:     cfgFn,
		http:      &http.Client{Timeout: 30 * time.Second},
		catalog:   catalog,
		playlists: playlists,
		scrobbles: scrobbles,
		resolver:  resolver,
		logger:    logger,
	}
}

// Enabled reports whether federation is turned on (implements the immerle
// FederationStatusProvider interface). Read live.
func (s *Service) Enabled() bool { return s != nil && s.cfg().Enabled }

// HubConfigured reports whether the hub URL and both keys are set (so hub-backed
// features such as playlist import are usable). Read live. Requiring the keys
// here means a partial config fails fast locally instead of as a hub 401.
func (s *Service) HubConfigured() bool {
	if s == nil {
		return false
	}
	c := s.cfg()
	return c.HubURL != "" && c.PublicKey != "" && c.PrivateKey != ""
}

// hubPlaylist is the portable playlist shape exchanged with the hub.
type hubPlaylist struct {
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	Comment string     `json:"comment"`
	Tracks  []hubTrack `json:"tracks"`
}

// hubTrack carries portable identifiers for cross-instance resolution.
type hubTrack struct {
	MBID   string `json:"mbid"`
	Artist string `json:"artist"`
	Title  string `json:"title"`
	Album  string `json:"album"`
}

// Register announces this instance to the hub (a heartbeat).
func (s *Service) Register(ctx context.Context) error {
	c := s.cfg()
	if !c.Enabled {
		return nil
	}
	// Identity/auth travel in the headers (public key → X-Instance-ID, private key
	// → Authorization Bearer), so the body only carries the instance version.
	body := map[string]any{"version": instanceVersion}
	_, err := s.do(ctx, http.MethodPost, "/api/v1/instances/register", body)
	if err != nil {
		return fmt.Errorf("register with hub: %w", err)
	}
	s.logger.Info("registered with immerle-hub", "publicKey", c.PublicKey, "hub", c.HubURL)
	return nil
}

// federationTick is how often Run re-reads its (hot-reloadable) config to decide
// whether/when to sync. It bounds the latency of an enable/disable taking effect.
const federationTick = time.Minute

// Run drives federation on a fixed tick, reading config live so enabling/
// disabling, the interval and the keys all apply without a restart. While
// enabled it heartbeats + syncs every configured interval; while disabled it
// idles. It never returns until ctx is done (unlike a one-shot loop).
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(federationTick)
	defer ticker.Stop()
	var lastSync time.Time
	for {
		c := s.cfg()
		if c.Enabled {
			interval := c.SyncInterval
			if interval <= 0 {
				interval = time.Hour
			}
			if lastSync.IsZero() || time.Since(lastSync) >= interval {
				if err := s.Register(ctx); err != nil {
					s.logger.Warn("hub registration failed", "error", err)
				}
				s.syncOnce(ctx)
				lastSync = time.Now()
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) syncOnce(ctx context.Context) {
	if err := s.SyncPlaylists(ctx); err != nil {
		s.logger.Warn("playlist sync failed", "error", err)
	}
	if s.cfg().ExportScrobbles {
		if err := s.ExportScrobbles(ctx); err != nil {
			s.logger.Warn("scrobble export failed", "error", err)
		}
	}
}

// SyncPlaylists fetches editorial/recommended playlists and materializes them as
// read-only federated playlists, resolving each track to a local one.
func (s *Service) SyncPlaylists(ctx context.Context) error {
	if !s.cfg().Enabled {
		return nil
	}
	raw, err := s.do(ctx, http.MethodGet, "/api/v1/playlists", nil)
	if err != nil {
		return err
	}
	var playlists []hubPlaylist
	if err := json.Unmarshal(raw, &playlists); err != nil {
		return fmt.Errorf("decode hub playlists: %w", err)
	}

	owner, err := s.federationOwner(ctx)
	if err != nil {
		return err
	}

	for _, hp := range playlists {
		if err := s.materialize(ctx, owner, hp); err != nil {
			s.logger.Warn("materialize federated playlist failed", "name", hp.Name, "error", err)
		}
	}
	return nil
}

// materialize creates/updates one federated playlist and resolves its tracks.
func (s *Service) materialize(ctx context.Context, ownerID string, hp hubPlaylist) error {
	var trackIDs []string
	for _, ht := range hp.Tracks {
		id, ok := s.resolveTrack(ctx, ownerID, ht)
		if ok {
			trackIDs = append(trackIDs, id)
		}
	}

	existing, err := s.playlists.FindFederated(ctx, hp.Name)
	now := time.Now()
	if err == nil {
		existing.Comment = hp.Comment
		_ = s.playlists.UpdateMeta(ctx, existing)
		return s.playlists.ReplaceTracks(ctx, existing.ID, trackIDs, "")
	}

	p := models.Playlist{
		ID:        uuid.NewString(),
		Name:      hp.Name,
		OwnerID:   ownerID,
		Comment:   hp.Comment,
		Public:    true,
		Federated: true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.playlists.Create(ctx, p); err != nil {
		return err
	}
	return s.playlists.ReplaceTracks(ctx, p.ID, trackIDs, "")
}

// resolveTrack maps a portable hub track to a local track id. Present tracks
// resolve immediately; missing tracks are downloaded on demand when enabled.
func (s *Service) resolveTrack(ctx context.Context, ownerID string, ht hubTrack) (string, bool) {
	if ht.MBID != "" {
		if id, exists, _ := s.catalog.TrackExistsByMBIDOrHash(ctx, ht.MBID, ""); exists {
			return id, true
		}
	}
	if !s.cfg().ResolveMissing || s.resolver == nil {
		return "", false
	}
	// On-demand resolution: search a provider for the track and download it.
	query := ht.Artist + " " + ht.Title
	remote, err := s.resolver.RemoteSearch(ctx, query, 1)
	if err != nil || len(remote) == 0 {
		return "", false
	}
	track, local, _, err := s.resolver.Resolve(ctx, ownerID, remote[0].ID)
	if err != nil || !local {
		return "", false
	}
	return track.ID, true
}

// ExportScrobbles pushes anonymized, aggregated scrobble counts to the hub. No
// PII (no user ids, no timestamps beyond a coarse day bucket) is sent.
func (s *Service) ExportScrobbles(ctx context.Context) error {
	scrobbles, err := s.scrobbles.Unexported(ctx, 1000)
	if err != nil {
		return err
	}
	if len(scrobbles) == 0 {
		return nil
	}

	// Aggregate by track, dropping user identity entirely.
	counts := map[string]int{}
	var ids []string
	for _, sc := range scrobbles {
		counts[sc.TrackID]++
		ids = append(ids, sc.ID)
	}
	type aggregate struct {
		TrackHash string `json:"trackHash"`
		Count     int    `json:"count"`
	}
	payload := make([]aggregate, 0, len(counts))
	for trackID, count := range counts {
		// Hash the track id so the hub cannot correlate back to a local catalog.
		sum := sha256.Sum256([]byte(s.cfg().PublicKey + ":" + trackID))
		payload = append(payload, aggregate{TrackHash: hex.EncodeToString(sum[:]), Count: count})
	}

	if _, err := s.do(ctx, http.MethodPost, "/api/v1/scrobbles", map[string]any{"aggregates": payload}); err != nil {
		return err
	}
	return s.scrobbles.MarkExported(ctx, ids)
}

// federationOwner returns the user id that owns federated playlists (a nominal
// owner; these playlists are public and read-only regardless). When not pinned,
// it is resolved lazily (and cached) — so enabling federation after first-run
// setup still finds an admin without a restart.
func (s *Service) federationOwner(ctx context.Context) (string, error) {
	if s.ownerID != "" {
		return s.ownerID, nil
	}
	if s.ownerFn != nil {
		id, err := s.ownerFn(ctx)
		if err != nil {
			return "", err
		}
		s.ownerID = id
		return id, nil
	}
	return "", fmt.Errorf("federation owner not configured")
}

// do performs an authenticated JSON request against the hub.
func (s *Service) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	c := s.cfg()
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.HubURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.PrivateKey)
	req.Header.Set("X-Instance-ID", c.PublicKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("hub %s %s: status %d", method, path, resp.StatusCode)
	}
	return data, nil
}
