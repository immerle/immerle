package immerle

// This file holds the remaining doc-only mirror types: shapes the OpenAPI spec
// documents while their handler still serializes something else (usually a
// map). Nothing guarantees they match the wire, so each one is replaced by the
// type the handler really returns, and deleted, as its domain is migrated. A
// few types here are used at runtime and simply have not moved yet.
//
// Responses carry the resource directly (no envelope). Errors use the apiError
// shape declared in respond.go.

// RuntimeSettingsDTO mirrors the admin-managed runtime settings.
type RuntimeSettingsDTO struct {
	Server struct {
		CORSAllowedOrigins []string `json:"corsAllowedOrigins,omitempty" example:"*" validate:"optional"`
	} `json:"server" validate:"optional"`
	Auth struct {
		DeviceTokenTTLSeconds int `json:"deviceTokenTtlSeconds" example:"2592000" validate:"optional"`
	} `json:"auth" validate:"optional"`
	LDAP struct {
		Enabled        bool   `json:"enabled" example:"false" validate:"optional"`
		URL            string `json:"url" example:"ldaps://ldap.example.com:636" validate:"optional"`
		BindDNTemplate string `json:"bindDnTemplate" example:"uid=%s,ou=people,dc=example,dc=com" validate:"optional"`
	} `json:"ldap" validate:"optional"`
	Transcode struct {
		FFmpegPath  string `json:"ffmpegPath" example:"ffmpeg" validate:"optional"`
		FFprobePath string `json:"ffprobePath" example:"ffprobe" validate:"optional"`
		Profiles    []struct {
			Name       string `json:"name" example:"mp3" validate:"optional"`
			Format     string `json:"format" example:"mp3" validate:"optional"`
			BitRate    int    `json:"bitRate" example:"192" validate:"optional"`
			FFmpegArgs string `json:"ffmpegArgs,omitempty" validate:"optional"`
		} `json:"profiles,omitempty" validate:"optional"`
	} `json:"transcode" validate:"optional"`
	Providers struct {
		AutoDownloadOnPlay   bool `json:"autoDownloadOnPlay" example:"true" validate:"optional"`
		SearchTimeoutSeconds int  `json:"searchTimeoutSeconds" example:"3" validate:"optional"`
	} `json:"providers" validate:"optional"`
	Scan struct {
		IntervalSeconds int  `json:"intervalSeconds" example:"3600" validate:"optional"`
		Watch           bool `json:"watch" example:"true" validate:"optional"`
	} `json:"scan" validate:"optional"`
	Cleanup struct {
		Enabled         bool `json:"enabled" example:"true" validate:"optional"`
		MaxAgeSeconds   int  `json:"maxAgeSeconds" example:"2592000" validate:"optional"`
		IntervalSeconds int  `json:"intervalSeconds" example:"21600" validate:"optional"`
	} `json:"cleanup" validate:"optional"`
	Federation struct {
		// Federation is active whenever linked (instanceId set); no enable flag.
		UserID string `json:"userId" example:"6f1c2b8e-1f0a-4f9b-9c3a-1e2d3c4b5a6f" validate:"optional"`
		// InstanceID is the hub-assigned fixed UUID (read-only). Sqid is the
		// editable handle. PrivateKey is never returned (redacted server-side).
		InstanceID      string `json:"instanceId" example:"3f1c2d4e-5a6b-7c8d-9e0f-1a2b3c4d5e6f" validate:"optional"`
		Sqid            string `json:"sqid" example:"my-node" validate:"optional"`
		InstanceName    string `json:"instanceName" example:"My living-room immerle" validate:"optional"`
		SyncPlaylists   bool   `json:"syncPlaylists" validate:"optional"`
		ExportScrobbles bool   `json:"exportScrobbles" validate:"optional"`
	} `json:"federation" validate:"optional"`
	Logs struct {
		RetentionDays int `json:"retentionDays" example:"30" validate:"optional"`
	} `json:"logs" validate:"optional"`
	Sonic struct {
		Enabled    bool `json:"enabled" example:"false" validate:"optional"`
		Workers    int  `json:"workers" example:"1" validate:"optional"`
		Radio      bool `json:"radio" example:"false" validate:"optional"`
		Autoplay   bool `json:"autoplay" example:"false" validate:"optional"`
		Moods      bool `json:"moods" example:"false" validate:"optional"`
		AutoDJ     bool `json:"autoDj" example:"false" validate:"optional"`
		Duplicates bool `json:"duplicates" example:"false" validate:"optional"`
	} `json:"sonic" validate:"optional"`
	Discover struct {
		Enabled          bool   `json:"enabled" example:"true" validate:"optional"`
		Scrobble         bool   `json:"scrobble" example:"true" validate:"optional"`
		MadeForYou       bool   `json:"madeForYou" example:"true" validate:"optional"`
		CuratedPlaylists bool   `json:"curatedPlaylists" example:"true" validate:"optional"`
		SimilarTracks    bool   `json:"similarTracks" example:"true" validate:"optional"`
		InstanceID       string `json:"instanceId,omitempty" example:"6f1c2a4e-0d2b-4c55-9a51-2b8f3c1d9e07" validate:"optional"`
	} `json:"discover" validate:"optional"`
}

