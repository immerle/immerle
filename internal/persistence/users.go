package persistence

import (
	"context"
	"database/sql"
	"errors"

	"github.com/immerle/immerle/internal/db"
	"github.com/immerle/immerle/internal/models"
)

// UserRepo persists user accounts.
type UserRepo struct{ *base }

const userColumns = `id, username, password_hash, email, is_admin, scrobble_enabled, activity_privacy, created_at, display_name, language`

func scanUser(s rowScanner) (models.User, error) {
	var u models.User
	var isAdmin, scrobble int
	var createdAt int64
	if err := s.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Email, &isAdmin, &scrobble, &u.ActivityPrivacy, &createdAt, &u.DisplayName, &u.Language); err != nil {
		return u, err
	}
	u.IsAdmin = isAdmin != 0
	u.ScrobbleEnabled = scrobble != 0
	u.CreatedAt = db.FromMillis(createdAt)
	return u, nil
}

// Create inserts a new user.
func (r *UserRepo) Create(ctx context.Context, u models.User) error {
	_, err := r.bexec(ctx, r.mel.NewInsert("users").
		Set("id", u.ID).Set("username", u.Username).Set("password_hash", u.PasswordHash).Set("email", u.Email).
		Set("is_admin", db.Bool(u.IsAdmin)).Set("scrobble_enabled", db.Bool(u.ScrobbleEnabled)).
		Set("activity_privacy", u.ActivityPrivacy).Set("created_at", db.Millis(u.CreatedAt)).
		Set("display_name", u.DisplayName).Set("language", u.Language))
	return err
}

// CreateIfEmpty atomically inserts a user only when the users table is empty.
// It reports whether the row was created. Used to bootstrap the first admin
// without a race: concurrent first-run requests resolve to a single winner.
// Stays hand-written: it runs inside a transaction (the builder helpers use the
// pool, not the tx).
func (r *UserRepo) CreateIfEmpty(ctx context.Context, u models.User) (bool, error) {
	var created bool
	err := r.withTx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, r.rebind(`SELECT COUNT(*) FROM users`)).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return nil
		}
		if _, err := tx.ExecContext(ctx, r.rebind(`INSERT INTO users (`+userColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
			u.ID, u.Username, u.PasswordHash, u.Email, db.Bool(u.IsAdmin), db.Bool(u.ScrobbleEnabled), u.ActivityPrivacy, db.Millis(u.CreatedAt), u.DisplayName, u.Language); err != nil {
			return err
		}
		created = true
		return nil
	})
	return created, err
}

// Update changes mutable user fields.
func (r *UserRepo) Update(ctx context.Context, u models.User) error {
	_, err := r.bexec(ctx, r.mel.NewUpdate("users").
		Set("password_hash", u.PasswordHash).Set("email", u.Email).Set("display_name", u.DisplayName).
		Set("language", u.Language).Set("is_admin", db.Bool(u.IsAdmin)).Set("scrobble_enabled", db.Bool(u.ScrobbleEnabled)).
		Set("activity_privacy", u.ActivityPrivacy).Where("id", "=", u.ID))
	return err
}

// Delete removes a user.
func (r *UserRepo) Delete(ctx context.Context, id string) error {
	_, err := r.bexec(ctx, r.mel.NewDelete("users").Where("id", "=", id))
	return err
}

// GetByUsername finds a user by username.
func (r *UserRepo) GetByUsername(ctx context.Context, username string) (models.User, error) {
	row := r.bqueryRow(ctx, r.mel.New("users").Select(userColumns).Where("username", "=", username))
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

// GetByID finds a user by id.
func (r *UserRepo) GetByID(ctx context.Context, id string) (models.User, error) {
	row := r.bqueryRow(ctx, r.mel.New("users").Select(userColumns).Where("id", "=", id))
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

// List returns all users ordered by username.
func (r *UserRepo) List(ctx context.Context) ([]models.User, error) {
	rows, err := r.bquery(ctx, r.mel.New("users").Select(userColumns).OrderBy("username", ""))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []models.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// GetTheme returns the user's raw theme JSON ("{}" when unset).
func (r *UserRepo) GetTheme(ctx context.Context, userID string) (string, error) {
	var theme string
	err := r.bqueryRow(ctx, r.mel.New("users").Select("theme").Where("id", "=", userID)).Scan(&theme)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if theme == "" {
		theme = "{}"
	}
	return theme, nil
}

// SetTheme stores the user's theme as a JSON document.
func (r *UserRepo) SetTheme(ctx context.Context, userID, theme string) error {
	res, err := r.bexec(ctx, r.mel.NewUpdate("users").Set("theme", theme).Where("id", "=", userID))
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// Count returns the number of users.
func (r *UserRepo) Count(ctx context.Context) (int, error) {
	var n int
	err := r.bqueryRow(ctx, r.mel.New("users").Select("COUNT(*)")).Scan(&n)
	return n, err
}
