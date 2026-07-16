package core

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/immerle/immerle/internal/testutil"
)

func newDeviceAuth(t *testing.T) (*AuthService, string) {
	store := testutil.NewStore(t)
	auth, _ := NewAuthService(store.Users, store.APITokens, store.Devices, "secret")
	if _, err := auth.CreateUser(context.Background(), "alice", "pw", "", "", false); err != nil {
		t.Fatal(err)
	}
	return auth, "alice"
}

func TestDeviceJWTLoginAuthRevoke(t *testing.T) {
	auth, username := newDeviceAuth(t)
	ctx := context.Background()

	token, dev, err := auth.IssueDeviceToken(ctx, Credentials{Username: username, Password: "pw", UserAgent: "UA/1", RemoteIP: "1.2.3.4"}, "Pixel 8", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !looksLikeJWT(token) {
		t.Fatalf("expected a JWT, got %q", token)
	}
	if dev.ID == "" || dev.Name != "Pixel 8" {
		t.Fatalf("unexpected device: %+v", dev)
	}

	u, err := auth.Authenticate(ctx, Credentials{APIToken: token, RemoteIP: "5.6.7.8"})
	if err != nil || u.Username != "alice" {
		t.Fatalf("jwt auth failed: %v", err)
	}

	// It shows up as a device with last-seen IP updated.
	devices, _ := auth.ListDevices(ctx, u.ID)
	if len(devices) != 1 || devices[0].LastIP != "5.6.7.8" {
		t.Fatalf("device tracking wrong: %+v", devices)
	}

	// Revoke → the JWT stops authenticating (revocation registry).
	ok, err := auth.RevokeDevice(ctx, dev.ID, u.ID)
	if err != nil || !ok {
		t.Fatalf("revoke failed: %v", err)
	}
	if _, err := auth.Authenticate(ctx, Credentials{APIToken: token}); err == nil {
		t.Fatal("revoked device JWT must be rejected")
	}
	if d, _ := auth.ListDevices(ctx, u.ID); len(d) != 0 {
		t.Fatalf("revoked device should not be listed, got %d", len(d))
	}
}

func TestDeviceJWTUniqueJTIPerLogin(t *testing.T) {
	auth, username := newDeviceAuth(t)
	ctx := context.Background()
	t1, d1, _ := auth.IssueDeviceToken(ctx, Credentials{Username: username, Password: "pw"}, "phone", time.Hour)
	t2, d2, _ := auth.IssueDeviceToken(ctx, Credentials{Username: username, Password: "pw"}, "phone", time.Hour)
	if d1.ID == d2.ID || t1 == t2 {
		t.Fatal("each login must mint a unique device id (jti)")
	}
	// Revoking one device does not affect the other.
	u, _ := auth.Authenticate(ctx, Credentials{APIToken: t1})
	_, _ = auth.RevokeDevice(ctx, d1.ID, u.ID)
	if _, err := auth.Authenticate(ctx, Credentials{APIToken: t1}); err == nil {
		t.Fatal("device 1 should be revoked")
	}
	if _, err := auth.Authenticate(ctx, Credentials{APIToken: t2}); err != nil {
		t.Fatal("device 2 should still authenticate")
	}
}

func TestDeviceJWTBadSignatureAndExpiry(t *testing.T) {
	auth, username := newDeviceAuth(t)
	ctx := context.Background()

	token, _, _ := auth.IssueDeviceToken(ctx, Credentials{Username: username, Password: "pw"}, "x", time.Hour)
	tampered := token[:len(token)-2] + "xy"
	if _, err := auth.Authenticate(ctx, Credentials{APIToken: tampered}); err == nil {
		t.Fatal("tampered JWT must be rejected")
	}

	// ttl<=0 means "never expires", so craft an already-expired exp directly.
	expired, err := signJWT(auth.jwtKey, jwtClaims{Sub: "x", JTI: "y", IAT: time.Now().Add(-2 * time.Hour).Unix(), EXP: time.Now().Add(-time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(expired, ".") {
		t.Fatal("expected a jwt")
	}
	if _, err := auth.Authenticate(ctx, Credentials{APIToken: expired}); err == nil {
		t.Fatal("expired JWT must be rejected")
	}
}

func TestDeviceJWTWrongPasswordNoToken(t *testing.T) {
	auth, username := newDeviceAuth(t)
	if _, _, err := auth.IssueDeviceToken(context.Background(), Credentials{Username: username, Password: "wrong"}, "x", time.Hour); err == nil {
		t.Fatal("login with wrong password must fail")
	}
}
