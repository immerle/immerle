// Package models defines the domain entities persisted by immerle-server.
package models

import "time"

// User is an account on the server.
type User struct {
	ID           string `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	Email        string `json:"email,omitempty"`
	// DisplayName is a free-text name shown in the UI in place of the login
	// username. Empty means the client should fall back to Username.
	DisplayName string    `json:"displayName,omitempty"`
	IsAdmin     bool      `json:"isAdmin"`
	CreatedAt   time.Time `json:"createdAt"`
	// ScrobbleEnabled and SharePublicActivity are privacy/feature toggles.
	ScrobbleEnabled bool `json:"scrobbleEnabled"`
	// ActivityPrivacy is "public", "friends", or "private".
	ActivityPrivacy string `json:"activityPrivacy"`
	// Language is the user's preferred UI language (e.g. "en", "fr"). Empty means
	// the client should fall back to the device locale.
	Language string `json:"language,omitempty"`
}

// ThemeSettings holds a user's per-account UI theme, applied client-side. It is
// stored as JSON so new properties can be added without a schema change; for now
// only the accent colour is supported.
type ThemeSettings struct {
	// AccentColor is a CSS hex colour (e.g. "#3b82f6"). Empty means the client
	// should fall back to its own default.
	AccentColor string `json:"accentColor,omitempty"`
}

// ProviderConfig is an admin-managed, runtime-configurable on-demand provider.
// It is content-neutral: a name, an HTTP endpoint and an opaque JSON config
// payload describing how to reach an external catalog/download service.
type ProviderConfig struct {
	// Name is the unique provider identifier (slug).
	Name string `json:"name"`
	// Kind selects the provider implementation: "http" for a dynamic provider, or
	// "builtin" for a compile-time/config-file provider (which cannot be deleted).
	Kind string `json:"kind"`
	// Endpoint is the base http(s) URL of the external service (http kind only).
	Endpoint string `json:"endpoint"`
	// Config is the raw JSON config payload passed to the provider (http kind).
	Config string `json:"config"`
	// Enabled controls whether the provider is live (registered and usable).
	Enabled bool `json:"enabled"`
	// SortOrder is the provider's priority/position (lower first); it also drives
	// which provider search falls back to when no explicit default is configured.
	SortOrder int       `json:"sortOrder"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Builtin reports whether this is a compile-time/config-file provider (which can
// be disabled and reordered, but not deleted).
func (p ProviderConfig) Builtin() bool { return p.Kind == "builtin" }

// ProviderLog is a single warn/error event from a provider action (search,
// resolve, download), persisted so the admin can inspect failures per provider.
type ProviderLog struct {
	ID        string    `json:"id"`
	Provider  string    `json:"provider"`
	Level     string    `json:"level"`  // "warn" | "error"
	Action    string    `json:"action"` // "search" | "resolve" | "download"
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"createdAt"`
}

// RuntimeSettings holds the admin-managed, hot-or-restart configurable settings.
// They are persisted in data/configuration.yaml and editable via the admin API —
// as opposed to the bootstrap settings that live in the environment (.env) and
// need a restart.
type RuntimeSettings struct {
	Server         ServerRuntime         `json:"server"`
	Auth           AuthRuntime           `json:"auth"`
	LDAP           LDAPRuntime           `json:"ldap"`
	Transcode      TranscodeRuntime      `json:"transcode"`
	Providers      ProviderRuntime       `json:"providers"`
	Scan           ScanRuntime           `json:"scan"`
	Cleanup        CleanupRuntime        `json:"cleanup"`
	Federation     FederationRuntime     `json:"federation"`
	Import         ImportRuntime         `json:"import"`
	Logs           LogsRuntime           `json:"logs"`
	SmartPlaylists SmartPlaylistsRuntime `json:"smartPlaylists"`
	Radio          RadioRuntime          `json:"radio"`
	Wrapped        WrappedRuntime        `json:"wrapped"`
	Offline        OfflineRuntime        `json:"offline"`
	HallOfFame     HallOfFameRuntime     `json:"hallOfFame"`
}

// SmartPlaylistsRuntime toggles rule-based "smart" playlists (hot-reloadable).
// When disabled, the smart-playlist endpoints 404 and clients hide them.
type SmartPlaylistsRuntime struct {
	Enabled bool `json:"enabled"`
}

// RadioRuntime toggles internet radio stations (hot-reloadable). When disabled,
// the radio endpoints 404 / return empty and clients hide the section.
type RadioRuntime struct {
	Enabled bool `json:"enabled"`
}

// WrappedRuntime toggles the "Wrapped" year-in-review feature (hot-reloadable).
// When disabled, the wrapped endpoint 404s and clients hide the entry point.
type WrappedRuntime struct {
	Enabled bool `json:"enabled"`
}

// OfflineRuntime toggles offline downloads (hot-reloadable). When disabled, the
// offlineDownloads capability advertises enabled=false and clients hide the
// download-for-offline UI. The per-track download endpoint itself is unaffected.
type OfflineRuntime struct {
	Enabled bool `json:"enabled"`
}

// HallOfFameRuntime toggles the personal Hall of Fame feature (hot-reloadable).
// When disabled, GET /hall-of-fame 404s and clients hide its sidebar entry.
type HallOfFameRuntime struct {
	Enabled bool `json:"enabled"`
}

// LogsRuntime configures retention of persisted diagnostic logs (provider logs
// today, any future log table tomorrow). Hot-reloadable: a daily background
// pruner reads RetentionDays live and drops rows older than that window.
type LogsRuntime struct {
	// RetentionDays is how long persisted logs are kept (default 30). 0 disables
	// pruning (keep forever).
	RetentionDays int `json:"retentionDays"`
}

// ImportRuntime configures playlist-import sources (hot-reloadable). Sources
// holds per-source config keyed by source name, for sources that authenticate
// directly. Hub-backed sources (e.g. spotify) need no entry here — they use the
// federation hub, which holds the third-party credentials.
type ImportRuntime struct {
	Sources map[string]map[string]string `json:"sources,omitempty"`
}

// CleanupRuntime configures the provider-download eviction sweep. Enabled is
// hot-reloadable; the cadence is read at boot.
type CleanupRuntime struct {
	Enabled         bool `json:"enabled"`
	MaxAgeSeconds   int  `json:"maxAgeSeconds"`
	IntervalSeconds int  `json:"intervalSeconds"`
}

// ServerRuntime holds hot-reloadable server settings.
type ServerRuntime struct {
	// CORSAllowedOrigins lists browser origins permitted via CORS ("*" = any).
	CORSAllowedOrigins []string `json:"corsAllowedOrigins"`
}

// AuthRuntime holds hot-reloadable auth settings.
type AuthRuntime struct {
	// DeviceTokenTTLSeconds is the device-session JWT lifetime (0 = never).
	DeviceTokenTTLSeconds int `json:"deviceTokenTtlSeconds"`
}

// LDAPRuntime configures optional LDAP simple-bind auth (hot-reloadable; read
// live on each password login). Enabled only when Enabled is true and both URL
// and BindDNTemplate are set. Local accounts always authenticate first.
type LDAPRuntime struct {
	Enabled bool `json:"enabled"`
	// URL is the directory endpoint, e.g. "ldaps://ldap.example.com:636".
	URL string `json:"url"`
	// BindDNTemplate builds the bind DN from the username via a single %s,
	// e.g. "uid=%s,ou=people,dc=example,dc=com".
	BindDNTemplate string `json:"bindDnTemplate"`
}

// TranscodeRuntime configures transcoding (restart-required: the streamer and
// tag extractor are built at boot).
type TranscodeRuntime struct {
	FFmpegPath  string                    `json:"ffmpegPath"`
	FFprobePath string                    `json:"ffprobePath"`
	Profiles    []TranscodeProfileRuntime `json:"profiles"`
}

// TranscodeProfileRuntime describes one named output format.
type TranscodeProfileRuntime struct {
	Name       string `json:"name"`
	Format     string `json:"format"`
	BitRate    int    `json:"bitRate"`
	FFmpegArgs string `json:"ffmpegArgs,omitempty"`
}

// ProviderRuntime is the on-demand provider behaviour (all hot-reloadable). The
// on-demand catalog is always running; with no enabled provider it simply has
// nothing to search or download. The provider used for search/enrichment is the
// first one by admin-controlled order (there is no separate "default").
type ProviderRuntime struct {
	// AutoDownloadOnPlay downloads a remote result when first streamed.
	AutoDownloadOnPlay bool `json:"autoDownloadOnPlay"`
	// SearchTimeoutSeconds bounds a remote search (default 3).
	SearchTimeoutSeconds int `json:"searchTimeoutSeconds"`
}

// ScanRuntime configures library scanning. Interval is hot-reloadable; toggling
// Watch needs a restart.
type ScanRuntime struct {
	IntervalSeconds int  `json:"intervalSeconds"` // 0 disables periodic rescan
	Watch           bool `json:"watch"`
}

// FederationRuntime configures the hub connection (hot-reloadable). The hub URL
// itself is hardcoded (config.HubURL, env-overridable) and not stored here.
// Federation is active whenever the instance is linked (InstanceID + PrivateKey
// set); there is no separate enable flag and no configurable sync cadence.
type FederationRuntime struct {
	// UserID is the hub owner's UUID. The operator pastes it from the hub to claim
	// this instance; the instance then bootstraps itself under that user.
	UserID string `json:"userId"`
	// InstanceID is the hub-assigned fixed UUID (the public key sent as the
	// X-Instance-ID header), returned at bootstrap. Immutable; not user-editable.
	InstanceID string `json:"instanceId"`
	// Sqid is the editable, unique hub handle (defaults to a sqid at bootstrap).
	Sqid string `json:"sqid"`
	// InstanceName is the human-readable instance label shown on the hub (editable).
	InstanceName string `json:"instanceName"`
	// PrivateKey is the hub-issued secret (Bearer token), returned once at
	// bootstrap. Not user-editable and redacted from API responses.
	PrivateKey string `json:"privateKey"`
	// SyncPlaylists opts this instance's public playlists into being pushed to the
	// hub (off by default). Hot-reloadable.
	SyncPlaylists   bool `json:"syncPlaylists"`
	ExportScrobbles bool `json:"exportScrobbles"`
}

// DefaultRuntimeSettings returns the seed settings used on first boot.
func DefaultRuntimeSettings() RuntimeSettings {
	return RuntimeSettings{
		// No CORS origins by default: same-origin requests need no CORS headers, so
		// a fresh instance doesn't silently allow arbitrary browser origins.
		Server: ServerRuntime{CORSAllowedOrigins: []string{}},
		Auth:   AuthRuntime{DeviceTokenTTLSeconds: 720 * 3600}, // 30 days
		Transcode: TranscodeRuntime{
			FFmpegPath:  "ffmpeg",
			FFprobePath: "ffprobe",
			Profiles: []TranscodeProfileRuntime{
				{Name: "opus", Format: "opus", BitRate: 128},
				{Name: "mp3", Format: "mp3", BitRate: 192},
			},
		},
		Providers: ProviderRuntime{AutoDownloadOnPlay: true, SearchTimeoutSeconds: 3},
		Scan:      ScanRuntime{IntervalSeconds: 600, Watch: true},
		Cleanup:   CleanupRuntime{Enabled: true, MaxAgeSeconds: 720 * 3600, IntervalSeconds: 6 * 3600},
		// Import sources that go through the hub (e.g. spotify) need no per-source
		// config here — they use the federation hub credentials. This map is for
		// future sources that authenticate directly.
		Import:         ImportRuntime{},
		Logs:           LogsRuntime{RetentionDays: 30},
		SmartPlaylists: SmartPlaylistsRuntime{Enabled: true},
		Radio:          RadioRuntime{Enabled: true},
		Wrapped:        WrappedRuntime{Enabled: true},
		Offline:        OfflineRuntime{Enabled: true},
		HallOfFame:     HallOfFameRuntime{Enabled: true},
	}
}

// SmartPlaylist is a saved set of rules that materializes to tracks on read
// (a "smart"/dynamic playlist). Rules are stored as opaque JSON and evaluated
// into a track query at fetch time.
type SmartPlaylist struct {
	ID        string     `json:"id"`
	OwnerID   string     `json:"ownerId"`
	Name      string     `json:"name"`
	Rules     SmartRules `json:"rules"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// SmartRules describes how to select and order tracks for a smart playlist.
type SmartRules struct {
	// Match is "all" (AND) or "any" (OR) across Conditions. Defaults to "all".
	Match string `json:"match"`
	// Conditions are the filters; an empty list matches the whole library.
	Conditions []SmartCondition `json:"conditions"`
	// Sort is a whitelisted sort key (e.g. "random", "playCount", "recentlyAdded").
	Sort string `json:"sort"`
	// Order is "asc" or "desc" (ignored for "random").
	Order string `json:"order"`
	// Limit caps the result (clamped 1..500).
	Limit int `json:"limit"`
}

// SmartCondition is one filter: a whitelisted field, a whitelisted operator and
// a value (kept as a string and parsed per field at evaluation time).
type SmartCondition struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value string `json:"value"`
}

// RadioStation is an internet radio station: a name, an audio stream URL and an
// optional homepage. Built-in stations ship with the server and can be edited
// or disabled but not deleted; custom ones are admin-added.
type RadioStation struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	StreamURL   string `json:"streamUrl"`
	HomepageURL string `json:"homepageUrl"`
	// Country is the lower-case group code the station belongs to (e.g. "fr",
	// "gb", "int"). Used to organize the browse UI by country.
	Country string `json:"country"`
	// CoverArt is the station logo: an embedded reference ("embed:fr/covers/x.png")
	// for built-ins, or a source URL for custom stations (fetched + cached). It is
	// served locally via the station cover endpoint. Empty means "no logo".
	CoverArt  string    `json:"coverArt"`
	Builtin   bool      `json:"builtin"`
	SortOrder int       `json:"sortOrder"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	// Liked is a per-request flag (the caller liked this station). Not stored on
	// the station row — it comes from the user's annotations.
	Liked bool `json:"liked"`
}

// Wrapped is a user's year-in-review: totals plus top tracks/artists/genres and
// a per-month play histogram. Computed on demand from the scrobble history.
type Wrapped struct {
	Year         int            `json:"year"`
	TotalPlays   int            `json:"totalPlays"`
	TotalSeconds int64          `json:"totalSeconds"`
	TopTracks    []WrappedTrack `json:"topTracks"`
	TopArtists   []WrappedCount `json:"topArtists"`
	TopGenres    []WrappedCount `json:"topGenres"`
	// ByMonth is plays per calendar month, index 0 = January .. 11 = December.
	ByMonth [12]int `json:"byMonth"`
}

// WrappedTrack is one entry in the top-tracks chart.
type WrappedTrack struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Artist string `json:"artist"`
	Plays  int    `json:"plays"`
}

// WrappedCount is a labelled play count (an artist or a genre).
type WrappedCount struct {
	Name  string `json:"name"`
	Plays int    `json:"plays"`
}

// LibraryStats is a snapshot of the library analytics: catalog cardinalities
// plus the aggregate on-disk size (bytes) and duration (seconds). It is cached
// and recomputed at each scan rather than on every request.
type LibraryStats struct {
	Artists       int       `json:"artists"`
	Albums        int       `json:"albums"`
	Tracks        int       `json:"tracks"`
	TotalSize     int64     `json:"totalSize"`
	TotalDuration int64     `json:"totalDuration"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// Library is a configured root of music folders.
type Library struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"createdAt"`
}

// Artist is a performer or album artist.
type Artist struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	SortName   string    `json:"sortName,omitempty"`
	MBID       string    `json:"mbid,omitempty"`
	CoverArt   string    `json:"coverArt,omitempty"`
	AlbumCount int       `json:"albumCount"`
	CreatedAt  time.Time `json:"createdAt"`
}

// Album groups tracks.
type Album struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	ArtistID      string    `json:"artistId"`
	ArtistName    string    `json:"artist"`
	SortName      string    `json:"sortName,omitempty"`
	MBID          string    `json:"mbid,omitempty"`
	Year          int       `json:"year,omitempty"`
	Genre         string    `json:"genre,omitempty"`
	CoverArt      string    `json:"coverArt,omitempty"`
	SongCount     int       `json:"songCount"`
	Duration      int       `json:"duration"`
	IsCompilation bool      `json:"isCompilation"`
	CreatedAt     time.Time `json:"createdAt"`
}

// Participant is one contributor to a track in a given role (e.g. role
// "producer", name "Nigel Godrich"). Modelled as plain data, not a catalog
// entity — ponytail: a JSON column, no per-role artist browsing. Promote to a
// join table if "browse by composer/producer" is ever needed.
type Participant struct {
	Role string `json:"role"`
	Name string `json:"name"`
}

// Track is a single audio file.
type Track struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	AlbumID      string `json:"albumId"`
	AlbumName    string `json:"album"`
	ArtistID     string `json:"artistId"`
	ArtistName   string `json:"artist"`
	TrackNo      int    `json:"track,omitempty"`
	DiscNo       int    `json:"discNumber,omitempty"`
	Composer     string `json:"composer,omitempty"`
	Genre        string `json:"genre,omitempty"`
	Year         int    `json:"year,omitempty"`
	Duration     int    `json:"duration"`
	BitRate      int    `json:"bitRate,omitempty"`
	TitleSort    string `json:"titleSort,omitempty"`
	Work         string `json:"work,omitempty"`         // TIT1 — classical work / content group
	MovementName string `json:"movementName,omitempty"` // MVNM
	MovementNo   int    `json:"movementNumber,omitempty"`
	Lyrics       string `json:"lyrics,omitempty"` // raw, may be plain or .lrc-style synced
	// Participants are extra contributors beyond the main artist (performer,
	// producer, lyricist, …), stored as a JSON blob on the track row.
	Participants    []Participant `json:"participants,omitempty"`
	Path            string        `json:"-"`
	Suffix          string        `json:"suffix,omitempty"`
	ContentType     string        `json:"contentType,omitempty"`
	Size            int64         `json:"size,omitempty"`
	MBID            string        `json:"mbid,omitempty"`
	FileHash        string        `json:"-"`
	CoverArt        string        `json:"coverArt,omitempty"`
	BPM             int           `json:"bpm,omitempty"`
	ReplayGainTrack float64       `json:"-"`
	ReplayGainAlbum float64       `json:"-"`
	// Remote marks a track that is not yet downloaded but available via a provider.
	Remote   bool   `json:"-"`
	Provider string `json:"-"`
	// Unresolved marks a federated-playlist entry not yet matched to a local
	// track (or whose match was later removed from the library): ID is empty,
	// only the portable Title/ArtistName/AlbumName/MBID are populated.
	Unresolved bool `json:"-"`
	// UploadedBy is the id of the user who uploaded this track ("local" library);
	// empty for scanned or provider-sourced tracks.
	UploadedBy string    `json:"-"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// Genre is a normalized genre name.
type Genre struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	SongCount  int    `json:"songCount"`
	AlbumCount int    `json:"albumCount"`
}

// ItemType enumerates annotatable/shareable entity kinds.
type ItemType string

// Item type values.
const (
	ItemArtist   ItemType = "artist"
	ItemAlbum    ItemType = "album"
	ItemTrack    ItemType = "track"
	ItemPlaylist ItemType = "playlist"
)

// Annotation holds per-user state for an item (star/rating/play stats).
type Annotation struct {
	UserID     string     `json:"userId"`
	ItemType   ItemType   `json:"itemType"`
	ItemID     string     `json:"itemId"`
	Starred    *time.Time `json:"starred,omitempty"`
	Rating     int        `json:"rating,omitempty"`
	PlayCount  int        `json:"playCount"`
	LastPlayed *time.Time `json:"lastPlayed,omitempty"`
}

// Playlist is a user-owned ordered collection of tracks.
type Playlist struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	OwnerID   string `json:"ownerId"`
	OwnerName string `json:"owner"`
	Comment   string `json:"comment,omitempty"`
	Public    bool   `json:"public"`
	// Collaborative allows non-owners (per playlist_collaborators) to edit.
	Collaborative bool `json:"collaborative"`
	// Federated marks a read-only playlist synced from the hub.
	Federated bool `json:"federated"`
	// SourceInstanceID/SourceExternalID identify a federated playlist's origin on
	// the hub (empty instance id = the hub's own editorial/recommendation
	// catalog). Together they're the dedupe key, so same-named playlists from
	// different instances materialize as distinct local playlists.
	SourceInstanceID string `json:"-"`
	SourceExternalID string `json:"-"`
	SongCount        int    `json:"songCount"`
	Duration         int    `json:"duration"`
	// CoverArt is the owner-chosen custom cover (uploaded or generated), served
	// like any other cover id. Empty falls back to the track mosaic below.
	CoverArt string `json:"coverArt,omitempty"`
	// CoverArts holds the cover-art ids of the playlist's first few tracks (up to
	// 4, in order) for a mosaic thumbnail. Empty for an empty playlist.
	CoverArts []string  `json:"coverArts,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Tracks    []Track   `json:"-"`
}

// HallOfFame is a user's personal top-tracks ranking — exactly one per user,
// auto-created on first access (see core.HallOfFameService.Get). Deliberately
// its own entity (not a playlist): a dedicated table, not a flag on playlists.
type HallOfFame struct {
	ID        string    `json:"id"`
	OwnerID   string    `json:"-"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// HallOfFameEntry is one ranked track, in position order, with an optional
// personal nostalgia note ("listened to this in college").
type HallOfFameEntry struct {
	Track   Track
	Comment string
}

// PlayQueue is a user's saved server-side playback queue.
type PlayQueue struct {
	UserID     string `json:"userId"`
	Current    string `json:"current,omitempty"`
	PositionMs int64  `json:"position"`
	// Playing reports whether Current was playing (vs paused) as of ChangedAt
	// — lets another device tell playing from paused when it applies this
	// queue, and lets a spectator device push a play/pause/skip command that
	// the active device picks up (see TargetDeviceID).
	Playing   bool      `json:"playing"`
	ChangedBy string    `json:"changedBy,omitempty"`
	ChangedAt time.Time `json:"changed"`
	TrackIDs  []string  `json:"trackIds"`
	// Shuffle/Repeat mirror the saving device's transport mode, so a device that
	// mirrors or takes over this queue resumes the same mode. Repeat is one of
	// "off", "queue", "track" (see the UI's RepeatMode); not validated server-side.
	Shuffle bool   `json:"shuffle"`
	Repeat  string `json:"repeat,omitempty"`
	// Entries is a snapshot of each queued track's display metadata, used as a
	// fallback when TrackID can't be resolved locally (e.g. a not-yet-downloaded
	// on-demand track) — without it, such a track would vanish from another
	// device's mirrored queue.
	Entries []QueueEntry `json:"entries,omitempty"`
	// TargetDeviceID, when set, is the sole device that should be actively
	// playing this queue — other devices pause instead of doubling the audio.
	// Empty means unrestricted: every device manages its own playback.
	TargetDeviceID  string     `json:"targetDeviceId,omitempty"`
	TargetChangedAt *time.Time `json:"targetChangedAt,omitempty"`
	// PendingCommand is a spectator's remote-control intent, applied by the
	// active (TargetDeviceID) device itself rather than adopted as raw state
	// — see CommandEnvelope. Set independently of Current/PositionMs/Playing
	// (see PlayQueueRepo.SetCommand), so it survives a normal position sync.
	PendingCommand *CommandEnvelope `json:"pendingCommand,omitempty"`
	// CommandSeq increases on every SetCommand call — lets a device tell "a
	// new command arrived" from "the same one I already applied".
	CommandSeq int64 `json:"commandSeq,omitempty"`
}

// QueueEntry is a lightweight display-metadata snapshot for one queued
// track, supplied by the client that saved the queue (see PlayQueue.Entries).
type QueueEntry struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	CoverArt string `json:"coverArt,omitempty"`
	Duration int    `json:"duration,omitempty"`
	Remote   bool   `json:"remote,omitempty"`
}

