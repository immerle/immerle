package immerle

import (
	"net/http"
	"time"
)

// handleTokens lists the caller's active API tokens (secrets are never returned).
//
// @Summary      List API tokens
// @Description  Lists the caller's active personal access tokens (no secrets).
// @Tags         tokens
// @Security     BearerAuth
// @Produce      json
// @Success      200  {array}  apiTokenView
// @Failure      401  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /tokens [get]
func (h *Handler) handleTokens(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	tokens, err := h.Auth.ListAPITokens(r.Context(), user.ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	out := make([]apiTokenView, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, apiTokenView{
			ID: t.ID, Name: t.Name, Prefix: t.Prefix, CreatedAt: t.CreatedAt,
			LastUsedAt: t.LastUsedAt, ExpiresAt: t.ExpiresAt, IsDevice: t.IsDevice,
			Connected: t.LastUsedAt != nil && time.Since(*t.LastUsedAt) < deviceOnlineWindow,
		})
	}
	writeResource(w, http.StatusOK, out)
}

// createTokenRequest is the body for POST /tokens.
type createTokenRequest struct {
	Name string `json:"name" validate:"optional"`
	// ExpiresAt is an optional RFC3339 timestamp; omit or null for a token that
	// never expires.
	ExpiresAt *time.Time `json:"expiresAt,omitempty" validate:"optional"`
	// Device marks this token as an app login session (one per installed
	// client) rather than a manually-created personal/CLI token — only
	// device tokens are offered as playback-transfer targets.
	Device bool `json:"device" validate:"optional"`
}

// handleCreateToken mints a personal access token; the secret is shown once.
//
// @Summary      Create an API token
// @Description  Creates a personal access token scoped to the caller. The secret is returned ONCE — store it now. Use it as "Authorization: Bearer <token>".
// @Tags         tokens
// @Security     BearerAuth
// @Accept       json
// @Produce      json
// @Param        body  body  createTokenRequest  true  "Token name and optional expiry"
// @Success      201  {object}  createdTokenView
// @Failure      400  {object}  errorResponse
// @Failure      401  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /tokens [post]
func (h *Handler) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	var req createTokenRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	secret, tok, err := h.Auth.CreateAPIToken(r.Context(), user.ID, req.Name, req.ExpiresAt, req.Device)
	if err != nil {
		writeInternal(w, err)
		return
	}
	h.Logger.Info("API token created", "user", user.Username, "name", tok.Name, "tokenId", tok.ID)
	writeResource(w, http.StatusCreated, createdTokenView{Token: secret, ID: tok.ID, Name: tok.Name, Prefix: tok.Prefix})
}

// apiTokenView is one of the caller's API tokens (never the secret).
type apiTokenView struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty" validate:"optional"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty" validate:"optional"`
	// IsDevice marks an app login session rather than a personal/CLI token.
	IsDevice bool `json:"isDevice"`
	// Connected is true if the token was used within the last few minutes.
	Connected bool `json:"connected"`
}

// createdTokenView is a freshly minted token: the only time the secret is shown.
type createdTokenView struct {
	Token  string `json:"token"`
	ID     string `json:"id"`
	Name   string `json:"name"`
	Prefix string `json:"prefix"`
}

// handleRevokeToken revokes one of the caller's tokens.
//
// @Summary      Revoke an API token
// @Tags         tokens
// @Security     BearerAuth
// @Param        id  path  string  true  "Token id to revoke"
// @Success      204  "revoked"
// @Failure      401  {object}  errorResponse
// @Failure      404  {object}  errorResponse
// @Failure      500  {object}  errorResponse
// @Router       /tokens/{id} [delete]
func (h *Handler) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	ok, err := h.Auth.RevokeAPIToken(r.Context(), pathParam(r, "id"), user.ID)
	if err != nil {
		writeInternal(w, err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "token not found")
		return
	}
	h.Logger.Info("API token revoked", "user", user.Username, "tokenId", pathParam(r, "id"))
	writeResource(w, http.StatusNoContent, nil)
}
