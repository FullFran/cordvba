// Package render draws images in a terminal using nothing but the standard
// library.
//
// The technique is half-blocks: each character cell shows two vertical pixels,
// the upper one as the foreground colour of U+2580 and the lower one as its
// background. That doubles the vertical resolution a terminal can express and
// works anywhere 24-bit colour does — including inside tmux, where the Kitty
// graphics protocol needs passthrough that is often not configured.
//
// image/jpeg is in the standard library, so this costs the dependency budget
// nothing.
package render

import (
	"fmt"
	"image"
	_ "image/jpeg" // JPEG is what every public traffic camera serves
	_ "image/png"
	"io"
	"strings"
)

// upperHalfBlock shows the top pixel as foreground and the bottom as background.
const upperHalfBlock = "▀"

// Bounds describes the character grid available for drawing.
type Bounds struct {
	// Cols is the terminal width in characters.
	Cols int
	// Rows is the maximum height in character rows. Each row is two pixels.
	Rows int
}

// DefaultBounds is a conservative size that fits an 80-column terminal.
var DefaultBounds = Bounds{Cols: 80, Rows: 24}

// Decode reads an image from r.
func Decode(r io.Reader) (image.Image, error) {
	img, format, err := image.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("render: decode: %w", err)
	}
	_ = format
	return img, nil
}

// HalfBlocks renders an image as ANSI half-block text, preserving aspect ratio.
//
// A terminal cell is roughly twice as tall as it is wide, and each cell holds
// two pixels vertically, so a cell is very nearly square in pixel terms. That
// is why the aspect calculation below does not need a fudge factor.
func HalfBlocks(img image.Image, b Bounds) string {
	if img == nil {
		return ""
	}
	cols, rows := fit(img, b)
	if cols <= 0 || rows <= 0 {
		return ""
	}

	src := img.Bounds()
	var sb strings.Builder
	sb.Grow(cols * rows * 40)

	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			// Two source pixels per cell: upper and lower.
			top := sample(img, src, col, row*2, cols, rows*2)
			bottom := sample(img, src, col, row*2+1, cols, rows*2)

			fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm%s",
				top.r, top.g, top.b, bottom.r, bottom.g, bottom.b, upperHalfBlock)
		}
		sb.WriteString("\x1b[0m\n")
	}
	return sb.String()
}

// rgb is an 8-bit colour.
type rgb struct{ r, g, b uint8 }

// sample takes the source pixel corresponding to a target cell position,
// using nearest-neighbour. A traffic camera frame is 853x480 and the target is
// a few dozen characters wide; anything more elaborate is not visible.
func sample(img image.Image, src image.Rectangle, x, y, outW, outH int) rgb {
	sx := src.Min.X + x*src.Dx()/outW
	sy := src.Min.Y + y*src.Dy()/outH

	if sx >= src.Max.X {
		sx = src.Max.X - 1
	}
	if sy >= src.Max.Y {
		sy = src.Max.Y - 1
	}

	r, g, b, _ := img.At(sx, sy).RGBA()
	return rgb{r: to8(r), g: to8(g), b: to8(b)}
}

// to8 narrows a colour channel from the 16 bits image/color returns to the 8
// bits an ANSI escape carries.
func to8(v uint32) uint8 {
	return uint8(v >> 8) // #nosec G115 -- RGBA() is 16-bit, so >>8 is always 0..255
}

// fit computes the character grid that keeps the image's aspect ratio inside
// the given bounds.
func fit(img image.Image, b Bounds) (cols, rows int) {
	maxCols, maxRows := b.Cols, b.Rows
	if maxCols <= 0 {
		maxCols = DefaultBounds.Cols
	}
	if maxRows <= 0 {
		maxRows = DefaultBounds.Rows
	}

	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if w <= 0 || h <= 0 {
		return 0, 0
	}

	// Start from the full width and derive the height, halving it because a
	// row holds two pixels.
	cols = maxCols
	rows = (cols * h / w) / 2

	if rows > maxRows {
		rows = maxRows
		cols = rows * 2 * w / h
	}
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	return cols, rows
}
