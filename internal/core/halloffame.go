package core

import (
	"context"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// HallOfFameService holds the Hall of Fame business logic: a user's personal
// top-tracks ranking, auto-created on first access. Always scoped to the
// caller — every method takes the user id, never a foreign id, since there is
// exactly one Hall of Fame per user.
type HallOfFameService struct {
	repo     *persistence.HallOfFameRepo
	onDemand *CatalogService // optional; resolves remote track ids before they hit hall_of_fame_entries
}

// NewHallOfFameService wires the Hall of Fame application service. onDemand is
// optional (pass nil when unused).
func NewHallOfFameService(repo *persistence.HallOfFameRepo, onDemand *CatalogService) *HallOfFameService {
	return &HallOfFameService{repo: repo, onDemand: onDemand}
}

// HallOfFameDetail is a Hall of Fame with its ranked entries.
type HallOfFameDetail struct {
	HallOfFame models.HallOfFame
	Entries    []models.HallOfFameEntry
}

// Get returns the caller's Hall of Fame, auto-creating an empty one on first access.
func (s *HallOfFameService) Get(ctx context.Context, userID string) (HallOfFameDetail, error) {
	h, err := s.repo.GetOrCreate(ctx, userID)
	if err != nil {
		return HallOfFameDetail{}, err
	}
	entries, err := s.repo.Entries(ctx, h.ID)
	if err != nil {
		return HallOfFameDetail{}, err
	}
	return HallOfFameDetail{HallOfFame: h, Entries: entries}, nil
}

// SetOrder replaces the caller's full ranked track list (reorder/add/remove
// in one call — the caller computes the desired final order).
func (s *HallOfFameService) SetOrder(ctx context.Context, userID string, trackIDs []string) error {
	h, err := s.repo.GetOrCreate(ctx, userID)
	if err != nil {
		return err
	}
	resolved, err := resolveTrackIDs(ctx, s.onDemand, userID, trackIDs)
	if err != nil {
		return err
	}
	return s.repo.ReplaceEntries(ctx, h.ID, resolved)
}

// Add appends a track to the caller's Hall of Fame (a no-op if it's already
// there). trackID may be a remote (not-yet-downloaded) provider id — it is
// resolved to a real local track before hitting hall_of_fame_entries, which
// has a foreign key on track_id.
func (s *HallOfFameService) Add(ctx context.Context, userID, trackID string) error {
	h, err := s.repo.GetOrCreate(ctx, userID)
	if err != nil {
		return err
	}
	resolved, err := resolveTrackIDs(ctx, s.onDemand, userID, []string{trackID})
	if err != nil {
		return err
	}
	trackID = resolved[0]
	entries, err := s.repo.Entries(ctx, h.ID)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(entries)+1)
	for _, e := range entries {
		if e.Track.ID == trackID {
			return nil
		}
		ids = append(ids, e.Track.ID)
	}
	ids = append(ids, trackID)
	return s.repo.ReplaceEntries(ctx, h.ID, ids)
}

// SetNote sets (or, given an empty comment, clears) a personal nostalgia note
// on one of the caller's ranked tracks.
func (s *HallOfFameService) SetNote(ctx context.Context, userID, trackID, comment string) error {
	h, err := s.repo.GetOrCreate(ctx, userID)
	if err != nil {
		return err
	}
	return s.repo.SetNote(ctx, h.ID, trackID, comment)
}
