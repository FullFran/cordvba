package render

import (
	"fmt"
	"io"
)

// Screen control sequences, used by the live board.
//
// These are the two-decade-stable parts of the terminal, not a library: the
// alternate buffer, cursor home, erase, and cursor visibility. eye takes over
// the screen, and it must give it back exactly as it found it — including when
// it is interrupted.
const (
	altScreenOn  = "\x1b[?1049h"
	altScreenOff = "\x1b[?1049l"
	cursorHome   = "\x1b[H"
	eraseDown    = "\x1b[0J"
	hideCursor   = "\x1b[?25l"
	showCursor   = "\x1b[?25h"
)

// Screen manages a full-screen live view.
type Screen struct {
	w       io.Writer
	enabled bool
}

// NewScreen prepares a live view on a writer. On a non-terminal it degrades to
// plain sequential output, so `eye watch | tee log.txt` still produces
// something readable rather than a stream of cursor moves.
func NewScreen(w io.Writer) *Screen {
	return &Screen{w: w, enabled: isTerminalWriter(w)}
}

// Enter takes over the screen.
func (s *Screen) Enter() {
	if !s.enabled {
		return
	}
	_, _ = io.WriteString(s.w, altScreenOn+hideCursor)
}

// Leave restores the terminal to how it was found.
//
// This must run on every exit path, signals included. A tool that leaves your
// cursor hidden is a tool you stop trusting with your terminal.
func (s *Screen) Leave() {
	if !s.enabled {
		return
	}
	_, _ = io.WriteString(s.w, showCursor+altScreenOff)
}

// Frame clears and redraws from the top.
//
// It erases downward from the cursor rather than clearing the whole screen
// first: clearing makes the display blink on every tick, and a board that
// flickers is one you stop watching.
func (s *Screen) Frame(draw func(io.Writer)) {
	if s.enabled {
		_, _ = io.WriteString(s.w, cursorHome)
	}
	draw(s.w)
	if s.enabled {
		_, _ = io.WriteString(s.w, eraseDown)
	}
}

// Separator writes a divider between frames in non-terminal output, where
// there is no screen to redraw.
func (s *Screen) Separator() {
	if s.enabled {
		return
	}
	_, _ = fmt.Fprintln(s.w)
}
