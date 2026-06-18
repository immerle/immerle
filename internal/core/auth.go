package core

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// ErrUnauthorized is returned when credentials are missing or invalid.
var ErrUnauthorized = errors.New("unauthorized")

// ErrForbidden is returned when an authenticated user lacks permission.
var ErrForbidden = errors.New("forbidden")

// AuthService authenticates users and manages accounts.
type AuthService struct {
	users   *persistence.UserRepo
	tokens  *persistence.APITokenRepo
	devices *persistence.DeviceRepo
	box     *secretBox
	jwtKey  []byte
	// dummyHash is a valid encrypted password used to equalize the timing of a
	// failed login when the username does not exist (so the decrypt+compare path
	// always runs), preventing account-enumeration via response timing.
	dummyHash string
}

// NewAuthService builds an AuthService. tokens/devices may be nil to disable the
// corresponding token type.
func NewAuthService(users *persistence.UserRepo, tokens *persistence.APITokenRepo, devices *persistence.DeviceRepo, secret string) (*AuthService, error) {
	box, err := newSecretBox(secret)
	if err != nil {
		return nil, err
	}
	// Derive a dedicated HMAC key for JWTs (separate from the password AES key).
	jwtKey := sha256.Sum256([]byte("jwt:" + secret))
	dummyHash, err := box.Encrypt("immerle-timing-equalizer")
	if err != nil {
		return nil, err
	}
	return &AuthService{users: users, tokens: tokens, devices: devices, box: box, jwtKey: jwtKey[:], dummyHash: dummyHash}, nil
}

// Credentials carry an authentication attempt.
type Credentials struct {
	Username string
	// Password is the plaintext (or "enc:"-hex-encoded) password, when provided.
	Password string
	// Token and Salt implement Subsonic token auth: Token = md5(password+salt).
	Token string
	Salt  string
	// APIToken is a personal access token or device JWT (Authorization: Bearer /
	// apiKey). When set it takes precedence and authenticates as its owner.
	APIToken string
	// RemoteIP and UserAgent are request metadata used for device tracking.
	RemoteIP  string
	UserAgent string
}

// CreateUser creates a new account, encrypting its password. displayName is a
// free-text UI name (optional; empty falls back to the username client-side).
func (a *AuthService) CreateUser(ctx context.Context, username, password, email, displayName string, admin bool) (models.User, error) {
	enc, err := a.box.Encrypt(password)
	if err != nil {
		return models.User{}, err
	}
	u := models.User{
		ID:              uuid.NewString(),
		Username:        username,
		PasswordHash:    enc,
		Email:           email,
		DisplayName:     NormalizeDisplayName(displayName),
		IsAdmin:         admin,
		ScrobbleEnabled: true,
		ActivityPrivacy: "friends",
		CreatedAt:       time.Now(),
	}
	if err := a.users.Create(ctx, u); err != nil {
		return models.User{}, err
	}
	return u, nil
}

// maxDisplayNameLen bounds a display name to keep responses and storage sane.
const maxDisplayNameLen = 128

// NormalizeDisplayName trims surrounding whitespace and truncates an over-long
// display name (by rune) so it is safe to store and render.
func NormalizeDisplayName(name string) string {
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) > maxDisplayNameLen {
		name = string([]rune(name)[:maxDisplayNameLen])
	}
	return name
}

// SetPassword changes a user's password.
func (a *AuthService) SetPassword(ctx context.Context, userID, password string) error {
	u, err := a.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	enc, err := a.box.Encrypt(password)
	if err != nil {
		return err
	}
	u.PasswordHash = enc
	return a.users.Update(ctx, u)
}

