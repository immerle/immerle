package persistence

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/gossignol/gossignol/internal/db"
	"github.com/gossignol/gossignol/internal/models"
)

// ---- Friendships ----

// FriendRepo persists friend relationships.
type FriendRepo struct{ *base }

// Request creates or refreshes a pending friend request.
func (r *FriendRepo) Request(ctx context.Context, f models.Friendship) error {
	_, err := r.exec(ctx, `INSERT INTO friendships (id, user_id, friend_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, friend_id) DO UPDATE SET status=excluded.status, updated_at=excluded.updated_at`,
		f.ID, f.UserID, f.FriendID, string(f.Status), db.Millis(f.CreatedAt), db.Millis(f.UpdatedAt))
	return err
}

// SetStatus updates a friendship's status (accept/block).
func (r *FriendRepo) SetStatus(ctx context.Context, userID, friendID string, status models.FriendshipStatus) error {
	_, err := r.exec(ctx, `UPDATE friendships SET status=?, updated_at=? WHERE user_id=? AND friend_id=?`,
		string(status), db.Millis(time.Now()), userID, friendID)
	return err
}

// Accept marks a request accepted and creates the reciprocal accepted edge.
func (r *FriendRepo) Accept(ctx context.Context, requesterID, accepterID, newID string) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		now := db.Millis(time.Now())
		if _, err := tx.ExecContext(ctx, r.rebind(`UPDATE friendships SET status='accepted', updated_at=? WHERE user_id=? AND friend_id=?`),
			now, requesterID, accepterID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, r.rebind(`INSERT INTO friendships (id, user_id, friend_id, status, created_at, updated_at)
			VALUES (?, ?, ?, 'accepted', ?, ?)
			ON CONFLICT(user_id, friend_id) DO UPDATE SET status='accepted', updated_at=excluded.updated_at`),
			newID, accepterID, requesterID, now, now)
		return err
	})
}

// AreFriends reports whether two users have an accepted friendship.
func (r *FriendRepo) AreFriends(ctx context.Context, a, b string) (bool, error) {
	var n int
	err := r.queryRow(ctx, `SELECT COUNT(*) FROM friendships WHERE user_id=? AND friend_id=? AND status='accepted'`, a, b).Scan(&n)
	return n > 0, err
}

