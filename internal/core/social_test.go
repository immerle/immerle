package core

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gossignol/gossignol/internal/models"
	"github.com/gossignol/gossignol/internal/testutil"
)

func TestActivityFeedRespectsPrivacy(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	users := map[string]models.User{}
	for _, spec := range []struct{ name, privacy string }{
		{"viewer", "friends"},
		{"friend", "friends"},
		{"public_user", "public"},
		{"private_user", "private"},
		{"stranger", "friends"},
	} {
		u := models.User{ID: uuid.NewString(), Username: spec.name, PasswordHash: "x", ActivityPrivacy: spec.privacy, CreatedAt: now}
		if err := store.Users.Create(ctx, u); err != nil {
			t.Fatal(err)
		}
		users[spec.name] = u
	}

	// viewer <-> friend are accepted friends.
	_ = store.Friends.Request(ctx, models.Friendship{ID: uuid.NewString(), UserID: users["viewer"].ID, FriendID: users["friend"].ID, Status: models.FriendPending, CreatedAt: now, UpdatedAt: now})
	_ = store.Friends.Accept(ctx, users["viewer"].ID, users["friend"].ID, uuid.NewString())

	svc := NewActivityService(store.Activity, store.Friends, store.Users)

	// Each user records a "listen" event.
	for _, name := range []string{"friend", "public_user", "private_user", "stranger"} {
		if err := svc.Record(ctx, users[name], "listen", models.ItemTrack, "track-"+name); err != nil {
			t.Fatal(err)
		}
	}

	events, err := svc.Feed(ctx, users["viewer"].ID, 50)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range events {
		seen[e.Username] = true
	}

	// Friend's friends-only event: visible.
	if !seen["friend"] {
		t.Error("friend's event should be visible to viewer")
	}
	// Public user's event: visible to everyone.
	if !seen["public_user"] {
		t.Error("public event should be visible")
	}
	// Private user's event: never recorded, never visible.
	if seen["private_user"] {
		t.Error("private user's event must not be visible")
	}
	// Stranger's friends-only event: not a friend, so hidden.
	if seen["stranger"] {
		t.Error("stranger's friends-only event must not be visible")
	}
}

func TestJamKeepsClientsSynchronized(t *testing.T) {
	store := testutil.NewStore(t)
	ctx := context.Background()
	now := time.Now()

	host := models.User{ID: uuid.NewString(), Username: "host", PasswordHash: "x", CreatedAt: now}
	guest := models.User{ID: uuid.NewString(), Username: "guest", PasswordHash: "x", CreatedAt: now}
	_ = store.Users.Create(ctx, host)
	_ = store.Users.Create(ctx, guest)

	svc := NewJamService(store.Jam)
	session, err := svc.Create(ctx, host.ID, "Friday", []string{"t1", "t2"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Join(ctx, session.ID, guest.ID); err != nil {
		t.Fatal(err)
	}

	// Guest subscribes (as an SSE client would).
	ch, unsubscribe := svc.Subscribe(session.ID)
	defer unsubscribe()

	// Host advances playback.
	if err := svc.UpdatePlayback(ctx, session.ID, "t2", 65000, "playing", []string{"t1", "t2"}); err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-ch:
		if ev.Session.CurrentTrackID != "t2" || ev.Session.PositionMs != 65000 || ev.Session.State != "playing" {
			t.Fatalf("guest not synchronized: %+v", ev.Session)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("guest did not receive playback update")
	}

	// Both clients see the same persisted state and 2 participants.
	state, _ := svc.Get(ctx, session.ID)
	if state.CurrentTrackID != "t2" || state.PositionMs != 65000 {
		t.Fatalf("persisted state wrong: %+v", state)
	}
	participants, _ := svc.Participants(ctx, session.ID)
	if len(participants) != 2 {
		t.Fatalf("expected 2 participants, got %d", len(participants))
	}
}
