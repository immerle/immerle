package persistence

import (
	"context"

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
	err := r.queryRow(ctx, `SELECT id FROM genres WHERE name=?`, name).Scan(&existing)
	if err == nil {
		return existing, nil
	}
	if _, err := r.exec(ctx, `INSERT INTO genres (id, name) VALUES (?, ?)`, id, name); err != nil {
		return "", err
	}
	return id, nil
}

// List returns all genres with album/song counts computed from tracks.
func (r *GenreRepo) List(ctx context.Context) ([]models.Genre, error) {
	rows, err := r.query(ctx, `
		SELECT g.id, g.name,
		       (SELECT COUNT(*) FROM tracks t WHERE t.genre = g.name) AS song_count,
		       (SELECT COUNT(*) FROM albums a WHERE a.genre = g.name) AS album_count
		FROM genres g ORDER BY g.name`)
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
