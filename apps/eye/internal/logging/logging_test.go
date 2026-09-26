package logging_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FullFran/cordvba/apps/eye/internal/logging"
)

func TestLevelOf(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want slog.Level
	}{
		{name: "debug", in: "debug", want: slog.LevelDebug},
		{name: "info", in: "info", want: slog.LevelInfo},
		{name: "warn", in: "warn", want: slog.LevelWarn},
		{name: "error", in: "error", want: slog.LevelError},
		{name: "case insensitive", in: "WARN", want: slog.LevelWarn},
		{name: "padded", in: "  debug  ", want: slog.LevelDebug},
		{name: "unknown falls back to info", in: "shout", want: slog.LevelInfo},
		{name: "empty falls back to info", in: "", want: slog.LevelInfo},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := logging.LevelOf(tc.in); got != tc.want {
				t.Errorf("LevelOf(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// A non-terminal writer gets JSON, which is what a daemon under systemd needs.
func TestNewWritesJSONToANonTerminal(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logging.New(logging.Options{Level: "info", Writer: &buf}).
		Info("poll complete", "source", "diario-cordoba", "records", 25)

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("output is not JSON: %v (%q)", err, buf.String())
	}
	if entry["msg"] != "poll complete" {
		t.Errorf("msg = %v", entry["msg"])
	}
	if entry["source"] != "diario-cordoba" {
		t.Errorf("structured field lost: %v", entry)
	}
}

func TestNewRespectsLevel(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	log := logging.New(logging.Options{Level: "warn", Writer: &buf})

	log.Info("should not appear")
	log.Warn("should appear")

	out := buf.String()
	if strings.Contains(out, "should not appear") {
		t.Errorf("info was logged at warn level: %q", out)
	}
	if !strings.Contains(out, "should appear") {
		t.Errorf("warn was not logged: %q", out)
	}
}

func TestDiscardWritesNothing(t *testing.T) {
	t.Parallel()

	log := logging.Discard()
	log.Error("even errors are discarded")
	// Nothing to assert beyond it not panicking; the point is that a
	// command whose output is its answer stays silent.
}

func TestNewForcesJSON(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logging.New(logging.Options{Level: "debug", Writer: &buf, JSON: true}).Debug("forced")

	if !json.Valid(buf.Bytes()) {
		t.Errorf("JSON was requested but output is not JSON: %q", buf.String())
	}
}

// Log lines must never contaminate a command's stdout: `eye news --json | jq`
// has to keep working while the daemon is chatty.
func TestNewDefaultsToStderr(t *testing.T) {
	t.Parallel()

	log := logging.New(logging.Options{Level: "info"})
	if log == nil {
		t.Fatal("New() returned nil")
	}
	log.Info("goes to stderr, not stdout")
}

func TestNewWithAFileWriter(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "eye.log")
	f, err := os.Create(path) // #nosec G304 -- path created by this test inside t.TempDir()
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _ = f.Close() }()

	// A regular file is not a terminal, so the handler must be JSON.
	logging.New(logging.Options{Level: "info", Writer: f}).Info("to a file")

	body, err := os.ReadFile(path) // #nosec G304 -- path created by this test
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !json.Valid(body) {
		t.Errorf("file output is not JSON: %q", body)
	}
}
