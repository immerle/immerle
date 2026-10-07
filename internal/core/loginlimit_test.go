package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/immerle/immerle/internal/testutil"
)

func TestAuthenticateThrottlesBruteForce(t *testing.T) {
	store := testutil.NewStore(t)
	auth, err := NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	auth.limiter.now = func() time.Time { return now }
	ctx := context.Background()
	if _, err := auth.CreateUser(ctx, "alice", "s3cret", "", "", false); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.CreateUser(ctx, "bob", "hunter2", "", "", false); err != nil {
		t.Fatal(err)
	}

	for range loginMaxFailures {
		if _, err := auth.Authenticate(ctx, Credentials{Username: "alice", Password: "wrong", RemoteIP: "1.2.3.4"}); errors.Is(err, ErrTooManyAttempts) {
			t.Fatal("throttled before reaching the failure budget")
		}
	}

	// The account is locked even with the right password, whatever the IP (and
	// case), and the attacking IP is locked for other accounts too.
	if _, err := auth.Authenticate(ctx, Credentials{Username: "ALICE", Password: "s3cret", RemoteIP: "9.9.9.9"}); !errors.Is(err, ErrTooManyAttempts) {
		t.Fatalf("want ErrTooManyAttempts for the account, got %v", err)
	}
	if _, err := auth.Authenticate(ctx, Credentials{Username: "bob", Password: "hunter2", RemoteIP: "1.2.3.4"}); !errors.Is(err, ErrTooManyAttempts) {
		t.Fatalf("want ErrTooManyAttempts for the IP, got %v", err)
	}
	if !errors.Is(ErrTooManyAttempts, ErrUnauthorized) {
		t.Fatal("ErrTooManyAttempts must still read as unauthorized")
	}
	// Other accounts from other IPs are unaffected.
	if _, err := auth.Authenticate(ctx, Credentials{Username: "bob", Password: "hunter2", RemoteIP: "5.6.7.8"}); err != nil {
		t.Fatalf("unrelated login rejected: %v", err)
	}

	now = now.Add(loginWindow)
	if _, err := auth.Authenticate(ctx, Credentials{Username: "alice", Password: "s3cret", RemoteIP: "1.2.3.4"}); err != nil {
		t.Fatalf("still locked after the window: %v", err)
	}
}
