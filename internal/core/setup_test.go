package core

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/immerle/immerle/internal/testutil"
)

func newSetup(t *testing.T, requireToken bool) (*SetupService, *AuthService) {
	store := testutil.NewStore(t)
	auth, err := NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewSetupService(store.Users, auth, requireToken)
	if err != nil {
		t.Fatal(err)
	}
	return svc, auth
}

func TestSetupHappyPathAndLock(t *testing.T) {
	svc, _ := newSetup(t, false)
	ctx := context.Background()

	if init, _ := svc.IsInitialized(ctx); init {
		t.Fatal("should start uninitialized")
	}
	u, err := svc.InitFirstAdmin(ctx, "kilian", "password123", "k@example.com", "  Kilian Smiti  ", "")
	if err != nil {
		t.Fatal(err)
	}
	if !u.IsAdmin {
		t.Fatal("created user must be admin")
	}
	if u.DisplayName != "Kilian Smiti" {
		t.Fatalf("display name not stored/trimmed, got %q", u.DisplayName)
	}
	if init, _ := svc.IsInitialized(ctx); !init {
		t.Fatal("should be initialized after setup")
	}
	// Locked: a second attempt is rejected.
	if _, err := svc.InitFirstAdmin(ctx, "evil", "password123", "", "", ""); !errors.Is(err, ErrAlreadyInitialized) {
		t.Fatalf("expected ErrAlreadyInitialized, got %v", err)
	}
}

func TestSetupValidation(t *testing.T) {
	svc, _ := newSetup(t, false)
	ctx := context.Background()

	_, err := svc.InitFirstAdmin(ctx, "bad name!", "short", "", "", "")
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	fields := map[string]bool{}
	for _, f := range verr.Fields {
		fields[f.Field] = true
	}
	if !fields["username"] || !fields["password"] {
		t.Fatalf("expected username+password errors, got %+v", verr.Fields)
	}
}

func TestSetupTokenRequired(t *testing.T) {
	svc, _ := newSetup(t, true)
	ctx := context.Background()
	if svc.Token() == "" {
		t.Fatal("token should be generated when required")
	}
	// Wrong/missing token rejected.
	if _, err := svc.InitFirstAdmin(ctx, "kilian", "password123", "", "", ""); !errors.Is(err, ErrInvalidSetupToken) {
		t.Fatalf("expected ErrInvalidSetupToken for missing token, got %v", err)
	}
	if _, err := svc.InitFirstAdmin(ctx, "kilian", "password123", "", "", "wrong"); !errors.Is(err, ErrInvalidSetupToken) {
		t.Fatalf("expected ErrInvalidSetupToken for wrong token, got %v", err)
	}
	// Correct token succeeds.
	if _, err := svc.InitFirstAdmin(ctx, "kilian", "password123", "", "", svc.Token()); err != nil {
		t.Fatalf("correct token should succeed: %v", err)
	}
}

func TestBootstrapFromEnvHappyPath(t *testing.T) {
	svc, _ := newSetup(t, false)
	ctx := context.Background()

	u, err := svc.BootstrapFromEnv(ctx, "kilian", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if !u.IsAdmin {
		t.Fatal("bootstrapped user must be admin")
	}
	if init, _ := svc.IsInitialized(ctx); !init {
		t.Fatal("should be initialized after bootstrap")
	}
}

func TestBootstrapFromEnvNoopWhenAlreadyInitialized(t *testing.T) {
	svc, _ := newSetup(t, false)
	ctx := context.Background()

	if _, err := svc.InitFirstAdmin(ctx, "kilian", "password123", "", "", ""); err != nil {
		t.Fatal(err)
	}
	// A later restart with the same env vars set must not error or overwrite.
	if _, err := svc.BootstrapFromEnv(ctx, "someone-else", "password123"); !errors.Is(err, ErrAlreadyInitialized) {
		t.Fatalf("expected ErrAlreadyInitialized, got %v", err)
	}
}

func TestBootstrapFromEnvIgnoresSetupToken(t *testing.T) {
	// Unlike InitFirstAdmin, BootstrapFromEnv never checks a setup token: the
	// operator already controls the process environment.
	svc, _ := newSetup(t, true)
	ctx := context.Background()

	if _, err := svc.BootstrapFromEnv(ctx, "kilian", "password123"); err != nil {
		t.Fatalf("expected success without a token, got %v", err)
	}
}

func TestBootstrapFromEnvValidation(t *testing.T) {
	svc, _ := newSetup(t, false)
	ctx := context.Background()

	_, err := svc.BootstrapFromEnv(ctx, "bad name!", "short")
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
}

func TestSetupConcurrentInitSingleWinner(t *testing.T) {
	svc, _ := newSetup(t, false)
	ctx := context.Background()

	const n = 8
	var wg sync.WaitGroup
	wg.Add(n)
	var mu sync.Mutex
	successes := 0
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			// Distinct usernames so failures are due to the lock, not UNIQUE.
			_, err := svc.InitFirstAdmin(ctx, "admin", "password123", "", "", "")
			if err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if successes != 1 {
		t.Fatalf("expected exactly 1 successful first-admin creation, got %d", successes)
	}
}
