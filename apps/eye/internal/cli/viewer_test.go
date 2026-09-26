package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRamDirPrefersXDGRuntimeDir(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", base)

	dir, err := ramDir()
	if err != nil {
		t.Fatalf("ramDir() = %v", err)
	}
	if !strings.HasPrefix(dir, base) {
		t.Errorf("ramDir() = %q, want it under %q", dir, base)
	}

	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Fatalf("ramDir() did not create a directory: %v", err)
	}
	// The frames directory holds images of public roads. It is not for
	// anyone else on the machine.
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("permissions = %o, want 700", perm)
	}
}

// Falling back to /tmp would defeat the point: on many systems it is a real
// directory on a real disk.
func TestRamDirRefusesRatherThanUsingDisk(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(t.TempDir(), "missing"))

	// /dev/shm exists on Linux, so this only asserts the error path when it
	// does not — which is the case this guards.
	if _, err := os.Stat("/dev/shm"); err == nil {
		t.Skip("/dev/shm exists on this machine; the refusal path cannot be exercised")
	}

	_, err := ramDir()
	if !errors.Is(err, ErrNoViewer) {
		t.Fatalf("ramDir() = %v, want ErrNoViewer", err)
	}
	if !strings.Contains(err.Error(), "will not write a camera frame to disk") {
		t.Errorf("error = %q, want it to say why", err)
	}
}

func TestFirstAvailable(t *testing.T) {
	t.Parallel()

	if got := firstAvailable([]string{"definitely-not-a-command-xyz", "sh"}); got != "sh" {
		t.Errorf("firstAvailable() = %q, want sh", got)
	}
	if got := firstAvailable([]string{"definitely-not-a-command-xyz"}); got != "" {
		t.Errorf("firstAvailable() = %q, want empty", got)
	}
	if got := firstAvailable(nil); got != "" {
		t.Errorf("firstAvailable(nil) = %q, want empty", got)
	}
}

// The spawned terminal must not believe it is inside tmux, or it falls back to
// half-blocks in a terminal that can draw the real image.
func TestEnvironWithoutDropsTmux(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,123,0")
	t.Setenv("TMUX_PANE", "%3")
	t.Setenv("EYE_TEST_KEEP", "kept")

	env := environWithout("TMUX", "TMUX_PANE")

	var keptFound bool
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		switch key {
		case "TMUX", "TMUX_PANE":
			t.Errorf("%s survived the filter", key)
		case "EYE_TEST_KEEP":
			keptFound = true
		}
	}
	if !keptFound {
		t.Error("environWithout dropped an unrelated variable")
	}
}

func TestTerminalExecFlag(t *testing.T) {
	t.Parallel()

	cases := []struct {
		term string
		want []string
	}{
		{term: "ghostty", want: []string{"-e"}},
		{term: "kitty", want: []string{"-e"}},
		{term: "wezterm", want: []string{"start", "--"}},
		{term: "something-else", want: []string{"-e"}},
	}

	for _, tc := range cases {
		t.Run(tc.term, func(t *testing.T) {
			t.Parallel()
			got := terminalExecFlag(tc.term)
			if strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Errorf("terminalExecFlag(%q) = %v, want %v", tc.term, got, tc.want)
			}
		})
	}
}

func TestParsePoint(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		wantErr bool
		lat     float64
	}{
		{name: "cordoba", in: "37.8882,-4.7794", lat: 37.8882},
		{name: "spaces are tolerated", in: " 37.8882 , -4.7794 ", lat: 37.8882},
		{name: "no comma", in: "37.8882", wantErr: true},
		{name: "not numeric", in: "norte,sur", wantErr: true},
		{name: "outside WGS84", in: "91,0", wantErr: true},
		{name: "empty", in: "", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parsePoint(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parsePoint(%q) = nil error", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("parsePoint(%q) = %v", tc.in, err)
			}
			if got.Lat != tc.lat {
				t.Errorf("lat = %v, want %v", got.Lat, tc.lat)
			}
		})
	}
}

func TestCameraImageURL(t *testing.T) {
	t.Parallel()

	withPayload := observationEntityWithID("dgt-cameras:421")
	withPayload.Payload = []byte(`{"image_url":"https://etraffic.dgt.es/camarasEtraffic/421.jpg","road":"A-4"}`)
	if got := cameraImageURL(withPayload); got != "https://etraffic.dgt.es/camarasEtraffic/421.jpg" {
		t.Errorf("cameraImageURL() = %q", got)
	}

	// A municipal camera publishes a position and nothing else.
	noPayload := observationEntityWithID("cordoba-cameras:num.7")
	noPayload.Payload = []byte(`{"name":"PLAZA DE LAS TENDILLAS"}`)
	if got := cameraImageURL(noPayload); got != "" {
		t.Errorf("cameraImageURL() = %q, want empty for a camera with no image", got)
	}

	if got := cameraImageURL(observationEntityWithID("x")); got != "" {
		t.Errorf("cameraImageURL() with no payload = %q, want empty", got)
	}
}

