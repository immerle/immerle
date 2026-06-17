package stream

import "testing"

func TestSafeFFmpegArgs(t *testing.T) {
	if _, ok := safeFFmpegArgs("-c:a libopus -b:a 128k -vbr on"); !ok {
		t.Error("legitimate codec args rejected")
	}
	// Reject attempts to inject an extra input/output or overwrite flag.
	for _, bad := range []string{
		"-i /etc/passwd",
		"-c:a libopus -y /tmp/evil.mp3",
		"-f mp3 -filter_complex amovie=/etc/shadow",
	} {
		if _, ok := safeFFmpegArgs(bad); ok {
			t.Errorf("safeFFmpegArgs(%q) accepted, want rejected", bad)
		}
	}
}
