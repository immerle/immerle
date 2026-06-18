package immerle

import (
	"encoding/json"
	"net/http"

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

// writeInternal reports a 500 from an unexpected error.
func writeInternal(w http.ResponseWriter, err error) {
	writeErrorParams(w, http.StatusInternalServerError, "internal", err.Error(), map[string]any{"detail": err.Error()})
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

// pathParam returns a URL path parameter (e.g. {id}).
func pathParam(r *http.Request, name string) string {
	return chi.URLParam(r, name)
}
