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
// @Success      200  {array}  APITokenDTO
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
	out := make([]map[string]any, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, map[string]any{
			"id":         t.ID,
			"name":       t.Name,
			"prefix":     t.Prefix,
			"createdAt":  t.CreatedAt,
			"lastUsedAt": t.LastUsedAt,
			"expiresAt":  t.ExpiresAt,
			"isDevice":   t.IsDevice,
			"connected":  t.LastUsedAt != nil && time.Since(*t.LastUsedAt) < deviceOnlineWindow,
		})
	}
	writeResource(w, http.StatusOK, out)
}

// createTokenRequest is the body for POST /tokens.
type createTokenRequest struct {
	Name string `json:"name"`
	// ExpiresAt is an optional RFC3339 timestamp; omit or null for a token that
	// never expires.
	ExpiresAt *time.Time `json:"expiresAt"`
	// Device marks this token as an app login session (one per installed
	// client) rather than a manually-created personal/CLI token — only
	// device tokens are offered as playback-transfer targets.
	Device bool `json:"device"`
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
// @Success      201  {object}  CreateTokenDTO
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
	writeResource(w, http.StatusCreated, map[string]any{
		"token":  secret, // shown once
		"id":     tok.ID,
		"name":   tok.Name,
		"prefix": tok.Prefix,
	})
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
