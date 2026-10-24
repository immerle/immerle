package immerle

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"

	"github.com/immerle/immerle/internal/models"
)

// handleSettings returns the DB-backed runtime settings.
//
// @Summary      Get runtime settings
// @Description  Admin only. Returns the current runtime settings (provider behaviour, artist avatars, scan cadence, federation) plus whether a restart is pending.
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  settingsView
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /admin/settings [get]
func (h *Handler) handleSettings(w http.ResponseWriter, r *http.Request) {
	if h.Settings == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "settings unavailable")
		return
	}
	writeResource(w, http.StatusOK, settingsBody(redactSettings(h.Settings.Get()), h.Settings.PendingRestart()))
}

// redactSettings clears secrets that must never leave the server (the hub
// private key, the concert-discovery API keys) before serializing settings to
// an admin client.
func redactSettings(rs models.RuntimeSettings) models.RuntimeSettings {
	rs.Import = models.ImportRuntime{} // per-source import credentials
	rs.Federation.PrivateKey = ""
	rs.Concerts.TicketmasterAPIKey = ""
	rs.Concerts.SkiddleAPIKey = ""
	rs.LastFm.APIKey = ""
	rs.LastFm.APISecret = ""
	rs.Discover.PrivateKey = ""
	// Never send null lists (the spec marks them required).
	if rs.Server.CORSAllowedOrigins == nil {
		rs.Server.CORSAllowedOrigins = []string{}
	}
	if rs.Transcode.Profiles == nil {
		rs.Transcode.Profiles = []models.TranscodeProfileRuntime{}
	}
	return rs
}

// handleSettingsUpdate applies a partial update to the runtime settings.
//
// @Summary      Update runtime settings
// @Description  Admin only. Partial update (send a JSON body with the fields to change; omitted fields keep their current value). Provider behaviour and the scan interval apply immediately (hot reload); avatars, the scan watcher and federation only take effect after a restart — the response sets restartRequired=true and lists the pending fields.
// @Tags         admin
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  settingsPatchRequest  true  "Settings fields to change (partial)"
// @Success      200  {object}  settingsView
// @Failure      400  {object}  errorResponse
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /admin/settings [patch]
func (h *Handler) handleSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	if h.Settings == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "settings unavailable")
		return
	}
	// Decode the body over the current values → partial update (omitted fields
	// keep their current value).
	current := h.Settings.Get()
	merged := current
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&merged); err != nil {
		writeErrorParams(w, http.StatusBadRequest, "invalid_body", "invalid settings JSON: "+err.Error(), map[string]any{"detail": err.Error()})
		return
	}
	// Mass-assignment guard: copy over only the fields settingsPatchRequest
	// declares. Everything else keeps its current value.
	next := current
	copyDeclared(reflect.TypeOf(settingsPatchRequest{}), reflect.ValueOf(&next).Elem(), reflect.ValueOf(merged))
	saved, pending, err := h.Settings.Update(next)
	if err != nil {
		writeInternal(w, err)
		return
	}
	// Turning playlist sync off removes this instance's playlists from the hub.
	if h.PlaylistSync != nil && current.Federation.SyncPlaylists && !saved.Federation.SyncPlaylists {
		if err := h.PlaylistSync.PurgePlaylists(r.Context()); err != nil {
			h.Logger.Warn("purge synced playlists failed", "error", err)
		}
	}
	if saved.Sonic.Enabled && !current.Sonic.Enabled {
		h.Sonic.Wake() // start analyzing now, not at the next idle tick
	}
	// Publish (or remove) the mood playlists now rather than at the next daily sync.
	if h.AutoPlaylists != nil && h.Sonic.MoodsEnabled() != (current.Sonic.Enabled && current.Sonic.Moods) {
		go func() {
			if _, err := h.AutoPlaylists.SyncNow(context.WithoutCancel(r.Context())); err != nil {
				h.Logger.Warn("auto-playlist sync after a moods toggle failed", "error", err)
			}
		}()
	}
	h.Logger.Info("runtime settings updated", "restartRequired", len(pending) > 0, "pending", pending, "by", userFrom(r.Context()).Username)
	writeResource(w, http.StatusOK, settingsBody(redactSettings(saved), pending))
}

// handleFederationRegister registers this instance with the hub.
//
// @Summary      Link with the hub
// @Description  Admin only. Bootstraps the instance under the configured hub user id (federation.userId) and persists the hub-issued identity (instance UUID, sqid handle and private key). The full HTTP exchange runs server-side. Returns the refreshed runtime settings.
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  settingsView
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      502  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /admin/federation/register [post]
func (h *Handler) handleFederationRegister(w http.ResponseWriter, r *http.Request) {
	if h.Federation == nil || h.Settings == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "federation unavailable")
		return
	}
	if err := h.Federation.Register(r.Context()); err != nil {
		writeError(w, http.StatusBadGateway, "register_failed", err.Error())
		return
	}
	// Register persists the assigned id + private key; return the fresh settings.
	writeResource(w, http.StatusOK, settingsBody(redactSettings(h.Settings.Get()), h.Settings.PendingRestart()))
}

// handleFederationProfile refreshes this instance's live name/sqid from the hub.
//
// @Summary      Get the live hub instance profile
// @Description  Admin only. Fetches this instance's current name and sqid handle from the hub (the source of truth) and persists them, then returns the refreshed runtime settings. The HTTP exchange runs server-side.
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  settingsView
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      409  {object}  errorResponse
// @Failure      502  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /admin/federation [get]
func (h *Handler) handleFederationProfile(w http.ResponseWriter, r *http.Request) {
	if h.Federation == nil || h.Settings == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "federation unavailable")
		return
	}
	if err := h.Federation.RefreshProfile(r.Context()); err != nil {
		writeError(w, http.StatusBadGateway, "refresh_failed", err.Error())
		return
	}
	writeResource(w, http.StatusOK, settingsBody(redactSettings(h.Settings.Get()), h.Settings.PendingRestart()))
}

