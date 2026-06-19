package providers

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Config is the unified provider configuration schema, shared by built-in and
// custom HTTP providers. It is content-neutral:
//
//   - Header: static HTTP headers added to every upstream request (e.g. auth).
//   - Params: static query parameters appended to every upstream request
//     (?key=value); there is never a request body.
//
// The scalar knobs (Quality/TimeoutSeconds/DownloadRetries) tune HTTP providers.
// Built-in providers read their own tunables from Params by name, with defaults.
type Config struct {
	Header          map[string]string `json:"header,omitempty"`
	Params          map[string]string `json:"params,omitempty"`
	Quality         string            `json:"quality,omitempty"`
	TimeoutSeconds  int               `json:"timeoutSeconds,omitempty"`
	DownloadRetries int               `json:"downloadRetries,omitempty"`
}

// configAux mirrors Config but also accepts the legacy "headers" key so that
// custom providers configured before the schema change keep authenticating.
type configAux struct {
	Header          map[string]string `json:"header"`
	HeadersLegacy   map[string]string `json:"headers"`
	Params          map[string]string `json:"params"`
	Quality         string            `json:"quality"`
	TimeoutSeconds  int               `json:"timeoutSeconds"`
	DownloadRetries int               `json:"downloadRetries"`
}

// ParseConfig decodes a provider config JSON payload. "" and "{}" yield a zero
// Config. Unknown top-level keys are rejected so typos (and the old flat
// built-in keys) surface as an error rather than being silently ignored.
func ParseConfig(s string) (Config, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Config{}, nil
	}
	dec := json.NewDecoder(strings.NewReader(s))
	dec.DisallowUnknownFields()
	var aux configAux
	if err := dec.Decode(&aux); err != nil {
		return Config{}, fmt.Errorf("invalid provider config: %w", err)
	}
	header := aux.Header
	if header == nil {
		header = aux.HeadersLegacy // legacy "headers" alias
	}
	return Config{
		Header:          header,
		Params:          aux.Params,
		Quality:         aux.Quality,
		TimeoutSeconds:  aux.TimeoutSeconds,
		DownloadRetries: aux.DownloadRetries,
	}, nil
}

// Param returns Params[key] or def when missing/empty.
func (c Config) Param(key, def string) string {
	if v, ok := c.Params[key]; ok && v != "" {
		return v
	}
	return def
}
