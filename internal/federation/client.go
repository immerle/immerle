// Package federation implements the client side of immerle-hub integration
// (S7): instance registration, periodic editorial/recommendation playlist sync,
// portable-id resolution, and anonymized scrobble export. Everything here is
// opt-in and fully disable-able via configuration.
package federation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/config"
	"github.com/immerle/immerle/internal/federation/hub"
	"github.com/immerle/immerle/internal/federation/stream"
	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// instanceVersion is reported to the hub on register/heartbeat.
const instanceVersion = "0.2.0"

// Resolver turns a portable track identifier into a local track id, optionally
// downloading the track on demand when it is missing.
type Resolver interface {
	// ResolveBestRemoteMatch searches every active provider for the track
	// best matching artist/title — a federated entry isn't tied to any single
	// provider, so unlike a plain RemoteSearch this can't stop at the admin's
	// first-ranked provider's single top guess.
	ResolveBestRemoteMatch(ctx context.Context, artist, title string) (models.Track, bool)
	Resolve(ctx context.Context, userID, trackID string) (models.Track, bool, string, error)
	// AutoDownloadOnPlay reports whether a remote result should be downloaded
	// on first listen (admin setting) — same policy ResolvePlaylistTrack
	// follows before persisting a provider-search hit for a federated entry.
	AutoDownloadOnPlay() bool
}

// Service is the federation client. Its configuration is read live (hot-
// reloadable): enabling/disabling, the hub URL/keys, the sync interval and the
// feature flags all take effect without a restart.
type Service struct {
	cfgFn       func() config.FederationConfig
	hub         *hub.Client
	catalog     *persistence.CatalogRepo
	playlists   *persistence.PlaylistRepo
	scrobbles   *persistence.ScrobbleRepo
	feedCursors *persistence.FeedCursorRepo
	resolver    Resolver // optional (on-demand catalog); may be nil
	stream      *stream.Client
	logger      *slog.Logger
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
	// replayHandler answers a replay.request with our current published state
	// (registered by PlaylistSyncer, which owns that state); nil until then, or
	// on an instance that only subscribes and never publishes.
	replayHandler func(context.Context, stream.Frame) error

	// resolveCacheMu/resolveCache short-circuit repeat taps on the same
	// unresolved playlist entry: a provider-search hit is remembered for a
	// while so it isn't re-searched on every play until either it downloads
	// locally (picked up by the MBID branch in ResolvePlaylistTrack) or the
	// entry is re-synced from the hub.
	resolveCacheMu sync.Mutex
	resolveCache   map[string]resolveCacheEntry
}

// resolveCacheTTL bounds how long a provider-search hit is reused for a given
// playlist entry before ResolvePlaylistTrack searches again.
const resolveCacheTTL = 10 * time.Minute

