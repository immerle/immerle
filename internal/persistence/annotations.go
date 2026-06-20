package persistence

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/immerle/immerle/internal/db"
	"github.com/immerle/immerle/internal/models"
)

// AnnotationRepo persists per-user item state (stars, ratings, play stats).
type AnnotationRepo struct{ *base }

// Get returns the annotation for a user+item, or a zero-value annotation.
func (r *AnnotationRepo) Get(ctx context.Context, userID string, it models.ItemType, itemID string) (models.Annotation, error) {
	row := r.bqueryRow(ctx, r.mel.New("annotations").
		Select("user_id", "item_type", "item_id", "starred_at", "rating", "play_count", "last_played").
		Where("user_id", "=", userID).Where("item_type", "=", string(it)).Where("item_id", "=", itemID))
	var a models.Annotation
	var starred, lastPlayed sql.NullInt64
	err := row.Scan(&a.UserID, &a.ItemType, &a.ItemID, &starred, &a.Rating, &a.PlayCount, &lastPlayed)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Annotation{UserID: userID, ItemType: it, ItemID: itemID}, nil
	}
	if err != nil {
		return a, err
	}
	a.Starred = db.TimePtr(starred)
	a.LastPlayed = db.TimePtr(lastPlayed)
	return a, nil
}

// ensure makes sure a row exists for upserts. The ON CONFLICT ... DO NOTHING
// (a conflict clause with no SET) can't be expressed by melody, so it stays
// hand-written.
func (r *AnnotationRepo) ensure(ctx context.Context, userID string, it models.ItemType, itemID string) error {
	_, err := r.exec(ctx, `INSERT INTO annotations (user_id, item_type, item_id) VALUES (?, ?, ?)
		ON CONFLICT(user_id, item_type, item_id) DO NOTHING`, userID, string(it), itemID)
	return err
}

// SetStarred stars or unstars an item.
func (r *AnnotationRepo) SetStarred(ctx context.Context, userID string, it models.ItemType, itemID string, starred bool) error {
	if err := r.ensure(ctx, userID, it, itemID); err != nil {
		return err
	}
	var val sql.NullInt64
	if starred {
		val = sql.NullInt64{Int64: db.Millis(time.Now()), Valid: true}
	}
	_, err := r.bexec(ctx, r.mel.NewUpdate("annotations").Set("starred_at", val).
		Where("user_id", "=", userID).Where("item_type", "=", string(it)).Where("item_id", "=", itemID))
	return err
}

// SetRating sets a 0-5 rating (0 clears).
func (r *AnnotationRepo) SetRating(ctx context.Context, userID string, it models.ItemType, itemID string, rating int) error {
	if err := r.ensure(ctx, userID, it, itemID); err != nil {
		return err
	}
	_, err := r.bexec(ctx, r.mel.NewUpdate("annotations").Set("rating", rating).
		Where("user_id", "=", userID).Where("item_type", "=", string(it)).Where("item_id", "=", itemID))
	return err
}

// IncrementPlay bumps the play count and last-played timestamp. The
// column-relative SET (play_count = play_count + 1) can't be expressed by
// melody, so it stays hand-written.
func (r *AnnotationRepo) IncrementPlay(ctx context.Context, userID string, it models.ItemType, itemID string, at time.Time) error {
	if err := r.ensure(ctx, userID, it, itemID); err != nil {
		return err
	}
	_, err := r.exec(ctx, `UPDATE annotations SET play_count = play_count + 1, last_played=?
		WHERE user_id=? AND item_type=? AND item_id=?`, db.Millis(at), userID, string(it), itemID)
	return err
}

// ListStarred returns the item ids of a given type starred by a user. The
// starred_at IS NOT NULL predicate can't be expressed by melody, so it stays
// hand-written.
func (r *AnnotationRepo) ListStarred(ctx context.Context, userID string, it models.ItemType) ([]string, error) {
	rows, err := r.query(ctx, `SELECT item_id FROM annotations WHERE user_id=? AND item_type=? AND starred_at IS NOT NULL
		ORDER BY starred_at DESC`, userID, string(it))
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

// AnnotationMap returns starred/rating for a set of item ids for one user.
func (r *AnnotationRepo) AnnotationMap(ctx context.Context, userID string, it models.ItemType) (map[string]models.Annotation, error) {
	rows, err := r.bquery(ctx, r.mel.New("annotations").
		Select("item_id", "starred_at", "rating", "play_count", "last_played").
		Where("user_id", "=", userID).Where("item_type", "=", string(it)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]models.Annotation)
	for rows.Next() {
		var a models.Annotation
		var starred, lastPlayed sql.NullInt64
		if err := rows.Scan(&a.ItemID, &starred, &a.Rating, &a.PlayCount, &lastPlayed); err != nil {
			return nil, err
		}
		a.UserID = userID
		a.ItemType = it
		a.Starred = db.TimePtr(starred)
		a.LastPlayed = db.TimePtr(lastPlayed)
		out[a.ItemID] = a
	}
	return out, rows.Err()
}