// Authenticate validates credentials and returns the user.
func (a *AuthService) Authenticate(ctx context.Context, c Credentials) (models.User, error) {
	// A token authenticates as its owner, regardless of username: a device JWT
	// (verified + checked against the revocation/device registry) or an opaque
	// personal API token.
	if c.APIToken != "" {
		if looksLikeJWT(c.APIToken) {
			return a.authenticateJWT(ctx, c.APIToken, c.RemoteIP)
		}
		return a.authenticateToken(ctx, c.APIToken)
	}
	if c.Username == "" {
		return models.User{}, ErrUnauthorized
	}
	u, err := a.users.GetByUsername(ctx, c.Username)
	notFound := errors.Is(err, persistence.ErrNotFound)
	if err != nil && !notFound {
		return models.User{}, err
	}
	// Always run the decrypt + constant-time compare, even when the username does
	// not exist (against a dummy hash), so a missing account is not detectable by
	// response timing. The notFound guard below makes success impossible.
	hash := u.PasswordHash
	if notFound {
		hash = a.dummyHash
	}
	stored, err := a.box.Decrypt(hash)
	if err != nil {
		return models.User{}, ErrUnauthorized
	}

	switch {
	case c.Token != "" && c.Salt != "":
		expected := md5.Sum([]byte(stored + c.Salt))
		if subtle.ConstantTimeCompare([]byte(hex.EncodeToString(expected[:])), []byte(strings.ToLower(c.Token))) == 1 && !notFound {
			return u, nil
		}
	case c.Password != "":
		pw := decodePassword(c.Password)
		if subtle.ConstantTimeCompare([]byte(pw), []byte(stored)) == 1 && !notFound {
			return u, nil
		}
	}
	return models.User{}, ErrUnauthorized
}

// decodePassword handles Subsonic's "enc:<hex>" password encoding.
func decodePassword(p string) string {
	if raw, ok := strings.CutPrefix(p, "enc:"); ok {
		if dec, err := hex.DecodeString(raw); err == nil {
			return string(dec)
		}
	}
	return p
}

// CreateFirstAdmin atomically creates the first administrator account, but only
// while the server has no users. It returns ErrAlreadyInitialized otherwise. The
// admin account can only ever be bootstrapped through this path (the setup API);
// there is no config/env provisioning.
func (a *AuthService) CreateFirstAdmin(ctx context.Context, username, password, email, displayName string) (models.User, error) {
	enc, err := a.box.Encrypt(password)
	if err != nil {
		return models.User{}, err
	}
	u := models.User{
		ID:              uuid.NewString(),
		Username:        username,
		PasswordHash:    enc,
		Email:           email,
		DisplayName:     NormalizeDisplayName(displayName),
		IsAdmin:         true,
		ScrobbleEnabled: true,
		ActivityPrivacy: "friends",
		CreatedAt:       time.Now(),
	}
	created, err := a.users.CreateIfEmpty(ctx, u)
	if err != nil {
		return models.User{}, err
	}
	if !created {
		return models.User{}, ErrAlreadyInitialized
	}
	return u, nil
}

// ErrAlreadyInitialized is returned when first-run setup is attempted on a server
// that already has at least one user.
var ErrAlreadyInitialized = errors.New("already initialized")

// ---- personal API tokens ----

const apiTokenPrefix = "gsk_"

// CreateAPIToken mints a personal access token for a user. The plaintext secret
// is returned once and never stored (only its SHA-256 hash is persisted).
func (a *AuthService) CreateAPIToken(ctx context.Context, userID, name string, expiresAt *time.Time) (string, models.APIToken, error) {
	if a.tokens == nil {
		return "", models.APIToken{}, errors.New("api tokens disabled")
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", models.APIToken{}, err
	}
	plaintext := apiTokenPrefix + base64.RawURLEncoding.EncodeToString(buf)
	tok := models.APIToken{
		ID:        uuid.NewString(),
		UserID:    userID,
		Name:      name,
		TokenHash: hashToken(plaintext),
		Prefix:    plaintext[:12], // e.g. "gsk_AbC12…" for display
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}
	if err := a.tokens.Create(ctx, tok); err != nil {
		return "", models.APIToken{}, err
	}
	return plaintext, tok, nil
}

// ListAPITokens returns a user's active tokens (no secrets).
func (a *AuthService) ListAPITokens(ctx context.Context, userID string) ([]models.APIToken, error) {
	if a.tokens == nil {
		return nil, nil
	}
	return a.tokens.ListByUser(ctx, userID)
}

