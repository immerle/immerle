package core

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// ActivityService records and queries the social activity feed, honoring each
// user's default privacy setting.
type ActivityService struct {
	activity *persistence.ActivityRepo
}

// NewActivityService builds an ActivityService.
func NewActivityService(activity *persistence.ActivityRepo) *ActivityService {
	return &ActivityService{activity: activity}
}

// Record stores an activity event. A "private" setting suppresses the event
// entirely; everything else is recorded as public.
func (s *ActivityService) Record(ctx context.Context, user models.User, eventType string, itemType models.ItemType, itemID string) error {
	if user.ActivityPrivacy == "private" {
		return nil
	}
	return s.activity.Insert(ctx, models.ActivityEvent{
		ID:        uuid.NewString(),
		UserID:    user.ID,
		Type:      eventType,
		ItemType:  itemType,
		ItemID:    itemID,
		Privacy:   "public",
		CreatedAt: time.Now(),
	})
}

// Feed returns activity events visible to the viewer.
func (s *ActivityService) Feed(ctx context.Context, viewerID string, limit int) ([]models.ActivityEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.activity.Feed(ctx, viewerID, limit)
}

// UserFeed returns an author's public activity.
func (s *ActivityService) UserFeed(ctx context.Context, authorID string, limit int) ([]models.ActivityEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.activity.ByAuthor(ctx, authorID, limit)
}
