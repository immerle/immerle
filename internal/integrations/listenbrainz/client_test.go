package listenbrainz

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSubmitListenRequestShape(t *testing.T) {
	var gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		var body submitListensRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		b, _ := json.Marshal(body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newClient(srv.URL, nil)
	err := c.SubmitListen(context.Background(), "my-token", Listen{
		ListenedAt: time.Unix(1700000000, 0),
		Artist:     "Daft Punk",
		Track:      "One More Time",
		Release:    "Discovery",
		DurationMs: 320000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Token my-token" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Token my-token")
	}
	want := `{"listen_type":"single","payload":[{"listened_at":1700000000,"track_metadata":{"artist_name":"Daft Punk","track_name":"One More Time","release_name":"Discovery","additional_info":{"duration_ms":320000}}}]}`
	if gotBody != want {
		t.Errorf("body = %s, want %s", gotBody, want)
	}
}

func TestSubmitListenRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := newClient(srv.URL, nil)
	err := c.SubmitListen(context.Background(), "t", Listen{Artist: "A", Track: "B"})
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}

func TestValidateToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Token good" {
			_ = json.NewEncoder(w).Encode(map[string]any{"valid": true, "user_name": "kilian"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"valid": false})
	}))
	defer srv.Close()

	c := newClient(srv.URL, nil)

	username, err := c.ValidateToken(context.Background(), "good")
	if err != nil {
		t.Fatal(err)
	}
	if username != "kilian" {
		t.Errorf("username = %q, want %q", username, "kilian")
	}

	_, err = c.ValidateToken(context.Background(), "bad")
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}
