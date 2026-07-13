package models

import "testing"

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