// FederationUpdateDTO is the body of PATCH /admin/federation: the instance
// name and its editable sqid handle, pushed to the hub.
type FederationUpdateDTO struct {
	Name string `json:"name" example:"My living-room immerle" validate:"optional"`
	Sqid string `json:"sqid" example:"my-node" validate:"optional"`
}

// SettingsDTO returns the runtime settings and restart state.
type SettingsDTO struct {
	Settings        RuntimeSettingsDTO `json:"settings" validate:"optional"`
	RestartRequired bool               `json:"restartRequired" example:"false" validate:"optional"`
	PendingRestart  []string           `json:"pendingRestart,omitempty" validate:"optional"`
}

// WrappedTrackDTO is one entry in the Wrapped top-tracks chart.
type WrappedTrackDTO struct {
	ID     string `json:"id" validate:"optional"`
	Title  string `json:"title" example:"One More Time" validate:"optional"`
	Artist string `json:"artist" example:"Daft Punk" validate:"optional"`
	Plays  int    `json:"plays" example:"42" validate:"optional"`
}

// WrappedCountDTO is a labelled play count (an artist or a genre).
type WrappedCountDTO struct {
	Name  string `json:"name" example:"Daft Punk" validate:"optional"`
	Plays int    `json:"plays" example:"120" validate:"optional"`
}

// WrappedDTO is a user's year-in-review.
type WrappedDTO struct {
	Year         int               `json:"year" example:"2025" validate:"optional"`
	TotalPlays   int               `json:"totalPlays" example:"1234" validate:"optional"`
	TotalSeconds int64             `json:"totalSeconds" example:"987654" validate:"optional"`
	TopTracks    []WrappedTrackDTO `json:"topTracks,omitempty" validate:"optional"`
	TopArtists   []WrappedCountDTO `json:"topArtists,omitempty" validate:"optional"`
	TopGenres    []WrappedCountDTO `json:"topGenres,omitempty" validate:"optional"`
	ByMonth      []int             `json:"byMonth,omitempty" validate:"optional"`
}

// SetupStatusDTO reports first-run setup state.
type SetupStatusDTO struct {
	Initialized        bool `json:"initialized" example:"false" validate:"optional"`
	NeedsSetup         bool `json:"needsSetup" example:"true" validate:"optional"`
	UserCount          int  `json:"userCount" example:"0" validate:"optional"`
	SetupTokenRequired bool `json:"setupTokenRequired" example:"false" validate:"optional"`
}

// SetupInitRequest creates the first administrator account.
type SetupInitRequest struct {
	Username string `json:"username" example:"kilian" validate:"optional"`
	Password string `json:"password" example:"a-strong-password" validate:"optional"`
	Email    string `json:"email,omitempty" example:"me@example.com" validate:"optional"`
	// DisplayName is an optional free-text name shown in the UI (falls back to
	// the username when empty).
	DisplayName string `json:"displayName,omitempty" example:"Kilian Smiti" validate:"optional"`
	// SetupToken is required only when the server was started with
	// auth.require_setup_token=true.
	SetupToken string `json:"setupToken,omitempty" validate:"optional"`
}

