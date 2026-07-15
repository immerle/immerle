// Package autoplaylists materializes auto-generated playlists, refreshed
// periodically, using the same public/federated-style materializer mechanism
// internal/charts uses for curated chart playlists — just sourced from the
// local library/scrobble history instead of a remote chart feed:
//
//   - Genre and decade playlists ("Rock", "Rap", "1990s"...): shared, public,
//     one per genre/decade with enough tracks.
//   - Personal listening lists ("Top du mois", "On Repeat", "Favoris
//     oubliés"): one each per user, private, real playlist rows rather than a
//     bespoke live-computed view. They're intentionally not subscribed into
//     the owner's library (see playlistSpec) — a user unsubscribing/unliking
//     one (easy to do by mistake, since Federated hides normal owner
//     controls) must not lose access to it. GET /me/custom-playlists is the
//     dedicated lookup, independent of subscriptions.
package autoplaylists

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// Source instance ids distinguish each kind in the dedupe key (FindFederated),
// so e.g. a genre and a decade never collide even if a genre happened to be
// named "1990s". Personal lists key their per-user row by (kind, userID)
// instead of (kind, name); the personal ones are exported so
// GET /me/custom-playlists (internal/api/immerle) can look each one up
// directly by (kind, callerID) without going through ListVisible/subscriptions.
const (
	sourceGenre  = "genre-mix"
	sourceDecade = "decade-mix"

	SourceTopMonth  = "top-month-mix"
	SourceOnRepeat  = "on-repeat-mix"
	SourceForgotten = "forgotten-mix"
)

// minTracks is the minimum catalog size for a genre/decade to get its own
// playlist — below this it'd be a near-empty, not-worth-it playlist. Personal
// lists have no such threshold (even 1-2 tracks are a meaningful personal
// recap); they're simply skipped for a user with zero matching tracks.
const minTracks = 15

// maxTracks bounds how many tracks a single genre/decade auto-playlist holds;
// maxPersonalTracks bounds a personal list.
const maxTracks = 100
const maxPersonalTracks = 20

// forgottenMinDays is how long a starred track must go unplayed (or never
// played at all) to count as "forgotten."
const forgottenMinDays = 90

// defaultInterval is the sync cadence. Daily (not weekly, like charts): unlike
// a remote chart feed, the local catalog/scrobbles change every day, so
// genre/decade membership and personal listening lists can shift daily.
const defaultInterval = 24 * time.Hour

// Service syncs genre/decade and personal-listening auto-playlists.
// Genre/decade tracks are already local, so they're set directly by id
// (PlaylistRepo.ReplaceTracks) — no artist/title resolution needed, unlike
// internal/charts.
type Service struct {
	catalog     *persistence.CatalogRepo
	genres      *persistence.GenreRepo
	wrapped     *persistence.WrappedRepo
	annotations *persistence.AnnotationRepo
	users       *persistence.UserRepo
	playlists   *persistence.PlaylistRepo
	interval    time.Duration
	logger      *slog.Logger

	ownerID string
	ownerFn func(ctx context.Context) (string, error)
}

// New builds a Service.
func New(
	catalog *persistence.CatalogRepo,
	genres *persistence.GenreRepo,
	wrapped *persistence.WrappedRepo,
	annotations *persistence.AnnotationRepo,
	users *persistence.UserRepo,
	playlists *persistence.PlaylistRepo,
	logger *slog.Logger,
) *Service {
	return &Service{
		catalog: catalog, genres: genres, wrapped: wrapped, annotations: annotations, users: users,
		playlists: playlists, interval: defaultInterval, logger: logger,
	}
}

// SetOwner pins the nominal owner of shared (genre/decade) synced playlists
// (mainly for tests) — personal lists are always owned by their own user.
func (s *Service) SetOwner(id string) { s.ownerID = id }

// SetOwnerResolver lazily resolves (and caches) the nominal owner of shared
// playlists — typically the first admin account — so this works even before
// one exists at boot.
func (s *Service) SetOwnerResolver(fn func(ctx context.Context) (string, error)) {
	s.ownerFn = fn
}

func (s *Service) owner(ctx context.Context) (string, error) {
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
	return "", fmt.Errorf("autoplaylists: owner not configured")
}

// SyncNow rebuilds every genre/decade auto-playlist and every user's personal
// listening lists, returning how many synced successfully (one failing is
// logged and skipped rather than aborting the rest).
func (s *Service) SyncNow(ctx context.Context) (int, error) {
	ownerID, err := s.owner(ctx)
	if err != nil {
		return 0, err
	}
	synced := 0
	synced += s.syncGenres(ctx, ownerID)
	synced += s.syncDecades(ctx, ownerID)
	synced += s.syncPersonal(ctx)
	return synced, nil
}

