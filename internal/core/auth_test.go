package core

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/gossignol/gossignol/internal/testutil"
)

func TestPasswordEncryptionRoundTrip(t *testing.T) {
	box, err := newSecretBox("a-secret")
	if err != nil {
		t.Fatal(err)
	}
	enc, err := box.Encrypt("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if enc == "hunter2" {
		t.Fatal("password stored in plaintext")
	}
	dec, err := box.Decrypt(enc)
	if err != nil {
		t.Fatal(err)
	}
	if dec != "hunter2" {
		t.Fatalf("roundtrip mismatch: %q", dec)
	}
}

func TestAuthenticateTokenAndPassword(t *testing.T) {
	store := testutil.NewStore(t)
	auth, err := NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	alice, err := auth.CreateUser(ctx, "alice", "s3cret", "", "Alice Wonder", false)
	if err != nil {
		t.Fatal(err)
	}
	if alice.DisplayName != "Alice Wonder" {
		t.Fatalf("display name not stored, got %q", alice.DisplayName)
	}

	// Password auth.
	if _, err := auth.Authenticate(ctx, Credentials{Username: "alice", Password: "s3cret"}); err != nil {
		t.Fatalf("password auth failed: %v", err)
	}

	// Token auth: t = md5(password + salt).
	salt := "abc123"
	sum := md5.Sum([]byte("s3cret" + salt))
	token := hex.EncodeToString(sum[:])
	if _, err := auth.Authenticate(ctx, Credentials{Username: "alice", Token: token, Salt: salt}); err != nil {
		t.Fatalf("token auth failed: %v", err)
	}

	// Wrong password rejected.
	if _, err := auth.Authenticate(ctx, Credentials{Username: "alice", Password: "wrong"}); err == nil {
		t.Fatal("expected wrong-password rejection")
	}
}

func TestCreateFirstAdminIsOneShot(t *testing.T) {
	store := testutil.NewStore(t)
	auth, _ := NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	ctx := context.Background()

	u, err := auth.CreateFirstAdmin(ctx, "admin", "password123", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !u.IsAdmin {
		t.Fatal("first account must be admin")
	}

	// Second attempt is rejected: the admin can only be bootstrapped once.
	if _, err := auth.CreateFirstAdmin(ctx, "other", "password123", "", ""); !errors.Is(err, ErrAlreadyInitialized) {
		t.Fatalf("expected ErrAlreadyInitialized, got %v", err)
	}
	n, _ := store.Users.Count(ctx)
	if n != 1 {
		t.Fatalf("expected 1 user, got %d", n)
	}
}
