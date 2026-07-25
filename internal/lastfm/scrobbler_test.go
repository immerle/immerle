package lastfm

import (
	"context"
	"testing"
	"time"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/outbox"
	"github.com/immerle/immerle/internal/testutil"
)

func TestEnqueueScrobbleGating(t *testing.T) {
	store := testutil.NewStore(t)
	worker := outbox.NewWorker(store.Outbox, testutil.NewLogger())
	credentials := func() models.LastFmRuntime { return models.LastFmRuntime{Enabled: true, APIKey: "k", APISecret: "s"} }
	s := NewScrobbler(NewClient(nil), credentials, worker, testutil.NewLogger())
	ctx := context.Background()

	track := models.Track{ID: "t1", Title: "Title", ArtistName: "Artist"}
	at := time.Now()

	cases := []struct {
		name string
		user models.User
		want bool // expect a job enqueued
	}{
		{"no session key", models.User{ID: "u1", ScrobbleEnabled: true}, false},
		{"scrobbling disabled", models.User{ID: "u1", ScrobbleEnabled: false, LastFmSessionKey: "sk"}, false},
		{"session key set and enabled", models.User{ID: "u1", ScrobbleEnabled: true, LastFmSessionKey: "sk"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s.EnqueueScrobble(ctx, c.user, track, at)
			job, err := store.Outbox.ClaimNext(ctx, time.Now().Add(time.Minute))
			got := err == nil
			if got != c.want {
				t.Fatalf("job enqueued = %v, want %v", got, c.want)
			}
			if got {
				if job.Kind != ScrobbleKind {
					t.Errorf("job kind = %q, want %q", job.Kind, ScrobbleKind)
				}
				_ = store.Outbox.Done(ctx, job.ID, job.ClaimToken)
			}
		})
	}
}