func (s *Service) syncGenres(ctx context.Context, ownerID string) int {
	genres, err := s.genres.List(ctx)
	if err != nil {
		s.logger.Warn("autoplaylists: list genres failed", "error", err)
		return 0
	}
	synced := 0
	for _, g := range genres {
		if g.Name == "" || g.SongCount < minTracks {
			continue
		}
		tracks, err := s.catalog.ListTracksByGenre(ctx, g.Name, maxTracks, 0)
		if err != nil {
			s.logger.Warn("autoplaylists: list genre tracks failed", "genre", g.Name, "error", err)
			continue
		}
		if err := s.upsert(ctx, playlistSpec{
			ownerID: ownerID, sourceInstanceID: sourceGenre, sourceExternalID: g.Name,
			name: g.Name, icon: musicNoteIcon, public: true, ids: trackIDs(tracks),
		}); err != nil {
			s.logger.Warn("autoplaylists: genre sync failed", "genre", g.Name, "error", err)
			continue
		}
		synced++
	}
	return synced
}

func (s *Service) syncDecades(ctx context.Context, ownerID string) int {
	synced := 0
	for _, d := range decades() {
		tracks, err := s.catalog.ListTracksByYearRange(ctx, d.from, d.from+10, maxTracks, 0)
		if err != nil {
			s.logger.Warn("autoplaylists: list decade tracks failed", "decade", d.label, "error", err)
			continue
		}
		if len(tracks) < minTracks {
			continue
		}
		if err := s.upsert(ctx, playlistSpec{
			ownerID: ownerID, sourceInstanceID: sourceDecade, sourceExternalID: d.label,
			name: d.label, icon: musicNoteIcon, public: true, ids: trackIDs(tracks),
		}); err != nil {
			s.logger.Warn("autoplaylists: decade sync failed", "decade", d.label, "error", err)
			continue
		}
		synced++
	}
	return synced
}

// syncPersonal rebuilds every user's "Top du mois"/"On Repeat"/"Favoris
// oubliés" — real, private playlists (looked up directly by GET
// /me/custom-playlists, not via the owner's subscribed library), not a
// live-computed view.
func (s *Service) syncPersonal(ctx context.Context) int {
	users, err := s.users.List(ctx)
	if err != nil {
		s.logger.Warn("autoplaylists: list users failed", "error", err)
		return 0
	}
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	synced := 0
	for _, u := range users {
		if s.syncPersonalOne(ctx, u.ID, SourceTopMonth, topMonthName, trendingUpIcon,
			func() ([]string, error) { return s.topTrackIDs(ctx, u.ID, monthStart.UnixMilli(), now.UnixMilli()) }) {
			synced++
		}
		if s.syncPersonalOne(ctx, u.ID, SourceOnRepeat, onRepeatName, repeatIcon,
			func() ([]string, error) {
				return s.topTrackIDs(ctx, u.ID, now.AddDate(0, 0, -30).UnixMilli(), now.UnixMilli())
			}) {
			synced++
		}
		if s.syncPersonalOne(ctx, u.ID, SourceForgotten, forgottenName, hourglassIcon,
			func() ([]string, error) {
				return s.annotations.ForgottenFavorites(ctx, u.ID, models.ItemTrack, now.AddDate(0, 0, -forgottenMinDays), maxPersonalTracks)
			}) {
			synced++
		}
	}
	return synced
}

// syncPersonalOne resolves one personal list's track ids and upserts it if
// non-empty (an inactive-this-window user just doesn't get that list — no
// point in an empty playlist), reporting whether it synced.
func (s *Service) syncPersonalOne(ctx context.Context, userID, sourceInstanceID, name, icon string, list func() ([]string, error)) bool {
	ids, err := list()
	if err != nil {
		s.logger.Warn("autoplaylists: personal list failed", "kind", sourceInstanceID, "user", userID, "error", err)
		return false
	}
	if len(ids) == 0 {
		return false
	}
	if err := s.upsert(ctx, playlistSpec{
		ownerID: userID, sourceInstanceID: sourceInstanceID, sourceExternalID: userID,
		name: name, icon: icon, public: false, ids: ids,
	}); err != nil {
		s.logger.Warn("autoplaylists: personal list sync failed", "kind", sourceInstanceID, "user", userID, "error", err)
		return false
	}
	return true
}

// topTrackIDs resolves a WrappedRepo.TopTracks window straight to track ids.
func (s *Service) topTrackIDs(ctx context.Context, userID string, start, end int64) ([]string, error) {
	top, err := s.wrapped.TopTracks(ctx, userID, start, end, maxPersonalTracks)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(top))
	for i, t := range top {
		ids[i] = t.ID
	}
	return ids, nil
}

// Personal-list display names. Plain strings (like genre/decade names, and
// like internal/charts' chart names) — a playlist's stored Name isn't
// per-viewer-locale, same limitation every server-generated playlist name has.
const (
	topMonthName  = "Top du mois"
	onRepeatName  = "On Repeat"
	forgottenName = "Favoris oubliés"
)

