package render

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"os"
	"strings"
)

// kittyChunk is the payload size per escape sequence. The protocol requires
// chunks of at most 4096 base64 bytes.
const kittyChunk = 4096

// SupportsKitty reports whether the terminal can render the Kitty graphics
// protocol directly.
//
// tmux is excluded on purpose. Passing graphics through tmux needs
// `allow-passthrough` and a wrapper sequence that many configurations do not
// have, and a half-working image is worse than a clean fallback.
func SupportsKitty() bool {
	if os.Getenv("TMUX") != "" {
		return false
	}
	term := strings.ToLower(os.Getenv("TERM"))
	program := strings.ToLower(os.Getenv("TERM_PROGRAM"))

	return strings.Contains(term, "kitty") ||
		strings.Contains(term, "ghostty") ||
		program == "ghostty" ||
		os.Getenv("KITTY_WINDOW_ID") != "" ||
		os.Getenv("GHOSTTY_RESOURCES_DIR") != ""
}

// Kitty renders an image using the Kitty graphics protocol, at true resolution
// rather than as coloured text.
//
// cols bounds the display width in character cells; the terminal scales the
// image to fit while preserving its aspect ratio.
func Kitty(img image.Image, cols int) (string, error) {
	if img == nil {
		return "", fmt.Errorf("render: no image")
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", fmt.Errorf("render: encode png: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())

	var sb strings.Builder
	sb.Grow(len(encoded) + len(encoded)/kittyChunk*32 + 64)

	for first := true; len(encoded) > 0; first = false {
		chunk := encoded
		if len(chunk) > kittyChunk {
			chunk = chunk[:kittyChunk]
		}
		encoded = encoded[len(chunk):]

		more := 0
		if len(encoded) > 0 {
			more = 1
		}

		if first {
			// a=T transmit and display, f=100 PNG payload, c= width in
			// cells; the terminal derives the height from the aspect.
			fmt.Fprintf(&sb, "\x1b_Ga=T,f=100,c=%d,m=%d;%s\x1b\\", cols, more, chunk)
			continue
		}
		fmt.Fprintf(&sb, "\x1b_Gm=%d;%s\x1b\\", more, chunk)
	}

	sb.WriteString("\n")
	return sb.String(), nil
}
