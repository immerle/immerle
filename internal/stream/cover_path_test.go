package stream

import (
	"path/filepath"
	"testing"
)

func TestCoverFileRejectsTraversal(t *testing.T) {
	c := &CoverService{coversDir: "/data/covers"}

	bad := []string{
		"../../../../etc/passwd",
		"../secret",
		"sub/../../escape",
		"/etc/passwd",
		"nested/file", // covers are flat; subdirs are not legitimate
	}
	for _, id := range bad {
		if _, ok := c.coverFile(id); ok {
			t.Errorf("coverFile(%q) accepted, want rejected", id)
		}
	}

	good := "album-123"
	p, ok := c.coverFile(good)
	if !ok {
		t.Fatalf("coverFile(%q) rejected, want accepted", good)
	}
	if want := filepath.Join("/data/covers", good); p != want {
		t.Errorf("coverFile(%q) = %q, want %q", good, p, want)
	}
}
