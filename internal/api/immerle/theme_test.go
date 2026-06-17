package immerle

import (
	"net/http"
	"net/url"
	"testing"
)

func themeOf(body map[string]any) map[string]any {
	t, _ := body["theme"].(map[string]any)
	return t
}

func TestThemeDefaultsEmpty(t *testing.T) {
	srv, _ := newEnv(t)
	body := postForm(t, srv, "/theme", creds("alice")) // POST without fields = read
	if body["ok"] != true {
		t.Fatalf("theme read failed: %+v", body)
	}
	if ac := themeOf(body)["accentColor"]; ac != nil && ac != "" {
		t.Fatalf("expected empty accent colour by default, got %v", ac)
	}
}

func TestThemeSetAndPersist(t *testing.T) {
	srv, _ := newEnv(t)

	v := creds("alice")
	v.Set("accentColor", "#3b82f6")
	body := postForm(t, srv, "/theme", v)
	if themeOf(body)["accentColor"] != "#3b82f6" {
		t.Fatalf("set accent failed: %+v", body)
	}

	// Persisted across a fresh read.
	body = postForm(t, srv, "/theme", creds("alice"))
	if themeOf(body)["accentColor"] != "#3b82f6" {
		t.Fatalf("accent not persisted: %+v", body)
	}

	// Scoped per user: bob is unaffected.
	body = postForm(t, srv, "/theme", creds("bob"))
	if ac := themeOf(body)["accentColor"]; ac != nil && ac != "" {
		t.Fatalf("bob's theme should be independent, got %v", ac)
	}
}

func TestThemeRejectsInvalidColor(t *testing.T) {
	srv, _ := newEnv(t)
	v := creds("alice")
	v.Set("accentColor", "blue")
	resp, err := http.PostForm(srv.URL+"/theme", v)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid colour should be 400, got %d", resp.StatusCode)
	}
}

func TestThemeClearsAccent(t *testing.T) {
	srv, _ := newEnv(t)

	v := creds("alice")
	v.Set("accentColor", "#abc")
	postForm(t, srv, "/theme", v)

	// Empty string clears it.
	clear := url.Values{"u": {"alice"}, "p": {"alicepw"}, "c": {"test"}, "accentColor": {""}}
	body := postForm(t, srv, "/theme", clear)
	if ac := themeOf(body)["accentColor"]; ac != nil && ac != "" {
		t.Fatalf("accent should be cleared, got %v", ac)
	}
}
