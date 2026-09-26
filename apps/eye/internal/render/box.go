package render

import "strings"

// Panels are drawn with single-line box characters. Double lines and heavy
// lines exist, but a board made of them reads as a form to fill in rather than
// as an instrument.
const (
	boxTopLeft     = "┌"
	boxTopRight    = "┐"
	boxBottomLeft  = "└"
	boxBottomRight = "┘"
	boxHorizontal  = "─"
	boxVertical    = "│"
)

// scanlineRune is the separator under the header. It is the top eighth block,
// which sits on the cap line and reads as the trace of a beam rather than as a
// rule between two tables.
const scanlineRune = "▔"

// blocks are the eighth-height blocks, from empty to full. They are what turns
// a series of counts into a histogram inside a single character row.
var blocks = []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// VisibleWidth counts the columns a string actually occupies, ignoring ANSI
// escape sequences.
//
// This is the measurement every panel depends on. A styled cell of five
// characters is twenty bytes, and code that measures bytes draws a box with a
// ragged right edge.
func VisibleWidth(s string) int {
	width := 0
	for _, run := range splitANSI(s) {
		if !run.escape {
			width += len([]rune(run.text))
		}
	}
	return width
}

// Clip truncates a string to a number of visible columns, keeping the escape
// sequences it passes through and closing any that were left open.
func Clip(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if VisibleWidth(s) <= width {
		return s
	}

	var (
		sb      strings.Builder
		visible int
		styled  bool
	)
	for _, run := range splitANSI(s) {
		if run.escape {
			sb.WriteString(run.text)
			styled = styled || run.text != reset
			continue
		}
		for _, r := range run.text {
			if visible == width {
				if styled {
					sb.WriteString(reset)
				}
				return sb.String()
			}
			sb.WriteRune(r)
			visible++
		}
	}
	if styled {
		sb.WriteString(reset)
	}
	return sb.String()
}

// Ellipsis truncates a string to a column width and marks that it was cut.
//
// It is the right tool for a table cell, where Clip is the right tool for a
// whole line: half a licence name with no mark reads as the value, and a reader
// has no way to tell "free-of-charg" from what the publisher actually wrote.
func Ellipsis(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if VisibleWidth(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return Clip(s, width-1) + "…"
}

// PadVisible pads a string with spaces to a number of visible columns, and
// clips it when it is already wider.
func PadVisible(s string, width int) string {
	got := VisibleWidth(s)
	if got > width {
		return Clip(s, width)
	}
	return s + strings.Repeat(" ", width-got)
}

// Bars renders a series of counts as a one-row histogram of block characters,
// scaled so the largest value is a full block.
//
// The scale is relative because the question a rate histogram answers is
// "when was it busy", not "how many exactly" — the exact figures belong in the
// numbers printed beside it.
func Bars(values []int) string {
	peak := 0
	for _, v := range values {
		if v > peak {
			peak = v
		}
	}

	var sb strings.Builder
	sb.Grow(len(values) * 3)
	for _, v := range values {
		if peak <= 0 || v <= 0 {
			sb.WriteRune(blocks[0])
			continue
		}
		// Rounding up keeps a single observation visible: a bar that is
		// rounded away says nothing happened, which is a lie.
		level := (v*(len(blocks)-1) + peak - 1) / peak
		sb.WriteRune(blocks[level])
	}
	return sb.String()
}

// BoxTop draws the top edge of a panel with its title inlaid.
func (t Theme) BoxTop(title string, width int) string {
	if width < 3 {
		return t.Chrome(strings.Repeat(boxHorizontal, max(width, 0)))
	}

	label := ""
	if title != "" {
		label = " " + strings.ToUpper(title) + " "
		// Two corners and one leading dash is the least a titled edge can
		// occupy; anything longer than the remainder gets clipped.
		if room := width - 3; len([]rune(label)) > room {
			label = Clip(label, room)
		}
	}

	fill := width - 2 - len([]rune(label)) - 1
	if fill < 0 {
		fill = 0
	}
	return t.Chrome(boxTopLeft+boxHorizontal) + t.Label(label) +
		t.Chrome(strings.Repeat(boxHorizontal, fill)+boxTopRight)
}

// BoxRow draws one content row of a panel, padded or clipped to fit.
func (t Theme) BoxRow(content string, width int) string {
	if width < 3 {
		return strings.Repeat(" ", max(width, 0))
	}
	inner := width - 4
	return t.Chrome(boxVertical) + " " + PadVisible(content, inner) + " " + t.Chrome(boxVertical)
}

// BoxBottom draws the bottom edge of a panel.
func (t Theme) BoxBottom(width int) string {
	if width < 3 {
		return t.Chrome(strings.Repeat(boxHorizontal, max(width, 0)))
	}
	return t.Chrome(boxBottomLeft + strings.Repeat(boxHorizontal, width-2) + boxBottomRight)
}

// Scanline draws the header separator.
func (t Theme) Scanline(width int) string {
	if width < 0 {
		width = 0
	}
	return t.Live(strings.Repeat(scanlineRune, width))
}

// ansiRun is a stretch of a string that is either an escape sequence or text.
type ansiRun struct {
	text   string
	escape bool
}

// splitANSI cuts a string into escape sequences and the text between them.
//
// It recognises CSI sequences (ESC [ … final byte), which is every sequence
// the theme emits. Anything else beginning with ESC is treated as two-byte,
// which is enough not to mismeasure it.
func splitANSI(s string) []ansiRun {
	var (
		runs  []ansiRun
		start int
	)
	for i := 0; i < len(s); {
		if s[i] != 0x1b {
			i++
			continue
		}
		if i > start {
			runs = append(runs, ansiRun{text: s[start:i]})
		}
		end := escapeEnd(s, i)
		runs = append(runs, ansiRun{text: s[i:end], escape: true})
		i, start = end, end
	}
	if start < len(s) {
		runs = append(runs, ansiRun{text: s[start:]})
	}
	return runs
}

// escapeEnd returns the index just past the escape sequence starting at i.
func escapeEnd(s string, i int) int {
	if i+1 >= len(s) {
		return len(s)
	}
	if s[i+1] != '[' {
		return i + 2
	}
	for j := i + 2; j < len(s); j++ {
		// A CSI sequence ends on its first byte in the 0x40..0x7e range.
		if s[j] >= 0x40 && s[j] <= 0x7e {
			return j + 1
		}
	}
	return len(s)
}
