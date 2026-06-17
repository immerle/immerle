package immerle

import (
	"net/http"

	"github.com/immerle/immerle/internal/api/httputil"
	"github.com/immerle/immerle/internal/core"
)

// handleLogin exchanges credentials for a device-session JWT.
//
// @Summary      Log in a device (issue a JWT)
// @Description  Authenticates with username + password (or Subsonic token auth) and returns a device-session JWT carrying a unique id (jti). Use it as "Authorization: Bearer <jwt>". The session is tracked in the devices registry and can be revoked.
// @Tags         devices
// @Produce      json
// @Param        u       query  string  true   "Username"
// @Param        p       query  string  false  "Password"
// @Param        t       query  string  false  "Subsonic token md5(password+salt)"
// @Param        s       query  string  false  "Subsonic salt"
// @Param        c       query  string  true   "Client/device name"
// @Param        device  query  string  false  "Device label (defaults to c)"
// @Success      200  {object}  LoginResponse
// @Failure      401  {object}  ErrorResponse
// @Router       /auth/login [post]
func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	deviceName := r.Form.Get("device")
	if deviceName == "" {
		deviceName = r.Form.Get("c")
	}
	creds := core.Credentials{
		Username:  r.Form.Get("u"),
		Password:  r.Form.Get("p"),
		Token:     r.Form.Get("t"),
		Salt:      r.Form.Get("s"),
		RemoteIP:  httputil.ClientIP(r),
		UserAgent: r.UserAgent(),
	}
	token, dev, err := h.Auth.IssueDeviceToken(r.Context(), creds, deviceName, h.deviceTokenTTL())
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, errorBody("unauthorized"))
		return
	}
	writeJSON(w, http.StatusOK, okBody(map[string]any{
		"token": token, // the JWT — store it
		"device": map[string]any{
			"id":        dev.ID,
			"name":      dev.Name,
			"expiresAt": dev.ExpiresAt,
		},
	}))
}

// handleDevices lists the caller's active device sessions.
//
// @Summary      List devices
// @Description  Lists the caller's active device sessions (one per issued JWT), with last-seen time, IP and user agent.
// @Tags         devices
// @Produce      json
// @Param        u  query  string  true   "Username (or Bearer token)"
// @Param        p  query  string  false  "Password"
// @Param        c  query  string  true   "Client name"
// @Success      200  {object}  DevicesResponse
// @Router       /devices [get]
func (h *Handler) handleDevices(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	devices, err := h.Auth.ListDevices(r.Context(), user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
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
		})
	}
	writeJSON(w, http.StatusOK, okBody(map[string]any{"devices": out}))
}

// handleRevokeDevice revokes a device session (its JWT stops working).
//
// @Summary      Revoke a device
// @Description  Revokes a device session by id — the associated JWT can no longer authenticate.
// @Tags         devices
// @Produce      json
// @Param        u   query  string  true   "Username (or Bearer token)"
// @Param        p   query  string  false  "Password"
// @Param        c   query  string  true   "Client name"
// @Param        id  query  string  true   "Device id (jti) to revoke"
// @Success      200  {object}  OKResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /devices/revoke [post]
func (h *Handler) handleRevokeDevice(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	ok, err := h.Auth.RevokeDevice(r.Context(), r.Form.Get("id"), user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, errorBody("device not found"))
		return
	}
	writeJSON(w, http.StatusOK, okBody(nil))
}
