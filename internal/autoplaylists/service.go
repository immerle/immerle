// Package autoplaylists materializes auto-generated playlists, refreshed
// periodically, using the same public/federated-style materializer mechanism
// internal/charts uses for curated chart playlists — just sourced from the
// local library/scrobble history instead of a remote chart feed:
//
//   - Genre and decade playlists ("Rock", "Rap", "1990s"...): shared, public,
//     one per genre/decade with enough tracks.
//   - "Tendances de la semaine": shared, public, single community-wide chart
//     of the most-scrobbled tracks (any user) in the last 7 days.
//   - Personal listening lists ("Top du mois", "On Repeat", "Favoris
//     oubliés", "Aléatoire"): one each per user, private, real playlist rows
//     rather than a bespoke live-computed view. They're intentionally not
//     subscribed into the owner's library (see playlistSpec) — a user
//     unsubscribing/unliking one (easy to do by mistake, since Federated
//     hides normal owner controls) must not lose access to it. GET
//     /me/custom-playlists is the dedicated lookup, independent of
//     subscriptions.
package autoplaylists

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
	"github.com/immerle/immerle/internal/reccobeats"
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

	SourceTopMonth    = "top-month-mix"
	SourceOnRepeat    = "on-repeat-mix"
	SourceForgotten   = "forgotten-mix"
	SourceRandom      = "random-mix"
	SourceRecommended = "recommended-mix"
	SourceTrending    = "weekly-trending-mix"
)

// AutoPlaylistKinds is the fixed, stable set of source instance ids this
// package's playlists use — personal lists plus the trending chart — safe to
// expose to API clients (see internal/api/immerle playlistView.
// AutoPlaylistKind) so they can render a translated label instead of the
// (French-only) stored Name. Unlike genre/decade playlists (free-text tags,
// not a fixed enum) or a federation-imported playlist's real instance id
// (internal only, never meant to leave the server), these six values never
// change and carry no per-instance/per-user information.
var AutoPlaylistKinds = map[string]bool{
	SourceTopMonth:    true,
	SourceOnRepeat:    true,
	SourceForgotten:   true,
	SourceRandom:      true,
	SourceRecommended: true,
	SourceTrending:    true,
}

// minTracks is the minimum catalog size for a genre/decade to get its own
// playlist — below this it'd be a near-empty, not-worth-it playlist. Personal
// lists have no such threshold (even 1-2 tracks are a meaningful personal
// recap); they're simply skipped for a user with zero matching tracks.
const minTracks = 15

// maxTracks bounds how many tracks a single genre/decade auto-playlist holds;
// maxPersonalTracks bounds a personal list.
const maxTracks = 100
const maxPersonalTracks = 20

// randomTrackCount is how many tracks the "Aléatoire" personal list holds —
// its own size, distinct from maxPersonalTracks.
const randomTrackCount = 30

// maxRecommendedTracks bounds the "Découvertes" personal list — the max the
// user asked for. recommendedSeedCount is how many of the user's top tracks
// seed the ReccoBeats request (capped at its own maxSeeds limit anyway).
// recommendedFetchSize is how many candidates to ask ReccoBeats for per seed
// batch — oversized relative to maxRecommendedTracks since most of its
// catalog (built from Spotify metadata) won't exist in any given local
// library.
const maxRecommendedTracks = 50
const recommendedSeedCount = 5
const recommendedFetchSize = 100

// forgottenMinDays is how long a starred track must go unplayed (or never
// played at all) to count as "forgotten."
const forgottenMinDays = 90

// trendingWindowDays/trendingTrackCount size the "Tendances de la semaine"
// chart; trendingExternalID is its dedupe key (there's only ever one).
const trendingWindowDays = 7
const trendingTrackCount = 50
const trendingExternalID = "global"

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

	// recco is nil unless SetRecommender is called — the "Découvertes"
	// personal list is simply skipped in that case (no external network call
	// made), same as any other optional wiring in this package.
	recco Recommender
}

