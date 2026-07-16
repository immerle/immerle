package persistence

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	melody "github.com/ermos/melody/v2"

	"github.com/immerle/immerle/internal/db"
	"github.com/immerle/immerle/internal/models"
)

// ---- Activity events ----

// ActivityRepo persists the social activity feed.
type ActivityRepo struct{ *base }

// Insert records an activity event.
func (r *ActivityRepo) Insert(ctx context.Context, e models.ActivityEvent) error {
	_, err := r.bexec(ctx, r.mel.NewInsert("activity_events").
		Set("id", e.ID).Set("user_id", e.UserID).Set("type", e.Type).Set("item_type", string(e.ItemType)).
		Set("item_id", e.ItemID).Set("privacy", e.Privacy).Set("created_at", db.Millis(e.CreatedAt)))
	return err
}

// Feed returns activity events visible to viewerID: their own events plus
// everyone's public events. Stays hand-written: a JOIN melody can't express.
func (r *ActivityRepo) Feed(ctx context.Context, viewerID string, limit int) ([]models.ActivityEvent, error) {
	q := `SELECT e.id, e.user_id, u.username, u.display_name, e.type, e.item_type, e.item_id, e.privacy, e.created_at
		FROM activity_events e JOIN users u ON u.id = e.user_id
		WHERE e.privacy='public' OR e.user_id=?
		ORDER BY e.created_at DESC LIMIT ?`
	rows, err := r.query(ctx, q, viewerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanActivityRows(rows)
}

// ByAuthor returns a single author's public activity events, newest first.
// Used to render a user's profile. Stays hand-written: a JOIN melody can't
// express.
func (r *ActivityRepo) ByAuthor(ctx context.Context, authorID string, limit int) ([]models.ActivityEvent, error) {
	q := `SELECT e.id, e.user_id, u.username, u.display_name, e.type, e.item_type, e.item_id, e.privacy, e.created_at
		FROM activity_events e JOIN users u ON u.id = e.user_id
		WHERE e.user_id=? AND e.privacy='public'
		ORDER BY e.created_at DESC LIMIT ?`
	rows, err := r.query(ctx, q, authorID, limit)
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
	_, err := r.bexec(ctx, r.mel.NewInsert("jam_sessions").
		Set("id", j.ID).Set("host_id", j.HostID).Set("name", j.Name).Set("current_track_id", j.CurrentTrackID).
		Set("position_ms", j.PositionMs).Set("state", j.State).Set("track_ids", strings.Join(j.TrackIDs, ",")).
		Set("created_at", db.Millis(j.CreatedAt)).Set("updated_at", db.Millis(j.UpdatedAt)))
	return err
}

// UpdatePlayback updates the shared playback state of a jam.
func (r *JamRepo) UpdatePlayback(ctx context.Context, id, currentTrackID string, positionMs int64, state string, trackIDs []string) error {
	_, err := r.bexec(ctx, r.mel.NewUpdate("jam_sessions").
		Set("current_track_id", currentTrackID).Set("position_ms", positionMs).Set("state", state).
		Set("track_ids", strings.Join(trackIDs, ",")).Set("updated_at", db.Millis(time.Now())).Where("id", "=", id))
	return err
}

func scanJam(s rowScanner) (models.JamSession, error) {
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
	row := r.bqueryRow(ctx, r.mel.New("jam_sessions").
		Select("id", "host_id", "name", "current_track_id", "position_ms", "state", "track_ids", "created_at", "updated_at").
		Where("id", "=", id))
	j, err := scanJam(row)
	if errors.Is(err, sql.ErrNoRows) {
		return j, ErrNotFound
	}
	return j, err
}

// Delete removes a jam session.
func (r *JamRepo) Delete(ctx context.Context, id string) error {
	_, err := r.bexec(ctx, r.mel.NewDelete("jam_sessions").Where("id", "=", id))
	return err
}

// AddParticipant joins a user to a jam.
func (r *JamRepo) AddParticipant(ctx context.Context, sessionID, userID string) error {
	_, err := r.bexec(ctx, r.mel.NewInsert("jam_participants").
		Set("session_id", sessionID).Set("user_id", userID).Set("joined_at", db.Millis(time.Now())).
		OnConflict("session_id", "user_id").OnConflictDoNothing())
	return err
}

// RemoveParticipant removes a user from a jam.
func (r *JamRepo) RemoveParticipant(ctx context.Context, sessionID, userID string) error {
	_, err := r.bexec(ctx, r.mel.NewDelete("jam_participants").
		Where("session_id", "=", sessionID).Where("user_id", "=", userID))
	return err
}

// Participants lists members of a jam. Stays hand-written: a JOIN melody can't
// express.
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

// GetByHost returns the caller's most recently created jam session, or
// ErrNotFound if they aren't hosting one — the header button's "create a Jam
// vs. invite to my Jam" check (the client can't reliably remember this across
// a reload; the in-memory jam store isn't persisted).
func (r *JamRepo) GetByHost(ctx context.Context, hostID string) (models.JamSession, error) {
	row := r.bqueryRow(ctx, r.mel.New("jam_sessions").
		Select("id", "host_id", "name", "current_track_id", "position_ms", "state", "track_ids", "created_at", "updated_at").
		Where("host_id", "=", hostID).OrderBy("created_at", melody.Desc).Limit(1))
	j, err := scanJam(row)
	if errors.Is(err, sql.ErrNoRows) {
		return j, ErrNotFound
	}
	return j, err
}

// CreateInvite invites a user to a session. Re-inviting the same (session,
// invitee) pair just refreshes created_at, so a dismissed invite resurfaces.
func (r *JamRepo) CreateInvite(ctx context.Context, inv models.JamInvite) error {
	_, err := r.bexec(ctx, r.mel.NewInsert("jam_invites").
		Set("id", inv.ID).Set("session_id", inv.SessionID).Set("inviter_id", inv.InviterID).
		Set("invitee_id", inv.InviteeID).Set("created_at", db.Millis(inv.CreatedAt)).UpdateDuplicateKey().
		OnConflict("session_id", "invitee_id"))
	return err
}

// ListInvitesForInvitee returns pending invites addressed to a user, newest
// first, with the session name and inviter's identity resolved via JOIN
// (avoids N+1 lookups from the client). Stays hand-written: a JOIN melody
// can't express.
func (r *JamRepo) ListInvitesForInvitee(ctx context.Context, userID string) ([]models.JamInvite, error) {
	rows, err := r.query(ctx, `SELECT ji.id, ji.session_id, s.name, ji.inviter_id, u.username, u.display_name, ji.created_at
		FROM jam_invites ji
		JOIN jam_sessions s ON s.id = ji.session_id
		JOIN users u ON u.id = ji.inviter_id
		WHERE ji.invitee_id=?
		ORDER BY ji.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.JamInvite
	for rows.Next() {
		var inv models.JamInvite
		var created int64
		if err := rows.Scan(&inv.ID, &inv.SessionID, &inv.SessionName, &inv.InviterID, &inv.InviterUsername, &inv.InviterDisplayName, &created); err != nil {
			return nil, err
		}
		inv.CreatedAt = db.FromMillis(created)
		out = append(out, inv)
	}
	return out, rows.Err()
}

// DeleteInviteForInvitee removes one pending invite — scoped to the invitee so
// a caller can only dismiss their own invites.
func (r *JamRepo) DeleteInviteForInvitee(ctx context.Context, id, inviteeID string) error {
	_, err := r.bexec(ctx, r.mel.NewDelete("jam_invites").Where("id", "=", id).Where("invitee_id", "=", inviteeID))
	return err
}

// DeleteInvitesForSession removes a user's pending invite to one session —
// called after they join, so an accepted invite stops showing as pending.
func (r *JamRepo) DeleteInvitesForSession(ctx context.Context, sessionID, inviteeID string) error {
	_, err := r.bexec(ctx, r.mel.NewDelete("jam_invites").
		Where("session_id", "=", sessionID).Where("invitee_id", "=", inviteeID))
	return err
}