// ListFriends returns accepted friend ids for a user.
func (r *FriendRepo) ListFriends(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.query(ctx, `SELECT friend_id FROM friendships WHERE user_id=? AND status='accepted'`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ListPending returns incoming pending requests for a user.
func (r *FriendRepo) ListPending(ctx context.Context, userID string) ([]models.Friendship, error) {
	rows, err := r.query(ctx, `SELECT id, user_id, friend_id, status, created_at, updated_at
		FROM friendships WHERE friend_id=? AND status='pending'`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Friendship
	for rows.Next() {
		var f models.Friendship
		var status string
		var created, updated int64
		if err := rows.Scan(&f.ID, &f.UserID, &f.FriendID, &status, &created, &updated); err != nil {
			return nil, err
		}
		f.Status = models.FriendshipStatus(status)
		f.CreatedAt = db.FromMillis(created)
		f.UpdatedAt = db.FromMillis(updated)
		out = append(out, f)
	}
	return out, rows.Err()
}

// ---- Activity events ----

// ActivityRepo persists the social activity feed.
type ActivityRepo struct{ *base }

// Insert records an activity event.
func (r *ActivityRepo) Insert(ctx context.Context, e models.ActivityEvent) error {
	_, err := r.exec(ctx, `INSERT INTO activity_events (id, user_id, type, item_type, item_id, privacy, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.UserID, e.Type, string(e.ItemType), e.ItemID, e.Privacy, db.Millis(e.CreatedAt))
	return err
}

// Feed returns activity events visible to viewerID from the given author ids,
// honoring per-event privacy. friendIDs are the viewer's accepted friends.
func (r *ActivityRepo) Feed(ctx context.Context, viewerID string, friendIDs []string, limit int) ([]models.ActivityEvent, error) {
	// Visible: own events; friends' events with privacy public/friends; anyone's public events.
	ids := append([]string{viewerID}, friendIDs...)
	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+2)
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	q := `SELECT e.id, e.user_id, u.username, u.display_name, e.type, e.item_type, e.item_id, e.privacy, e.created_at
		FROM activity_events e JOIN users u ON u.id = e.user_id
		WHERE (e.privacy='public')
		   OR (e.user_id=?)
		   OR (e.privacy='friends' AND e.user_id IN (` + strings.Join(placeholders, ",") + `))
		ORDER BY e.created_at DESC LIMIT ?`
	finalArgs := append([]any{viewerID}, args...)
	finalArgs = append(finalArgs, limit)
	rows, err := r.query(ctx, q, finalArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanActivityRows(rows)
}

// ByAuthor returns a single author's activity events whose privacy is in the
// allowed set, newest first. Used to render a user profile from a viewer's
// vantage point (the caller computes which privacy levels are visible).
func (r *ActivityRepo) ByAuthor(ctx context.Context, authorID string, privacies []string, limit int) ([]models.ActivityEvent, error) {
	if len(privacies) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(privacies))
	args := make([]any, 0, len(privacies)+2)
	args = append(args, authorID)
	for i, p := range privacies {
		placeholders[i] = "?"
		args = append(args, p)
	}
	args = append(args, limit)
	q := `SELECT e.id, e.user_id, u.username, u.display_name, e.type, e.item_type, e.item_id, e.privacy, e.created_at
		FROM activity_events e JOIN users u ON u.id = e.user_id
		WHERE e.user_id=? AND e.privacy IN (` + strings.Join(placeholders, ",") + `)
		ORDER BY e.created_at DESC LIMIT ?`
	rows, err := r.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanActivityRows(rows)
}

// scanActivityRows decodes activity rows that select the standard column set
// (incl. the author's username and display name).
func scanActivityRows(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]models.ActivityEvent, error) {
	var out []models.ActivityEvent
	for rows.Next() {
		var e models.ActivityEvent
		var itemType string
		var created int64
		if err := rows.Scan(&e.ID, &e.UserID, &e.Username, &e.DisplayName, &e.Type, &itemType, &e.ItemID, &e.Privacy, &created); err != nil {
			return nil, err
		}
		e.ItemType = models.ItemType(itemType)
		e.CreatedAt = db.FromMillis(created)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---- Jam sessions ----

// JamRepo persists synchronized listening sessions.
type JamRepo struct{ *base }

// Create inserts a jam session.
func (r *JamRepo) Create(ctx context.Context, j models.JamSession) error {
	_, err := r.exec(ctx, `INSERT INTO jam_sessions (id, host_id, name, current_track_id, position_ms, state, track_ids, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		j.ID, j.HostID, j.Name, j.CurrentTrackID, j.PositionMs, j.State, strings.Join(j.TrackIDs, ","),
		db.Millis(j.CreatedAt), db.Millis(j.UpdatedAt))
	return err
}

// UpdatePlayback updates the shared playback state of a jam.
func (r *JamRepo) UpdatePlayback(ctx context.Context, id, currentTrackID string, positionMs int64, state string, trackIDs []string) error {
	_, err := r.exec(ctx, `UPDATE jam_sessions SET current_track_id=?, position_ms=?, state=?, track_ids=?, updated_at=? WHERE id=?`,
		currentTrackID, positionMs, state, strings.Join(trackIDs, ","), db.Millis(time.Now()), id)
	return err
}

func scanJam(s interface{ Scan(...any) error }) (models.JamSession, error) {
	var j models.JamSession
	var ids string
	var created, updated int64
	if err := s.Scan(&j.ID, &j.HostID, &j.Name, &j.CurrentTrackID, &j.PositionMs, &j.State, &ids, &created, &updated); err != nil {
		return j, err
	}
	if ids != "" {
		j.TrackIDs = strings.Split(ids, ",")
	}
	j.CreatedAt = db.FromMillis(created)
	j.UpdatedAt = db.FromMillis(updated)
	return j, nil
}

// Get returns a jam session.
func (r *JamRepo) Get(ctx context.Context, id string) (models.JamSession, error) {
	row := r.queryRow(ctx, `SELECT id, host_id, name, current_track_id, position_ms, state, track_ids, created_at, updated_at
		FROM jam_sessions WHERE id=?`, id)
	j, err := scanJam(row)
	if errors.Is(err, sql.ErrNoRows) {
		return j, ErrNotFound
	}
	return j, err
}

// Delete removes a jam session.
func (r *JamRepo) Delete(ctx context.Context, id string) error {
	_, err := r.exec(ctx, `DELETE FROM jam_sessions WHERE id=?`, id)
	return err
}

// AddParticipant joins a user to a jam.
func (r *JamRepo) AddParticipant(ctx context.Context, sessionID, userID string) error {
	_, err := r.exec(ctx, `INSERT INTO jam_participants (session_id, user_id, joined_at) VALUES (?, ?, ?)
		ON CONFLICT(session_id, user_id) DO NOTHING`, sessionID, userID, db.Millis(time.Now()))
	return err
}

// RemoveParticipant removes a user from a jam.
func (r *JamRepo) RemoveParticipant(ctx context.Context, sessionID, userID string) error {
	_, err := r.exec(ctx, `DELETE FROM jam_participants WHERE session_id=? AND user_id=?`, sessionID, userID)
	return err
}

// Participants lists members of a jam.
func (r *JamRepo) Participants(ctx context.Context, sessionID string) ([]models.JamParticipant, error) {
	rows, err := r.query(ctx, `SELECT jp.session_id, jp.user_id, u.username, jp.joined_at
		FROM jam_participants jp JOIN users u ON u.id = jp.user_id WHERE jp.session_id=? ORDER BY jp.joined_at`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.JamParticipant
	for rows.Next() {
		var p models.JamParticipant
		var joined int64
		if err := rows.Scan(&p.SessionID, &p.UserID, &p.Username, &joined); err != nil {
			return nil, err
		}
		p.JoinedAt = db.FromMillis(joined)
		out = append(out, p)
	}
	return out, rows.Err()
}
