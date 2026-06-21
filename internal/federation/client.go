// Package federation implements the client side of immerle-hub integration
// (S7): instance registration, periodic editorial/recommendation playlist sync,
// portable-id resolution, and anonymized scrobble export. Everything here is
// opt-in and fully disable-able via configuration.
package federation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/config"
	"github.com/immerle/immerle/internal/federation/hub"
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
	hub       *hub.Client
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
	// saveCreds persists hub-issued identity (instance UUID, sqid, private key,
	// name) back into the runtime settings after bootstrap/update (optional;
	// empty fields are left unchanged by the saver).
	saveCreds func(context.Context, Credentials) error
	// clearCreds wipes the stored hub identity on unlink (optional).
	clearCreds func(context.Context) error
}

// Credentials carries hub-issued identity persisted into the runtime settings.
// Empty fields are ignored by the saver (so an update can touch only name/sqid).
type Credentials struct {
	InstanceID string
	Sqid       string
	PrivateKey string
	Name       string
}

// SetCredentialsSaver registers a callback used to persist hub-issued identity
// (returned at bootstrap, or the canonical name/sqid returned at update).
func (s *Service) SetCredentialsSaver(fn func(context.Context, Credentials) error) { s.saveCreds = fn }

// SetCredentialsClearer registers a callback used to wipe the stored hub
// identity when the operator unlinks the instance.
func (s *Service) SetCredentialsClearer(fn func(context.Context) error) { s.clearCreds = fn }

// auth returns the per-instance hub credentials (private key + instance UUID).
func (s *Service) auth() hub.Auth {
	c := s.cfg()
	return hub.Auth{InstanceID: c.InstanceID, PrivateKey: c.PrivateKey}
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
		hub:       hub.New(cfgFn().HubURL, &http.Client{Timeout: 30 * time.Second}),
		catalog:   catalog,
		playlists: playlists,
		scrobbles: scrobbles,
		resolver:  resolver,
		logger:    logger,
	}
}

// Enabled reports whether federation is active — i.e. the instance is linked to
// the hub (implements the immerle FederationStatusProvider interface). There is
// no separate enable flag: linked means active. Read live.
func (s *Service) Enabled() bool { return s.HubConfigured() }

// HubConfigured reports whether the instance has bootstrapped with the hub
// (instance UUID + private key both set) so authenticated hub calls (playlist
// sync, import, ingest) are usable. Read live.
func (s *Service) HubConfigured() bool {
	if s == nil {
		return false
	}
	c := s.cfg()
	return c.InstanceID != "" && c.PrivateKey != ""
}

// Register ensures the instance is known to the hub: it bootstraps (claims the
// configured owner UserID) on first run, then heartbeats on subsequent calls.
// Bootstrap credentials (instance UUID, sqid, private key) are persisted via the
// credentials saver. Callable independently of the Enabled flag (the admin
// triggers it explicitly via the register endpoint).
func (s *Service) Register(ctx context.Context) error {
	c := s.cfg()
	if c.PrivateKey == "" {
		return s.bootstrap(ctx, c)
	}
	if _, err := s.hub.Register(ctx, s.auth(), instanceVersion); err != nil {
		return fmt.Errorf("hub heartbeat: %w", err)
	}
	s.logger.Info("heartbeat sent to immerle-hub", "instanceId", c.InstanceID)
	return nil
}

// bootstrap self-registers the instance under the configured owner UserID and
// persists the hub-issued credentials (instance UUID, sqid, private key).
func (s *Service) bootstrap(ctx context.Context, c config.FederationConfig) error {
	if c.UserID == "" {
		return fmt.Errorf("federation: no hub user id configured")
	}
	name, optIn, ver := c.InstanceName, c.ExportScrobbles, instanceVersion
	resp, err := s.hub.Bootstrap(ctx, hub.PublicBootstrapRequest{
		UserId:      &c.UserID,
		Name:        &name,
		OptInIngest: &optIn,
		Version:     &ver,
	})
	if err != nil {
		return fmt.Errorf("bootstrap with hub: %w", err)
	}
	creds := Credentials{
		InstanceID: deref(resp.Id),
		Sqid:       deref(resp.Sqid),
		PrivateKey: deref(resp.PrivateKey),
		Name:       deref(resp.Name),
	}
	if creds.InstanceID == "" || creds.PrivateKey == "" {
		return fmt.Errorf("hub bootstrap returned incomplete credentials")
	}
	if s.saveCreds != nil {
		if err := s.saveCreds(ctx, creds); err != nil {
			return fmt.Errorf("persist hub credentials: %w", err)
		}
	}
	s.logger.Info("bootstrapped with immerle-hub", "instanceId", creds.InstanceID, "sqid", creds.Sqid)
	return nil
}

