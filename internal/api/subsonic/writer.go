package subsonic

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
)

// Subsonic error codes.
const (
	ErrGeneric            = 0
	ErrMissingParameter   = 10
	ErrClientTooOld       = 20
	ErrServerTooOld       = 30
	ErrWrongCredentials   = 40
	ErrTokenUnsupported   = 41
	ErrUnauthorizedAction = 50
	ErrTrialExpired       = 60
	ErrDataNotFound       = 70
)

// jsonEnvelope wraps a response under the "subsonic-response" key for JSON.
type jsonEnvelope struct {
	Response *Response `json:"subsonic-response"`
}

// write renders resp in the format requested by the client (f parameter):
// "xml" (default), "json", or "jsonp" (requires callback).
func write(w http.ResponseWriter, r *http.Request, resp *Response) {
	format := r.URL.Query().Get("f")
	switch format {
	case "json":
		writeJSON(w, resp, "")
	case "jsonp":
		callback := r.URL.Query().Get("callback")
		if callback == "" {
			callback = "callback"
		}
		writeJSON(w, resp, callback)
	default:
		writeXML(w, resp)
	}
}

func writeXML(w http.ResponseWriter, resp *Response) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	_ = enc.Encode(resp)
}

func writeJSON(w http.ResponseWriter, resp *Response, callback string) {
	body, err := json.Marshal(jsonEnvelope{Response: resp})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if callback != "" {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "%s(%s);", callback, body)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// writeError renders a Subsonic error response.
func writeError(w http.ResponseWriter, r *http.Request, code int, message string) {
	write(w, r, errorResponse(code, message))
}

// writeOK renders a bare success response.
func writeOK(w http.ResponseWriter, r *http.Request) {
	write(w, r, newResponse())
}
