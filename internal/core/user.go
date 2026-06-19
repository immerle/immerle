package core

import (
	"context"
	"strconv"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// UserService holds user administration with its authorization rules: who may
// read or mutate which account. Passwords reach it already decoded (the Subsonic
// "enc:" wire encoding is a presentation concern).
type UserService struct {
	users *persistence.UserRepo
	auth  *AuthService
}

// NewUserService wires the user administration application service.
func NewUserService(users *persistence.UserRepo, auth *AuthService) *UserService {
	return &UserService{users: users, auth: auth}
}

// UserUpdate carries the optional changes for UpdateUser; nil fields are left
// unchanged. AdminRaw/ScrobbleRaw are parsed as bools and applied only when
// present and valid (a malformed value leaves the field unchanged, matching the
// historical boolParam-with-current-default behavior). Password applies when
// non-empty.
type UserUpdate struct {
	Email       *string
	DisplayName *string
	AdminRaw    *string
	ScrobbleRaw *string
	Password    string
}

// GetUser returns a user by name. An empty name means the caller; a non-admin
// caller may only read themselves (ErrForbidden otherwise).
func (s *UserService) GetUser(ctx context.Context, caller models.User, username string) (models.User, error) {
	if username == "" {
		username = caller.Username
	}
	if username != caller.Username && !caller.IsAdmin {
		return models.User{}, ErrForbidden
	}
	return s.users.GetByUsername(ctx, username)
}

// ListUsers returns every user; admin only.
func (s *UserService) ListUsers(ctx context.Context, caller models.User) ([]models.User, error) {
	if !caller.IsAdmin {
		return nil, ErrForbidden
	}
	return s.users.List(ctx)
}

// CreateUser creates an account; admin only. The caller validates that username
// and password are present.
func (s *UserService) CreateUser(ctx context.Context, caller models.User, username, password, email, displayName string, admin bool) error {
	if !caller.IsAdmin {
		return ErrForbidden
	}
	_, err := s.auth.CreateUser(ctx, username, password, email, displayName, admin)
	return err
}

// UpdateUser applies metadata changes and, when set, a new password; admin only.
func (s *UserService) UpdateUser(ctx context.Context, caller models.User, username string, upd UserUpdate) error {
	if !caller.IsAdmin {
		return ErrForbidden
	}
	u, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		return err
	}
	if upd.Email != nil {
		u.Email = *upd.Email
	}
	if upd.DisplayName != nil {
		u.DisplayName = NormalizeDisplayName(*upd.DisplayName)
	}
	if upd.AdminRaw != nil {
		if b, err := strconv.ParseBool(*upd.AdminRaw); err == nil {
			u.IsAdmin = b
		}
	}
	if upd.ScrobbleRaw != nil {
		if b, err := strconv.ParseBool(*upd.ScrobbleRaw); err == nil {
			u.ScrobbleEnabled = b
		}
	}
	if err := s.users.Update(ctx, u); err != nil {
		return err
	}
	if upd.Password != "" {
		_ = s.auth.SetPassword(ctx, u.ID, upd.Password)
	}
	return nil
}

// DeleteUser removes an account; admin only.
func (s *UserService) DeleteUser(ctx context.Context, caller models.User, username string) error {
	if !caller.IsAdmin {
		return ErrForbidden
	}
	u, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		return err
	}
	return s.users.Delete(ctx, u.ID)
}

// ChangePassword sets a user's password. A non-admin caller may only change
// their own (ErrForbidden otherwise).
func (s *UserService) ChangePassword(ctx context.Context, caller models.User, username, password string) error {
	if username != caller.Username && !caller.IsAdmin {
		return ErrForbidden
	}
	u, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		return err
	}
	return s.auth.SetPassword(ctx, u.ID, password)
}
