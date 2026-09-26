//go:build unix

package tui

import (
	"os"
	"syscall"
)

// resizeSignals is the signal a terminal sends when its window changes.
//
// It lives behind a build tag because SIGWINCH does not exist everywhere Go
// compiles, and a cockpit that will not build on a platform it degrades
// gracefully on is a worse answer than one that ignores resizes there.
func resizeSignals() []os.Signal { return []os.Signal{syscall.SIGWINCH} }
