package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/immerle/immerle/internal/models"
)

// stubLDAPConfig is a fixed LDAPRuntime provider for tests.
type stubLDAPConfig models.LDAPRuntime

func (s stubLDAPConfig) LDAPConfig() models.LDAPRuntime { return models.LDAPRuntime(s) }

func TestLDAPAuthFailsClosed(t *testing.T) {
	cases := map[string]models.LDAPRuntime{
		"disabled":     {Enabled: false, URL: "ldap://127.0.0.1:1", BindDNTemplate: "uid=%s,dc=x"},
		"missing url":  {Enabled: true, BindDNTemplate: "uid=%s,dc=x"},
		"missing tmpl": {Enabled: true, URL: "ldap://127.0.0.1:1"},
		"unreachable":  {Enabled: true, URL: "ldap://127.0.0.1:1", BindDNTemplate: "uid=%s,dc=x"},
	}
	for name, cfg := range cases {
		l := &ldapAuth{cfg: stubLDAPConfig(cfg)}
		// a is nil: a disabled/failed path must return before touching the user
		// store, so it must never provision or panic.
		if _, err := l.authenticate(context.Background(), nil, "alice", "pw"); !errors.Is(err, ErrUnauthorized) {
			t.Errorf("%s: want ErrUnauthorized, got %v", name, err)
		}
	}
	// Empty password is rejected before any dial.
	l := &ldapAuth{cfg: stubLDAPConfig(models.LDAPRuntime{Enabled: true, URL: "ldap://127.0.0.1:1", BindDNTemplate: "uid=%s,dc=x"})}
	if _, err := l.authenticate(context.Background(), nil, "alice", ""); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("empty password: want ErrUnauthorized, got %v", err)
	}
}

func TestLDAPBindCache(t *testing.T) {
	l := &ldapAuth{}
	k := bindKey("alice", "pw")

	if l.cached(k) {
		t.Fatal("empty cache should miss")
	}
	l.store(k)
	if !l.cached(k) {
		t.Fatal("freshly stored bind should hit")
	}
	// A different password is a different key, never served from cache.
	if l.cached(bindKey("alice", "other")) {
		t.Fatal("changed password must not hit cache")
	}
	// Expired entries miss and are pruned.
	l.cache[k] = time.Now().Add(-time.Second)
	if l.cached(k) {
		t.Fatal("expired bind should miss")
	}
	if _, ok := l.cache[k]; ok {
		t.Fatal("expired bind should be pruned")
	}
}
