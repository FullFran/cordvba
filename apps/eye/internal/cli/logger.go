package cli

import (
	"log/slog"

	application "github.com/FullFran/cordvba/apps/eye/internal/observation/application"
)

// loggerAlias names the logger type these commands pass around. It exists only
// so the daemon's signatures read as intent rather than as import plumbing.
type loggerAlias = slog.Logger

// applicationResult names the collector's per-source outcome, so the live board
// can hold them without its signatures reading as import plumbing.
type applicationResult = application.Result
