package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	melody "github.com/ermos/melody/v2"

	"github.com/immerle/immerle/internal/db"
	"github.com/immerle/immerle/internal/models"
)

// SmartPlaylistRepo persists rule-based playlists and evaluates their rules into
// a concrete track list on read.
type SmartPlaylistRepo struct{ *base }

const smartCols = `id, owner_id, name, rules, created_at, updated_at`

func scanSmart(s rowScanner) (models.SmartPlaylist, error) {
	var sp models.SmartPlaylist
	var rules string
	var created, updated int64
	if err := s.Scan(&sp.ID, &sp.OwnerID, &sp.Name, &rules, &created, &updated); err != nil {
		return sp, err
	}
	// A malformed rules blob degrades to "match the whole library" rather than
	// failing the whole list — the playlist stays usable and editable.
	_ = json.Unmarshal([]byte(rules), &sp.Rules)
	sp.CreatedAt = db.FromMillis(created)
	sp.UpdatedAt = db.FromMillis(updated)
	return sp, nil
}

// Create inserts a smart playlist.
func (r *SmartPlaylistRepo) Create(ctx context.Context, sp models.SmartPlaylist) error {
	rules, err := json.Marshal(sp.Rules)
	if err != nil {
		return err
	}
	_, err = r.bexec(ctx, r.mel.NewInsert("smart_playlists").
		Set("id", sp.ID).Set("owner_id", sp.OwnerID).Set("name", sp.Name).Set("rules", string(rules)).
		Set("created_at", db.Millis(sp.CreatedAt)).Set("updated_at", db.Millis(sp.UpdatedAt)))
	return err
}

// Update changes an owner's smart playlist (name + rules).
func (r *SmartPlaylistRepo) Update(ctx context.Context, sp models.SmartPlaylist) error {
	rules, err := json.Marshal(sp.Rules)
	if err != nil {
		return err
	}
	_, err = r.bexec(ctx, r.mel.NewUpdate("smart_playlists").
		Set("name", sp.Name).Set("rules", string(rules)).Set("updated_at", db.Millis(sp.UpdatedAt)).
		Where("id", "=", sp.ID).Where("owner_id", "=", sp.OwnerID))
	return err
}

// Get returns a smart playlist owned by ownerID, or ErrNotFound.
func (r *SmartPlaylistRepo) Get(ctx context.Context, id, ownerID string) (models.SmartPlaylist, error) {
	sp, err := scanSmart(r.bqueryRow(ctx, r.mel.New("smart_playlists").Select(smartCols).
		Where("id", "=", id).Where("owner_id", "=", ownerID)))
	if errors.Is(err, sql.ErrNoRows) {
		return sp, ErrNotFound
	}
	return sp, err
}

