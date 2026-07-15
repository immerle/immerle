package models

import (
	"net/url"
	"testing"
)

func TestRemoteCoverIDRoundTrip(t *testing.T) {
	url := "https://example.com/cover.jpg"
	id := RemoteCoverID(url)
	if !IsRemoteCoverID(id) {
		t.Fatalf("IsRemoteCoverID(%q) = false, want true", id)
	}
	got, ok := DecodeRemoteCoverID(id)
	if !ok || got != url {
		t.Errorf("DecodeRemoteCoverID(%q) = (%q, %v), want (%q, true)", id, got, ok, url)
	}
}

func TestRemoteCoverIDEmptyURL(t *testing.T) {
	if id := RemoteCoverID(""); id != "" {
		t.Errorf("RemoteCoverID(\"\") = %q, want empty", id)
	}
}

func TestIsRemoteCoverIDRejectsLocalIDs(t *testing.T) {
	if IsRemoteCoverID("local-file-id") {
		t.Error("a local cover id should not be reported as remote")
	}
}

func TestDecodeRemoteCoverIDRejectsNonRemote(t *testing.T) {
	if _, ok := DecodeRemoteCoverID("local-file-id"); ok {
		t.Error("decoding a non-remote id should fail")
	}
}

func TestDecodeRemoteCoverIDRejectsMalformedBase64(t *testing.T) {
	if _, ok := DecodeRemoteCoverID(RemoteCoverPrefix + "not-valid-base64!!!"); ok {
		t.Error("decoding malformed base64 should fail")
	}
}

func TestGeneratorCoverIDRoundTrip(t *testing.T) {
	q := url.Values{"icon": {"1f30d"}, "title": {"charts.top50"}, "size": {"180"}, "locale": {"fr"}}
	id := GeneratorCoverID(q)
	if !IsGeneratorCoverID(id) {
		t.Fatalf("IsGeneratorCoverID(%q) = false, want true", id)
	}
	got, ok := DecodeGeneratorCoverID(id)
	if !ok {
		t.Fatalf("DecodeGeneratorCoverID(%q) failed", id)
	}
	if got.Get("icon") != "1f30d" || got.Get("title") != "charts.top50" {
		t.Errorf("DecodeGeneratorCoverID(%q) = %v, want icon/title preserved", id, got)
	}
	// size/locale aren't builder params — dropped from the id/cache key.
	if got.Get("size") != "" || got.Get("locale") != "" {
		t.Errorf("DecodeGeneratorCoverID(%q) = %v, want size/locale excluded", id, got)
	}
}

func TestDecodeGeneratorCoverIDRejectsNonGenerator(t *testing.T) {
	if _, ok := DecodeGeneratorCoverID("local-file-id"); ok {
		t.Error("decoding a non-generator id should fail")
	}
}
