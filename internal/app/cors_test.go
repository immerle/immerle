package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func corsHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/x", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	return corsMiddleware(func() []string { return []string{"http://localhost:8081", "http://127.0.0.1:8081"} }, mux)
}

func TestCORSAllowsConfiguredOrigin(t *testing.T) {
	srv := httptest.NewServer(corsHandler())
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/x", nil)
	req.Header.Set("Origin", "http://localhost:8081")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:8081" {
		t.Fatalf("expected allowed origin echoed, got %q", got)
	}
	if got := resp.Header.Get("Access-Control-Expose-Headers"); got == "" {
		t.Fatal("expected Expose-Headers for range streaming")
	}
}

func TestCORSRejectsUnknownOrigin(t *testing.T) {
	srv := httptest.NewServer(corsHandler())
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/x", nil)
	req.Header.Set("Origin", "http://evil.example")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unknown origin must not be allowed, got %q", got)
	}
}

func TestCORSPreflightShortCircuits(t *testing.T) {
	srv := httptest.NewServer(corsHandler())
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodOptions, srv.URL+"/x", nil)
	req.Header.Set("Origin", "http://localhost:8081")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 for preflight, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Methods"); got == "" {
		t.Fatal("preflight should advertise allowed methods")
	}
	if got := resp.Header.Get("Access-Control-Allow-Headers"); got != "Content-Type" {
		t.Fatalf("preflight should echo requested headers, got %q", got)
	}
}

func TestCORSWildcard(t *testing.T) {
	srv := httptest.NewServer(corsMiddleware(func() []string { return []string{"*"} }, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/x", nil)
	req.Header.Set("Origin", "http://anything.example")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("wildcard should allow any origin, got %q", got)
	}
}
