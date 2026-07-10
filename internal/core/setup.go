package core

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// ErrInvalidSetupToken is returned when the setup token is missing or wrong.
var ErrInvalidSetupToken = errors.New("invalid setup token")

// minPasswordLen is the minimum accepted password length at first-run setup.
const minPasswordLen = 8

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,64}$`)

// FieldError is a per-field validation error returned by the setup API.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationError aggregates per-field errors.
type ValidationError struct {
	Fields []FieldError
}

func (e *ValidationError) Error() string {
	msgs := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		msgs[i] = f.Field + ": " + f.Message
	}
	return "validation failed: " + strings.Join(msgs, "; ")
}

// SetupService handles first-run provisioning of the initial admin account,
// either through the setup API (see InitFirstAdmin) or, at startup, from
// ADMIN_USERNAME/ADMIN_PASSWORD (see BootstrapFromEnv).
type SetupService struct {
	users *persistence.UserRepo
	auth  *AuthService

	requireToken bool
	token        string
}

// NewSetupService builds a SetupService. When requireToken is true and the server
// is not yet initialized, a random setup token is generated; expose it via Token.
func NewSetupService(users *persistence.UserRepo, auth *AuthService, requireToken bool) (*SetupService, error) {
	s := &SetupService{users: users, auth: auth, requireToken: requireToken}
	if requireToken {
		tok, err := randomToken()
		if err != nil {
			return nil, err
		}
		s.token = tok
	}
	return s, nil
}

// IsInitialized reports whether the server already has at least one user.
func (s *SetupService) IsInitialized(ctx context.Context) (bool, error) {
	n, err := s.users.Count(ctx)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// TokenRequired reports whether a setup token must be supplied.
func (s *SetupService) TokenRequired() bool { return s.requireToken }

// Token returns the generated setup token (empty when not required).
func (s *SetupService) Token() string { return s.token }

// PersistToken writes the setup token to <dataDir>/setup-token (0600) so an
// operator can retrieve it without scraping logs. No-op when not required.
func (s *SetupService) PersistToken(dataDir string) error {
	if !s.requireToken || s.token == "" {
		return nil
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, "setup-token"), []byte(s.token+"\n"), 0o600)
}

// InitFirstAdmin validates input, checks the token and atomically creates the
// first admin. Returns ErrAlreadyInitialized, ErrInvalidSetupToken or a
// *ValidationError on failure.
func (s *SetupService) InitFirstAdmin(ctx context.Context, username, password, email, displayName, token string) (models.User, error) {
	initialized, err := s.IsInitialized(ctx)
	if err != nil {
		return models.User{}, err
	}
	if initialized {
		return models.User{}, ErrAlreadyInitialized
	}

	if s.requireToken {
		if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) != 1 {
			return models.User{}, ErrInvalidSetupToken
		}
	}

	if verr := validateAdminInput(username, password); verr != nil {
		return models.User{}, verr
	}

	// CreateFirstAdmin re-checks emptiness inside a transaction (anti-race).
	return s.auth.CreateFirstAdmin(ctx, username, password, email, displayName)
}

// BootstrapFromEnv creates the first admin from ADMIN_USERNAME/ADMIN_PASSWORD at
// startup. Unlike InitFirstAdmin it never checks a setup token: the operator
// already controls the process environment, so there is nothing left to prove.
// A no-op (ErrAlreadyInitialized) on every restart once a user exists — this is
// meant to be left set permanently in a deployment's env without recreating or
// resetting the admin account each time.
func (s *SetupService) BootstrapFromEnv(ctx context.Context, username, password string) (models.User, error) {
	initialized, err := s.IsInitialized(ctx)
	if err != nil {
		return models.User{}, err
	}
	if initialized {
		return models.User{}, ErrAlreadyInitialized
	}
	if verr := validateAdminInput(username, password); verr != nil {
		return models.User{}, verr
	}
	// CreateFirstAdmin re-checks emptiness inside a transaction (anti-race).
	return s.auth.CreateFirstAdmin(ctx, username, password, "", "")
}

func validateAdminInput(username, password string) error {
	var fields []FieldError
	if !usernamePattern.MatchString(username) {
		fields = append(fields, FieldError{Field: "username", Message: "1-64 chars, letters/digits/._- only"})
	}
	if utf8.RuneCountInString(password) < minPasswordLen {
		fields = append(fields, FieldError{Field: "password", Message: "must be at least 8 characters"})
	}
	if len(fields) > 0 {
		return &ValidationError{Fields: fields}
	}
	return nil
}

func randomToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
