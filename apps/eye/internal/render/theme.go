package render

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Theme carries the escape sequences for terminal styling.
//
// eye's output should look like a situation board in an operations room: an
// amber-and-phosphor terminal that a person watches. What it must never do is
// perform. Every figure on that board traces back to a public source, and a
// blinking indicator has to mean something is actually happening — the styling
// dresses the truth, it does not stand in for it.
type Theme struct {
	enabled bool
}

// ANSI sequences. Phosphor green for live data, amber for anything the reader
// should not take at face value, dim for chrome.
const (
	reset    = "\x1b[0m"
	bold     = "\x1b[1m"
	dim      = "\x1b[2m"
	phosphor = "\x1b[38;2;0;255;110m"
	amber    = "\x1b[38;2;255;176;0m"
	crimson  = "\x1b[38;2;255;70;70m"
	cyan     = "\x1b[38;2;0;220;255m"
	slate    = "\x1b[38;2;120;140;150m"
)

// NewTheme builds a theme for a writer, honouring NO_COLOR and pipes.
//
// Colour is disabled whenever the output is not a terminal, so `eye news --json
// | jq` and `eye status > report.txt` stay clean. NO_COLOR is respected because
// people who set it mean it.
func NewTheme(w io.Writer) Theme {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("EYE_NO_COLOR") != "" {
		return Theme{}
	}
	return Theme{enabled: isTerminalWriter(w)}
}

// PlainTheme renders without any escape sequences.
func PlainTheme() Theme { return Theme{} }

// Enabled reports whether styling is on.
func (t Theme) Enabled() bool { return t.enabled }

// wrap applies a sequence when styling is on.
func (t Theme) wrap(seq, s string) string {
	if !t.enabled || s == "" {
		return s
	}
	return seq + s + reset
}

// Live styles a figure that is current and sourced.
func (t Theme) Live(s string) string { return t.wrap(phosphor, s) }

// Warn styles something the reader should not take at face value.
func (t Theme) Warn(s string) string { return t.wrap(amber, s) }

// Alert styles a critical state.
func (t Theme) Alert(s string) string { return t.wrap(crimson, s) }

// Label styles a field name.
func (t Theme) Label(s string) string { return t.wrap(cyan, s) }

// Chrome styles borders and furniture.
func (t Theme) Chrome(s string) string { return t.wrap(slate, s) }

// Dim styles secondary text.
func (t Theme) Dim(s string) string { return t.wrap(dim, s) }

// Bold styles emphasis.
func (t Theme) Bold(s string) string { return t.wrap(bold, s) }

// Banner is the masthead. The eye is drawn open, which is the whole idea.
const bannerArt = `    ▄▄▄▄▄▄▄▄▄▄▄▄▄▄▄
 ▄▀▀               ▀▀▄
▐     ▄▄▄▄▄▄▄▄▄▄▄     ▌
 ▀▄  █   ▄▄▄▄▄   █  ▄▀
    ▀█  █ ▓▓▓ █  █▀
      ▀▄ ▀▄▄▄▀ ▄▀
         ▀▀▀▀▀`

// Banner writes the masthead with a subtitle line.
func (t Theme) Banner(w io.Writer, subtitle string) {
	for _, line := range strings.Split(bannerArt, "\n") {
		_, _ = fmt.Fprintln(w, t.Live(line))
	}
	_, _ = fmt.Fprintf(w, "\n%s   %s\n\n",
		t.Bold(t.Live("E Y E")),
		t.Dim(subtitle))
}

// Rule draws a horizontal divider of the given width.
func (t Theme) Rule(width int) string {
	if width < 4 {
		width = 4
	}
	return t.Chrome(strings.Repeat("─", width))
}

// Section draws a titled divider: ── TITLE ──────────────
func (t Theme) Section(title string, width int) string {
	label := " " + strings.ToUpper(title) + " "
	fill := width - len(label) - 2
	if fill < 0 {
		fill = 0
	}
	return t.Chrome("──") + t.Label(label) + t.Chrome(strings.Repeat("─", fill))
}

// Indicator returns a status dot for a source state.
func (t Theme) Indicator(state string) string {
	switch state {
	case "live", "healthy", "ok":
		return t.Live("●")
	case "stale", "held", "review_terms":
		return t.Warn("◐")
	case "failing", "error":
		return t.Alert("✖")
	default:
		return t.Dim("○")
	}
}

// Pad left-aligns a string to a width and only then styles it.
//
// tabwriter measures bytes, and an ANSI sequence is bytes it cannot see past,
// so a styled cell of "5 visible characters" is really 20 and the column
// collapses. Padding before styling is the fix; the alternative is a
// width-aware writer, which is a lot of machinery for a status screen.
func (t Theme) Pad(s string, width int) string {
	runes := []rune(s)
	if len(runes) < width {
		return s + strings.Repeat(" ", width-len(runes))
	}
	return s
}

// isTerminalWriter reports whether a writer is an interactive terminal.
func isTerminalWriter(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	if info.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	// /dev/null is a character device with nobody behind it.
	if devNull, err := os.Stat(os.DevNull); err == nil && os.SameFile(info, devNull) {
		return false
	}
	return true
}
