package render_test

import (
	"strings"
	"testing"

	"github.com/FullFran/eye/internal/render"
)

// Escape sequences are bytes the terminal never shows, so anything that lays
// out columns has to measure past them. Getting this wrong is how a styled
// panel ends up with a ragged right border.
func TestVisibleWidthIgnoresEscapeSequences(t *testing.T) {
	t.Parallel()

	theme := render.AnsiTheme()

	cases := []struct {
		name string
		in   string
		want int
	}{
		{name: "plain", in: "hello", want: 5},
		{name: "styled", in: theme.Live("hello"), want: 5},
		{name: "nested styling", in: theme.Live("a") + theme.Warn("bc"), want: 3},
		{name: "empty", in: "", want: 0},
		{name: "accented text counts runes", in: "CÓRDOBA", want: 7},
		{name: "braille", in: "⠁⠂⠄", want: 3},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := render.VisibleWidth(tc.in); got != tc.want {
				t.Errorf("VisibleWidth(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestClipKeepsStylingIntactAndClosesIt(t *testing.T) {
	t.Parallel()

	theme := render.AnsiTheme()
	got := render.Clip(theme.Live("abcdefghij"), 4)

	if w := render.VisibleWidth(got); w != 4 {
		t.Errorf("Clip to 4 produced %d visible columns: %q", w, got)
	}
	if !strings.HasSuffix(got, "\x1b[0m") {
		t.Errorf("Clip left the styling open: %q", got)
	}
}

func TestClipLeavesShortStringsAlone(t *testing.T) {
	t.Parallel()

	if got := render.Clip("abc", 10); got != "abc" {
		t.Errorf("Clip(%q, 10) = %q", "abc", got)
	}
}

func TestPadVisibleFillsToTheColumnWidth(t *testing.T) {
	t.Parallel()

	theme := render.AnsiTheme()
	got := render.PadVisible(theme.Warn("ok"), 6)

	if w := render.VisibleWidth(got); w != 6 {
		t.Errorf("PadVisible produced %d columns: %q", w, got)
	}
}

// A panel is only useful if its borders line up, whatever is inside it.
func TestBoxRowsAreAllTheSameVisibleWidth(t *testing.T) {
	t.Parallel()

	theme := render.AnsiTheme()
	const width = 30

	rows := []string{
		theme.BoxTop("sources", width),
		theme.BoxRow("plain", width),
		theme.BoxRow(theme.Live("styled")+" "+theme.Alert("mixed"), width),
		theme.BoxRow("a line far longer than the box could ever hold without clipping", width),
		theme.BoxBottom(width),
	}

	for i, row := range rows {
		if got := render.VisibleWidth(row); got != width {
			t.Errorf("row %d is %d columns wide, want %d: %q", i, got, width, row)
		}
	}
}

func TestBoxTopCarriesItsTitle(t *testing.T) {
	t.Parallel()

	got := render.PlainTheme().BoxTop("transit", 24)
	if !strings.Contains(got, "TRANSIT") {
		t.Errorf("BoxTop dropped the title: %q", got)
	}
	if !strings.HasPrefix(got, "┌") || !strings.HasSuffix(got, "┐") {
		t.Errorf("BoxTop is not a box corner: %q", got)
	}
}

// A title longer than the box must not push the corner off the end.
func TestBoxTopClipsAnOversizedTitle(t *testing.T) {
	t.Parallel()

	got := render.PlainTheme().BoxTop("a very long panel title indeed", 14)
	if w := render.VisibleWidth(got); w != 14 {
		t.Errorf("BoxTop is %d columns wide, want 14: %q", w, got)
	}
}

func TestBoxHandlesAWidthTooSmallToDraw(t *testing.T) {
	t.Parallel()

	theme := render.PlainTheme()
	for _, width := range []int{0, 1, 2} {
		if got := theme.BoxTop("x", width); render.VisibleWidth(got) > 2 {
			t.Errorf("BoxTop(%d) = %q, which is wider than it was given", width, got)
		}
		if got := theme.BoxRow("x", width); render.VisibleWidth(got) > 2 {
			t.Errorf("BoxRow(%d) = %q, which is wider than it was given", width, got)
		}
	}
}

func TestScanlineFillsExactly(t *testing.T) {
	t.Parallel()

	got := render.PlainTheme().Scanline(20)
	if w := render.VisibleWidth(got); w != 20 {
		t.Errorf("Scanline(20) is %d columns wide: %q", w, got)
	}
}

// The histogram is how the dashboard shows a rate. An empty series must render
// as empty rather than as a full bar, and the tallest bar must be full.
func TestBarsScaleToTheLargestValue(t *testing.T) {
	t.Parallel()

	got := render.Bars([]int{0, 1, 4})
	runes := []rune(got)

	if len(runes) != 3 {
		t.Fatalf("Bars produced %d cells for 3 values: %q", len(runes), got)
	}
	if runes[0] != ' ' {
		t.Errorf("a zero rendered as %q, want a blank", string(runes[0]))
	}
	if runes[2] != '█' {
		t.Errorf("the largest value rendered as %q, want a full block", string(runes[2]))
	}
	if runes[1] == runes[2] {
		t.Error("a quarter-height value rendered as tall as the peak")
	}
}

func TestBarsOnAllZerosDrawsNothing(t *testing.T) {
	t.Parallel()

	if got := render.Bars([]int{0, 0, 0}); strings.TrimSpace(got) != "" {
		t.Errorf("Bars([0,0,0]) = %q, want blanks", got)
	}
}

// A table cell that is cut has to say so. Half a licence name with no mark
// reads as the value the publisher declared, which is a quiet lie.
func TestEllipsisMarksWhatItCut(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		in    string
		width int
		want  string
	}{
		{name: "fits", in: "datex2", width: 12, want: "datex2"},
		{name: "exact", in: "datex2", width: 6, want: "datex2"},
		{name: "cut", in: "datex2-3.7-xml", width: 10, want: "datex2-3.…"},
		{name: "one column", in: "datex2", width: 1, want: "…"},
		{name: "no room", in: "datex2", width: 0, want: ""},
		{name: "accented", in: "CÓRDOBA capital", width: 8, want: "CÓRDOBA…"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := render.Ellipsis(tc.in, tc.width)
			if got != tc.want {
				t.Errorf("Ellipsis(%q, %d) = %q, want %q", tc.in, tc.width, got, tc.want)
			}
			if w := render.VisibleWidth(got); w > tc.width {
				t.Errorf("Ellipsis(%q, %d) is %d columns wide", tc.in, tc.width, w)
			}
		})
	}
}

func TestEllipsisKeepsStylingClosed(t *testing.T) {
	t.Parallel()

	got := render.Ellipsis(render.AnsiTheme().Live("abcdefghij"), 5)
	if render.VisibleWidth(got) != 5 {
		t.Errorf("styled ellipsis is %d columns: %q", render.VisibleWidth(got), got)
	}
	if !strings.Contains(got, "…") {
		t.Errorf("the cut is not marked: %q", got)
	}
}
