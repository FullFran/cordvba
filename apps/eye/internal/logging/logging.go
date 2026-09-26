// Package logging builds the structured logger. eye uses log/slog and nothing
// else — a logging library is not in the dependency budget, and slog covers
// every need this project has.
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Levels accepted by New, matching EYE_LOG_LEVEL.
var levels = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

// Options configures the logger.
type Options struct {
	// Level is one of debug, info, warn, error. Anything else falls back to
	// info rather than failing: config validation already rejected a bad
	// value, and a logger is not the place to die.
	Level string
	// Writer receives the output. Defaults to os.Stderr, so log lines never
	// contaminate a command's stdout — `eye news --json | jq` must keep
	// working while the daemon is chatty.
	Writer io.Writer
	// JSON forces machine-readable output. When false, the handler is
	// chosen from whether Writer is a terminal.
	JSON bool
}

// New builds a logger from the options.
func New(opts Options) *slog.Logger {
	w := opts.Writer
	if w == nil {
		w = os.Stderr
	}

	handlerOpts := &slog.HandlerOptions{Level: LevelOf(opts.Level)}

	if opts.JSON || !isTerminal(w) {
		return slog.New(slog.NewJSONHandler(w, handlerOpts))
	}
	return slog.New(slog.NewTextHandler(w, handlerOpts))
}

// LevelOf maps a configured level name onto slog's level.
func LevelOf(name string) slog.Level {
	if level, ok := levels[strings.ToLower(strings.TrimSpace(name))]; ok {
		return level
	}
	return slog.LevelInfo
}

// Discard returns a logger that writes nothing, for tests and for commands
// whose output is the answer rather than a log.
func Discard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

// isTerminal reports whether the writer is an interactive terminal. It is a
// character device check rather than a dependency on a TTY library.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
