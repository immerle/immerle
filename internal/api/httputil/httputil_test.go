package httputil

import (
	"context"
	"net"
	"net/http"
	"testing"
)

func TestValidateFetchURL(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		wantErr bool
	}{
		{"malformed", "http://[::1", true},
		{"bad scheme", "ftp://8.8.8.8/file", true},
		{"empty host", "http:///path", true},
		{"loopback", "http://127.0.0.1/", true},
		{"private", "http://10.0.0.5/", true},
		{"unresolvable", "http://this-host-does-not-exist.invalid/", true},
		{"public IP literal", "http://8.8.8.8/", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFetchURL(context.Background(), tt.rawURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFetchURL(%q) error = %v, wantErr %v", tt.rawURL, err, tt.wantErr)
			}
		})
	}
}

func TestIsPublicIP(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"8.8.8.8", true},
		{"127.0.0.1", false},
		{"10.0.0.1", false},
		{"169.254.1.1", false},
		{"224.0.0.1", false},
		{"0.0.0.0", false},
	}
	for _, tt := range tests {
		ip := net.ParseIP(tt.ip)
		if got := isPublicIP(ip); got != tt.want {
			t.Errorf("isPublicIP(%s) = %v, want %v", tt.ip, got, tt.want)
		}
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		xff        string
		remoteAddr string
		want       string
	}{
		{"single XFF", "203.0.113.9", "10.0.0.1:1234", "203.0.113.9"},
		{"multi-hop XFF takes first", "203.0.113.9, 10.0.0.2", "10.0.0.1:1234", "203.0.113.9"},
		{"no XFF, host:port", "", "203.0.113.9:1234", "203.0.113.9"},
		{"no XFF, no port", "", "203.0.113.9", "203.0.113.9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{Header: http.Header{}, RemoteAddr: tt.remoteAddr}
			if tt.xff != "" {
				r.Header.Set("X-Forwarded-For", tt.xff)
			}
			if got := ClientIP(r); got != tt.want {
				t.Errorf("ClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAPITokenFromRequest(t *testing.T) {
	t.Run("bearer header", func(t *testing.T) {
		r := &http.Request{Header: http.Header{"Authorization": []string{"Bearer secret-token"}}}
		if got := APITokenFromRequest(r); got != "secret-token" {
			t.Errorf("got %q, want %q", got, "secret-token")
		}
	})

	t.Run("bearer header case-insensitive", func(t *testing.T) {
		r := &http.Request{Header: http.Header{"Authorization": []string{"bearer secret-token"}}}
		if got := APITokenFromRequest(r); got != "secret-token" {
			t.Errorf("got %q, want %q", got, "secret-token")
		}
	})

	t.Run("apiKey form param fallback", func(t *testing.T) {
		r, err := http.NewRequest(http.MethodGet, "http://example.com/?apiKey=form-token", nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := APITokenFromRequest(r); got != "form-token" {
			t.Errorf("got %q, want %q", got, "form-token")
		}
	})
}
