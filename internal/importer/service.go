package importer

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gossignol/gossignol/internal/models"
	"github.com/gossignol/gossignol/internal/persistence"
)

// ContentResolver is the slice of the on-demand catalog the importer needs:
// search the content providers for a free-text query, and turn a (remote) track
// id into a local track id by downloading/ingesting it. *core.CatalogService is
// adapted to this in the app wiring.
type ContentResolver interface {
	SearchTracks(ctx context.Context, query string, limit int) ([]models.Track, error)
	Resolve(ctx context.Context, userID, trackID string) (localTrackID string, err error)
}

// Service runs playlist imports: it pulls a playlist from a source, creates a
// gossignol playlist and resolves each source track against the content
// providers, recording per-track status so a UI can follow progress.
type Service struct {
	imports   *persistence.ImportRepo
	playlists *persistence.PlaylistRepo
	resolver  ContentResolver
	// hub fetches external playlists through the federation hub (nil when no hub
	// is configured). Hub-backed sources (e.g. spotify) are unavailable then.
	hub HubFetcher
	// sourceConfig returns the live per-source settings (keyed by source name).
	sourceConfig func() map[string]map[string]string
	logger       *slog.Logger
	wake         chan struct{}
}

// NewService builds the import service. hub may be nil (no federation hub).
// sourceConfig is read live so credential changes apply without a restart.
func NewService(imports *persistence.ImportRepo, playlists *persistence.PlaylistRepo, resolver ContentResolver, hub HubFetcher, sourceConfig func() map[string]map[string]string, logger *slog.Logger) *Service {
	return &Service{
		imports:      imports,
		playlists:    playlists,
		resolver:     resolver,
		hub:          hub,
		sourceConfig: sourceConfig,
		logger:       logger,
		wake:         make(chan struct{}, 1),
	}
}

// build instantiates a source by name with the live deps (settings + hub).
func (s *Service) build(name string) (Source, error) {
	var settings map[string]string
	if cfg := s.sourceConfig(); cfg != nil {
		settings = cfg[name]
	}
	return Build(name, SourceDeps{Settings: settings, Hub: s.hub})
}

// SourceInfo describes an available import source and whether it is configured.
type SourceInfo struct {
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
}

// Sources lists the registered import sources and whether each has usable config.
func (s *Service) Sources() []SourceInfo {
	out := make([]SourceInfo, 0)
	for _, name := range Available() {
		_, err := s.build(name)
		out = append(out, SourceInfo{Name: name, Configured: err == nil})
	}
	return out
}

// Start validates the source and queues an import for the user. The heavy work
// runs in the background Worker; poll Get for progress.
func (s *Service) Start(ctx context.Context, userID, source, ref string) (models.Import, error) {
	source = strings.TrimSpace(source)
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return models.Import{}, fmt.Errorf("a playlist reference is required")
	}
	if !HasFactory(source) {
		return models.Import{}, fmt.Errorf("unknown import source %q", source)
	}
	// Fail fast if the source isn't configured (so the user gets an immediate
	// error instead of a queued job that flips to failed).
	if _, err := s.build(source); err != nil {
		return models.Import{}, err
	}
	now := time.Now()
	im := models.Import{
		ID:        uuid.NewString(),
		UserID:    userID,
		Source:    source,
		SourceRef: ref,
		Status:    models.ImportQueued,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.imports.Create(ctx, im); err != nil {
		return models.Import{}, err
	}
	s.signal()
	return im, nil
}

// Get returns an import with its items (ownership-checked).
func (s *Service) Get(ctx context.Context, userID, id string) (models.Import, error) {
	im, err := s.imports.Get(ctx, id)
	if err != nil {
		return models.Import{}, err
	}
	if im.UserID != userID {
		return models.Import{}, persistence.ErrNotFound
	}
	items, err := s.imports.ListItems(ctx, id)
	if err != nil {
		return models.Import{}, err
	}
	im.Items = items
	return im, nil
}

// List returns a user's imports (without items).
func (s *Service) List(ctx context.Context, userID string) ([]models.Import, error) {
	return s.imports.ListByUser(ctx, userID, 50)
}

