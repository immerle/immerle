package immerle

import (
	"errors"
	"net/http"
	"time"

	"github.com/immerle/immerle/internal/api/httputil"
	"github.com/immerle/immerle/internal/core"
)

// deviceOnlineWindow is how recently a device must have been seen (via an
// authenticated request) to be reported as "connected".
const deviceOnlineWindow = 5 * time.Minute

// loginRequest is the body for POST /auth/sessions.
type loginRequest struct {
	Username string `json:"username" validate:"optional"`
	Password string `json:"password" validate:"optional"`
	// Token + Salt allow Subsonic-style token auth instead of a raw password.
	Token string `json:"token" validate:"optional"`
	Salt  string `json:"salt" validate:"optional"`
	// Device is an optional human label for the session (defaults to the username).
	Device string `json:"device" validate:"optional"`
}

// handleLogin exchanges credentials for a device-session JWT.
//
// @Summary      Create a device session (issue a JWT)
// @Description  Authenticates with username + password (or Subsonic token auth) and returns a device-session JWT carrying a unique id (jti). Use it as "Authorization: Bearer <jwt>". The session is tracked in the devices registry and can be revoked.
// @Tags         devices
// @Accept       json
// @Produce      json
// @Param        body  body  loginRequest  true  "Credentials"
// @Success      201  {object}  loginView
// @Failure      400  {object}  errorResponse
// @Failure      401  {object}  errorResponse
// @Failure      429  {object}  errorResponse
// @Router       /auth/sessions [post]
func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	deviceName := req.Device
	if deviceName == "" {
		deviceName = req.Username
	}
	creds := core.Credentials{
		Username:  req.Username,
		Password:  req.Password,
		Token:     req.Token,
		Salt:      req.Salt,
		RemoteIP:  httputil.ClientIP(r),
		UserAgent: r.UserAgent(),
	}
	token, dev, err := h.Auth.IssueDeviceToken(r.Context(), creds, deviceName, h.deviceTokenTTL())
	if errors.Is(err, core.ErrTooManyAttempts) {
		h.Logger.Warn("login throttled", "username", req.Username, "remote", httputil.ClientIP(r))
		writeError(w, http.StatusTooManyRequests, "too_many_attempts", "too many failed login attempts, try again later")
		return
	}
	if err != nil {
		h.Logger.Warn("login failed", "username", req.Username, "remote", r.RemoteAddr)
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
		return
	}
	h.Logger.Info("user logged in", "username", req.Username, "device", dev.Name, "remote", r.RemoteAddr)
	writeResource(w, http.StatusCreated, loginView{
		Token:  token,
		Device: loginDeviceView{ID: dev.ID, Name: dev.Name, ExpiresAt: dev.ExpiresAt},
	})
}

// loginView is a new device session: the JWT to store and its device.
type loginView struct {
	Token  string          `json:"token"`
	Device loginDeviceView `json:"device"`
}

type loginDeviceView struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty" validate:"optional"`
}

// deviceView is one of the caller's device sessions.
type deviceView struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	UserAgent string     `json:"userAgent"`
	LastSeen  *time.Time `json:"lastSeen,omitempty" validate:"optional"`
	LastIP    string     `json:"lastIp"`
	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty" validate:"optional"`
	// Connected is true if the device was seen within the last few minutes.
	Connected bool `json:"connected"`
}

// handleDevices lists the caller's active device sessions.
//
// @Summary      List devices
// @Description  Lists the caller's active device sessions (one per issued JWT), with last-seen time, IP, user agent, and whether it's currently connected (seen recently).
// @Tags         devices
// @Security     BearerAuth
// @Produce      json
// @Success      200  {array}  deviceView
// @Failure      401  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /devices [get]
func (h *Handler) handleDevices(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	devices, err := h.Auth.ListDevices(r.Context(), user.ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	out := make([]deviceView, 0, len(devices))
	for _, d := range devices {
		out = append(out, deviceView{
			ID: d.ID, Name: d.Name, UserAgent: d.UserAgent, LastSeen: d.LastSeenAt, LastIP: d.LastIP,
			CreatedAt: d.CreatedAt, ExpiresAt: d.ExpiresAt,
			Connected: d.LastSeenAt != nil && time.Since(*d.LastSeenAt) < deviceOnlineWindow,
		})
	}
	writeResource(w, http.StatusOK, out)
}

// handleRevokeDevice revokes a device session (its JWT stops working).
//
// @Summary      Revoke a device
// @Description  Revokes a device session by id — the associated JWT can no longer authenticate.
// @Tags         devices
// @Security     BearerAuth
// @Param        id  path  string  true  "Device id (jti) to revoke"
// @Success      204  "revoked"
// @Failure      401  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /devices/{id} [delete]
func (h *Handler) handleRevokeDevice(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	ok, err := h.Auth.RevokeDevice(r.Context(), pathParam(r, "id"), user.ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "device not found")
		return
	}
	h.Logger.Info("device revoked", "user", user.Username, "device", pathParam(r, "id"))
	writeResource(w, http.StatusNoContent, nil)
}
