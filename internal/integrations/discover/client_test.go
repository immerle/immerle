package discover

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/immerle/immerle/internal/models"
)

func TestClientRegistersOnceThenAuthenticatesAndDropsRevokedKey(t *testing.T) {
	registrations, revoked := 0, false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/instances":
			registrations++
			_ = json.NewEncoder(w).Encode(map[string]any{"instance": map[string]string{"id": "inst"}, "privateKey": "key"})
		case "/api/v1/scrobbles":
			if revoked || r.Header.Get("X-Instance-ID") != "inst" || r.Header.Get("Authorization") != "Bearer key" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			var body struct {
				Listener string `json:"listener"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Listener == "" || body.Listener == "user-1" {
				t.Errorf("listener must be an opaque reference, got %q", body.Listener)
			}
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	}))
	defer srv.Close()

	settings := models.DiscoverRuntime{Enabled: true, Scrobble: true}
	c := New(srv.URL, srv.Client(), func() models.DiscoverRuntime { return settings },
		func(cr Credentials) error {
			settings.InstanceID, settings.PrivateKey = cr.InstanceID, cr.PrivateKey
			return nil
		},
		func() string { return "secret" })
	ctx := context.Background()
	play := []Play{{Artist: "A", Title: "T", PlayedAt: "2026-10-20T12:00:00Z"}}

	for range 2 {
		if err := c.Scrobble(ctx, "user-1", play); err != nil {
			t.Fatal(err)
		}
	}
	if registrations != 1 || settings.PrivateKey != "key" {
		t.Fatalf("registrations = %d, key = %q; want one registration, persisted", registrations, settings.PrivateKey)
	}

	revoked = true
	if err := c.Scrobble(ctx, "user-1", play); err == nil {
		t.Fatal("expected an error on 401")
	}
	if settings.InstanceID != "" || settings.PrivateKey != "" {
		t.Fatal("a 401 must drop the credentials so the next call registers again")
	}

	settings.Enabled = false
	if err := c.Scrobble(ctx, "user-1", play); err != ErrDisabled {
		t.Fatalf("err = %v, want ErrDisabled", err)
	}
}

func TestForgetListenerDeletesEvenWhenDisabled(t *testing.T) {
	var deleted []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.Header.Get("Authorization") != "Bearer key" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		deleted = append(deleted, r.URL.Path)
		_, _ = w.Write([]byte(`{"ok":true,"deleted":true}`))
	}))
	defer srv.Close()

	settings := models.DiscoverRuntime{}
	c := New(srv.URL, srv.Client(), func() models.DiscoverRuntime { return settings },
		func(Credentials) error { return nil }, func() string { return "secret" })
	ctx := context.Background()

	if err := c.ForgetListener(ctx, "user-1"); err != nil || len(deleted) != 0 {
		t.Fatalf("never registered: err = %v, calls = %v; want nothing sent", err, deleted)
	}
	settings.InstanceID, settings.PrivateKey = "inst", "key" // registered, then switched off
	if err := c.ForgetListener(ctx, "user-1"); err != nil {
		t.Fatal(err)
	}
	if want := "/api/v1/listeners/" + c.Listener("user-1"); len(deleted) != 1 || deleted[0] != want {
		t.Fatalf("calls = %v, want [%s]", deleted, want)
	}
}