// UserDTO is a public user representation.
type UserDTO struct {
	ID          string `json:"id" validate:"optional"`
	Username    string `json:"username" example:"kilian" validate:"optional"`
	DisplayName string `json:"displayName,omitempty" example:"Kilian Smiti" validate:"optional"`
	IsAdmin     bool   `json:"isAdmin" example:"true" validate:"optional"`
}

// ConcertDTO is one upcoming show matched to the caller's listening history.
type ConcertDTO struct {
	ID         string `json:"id" validate:"optional"`
	Source     string `json:"source" example:"ticketmaster" validate:"optional"`
	ArtistName string `json:"artistName" example:"Daft Punk" validate:"optional"`
	EventName  string `json:"eventName" example:"Daft Punk World Tour" validate:"optional"`
	Venue      string `json:"venue,omitempty" example:"Accor Arena" validate:"optional"`
	City       string `json:"city,omitempty" example:"Paris" validate:"optional"`
	StartTime  string `json:"startTime" example:"2026-09-12T19:00:00Z" validate:"optional"`
	URL        string `json:"url,omitempty" example:"https://www.ticketmaster.com/event/..." validate:"optional"`
}

// ConcertsDTO wraps the caller's upcoming concert matches.
type ConcertsDTO struct {
	Concerts []ConcertDTO `json:"concerts,omitempty" validate:"optional"`
}

// ConcertsStatusDTO is the admin view of concert-discovery config. The API
// keys themselves are write-only — this only reports whether one is set.
type ConcertsStatusDTO struct {
	Enabled bool `json:"enabled" example:"false"`
	// Country is an ISO 3166-1 alpha-2 code (e.g. "FR"), picked from a fixed
	// dropdown in the admin UI — not a secret, returned as-is.
	Country                string `json:"country,omitempty" example:"FR" validate:"optional"`
	TicketmasterConfigured bool   `json:"ticketmasterConfigured" example:"false"`
	SkiddleConfigured      bool   `json:"skiddleConfigured" example:"false"`
}

// LastFmStatusDTO is the caller's Last.fm connection state.
type LastFmStatusDTO struct {
	Connected bool   `json:"connected"`
	Username  string `json:"username,omitempty" example:"bob" validate:"optional"`
}

// LastFmAdminDTO is the admin view of Last.fm scrobbling config. The API
// key/secret are write-only — this only reports whether they're set.
type LastFmAdminDTO struct {
	Enabled    bool `json:"enabled" example:"false"`
	Configured bool `json:"configured" example:"false"`
}

// BandcampStatusDTO is the caller's Bandcamp connection state. The cookie
// itself is write-only — this never echoes it back.
type BandcampStatusDTO struct {
	Connected      bool   `json:"connected"`
	FanID          string `json:"fanId,omitempty" validate:"optional"`
	LastSyncedAt   string `json:"lastSyncedAt,omitempty" example:"2026-07-18T21:42:00Z" validate:"optional"`
	NeedsReconnect bool   `json:"needsReconnect,omitempty" validate:"optional"`
}

// BandcampCollectionItemDTO is one purchased Bandcamp item, annotated with any
// existing import job for it.
type BandcampCollectionItemDTO struct {
	SaleItemType string `json:"saleItemType" example:"p"`
	SaleItemID   string `json:"saleItemId" example:"123456789"`
	ItemType     string `json:"itemType" example:"album"`
	ArtistName   string `json:"artistName"`
	ItemTitle    string `json:"itemTitle"`
	ArtURL       string `json:"artUrl,omitempty" validate:"optional"`
	Purchased    string `json:"purchased" example:"2021-01-01T10:00:00Z"`
	JobStatus    string `json:"jobStatus,omitempty" example:"completed" validate:"optional"`
	JobID        string `json:"jobId,omitempty" validate:"optional"`
}

