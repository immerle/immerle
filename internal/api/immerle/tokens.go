package immerle

import (
	"net/http"
	"strconv"
	"time"
)

// handleTokens lists the caller's active API tokens (secrets are never returned).
//
// @Summary      List API tokens
// @Description  Lists the caller's active personal access tokens (no secrets).
// @Tags         tokens
// @Produce      json
// @Param        u  query  string  true   "Subsonic username (or use a Bearer token)"
// @Param        p  query  string  false  "Subsonic password"
// @Param        c  query  string  true   "Client name"
// @Success      200  {object}  TokensResponse
// @Router       /tokens [get]
func (h *Handler) handleTokens(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	tokens, err := h.Auth.ListAPITokens(r.Context(), user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
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
		})
	}
	writeJSON(w, http.StatusOK, okBody(map[string]any{"tokens": out}))
}

// handleCreateToken mints a new personal access token for the caller. The secret
// is returned exactly once.
//
// @Summary      Create an API token
// @Description  Creates a personal access token scoped to the caller. The secret is returned ONCE — store it now. Use it as "Authorization: Bearer <token>" or "?apiKey=<token>".
// @Tags         tokens
// @Produce      json
// @Param        u        query  string  true   "Subsonic username"
// @Param        p        query  string  false  "Subsonic password"
// @Param        c        query  string  true   "Client name"
// @Param        name     query  string  false  "Label for the token"
// @Param        expires  query  int     false  "Expiry as unix epoch millis (0 = never)"
// @Success      201  {object}  CreateTokenResponse
// @Router       /tokens/create [post]
func (h *Handler) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	var expires *time.Time
	if ms := r.Form.Get("expires"); ms != "" {
		if n, err := time.Parse(time.RFC3339, ms); err == nil {
			expires = &n
		} else if epoch, _ := strconv.ParseInt(ms, 10, 64); epoch > 0 {
			t := time.UnixMilli(epoch)
			expires = &t
		}
	}
	secret, tok, err := h.Auth.CreateAPIToken(r.Context(), user.ID, r.Form.Get("name"), expires)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusCreated, okBody(map[string]any{
		"token":  secret, // shown once
		"id":     tok.ID,
		"name":   tok.Name,
		"prefix": tok.Prefix,
	}))
}

// handleRevokeToken revokes one of the caller's tokens.
//
// @Summary      Revoke an API token
// @Tags         tokens
// @Produce      json
// @Param        u   query  string  true   "Subsonic username"
// @Param        p   query  string  false  "Subsonic password"
// @Param        c   query  string  true   "Client name"
// @Param        id  query  string  true   "Token id to revoke"
// @Success      200  {object}  OKResponse
// @Failure      404  {object}  ErrorResponse
// @Router       /tokens/revoke [post]
func (h *Handler) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	user := userFrom(r.Context())
	id := r.Form.Get("id")
	ok, err := h.Auth.RevokeAPIToken(r.Context(), id, user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorBody(err.Error()))
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, errorBody("token not found"))
		return
	}
	writeJSON(w, http.StatusOK, okBody(nil))
}
