package immerle

import (
	"net/http"
	"testing"
)

func TestThemeDefaultsEmpty(t *testing.T) {
	srv, _ := newEnv(t)
	alice := login(t, srv, "alice")
	status, theme := doMap(t, srv, http.MethodGet, "/theme", alice, nil)
	if status != http.StatusOK {
		t.Fatalf("theme read failed: status %d %+v", status, theme)
	}
	if ac := theme["accentColor"]; ac != nil && ac != "" {
		t.Fatalf("expected empty accent colour by default, got %v", ac)
	}
}

func TestThemeSetAndPersist(t *testing.T) {
	srv, _ := newEnv(t)
	alice := login(t, srv, "alice")
	bob := login(t, srv, "bob")

	status, theme := doMap(t, srv, http.MethodPatch, "/theme", alice, map[string]any{"accentColor": "#3b82f6"})
	if status != http.StatusOK || theme["accentColor"] != "#3b82f6" {
		t.Fatalf("set accent failed: status %d %+v", status, theme)
	}

	// Persisted across a fresh read.
	_, theme = doMap(t, srv, http.MethodGet, "/theme", alice, nil)
	if theme["accentColor"] != "#3b82f6" {
		t.Fatalf("accent not persisted: %+v", theme)
	}

	// Scoped per user: bob is unaffected.
	_, theme = doMap(t, srv, http.MethodGet, "/theme", bob, nil)
	if ac := theme["accentColor"]; ac != nil && ac != "" {
		t.Fatalf("bob's theme should be independent, got %v", ac)
	}
}

func TestThemeRejectsInvalidColor(t *testing.T) {
	srv, _ := newEnv(t)
	alice := login(t, srv, "alice")
	status, body := doMap(t, srv, http.MethodPatch, "/theme", alice, map[string]any{"accentColor": "blue"})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid colour should be 400, got %d", status)
	}
	errObj, _ := body["error"].(map[string]any)
	if errObj == nil || errObj["code"] != "validation" {
		t.Fatalf("expected validation error, got %+v", body)
	}
}

func TestThemeClearsAccent(t *testing.T) {
	srv, _ := newEnv(t)
	alice := login(t, srv, "alice")

	doStatus(t, srv, http.MethodPatch, "/theme", alice, map[string]any{"accentColor": "#abc"})

	// Empty string clears it.
	_, theme := doMap(t, srv, http.MethodPatch, "/theme", alice, map[string]any{"accentColor": ""})
	if ac := theme["accentColor"]; ac != nil && ac != "" {
		t.Fatalf("accent should be cleared, got %v", ac)
	}
}
