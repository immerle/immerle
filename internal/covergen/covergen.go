// Package covergen renders square cover-art images — gradient/solid
// backgrounds, centered text, composited overlay images — shared by the
// user-facing playlist cover generator (internal/api/immerle) and the
// curated chart cover generator (internal/charts), so both draw from the
// same primitives instead of two divergent implementations.
package covergen

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"math"
	"strconv"
	"strings"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Size is the square pixel size of a generated cover. Readers resize/cache
// smaller variants on demand, so one large master is enough.
const Size = 1000

// NewCanvas returns a blank Size×Size RGBA image.
func NewCanvas() *image.RGBA {
	return image.NewRGBA(image.Rect(0, 0, Size, Size))
}

// FillSolid paints the whole canvas one color.
func FillSolid(img *image.RGBA, c color.Color) {
	draw.Draw(img, img.Bounds(), &image.Uniform{c}, image.Point{}, draw.Src)
}

// FillGradient paints a linear gradient from c1 to c2 across `angle` degrees.
func FillGradient(img *image.RGBA, c1, c2 color.Color, angle float64) {
	rad := angle * math.Pi / 180
	dx, dy := math.Cos(rad), math.Sin(rad)
	// Project the corners onto the gradient axis to get the value range.
	min, max := math.Inf(1), math.Inf(-1)
	for _, c := range [][2]float64{{0, 0}, {Size, 0}, {0, Size}, {Size, Size}} {
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
	for y := 0; y < Size; y++ {
		for x := 0; x < Size; x++ {
			t := (float64(x)*dx + float64(y)*dy - min) / span
			img.SetRGBA(x, y, color.RGBA{lerp(r1, r2, t), lerp(g1, g2, t), lerp(b1, b2, t), 255})
		}
	}
}

// DrawImage scales src to fill r and composites it onto dst (alpha-blended),
// e.g. an uploaded background photo, or an emoji/icon overlay.
func DrawImage(dst *image.RGBA, src image.Image, r image.Rectangle) {
	draw.CatmullRom.Scale(dst, r, src, src.Bounds(), draw.Over, nil)
}

// TextSpec describes one block of aligned text (one line per "\n").
type TextSpec struct {
	Text     string
	Color    color.Color
	FontFrac float64 // font size, fraction of Size; default 0.12
	Align    string  // left|center|right (default center)
	Valign   string  // top|middle|bottom (default middle)
	// TopFrac, if non-nil, positions the text block's top at this fraction of
	// Size directly, overriding Valign — for precise placement (e.g. a label
	// under a composited icon) rather than the coarse top/middle/bottom.
	TopFrac *float64
}

// DrawText renders spec as a block of lines, aligned to the chosen
// corner/edge/centre. The whole block is positioned as a unit, so multi-line
// text stays correctly centred.
func DrawText(img *image.RGBA, spec TextSpec) error {
	frac := spec.FontFrac
	if frac <= 0 {
		frac = 0.12
	}
	parsed, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return err
	}
	size := frac * Size
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return err
	}
	defer func() { _ = face.Close() }()

	c := spec.Color
	if c == nil {
		c = color.White
	}
	d := &font.Drawer{Dst: img, Src: image.NewUniform(c), Face: face}
	lines := strings.Split(spec.Text, "\n")
	const margin = 0.06 * Size
	const inner = Size - 2*margin
	lineH := size * 1.25
	blockH := lineH * float64(len(lines))
	m := face.Metrics()
	ascent, descent := float64(m.Ascent.Round()), float64(m.Descent.Round())

	// Top of the text block.
	top := (Size - blockH) / 2 // middle
	switch spec.Valign {
	case "top":
		top = margin
	case "bottom":
		top = Size - margin - blockH
	}
	if spec.TopFrac != nil {
		top = *spec.TopFrac * Size
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

// ParseHex turns "#rrggbb"/"#rgb" into a colour, falling back to def.
func ParseHex(s string, def color.Color) color.Color {
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

// Encode PNG-encodes img.
func Encode(img *image.RGBA) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Spec describes a generated cover: a solid or angled-gradient background (or
// a supplied overlay image), an optional centered icon (an emoji fetched from
// Twemoji, see FetchEmoji), a title, and an optional smaller subtitle below
// it. This is the single cover builder used both by the user-facing playlist
// cover generator and the curated chart cover generator, so a new use case is
// a new Spec, not a new renderer.
type Spec struct {
	Color     string  `json:"color"`     // background, hex (#rrggbb)
	Color2    string  `json:"color2"`    // gradient end; empty = solid
	Angle     float64 `json:"angle"`     // gradient angle, degrees
	Icon      string  `json:"icon"`      // Twemoji codepoint (e.g. "1f30d", "1f1eb-1f1f7"), centered above the title
	Text      string  `json:"text"`      // title; may contain \n for multiple lines. Always bold.
	Subtitle  string  `json:"subTitle"`  // smaller text below the title; grouped with it (and the icon, if set)
	TextColor string  `json:"textColor"` // hex, shared by the title and subtitle
	FontSize  float64 `json:"fontSize"`  // title font size, fraction of the square (default 0.12); the subtitle is subtitleScale of this
	Align     string  `json:"align"`     // left|center|right (default center)
	Valign    string  `json:"valign"`    // top|middle|bottom (default middle); ignored if Icon or Subtitle is set
}

// iconInsetFrac/iconFrac/iconGapFrac lay out the icon above the title: the
// icon is iconFrac square, inset iconInsetFrac from each edge, with
// iconGapFrac of space before the title below it. textGapFrac is the
// (tighter) gap between the title and the subtitle. subtitleScale is the
// subtitle's font size relative to the title's.
const iconInsetFrac = 0.32
const iconFrac = 1 - 2*iconInsetFrac
const iconGapFrac = 0.045
const textGapFrac = 0.02
const subtitleScale = 0.07 / 0.13

// Render paints spec's background (bg if given, else its gradient/solid
// color), then the icon/title/subtitle as one centered group, returning PNG
// bytes. A failed icon fetch just omits the icon rather than erroring.
func Render(ctx context.Context, spec Spec, bg image.Image) ([]byte, error) {
	img := NewCanvas()
	switch {
	case bg != nil:
		DrawImage(img, bg, img.Bounds())
	case spec.Color2 != "":
		FillGradient(img, ParseHex(spec.Color, color.Black), ParseHex(spec.Color2, color.Black), spec.Angle)
	default:
		FillSolid(img, ParseHex(spec.Color, color.Black))
	}

	titleFrac := spec.FontSize
	if titleFrac <= 0 {
		titleFrac = 0.12
	}
	subtitleFrac := titleFrac * subtitleScale
	textColor := ParseHex(spec.TextColor, color.White)

	var titleTop *float64
	if spec.Icon != "" || spec.Subtitle != "" {
		titleLines := 0.0
		if strings.TrimSpace(spec.Text) != "" {
			titleLines = float64(strings.Count(spec.Text, "\n") + 1)
		}
		titleHeightFrac := titleFrac * 1.25 * titleLines

		subtitleHeightFrac := 0.0
		if spec.Subtitle != "" {
			subtitleLines := float64(strings.Count(spec.Subtitle, "\n") + 1)
			subtitleHeightFrac = textGapFrac + subtitleFrac*1.25*subtitleLines
		}

		iconHeightFrac := 0.0
		if spec.Icon != "" {
			iconHeightFrac = iconFrac + iconGapFrac
		}

		groupTopFrac := (1 - (iconHeightFrac + titleHeightFrac + subtitleHeightFrac)) / 2

		if spec.Icon != "" {
			if icon, err := FetchEmoji(ctx, spec.Icon); err == nil {
				inset := int(iconInsetFrac * Size)
				top := int(groupTopFrac * Size)
				DrawImage(img, icon, image.Rect(inset, top, Size-inset, top+int(iconFrac*Size)))
			}
		}

		top := groupTopFrac + iconHeightFrac
		titleTop = &top

		if spec.Subtitle != "" {
			subtitleTop := top + titleHeightFrac + textGapFrac
			err := DrawText(img, TextSpec{
				Text: spec.Subtitle, Color: textColor,
				FontFrac: subtitleFrac, Align: spec.Align, TopFrac: &subtitleTop,
			})
			if err != nil {
				return nil, err
			}
		}
	}

	if strings.TrimSpace(spec.Text) != "" {
		err := DrawText(img, TextSpec{
			Text: spec.Text, Color: textColor,
			FontFrac: titleFrac, Align: spec.Align, Valign: spec.Valign, TopFrac: titleTop,
		})
		if err != nil {
			return nil, err
		}
	}

	return Encode(img)
}
