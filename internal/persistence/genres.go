package persistence

import (
	"context"
	"database/sql"
	"errors"

	melody "github.com/ermos/melody/v2"

	"github.com/immerle/immerle/internal/models"
)

// GenreRepo persists genres derived from track metadata.
type GenreRepo struct{ *base }

// Upsert ensures the genre exists, returning its id.
func (r *GenreRepo) Upsert(ctx context.Context, id, name string) (string, error) {
	if name == "" {
		return "", nil
	}
	var existing string
	err := r.bqueryRow(ctx, r.mel.New("genres").Select("id").Where("name", "=", name)).Scan(&existing)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		// A real query error must not silently fall through to an INSERT.
		return "", err
	}
	if _, err := r.bexec(ctx, r.mel.NewInsert("genres").Set("id", id).Set("name", name)); err != nil {
		return "", err
	}
	return id, nil
}

// List returns all genres with album/song counts computed from tracks.
func (r *GenreRepo) List(ctx context.Context) ([]models.Genre, error) {
	// The two counts are correlated subqueries; melody passes raw select columns
	// through verbatim, so the generated SQL is identical.
	rows, err := r.bquery(ctx, r.mel.New("genres g").Select(
		"g.id", "g.name",
		"(SELECT COUNT(*) FROM tracks t WHERE t.genre = g.name) AS song_count",
		"(SELECT COUNT(*) FROM albums a WHERE a.genre = g.name) AS album_count",
	).OrderBy("g.name", melody.Asc))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Genre
	for rows.Next() {
		var g models.Genre
		if err := rows.Scan(&g.ID, &g.Name, &g.SongCount, &g.AlbumCount); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
