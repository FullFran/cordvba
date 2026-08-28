package cli

import "log/slog"

// loggerAlias names the logger type these commands pass around. It exists only
// so the daemon's signatures read as intent rather than as import plumbing.
type loggerAlias = slog.Logger
