package lastfm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSign checks the signing algorithm against a hand-computed md5 vector
// (sorted key+value concatenation + secret), independent of this package's
// own use of crypto/md5.
func TestSign(t *testing.T) {
	got := sign(map[string]string{
		"method":  "auth.getSession",
		"api_key": "ak",
		"token":   "tok",
	}, "secret123")
	want := "b961677f59467e4f0ad52bde5ac59211"
	if got != want {
		t.Fatalf("sign() = %q, want %q", got, want)
	}
}

func TestGetToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("method") != "auth.getToken" {
			t.Errorf("method = %q, want auth.getToken", r.URL.Query().Get("method"))
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "abc123"})
	}))
	defer srv.Close()

	c := newClient(srv.URL, nil)
	token, err := c.GetToken(context.Background(), "key", "secret")
	if err != nil {
		t.Fatalf("GetToken() error = %v", err)
	}
	if token != "abc123" {
		t.Errorf("token = %q, want abc123", token)
	}
}

func TestGetSessionPending(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"error": 14, "message": "not authorized"})
	}))
	defer srv.Close()

	c := newClient(srv.URL, nil)
	_, _, err := c.GetSession(context.Background(), "key", "secret", "tok")
	if !errors.Is(err, ErrPending) {
		t.Fatalf("err = %v, want ErrPending", err)
	}
}

func TestGetSessionSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"session": map[string]string{"name": "bob", "key": "sk-123"},
		})
	}))
	defer srv.Close()

	c := newClient(srv.URL, nil)
	sk, username, err := c.GetSession(context.Background(), "key", "secret", "tok")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if sk != "sk-123" || username != "bob" {
		t.Errorf("got (%q, %q), want (sk-123, bob)", sk, username)
	}
}

func TestScrobbleRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"error": 29, "message": "rate limit exceeded"})
	}))
	defer srv.Close()

	c := newClient(srv.URL, nil)
	err := c.Scrobble(context.Background(), "key", "secret", "sk-123", Listen{Artist: "A", Track: "T"})
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
}
