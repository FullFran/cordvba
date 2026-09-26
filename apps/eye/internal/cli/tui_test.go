package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FullFran/cordvba/apps/eye/internal/tui"
)

// tuiRegistry writes a registry with one entry of each interesting shape, so
// the cockpit has something honest to draw.
func tuiRegistry(t *testing.T) string {
	t.Helper()

	body := `
sources:
  - id: test-press
    authority: Diario de Prueba
    topic: press
    url: https://example.test/rss
    format: rss
    license: CC BY 4.0
    access: documented_api
    automation: enabled
    interval: 15m

  - id: test-held
    authority: Fuente Retenida
    topic: hydrology
    url: https://example.test/held
    format: web
    license: unspecified
    access: public_html
    automation: review_terms
    notes: Reuse terms unresolved.
`

	path := filepath.Join(t.TempDir(), "sources.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write registry: %v", err)
	}
	return path
}

// runTUI draws a single frame with the command wired to a temp store.
func runTUI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()

	app := New()
	app.Register(tuiCommand())

	base := []string{
		"tui", "--once",
		"--registry", tuiRegistry(t),
		"--data-dir", t.TempDir(),
		"--fallback-size", "120x36",
	}
	full := append(base, args...)

	var stdout, stderr bytes.Buffer
	code := app.Run(context.Background(), full, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// The whole point of a full-screen view is that it can be rendered without one,
// so this draws a frame against a temp store and asserts on the text.
func TestTUIDrawsADashboardFrame(t *testing.T) {
	t.Parallel()

	code, stdout, stderr := runTUI(t)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, stderr)
	}

	for _, want := range []string{
		"EYE", "DASHBOARD", "SOURCE HEALTH",
		"test-press", "test-held", "review_terms",
		"BY TOPIC", "RECORDS PER HOUR",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the frame is missing %q:\n%s", want, stdout)
		}
	}
}

// Without a keyboard the footer must say so rather than list keys that do
// nothing. `eye tui | tee frame.txt` is a reasonable thing to type, and telling
// the reader to press q there would be a small lie.
func TestTUIWithoutATerminalSaysThereIsNoKeyboard(t *testing.T) {
	t.Parallel()

	_, stdout, _ := runTUI(t)
	if !strings.Contains(stdout, "no keyboard") {
		t.Errorf("the frame does not admit that input is unavailable:\n%s", stdout)
	}
}

// A fresh store holds nothing, and the frame has to say so rather than draw an
// empty board that looks like a working one.
func TestTUIOnAnEmptyStoreSaysItIsEmpty(t *testing.T) {
	t.Parallel()

	_, stdout, _ := runTUI(t)
	if !strings.Contains(stdout, "0 records") {
		t.Errorf("an empty store is not reported:\n%s", stdout)
	}
}

func TestTUICanOpenOnAnyView(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"map":       "ON SCREEN",
		"feed":      "TIME",
		"sources":   "AUTOMATION",
		"transit":   "ARRIVALS",
		"dashboard": "SOURCE HEALTH",
		"DASHBOARD": "SOURCE HEALTH",
	}

	for view, want := range cases {
		t.Run(view, func(t *testing.T) {
			t.Parallel()

			code, stdout, stderr := runTUI(t, "--view", view)
			if code != 0 {
				t.Fatalf("exit = %d, stderr = %s", code, stderr)
			}
			if !strings.Contains(stdout, want) {
				t.Errorf("--view %s did not open that screen:\n%s", view, stdout)
			}
		})
	}
}

func TestTUIRejectsAnUnknownView(t *testing.T) {
	t.Parallel()

	code, _, stderr := runTUI(t, "--view", "sonar")
	if code != 1 {
		t.Errorf("exit = %d, want 1 for an unknown view", code)
	}
	if !strings.Contains(stderr, "sonar") {
		t.Errorf("the error does not name the view asked for: %q", stderr)
	}
}

// The fallback size is how the cockpit behaves where there is no terminal to
// ask, which is exactly the situation a test is in.
func TestParseSize(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		want    tui.Size
		wantErr bool
	}{
		{name: "lowercase x", in: "120x40", want: tui.Size{Cols: 120, Rows: 40}},
		{name: "uppercase X", in: "80X24", want: tui.Size{Cols: 80, Rows: 24}},
		{name: "spaces", in: " 100 x 30 ", want: tui.Size{Cols: 100, Rows: 30}},
		{name: "no separator", in: "12040", wantErr: true},
		{name: "not a number", in: "wide x tall", wantErr: true},
		{name: "zero", in: "0x24", wantErr: true},
		{name: "negative", in: "-10x24", wantErr: true},
		{name: "empty", in: "", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseSize(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseSize(%q) = %+v, want an error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSize(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("parseSize(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestTUIRejectsABadFallbackSize(t *testing.T) {
	t.Parallel()

	app := New()
	app.Register(tuiCommand())

	var stdout, stderr bytes.Buffer
	code := app.Run(context.Background(),
		[]string{"tui", "--once", "--fallback-size", "enormous", "--data-dir", t.TempDir()},
		&stdout, &stderr)

	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "fallback-size") {
		t.Errorf("the error does not name the flag: %q", stderr.String())
	}
}
