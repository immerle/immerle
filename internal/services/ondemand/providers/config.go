package providers

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Config is the unified provider configuration schema, shared by built-in and
// custom HTTP providers. It is content-neutral:
//
//   - Headers: static HTTP headers added to every upstream request (e.g. auth).
//   - Params: static query parameters appended to every upstream request
//     (?key=value); there is never a request body.
//
// The scalar knobs (Quality/TimeoutSeconds/DownloadRetries) tune HTTP providers.
// Built-in providers read their own tunables from Params by name, with defaults.
type Config struct {
	Headers         map[string]string `json:"headers,omitempty"`
	Params          map[string]string `json:"params,omitempty"`
	Quality         string            `json:"quality,omitempty"`
	TimeoutSeconds  int               `json:"timeoutSeconds,omitempty"`
	DownloadRetries int               `json:"downloadRetries,omitempty"`
}

// configAux mirrors Config but also accepts the legacy singular "header" key so
// that providers configured before it was pluralized keep authenticating.
type configAux struct {
	Headers         map[string]string `json:"headers"`
	HeaderLegacy    map[string]string `json:"header"`
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
	headers := aux.Headers
	if headers == nil {
		headers = aux.HeaderLegacy // legacy singular "header" alias
	}
	return Config{
		Headers:         headers,
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
