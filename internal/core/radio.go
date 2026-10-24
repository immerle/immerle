package core

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// ErrInvalidStation is returned when a station lacks a name or a http(s) stream.
var ErrInvalidStation = errors.New("name and a http(s) streamUrl are required")

// ErrBuiltinStation is returned when editing or deleting a built-in station:
// built-ins are server-managed (refreshed from the embedded list on boot).
var ErrBuiltinStation = errors.New("built-in stations are read-only")

// RadioService holds the custom-station rules shared by the Subsonic and native
// APIs. Listing and likes have no rule and go straight to the repository.
type RadioService struct {
	repo *persistence.RadioRepo
}

// NewRadioService wires the radio station service.
func NewRadioService(repo *persistence.RadioRepo) *RadioService {
	return &RadioService{repo: repo}
}

// Create adds a custom station from st's name, stream, homepage and cover.
func (s *RadioService) Create(ctx context.Context, st models.RadioStation) (models.RadioStation, error) {
	now := time.Now()
	st = models.RadioStation{
		ID: persistence.NewStationID(), Name: st.Name, StreamURL: st.StreamURL,
		HomepageURL: st.HomepageURL, CoverArt: st.CoverArt, CreatedAt: now, UpdatedAt: now,
	}
	if err := validStation(&st); err != nil {
		return models.RadioStation{}, err
	}
	return st, s.repo.Create(ctx, st)
}

// Update applies edit to a custom station and saves it. It returns the station
// before and after the edit (callers use it to drop a stale cached logo).
func (s *RadioService) Update(ctx context.Context, id string, edit func(*models.RadioStation)) (before, after models.RadioStation, err error) {
	before, err = s.repo.Get(ctx, id)
	if err != nil {
		return before, after, err
	}
	if before.Builtin {
		return before, after, ErrBuiltinStation
	}
	after = before
	edit(&after)
	if err := validStation(&after); err != nil {
		return before, models.RadioStation{}, err
	}
	after.UpdatedAt = time.Now()
	return before, after, s.repo.Update(ctx, after)
}

// Delete removes a custom station.
func (s *RadioService) Delete(ctx context.Context, id string) error {
	st, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if st.Builtin {
		return ErrBuiltinStation
	}
	return s.repo.Delete(ctx, id)
}

// validStation trims st's fields and checks it has a name and a http(s) stream.
func validStation(st *models.RadioStation) error {
	st.Name = strings.TrimSpace(st.Name)
	st.StreamURL = strings.TrimSpace(st.StreamURL)
	st.HomepageURL = strings.TrimSpace(st.HomepageURL)
	st.CoverArt = strings.TrimSpace(st.CoverArt)
	if st.Name == "" || (!strings.HasPrefix(st.StreamURL, "http://") && !strings.HasPrefix(st.StreamURL, "https://")) {
		return ErrInvalidStation
	}
	return nil
}
