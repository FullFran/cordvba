//go:build !unix

package tui

import "os"

// resizeSignals is empty on platforms with no window-change signal. The size is
// still re-queried on every store read, so the board catches up on the next
// tick rather than never.
func resizeSignals() []os.Signal { return nil }