// UpdateInstance pushes a name/sqid change to the hub (opt-in mirrors the local
// ExportScrobbles flag) and persists the hub-canonical name/sqid. The hub
// validates sqid uniqueness, so a clashing handle surfaces as an error.
func (s *Service) UpdateInstance(ctx context.Context, name, sqid string) error {
	if !s.HubConfigured() {
		return fmt.Errorf("federation: instance not registered with the hub")
	}
	optIn := s.cfg().ExportScrobbles
	resp, err := s.hub.UpdateInstance(ctx, s.auth(), hub.PublicUpdateInstanceRequest{
		Name: &name, Sqid: &sqid, OptInIngest: &optIn,
	})
	if err != nil {
		return fmt.Errorf("update instance on hub: %w", err)
	}
	if resp.Instance != nil && s.saveCreds != nil {
		if err := s.saveCreds(ctx, Credentials{Sqid: deref(resp.Instance.Sqid), Name: deref(resp.Instance.Name)}); err != nil {
			return fmt.Errorf("persist hub instance update: %w", err)
		}
	}
	return nil
}

// InstanceSummary is a federated instance as surfaced by discovery/subscriptions
// (no sensitive fields). It mirrors the hub's public InstanceSummary.
type InstanceSummary struct {
	ID         string `json:"id"`
	Sqid       string `json:"sqid"`
	Name       string `json:"name"`
	Region     string `json:"region"`
	LastSeenAt string `json:"lastSeenAt,omitempty"`
}

func summarize(in []hub.PublicInstanceSummary) []InstanceSummary {
	out := make([]InstanceSummary, 0, len(in))
	for _, s := range in {
		out = append(out, InstanceSummary{
			ID: deref(s.Id), Sqid: deref(s.Sqid), Name: deref(s.Name),
			Region: deref(s.Region), LastSeenAt: deref(s.LastSeenAt),
		})
	}
	return out
}

// SearchInstances discovers other instances on the hub by sqid or name.
func (s *Service) SearchInstances(ctx context.Context, query string) ([]InstanceSummary, error) {
	if !s.HubConfigured() {
		return nil, fmt.Errorf("federation: instance not linked to the hub")
	}
	resp, err := s.hub.SearchInstances(ctx, s.auth(), query)
	if err != nil {
		return nil, err
	}
	if resp.Instances == nil {
		return nil, nil
	}
	return summarize(*resp.Instances), nil
}

// Subscriptions lists the instances this one follows on the hub.
func (s *Service) Subscriptions(ctx context.Context) ([]InstanceSummary, error) {
	if !s.HubConfigured() {
		return nil, fmt.Errorf("federation: instance not linked to the hub")
	}
	resp, err := s.hub.Subscriptions(ctx, s.auth())
	if err != nil {
		return nil, err
	}
	if resp.Subscriptions == nil {
		return nil, nil
	}
	return summarize(*resp.Subscriptions), nil
}

// Subscribe follows the target instance (by hub id UUID or sqid handle).
func (s *Service) Subscribe(ctx context.Context, instanceID, sqid string) error {
	if !s.HubConfigured() {
		return fmt.Errorf("federation: instance not linked to the hub")
	}
	req := hub.PublicSubscribeRequest{}
	if instanceID != "" {
		req.InstanceId = &instanceID
	}
	if sqid != "" {
		req.Sqid = &sqid
	}
	if _, err := s.hub.Subscribe(ctx, s.auth(), req); err != nil {
		return err
	}
	return nil
}

// Unsubscribe stops following the instance with the given hub id (UUID).
func (s *Service) Unsubscribe(ctx context.Context, instanceID string) error {
	if !s.HubConfigured() {
		return fmt.Errorf("federation: instance not linked to the hub")
	}
	if _, err := s.hub.Unsubscribe(ctx, s.auth(), instanceID); err != nil {
		return err
	}
	return nil
}

// RefreshProfile fetches this instance's current name/sqid from the hub (the
// hub is the source of truth) and persists them locally.
func (s *Service) RefreshProfile(ctx context.Context) error {
	if !s.HubConfigured() {
		return fmt.Errorf("federation: instance not linked to the hub")
	}
	resp, err := s.hub.Me(ctx, s.auth())
	if err != nil {
		return fmt.Errorf("fetch instance profile from hub: %w", err)
	}
	if resp.Instance != nil && s.saveCreds != nil {
		return s.saveCreds(ctx, Credentials{Sqid: deref(resp.Instance.Sqid), Name: deref(resp.Instance.Name)})
	}
	return nil
}