// ResolveItem validates or modifies a not-yet-matched item: it downloads a
// chosen track and adds it to the import's playlist, flipping the item to
// matched and adjusting the import counters.
//
//   - query == "" : validate — resolve the item's flagged candidate as-is.
//   - query != "" : modify  — re-search the content providers with the corrected
//     query and use the best result (its similarity becomes the new confidence).
//
// It is ownership-checked and rejects items that are already matched.
func (s *Service) ResolveItem(ctx context.Context, userID, itemID, query string) (models.ImportItem, error) {
	it, err := s.imports.GetItem(ctx, itemID)
	if err != nil {
		return models.ImportItem{}, err
	}
	im, err := s.imports.Get(ctx, it.ImportID)
	if err != nil {
		return models.ImportItem{}, err
	}
	if im.UserID != userID {
		return models.ImportItem{}, persistence.ErrNotFound
	}
	if it.Status == models.ImportItemMatched {
		return models.ImportItem{}, fmt.Errorf("item is already matched")
	}

	candidateID := it.CandidateID
	query = strings.TrimSpace(query)
	if query != "" {
		// Modify: re-search and take the best candidate for the corrected query.
		cands, err := s.resolver.SearchTracks(ctx, query, 5)
		if err != nil {
			return models.ImportItem{}, fmt.Errorf("search failed: %w", err)
		}
		if len(cands) == 0 {
			return models.ImportItem{}, fmt.Errorf("no track found for %q", query)
		}
		best, score := bestMatch(it.SourceArtist, it.SourceTitle, cands)
		candidateID = best.ID
		it.ResolvedTitle = best.Title
		it.ResolvedArtist = best.ArtistName
		it.CandidateCoverArt = best.CoverArt
		it.Confidence = score
	}
	if candidateID == "" {
		return models.ImportItem{}, fmt.Errorf("no candidate to validate; provide a query")
	}

	localID, err := s.resolver.Resolve(ctx, userID, candidateID)
	if err != nil {
		return models.ImportItem{}, fmt.Errorf("download failed: %w", err)
	}
	if err := s.playlists.AppendTracks(ctx, im.PlaylistID, []string{localID}, userID); err != nil {
		return models.ImportItem{}, fmt.Errorf("add to playlist failed: %w", err)
	}

	// Move the item to matched and rebalance the import counters.
	switch it.Status {
	case models.ImportItemDoubtful:
		im.Doubtful--
	case models.ImportItemMissing:
		im.Missing--
	case models.ImportItemFailed:
		im.Failed--
	}
	im.Matched++
	it.Status = models.ImportItemMatched
	it.MatchedTrackID = localID
	it.CandidateID = candidateID
	it.Note = ""

	if err := s.imports.UpdateItem(ctx, it); err != nil {
		return models.ImportItem{}, err
	}
	if err := s.imports.Update(ctx, im); err != nil {
		return models.ImportItem{}, err
	}
	return it, nil
}

func (s *Service) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Worker drains queued imports until ctx is done. On start it requeues imports
// left running by a previous crash.
func (s *Service) Worker(ctx context.Context) {
	_ = s.imports.RequeueStale(ctx)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		for {
			im, err := s.imports.ClaimNext(ctx)
			if err != nil {
				break // empty queue (ErrNotFound) or transient error
			}
			s.runImport(ctx, im)
			if ctx.Err() != nil {
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		case <-ticker.C:
		}
	}
}

// runImport executes a claimed import, recording progress as it goes. Fatal
// errors (source fetch) mark the whole import failed; per-track problems are
// recorded on the item and counted, leaving the import "completed".
func (s *Service) runImport(ctx context.Context, im models.Import) {
	if err := s.process(ctx, &im); err != nil {
		im.Status = models.ImportFailed
		im.Error = err.Error()
		im.UpdatedAt = time.Now()
		if uErr := s.imports.Update(ctx, im); uErr != nil {
			s.logger.Warn("import update failed", "import", im.ID, "error", uErr)
		}
		s.logger.Warn("import failed", "import", im.ID, "source", im.Source, "error", err)
		return
	}
	im.Status = models.ImportCompleted
	if err := s.imports.Update(ctx, im); err != nil {
		s.logger.Warn("import update failed", "import", im.ID, "error", err)
	}
	s.logger.Info("import complete", "import", im.ID, "matched", im.Matched,
		"doubtful", im.Doubtful, "missing", im.Missing, "failed", im.Failed)
}