// Recommender resolves recommended tracks from seed tracks. *reccobeats.Client
// satisfies this; it's a separate interface only so tests can fake it instead
// of making a real network call.
type Recommender interface {
	Recommend(ctx context.Context, seeds []reccobeats.Seed, size int) ([]reccobeats.Track, error)
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

// SetRecommender wires up the "Découvertes" personal list (recommendations
// from the keyless ReccoBeats API, seeded from each user's top tracks).
// Optional: not calling this simply leaves that list unsynced.
func (s *Service) SetRecommender(r Recommender) {
	s.recco = r
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
	synced += s.syncTrending(ctx, ownerID)
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

// syncTrending rebuilds the single community-wide "Tendances de la semaine"
// chart from the last trendingWindowDays of scrobbles, any user. Below
// minTracks it's skipped, same threshold as genre/decade.
func (s *Service) syncTrending(ctx context.Context, ownerID string) int {
	now := time.Now()
	top, err := s.wrapped.GlobalTopTracks(ctx, now.AddDate(0, 0, -trendingWindowDays).UnixMilli(), now.UnixMilli(), trendingTrackCount)
	if err != nil {
		s.logger.Warn("autoplaylists: trending sync failed", "error", err)
		return 0
	}
	ids := make([]string, len(top))
	for i, t := range top {
		ids[i] = t.ID
	}
	if len(ids) < minTracks {
		return 0
	}
	if err := s.upsert(ctx, playlistSpec{
		ownerID: ownerID, sourceInstanceID: SourceTrending, sourceExternalID: trendingExternalID,
		name: trendingName, icon: fireIcon, public: true, ids: ids,
	}); err != nil {
		s.logger.Warn("autoplaylists: trending sync failed", "error", err)
		return 0
	}
	return 1
}

// syncPersonal rebuilds every user's "Top du mois"/"On Repeat"/"Favoris
// oubliés"/"Aléatoire" — real, private playlists (looked up directly by GET
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
		if s.syncPersonalOne(ctx, u.ID, SourceRandom, randomName, diceIcon,
			func() ([]string, error) { return s.randomTrackIDs(ctx) }) {
			synced++
		}
		if s.recco != nil && s.syncRecommendedOne(ctx, u.ID) {
			synced++
		}
	}
	return synced
}

