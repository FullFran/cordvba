package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNoViewer is returned when no way of showing an image outside the terminal
// could be found.
var ErrNoViewer = errors.New("no image viewer available")

// ramDir returns a memory-backed directory for a frame that a viewer has to
// open as a file.
//
// This is how `--open` stays inside ADR-0007. The rule is that a camera frame
// never reaches persistent storage, and tmpfs is RAM: XDG_RUNTIME_DIR and
// /dev/shm are both memory filesystems, wiped on reboot and never written to a
// disk. The frame is deleted the moment the viewer exits.
//
// If neither is available the answer is to refuse, not to fall back to /tmp —
// /tmp is a real directory on a real disk on plenty of systems.
func ramDir() (string, error) {
	candidates := []string{os.Getenv("XDG_RUNTIME_DIR"), "/dev/shm"}

	for _, base := range candidates {
		if base == "" {
			continue
		}
		info, err := os.Stat(base) // #nosec G703 -- base comes from a fixed candidate list, not from user input
		if err != nil || !info.IsDir() {
			continue
		}

		dir := filepath.Join(base, "eye-frames")
		if err := os.MkdirAll(dir, 0o700); err != nil { // #nosec G703 -- dir is a fixed name under a vetted tmpfs base
			continue
		}
		return dir, nil
	}

	return "", fmt.Errorf("%w: no memory-backed directory (XDG_RUNTIME_DIR or /dev/shm) is available, and eye will not write a camera frame to disk", ErrNoViewer)
}

// imageViewers are tried in order for --open.
var imageViewers = []string{"xdg-open", "eog", "loupe", "imv", "nsxiv", "feh", "gwenview", "eom"}

// openInViewer shows a frame in the system image viewer and blocks until it is
// closed, then deletes the file.
//
// Blocking is deliberate rather than an oversight: it is what makes "the frame
// exists only while you are looking at it" literally true instead of a promise.
func openInViewer(ctx context.Context, body []byte, name string, stderr io.Writer) error {
	viewer := firstAvailable(imageViewers)
	if viewer == "" {
		return fmt.Errorf("%w: install one of %s", ErrNoViewer, strings.Join(imageViewers[1:4], ", "))
	}

	dir, err := ramDir()
	if err != nil {
		return err
	}

	path := filepath.Join(dir, name+".jpg")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return fmt.Errorf("write frame to memory: %w", err)
	}
	defer func() { _ = os.Remove(path) }()

	_, _ = fmt.Fprintf(stderr, "Opening in %s. The frame lives in %s and is deleted when you close it.\n", viewer, dir)

	cmd := exec.CommandContext(ctx, viewer, path) // #nosec G204 -- viewer chosen from a fixed allow-list, path built by eye
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", viewer, err)
	}
	return nil
}

// terminalWindows are tried in order for --window.
var terminalWindows = []string{"ghostty", "kitty", "wezterm", "alacritty", "foot"}

// openInTerminal re-runs eye in a fresh graphical terminal, outside tmux.
//
// Nothing is written anywhere: the new terminal simply runs the same command
// again, and a terminal that speaks the Kitty graphics protocol will draw the
// frame at real resolution rather than as coloured half-blocks.
func openInTerminal(ctx context.Context, cameraRef string, extra []string, stderr io.Writer) error {
	term := firstAvailable(terminalWindows)
	if term == "" {
		return fmt.Errorf("%w: no graphical terminal found (%s)", ErrNoViewer, strings.Join(terminalWindows, ", "))
	}

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate eye: %w", err)
	}

	inner := append([]string{self, "camera", cameraRef, "--hold"}, extra...)
	args := append(terminalExecFlag(term), inner...)

	_, _ = fmt.Fprintf(stderr, "Opening a %s window outside tmux.\n", term)

	cmd := exec.CommandContext(ctx, term, args...) // #nosec G204 -- terminal chosen from a fixed allow-list
	cmd.Stderr = stderr

	// TMUX is cleared so the child does not think it is inside a multiplexer
	// and fall back to half-blocks in a terminal that can do better.
	cmd.Env = append(environWithout("TMUX", "TMUX_PANE"), "TERM=xterm-kitty")

	return cmd.Start()
}

// terminalExecFlag is how each terminal is told to run a command.
func terminalExecFlag(term string) []string {
	switch term {
	case "ghostty":
		return []string{"-e"}
	case "wezterm":
		return []string{"start", "--"}
	default:
		return []string{"-e"}
	}
}

// environWithout returns the environment minus the given variables.
func environWithout(drop ...string) []string {
	skip := make(map[string]bool, len(drop))
	for _, d := range drop {
		skip[d] = true
	}

	out := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if !skip[key] {
			out = append(out, kv)
		}
	}
	return out
}

// firstAvailable returns the first command on PATH.
func firstAvailable(candidates []string) string {
	for _, c := range candidates {
		if _, err := exec.LookPath(c); err == nil {
			return c
		}
	}
	return ""
}