// CommandEnvelope is a spectator device's remote-control command (next,
// previous, seek, toggle, skip to a track, toggle shuffle, cycle repeat),
// sent as an intent rather than a computed snapshot so the active device
// applies it against its own true state. "Latest command wins" (not a real
// queue) is a deliberate trade-off — don't "fix" it into a real queue
// without discussing it first.
type CommandEnvelope struct {
	// Type is one of "toggle", "next", "previous", "seekTo", "skipTo",
	// "toggleShuffle", "cycleRepeat".
	Type string `json:"type"`
	// PositionMs is the target position for a "seekTo" command.
	PositionMs int64 `json:"positionMs,omitempty"`
	// TrackID is the track to jump to for a "skipTo" command — the receiving
	// device resolves it against its own queue (not the sender's), since
	// queue contents can differ momentarily between devices.
	TrackID string `json:"trackId,omitempty"`
	// QueueIndex disambiguates a "skipTo" when TrackID appears more than once
	// in the queue (nearest match to this index) — never the primary lookup.
	QueueIndex int `json:"queueIndex,omitempty"`
	// ForTarget is the device id this command was addressed to (the sender's
	// view of TargetDeviceID at send time). The receiver ignores it if
	// ForTarget doesn't match its own id — scoped to one active-device tenure
	// so a stale command from before a handoff can't fire late.
	ForTarget string `json:"forTarget,omitempty"`
	// IssuedBy is the sending device's id.
	IssuedBy string `json:"issuedBy,omitempty"`
}

