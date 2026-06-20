package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/google/uuid"

	"github.com/immerle/immerle/internal/db"
	"github.com/immerle/immerle/internal/models"
)

// PodcastRepo persists podcast channels and their episodes.
type PodcastRepo struct{ *base }

const channelCols = `id, url, title, description, cover_art, status, error, created_at, updated_at`

const episodeCols = `id, channel_id, guid, title, description, publish_date, duration, size, suffix, content_type, bit_rate, stream_url, media_path, status, created_at, updated_at`

func scanChannel(s rowScanner) (models.PodcastChannel, error) {
	var c models.PodcastChannel
	var created, updated int64
	if err := s.Scan(&c.ID, &c.URL, &c.Title, &c.Description, &c.CoverArt, &c.Status, &c.Error, &created, &updated); err != nil {
		return c, err
	}
	c.CreatedAt = db.FromMillis(created)
	c.UpdatedAt = db.FromMillis(updated)
	return c, nil
}

func scanEpisode(s rowScanner) (models.PodcastEpisode, error) {
	var e models.PodcastEpisode
	var pub, created, updated int64
	if err := s.Scan(&e.ID, &e.ChannelID, &e.GUID, &e.Title, &e.Description, &pub, &e.Duration,
		&e.Size, &e.Suffix, &e.ContentType, &e.BitRate, &e.StreamURL, &e.MediaPath, &e.Status, &created, &updated); err != nil {
		return e, err
	}
	if pub != 0 {
		e.PublishDate = db.FromMillis(pub)
	}
	e.CreatedAt = db.FromMillis(created)
	e.UpdatedAt = db.FromMillis(updated)
	return e, nil
}

