package core

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/go-ldap/ldap/v3"

	"github.com/immerle/immerle/internal/models"
	"github.com/immerle/immerle/internal/persistence"
)

// ldapBindTTL caps how long a successful LDAP bind is trusted without re-binding.
// Subsonic clients (password mode) send credentials on every request, so without
// this each API call would hit the directory; 5 minutes bounds the staleness of
// a password change against that load.
const ldapBindTTL = 5 * time.Minute

// ldapCacheMax bounds the bind cache so a stream of distinct username+password
// attempts (including failed guesses that reach a successful bind) can't grow the
// map without limit. When full, store sweeps expired entries and, if still at
// capacity, evicts an arbitrary (map-random) live one.
const ldapCacheMax = 1024

// ldapConfig provides the live LDAP settings (read on each login so admin edits
// in the UI apply without a restart). Implemented by *SettingsService.
type ldapConfig interface {
	LDAPConfig() models.LDAPRuntime
}

// ldapAuth authenticates users via a direct LDAP simple bind: it binds as the
// user with a DN built from a template (no service account / search). On the
// first successful bind the user is provisioned locally, because FK relations
// (playlists, scrobbles, devices) need a users row.
//
// ponytail: simple-bind only. Add a search-then-bind path if your directory
// can't express the bind DN as a single template.
type ldapAuth struct {
	cfg ldapConfig

	mu    sync.Mutex
	cache map[string]time.Time // sha256(user\x00password) -> bind expiry
}

// authenticate binds to LDAP as username/password and returns the local user,
// creating it on first successful login. Returns ErrUnauthorized when LDAP is
// disabled/misconfigured or the bind fails (fail closed). A successful bind is
// cached for ldapBindTTL so repeated Subsonic calls don't re-hit the directory.
func (l *ldapAuth) authenticate(ctx context.Context, a *AuthService, username, password string) (models.User, error) {
	c := l.cfg.LDAPConfig()
	// Enabled/config is checked before the cache so toggling LDAP off in the UI
	// takes effect immediately (cached binds are never served while disabled).
	if !c.Enabled || c.URL == "" || c.BindDNTemplate == "" || password == "" {
		return models.User{}, ErrUnauthorized
	}
	key := bindKey(username, password)
	if l.cached(key) {
		return a.users.GetByUsername(ctx, username)
	}

	conn, err := ldap.DialURL(c.URL)
	if err != nil {
		return models.User{}, ErrUnauthorized
	}
	defer func() { _ = conn.Close() }()
	dn := fmt.Sprintf(c.BindDNTemplate, ldap.EscapeDN(username))
	if err := conn.Bind(dn, password); err != nil {
		return models.User{}, ErrUnauthorized
	}
	// Bind succeeded: the directory vouched for the password. Reuse or create.
	u, err := a.users.GetByUsername(ctx, username)
	if errors.Is(err, persistence.ErrNotFound) {
		// JIT-provision with a random, unusable local password so the local
		// compare path can never authenticate this account.
		var b [32]byte
		if _, err := rand.Read(b[:]); err != nil {
			return models.User{}, err
		}
		u, err = a.CreateUser(ctx, username, hex.EncodeToString(b[:]), "", username, false)
	}
	if err != nil {
		return models.User{}, err
	}
	l.store(key)
	return u, nil
}

// bindKey hashes the credentials so plaintext passwords never sit in the cache
// map. A changed password yields a different key, so it can't be served stale.
func bindKey(username, password string) string {
	h := sha256.Sum256([]byte(username + "\x00" + password))
	return string(h[:])
}

// cached reports whether key has a live (non-expired) bind, pruning it if stale.
func (l *ldapAuth) cached(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	exp, ok := l.cache[key]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(l.cache, key)
		return false
	}
	return true
}

func (l *ldapAuth) store(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.cache == nil {
		l.cache = make(map[string]time.Time)
	}
	if len(l.cache) >= ldapCacheMax {
		now := time.Now()
		for k, exp := range l.cache {
			if now.After(exp) {
				delete(l.cache, k)
			}
		}
		for k := range l.cache {
			if len(l.cache) < ldapCacheMax {
				break
			}
			delete(l.cache, k)
		}
	}
	l.cache[key] = time.Now().Add(ldapBindTTL)
}