// syncRecommendedOne rebuilds one user's "Découvertes" list. Unlike every
// other personal list, its tracks are kept as portable artist/title
// references (persistence.FederatedTrackRef), resolved lazily at play time
// (federation.Service.ResolvePlaylistTrack) exactly like a kworb chart entry
// — see internal/charts. ReccoBeats' catalog is Spotify-side and has no
// notion of what's already in any given local library, so requiring an exact
// local match up front (like every other personal list does) would leave
// "Découvertes" empty for almost anyone with a small or niche collection.
func (s *Service) syncRecommendedOne(ctx context.Context, userID string) bool {
	refs, err := s.recommendedTrackRefs(ctx, userID)
	if err != nil {
		s.logger.Warn("autoplaylists: personal list failed", "kind", SourceRecommended, "user", userID, "error", err)
		return false
	}
	if len(refs) == 0 {
		return false
	}
	if err := s.upsert(ctx, playlistSpec{
		ownerID: userID, sourceInstanceID: SourceRecommended, sourceExternalID: userID,
		name: recommendedName, icon: compassIcon, public: false, refs: refs,
	}); err != nil {
		s.logger.Warn("autoplaylists: personal list sync failed", "kind", SourceRecommended, "user", userID, "error", err)
		return false
	}
	return true
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

// randomTrackIDs picks randomTrackCount random tracks from the whole catalog
// (not scoped to the user — unlike the other personal lists, "Aléatoire"
// doesn't depend on listening history, just a fresh shuffle each sync).
func (s *Service) randomTrackIDs(ctx context.Context) ([]string, error) {
	tracks, err := s.catalog.RandomTracks(ctx, randomTrackCount, "", 0, 0)
	if err != nil {
		return nil, err
	}
	return trackIDs(tracks), nil
}

// recommendedTrackRefs seeds a ReccoBeats recommendation request with the
// user's all-time top tracks and returns up to maxRecommendedTracks of what
// comes back as portable artist/title references — deliberately not matched
// against the local catalog here (unlike every other personal list): with a
// small or niche library, requiring an exact match up front would leave
// "Découvertes" empty almost always, since ReccoBeats' own catalog skews
// toward globally mainstream/Spotify-side metadata. Left unresolved, each
// entry instead gets resolved the same lazy way a kworb chart entry does —
// local catalog first, else a remote provider search — the first time it's
// actually played (see internal/charts and federation.Service.
// ResolvePlaylistTrack). A user with no listening history yet, or whose
// favorites don't resolve to a ReccoBeats seed, just skips this list (like
// any other personal list with nothing to show).
func (s *Service) recommendedTrackRefs(ctx context.Context, userID string) ([]persistence.FederatedTrackRef, error) {
	top, err := s.wrapped.TopTracks(ctx, userID, 0, time.Now().UnixMilli(), recommendedSeedCount)
	if err != nil || len(top) == 0 {
		return nil, err
	}
	seeds := make([]reccobeats.Seed, len(top))
	seen := make(map[string]bool, len(top))
	for i, t := range top {
		seeds[i] = reccobeats.Seed{Artist: t.Artist, Title: t.Title}
		seen[seedKey(t.Artist, t.Title)] = true
	}
	recs, err := s.recco.Recommend(ctx, seeds, recommendedFetchSize)
	if err != nil {
		return nil, err
	}
	refs := make([]persistence.FederatedTrackRef, 0, maxRecommendedTracks)
	for _, r := range recs {
		if len(refs) >= maxRecommendedTracks {
			break
		}
		key := seedKey(r.Artist, r.Title)
		if seen[key] {
			continue
		}
		seen[key] = true
		refs = append(refs, persistence.FederatedTrackRef{Artist: r.Artist, Title: r.Title})
	}
	return refs, nil
}

// seedKey is a case-insensitive (artist, title) dedupe key: ReccoBeats often
// returns the same track multiple times over (different Spotify pressings
// sharing one ISRC), and a recommendation must not just echo back one of the
// seeds it was generated from.
func seedKey(artist, title string) string {
	return strings.ToLower(artist) + "\x00" + strings.ToLower(title)
}

// Personal-list display names. Plain strings (like genre/decade names, and
// like internal/charts' chart names) — a playlist's stored Name isn't
// per-viewer-locale, same limitation every server-generated playlist name has.
const (
	topMonthName    = "Top du mois"
	onRepeatName    = "On Repeat"
	forgottenName   = "Favoris oubliés"
	randomName      = "Aléatoire"
	trendingName    = "Tendances de la semaine"
	recommendedName = "Découvertes"
)

// Twemoji codepoints (see covergen.FetchEmoji) for each auto-playlist kind.
const (
	musicNoteIcon  = "1f3b5" // 🎵 — genre/decade playlists (free-text tags, no fixed per-genre icon)
	trendingUpIcon = "1f4c8" // 📈 — Top du mois
	repeatIcon     = "1f501" // 🔁 — On Repeat
	diceIcon       = "1f3b2" // 🎲 — Aléatoire
	hourglassIcon  = "23f3"  // ⏳ — Favoris oubliés
	fireIcon       = "1f525" // 🔥 — Tendances de la semaine
	compassIcon    = "1f9ed" // 🧭 — Découvertes
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

// playlistSpec is one auto-playlist to create-or-refresh. Exactly one of ids
// (already-local tracks, set directly by id) or refs (portable artist/title
// references, resolved lazily at play time — see replaceTracks) is set;
// every kind but recommended-mix uses ids.
type playlistSpec struct {
	ownerID          string
	sourceInstanceID string
	sourceExternalID string
	name             string
	icon             string
	public           bool
	ids              []string
	refs             []persistence.FederatedTrackRef
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
		return s.replaceTracks(ctx, existing.ID, spec)
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
		return s.replaceTracks(ctx, p.ID, spec)
	default:
		return err
	}
}

// replaceTracks writes spec's tracks into playlistID: refs (portable
// artist/title, resolved lazily at play time — see federation.Service.
// ResolvePlaylistTrack) when set, else ids (already-local tracks).
func (s *Service) replaceTracks(ctx context.Context, playlistID string, spec playlistSpec) error {
	if spec.refs != nil {
		return s.playlists.ReplaceFederatedTracks(ctx, playlistID, spec.refs)
	}
	return s.playlists.ReplaceTracks(ctx, playlistID, spec.ids, spec.ownerID)
}

// coverGradients cycles a small fixed palette across genres/decades/personal
// lists — picked deterministically by name so a given one always gets the
// same look across resyncs, without needing a per-name color mapping (genre
// names in particular are open-ended free-text tags, no fixed list to map).
var coverGradients = [][2]string{
	{"#22d3ee", "#0c4a6e"}, // cyan → deep teal
	{"#f472b6", "#831843"}, // pink → deep magenta
	{"#fb923c", "#7c2d12"}, // orange → burnt umber
	{"#34d399", "#064e3b"}, // emerald → deep green
	{"#a78bfa", "#3730a3"}, // violet → indigo
	{"#facc15", "#78350f"}, // amber → deep gold-brown
	{"#38bdf8", "#1e3a8a"}, // sky blue → navy
	{"#fb7185", "#881337"}, // rose → deep maroon
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
