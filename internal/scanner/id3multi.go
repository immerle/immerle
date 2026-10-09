package scanner

import (
	"bytes"
	"encoding/binary"
	"io"
	"strings"
	"unicode/utf16"
)

// readID3MultiValues returns the separate values of the given ID3v2.3/2.4 text
// frames (e.g. TPE1 = ["Santana", "Juanes"]). The tag library joins them with no
// separator ("SantanaJuanes"), so the frames are re-read here. Only frames with
// more than one value are returned; anything unusual (v2.2, unsynchronisation,
// compressed or encrypted frames) is skipped and the tag library's value kept.
func readID3MultiValues(r io.ReadSeeker, ids ...string) map[string][]string {
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return nil
	}
	var h [10]byte
	if _, err := io.ReadFull(r, h[:]); err != nil || string(h[:3]) != "ID3" {
		return nil
	}
	version, flags := h[3], h[5]
	if (version != 3 && version != 4) || flags&0x80 != 0 { // tag-level unsynchronisation
		return nil
	}
	tag := make([]byte, syncsafe(h[6:10]))
	if _, err := io.ReadFull(r, tag); err != nil {
		return nil
	}
	if flags&0x40 != 0 && len(tag) >= 4 { // extended header
		size := int(binary.BigEndian.Uint32(tag[:4])) + 4
		if version == 4 {
			size = syncsafe(tag[:4])
		}
		if size > len(tag) {
			return nil
		}
		tag = tag[size:]
	}

	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	out := map[string][]string{}
	for len(tag) >= 10 && tag[0] != 0 {
		id := string(tag[:4])
		size := int(binary.BigEndian.Uint32(tag[4:8]))
		if version == 4 {
			size = syncsafe(tag[4:8])
		}
		formatFlags := tag[9]
		if size < 0 || 10+size > len(tag) {
			break
		}
		body := tag[10 : 10+size]
		tag = tag[10+size:]
		if !want[id] || len(body) < 2 || formatFlags&0x0f != 0 && version == 4 || formatFlags&0xc0 != 0 && version == 3 {
			continue
		}
		if vals := splitID3Text(body[0], body[1:]); len(vals) > 1 {
			out[id] = vals
		}
	}
	return out
}

func syncsafe(b []byte) int {
	return int(b[0])<<21 | int(b[1])<<14 | int(b[2])<<7 | int(b[3])
}

// splitID3Text decodes a text frame body and splits it on its NUL separators.
func splitID3Text(enc byte, b []byte) []string {
	var raw []string
	switch enc {
	case 0: // ISO-8859-1
		for _, part := range bytes.Split(b, []byte{0}) {
			r := make([]rune, len(part))
			for i, c := range part {
				r[i] = rune(c)
			}
			raw = append(raw, string(r))
		}
	case 3: // UTF-8
		raw = strings.Split(string(b), "\x00")
	case 1, 2: // UTF-16 with BOM per value / UTF-16BE
		for i := 0; i+1 < len(b); {
			j := i
			for j+1 < len(b) && (b[j] != 0 || b[j+1] != 0) {
				j += 2
			}
			raw = append(raw, decodeUTF16(b[i:j], enc == 1))
			i = j + 2
		}
	default:
		return nil
	}
	var vals []string
	for _, v := range raw {
		if v = strings.TrimSpace(v); v != "" {
			vals = append(vals, v)
		}
	}
	return vals
}

func decodeUTF16(b []byte, bom bool) string {
	order := binary.ByteOrder(binary.BigEndian)
	if bom && len(b) >= 2 {
		if b[0] == 0xff && b[1] == 0xfe {
			order = binary.LittleEndian
		}
		if (b[0] == 0xff && b[1] == 0xfe) || (b[0] == 0xfe && b[1] == 0xff) {
			b = b[2:]
		}
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = order.Uint16(b[2*i:])
	}
	return string(utf16.Decode(u))
}
