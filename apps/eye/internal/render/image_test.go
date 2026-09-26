package render_test

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"

	"github.com/FullFran/cordvba/apps/eye/internal/render"
)

// testImage builds a solid-colour image of the given size.
func testImage(w, h int, c color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

// asJPEG encodes an image the way a traffic camera serves it.
func asJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.Bytes()
}

func TestDecodeJPEG(t *testing.T) {
	t.Parallel()

	// The real frame size DGT serves.
	body := asJPEG(t, testImage(853, 480, color.RGBA{R: 10, G: 120, B: 200, A: 255}))

	img, err := render.Decode(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Decode() = %v", err)
	}
	if got := img.Bounds().Dx(); got != 853 {
		t.Errorf("width = %d, want 853", got)
	}
}

func TestDecodeRejectsNonImage(t *testing.T) {
	t.Parallel()

	if _, err := render.Decode(strings.NewReader("<html>404</html>")); err == nil {
		t.Fatal("Decode() on HTML = nil error")
	}
}

func TestHalfBlocksPreservesAspectRatio(t *testing.T) {
	t.Parallel()

	// 853x480 is roughly 16:9. At 64 columns the image should be about 18
	// rows: 64 * (480/853) / 2.
	out := render.HalfBlocks(testImage(853, 480, color.RGBA{A: 255}), render.Bounds{Cols: 64, Rows: 40})

	rows := strings.Count(out, "\n")
	if rows < 15 || rows > 21 {
		t.Errorf("rows = %d, want about 18 for a 16:9 image at 64 columns", rows)
	}

	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if got := strings.Count(line, "▀"); got != 64 {
			t.Fatalf("a row has %d cells, want 64", got)
		}
	}
}

func TestHalfBlocksRespectsRowLimit(t *testing.T) {
	t.Parallel()

	// A tall image must be bounded by rows, not by columns.
	out := render.HalfBlocks(testImage(100, 1000, color.RGBA{A: 255}), render.Bounds{Cols: 80, Rows: 10})

	if rows := strings.Count(out, "\n"); rows > 10 {
		t.Errorf("rows = %d, want at most 10", rows)
	}
}

func TestHalfBlocksCarriesColour(t *testing.T) {
	t.Parallel()

	out := render.HalfBlocks(testImage(10, 10, color.RGBA{R: 255, G: 0, B: 0, A: 255}), render.Bounds{Cols: 8, Rows: 8})

	// JPEG is lossy, so this is a solid-colour image rendered from RGBA:
	// the foreground escape must carry a strong red.
	if !strings.Contains(out, "\x1b[38;2;255;0;0m") {
		t.Errorf("no red foreground in the output: %q", out[:min(120, len(out))])
	}
	if !strings.HasSuffix(out, "\x1b[0m\n") {
		t.Error("output does not reset the colour at the end of the last row")
	}
}

func TestHalfBlocksHandlesNilAndDegenerateBounds(t *testing.T) {
	t.Parallel()

	if got := render.HalfBlocks(nil, render.Bounds{Cols: 10, Rows: 10}); got != "" {
		t.Errorf("HalfBlocks(nil) = %q, want empty", got)
	}
	// Zero bounds fall back to the defaults rather than producing nothing.
	if got := render.HalfBlocks(testImage(10, 10, color.RGBA{A: 255}), render.Bounds{}); got == "" {
		t.Error("HalfBlocks with zero bounds produced nothing")
	}
}

// noisyImage does not compress, so its PNG is large enough to force the
// multi-chunk path. A solid colour would fit in one escape and never exercise
// the continuation logic.
func noisyImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	seed := uint32(12345)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			seed = seed*1664525 + 1013904223
			img.Set(x, y, color.RGBA{R: byteOf(seed, 24), G: byteOf(seed, 16), B: byteOf(seed, 8), A: 255})
		}
	}
	return img
}

func TestKittySingleChunk(t *testing.T) {
	t.Parallel()

	// A solid colour compresses to well under one chunk, so the whole
	// payload rides in the first escape and must be marked complete there.
	out, err := render.Kitty(testImage(64, 48, color.RGBA{R: 3, G: 200, B: 90, A: 255}), 40)
	if err != nil {
		t.Fatalf("Kitty() = %v", err)
	}

	if !strings.HasPrefix(out, "\x1b_Ga=T,f=100,c=40,m=0;") {
		t.Errorf("a single-chunk payload must be marked m=0 in its own header: %q", out[:min(60, len(out))])
	}
	if strings.Contains(out, "\x1b_Gm=") {
		t.Error("a single-chunk payload emitted a continuation escape")
	}
}

func TestKittyMultipleChunks(t *testing.T) {
	t.Parallel()

	out, err := render.Kitty(noisyImage(300, 200), 60)
	if err != nil {
		t.Fatalf("Kitty() = %v", err)
	}

	if !strings.HasPrefix(out, "\x1b_Ga=T,f=100,c=60,m=1;") {
		t.Errorf("first chunk of a long payload must be marked m=1: %q", out[:min(60, len(out))])
	}
	if strings.Count(out, "\x1b_Gm=1;") == 0 {
		t.Error("no continuation chunks in a payload that needs them")
	}
	// The last chunk must say m=0, or the terminal waits forever.
	if strings.Count(out, "\x1b_Gm=0;") != 1 {
		t.Errorf("expected exactly one terminating chunk, got %d", strings.Count(out, "\x1b_Gm=0;"))
	}
	if !strings.HasSuffix(out, "\x1b\\\n") {
		t.Error("output does not end with a terminated escape")
	}
}

func TestKittyRejectsNil(t *testing.T) {
	t.Parallel()

	if _, err := render.Kitty(nil, 40); err == nil {
		t.Fatal("Kitty(nil) = nil error")
	}
}

// tmux is excluded on purpose: passthrough is often unconfigured, and a
// half-drawn image is worse than a clean fallback to half-blocks.
func TestSupportsKittyIsFalseInsideTmux(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,123,0")
	t.Setenv("TERM", "xterm-kitty")

	if render.SupportsKitty() {
		t.Error("SupportsKitty() = true inside tmux")
	}
}

func TestSupportsKittyDetectsTerminals(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{name: "kitty via TERM", env: map[string]string{"TERM": "xterm-kitty"}, want: true},
		{name: "ghostty via TERM_PROGRAM", env: map[string]string{"TERM_PROGRAM": "ghostty"}, want: true},
		{name: "ghostty via resources dir", env: map[string]string{"GHOSTTY_RESOURCES_DIR": "/usr/share/ghostty"}, want: true},
		{name: "plain xterm", env: map[string]string{"TERM": "xterm-256color"}, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, k := range []string{"TMUX", "TERM", "TERM_PROGRAM", "KITTY_WINDOW_ID", "GHOSTTY_RESOURCES_DIR"} {
				t.Setenv(k, "")
			}
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if got := render.SupportsKitty(); got != tc.want {
				t.Errorf("SupportsKitty() = %v, want %v", got, tc.want)
			}
		})
	}
}

// byteOf extracts one byte from a PRNG word.
func byteOf(v uint32, shift uint) uint8 {
	return uint8(v >> shift & 0xff) // #nosec G115 -- masked to 8 bits
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
