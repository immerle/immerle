-- +goose Up
-- +goose StatementBegin
CREATE TABLE users (
    id               TEXT PRIMARY KEY,
    username         TEXT NOT NULL UNIQUE,
    password_hash    TEXT NOT NULL,
    email            TEXT NOT NULL DEFAULT '',
    is_admin         INTEGER NOT NULL DEFAULT 0,
    scrobble_enabled INTEGER NOT NULL DEFAULT 1,
    activity_privacy TEXT NOT NULL DEFAULT 'friends',
    created_at       INTEGER NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE libraries (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    path       TEXT NOT NULL UNIQUE,
    created_at INTEGER NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE folders (
    id         TEXT PRIMARY KEY,
    library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    parent_id  TEXT REFERENCES folders(id) ON DELETE CASCADE,
    path       TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE artists (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    sort_name   TEXT NOT NULL DEFAULT '',
    mbid        TEXT NOT NULL DEFAULT '',
    cover_art   TEXT NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE UNIQUE INDEX idx_artists_name ON artists(name);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_artists_mbid ON artists(mbid);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE albums (
    id             TEXT PRIMARY KEY,
    name           TEXT NOT NULL,
    artist_id      TEXT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    mbid           TEXT NOT NULL DEFAULT '',
    year           INTEGER NOT NULL DEFAULT 0,
    genre          TEXT NOT NULL DEFAULT '',
    cover_art      TEXT NOT NULL DEFAULT '',
    is_compilation INTEGER NOT NULL DEFAULT 0,
    created_at     INTEGER NOT NULL
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_albums_artist ON albums(artist_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE UNIQUE INDEX idx_albums_artist_name ON albums(artist_id, name);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_albums_mbid ON albums(mbid);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE tracks (
    id               TEXT PRIMARY KEY,
    title            TEXT NOT NULL,
    album_id         TEXT NOT NULL REFERENCES albums(id) ON DELETE CASCADE,
    artist_id        TEXT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    track_no         INTEGER NOT NULL DEFAULT 0,
    disc_no          INTEGER NOT NULL DEFAULT 0,
    genre            TEXT NOT NULL DEFAULT '',
    year             INTEGER NOT NULL DEFAULT 0,
    duration         INTEGER NOT NULL DEFAULT 0,
    bitrate          INTEGER NOT NULL DEFAULT 0,
    path             TEXT NOT NULL DEFAULT '',
    suffix           TEXT NOT NULL DEFAULT '',
    content_type     TEXT NOT NULL DEFAULT '',
    size             INTEGER NOT NULL DEFAULT 0,
    mbid             TEXT NOT NULL DEFAULT '',
    file_hash        TEXT NOT NULL DEFAULT '',
    cover_art        TEXT NOT NULL DEFAULT '',
    bpm              INTEGER NOT NULL DEFAULT 0,
    replaygain_track REAL NOT NULL DEFAULT 0,
    replaygain_album REAL NOT NULL DEFAULT 0,
    remote           INTEGER NOT NULL DEFAULT 0,
    provider         TEXT NOT NULL DEFAULT '',
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_tracks_album ON tracks(album_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_tracks_artist ON tracks(artist_id);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE UNIQUE INDEX idx_tracks_path ON tracks(path) WHERE path <> '';
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_tracks_mbid ON tracks(mbid);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_tracks_hash ON tracks(file_hash);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE genres (
    id   TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE annotations (
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    item_type   TEXT NOT NULL,
    item_id     TEXT NOT NULL,
    starred_at  INTEGER,
    rating      INTEGER NOT NULL DEFAULT 0,
    play_count  INTEGER NOT NULL DEFAULT 0,
    last_played INTEGER,
    PRIMARY KEY (user_id, item_type, item_id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE playlists (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    owner_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    comment       TEXT NOT NULL DEFAULT '',
    public        INTEGER NOT NULL DEFAULT 0,
    collaborative INTEGER NOT NULL DEFAULT 0,
    federated     INTEGER NOT NULL DEFAULT 0,
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE playlist_tracks (
    playlist_id TEXT NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    track_id    TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    position    INTEGER NOT NULL,
    added_by    TEXT NOT NULL DEFAULT '',
    added_at    INTEGER NOT NULL,
    PRIMARY KEY (playlist_id, position)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE playlist_collaborators (
    playlist_id TEXT NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (playlist_id, user_id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE play_queues (
    user_id     TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    track_ids   TEXT NOT NULL DEFAULT '',
    current     TEXT NOT NULL DEFAULT '',
    position_ms INTEGER NOT NULL DEFAULT 0,
    changed_by  TEXT NOT NULL DEFAULT '',
    changed_at  INTEGER NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE scrobbles (
    id        TEXT PRIMARY KEY,
    user_id   TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    track_id  TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
    played_at INTEGER NOT NULL,
    submitted INTEGER NOT NULL DEFAULT 1,
    exported  INTEGER NOT NULL DEFAULT 0
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_scrobbles_user ON scrobbles(user_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE shares (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    item_type   TEXT NOT NULL,
    item_id     TEXT NOT NULL,
    secret      TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    expires_at  INTEGER,
    created_at  INTEGER NOT NULL,
    view_count  INTEGER NOT NULL DEFAULT 0
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE friendships (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    friend_id  TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status     TEXT NOT NULL DEFAULT 'pending',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE (user_id, friend_id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE activity_events (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type       TEXT NOT NULL,
    item_type  TEXT NOT NULL,
    item_id    TEXT NOT NULL,
    privacy    TEXT NOT NULL DEFAULT 'friends',
    created_at INTEGER NOT NULL
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_activity_user ON activity_events(user_id, created_at);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE jam_sessions (
    id               TEXT PRIMARY KEY,
    host_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name             TEXT NOT NULL DEFAULT '',
    current_track_id TEXT NOT NULL DEFAULT '',
    position_ms      INTEGER NOT NULL DEFAULT 0,
    state            TEXT NOT NULL DEFAULT 'paused',
    track_ids        TEXT NOT NULL DEFAULT '',
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE jam_participants (
    session_id TEXT NOT NULL REFERENCES jam_sessions(id) ON DELETE CASCADE,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joined_at  INTEGER NOT NULL,
    PRIMARY KEY (session_id, user_id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE download_jobs (
    id                TEXT PRIMARY KEY,
    user_id           TEXT NOT NULL DEFAULT '',
    provider          TEXT NOT NULL,
    provider_track_id TEXT NOT NULL,
    query             TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL DEFAULT 'queued',
    track_id          TEXT NOT NULL DEFAULT '',
    error             TEXT NOT NULL DEFAULT '',
    attempts          INTEGER NOT NULL DEFAULT 0,
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_download_jobs_status ON download_jobs(status);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE UNIQUE INDEX idx_download_jobs_provider_track ON download_jobs(provider, provider_track_id);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE provider_cache (
    id         TEXT PRIMARY KEY,
    provider   TEXT NOT NULL,
    query_hash TEXT NOT NULL,
    response   BLOB NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE (provider, query_hash)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS provider_cache;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS download_jobs;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS jam_participants;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS jam_sessions;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS activity_events;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS friendships;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS shares;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS scrobbles;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS play_queues;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS playlist_collaborators;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS playlist_tracks;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS playlists;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS annotations;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS genres;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS tracks;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS albums;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS artists;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS folders;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS libraries;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
