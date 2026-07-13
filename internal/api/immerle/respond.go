package immerle

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"

	chi "github.com/go-chi/chi/v5"
)

// This file holds the REST response/request helpers shared by every handler.
// Responses carry the resource directly (no envelope); errors use a single
// {"error":{code,message}} shape and a native HTTP status.

// fieldError is a single field validation failure.
type fieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// apiError describes a single failure (code, message and optional per-field
// validation details). It is the value nested under the "error" key.
type apiError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Params  map[string]any `json:"params"` // i18n interpolation values; always present ({} when none)
	Fields  []fieldError   `json:"fields,omitempty"`
}

// errorResponse is the wire envelope for every non-2xx response: the apiError is
// nested under an "error" key, e.g. {"error":{"code":"not_found",...}}.
type errorResponse struct {
	Error apiError `json:"error"`
}

// writeResource writes v as JSON with the given status. A nil v (e.g. 204) sends
// no body.
func writeResource(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// writeError sends {"error":{code,message,params}} with the given status.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeErrorParams(w, status, code, message, nil)
}

// writeErrorParams is writeError with i18n interpolation params (e.g. the
// offending username or an error detail) the frontend fills into the translated
// message keyed by code. A nil map serializes as an empty object.
func writeErrorParams(w http.ResponseWriter, status int, code, message string, params map[string]any) {
	if params == nil {
		params = map[string]any{}
	}
	writeResource(w, status, errorResponse{Error: apiError{Code: code, Message: message, Params: params}})
}

// writeInternal reports a 500 from an unexpected error. The real error is logged
// server-side and never returned to the client: it can carry SQL, filesystem
// paths or other internal detail that must not leak over the API.
func writeInternal(w http.ResponseWriter, err error) {
	slog.Error("internal server error", "error", err)
	writeError(w, http.StatusInternalServerError, "internal", "internal server error")
}

// writeValidation sends a 400 with per-field details.
func writeValidation(w http.ResponseWriter, fields []fieldError) {
	writeResource(w, http.StatusBadRequest, errorResponse{
		Error: apiError{Code: "validation", Message: "validation failed", Params: map[string]any{}, Fields: fields},
	})
}

// decodeJSON reads a (size-capped) JSON request body into dst. On failure it
// writes a 400 and returns false. An empty body decodes to the zero value.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		// Empty body is allowed for partial updates / no-op bodies.
		if err.Error() == "EOF" {
			return true
		}
		writeErrorParams(w, http.StatusBadRequest, "invalid_body", "invalid JSON body: "+err.Error(), map[string]any{"detail": err.Error()})
		return false
	}
	return true
}

// pathParam returns a URL path parameter (e.g. {id}). chi hands back the raw
// (still percent-encoded) path segment, so we unescape it: remote ids like
// "rart:..."/"rcov:..." reach us as "rart%3A..." and would otherwise miss their
// prefix checks. Falls back to the raw value if it isn't valid escaping.
func pathParam(r *http.Request, name string) string {
	v := chi.URLParam(r, name)
	if dec, err := url.PathUnescape(v); err == nil {
		return dec
	}
	return v
}