// ListChannels returns all channels, newest first.
func (r *PodcastRepo) ListChannels(ctx context.Context) ([]models.PodcastChannel, error) {
	rows, err := r.bquery(ctx, r.mel.New("podcast_channels").Select(channelCols).OrderBy("created_at", "DESC"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.PodcastChannel{}
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetChannel returns a channel by id, or ErrNotFound.
func (r *PodcastRepo) GetChannel(ctx context.Context, id string) (models.PodcastChannel, error) {
	c, err := scanChannel(r.bqueryRow(ctx, r.mel.New("podcast_channels").Select(channelCols).Where("id", "=", id)))
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// CreateChannel inserts a channel.
func (r *PodcastRepo) CreateChannel(ctx context.Context, c models.PodcastChannel) error {
	_, err := r.bexec(ctx, r.mel.NewInsert("podcast_channels").
		Set("id", c.ID).Set("url", c.URL).Set("title", c.Title).Set("description", c.Description).
		Set("cover_art", c.CoverArt).Set("status", c.Status).Set("error", c.Error).
		Set("created_at", db.Millis(c.CreatedAt)).Set("updated_at", db.Millis(c.UpdatedAt)))
	return err
}

// UpdateChannel refreshes a channel's feed-derived metadata and status.
func (r *PodcastRepo) UpdateChannel(ctx context.Context, c models.PodcastChannel) error {
	_, err := r.bexec(ctx, r.mel.NewUpdate("podcast_channels").
		Set("title", c.Title).Set("description", c.Description).Set("cover_art", c.CoverArt).
		Set("status", c.Status).Set("error", c.Error).Set("updated_at", db.Millis(c.UpdatedAt)).
		Where("id", "=", c.ID))
	return err
}

// DeleteChannel removes a channel and all its episodes.
func (r *PodcastRepo) DeleteChannel(ctx context.Context, id string) error {
	if _, err := r.bexec(ctx, r.mel.NewDelete("podcast_episodes").Where("channel_id", "=", id)); err != nil {
		return err
	}
	_, err := r.bexec(ctx, r.mel.NewDelete("podcast_channels").Where("id", "=", id))
	return err
}

// ListEpisodes returns a channel's episodes, newest first.
func (r *PodcastRepo) ListEpisodes(ctx context.Context, channelID string) ([]models.PodcastEpisode, error) {
	return r.queryEpisodes(ctx, r.mel.New("podcast_episodes").Select(episodeCols).
		Where("channel_id", "=", channelID).OrderBy("publish_date", "DESC"))
}

// NewestEpisodes returns the most recent episodes across all channels.
func (r *PodcastRepo) NewestEpisodes(ctx context.Context, limit int) ([]models.PodcastEpisode, error) {
	return r.queryEpisodes(ctx, r.mel.New("podcast_episodes").Select(episodeCols).
		OrderBy("publish_date", "DESC").Limit(limit))
}

func (r *PodcastRepo) queryEpisodes(ctx context.Context, sb sqlBuilder) ([]models.PodcastEpisode, error) {
	rows, err := r.bquery(ctx, sb)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.PodcastEpisode{}
	for rows.Next() {
		e, err := scanEpisode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetEpisode returns an episode by id, or ErrNotFound.
func (r *PodcastRepo) GetEpisode(ctx context.Context, id string) (models.PodcastEpisode, error) {
	e, err := scanEpisode(r.bqueryRow(ctx, r.mel.New("podcast_episodes").Select(episodeCols).Where("id", "=", id)))
	if errors.Is(err, sql.ErrNoRows) {
		return e, ErrNotFound
	}
	return e, err
}

// EpisodeExists reports whether an episode with this (channel, guid) is known —
// used by refresh to skip items already imported.
func (r *PodcastRepo) EpisodeExists(ctx context.Context, channelID, guid string) (bool, error) {
	var n int
	err := r.bqueryRow(ctx, r.mel.New("podcast_episodes").Select("COUNT(1)").
		Where("channel_id", "=", channelID).Where("guid", "=", guid)).Scan(&n)
	return n > 0, err
}

// CreateEpisode inserts a freshly discovered episode.
func (r *PodcastRepo) CreateEpisode(ctx context.Context, e models.PodcastEpisode) error {
	_, err := r.bexec(ctx, r.mel.NewInsert("podcast_episodes").
		Set("id", e.ID).Set("channel_id", e.ChannelID).Set("guid", e.GUID).Set("title", e.Title).
		Set("description", e.Description).Set("publish_date", db.Millis(e.PublishDate)).Set("duration", e.Duration).
		Set("size", e.Size).Set("suffix", e.Suffix).Set("content_type", e.ContentType).Set("bit_rate", e.BitRate).
		Set("stream_url", e.StreamURL).Set("media_path", e.MediaPath).Set("status", e.Status).
		Set("created_at", db.Millis(e.CreatedAt)).Set("updated_at", db.Millis(e.UpdatedAt)))
	return err
}

// UpdateEpisode persists download progress (status, local file, size).
func (r *PodcastRepo) UpdateEpisode(ctx context.Context, e models.PodcastEpisode) error {
	_, err := r.bexec(ctx, r.mel.NewUpdate("podcast_episodes").
		Set("status", e.Status).Set("media_path", e.MediaPath).Set("size", e.Size).
		Set("suffix", e.Suffix).Set("content_type", e.ContentType).Set("updated_at", db.Millis(e.UpdatedAt)).
		Where("id", "=", e.ID))
	return err
}

// DeleteEpisode removes an episode row.
func (r *PodcastRepo) DeleteEpisode(ctx context.Context, id string) error {
	_, err := r.bexec(ctx, r.mel.NewDelete("podcast_episodes").Where("id", "=", id))
	return err
}

// NewPodcastID returns a fresh id for a channel or episode.
func NewPodcastID() string { return uuid.NewString() }

// --- directory provider config (enabled + credentials per built-in adapter) ---

// ProviderConfigs returns the stored config keyed by provider name. Providers
// with no row yet are simply absent (treated as disabled, empty config).
func (r *PodcastRepo) ProviderConfigs(ctx context.Context) (map[string]models.PodcastProviderConfig, error) {
	rows, err := r.bquery(ctx, r.mel.New("podcast_providers").Select("name, enabled, config, updated_at"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]models.PodcastProviderConfig{}
	for rows.Next() {
		var c models.PodcastProviderConfig
		var enabled int
		var cfg string
		var updated int64
		if err := rows.Scan(&c.Name, &enabled, &cfg, &updated); err != nil {
			return nil, err
		}
		c.Enabled = enabled != 0
		c.Config = map[string]string{}
		_ = json.Unmarshal([]byte(cfg), &c.Config)
		c.UpdatedAt = db.FromMillis(updated)
		out[c.Name] = c
	}
	return out, rows.Err()
}

// SaveProviderConfig upserts a provider's enabled flag and credentials.
func (r *PodcastRepo) SaveProviderConfig(ctx context.Context, c models.PodcastProviderConfig) error {
	raw, err := json.Marshal(c.Config)
	if err != nil {
		return err
	}
	_, err = r.bexec(ctx, r.mel.NewInsert("podcast_providers").
		Set("name", c.Name).
		Set("enabled", db.Bool(c.Enabled)).UpdateDuplicateKey().
		Set("config", string(raw)).UpdateDuplicateKey().
		Set("updated_at", db.Millis(c.UpdatedAt)).UpdateDuplicateKey().
		OnConflict("name"))
	return err
}
