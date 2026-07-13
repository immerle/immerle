// Package httputil holds small request helpers shared across the API handlers.
package httputil

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// ValidateFetchURL rejects rawURL unless it is an http(s) URL whose host resolves
// only to routable public addresses. It guards outbound fetches of user- or
// admin-supplied URLs (podcast feeds, provider endpoints, station logos) against
// SSRF into the server's own network. The returned errors are safe to surface to
// clients — they carry no resolved-address detail.
func ValidateFetchURL(ctx context.Context, rawURL string) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return errors.New("invalid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("URL scheme must be http or https")
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("URL has no host")
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return errors.New("could not resolve URL host")
	}
	for _, ip := range ips {
		if !isPublicIP(ip.IP) {
			return errors.New("URL resolves to a disallowed address")
		}
	}
	return nil
}

// isPublicIP reports whether ip is routable on the public internet, i.e. not a
// loopback, link-local, private, unspecified or multicast address.
func isPublicIP(ip net.IP) bool {
	return !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() &&
		!ip.IsMulticast() && !ip.IsUnspecified() && !ip.IsPrivate()
}

// ClientIP returns the best-effort client IP (first X-Forwarded-For hop, else
// the remote address without its port).
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// APITokenFromRequest extracts a personal API token / device JWT from the
// Authorization Bearer header or the apiKey parameter. r.ParseForm must have
// been called.
func APITokenFromRequest(r *http.Request) string {
	if h := r.Header.Get("Authorization"); len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return r.Form.Get("apiKey")
}