// BandcampCollectionDTO wraps the caller's live Bandcamp purchase collection.
type BandcampCollectionDTO struct {
	Items []BandcampCollectionItemDTO `json:"items,omitempty" validate:"optional"`
}

// BandcampJobDTO is one queued/running/completed/failed Bandcamp import.
type BandcampJobDTO struct {
	ID           string   `json:"id"`
	SaleItemType string   `json:"saleItemType"`
	SaleItemID   string   `json:"saleItemId"`
	ItemType     string   `json:"itemType" example:"album"`
	ArtistName   string   `json:"artistName"`
	ItemTitle    string   `json:"itemTitle"`
	Format       string   `json:"format,omitempty" example:"flac" validate:"optional"`
	Status       string   `json:"status" example:"completed"`
	TrackIDs     []string `json:"trackIds,omitempty" validate:"optional"`
	Error        string   `json:"error,omitempty" validate:"optional"`
	Attempts     int      `json:"attempts"`
	CreatedAt    string   `json:"createdAt" example:"2026-07-18T21:42:00Z"`
	UpdatedAt    string   `json:"updatedAt" example:"2026-07-18T21:43:00Z"`
}

// BandcampJobsDTO wraps the caller's Bandcamp import job history.
type BandcampJobsDTO struct {
	Jobs []BandcampJobDTO `json:"jobs,omitempty" validate:"optional"`
}

// ImportItemDTO is one source track and the outcome of resolving it.
type ImportItemDTO struct {
	ID             string `json:"id" validate:"optional"`
	Position       int    `json:"position" validate:"optional"`
	SourceTitle    string `json:"sourceTitle" example:"Da Funk" validate:"optional"`
	SourceArtist   string `json:"sourceArtist" example:"Daft Punk" validate:"optional"`
	SourceAlbum    string `json:"sourceAlbum,omitempty" validate:"optional"`
	Status         string `json:"status" example:"matched" validate:"optional"`
	MatchedTrackID string `json:"matchedTrackId,omitempty" validate:"optional"`
	// CandidateTrackID / CandidateCoverArt let a client preview a doubtful item:
	// play the candidate (stream accepts the remote track id) and show its cover.
	CandidateTrackID  string  `json:"candidateTrackId,omitempty" validate:"optional"`
	CandidateCoverArt string  `json:"candidateCoverArt,omitempty" validate:"optional"`
	ResolvedTitle     string  `json:"resolvedTitle,omitempty" validate:"optional"`
	ResolvedArtist    string  `json:"resolvedArtist,omitempty" validate:"optional"`
	Confidence        float64 `json:"confidence" example:"0.62" validate:"optional"`
	Note              string  `json:"note,omitempty" validate:"optional"`
}

// ImportDTO is a playlist-import job (with items on a detail fetch).
type ImportDTO struct {
	ID                 string          `json:"id" validate:"optional"`
	Source             string          `json:"source" example:"spotify" validate:"optional"`
	SourceRef          string          `json:"sourceRef" validate:"optional"`
	SourcePlaylistName string          `json:"sourcePlaylistName,omitempty" validate:"optional"`
	PlaylistID         string          `json:"playlistId,omitempty" validate:"optional"`
	Status             string          `json:"status" example:"completed" validate:"optional"`
	Total              int             `json:"total" example:"42" validate:"optional"`
	Matched            int             `json:"matched" example:"38" validate:"optional"`
	Doubtful           int             `json:"doubtful" example:"2" validate:"optional"`
	Missing            int             `json:"missing" example:"2" validate:"optional"`
	Failed             int             `json:"failed" example:"0" validate:"optional"`
	Error              string          `json:"error,omitempty" validate:"optional"`
	Items              []ImportItemDTO `json:"items,omitempty" validate:"optional"`
}

// ImportSourceDTO describes an available import source.
type ImportSourceDTO struct {
	Name       string `json:"name" example:"spotify" validate:"optional"`
	Configured bool   `json:"configured" example:"true" validate:"optional"`
}