// Twemoji codepoints (see covergen.FetchEmoji) for each auto-playlist kind.
const (
	musicNoteIcon  = "1f3b5" // 🎵 — genre/decade playlists (free-text tags, no fixed per-genre icon)
	trendingUpIcon = "1f4c8" // 📈 — Top du mois
	repeatIcon     = "1f501" // 🔁 — On Repeat
	hourglassIcon  = "23f3"  // ⏳ — Favoris oubliés
)

// decade is one candidate decade bucket: [from, from+10).
type decade struct {
	from  int
	label string
}

// decades enumerates every decade from the 1950s through the current one —
// ListTracksByYearRange returning too few tracks for a given decade (see
// minTracks in syncDecades) naturally skips ones the local catalog has
// nothing in, so there's no need to first query which years are present.
func decades() []decade {
	var out []decade
	for from := 1950; from <= (time.Now().Year()/10)*10; from += 10 {
		out = append(out, decade{from: from, label: fmt.Sprintf("%ds", from)})
	}
	return out
}

func trackIDs(tracks []models.Track) []string {
	ids := make([]string, len(tracks))
	for i, t := range tracks {
		ids[i] = t.ID
	}
	return ids
}

// playlistSpec is one auto-playlist to create-or-refresh.
type playlistSpec struct {
	ownerID          string
	sourceInstanceID string
	sourceExternalID string
	name             string
	icon             string
	public           bool
	ids              []string
}

// upsert creates or refreshes the auto-playlist for (sourceInstanceID,
// sourceExternalID), mirroring internal/charts.syncOne's find-or-create
// shape. Always Federated (read-only — a resync would just undo any edit),
// public or private per spec.public.
func (s *Service) upsert(ctx context.Context, spec playlistSpec) error {
	cover := models.GeneratorCoverID(coverParams(spec.name, spec.icon))
	existing, err := s.playlists.FindFederated(ctx, spec.sourceInstanceID, spec.sourceExternalID)
	switch {
	case err == nil:
		existing.Name = spec.name
		existing.Public = spec.public
		if err := s.playlists.UpdateMeta(ctx, existing); err != nil {
			return err
		}
		if existing.CoverArt != cover {
			if err := s.playlists.SetCover(ctx, existing.ID, cover); err != nil {
				s.logger.Warn("autoplaylists: cover update failed", "name", spec.name, "error", err)
			}
		}
		return s.playlists.ReplaceTracks(ctx, existing.ID, spec.ids, spec.ownerID)
	case errors.Is(err, persistence.ErrNotFound):
		now := time.Now()
		p := models.Playlist{
			ID: uuid.NewString(), Name: spec.name, OwnerID: spec.ownerID, Public: spec.public, Federated: true,
			SourceInstanceID: spec.sourceInstanceID, SourceExternalID: spec.sourceExternalID,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := s.playlists.Create(ctx, p); err != nil {
			return err
		}
		if err := s.playlists.SetCover(ctx, p.ID, cover); err != nil {
			s.logger.Warn("autoplaylists: cover save failed", "name", spec.name, "error", err)
		}
		return s.playlists.ReplaceTracks(ctx, p.ID, spec.ids, spec.ownerID)
	default:
		return err
	}
}

// coverGradients cycles a small fixed palette across genres/decades/personal
// lists — picked deterministically by name so a given one always gets the
// same look across resyncs, without needing a per-name color mapping (genre
// names in particular are open-ended free-text tags, no fixed list to map).
var coverGradients = [][2]string{
	{"#1db954", "#0b3d20"},
	{"#5b21b6", "#1e1b4b"},
	{"#be123c", "#4c0519"},
	{"#0369a1", "#0c2340"},
	{"#b45309", "#451a03"},
	{"#0f766e", "#042f2e"},
}

// coverParams builds the generator-cover query values (see
// internal/models.GeneratorCoverID and GET /cover/generator) for an
// auto-playlist: icon over a gradient picked from coverGradients by name.
func coverParams(name, icon string) url.Values {
	sum := 0
	for _, r := range name {
		sum += int(r)
	}
	g := coverGradients[sum%len(coverGradients)]

	vals := url.Values{}
	vals.Set("icon", icon)
	vals.Set("title", name)
	vals.Set("color", g[0])
	vals.Set("color2", g[1])
	vals.Set("angle", "45")
	return vals
}

// Run syncs on the configured interval until ctx is cancelled — once
// immediately, then every interval (daily by default).
func (s *Service) Run(ctx context.Context) {
	if _, err := s.SyncNow(ctx); err != nil {
		s.logger.Warn("autoplaylists: initial sync failed", "error", err)
	}
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := s.SyncNow(ctx); err != nil {
				s.logger.Warn("autoplaylists: sync failed", "error", err)
			}
		}
	}
}