// Unlink deletes this instance's data on the hub (best-effort) and wipes the
// locally stored identity, returning the instance to the unlinked state.
func (s *Service) Unlink(ctx context.Context) error {
	if s.HubConfigured() {
		if err := s.hub.DeleteData(ctx, s.auth()); err != nil {
			// Don't block local unlink on a hub error (the operator owns this
			// instance); the hub data is also GC'd on its side over time.
			s.logger.Warn("hub data deletion failed (unlinking locally anyway)", "error", err)
		}
	}
	if s.clearCreds != nil {
		return s.clearCreds(ctx)
	}
	return nil
}

// deref returns the value of a string pointer, or "" when nil.
func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// federationTick is how often Run re-reads its (hot-reloadable) config to decide
// whether to sync; federationSyncInterval is the fixed heartbeat+sync cadence
// (the cadence is not user-configurable — linking is the only knob).
const (
	federationTick         = time.Minute
	federationSyncInterval = time.Hour
)

// Run drives federation on a fixed tick, reading config live so linking/
// unlinking applies without a restart. While linked it heartbeats + syncs every
// federationSyncInterval; otherwise it idles. It never returns until ctx is done.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(federationTick)
	defer ticker.Stop()
	var lastSync time.Time
	for {
		if s.HubConfigured() {
			if lastSync.IsZero() || time.Since(lastSync) >= federationSyncInterval {
				if err := s.Register(ctx); err != nil {
					s.logger.Warn("hub heartbeat failed", "error", err)
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
	if !s.HubConfigured() {
		return nil
	}
	playlists, err := s.hub.ListPlaylists(ctx, s.auth(), "")
	if err != nil {
		return err
	}

	owner, err := s.federationOwner(ctx)
	if err != nil {
		return err
	}

	for _, hp := range playlists {
		if err := s.materialize(ctx, owner, hp); err != nil {
			s.logger.Warn("materialize federated playlist failed", "name", deref(hp.Name), "error", err)
		}
	}
	return nil
}

// materialize creates/updates one federated playlist and resolves its tracks.
func (s *Service) materialize(ctx context.Context, ownerID string, hp hub.PublicDistributionPlaylist) error {
	var trackIDs []string
	if hp.Tracks != nil {
		for _, ht := range *hp.Tracks {
			if id, ok := s.resolveTrack(ctx, ownerID, ht); ok {
				trackIDs = append(trackIDs, id)
			}
		}
	}

	name, comment := deref(hp.Name), deref(hp.Comment)
	existing, err := s.playlists.FindFederated(ctx, name)
	now := time.Now()
	if err == nil {
		existing.Comment = comment
		_ = s.playlists.UpdateMeta(ctx, existing)
		return s.playlists.ReplaceTracks(ctx, existing.ID, trackIDs, "")
	}

	p := models.Playlist{
		ID:        uuid.NewString(),
		Name:      name,
		OwnerID:   ownerID,
		Comment:   comment,
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
func (s *Service) resolveTrack(ctx context.Context, ownerID string, ht hub.PublicDistributionTrack) (string, bool) {
	if mbid := deref(ht.Mbid); mbid != "" {
		if id, exists, _ := s.catalog.TrackExistsByMBIDOrHash(ctx, mbid, ""); exists {
			return id, true
		}
	}
	if !s.cfg().ResolveMissing || s.resolver == nil {
		return "", false
	}
	// On-demand resolution: search a provider for the track and download it.
	query := deref(ht.Artist) + " " + deref(ht.Title)
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
	instanceID := s.cfg().InstanceID
	payload := make([]hub.PublicScrobbleAggregateDoc, 0, len(counts))
	for trackID, count := range counts {
		// Hash the track id so the hub cannot correlate back to a local catalog.
		sum := sha256.Sum256([]byte(instanceID + ":" + trackID))
		hash, n := hex.EncodeToString(sum[:]), count
		payload = append(payload, hub.PublicScrobbleAggregateDoc{TrackHash: &hash, Count: &n})
	}

	if _, err := s.hub.IngestScrobbles(ctx, s.auth(), hub.PublicScrobblesRequest{Aggregates: &payload}); err != nil {
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
