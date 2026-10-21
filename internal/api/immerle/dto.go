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
