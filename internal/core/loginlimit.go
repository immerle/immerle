package core

import (
	"errors"
	"strings"
	"sync"
	"time"
)

// ErrTooManyAttempts is returned when a username or client IP has failed to
// log in too often recently. It wraps ErrUnauthorized so callers that only
// check for bad credentials keep rejecting the request.
var ErrTooManyAttempts = errors.Join(ErrUnauthorized, errors.New("too many failed login attempts"))

const (
	loginMaxFailures = 10
	loginWindow      = 15 * time.Minute
	// loginMaxEntries bounds the tracker's memory under a flood of distinct
	// usernames/IPs; expired entries are swept once it is exceeded.
	loginMaxEntries = 10000
)

// loginLimiter counts failed password logins per key (username and client IP)
// in a fixed window and blocks a key once it reaches loginMaxFailures. Both keys
// are tracked because the client IP comes from X-Forwarded-For and can be
// spoofed, while the username alone would let one IP spray many accounts.
type loginLimiter struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[string]*loginEntry
}

type loginEntry struct {
	failures int
	start    time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{now: time.Now, entries: map[string]*loginEntry{}}
}

func loginKeys(username, ip string) []string {
	keys := []string{"u:" + strings.ToLower(username)}
	if ip != "" {
		keys = append(keys, "ip:"+ip)
	}
	return keys
}

// blocked reports whether any of the keys has exhausted its failure budget.
func (l *loginLimiter) blocked(keys []string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	for _, k := range keys {
		if e, ok := l.entries[k]; ok && now.Sub(e.start) < loginWindow && e.failures >= loginMaxFailures {
			return true
		}
	}
	return false
}

func (l *loginLimiter) fail(keys []string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.entries) >= loginMaxEntries {
		for k, e := range l.entries {
			if now.Sub(e.start) >= loginWindow {
				delete(l.entries, k)
			}
		}
	}
	for _, k := range keys {
		e, ok := l.entries[k]
		if !ok || now.Sub(e.start) >= loginWindow {
			e = &loginEntry{start: now}
			l.entries[k] = e
		}
		e.failures++
	}
}

// succeed clears the username's failures. The IP's are kept, so one valid
// account cannot be used to reset the budget while spraying others.
func (l *loginLimiter) succeed(username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, "u:"+strings.ToLower(username))
}
