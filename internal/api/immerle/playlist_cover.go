package immerle

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/immerle/immerle/internal/models"
)

// renderSize is the square pixel size of a generated playlist cover. The cover
// service resizes/caches smaller variants on demand, so one large master is enough.
const renderSize = 1000

// handlePlaylistCover replaces a playlist's cover with an uploaded image
// (multipart field "file"). Owner-only. Mirrors handleTrackCover; bypasses the
// typed client like all cover uploads.
func (h *Handler) handlePlaylistCover(w http.ResponseWriter, r *http.Request) {
	p, ok := h.coverTarget(w, r)
	if !ok {
		return
	}
	data, ok := readCoverUpload(w, r)
	if !ok {
		return
	}
	h.storePlaylistCover(w, r, p, data)
}

// coverSpec describes a generated cover: a solid or angled-gradient background
// (or an uploaded background image, sent alongside as multipart field "file"),
// with one positioned text block. Positions/size are fractions of the square.
type coverSpec struct {
	Color     string  `json:"color"`     // background, hex (#rrggbb)
	Color2    string  `json:"color2"`    // gradient end; empty = solid
	Angle     float64 `json:"angle"`     // gradient angle, degrees
	Text      string  `json:"text"`      // may contain \n for multiple lines
	TextColor string  `json:"textColor"` // hex
	FontSize  float64 `json:"fontSize"`  // fraction of the square (default 0.12)
	Align     string  `json:"align"`     // left|center|right (default center)
	Valign    string  `json:"valign"`    // top|middle|bottom (default middle)
}

// handlePlaylistCoverGenerate renders a cover from a JSON spec (multipart field
// "spec") plus an optional background image (field "file"). Owner-only.
func (h *Handler) handlePlaylistCoverGenerate(w http.ResponseWriter, r *http.Request) {
	p, ok := h.coverTarget(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxCoverBytes)
	if err := r.ParseMultipartForm(maxCoverBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "malformed multipart form")
		return
	}
	var spec coverSpec
	if err := json.Unmarshal([]byte(r.FormValue("spec")), &spec); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "invalid cover spec")
		return
	}

	var bg image.Image
	if file, _, err := r.FormFile("file"); err == nil {
		defer func() { _ = file.Close() }()
		if img, _, err := image.Decode(file); err == nil {
			bg = img
		}
	}

	data, err := renderCover(spec, bg)
	if err != nil {
		writeInternal(w, err)
		return
	}
	h.storePlaylistCover(w, r, p, data)
}

// coverTarget loads the playlist and enforces owner-only cover edits.
func (h *Handler) coverTarget(w http.ResponseWriter, r *http.Request) (models.Playlist, bool) {
	p, err := h.playlistSvc.CoverTarget(r.Context(), userFrom(r.Context()), pathParam(r, "id"))
	if err != nil {
		writeServiceError(w, err)
		return models.Playlist{}, false
	}
	return p, true
}