type resolveCacheEntry struct {
	track models.Track
	at    time.Time
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

// SetReplayHandler registers the callback that answers a replay.request from a
// reconnected subscriber (registered by PlaylistSyncer, the owner of published
// state). Until set (or on a subscribe-only instance), replay requests are
// just logged — see handleReplayRequest.
func (s *Service) SetReplayHandler(fn func(context.Context, stream.Frame) error) {
	s.replayHandler = fn
}

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
func New(cfgFn func() config.FederationConfig, catalog *persistence.CatalogRepo, playlists *persistence.PlaylistRepo, scrobbles *persistence.ScrobbleRepo, feedCursors *persistence.FeedCursorRepo, resolver Resolver, logger *slog.Logger) *Service {
	s := &Service{
		cfgFn:       cfgFn,
		hub:         hub.New(func() string { return cfgFn().HubURL }, &http.Client{Timeout: 30 * time.Second}),
		catalog:     catalog,
		playlists:   playlists,
		scrobbles:   scrobbles,
		feedCursors: feedCursors,
		resolver:    resolver,
		logger:      logger,
	}
	s.stream = stream.New(s.auth, func() string { return s.cfg().HubURL }, s.resumeCursors, stream.Handlers{
		OnUpsert: s.applyStreamUpsert,
		OnDelete: s.applyStreamDelete,
		OnReplay: s.handleReplayRequest,
	}, logger)
	return s
}

// RunStream starts the federation feed socket once the instance is linked to
// the hub (HubConfigured), so an unlinked/never-configured instance opens no
// outbound socket at all (federation stays fully opt-in). It waits on the same
// tick as Run and, once linked, delegates to the socket client's own
// reconnect-with-backoff loop for the rest of the process lifetime.
func (s *Service) RunStream(ctx context.Context) {
	ticker := time.NewTicker(federationTick)
	defer ticker.Stop()
	for !s.HubConfigured() {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
	s.stream.Run(ctx)
}

// resumeCursors builds the socket's resume payload: for every source instance
// currently followed, the last feed version applied locally (empty for one
// never seen, meaning full catch-up) — see stream.Client.
func (s *Service) resumeCursors(ctx context.Context) (map[string]string, error) {
	subs, err := s.Subscriptions(ctx)
	if err != nil {
		return nil, err
	}
	cursors := make(map[string]string, len(subs))
	for _, sub := range subs {
		v, err := s.feedCursors.Get(ctx, sub.ID)
		if err != nil {
			return nil, err
		}
		cursors[sub.ID] = v
	}
	return cursors, nil
}

// streamMetadata is the free-form JSON the frame's Metadata field carries for
// a playlist.upsert (opaque to the hub, meaningful only between instances) —
// name/description have no dedicated Frame field, unlike the REST sync payload.
type streamMetadata struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// applyStreamUpsert materializes a playlist.upsert frame relayed from a
// followed instance, reusing materializeFeed exactly as the REST feed pull
// does (RFC-socket-federation-client.md §5) — no new materialization logic.
func (s *Service) applyStreamUpsert(ctx context.Context, f stream.Frame) error {
	subs, err := s.Subscriptions(ctx)
	if err != nil {
		return err
	}
	// Defense in depth: the hub already filters relayed frames by
	// instance_subscriptions, so this only guards against a frame for a
	// subscription that ended locally in the gap before the hub notices.
	if !subscribedTo(subs, f.AuthorID) {
		return nil
	}

	var meta streamMetadata
	if len(f.Metadata) > 0 {
		_ = json.Unmarshal(f.Metadata, &meta) // best-effort; malformed metadata just yields an unnamed playlist
	}
	var tracks []hub.FeedTrack
	if len(f.Tracks) > 0 {
		_ = json.Unmarshal(f.Tracks, &tracks) // best-effort; malformed tracks are simply dropped
	}

	owner, err := s.federationOwner(ctx)
	if err != nil {
		return err
	}
	if err := s.materializeFeed(ctx, owner, hub.FeedPlaylistDetail{
		InstanceID:  f.AuthorID,
		ExternalID:  f.ExternalID,
		Name:        meta.Name,
		Description: meta.Description,
		Image:       f.Image,
		Tracks:      tracks,
	}); err != nil {
		return err
	}
	// A per-source (not per-playlist) watermark: only used to build the next
	// resume request, not to gate applying this frame. ponytail: no
	// per-playlist reordering guard here (would need a version column on the
	// federated playlist itself); add one if stale-overwrite reports show up.
	if f.Version > "" {
		return s.feedCursors.Set(ctx, f.AuthorID, f.Version)
	}
	return nil
}

// applyStreamDelete removes the local copy of a playlist.delete'd source
// playlist, if materialized. No-op if we never had it (already gone, or never
// synced in the first place).
func (s *Service) applyStreamDelete(ctx context.Context, f stream.Frame) error {
	existing, err := s.playlists.FindFederated(ctx, f.AuthorID, f.ExternalID)
	if err != nil {
		if errors.Is(err, persistence.ErrNotFound) {
			return nil
		}
		return err
	}
	return s.playlists.Delete(ctx, existing.ID)
}

// handleReplayRequest responds to a replay.request from a subscriber that just
// reconnected, delegating to the registered replayHandler (PlaylistSyncer,
// which owns our published state). Just logged on an instance that never
// publishes (no PlaylistSyncer wired), rather than swallowed silently.
func (s *Service) handleReplayRequest(ctx context.Context, f stream.Frame) error {
	if s.replayHandler == nil {
		s.logger.Debug("federation stream: replay request with no publisher registered", "forSubscriberId", f.ForSubscriberID, "sinceVersion", f.SinceVersion)
		return nil
	}
	return s.replayHandler(ctx, f)
}

// subscribedTo reports whether id is among the given subscriptions.
func subscribedTo(subs []InstanceSummary, id string) bool {
	for _, s := range subs {
		if s.ID == id {
			return true
		}
	}
	return false
}

// Enabled reports whether federation is active — i.e. the instance is linked to
// the hub. There is no separate enable flag: linked means active. Read live.
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
	// Close the feed socket now rather than leave it open under credentials
	// about to be revoked until the hub notices a missed heartbeat (RFC-socket-
	// federation-client.md §10.3).
	s.stream.Disconnect()
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

// maxFeedPages bounds the subscription-feed pagination loop (at up to 50
// playlists/page, ~5000 playlists) — a safety net against a misbehaving hub
// looping forever, not an expected ceiling.
const maxFeedPages = 100

// SyncPlaylists fetches the hub's editorial/recommendation catalog and, for
// each subscribed instance, its public playlist feed — materializing both as
// read-only federated playlists, resolving each track to a local one.
func (s *Service) SyncPlaylists(ctx context.Context) error {
	if !s.HubConfigured() {
		return nil
	}
	owner, err := s.federationOwner(ctx)
	if err != nil {
		return err
	}

	dist, err := s.hub.ListPlaylists(ctx, s.auth(), "")
	if err != nil {
		return err
	}
	for _, hp := range dist {
		if err := s.materializeDistribution(ctx, owner, hp); err != nil {
			s.logger.Warn("materialize distribution playlist failed", "name", deref(hp.Name), "error", err)
		}
	}

	if err := s.syncFeed(ctx, owner); err != nil {
		s.logger.Warn("playlist feed sync failed", "error", err)
	}
	return nil
}

// syncFeed pages through the subscribed-instances playlist feed and
// materializes each header's full playlist (tracks are fetched per header, the
// feed itself only carries metadata).
func (s *Service) syncFeed(ctx context.Context, owner string) error {
	var after string
	for page := 0; page < maxFeedPages; page++ {
		resp, err := s.hub.FeedPlaylists(ctx, s.auth(), after)
		if err != nil {
			return err
		}
		if resp.Playlists != nil {
			for _, hdr := range *resp.Playlists {
				if hdr.Author == nil || hdr.ExternalId == nil {
					continue
				}
				full, err := s.hub.GetFeedPlaylist(ctx, s.auth(), deref(hdr.Author.Id), deref(hdr.ExternalId))
				if err != nil {
					s.logger.Warn("fetch feed playlist failed", "instance", deref(hdr.Author.Id), "externalId", deref(hdr.ExternalId), "error", err)
					continue
				}
				if err := s.materializeFeed(ctx, owner, full); err != nil {
					s.logger.Warn("materialize feed playlist failed", "name", full.Name, "error", err)
				}
			}
		}
		if resp.HasMore == nil || !*resp.HasMore || resp.NextUpdatedAfter == nil {
			return nil
		}
		after = *resp.NextUpdatedAfter
	}
	s.logger.Warn("playlist feed sync stopped early", "pages", maxFeedPages)
	return nil
}

// materializeDistribution creates/updates one hub-editorial playlist (empty
// source instance id). Tracks are resolved locally (by mbid) only — no
// provider search here; an unmatched track is kept as an unresolved entry,
// resolved lazily at play time (see ResolvePlaylistTrack).
func (s *Service) materializeDistribution(ctx context.Context, ownerID string, hp hub.PublicDistributionPlaylist) error {
	var entries []persistence.FederatedTrackRef
	if hp.Tracks != nil {
		for _, ht := range *hp.Tracks {
			entries = append(entries, s.localTrackRef(ctx, deref(ht.Mbid), deref(ht.Artist), deref(ht.Title), deref(ht.Album)))
		}
	}
	return s.upsertFederated(ctx, ownerID, "", deref(hp.Id), deref(hp.Name), deref(hp.Comment), deref(hp.Image), entries)
}

// materializeFeed creates/updates one subscribed-instance playlist, keyed by
// (instance, externalId) so playlists sharing a name across instances don't
// collapse into one. Tracks are resolved locally only, same as distribution.
func (s *Service) materializeFeed(ctx context.Context, ownerID string, fp hub.FeedPlaylistDetail) error {
	entries := make([]persistence.FederatedTrackRef, 0, len(fp.Tracks))
	for _, ft := range fp.Tracks {
		entries = append(entries, s.localTrackRef(ctx, ft.Mbid, ft.Artist, ft.Title, ""))
	}
	return s.upsertFederated(ctx, ownerID, fp.InstanceID, fp.ExternalID, fp.Name, fp.Description, fp.Image, entries)
}

// localTrackRef resolves a portable track against the local catalog by mbid
// only (fast, no network); on a miss it keeps the portable identity for lazy
// resolution at play time instead of dropping the track.
func (s *Service) localTrackRef(ctx context.Context, mbid, artist, title, album string) persistence.FederatedTrackRef {
	ref := persistence.FederatedTrackRef{MBID: mbid, Artist: artist, Title: title, Album: album}
	if mbid != "" {
		if id, exists, _ := s.catalog.TrackExistsByMBIDOrHash(ctx, mbid, ""); exists {
			ref.TrackID = id
		}
	}
	return ref
}

// upsertFederated creates or updates the federated playlist sourced from
// (sourceInstanceID, sourceExternalID), replacing its tracks and cover.
func (s *Service) upsertFederated(ctx context.Context, ownerID, sourceInstanceID, sourceExternalID, name, comment, image string, entries []persistence.FederatedTrackRef) error {
	coverArt := s.federatedCoverArt(image)
	existing, err := s.playlists.FindFederated(ctx, sourceInstanceID, sourceExternalID)
	now := time.Now()
	if err == nil {
		existing.Name = name
		existing.Comment = comment
		_ = s.playlists.UpdateMeta(ctx, existing)
		if coverArt != existing.CoverArt {
			_ = s.playlists.SetCover(ctx, existing.ID, coverArt)
		}
		return s.playlists.ReplaceFederatedTracks(ctx, existing.ID, entries)
	}

	p := models.Playlist{
		ID:               uuid.NewString(),
		Name:             name,
		OwnerID:          ownerID,
		Comment:          comment,
		Public:           true,
		Federated:        true,
		SourceInstanceID: sourceInstanceID,
		SourceExternalID: sourceExternalID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.playlists.Create(ctx, p); err != nil {
		return err
	}
	// Create doesn't take a cover (playlists normally get one via a separate
	// SetCover call), so set it explicitly here.
	if coverArt != "" {
		_ = s.playlists.SetCover(ctx, p.ID, coverArt)
	}
	return s.playlists.ReplaceFederatedTracks(ctx, p.ID, entries)
}

// ErrUnresolvable is returned by ResolvePlaylistTrack when a playlist entry
// cannot be matched to a playable track (no local match and either portable-id
// resolution is off or no provider search turned up a candidate).
var ErrUnresolvable = fmt.Errorf("federation: track not resolvable")

// ResolvePlaylistTrack resolves one playlist entry to a playable track,
// lazily, at the moment the caller wants to play it: a local catalog lookup
// first (persisted back so future plays skip it), then a provider search
// returning an on-demand track (played progressively; the caller doesn't wait
// for a download). The entry's track_id column is a real foreign key into the
// local catalog, so a synthetic remote id can never be written there — a
// provider-search hit is kept in cachedResolve/cacheResolve for a while so
// repeat taps skip the search while a real local copy is pending. When the
// admin's auto-download-on-play setting is on (same policy as any other
// remote search result), a background download is kicked off so the entry
// gets a real, permanent track_id once it lands — see persistResolvedTrack.
func (s *Service) ResolvePlaylistTrack(ctx context.Context, playlistID string, position int) (models.Track, error) {
	ref, err := s.playlists.TrackRef(ctx, playlistID, position)
	if err != nil {
		return models.Track{}, err
	}
	if ref.TrackID != "" {
		return s.catalog.GetTrack(ctx, ref.TrackID)
	}
	if ref.MBID != "" {
		if id, exists, _ := s.catalog.TrackExistsByMBIDOrHash(ctx, ref.MBID, ""); exists {
			_ = s.playlists.ResolveFederatedTrack(ctx, playlistID, position, id)
			return s.catalog.GetTrack(ctx, id)
		}
	}
	// No (or no matching) mbid: the track may still already be in the local
	// catalog under a different mbid (or none at all, e.g. manually uploaded)
	// — check by artist+title before resorting to a remote provider search.
	if t, found, _ := s.catalog.FindByArtistTitle(ctx, ref.Artist, ref.Title); found {
		_ = s.playlists.ResolveFederatedTrack(ctx, playlistID, position, t.ID)
		return t, nil
	}
	// Keyed by content, not just position: a re-sync can replace the entry at
	// this position with a different track, which must not reuse a stale hit.
	cacheKey := playlistID + ":" + strconv.Itoa(position) + ":" + ref.Artist + "|" + ref.Title
	if t, ok := s.cachedResolve(cacheKey); ok {
		return t, nil
	}
	if s.resolver == nil {
		return models.Track{}, ErrUnresolvable
	}
	hit, ok := s.resolver.ResolveBestRemoteMatch(ctx, ref.Artist, ref.Title)
	if !ok {
		return models.Track{}, ErrUnresolvable
	}
	s.cacheResolve(cacheKey, hit)
	if s.resolver.AutoDownloadOnPlay() {
		go s.persistResolvedTrack(playlistID, position, hit.ID)
	}
	return hit, nil
}

// persistResolvedTrack downloads a provider hit in the background and, once it
// lands as a real local track, writes its id back to the playlist entry so
// future plays skip both the search and the download. Runs detached from the
// triggering request's context, which is very likely gone by the time a
// download finishes.
func (s *Service) persistResolvedTrack(playlistID string, position int, remoteID string) {
	ctx := context.Background()
	track, local, _, err := s.resolver.Resolve(ctx, "", remoteID)
	if err != nil || !local {
		return
	}
	_ = s.playlists.ResolveFederatedTrack(ctx, playlistID, position, track.ID)
}

// cachedResolve returns a still-fresh provider-search hit cached by
// cacheResolve, so repeat taps on the same unresolved entry skip the search.
func (s *Service) cachedResolve(key string) (models.Track, bool) {
	s.resolveCacheMu.Lock()
	defer s.resolveCacheMu.Unlock()
	e, ok := s.resolveCache[key]
	if !ok || time.Since(e.at) > resolveCacheTTL {
		return models.Track{}, false
	}
	return e.track, true
}

// cacheResolve remembers a provider-search hit for cacheKey.
// ponytail: process-memory only (lost on restart, not shared across
// instances behind a load balancer); promote to a DB column
// (provider/providerTrackID on playlist_tracks) if that turns out to matter.
func (s *Service) cacheResolve(key string, t models.Track) {
	s.resolveCacheMu.Lock()
	defer s.resolveCacheMu.Unlock()
	if s.resolveCache == nil {
		s.resolveCache = map[string]resolveCacheEntry{}
	}
	s.resolveCache[key] = resolveCacheEntry{track: t, at: time.Now()}
}

// federatedCoverArt turns a hub-sourced cover reference into a local remote-
// cover id (fetched and cached on demand by the cover service, subject to its
// host allowlist). The hub returns cover URLs relative to itself (e.g.
// "/api/v1/covers/<hash>"), so a relative value is resolved against the
// configured hub URL; an already-absolute URL (the editorial catalog may send
// one) is used as-is.
func (s *Service) federatedCoverArt(image string) string {
	if image == "" {
		return ""
	}
	if strings.HasPrefix(image, "http://") || strings.HasPrefix(image, "https://") {
		return models.RemoteCoverID(image)
	}
	return models.RemoteCoverID(strings.TrimRight(s.cfg().HubURL, "/") + image)
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
