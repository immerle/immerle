package immerle

import (
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
	Username string `json:"username"`
	Password string `json:"password"`
	// Token + Salt allow Subsonic-style token auth instead of a raw password.
	Token string `json:"token"`
	Salt  string `json:"salt"`
	// Device is an optional human label for the session (defaults to the username).
	Device string `json:"device"`
}

// handleLogin exchanges credentials for a device-session JWT.
//
// @Summary      Create a device session (issue a JWT)
// @Description  Authenticates with username + password (or Subsonic token auth) and returns a device-session JWT carrying a unique id (jti). Use it as "Authorization: Bearer <jwt>". The session is tracked in the devices registry and can be revoked.
// @Tags         devices
// @Accept       json
// @Produce      json
// @Param        body  body  loginRequest  true  "Credentials"
// @Success      201  {object}  LoginDTO
// @Failure      400  {object}  errorResponse
// @Failure      401  {object}  errorResponse
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
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
		return
	}
	writeResource(w, http.StatusCreated, map[string]any{
		"token": token, // the JWT — store it
		"device": map[string]any{
			"id":        dev.ID,
			"name":      dev.Name,
			"expiresAt": dev.ExpiresAt,
		},
	})
}

// handleDevices lists the caller's active device sessions.
//
// @Summary      List devices
// @Description  Lists the caller's active device sessions (one per issued JWT), with last-seen time, IP, user agent, and whether it's currently connected (seen recently).
// @Tags         devices
// @Security     BearerAuth
// @Produce      json
// @Success      200  {array}  DeviceDTO
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
	out := make([]map[string]any, 0, len(devices))
	for _, d := range devices {
		out = append(out, map[string]any{
			"id":        d.ID,
			"name":      d.Name,
			"userAgent": d.UserAgent,
			"lastSeen":  d.LastSeenAt,
			"lastIp":    d.LastIP,
			"createdAt": d.CreatedAt,
			"expiresAt": d.ExpiresAt,
			"connected": d.LastSeenAt != nil && time.Since(*d.LastSeenAt) < deviceOnlineWindow,
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
	writeResource(w, http.StatusNoContent, nil)
}