// RevokeAPIToken revokes one of the user's tokens.
func (a *AuthService) RevokeAPIToken(ctx context.Context, id, userID string) (bool, error) {
	if a.tokens == nil {
		return false, nil
	}
	return a.tokens.Revoke(ctx, id, userID)
}

// authenticateToken resolves a personal access token to its owning user.
func (a *AuthService) authenticateToken(ctx context.Context, plaintext string) (models.User, error) {
	if a.tokens == nil {
		return models.User{}, ErrUnauthorized
	}
	tok, err := a.tokens.GetByHash(ctx, hashToken(plaintext))
	if err != nil {
		return models.User{}, ErrUnauthorized
	}
	if tok.ExpiresAt != nil && tok.ExpiresAt.Before(time.Now()) {
		return models.User{}, ErrUnauthorized
	}
	user, err := a.users.GetByID(ctx, tok.UserID)
	if err != nil {
		return models.User{}, ErrUnauthorized
	}
	_ = a.tokens.TouchLastUsed(ctx, tok.ID, time.Now())
	return user, nil
}

func hashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// ---- device sessions (JWT) ----

// IssueDeviceToken authenticates a user and mints a device-session JWT. The JWT
// carries a unique jti recorded in the devices registry, so it can be tracked
// and revoked. ttl <= 0 means the token never expires.
func (a *AuthService) IssueDeviceToken(ctx context.Context, c Credentials, deviceName string, ttl time.Duration) (string, models.Device, error) {
	// Authenticate with username + password/Subsonic-token (not a token itself).
	user, err := a.Authenticate(ctx, Credentials{Username: c.Username, Password: c.Password, Token: c.Token, Salt: c.Salt})
	if err != nil {
		return "", models.Device{}, err
	}
	if a.devices == nil {
		return "", models.Device{}, errors.New("device sessions disabled")
	}
	now := time.Now()
	jti := uuid.NewString()
	dev := models.Device{
		ID:         jti,
		UserID:     user.ID,
		Name:       deviceName,
		UserAgent:  c.UserAgent,
		CreatedAt:  now,
		LastSeenAt: &now,
		LastIP:     c.RemoteIP,
	}
	claims := jwtClaims{Sub: user.ID, JTI: jti, IAT: now.Unix()}
	if ttl > 0 {
		exp := now.Add(ttl)
		dev.ExpiresAt = &exp
		claims.EXP = exp.Unix()
	}
	if err := a.devices.Create(ctx, dev); err != nil {
		return "", models.Device{}, err
	}
	token, err := signJWT(a.jwtKey, claims)
	if err != nil {
		return "", models.Device{}, err
	}
	return token, dev, nil
}

// authenticateJWT verifies a device JWT and resolves it against the device
// registry (must exist, not revoked, not expired). It records last-seen/IP.
func (a *AuthService) authenticateJWT(ctx context.Context, token, ip string) (models.User, error) {
	if a.devices == nil {
		return models.User{}, ErrUnauthorized
	}
	claims, err := parseJWT(a.jwtKey, token)
	if err != nil {
		return models.User{}, ErrUnauthorized
	}
	dev, err := a.devices.Get(ctx, claims.JTI)
	if err != nil || dev.Revoked || dev.UserID != claims.Sub {
		return models.User{}, ErrUnauthorized
	}
	if dev.ExpiresAt != nil && dev.ExpiresAt.Before(time.Now()) {
		return models.User{}, ErrUnauthorized
	}
	user, err := a.users.GetByID(ctx, dev.UserID)
	if err != nil {
		return models.User{}, ErrUnauthorized
	}
	_ = a.devices.TouchSeen(ctx, dev.ID, ip, time.Now())
	return user, nil
}

// ListDevices returns a user's active device sessions.
func (a *AuthService) ListDevices(ctx context.Context, userID string) ([]models.Device, error) {
	if a.devices == nil {
		return nil, nil
	}
	return a.devices.ListByUser(ctx, userID)
}

// RevokeDevice revokes a device session (its JWT stops authenticating).
func (a *AuthService) RevokeDevice(ctx context.Context, id, userID string) (bool, error) {
	if a.devices == nil {
		return false, nil
	}
	return a.devices.Revoke(ctx, id, userID)
}