func (s *Service) process(ctx context.Context, im *models.Import) error {
	src, err := s.build(im.Source)
	if err != nil {
		return err
	}
	pl, err := src.FetchPlaylist(ctx, im.SourceRef)
	if err != nil {
		return err
	}

	// Create the destination gossignol playlist (distinct from the source).
	now := time.Now()
	name := strings.TrimSpace(pl.Name)
	if name == "" {
		name = "Imported playlist"
	}
	dst := models.Playlist{
		ID: uuid.NewString(), Name: name, OwnerID: im.UserID,
		Comment: "Imported from " + im.Source, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.playlists.Create(ctx, dst); err != nil {
		return err
	}

	im.PlaylistID = dst.ID
	im.SourcePlaylistName = pl.Name
	im.Total = len(pl.Tracks)
	if err := s.imports.Update(ctx, *im); err != nil {
		return err
	}

	// Seed all items as pending so the UI shows the full list immediately.
	items := make([]models.ImportItem, len(pl.Tracks))
	for i, t := range pl.Tracks {
		items[i] = models.ImportItem{
			ID: uuid.NewString(), ImportID: im.ID, Position: i,
			SourceTitle: t.Title, SourceArtist: t.Artist, SourceAlbum: t.Album,
			Status: models.ImportItemPending, CreatedAt: now, UpdatedAt: now,
		}
	}
	if err := s.imports.InsertItems(ctx, items); err != nil {
		return err
	}

	for i := range items {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s.resolveItem(ctx, im, &items[i])
		if err := s.imports.UpdateItem(ctx, items[i]); err != nil {
			s.logger.Warn("import item update failed", "item", items[i].ID, "error", err)
		}
		if err := s.imports.Update(ctx, *im); err != nil {
			s.logger.Warn("import progress update failed", "import", im.ID, "error", err)
		}
	}
	return nil
}

// resolveItem searches the content providers for one source track, decides the
// outcome (matched/doubtful/missing/failed) and, when confident, downloads it and
// appends it to the playlist. It mutates the item and the import's counters.
func (s *Service) resolveItem(ctx context.Context, im *models.Import, it *models.ImportItem) {
	query := strings.TrimSpace(it.SourceArtist + " " + it.SourceTitle)
	cands, err := s.resolver.SearchTracks(ctx, query, 5)
	if err != nil {
		it.Status = models.ImportItemFailed
		it.Note = "search failed: " + err.Error()
		im.Failed++
		return
	}
	if len(cands) == 0 {
		it.Status = models.ImportItemMissing
		im.Missing++
		return
	}

	best, score := bestMatch(it.SourceArtist, it.SourceTitle, cands)
	it.Confidence = score
	it.ResolvedTitle = best.Title
	it.ResolvedArtist = best.ArtistName
	// Keep the candidate's ids so a doubtful item can be previewed (play/cover)
	// and validated without re-searching.
	it.CandidateID = best.ID
	it.CandidateCoverArt = best.CoverArt

	if score < MatchThreshold {
		it.Status = models.ImportItemDoubtful
		it.Note = fmt.Sprintf("best candidate %.0f%% below %.0f%% threshold", score*100, MatchThreshold*100)
		im.Doubtful++
		return
	}

	localID, err := s.resolver.Resolve(ctx, im.UserID, best.ID)
	if err != nil {
		it.Status = models.ImportItemFailed
		it.Note = "download failed: " + err.Error()
		im.Failed++
		return
	}
	if err := s.playlists.AppendTracks(ctx, im.PlaylistID, []string{localID}, im.UserID); err != nil {
		it.Status = models.ImportItemFailed
		it.Note = "add to playlist failed: " + err.Error()
		im.Failed++
		return
	}
	it.MatchedTrackID = localID
	it.Status = models.ImportItemMatched
	im.Matched++
}

// bestMatch returns the candidate most similar to the source (artist, title) and
// its 0..1 score.
func bestMatch(srcArtist, srcTitle string, cands []models.Track) (models.Track, float64) {
	var best models.Track
	bestScore := -1.0
	for _, c := range cands {
		score := trackSimilarity(srcArtist, srcTitle, c.ArtistName, c.Title)
		if score > bestScore {
			bestScore, best = score, c
		}
	}
	if bestScore < 0 {
		bestScore = 0
	}
	return best, bestScore
}
