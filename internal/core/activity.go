package core

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/gossignol/gossignol/internal/models"
	"github.com/gossignol/gossignol/internal/persistence"
)

// ActivityService records and queries the social activity feed, honoring each
// user's default privacy setting.
type ActivityService struct {
	activity *persistence.ActivityRepo
	friends  *persistence.FriendRepo
	users    *persistence.UserRepo
}

// NewActivityService builds an ActivityService.
func NewActivityService(activity *persistence.ActivityRepo, friends *persistence.FriendRepo, users *persistence.UserRepo) *ActivityService {
	return &ActivityService{activity: activity, friends: friends, users: users}
}

// Record stores an activity event. The privacy defaults to the user's configured
// activity privacy when empty. A "private" setting suppresses the event entirely.
func (s *ActivityService) Record(ctx context.Context, user models.User, eventType string, itemType models.ItemType, itemID string) error {
	privacy := user.ActivityPrivacy
	if privacy == "" {
		privacy = "friends"
	}
	if privacy == "private" {
		return nil
	}
	return s.activity.Insert(ctx, models.ActivityEvent{
		ID:        uuid.NewString(),
		UserID:    user.ID,
		Type:      eventType,
		ItemType:  itemType,
		ItemID:    itemID,
		Privacy:   privacy,
		CreatedAt: time.Now(),
	})
}

// Feed returns activity events visible to the viewer.
func (s *ActivityService) Feed(ctx context.Context, viewerID string, limit int) ([]models.ActivityEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	friends, err := s.friends.ListFriends(ctx, viewerID)
	if err != nil {
		return nil, err
	}
	return s.activity.Feed(ctx, viewerID, friends, limit)
}

// UserFeed returns a single author's activity as visible to viewerID. The viewer
// sees "public" events from anyone; the author themselves and their accepted
// friends additionally see "friends" events. "private" events are never stored.
func (s *ActivityService) UserFeed(ctx context.Context, viewerID, authorID string, limit int) ([]models.ActivityEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	privacies := []string{"public"}
	if viewerID == authorID {
		privacies = append(privacies, "friends")
	} else if ok, err := s.friends.AreFriends(ctx, viewerID, authorID); err != nil {
		return nil, err
	} else if ok {
		privacies = append(privacies, "friends")
	}
	return s.activity.ByAuthor(ctx, authorID, privacies, limit)
}
