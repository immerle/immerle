package immerle

import (
	"errors"
	"net/http"
	"strings"

	"github.com/immerle/immerle/internal/lastfm"
)

// lastFmConfigured reports whether the admin has enabled Last.fm scrobbling
// and set both the API key and shared secret.
func (h *Handler) lastFmConfigured() bool {
	if h.Settings == nil {
		return false
	}
	cfg := h.Settings.Get().LastFm
	return cfg.Enabled && cfg.APIKey != "" && cfg.APISecret != ""
}

func lastFmStatusView(username string) LastFmStatusDTO {
	return LastFmStatusDTO{Connected: username != "", Username: username}
}

// handleLastFmStatus reports the caller's Last.fm connection state.
//
// @Summary      Last.fm connection status
// @Description  Reports whether the caller has connected their Last.fm account.
// @Tags         users
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  LastFmStatusDTO
// @Failure      401  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /me/lastfm [get]
func (h *Handler) handleLastFmStatus(w http.ResponseWriter, r *http.Request) {
	user, err := h.Users.GetByID(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusOK, lastFmStatusView(user.LastFmUsername))
}

// lastFmConnectStartResponse carries the auth token and the URL the caller
// must open in a browser to approve it.
type lastFmConnectStartResponse struct {
	Token   string `json:"token"`
	AuthURL string `json:"authUrl"`
}

// handleLastFmConnectStart begins the Last.fm desktop-auth handshake: mints a
// fresh token and the URL to approve it.
//
// @Summary      Start connecting a Last.fm account
// @Description  Requests a fresh auth token and returns the URL the caller must open to approve it, the first step of Last.fm's desktop-auth handshake.
// @Tags         users
// @Security     BearerAuth
// @Produce      json
// @Success      200  {object}  lastFmConnectStartResponse
// @Failure      401  {object}  errorResponse
// @Failure      502  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /me/lastfm/connect/start [post]
func (h *Handler) handleLastFmConnectStart(w http.ResponseWriter, r *http.Request) {
	if h.LastFm == nil || !h.lastFmConfigured() {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Last.fm scrobbling is not configured")
		return
	}
	cfg := h.Settings.Get().LastFm
	token, err := h.LastFm.GetToken(r.Context(), cfg.APIKey, cfg.APISecret)
	if err != nil {
		writeError(w, http.StatusBadGateway, "lastfm_unreachable", "could not reach Last.fm: "+err.Error())
		return
	}
	writeResource(w, http.StatusOK, lastFmConnectStartResponse{Token: token, AuthURL: h.LastFm.AuthURL(cfg.APIKey, token)})
}

// lastFmConnectFinishRequest carries the token approved via the auth URL.
type lastFmConnectFinishRequest struct {
	Token string `json:"token"`
}

// handleLastFmConnectFinish completes the handshake: exchanges an approved
// token for a permanent session key and stores it on the caller's account.
//
// @Summary      Finish connecting a Last.fm account
// @Description  Exchanges a token approved via the auth URL for a permanent session key, and stores it on the caller's account. Returns 400 not_authorized if the token hasn't been approved yet.
// @Tags         users
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  lastFmConnectFinishRequest  true  "Approved auth token"
// @Success      200  {object}  LastFmStatusDTO
// @Failure      400  {object}  errorResponse
// @Failure      401  {object}  errorResponse
// @Failure      503  {object}  errorResponse
// @Router       /me/lastfm/connect/finish [post]
func (h *Handler) handleLastFmConnectFinish(w http.ResponseWriter, r *http.Request) {
	if h.LastFm == nil || !h.lastFmConfigured() {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Last.fm scrobbling is not configured")
		return
	}
	var req lastFmConnectFinishRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	token := strings.TrimSpace(req.Token)
	if token == "" {
		writeError(w, http.StatusBadRequest, "validation", "token is required")
		return
	}
	cfg := h.Settings.Get().LastFm
	sessionKey, username, err := h.LastFm.GetSession(r.Context(), cfg.APIKey, cfg.APISecret, token)
	if errors.Is(err, lastfm.ErrPending) {
		writeError(w, http.StatusBadRequest, "not_authorized", "this token hasn't been approved yet — open the auth page and try again")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "lastfm_unreachable", "could not reach Last.fm: "+err.Error())
		return
	}
	user, err := h.Users.GetByID(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	user.LastFmSessionKey = sessionKey
	user.LastFmUsername = username
	if err := h.Users.Update(r.Context(), user); err != nil {
		writeInternal(w, err)
		return
	}
	writeResource(w, http.StatusOK, lastFmStatusView(user.LastFmUsername))
}

// handleLastFmDisconnect removes the caller's Last.fm connection.
//
// @Summary      Disconnect Last.fm
// @Tags         users
// @Security     BearerAuth
// @Success      204  "disconnected"
// @Failure      401  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /me/lastfm [delete]
func (h *Handler) handleLastFmDisconnect(w http.ResponseWriter, r *http.Request) {
	user, err := h.Users.GetByID(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	user.LastFmSessionKey = ""
	user.LastFmUsername = ""
	if err := h.Users.Update(r.Context(), user); err != nil {
		writeInternal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
