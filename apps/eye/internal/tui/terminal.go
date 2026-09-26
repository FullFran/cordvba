package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ttyPath is the controlling terminal, which is where a full-screen view has to
// read from.
//
// It is not stdin: `eye tui < /dev/null` and `eye tui | tee frame.txt` are both
// reasonable things to type, and in neither case is stdin the keyboard.
const ttyPath = "/dev/tty"

// sttyTimeout bounds each stty call. It is generous for a process that reads
// one ioctl, and it means a wedged stty cannot wedge the cockpit — including
// on the exit path, where hanging would leave the terminal in raw mode.
const sttyTimeout = 5 * time.Second

// readBuffer is how much is read from the terminal at a time. A pasted line and
// the longest escape sequence both fit comfortably.
const readBuffer = 256

// Terminal is the raw-mode plumbing: the one part of the cockpit that talks to
// the operating system.
//
// Raw mode is set by shelling out to stty rather than by calling tcsetattr,
// because eye has a dependency budget of three and none of them is a terminal
// library (ADR-0004). stty is in POSIX, it is on every machine that has a
// terminal, and if it is missing the cockpit degrades to a refreshing board
// instead of failing.
type Terminal struct {
	tty *os.File

	// saved is the terminal's settings as `stty -g` printed them, so they can
	// be put back exactly. A tool that hands your shell back without echo is
	// a tool you stop running.
	saved string
	raw   bool

	// flag is whichever of -F and -f this machine's stty understands.
	flag string

	// note explains why input is unavailable, when it is.
	note string
}

// OpenTerminal opens the controlling terminal, optionally putting it into raw
// mode.
//
// raw is false when the caller only wants to know how big the terminal is —
// drawing a single frame needs no keyboard, and changing somebody's terminal
// settings to print one screenful and exit would be rude.
//
// It never returns an error. A machine with no /dev/tty or no stty is a machine
// that can still watch a board refresh itself, and crashing there would be a
// worse answer than degrading.
func OpenTerminal(raw bool) *Terminal {
	t := &Terminal{}

	tty, err := os.OpenFile(ttyPath, os.O_RDWR, 0)
	if err != nil {
		t.note = "no " + ttyPath
		return t
	}
	t.tty = tty

	flag, saved, err := savedSettings()
	if err != nil {
		t.note = "stty unavailable: " + err.Error()
		return t
	}
	t.flag, t.saved = flag, saved

	if !raw {
		t.note = "reading the size only"
		return t
	}

	// raw turns off line buffering and signal generation; -echo stops the
	// keystrokes being painted over the board.
	if _, err := stty(flag, "raw", "-echo"); err != nil {
		t.note = "raw mode refused: " + err.Error()
		return t
	}
	t.raw = true
	return t
}

// Interactive reports whether a keyboard is attached.
func (t *Terminal) Interactive() bool { return t.raw && t.tty != nil }

// Note explains why input is unavailable.
func (t *Terminal) Note() string { return t.note }

// File is the open terminal, or nil when there is none.
//
// The reader goroutine takes the handle once and holds it, rather than reading
// the field on every pass: Restore clears the field when the session ends, and
// closing a file while another goroutine is blocked reading it is fine, while
// racing on the field is not.
func (t *Terminal) File() *os.File { return t.tty }

// Size queries the terminal size, falling back to what the caller was told to
// use when the terminal cannot say.
//
// The fallback is a parameter rather than a read of COLUMNS and LINES because
// os.Getenv is confined to internal/config; the command passes it in.
func (t *Terminal) Size(fallback Size) Size {
	if t.flag == "" {
		return fallback
	}

	out, err := stty(t.flag, "size")
	if err != nil {
		return fallback
	}

	fields := strings.Fields(out)
	if len(fields) != 2 {
		return fallback
	}
	rows, rowErr := strconv.Atoi(fields[0])
	cols, colErr := strconv.Atoi(fields[1])
	if rowErr != nil || colErr != nil || rows <= 0 || cols <= 0 {
		return fallback
	}
	return Size{Cols: cols, Rows: rows}
}

// Restore puts the terminal back exactly as it was found.
//
// It must run on every exit path, signals included, and it must be safe to call
// twice: the deferred call and the signal handler can both reach it.
func (t *Terminal) Restore() {
	if t.raw {
		if t.saved != "" {
			_, _ = stty(t.flag, t.saved)
		} else {
			// Without saved settings, "sane" is the honest second best.
			_, _ = stty(t.flag, "sane")
		}
		t.raw = false
	}
	if t.tty != nil {
		_ = t.tty.Close()
		t.tty = nil
	}
}

// savedSettings captures the current terminal settings and works out which
// device flag this machine's stty speaks.
//
// GNU coreutils uses -F and the BSDs use -f. Trying both is two failed
// processes at worst, once per session.
func savedSettings() (flag, saved string, err error) {
	var lastErr error
	for _, candidate := range []string{"-F", "-f"} {
		out, err := stty(candidate, "-g")
		if err == nil && strings.TrimSpace(out) != "" {
			return candidate, strings.TrimSpace(out), nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("stty -g returned nothing")
	}
	return "", "", lastErr
}

// stty runs stty against the controlling terminal.
func stty(args ...string) (string, error) {
	// The device is appended rather than inherited from stdin, because the
	// caller may well have redirected stdin away from the terminal.
	full := make([]string, 0, len(args)+1)
	if len(args) > 0 && (args[0] == "-F" || args[0] == "-f") {
		full = append(full, args[0], ttyPath)
		args = args[1:]
	}
	full = append(full, args...)

	ctx, cancel := context.WithTimeout(context.Background(), sttyTimeout)
	defer cancel()

	// #nosec G204 -- every argument is a package constant or a string stty
	// itself printed from `stty -g`; none of it is user input.
	cmd := exec.CommandContext(ctx, "stty", full...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("stty %s: %w", strings.Join(full, " "), err)
	}
	return string(out), nil
}
