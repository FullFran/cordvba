package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/FullFran/eye/internal/render"
)

// Reader is everything the cockpit needs from the outside world.
//
// The model never calls it: the runner does, on the model's instruction. That
// separation is what lets every screen be rendered into a bytes.Buffer in a
// test with no store, no registry and no network.
type Reader interface {
	// Read takes a picture of the store. It touches no network.
	Read(ctx context.Context) (Data, error)
	// Poll fetches one source live and reports what happened.
	Poll(ctx context.Context, sourceID string) (string, error)
	// Arrivals fetches live estimates for one stop.
	Arrivals(ctx context.Context, stopID string) ([]Arrival, error)
}

// RunOptions configure one cockpit session.
type RunOptions struct {
	Options

	// Out is where frames are written.
	Out io.Writer
	// Reader supplies the data.
	Reader Reader
	// Fallback is the size to use when the terminal cannot be asked.
	Fallback Size
	// Once draws a single frame and returns, which is what makes the whole
	// thing checkable from a script.
	Once bool
}

// clockTick is how often the header clock is redrawn. It is separate from the
// store interval on purpose: the clock has to move every second, and re-reading
// the store every second would be a busy loop over somebody's disk.
const clockTick = time.Second

// Run opens the terminal, draws the cockpit and gives the terminal back.
//
// Every exit path restores the terminal — a normal quit, a cancelled context, a
// signal, or a panic on the way out — because a tool that leaves your shell
// without an echo is a tool you stop trusting with your terminal.
func Run(ctx context.Context, opts RunOptions) error {
	if opts.Fallback.Cols <= 0 || opts.Fallback.Rows <= 0 {
		opts.Fallback = defaultSize
	}
	if opts.Interval <= 0 {
		opts.Interval = defaultInterval
	}

	// A single frame needs the size and nothing else, so it never touches the
	// terminal's settings.
	terminal := OpenTerminal(!opts.Once)
	defer terminal.Restore()

	opts.Interactive = terminal.Interactive()
	opts.InputNote = inputNote(terminal, opts)

	model := New(opts.Options)
	size := terminal.Size(opts.Fallback)
	model.Update(Event{Kind: EventResize, Size: size, At: time.Now()})

	screen := render.NewScreen(opts.Out)
	screen.Enter()
	defer screen.Leave()

	events := make(chan Event, 64)
	runner := &loop{opts: opts, model: model, screen: screen, terminal: terminal, events: events}

	runner.refresh(ctx)
	runner.draw(size)
	if opts.Once {
		return nil
	}
	return runner.run(ctx, size)
}

// inputNote explains, in one clause, why there is no keyboard.
func inputNote(terminal *Terminal, opts RunOptions) string {
	if terminal.Interactive() {
		return ""
	}
	if opts.Once {
		return "one frame, then out"
	}
	return terminal.Note() + " · refreshing every " + shortDuration(opts.Interval)
}

// defaultSize is the terminal every terminal is at least. It is a constant
// rather than a read of COLUMNS and LINES, because os.Getenv belongs to
// internal/config; the command passes a real fallback in.
var defaultSize = Size{Cols: 80, Rows: 24}

// defaultInterval is how often the store is re-read when the caller names no
// interval. A ticker built from zero panics, so this is a guard as well as a
// default.
const defaultInterval = 5 * time.Second

// loop owns the running session. Only this goroutine touches the model.
type loop struct {
	opts     RunOptions
	model    *Model
	screen   *render.Screen
	terminal *Terminal
	events   chan Event
}

