package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// ShareService holds the share business logic: classifying the shared item,
// minting the share with a random secret, and resolving a share into its
// tracklist. Ownership is enforced by the repository (updates/deletes are scoped
// to the user id).
type ShareService struct {
	shares    *persistence.ShareRepo
	catalog   *persistence.CatalogRepo
	playlists *persistence.PlaylistRepo
}

// NewShareService wires the share application service.
func NewShareService(shares *persistence.ShareRepo, catalog *persistence.CatalogRepo, playlists *persistence.PlaylistRepo) *ShareService {
	return &ShareService{shares: shares, catalog: catalog, playlists: playlists}
}

// ShareWithEntries is a share resolved into the tracks it exposes.
type ShareWithEntries struct {
	Share   models.Share
	Entries []models.Track
}

// List returns the user's shares, each resolved into its tracks.
func (s *ShareService) List(ctx context.Context, userID string) ([]ShareWithEntries, error) {
	shares, err := s.shares.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]ShareWithEntries, 0, len(shares))
	for _, sh := range shares {
		out = append(out, ShareWithEntries{Share: sh, Entries: s.entries(ctx, sh)})
	}
	return out, nil
}

// Create mints a share for itemID (its type is classified), owned by the user.
func (s *ShareService) Create(ctx context.Context, userID, itemID, description string, expiresAt *time.Time) (ShareWithEntries, error) {
	share := models.Share{
		ID:          uuid.NewString(),
		UserID:      userID,
		ItemType:    s.classify(ctx, itemID),
		ItemID:      itemID,
		Secret:      randomShareSecret(),
		Description: description,
		CreatedAt:   time.Now(),
		ExpiresAt:   expiresAt,
	}
	if err := s.shares.Create(ctx, share); err != nil {
		return ShareWithEntries{}, err
	}
	return ShareWithEntries{Share: share, Entries: s.entries(ctx, share)}, nil
}

// Update changes a share's description and expiry (scoped to the owner).
func (s *ShareService) Update(ctx context.Context, id, userID, description string, expiresAt *time.Time) error {
	return s.shares.Update(ctx, id, userID, description, expiresAt)
}

// Delete removes a share (scoped to the owner).
func (s *ShareService) Delete(ctx context.Context, id, userID string) error {
	return s.shares.Delete(ctx, id, userID)
}

// entries resolves a share into the tracks it exposes (a single track, an
// album's tracks, or a playlist's tracks). Lookup failures yield no entries.
func (s *ShareService) entries(ctx context.Context, share models.Share) []models.Track {
	switch share.ItemType {
	case models.ItemTrack:
		if t, err := s.catalog.GetTrack(ctx, share.ItemID); err == nil {
			return []models.Track{t}
		}
	case models.ItemAlbum:
		if tracks, err := s.catalog.ListTracksByAlbum(ctx, share.ItemID); err == nil {
			return tracks
		}
	case models.ItemPlaylist:
		if tracks, err := s.playlists.Tracks(ctx, share.ItemID); err == nil {
			return tracks
		}
	}
	return nil
}

// classify determines whether an id is an album, playlist or (default) track.
func (s *ShareService) classify(ctx context.Context, id string) models.ItemType {
	if _, err := s.catalog.GetAlbum(ctx, id); err == nil {
		return models.ItemAlbum
	}
	if _, err := s.playlists.Get(ctx, id); err == nil {
		return models.ItemPlaylist
	}
	return models.ItemTrack
}

func randomShareSecret() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