// ListByOwner returns a user's smart playlists, newest first.
func (r *SmartPlaylistRepo) ListByOwner(ctx context.Context, ownerID string) ([]models.SmartPlaylist, error) {
	rows, err := r.bquery(ctx, r.mel.New("smart_playlists").Select(smartCols).
		Where("owner_id", "=", ownerID).OrderBy("created_at", melody.Desc))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.SmartPlaylist
	for rows.Next() {
		sp, err := scanSmart(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

// Delete removes an owner's smart playlist.
func (r *SmartPlaylistRepo) Delete(ctx context.Context, id, ownerID string) error {
	_, err := r.bexec(ctx, r.mel.NewDelete("smart_playlists").Where("id", "=", id).Where("owner_id", "=", ownerID))
	return err
}

// Evaluate resolves the playlist's rules into a concrete track list for userID
// (whose per-user play counts / ratings / stars drive the relevant filters).
func (r *SmartPlaylistRepo) Evaluate(ctx context.Context, rules models.SmartRules, userID string) ([]models.Track, error) {
	query, args := buildSmartQuery(userID, rules, time.Now())
	rows, err := r.query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Track{}
	for rows.Next() {
		t, err := scanTrack(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// --- rule → SQL (whitelisted; values are always parameterized) ---
//
// buildSmartQuery stays hand-written: it composes a dynamic predicate tree
// (AND/OR over a runtime set of conditions) with LOWER(col) LIKE LOWER(?),
// IS NULL / IS NOT NULL, COALESCE, column-relative comparisons, a literal
// JOIN ... ON, and ORDER BY RANDOM() — none of which melody can express.

// stringField maps a string condition field to its SQL column.
var stringField = map[string]string{
	"genre": "t.genre", "artist": "ar.name", "album": "al.name", "title": "t.title",
}

// intField maps a numeric condition field to its SQL expression.
var intField = map[string]string{
	"year": "t.year", "bpm": "t.bpm",
	"playCount": "COALESCE(an.play_count,0)", "rating": "COALESCE(an.rating,0)",
}

// sortExpr maps a whitelisted sort key to its SQL ordering expression.
var sortExpr = map[string]string{
	"playCount": "COALESCE(an.play_count,0)", "recentlyAdded": "t.created_at",
	"recentlyPlayed": "an.last_played", "year": "t.year", "title": "t.title", "bpm": "t.bpm",
}

// buildSmartQuery turns rules into a track query. now is injected for testability
// (the "within N days" conditions are relative to it). The userID is bound to the
// annotations join so play counts/ratings/stars are the caller's.
func buildSmartQuery(userID string, rules models.SmartRules, now time.Time) (string, []any) {
	args := []any{userID}
	var conds []string

	for _, c := range rules.Conditions {
		if col, ok := stringField[c.Field]; ok {
			switch c.Op {
			case "is":
				conds = append(conds, col+" = ?")
				args = append(args, c.Value)
			case "isNot":
				conds = append(conds, col+" <> ?")
				args = append(args, c.Value)
			case "contains":
				conds = append(conds, "LOWER("+col+") LIKE LOWER(?)")
				args = append(args, "%"+c.Value+"%")
			}
			continue
		}
		if col, ok := intField[c.Field]; ok {
			n, err := strconv.Atoi(strings.TrimSpace(c.Value))
			if err != nil {
				continue
			}
			switch c.Op {
			case "is":
				conds = append(conds, col+" = ?")
				args = append(args, n)
			case "gte":
				conds = append(conds, col+" >= ?")
				args = append(args, n)
			case "lte":
				conds = append(conds, col+" <= ?")
				args = append(args, n)
			}
			continue
		}
		switch c.Field {
		case "starred":
			if c.Value == "false" {
				conds = append(conds, "an.starred_at IS NULL")
			} else {
				conds = append(conds, "an.starred_at IS NOT NULL")
			}
		case "neverPlayed":
			conds = append(conds, "COALESCE(an.play_count,0) = 0")
		case "addedWithinDays":
			if n, err := strconv.Atoi(strings.TrimSpace(c.Value)); err == nil {
				conds = append(conds, "t.created_at >= ?")
				args = append(args, now.Add(-time.Duration(n)*24*time.Hour).UnixMilli())
			}
		case "playedWithinDays":
			if n, err := strconv.Atoi(strings.TrimSpace(c.Value)); err == nil {
				conds = append(conds, "an.last_played >= ?")
				args = append(args, now.Add(-time.Duration(n)*24*time.Hour).UnixMilli())
			}
		}
	}

	q := trackSelect + ` LEFT JOIN annotations an ON an.user_id=? AND an.item_type='track' AND an.item_id=t.id`
	if len(conds) > 0 {
		joiner := " AND "
		if rules.Match == "any" {
			joiner = " OR "
		}
		q += " WHERE " + strings.Join(conds, joiner)
	}

	// Sort.
	if rules.Sort == "random" {
		q += " ORDER BY RANDOM()"
	} else if expr, ok := sortExpr[rules.Sort]; ok {
		dir := "DESC"
		if rules.Order == "asc" {
			dir = "ASC"
		}
		q += " ORDER BY " + expr + " " + dir
	}

	// Limit (clamped).
	limit := rules.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	q += " LIMIT ?"
	args = append(args, limit)
	return q, args
}