// run drives the session until the model quits or the context is cancelled.
func (l *loop) run(ctx context.Context, size Size) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if l.terminal.Interactive() {
		// The file is captured here rather than read inside the goroutine:
		// Restore clears the field on this goroutine when the session ends,
		// and reading it from two goroutines would be a race the detector
		// is right to complain about.
		go l.readKeys(ctx, l.terminal.File())
	}

	signals := make(chan os.Signal, 4)
	signal.Notify(signals, append(resizeSignals(), syscall.SIGINT, syscall.SIGTERM)...)
	defer signal.Stop(signals)

	clock := time.NewTicker(clockTick)
	defer clock.Stop()
	store := time.NewTicker(l.opts.Interval)
	defer store.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		case sig := <-signals:
			if isResize(sig) {
				size = l.terminal.Size(l.opts.Fallback)
				l.model.Update(Event{Kind: EventResize, Size: size, At: time.Now()})
				l.draw(size)
				continue
			}
			// SIGINT and SIGTERM leave through the same door as q, so the
			// deferred restore runs.
			return nil

		case <-clock.C:
			l.model.Update(Event{Kind: EventTick, At: time.Now()})
			l.draw(size)

		case <-store.C:
			l.refresh(ctx)
			l.draw(size)

		case e := <-l.events:
			cmd := l.model.Update(e)
			if l.dispatch(ctx, cmd) {
				return nil
			}
			// A note only ever follows a live poll, and a live poll writes
			// what it fetched into the store, so the board has to re-read
			// or it shows the result of the previous poll under the new
			// message.
			if e.Kind == EventNote {
				l.refresh(ctx)
			}
			l.draw(size)
		}
	}
}

// dispatch acts on what the model asked for, and reports whether to stop.
func (l *loop) dispatch(ctx context.Context, cmd Command) (done bool) {
	switch cmd.Action {
	case ActionQuit:
		return true
	case ActionRefresh:
		l.refresh(ctx)
	case ActionPollSource:
		go l.poll(ctx, cmd.Target)
	case ActionArrivals:
		go l.arrivals(ctx, cmd.Target)
	}
	return false
}

// refresh re-reads the store, in the foreground: it is local, and a board that
// redraws before its data has landed shows the previous frame's numbers under
// the current frame's clock.
func (l *loop) refresh(ctx context.Context) {
	data, err := l.opts.Reader.Read(ctx)
	data.Err = err
	l.model.Update(Event{Kind: EventData, Data: &data, At: time.Now()})
}

// poll fetches one source in the background, so a slow publisher cannot freeze
// the board.
func (l *loop) poll(ctx context.Context, id string) {
	note, err := l.opts.Reader.Poll(ctx, id)
	l.post(ctx, Event{Kind: EventNote, Note: note, Err: err, At: time.Now()})
}

// arrivals fetches live estimates in the background.
func (l *loop) arrivals(ctx context.Context, stop string) {
	list, err := l.opts.Reader.Arrivals(ctx, stop)
	note := ""
	if err == nil {
		note = fmt.Sprintf("stop %s · %d arrivals · operator estimate", stop, len(list))
	}
	l.post(ctx, Event{Kind: EventArrivals, Arrivals: list, Note: note, Err: err, At: time.Now()})
}

// post delivers an event without blocking a background fetch on a full queue.
func (l *loop) post(ctx context.Context, e Event) {
	select {
	case l.events <- e:
	case <-ctx.Done():
	}
}

// readKeys decodes the terminal into events until the terminal is closed.
func (l *loop) readKeys(ctx context.Context, tty *os.File) {
	if tty == nil {
		return
	}

	var pending []byte
	buf := make([]byte, readBuffer)

	for {
		n, err := tty.Read(buf)
		if n > 0 {
			// A fresh slice each time: DecodeKeys hands back a subslice of
			// what it was given, and appending onto that would rewrite the
			// bytes it is still holding.
			chunk := make([]byte, 0, len(pending)+n)
			chunk = append(chunk, pending...)
			chunk = append(chunk, buf[:n]...)

			keys, rest := DecodeKeys(chunk)
			pending = rest
			for _, k := range keys {
				l.post(ctx, Event{Kind: EventKey, Key: k, At: time.Now()})
			}
		}
		if err != nil || ctx.Err() != nil {
			return
		}
	}
}

// draw renders one frame.
func (l *loop) draw(size Size) {
	l.screen.Frame(func(w io.Writer) {
		l.model.View(w, size)
	})
	l.screen.Separator()
}

// isResize reports whether a signal is a window change.
func isResize(sig os.Signal) bool {
	for _, s := range resizeSignals() {
		if sig == s {
			return true
		}
	}
	return false
}
