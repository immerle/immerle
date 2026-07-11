package federation

import (
	"bytes"
	"errors"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"

	"golang.org/x/image/draw"
)

// mosaicSize is the pixel side of a generated playlist mosaic cover (matches
// the hand-designed cover generator's renderSize for consistent resolution
// across generated covers).
const mosaicSize = 1000

var errNoCovers = errors.New("federation: no covers to compose")

// renderMosaic composes up to 4 cover images into a single square JPEG,
// mirroring the client's PlaylistMosaic layout exactly (1 cover fills the
// square; 2 alternate diagonally; 3 cycle to fill the fourth cell; 4+ are used
// as-is) so the cover pushed to the hub matches what's shown locally.
// Undecodable tiles are skipped; an empty or fully-undecodable input errors.
func renderMosaic(tiles [][]byte) ([]byte, string, error) {
	imgs := make([]image.Image, 0, len(tiles))
	for _, t := range tiles {
		img, _, err := image.Decode(bytes.NewReader(t))
		if err != nil {
			continue
		}
		imgs = append(imgs, img)
	}
	if len(imgs) == 0 {
		return nil, "", errNoCovers
	}
	if len(imgs) == 1 {
		return encodeSquareJPEG(imgs[0], mosaicSize)
	}

	cells := mosaicCells(imgs)
	half := mosaicSize / 2
	canvas := image.NewRGBA(image.Rect(0, 0, mosaicSize, mosaicSize))
	offsets := [4]image.Point{{X: 0, Y: 0}, {X: half, Y: 0}, {X: 0, Y: half}, {X: half, Y: half}}
	for i, img := range cells {
		src := squareBounds(img.Bounds())
		dst := image.Rect(offsets[i].X, offsets[i].Y, offsets[i].X+half, offsets[i].Y+half)
		draw.CatmullRom.Scale(canvas, dst, img, src, draw.Over, nil)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, canvas, &jpeg.Options{Quality: 85}); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), "image/jpeg", nil
}

// mosaicCells fills the 4 grid cells from 2-4 available images, matching
// PlaylistMosaic.tsx's pattern exactly: 2 images alternate diagonally
// ([0,1,1,0]), 3 cycle to fill the fourth cell ([0,1,2,0]), 4+ are used as-is.
func mosaicCells(imgs []image.Image) [4]image.Image {
	n := len(imgs)
	if n >= 4 {
		return [4]image.Image{imgs[0], imgs[1], imgs[2], imgs[3]}
	}
	if n == 2 {
		return [4]image.Image{imgs[0], imgs[1], imgs[1], imgs[0]}
	}
	return [4]image.Image{imgs[0%n], imgs[1%n], imgs[2%n], imgs[3%n]}
}

// squareBounds returns the largest centered square within b (a center crop,
// so scaling into a square tile doesn't distort a non-square source).
func squareBounds(b image.Rectangle) image.Rectangle {
	w, h := b.Dx(), b.Dy()
	s := w
	if h < s {
		s = h
	}
	x0 := b.Min.X + (w-s)/2
	y0 := b.Min.Y + (h-s)/2
	return image.Rect(x0, y0, x0+s, y0+s)
}

func encodeSquareJPEG(img image.Image, size int) ([]byte, string, error) {
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, squareBounds(img.Bounds()), draw.Over, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85}); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), "image/jpeg", nil
}