// Scrobble records a play submission.
type Scrobble struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId"`
	TrackID   string    `json:"trackId"`
	PlayedAt  time.Time `json:"playedAt"`
	Submitted bool      `json:"submitted"`
	// Exported marks scrobbles already pushed to the hub.
	Exported bool `json:"exported"`
}

// NowPlaying is an in-memory "currently playing" entry.
type NowPlaying struct {
	UserID   string    `json:"userId"`
	Username string    `json:"username"`
	TrackID  string    `json:"trackId"`
	At       time.Time `json:"at"`
}

// Share is a public link to an item.
type Share struct {
	ID          string     `json:"id"`
	UserID      string     `json:"userId"`
	ItemType    ItemType   `json:"itemType"`
	ItemID      string     `json:"itemId"`
	Secret      string     `json:"secret"`
	Description string     `json:"description,omitempty"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	ViewCount   int        `json:"viewCount"`
}

// FriendshipStatus enumerates the state of a friendship request.
type FriendshipStatus string

// Friendship status values.
const (
	FriendPending  FriendshipStatus = "pending"
	FriendAccepted FriendshipStatus = "accepted"
	FriendBlocked  FriendshipStatus = "blocked"
)

// Friendship is a directed friend relationship/request.
type Friendship struct {
	ID        string           `json:"id"`
	UserID    string           `json:"userId"`
	FriendID  string           `json:"friendId"`
	Status    FriendshipStatus `json:"status"`
	CreatedAt time.Time        `json:"createdAt"`
	UpdatedAt time.Time        `json:"updatedAt"`
}

// ActivityEvent records a user action for the social activity feed.
type ActivityEvent struct {
	ID       string `json:"id"`
	UserID   string `json:"userId"`
	Username string `json:"username"`
	// DisplayName is the author's free-text UI name (empty → client falls back to
	// Username).
	DisplayName string `json:"displayName,omitempty"`
	// Type is "listen", "add", "favorite", etc.
	Type     string   `json:"type"`
	ItemType ItemType `json:"itemType"`
	ItemID   string   `json:"itemId"`
	// Privacy is "public", "friends", or "private".
	Privacy   string    `json:"privacy"`
	CreatedAt time.Time `json:"createdAt"`
}

// JamSession is a synchronized listening session.
type JamSession struct {
	ID             string `json:"id"`
	HostID         string `json:"hostId"`
	Name           string `json:"name"`
	CurrentTrackID string `json:"currentTrackId,omitempty"`
	PositionMs     int64  `json:"position"`
	// State is "playing" or "paused".
	State     string    `json:"state"`
	TrackIDs  []string  `json:"trackIds"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// JamParticipant is a member of a jam session.
type JamParticipant struct {
	SessionID string    `json:"sessionId"`
	UserID    string    `json:"userId"`
	Username  string    `json:"username"`
	JoinedAt  time.Time `json:"joinedAt"`
}

// DownloadStatus enumerates download job states.
type DownloadStatus string

// Download job status values.
const (
	DownloadQueued    DownloadStatus = "queued"
	DownloadRunning   DownloadStatus = "running"
	DownloadCompleted DownloadStatus = "completed"
	DownloadFailed    DownloadStatus = "failed"
)

// DownloadJob is an async fetch of a remote track into the local library.
type DownloadJob struct {
	ID              string         `json:"id"`
	UserID          string         `json:"userId"`
	Provider        string         `json:"provider"`
	ProviderTrackID string         `json:"providerTrackId"`
	Query           string         `json:"query,omitempty"`
	Status          DownloadStatus `json:"status"`
	TrackID         string         `json:"trackId,omitempty"`
	Error           string         `json:"error,omitempty"`
	Attempts        int            `json:"attempts"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
}

// ImportStatus enumerates playlist-import job states.
type ImportStatus string

// Playlist-import job status values.
const (
	ImportQueued    ImportStatus = "queued"
	ImportRunning   ImportStatus = "running"
	ImportCompleted ImportStatus = "completed"
	ImportFailed    ImportStatus = "failed"
)

// ImportItemStatus enumerates the matching outcome of a single source track.
type ImportItemStatus string

// Per-track import outcomes.
const (
	ImportItemPending  ImportItemStatus = "pending"
	ImportItemMatched  ImportItemStatus = "matched"  // confidently matched and added to the playlist
	ImportItemDoubtful ImportItemStatus = "doubtful" // a candidate was found but below the confidence threshold; not added
	ImportItemMissing  ImportItemStatus = "missing"  // no candidate found at the content providers
	ImportItemFailed   ImportItemStatus = "failed"   // an error occurred while searching or downloading
)

// Import is a playlist-import job. It pulls a playlist from an external source
// (e.g. Spotify), creates a immerle playlist, then resolves each source track
// against the on-demand content providers. It is deliberately distinct from both
// the source listing and the created playlist so an "imports" page can track
// progress independently.
type Import struct {
	ID     string `json:"id"`
	UserID string `json:"userId"`
	// Source is the import-source name (e.g. "spotify").
	Source string `json:"source"`
	// SourceRef is the source playlist identifier/URL supplied by the user.
	SourceRef string `json:"sourceRef"`
	// SourcePlaylistName is the playlist name as seen at the source.
	SourcePlaylistName string `json:"sourcePlaylistName,omitempty"`
	// PlaylistID is the immerle playlist created for this import.
	PlaylistID string       `json:"playlistId,omitempty"`
	Status     ImportStatus `json:"status"`
	Total      int          `json:"total"`
	Matched    int          `json:"matched"`
	Doubtful   int          `json:"doubtful"`
	Missing    int          `json:"missing"`
	Failed     int          `json:"failed"`
	Error      string       `json:"error,omitempty"`
	CreatedAt  time.Time    `json:"createdAt"`
	UpdatedAt  time.Time    `json:"updatedAt"`
	// Items is populated only on a detail fetch.
	Items []ImportItem `json:"items,omitempty"`
}

// ImportItem is one source track and the outcome of resolving it against the
// content providers.
type ImportItem struct {
	ID           string           `json:"id"`
	ImportID     string           `json:"importId"`
	Position     int              `json:"position"`
	SourceTitle  string           `json:"sourceTitle"`
	SourceArtist string           `json:"sourceArtist"`
	SourceAlbum  string           `json:"sourceAlbum,omitempty"`
	Status       ImportItemStatus `json:"status"`
	// MatchedTrackID is the local track added to the playlist (matched items).
	MatchedTrackID string `json:"matchedTrackId,omitempty"`
	// CandidateID is the provider (remote) track id of the best candidate found
	// while resolving. For a doubtful item it lets the client preview/play the
	// candidate (the stream endpoint accepts remote ids) and validate it without
	// re-searching.
	CandidateID string `json:"candidateTrackId,omitempty"`
	// CandidateCoverArt is the candidate's cover-art id (a remote cover id usable
	// directly with getCoverArt), so a doubtful item can show artwork.
	CandidateCoverArt string `json:"candidateCoverArt,omitempty"`
	// ResolvedTitle/ResolvedArtist describe the chosen content-provider candidate.
	ResolvedTitle  string `json:"resolvedTitle,omitempty"`
	ResolvedArtist string `json:"resolvedArtist,omitempty"`
	// Confidence is the 0..1 string similarity of the chosen candidate to the
	// source track (artist + title).
	Confidence float64   `json:"confidence"`
	Note       string    `json:"note,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// APIToken is a personal access token that authenticates API requests as its
// owning user. Only a hash of the secret is stored.
type APIToken struct {
	ID         string     `json:"id"`
	UserID     string     `json:"userId"`
	Name       string     `json:"name"`
	TokenHash  string     `json:"-"`
	Prefix     string     `json:"prefix"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	Revoked    bool       `json:"revoked"`
	// IsDevice marks a token minted by the app's own login flow (one per
	// installed client) rather than a manually-created personal/CLI token —
	// only these are offered as playback-transfer targets.
	IsDevice bool `json:"isDevice,omitempty"`
}

// Device is an authenticated client session identified by a JWT's jti. The row
// is the device registry, the JWT-revocation list, and the last-seen tracker.
type Device struct {
	ID         string     `json:"id"` // = JWT jti
	UserID     string     `json:"userId"`
	Name       string     `json:"name"`
	UserAgent  string     `json:"userAgent,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastSeenAt *time.Time `json:"lastSeenAt,omitempty"`
	LastIP     string     `json:"lastIp,omitempty"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"`
	Revoked    bool       `json:"revoked"`
}

// PodcastChannel is a subscribed podcast RSS feed. Its metadata (title, image,
// description) is filled in from the feed on refresh; Status follows the
// Subsonic podcast lifecycle (new → completed, or error on a fetch failure).
type PodcastChannel struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	// CoverArt is the channel image URL taken from the feed (served by the client
	// directly; immerle does not proxy it).
	CoverArt  string    `json:"coverArt"`
	Status    string    `json:"status"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	// Episodes is populated on demand (newest first); empty when not requested.
	Episodes []PodcastEpisode `json:"episodes,omitempty"`
}

// PodcastProviderConfig is the admin-managed state of a built-in podcast
// directory adapter: whether it is enabled and its per-source credentials
// (stored as an opaque key→value map, e.g. an API key/secret).
type PodcastProviderConfig struct {
	Name      string            `json:"name"`
	Enabled   bool              `json:"enabled"`
	Config    map[string]string `json:"config"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

// PodcastEpisode is one item of a podcast feed. It is "skipped" until a user
// downloads it, at which point its audio is fetched to disk (MediaPath) and it
// becomes streamable.
type PodcastEpisode struct {
	ID          string    `json:"id"`
	ChannelID   string    `json:"channelId"`
	GUID        string    `json:"guid"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	PublishDate time.Time `json:"publishDate"`
	Duration    int       `json:"duration"` // seconds
	Size        int64     `json:"size"`
	Suffix      string    `json:"suffix"`
	ContentType string    `json:"contentType"`
	BitRate     int       `json:"bitRate"`
	// StreamURL is the original enclosure URL; MediaPath is the local file once
	// the episode has been downloaded.
	StreamURL string    `json:"streamUrl"`
	MediaPath string    `json:"-"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
