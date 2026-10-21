package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
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
// Create makes a public link to a track, album or playlist the user can see.
// Returns persistence.ErrNotFound for an unknown item and ErrForbidden for a
// playlist the user cannot view (a share would expose its tracks).
func (s *ShareService) Create(ctx context.Context, user models.User, itemID, description string, expiresAt *time.Time) (ShareWithEntries, error) {
	itemType, err := s.classify(ctx, user, itemID)
	if err != nil {
		return ShareWithEntries{}, err
	}
	share := models.Share{
		ID:          uuid.NewString(),
		UserID:      user.ID,
		ItemType:    itemType,
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
// classify resolves what id points to, checking the user may share it. Tracks
// and albums are the shared catalog, readable by every user.
func (s *ShareService) classify(ctx context.Context, user models.User, id string) (models.ItemType, error) {
	if _, err := s.catalog.GetAlbum(ctx, id); err == nil {
		return models.ItemAlbum, nil
	} else if !errors.Is(err, persistence.ErrNotFound) {
		return "", err
	}
	if p, err := s.playlists.Get(ctx, id); err == nil {
		if !canViewPlaylist(ctx, s.playlists, p, user) {
			return "", ErrForbidden
		}
		return models.ItemPlaylist, nil
	} else if !errors.Is(err, persistence.ErrNotFound) {
		return "", err
	}
	if _, err := s.catalog.GetTrack(ctx, id); err != nil {
		return "", err
	}
	return models.ItemTrack, nil
}

func randomShareSecret() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
