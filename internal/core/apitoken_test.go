package core

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gossignol/gossignol/internal/testutil"
)

func TestAPITokenCreateAuthenticateRevoke(t *testing.T) {
	store := testutil.NewStore(t)
	auth, _ := NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	ctx := context.Background()

	user, err := auth.CreateUser(ctx, "alice", "pw", "", "", false)
	if err != nil {
		t.Fatal(err)
	}

	secret, tok, err := auth.CreateAPIToken(ctx, user.ID, "cli", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(secret, "gsk_") {
		t.Fatalf("token should be prefixed gsk_, got %q", secret)
	}

	// The secret is not stored in plaintext.
	if tok.TokenHash == secret || tok.TokenHash == "" {
		t.Fatal("only a hash of the token must be stored")
	}

	// Authenticate with the token → resolves to the owning user, no username needed.
	got, err := auth.Authenticate(ctx, Credentials{APIToken: secret})
	if err != nil || got.ID != user.ID {
		t.Fatalf("token auth failed: user=%s err=%v", got.ID, err)
	}

	// Wrong token rejected.
	if _, err := auth.Authenticate(ctx, Credentials{APIToken: "gsk_bogus"}); err == nil {
		t.Fatal("bogus token must be rejected")
	}

	// Revoke → token no longer authenticates.
	ok, err := auth.RevokeAPIToken(ctx, tok.ID, user.ID)
	if err != nil || !ok {
		t.Fatalf("revoke failed: ok=%v err=%v", ok, err)
	}
	if _, err := auth.Authenticate(ctx, Credentials{APIToken: secret}); err == nil {
		t.Fatal("revoked token must be rejected")
	}
}

func TestAPITokenScopedToCreator(t *testing.T) {
	store := testutil.NewStore(t)
	auth, _ := NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	ctx := context.Background()

	alice, _ := auth.CreateUser(ctx, "alice", "pw", "", "", false)
	bob, _ := auth.CreateUser(ctx, "bob", "pw", "", "", false)

	secret, tok, _ := auth.CreateAPIToken(ctx, alice.ID, "a", nil)

	// Token authenticates as Alice (its creator), never Bob.
	got, _ := auth.Authenticate(ctx, Credentials{APIToken: secret})
	if got.ID != alice.ID {
		t.Fatalf("token must act as its creator (alice), got %s", got.Username)
	}

	// Bob cannot revoke Alice's token; Alice's list shows it.
	if ok, _ := auth.RevokeAPIToken(ctx, tok.ID, bob.ID); ok {
		t.Fatal("a user must not revoke another user's token")
	}
	list, _ := auth.ListAPITokens(ctx, alice.ID)
	if len(list) != 1 {
		t.Fatalf("alice should have 1 token, got %d", len(list))
	}
	if l, _ := auth.ListAPITokens(ctx, bob.ID); len(l) != 0 {
		t.Fatalf("bob should have 0 tokens, got %d", len(l))
	}
}

func TestAPITokenExpiry(t *testing.T) {
	store := testutil.NewStore(t)
	auth, _ := NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	ctx := context.Background()
	user, _ := auth.CreateUser(ctx, "alice", "pw", "", "", false)

	past := time.Now().Add(-time.Hour)
	secret, _, _ := auth.CreateAPIToken(ctx, user.ID, "expired", &past)
	if _, err := auth.Authenticate(ctx, Credentials{APIToken: secret}); err == nil {
		t.Fatal("expired token must be rejected")
	}
}