func TestTerminalWidth(t *testing.T) {
	t.Setenv("COLUMNS", "120")
	if got := terminalWidth(); got != 120 {
		t.Errorf("terminalWidth() = %d, want 120", got)
	}

	// Absurdly wide terminals are capped: a 400-column image is not more
	// readable, just slower to draw.
	t.Setenv("COLUMNS", "400")
	if got := terminalWidth(); got != 160 {
		t.Errorf("terminalWidth() = %d, want the 160 cap", got)
	}

	t.Setenv("COLUMNS", "not a number")
	if got := terminalWidth(); got <= 0 {
		t.Errorf("terminalWidth() = %d with a bad COLUMNS, want the default", got)
	}

	t.Setenv("COLUMNS", "3")
	if got := terminalWidth(); got <= 20 {
		t.Errorf("terminalWidth() = %d for an implausible width, want the default", got)
	}
}

// fakeCommand puts an executable of the given name first on PATH. It records
// the arguments and environment it was invoked with, so the spawning helpers
// can be tested for real instead of only through their pure parts.
func fakeCommand(t *testing.T, name string) (argsFile, envFile string) {
	t.Helper()

	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args")
	envFile = filepath.Join(dir, "env")

	// The helpers under test replace PATH with the fake's directory, so the
	// script restores a usable one before calling anything but a builtin.
	body := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > " + argsFile + "\n" +
		"PATH=/usr/bin:/bin /usr/bin/env > " + envFile + " 2>/dev/null || true\n"
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil { // #nosec G306 -- must be executable
		t.Fatalf("write fake %s: %v", name, err)
	}

	t.Setenv("PATH", dir)
	return argsFile, envFile
}

// readLines returns a recorded file's non-empty lines.
func readLines(t *testing.T, path string) []string {
	t.Helper()

	body, err := os.ReadFile(path) // #nosec G304 -- path created by this test
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var out []string
	for _, line := range strings.Split(string(body), "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// The spawned terminal must not inherit TMUX, or the new window falls back to
// half-blocks in a terminal that can draw the real image.
func TestOpenInTerminalClearsTmux(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,123,0")
	t.Setenv("TMUX_PANE", "%3")

	argsFile, envFile := fakeCommand(t, "ghostty")

	var stderr strings.Builder
	if err := openInTerminal(t.Context(), "A-4 km 399.1", nil, &stderr); err != nil {
		t.Fatalf("openInTerminal() = %v", err)
	}

	// Start() does not wait, so give the child a moment to record itself.
	waitForFile(t, argsFile)

	args := readLines(t, argsFile)
	if len(args) == 0 || args[0] != "-e" {
		t.Fatalf("ghostty args = %v, want it to start with -e", args)
	}
	if !containsAll(args, "camera", "A-4 km 399.1", "--hold") {
		t.Errorf("ghostty args = %v, want the camera command and --hold", args)
	}

	for _, kv := range readLines(t, envFile) {
		key, _, _ := strings.Cut(kv, "=")
		if key == "TMUX" || key == "TMUX_PANE" {
			t.Errorf("%s was inherited by the spawned terminal", key)
		}
	}
}

func TestOpenInTerminalReportsNoTerminal(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	var stderr strings.Builder
	err := openInTerminal(t.Context(), "A-4", nil, &stderr)

	if !errors.Is(err, ErrNoViewer) {
		t.Fatalf("openInTerminal() = %v, want ErrNoViewer", err)
	}
}

// The frame handed to a viewer must live in RAM and be gone afterwards.
func TestOpenInViewerDeletesTheFrameAfterwards(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", base)

	argsFile, _ := fakeCommand(t, "xdg-open")
	// fakeCommand replaced PATH, so re-point XDG_RUNTIME_DIR after it.
	t.Setenv("XDG_RUNTIME_DIR", base)

	var stderr strings.Builder
	if err := openInViewer(t.Context(), []byte("\xff\xd8\xffnot really a jpeg"), "camera-421", &stderr); err != nil {
		t.Fatalf("openInViewer() = %v", err)
	}

	args := readLines(t, argsFile)
	if len(args) != 1 {
		t.Fatalf("viewer args = %v, want exactly the frame path", args)
	}
	framePath := args[0]

	if !strings.HasPrefix(framePath, base) {
		t.Errorf("frame written to %q, want it under the memory-backed %q", framePath, base)
	}
	// openInViewer blocks until the viewer exits, then removes the file.
	// That is what makes "only while you are looking at it" literally true.
	if _, err := os.Stat(framePath); !os.IsNotExist(err) {
		t.Errorf("the frame survived at %s", framePath)
	}
}

func TestOpenInViewerReportsNoViewer(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	var stderr strings.Builder
	err := openInViewer(t.Context(), []byte("x"), "camera-1", &stderr)

	if !errors.Is(err, ErrNoViewer) {
		t.Fatalf("openInViewer() = %v, want ErrNoViewer", err)
	}
}

// waitForFile polls briefly for a file a spawned process writes.
func waitForFile(t *testing.T, path string) {
	t.Helper()

	for range 100 {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s never appeared", path)
}

// containsAll reports whether every needle is among the values.
func containsAll(values []string, needles ...string) bool {
	for _, n := range needles {
		var found bool
		for _, v := range values {
			if v == n {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