// handleFederationUnlink unlinks this instance from the hub.
//
// @Summary      Unlink this instance from the hub
// @Description  Admin only. Deletes this instance's data on the hub (best-effort) and clears the locally stored hub identity, returning the instance to the unlinked state. Returns the refreshed runtime settings.
// @Tags         admin
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  settingsView
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /admin/federation [delete]
func (h *Handler) handleFederationUnlink(w http.ResponseWriter, r *http.Request) {
	if h.Federation == nil || h.Settings == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "federation unavailable")
		return
	}
	if err := h.Federation.Unlink(r.Context()); err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusOK, settingsBody(redactSettings(h.Settings.Get()), h.Settings.PendingRestart()))
}

// handleFederationUpdate pushes a name / sqid change to the hub.
//
// @Summary      Update this instance on the hub
// @Description  Admin only. Pushes the instance name and sqid (the editable, unique hub handle) to the hub, which validates sqid uniqueness, then persists the hub-canonical values. The HTTP exchange runs server-side. Returns the refreshed runtime settings.
// @Tags         admin
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  federationUpdateRequest  true  "Instance name and sqid"
// @Success      200  {object}  settingsView
// @Failure      400  {object}  errorResponse
// @Failure      401  {object}  errorResponse
// @Failure      403  {object}  errorResponse
// @Failure      409  {object}  errorResponse
// @Failure      502  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /admin/federation [patch]
func (h *Handler) handleFederationUpdate(w http.ResponseWriter, r *http.Request) {
	if h.Federation == nil || h.Settings == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "federation unavailable")
		return
	}
	var body federationUpdateRequest
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorParams(w, http.StatusBadRequest, "invalid_body", "invalid JSON: "+err.Error(), map[string]any{"detail": err.Error()})
		return
	}
	if err := h.Federation.UpdateInstance(r.Context(), body.Name, body.Sqid); err != nil {
		writeError(w, http.StatusBadGateway, "update_failed", err.Error())
		return
	}
	writeResource(w, http.StatusOK, settingsBody(redactSettings(h.Settings.Get()), h.Settings.PendingRestart()))
}

// settingsView is the (redacted) runtime settings plus the restart status.
type settingsView struct {
	Settings        models.RuntimeSettings `json:"settings"`
	RestartRequired bool                   `json:"restartRequired"`
	// PendingRestart lists the changed settings that apply after a restart.
	PendingRestart []string `json:"pendingRestart" validate:"required"`
}

func settingsBody(settings models.RuntimeSettings, pending []string) settingsView {
	if pending == nil {
		pending = []string{}
	}
	return settingsView{Settings: settings, RestartRequired: len(pending) > 0, PendingRestart: pending}
}

// federationUpdateRequest is the PATCH /admin/federation body.
type federationUpdateRequest struct {
	Name string `json:"name" example:"My living-room immerle" validate:"optional"`
	Sqid string `json:"sqid" example:"my-node" validate:"optional"`
}

// settingsPatchRequest is the PATCH /admin/settings body: every field is
// optional (omitted fields keep their value), and it lists exactly the fields
// the endpoint writes. The handler copies those, and only those, from the
// decoded body (see copyDeclared), so the documented and the writable fields
// cannot drift apart. Feature toggles with their own endpoint, import
// credentials and the hub-managed identities are not in it.
type settingsPatchRequest struct {
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
		// Federation is active whenever linked; the instance identity is managed
		// through PATCH /admin/federation.
		UserID          string `json:"userId" example:"6f1c2b8e-1f0a-4f9b-9c3a-1e2d3c4b5a6f" validate:"optional"`
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
		Enabled          bool `json:"enabled" example:"true" validate:"optional"`
		Scrobble         bool `json:"scrobble" example:"true" validate:"optional"`
		MadeForYou       bool `json:"madeForYou" example:"true" validate:"optional"`
		CuratedPlaylists bool `json:"curatedPlaylists" example:"true" validate:"optional"`
		SimilarTracks    bool `json:"similarTracks" example:"true" validate:"optional"`
	} `json:"discover" validate:"optional"`
}

// copyDeclared copies from src into dst every field that the patch type
// declares, matched by json name and recursing into nested structs; a declared
// slice or scalar is copied whole. dst and src are the same struct type. It
// panics on a declared field dst lacks, which the settings tests catch.
func copyDeclared(patch reflect.Type, dst, src reflect.Value) {
	for i := 0; i < patch.NumField(); i++ {
		pf := patch.Field(i)
		name := jsonName(pf)
		j := fieldIndexByJSON(dst.Type(), name)
		if j < 0 {
			panic("settings patch field " + name + " has no runtime-settings counterpart in " + dst.Type().Name())
		}
		df, sf := dst.Field(j), src.Field(j)
		if pf.Type.Kind() == reflect.Struct && df.Kind() == reflect.Struct {
			copyDeclared(pf.Type, df, sf)
			continue
		}
		df.Set(sf)
	}
}

func jsonName(f reflect.StructField) string {
	name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	return name
}

func fieldIndexByJSON(t reflect.Type, name string) int {
	for i := 0; i < t.NumField(); i++ {
		if jsonName(t.Field(i)) == name {
			return i
		}
	}
	return -1
}
