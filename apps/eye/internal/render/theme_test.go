package render_test

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/FullFran/cordvba/apps/eye/internal/render"
)

// A pipe is not a terminal. `eye news --json | jq` and `eye status > report.txt`
// must never receive escape sequences.
func TestThemeIsDisabledForANonTerminal(t *testing.T) {
	t.Parallel()

	theme := render.NewTheme(&bytes.Buffer{})
	if theme.Enabled() {
		t.Fatal("styling is on for a buffer")
	}
	if got := theme.Live("21"); got != "21" {
		t.Errorf("Live() = %q, want the bare string", got)
	}
}

// People who set NO_COLOR mean it.
func TestThemeRespectsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	if render.NewTheme(os.Stdout).Enabled() {
		t.Error("styling is on despite NO_COLOR")
	}
}

func TestThemeRespectsEyeNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("EYE_NO_COLOR", "1")

	if render.NewTheme(os.Stdout).Enabled() {
		t.Error("styling is on despite EYE_NO_COLOR")
	}
}

func TestPlainThemeNeverStyles(t *testing.T) {
	t.Parallel()

	theme := render.PlainTheme()
	for name, got := range map[string]string{
		"Live":   theme.Live("x"),
		"Warn":   theme.Warn("x"),
		"Alert":  theme.Alert("x"),
		"Label":  theme.Label("x"),
		"Chrome": theme.Chrome("x"),
		"Dim":    theme.Dim("x"),
		"Bold":   theme.Bold("x"),
	} {
		if got != "x" {
			t.Errorf("%s() = %q, want the bare string", name, got)
		}
	}
}

// tabwriter measures bytes and cannot see past an escape sequence, so padding
// has to happen before styling or every coloured column collapses.
func TestPadMeasuresRunesNotBytes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		in    string
		width int
		want  int
	}{
		{name: "ascii", in: "press", width: 12, want: 12},
		{name: "accented runes count once", in: "geofísica", width: 12, want: 12},
		{name: "already wide enough", in: "observations", width: 6, want: 12},
		{name: "empty", in: "", width: 4, want: 4},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := len([]rune(render.PlainTheme().Pad(tc.in, tc.width))); got != tc.want {
				t.Errorf("Pad(%q, %d) is %d runes, want %d", tc.in, tc.width, got, tc.want)
			}
		})
	}
}

func TestSectionAndRuleFitTheWidth(t *testing.T) {
	t.Parallel()

	theme := render.PlainTheme()

	if got := len([]rune(theme.Rule(66))); got != 66 {
		t.Errorf("Rule(66) is %d runes", got)
	}
	section := theme.Section("sources", 66)
	if got := len([]rune(section)); got != 66 {
		t.Errorf("Section() is %d runes, want 66: %q", got, section)
	}
	if !strings.Contains(section, "SOURCES") {
		t.Errorf("Section() lost its title: %q", section)
	}
	// A title longer than the width must not produce a negative repeat.
	if got := theme.Section(strings.Repeat("long", 40), 20); got == "" {
		t.Error("Section() with an oversized title produced nothing")
	}
}

func TestIndicatorDistinguishesStates(t *testing.T) {
	t.Parallel()

	theme := render.PlainTheme()
	seen := map[string]string{}
	for _, state := range []string{"live", "stale", "failing", "unknown"} {
		seen[state] = theme.Indicator(state)
	}

	if seen["live"] == seen["failing"] {
		t.Error("a healthy source and a failing one share an indicator")
	}
	if seen["stale"] == seen["live"] {
		t.Error("a stale source and a live one share an indicator")
	}
}

func TestBannerWritesTheSubtitle(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	render.PlainTheme().Banner(&buf, "CÓRDOBA · 28 Aug 2026")

	out := buf.String()
	if !strings.Contains(out, "E Y E") {
		t.Errorf("banner has no wordmark:\n%s", out)
	}
	if !strings.Contains(out, "CÓRDOBA · 28 Aug 2026") {
		t.Errorf("banner lost its subtitle:\n%s", out)
	}
}

// `eye watch | tee log.txt` must produce something readable, not a stream of
// cursor moves.
func TestScreenIsInertForANonTerminal(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	screen := render.NewScreen(&buf)

	screen.Enter()
	screen.Frame(func(w io.Writer) { _, _ = io.WriteString(w, "board\n") })
	screen.Separator()
	screen.Leave()

	out := buf.String()
	if strings.Contains(out, "\x1b[") {
		t.Errorf("screen wrote escape sequences to a buffer: %q", out)
	}
	if !strings.Contains(out, "board") {
		t.Errorf("the frame content was lost: %q", out)
	}
}

func TestScreenFrameCallsTheDrawFunction(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	var called int
	render.NewScreen(&buf).Frame(func(io.Writer) { called++ })

	if called != 1 {
		t.Errorf("draw called %d times, want 1", called)
	}
}
