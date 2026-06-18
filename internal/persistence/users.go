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
	_, err := r.exec(ctx, `INSERT INTO users (`+userColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.Username, u.PasswordHash, u.Email, db.Bool(u.IsAdmin), db.Bool(u.ScrobbleEnabled), u.ActivityPrivacy, db.Millis(u.CreatedAt), u.DisplayName, u.Language)
	return err
}

// CreateIfEmpty atomically inserts a user only when the users table is empty.
// It reports whether the row was created. Used to bootstrap the first admin
// without a race: concurrent first-run requests resolve to a single winner.
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
	_, err := r.exec(ctx, `UPDATE users SET password_hash=?, email=?, display_name=?, language=?, is_admin=?, scrobble_enabled=?, activity_privacy=? WHERE id=?`,
		u.PasswordHash, u.Email, u.DisplayName, u.Language, db.Bool(u.IsAdmin), db.Bool(u.ScrobbleEnabled), u.ActivityPrivacy, u.ID)
	return err
}

// Delete removes a user.
func (r *UserRepo) Delete(ctx context.Context, id string) error {
	_, err := r.exec(ctx, `DELETE FROM users WHERE id=?`, id)
	return err
}

// GetByUsername finds a user by username.
func (r *UserRepo) GetByUsername(ctx context.Context, username string) (models.User, error) {
	row := r.queryRow(ctx, `SELECT `+userColumns+` FROM users WHERE username=?`, username)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

// GetByID finds a user by id.
func (r *UserRepo) GetByID(ctx context.Context, id string) (models.User, error) {
	row := r.queryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id=?`, id)
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

// List returns all users ordered by username.
func (r *UserRepo) List(ctx context.Context) ([]models.User, error) {
	rows, err := r.query(ctx, `SELECT `+userColumns+` FROM users ORDER BY username`)
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
	err := r.queryRow(ctx, `SELECT theme FROM users WHERE id=?`, userID).Scan(&theme)
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
	res, err := r.exec(ctx, `UPDATE users SET theme=? WHERE id=?`, theme, userID)
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
	err := r.queryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}
