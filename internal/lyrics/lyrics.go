// Package lyrics parses stored lyrics text (plain, or LRC with [mm:ss.xx]
// timestamps) into a structured document shared by the Subsonic and native
// REST APIs, so both expose lyrics through one implementation.
package lyrics

import (
	"regexp"
	"strconv"
	"strings"
)

// Line is one lyrics line. StartMs is the playback offset in milliseconds for a
// synced line; it is 0 in an unsynced document.
type Line struct {
	StartMs int64
	Text    string
}

// Document is the parsed lyrics for one track.
type Document struct {
	Synced bool
	Lines  []Line
}

// lrcLine matches a leading "[mm:ss.xx]" (or "[mm:ss]") synced-lyrics timestamp.
var lrcLine = regexp.MustCompile(`^\[(\d+):(\d{2})(?:[.:](\d{1,3}))?\]`)

// Parse turns raw lyrics text into a Document. If any line carries an
// [mm:ss.xx] timestamp the document is marked synced and each StartMs is the
// offset in milliseconds; otherwise lines are returned unsynced. Empty input
// yields an empty document (no lines).
func Parse(raw string) Document {
	if strings.TrimSpace(raw) == "" {
		return Document{}
	}
	var doc Document
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		m := lrcLine.FindStringSubmatch(line)
		if m == nil {
			doc.Lines = append(doc.Lines, Line{Text: line})
			continue
		}
		doc.Synced = true
		min, _ := strconv.Atoi(m[1])
		sec, _ := strconv.Atoi(m[2])
		ms := 0
		if m[3] != "" {
			ms, _ = strconv.Atoi((m[3] + "000")[:3]) // pad fraction to milliseconds
		}
		doc.Lines = append(doc.Lines, Line{
			StartMs: int64(min*60_000 + sec*1_000 + ms),
			Text:    strings.TrimSpace(lrcLine.ReplaceAllString(line, "")),
		})
	}
	return doc
}