// storePlaylistCover writes the cover bytes under a fresh id, points the
// playlist at it, removes the previous custom cover, and returns the playlist.
func (h *Handler) storePlaylistCover(w http.ResponseWriter, r *http.Request, p models.Playlist, data []byte) {
	coverID := uuid.NewString()
	if err := os.MkdirAll(h.CoversDir, 0o755); err != nil {
		writeInternal(w, err)
		return
	}
	if err := os.WriteFile(coverPath(h.CoversDir, coverID), data, 0o644); err != nil {
		writeInternal(w, err)
		return
	}
	if err := h.playlistSvc.SaveCover(r.Context(), p.ID, coverID); err != nil {
		_ = os.Remove(coverPath(h.CoversDir, coverID))
		writeInternal(w, err)
		return
	}
	if p.CoverArt != "" {
		_ = os.Remove(coverPath(h.CoversDir, p.CoverArt))
	}
	d, err := h.playlistSvc.Get(r.Context(), userFrom(r.Context()), p.ID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeResource(w, http.StatusOK, detailToView(d))
}

// renderCover paints the background (image, gradient or solid) then the text,
// returning PNG bytes.
func renderCover(spec coverSpec, bg image.Image) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, renderSize, renderSize))
	switch {
	case bg != nil:
		draw.CatmullRom.Scale(img, img.Bounds(), bg, bg.Bounds(), draw.Over, nil)
	case spec.Color2 != "":
		fillGradient(img, parseHex(spec.Color, color.Black), parseHex(spec.Color2, color.Black), spec.Angle)
	default:
		draw.Draw(img, img.Bounds(), &image.Uniform{parseHex(spec.Color, color.Black)}, image.Point{}, draw.Src)
	}

	if strings.TrimSpace(spec.Text) != "" {
		if err := drawText(img, spec); err != nil {
			return nil, err
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// fillGradient paints a linear gradient from c1 to c2 across `angle` degrees.
func fillGradient(img *image.RGBA, c1, c2 color.Color, angle float64) {
	rad := angle * math.Pi / 180
	dx, dy := math.Cos(rad), math.Sin(rad)
	// Project the corners onto the gradient axis to get the value range.
	min, max := math.Inf(1), math.Inf(-1)
	for _, c := range [][2]float64{{0, 0}, {renderSize, 0}, {0, renderSize}, {renderSize, renderSize}} {
		v := c[0]*dx + c[1]*dy
		min, max = math.Min(min, v), math.Max(max, v)
	}
	span := max - min
	if span == 0 {
		span = 1
	}
	r1, g1, b1, _ := c1.RGBA()
	r2, g2, b2, _ := c2.RGBA()
	lerp := func(a, b uint32, t float64) uint8 { return uint8((float64(a)*(1-t) + float64(b)*t) / 256) }
	for y := 0; y < renderSize; y++ {
		for x := 0; x < renderSize; x++ {
			t := (float64(x)*dx + float64(y)*dy - min) / span
			img.SetRGBA(x, y, color.RGBA{lerp(r1, r2, t), lerp(g1, g2, t), lerp(b1, b2, t), 255})
		}
	}
}

// drawText renders the spec's text as a block of lines (one per "\n"), aligned
// to the chosen corner/edge/centre. The whole block is positioned as a unit, so
// multi-line text stays correctly centred.
func drawText(img *image.RGBA, spec coverSpec) error {
	frac := spec.FontSize
	if frac <= 0 {
		frac = 0.12
	}
	parsed, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return err
	}
	size := frac * renderSize
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return err
	}
	defer func() { _ = face.Close() }()

	d := &font.Drawer{Dst: img, Src: image.NewUniform(parseHex(spec.TextColor, color.White)), Face: face}
	lines := strings.Split(spec.Text, "\n")
	const margin = 0.06 * renderSize
	const inner = renderSize - 2*margin
	lineH := size * 1.25
	blockH := lineH * float64(len(lines))
	m := face.Metrics()
	ascent, descent := float64(m.Ascent.Round()), float64(m.Descent.Round())

	// Top of the text block.
	top := (renderSize - blockH) / 2 // middle
	switch spec.Valign {
	case "top":
		top = margin
	case "bottom":
		top = renderSize - margin - blockH
	}

	for i, line := range lines {
		w := float64(d.MeasureString(line).Round())
		x := margin + (inner-w)/2 // center
		switch spec.Align {
		case "left":
			x = margin
		case "right":
			x = margin + inner - w
		}
		// Centre each line vertically within its line slot.
		baseline := top + lineH*float64(i) + (lineH-ascent-descent)/2 + ascent
		d.Dot = fixed.Point26_6{X: fixed.I(int(x)), Y: fixed.I(int(baseline))}
		d.DrawString(line)
	}
	return nil
}

// parseHex turns "#rrggbb"/"#rgb" into a colour, falling back to def.
func parseHex(s string, def color.Color) color.Color {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil || len(s) != 6 {
		return def
	}
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}
}
