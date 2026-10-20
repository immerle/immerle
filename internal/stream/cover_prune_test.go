package stream

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/immerle/immerle/internal/models"
)

func TestPruneRemoteDeletesOldUnwantedRemoteCovers(t *testing.T) {
	c := NewCoverService(nil, t.TempDir())
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// "???" encodes to base64url containing "_", the ambiguous case.
	drop := models.RemoteCoverID("https://i.scdn.co/image/???")
	keep := models.RemoteCoverID("https://i.scdn.co/image/kept")
	if !strings.Contains(drop, "_") {
		t.Fatalf("test id %q should contain an underscore", drop)
	}
	write := func(name string, age time.Duration) {
		t.Helper()
		p := filepath.Join(c.cacheDir, name)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		at := time.Now().Add(-age)
		if err := os.Chtimes(p, at, at); err != nil {
			t.Fatal(err)
		}
	}
	b64 := func(id string) string { return strings.TrimPrefix(id, models.RemoteCoverPrefix) }
	old := 8 * 24 * time.Hour
	write("remote_"+b64(drop), old)
	write("rcov_"+b64(drop)+"_300.jpg", old)
	write("remote_"+b64(keep), old)                 // prune says no
	write("rcov_"+b64(drop)+"_64.jpg", time.Minute) // too recent
	write("album123_300.jpg", old)                  // not a remote cover

	var asked []string
	n := c.PruneRemote(time.Now().Add(-7*24*time.Hour), func(id string) bool {
		asked = append(asked, id)
		return id == drop
	})
	if n != 2 {
		t.Fatalf("removed %d files, want 2 (asked about %v)", n, asked)
	}
	left, _ := os.ReadDir(c.cacheDir)
	if len(left) != 3 {
		t.Fatalf("%d files left, want 3", len(left))
	}
}
