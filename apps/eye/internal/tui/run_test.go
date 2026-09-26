package tui

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	"github.com/FullFran/cordvba/apps/eye/internal/render"
)

// errFake stands in for a source that could not be reached.
var errFake = errors.New("503 from upstream")

// fakeReader answers without a store, a registry or a network.
type fakeReader struct {
	mu       sync.Mutex
	reads    int
	polls    []string
	stops    []string
	data     Data
	readErr  error
	pollErr  error
	arrivals []Arrival
}

func (f *fakeReader) Read(context.Context) (Data, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.reads++
	f.data.ReadAt = time.Now()
	return f.data, f.readErr
}

func (f *fakeReader) Poll(_ context.Context, id string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.polls = append(f.polls, id)
	return id + " · 3 records", f.pollErr
}

func (f *fakeReader) Arrivals(_ context.Context, stop string) ([]Arrival, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.stops = append(f.stops, stop)
	return f.arrivals, nil
}

func (f *fakeReader) counts() (reads int, polls, stops []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads, append([]string(nil), f.polls...), append([]string(nil), f.stops...)
}

// runOptions builds a session that touches nothing outside the test.
func runOptions(out *bytes.Buffer, reader Reader, once bool) RunOptions {
	return RunOptions{
		Options: Options{
			Version: "test", Registry: "(test)", Theme: render.PlainTheme(),
			Interval: 5 * time.Millisecond,
		},
		Out:      out,
		Reader:   reader,
		Fallback: Size{Cols: 100, Rows: 30},
		Once:     once,
	}
}

// One frame and out is what makes the cockpit checkable from a script, and it
// is the mode every test and every pipe runs in.
func TestRunDrawsOneFrameAndReturns(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	reader := &fakeReader{}

	if err := Run(context.Background(), runOptions(&out, reader, true)); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if reads, _, _ := reader.counts(); reads != 1 {
		t.Errorf("a single frame read the store %d times", reads)
	}
	if !strings.Contains(out.String(), "EYE") {
		t.Errorf("no frame was written:\n%s", out.String())
	}
}

// Drawing one frame must not change the operator's terminal settings. Raw mode
// for a screenful that exits immediately would swallow a keystroke for nothing.
func TestASingleFrameNeverEntersRawMode(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := Run(context.Background(), runOptions(&out, &fakeReader{}, true)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "one frame, then out") {
		t.Errorf("the frame does not say it is a one-shot:\n%s", out.String())
	}
}

func TestRunKeepsReadingUntilTheContextIsCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	var out bytes.Buffer
	reader := &fakeReader{}

	done := make(chan error, 1)
	go func() { done <- Run(ctx, runOptions(&out, reader, false)) }()

	// Long enough for several store ticks at the 5ms interval above.
	time.Sleep(60 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after its context was cancelled")
	}

	if reads, _, _ := reader.counts(); reads < 2 {
		t.Errorf("the store was read %d times over several intervals", reads)
	}
}

// A store that cannot be read has to reach the screen, not the bin.
func TestRunShowsAFailedRead(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	reader := &fakeReader{readErr: errFake}

	if err := Run(context.Background(), runOptions(&out, reader, true)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "could not read the store") {
		t.Errorf("the failed read is not on the frame:\n%s", out.String())
	}
}

// newLoop builds a runner with no terminal behind it.
func newLoop(out *bytes.Buffer, reader Reader) *loop {
	opts := runOptions(out, reader, false)
	return &loop{
		opts:     opts,
		model:    New(opts.Options),
		screen:   render.NewScreen(out),
		terminal: &Terminal{},
		events:   make(chan Event, 8),
	}
}

func TestDispatchQuits(t *testing.T) {
	t.Parallel()

	l := newLoop(&bytes.Buffer{}, &fakeReader{})
	if !l.dispatch(context.Background(), Command{Action: ActionQuit}) {
		t.Error("ActionQuit did not stop the session")
	}
}

func TestDispatchRefreshReadsTheStore(t *testing.T) {
	t.Parallel()

	reader := &fakeReader{}
	l := newLoop(&bytes.Buffer{}, reader)

	l.dispatch(context.Background(), Command{Action: ActionRefresh})
	if reads, _, _ := reader.counts(); reads != 1 {
		t.Errorf("ActionRefresh read the store %d times", reads)
	}
}

// A live poll runs in the background, so a slow publisher cannot freeze the
// board, and it comes back as a note the operator can read.
func TestDispatchPollComesBackAsANote(t *testing.T) {
	t.Parallel()

	reader := &fakeReader{}
	l := newLoop(&bytes.Buffer{}, reader)

	l.dispatch(context.Background(), Command{Action: ActionPollSource, Target: "dgt-incidents"})

	select {
	case e := <-l.events:
		if e.Kind != EventNote || !strings.Contains(e.Note, "dgt-incidents") {
			t.Errorf("poll produced %+v", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the poll never reported back")
	}

	if _, polls, _ := reader.counts(); len(polls) != 1 || polls[0] != "dgt-incidents" {
		t.Errorf("polled %v", polls)
	}
}

func TestDispatchArrivalsComesBackAsAReading(t *testing.T) {
	t.Parallel()

	reader := &fakeReader{arrivals: []Arrival{{StopID: "401", Line: "3", Minutes: 4}}}
	l := newLoop(&bytes.Buffer{}, reader)

	l.dispatch(context.Background(), Command{Action: ActionArrivals, Target: "401"})

	select {
	case e := <-l.events:
		if e.Kind != EventArrivals || len(e.Arrivals) != 1 {
			t.Errorf("arrivals produced %+v", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the arrivals request never reported back")
	}

	if _, _, stops := reader.counts(); len(stops) != 1 || stops[0] != "401" {
		t.Errorf("asked about %v", stops)
	}
}

// post must not block a background fetch forever when nobody is draining the
// queue and the session is already over.
func TestPostGivesUpWhenTheSessionEnds(t *testing.T) {
	t.Parallel()

	l := newLoop(&bytes.Buffer{}, &fakeReader{})
	ctx, cancel := context.WithCancel(context.Background())

	for range cap(l.events) {
		l.events <- Event{Kind: EventTick}
	}
	cancel()

	done := make(chan struct{})
	go func() {
		l.post(ctx, Event{Kind: EventTick})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("post blocked on a full queue after the session ended")
	}
}

// Without a terminal the plumbing has to degrade rather than fail: `eye tui`
// in a pipe is a reasonable thing to type.
func TestTerminalWithoutADeviceDegrades(t *testing.T) {
	t.Parallel()

	empty := &Terminal{}
	if empty.Interactive() {
		t.Error("a terminal with no device claims to be interactive")
	}
	if empty.File() != nil {
		t.Error("a terminal with no device handed out a file")
	}

	fallback := Size{Cols: 132, Rows: 43}
	if got := empty.Size(fallback); got != fallback {
		t.Errorf("Size() = %+v with no terminal to ask, want the fallback", got)
	}
}

// Restore runs on every exit path, and more than one of them can reach it.
func TestRestoreIsSafeToCallTwice(t *testing.T) {
	t.Parallel()

	terminal := &Terminal{}
	terminal.Restore()
	terminal.Restore()
}

func TestIsResizeRecognisesOnlyTheWindowSignal(t *testing.T) {
	t.Parallel()

	signals := resizeSignals()
	for _, sig := range signals {
		if !isResize(sig) {
			t.Errorf("%v is not recognised as a resize", sig)
		}
	}
	if isResize(errSignal{}) {
		t.Error("an unrelated signal was taken for a resize")
	}
}

// errSignal is a signal that is not a window change.
type errSignal struct{}

func (errSignal) String() string { return "not-a-resize" }
func (errSignal) Signal()        {}

func TestParseViewNamesEveryScreen(t *testing.T) {
	t.Parallel()

	for i, name := range viewNames {
		got, err := ParseView(strings.ToLower(name))
		if err != nil {
			t.Fatalf("ParseView(%q): %v", name, err)
		}
		if got != ViewID(i) {
			t.Errorf("ParseView(%q) = %v", name, got)
		}
		if got.String() != name {
			t.Errorf("%v.String() = %q, want %q", got, got.String(), name)
		}
	}

	if _, err := ParseView("sonar"); err == nil {
		t.Error("an unknown view was accepted")
	}
	if got := ViewID(99).String(); got != "UNKNOWN" {
		t.Errorf("ViewID(99).String() = %q", got)
	}
}

// The three states a source can be in are three different problems, and the
// board must not flatten them into one.
func TestSourceRowState(t *testing.T) {
	t.Parallel()

	now := time.Now()
	cases := []struct {
		name string
		row  SourceRow
		want string
	}{
		{
			name: "live",
			row:  SourceRow{Automation: "enabled", HasAdapter: true, Pollable: true, LastSuccess: now},
			want: "live",
		},
		{
			name: "stale",
			row:  SourceRow{Automation: "enabled", HasAdapter: true, Pollable: true, LastSuccess: now, Stale: true},
			want: "stale",
		},
		{
			name: "failing",
			row:  SourceRow{Automation: "enabled", HasAdapter: true, Pollable: true, LastError: "boom"},
			want: "failing",
		},
		{
			name: "never polled",
			row:  SourceRow{Automation: "enabled", HasAdapter: true, Pollable: true},
			want: "never polled",
		},
		{
			name: "held for its licence",
			row:  SourceRow{Automation: "review_terms"},
			want: "review_terms",
		},
		{
			name: "permitted but unreadable by this build",
			row:  SourceRow{Automation: "enabled", HasAdapter: false},
			want: "no adapter",
		},
		{
			name: "permitted but the machine has not opted in",
			row:  SourceRow{Automation: "enabled", HasAdapter: true, Pollable: false},
			want: "held",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := tc.row.State(); got != tc.want {
				t.Errorf("State() = %q, want %q", got, tc.want)
			}
			// Every state must map onto an indicator, or the dot beside it
			// is meaningless.
			if tc.row.indicator() == "" {
				t.Error("the state has no indicator")
			}
		})
	}
}

// The problems come first. A board where the broken source is the last thing
// you scroll to is a board you stop trusting.
func TestHealthRankPutsProblemsFirst(t *testing.T) {
	t.Parallel()

	failing := SourceRow{Automation: "enabled", HasAdapter: true, Pollable: true, LastError: "boom"}
	live := SourceRow{Automation: "enabled", HasAdapter: true, Pollable: true, LastSuccess: time.Now()}
	held := SourceRow{Automation: "review_terms"}

	if healthRank(failing) >= healthRank(live) {
		t.Error("a failing source does not sort above a live one")
	}
	if healthRank(live) >= healthRank(held) {
		t.Error("a live source does not sort above a held one")
	}
}

func TestArrivalCountsDownAndGoesStale(t *testing.T) {
	t.Parallel()

	read := time.Date(2026, 9, 3, 15, 0, 0, 0, time.UTC)
	a := Arrival{Minutes: 4, ReadAt: read}

	if a.Stale(read.Add(10 * time.Second)) {
		t.Error("a ten-second-old estimate is already stale")
	}
	if !a.Stale(read.Add(5 * time.Minute)) {
		t.Error("a five-minute-old estimate is still presented as live")
	}
	if got := a.ETA(read.Add(time.Minute)); got != 3*time.Minute {
		t.Errorf("ETA after a minute = %s, want 3m", got)
	}

	// An estimate with no reading time behind it is never live.
	if !(Arrival{Minutes: 2}).Stale(read) {
		t.Error("an estimate with no read time passed as live")
	}
}

func TestDeparturesFromKeepsOnlyTheFutureAndOrdersThem(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 3, 15, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *time.Time { v := now.Add(d); return &v }

	records := []observation.Record{
		{Kind: departureKind, Title: "later", ValidFrom: at(2 * time.Hour)},
		{Kind: departureKind, Title: "past", ValidFrom: at(-time.Hour)},
		{Kind: departureKind, Title: "soon", ValidFrom: at(10 * time.Minute)},
		{Kind: departureKind, Title: "no time"},
		{Kind: "article", Title: "not a departure", ValidFrom: at(time.Minute)},
	}

	got := departuresFrom(records, now)
	if len(got) != 2 {
		t.Fatalf("departuresFrom kept %d of them: %+v", len(got), got)
	}
	if got[0].Title != "soon" || got[1].Title != "later" {
		t.Errorf("departures are not soonest-first: %s then %s", got[0].Title, got[1].Title)
	}
}

// The stop number the arrivals endpoint expects is the one on the pole, which
// the adapter puts in the payload. Falling back to the entity id is what keeps
// an older store readable.
func TestStopsFromReadsTheNumberOnThePole(t *testing.T) {
	t.Parallel()

	entities := []observation.Entity{
		{ID: "aucorsa:1", Kind: "bus_stop", Title: "From the payload", Payload: []byte(`{"stop_id":"401"}`)},
		{ID: "aucorsa:233", Kind: "bus_stop", Title: "From the id"},
		{ID: "no-colon", Kind: "bus_stop", Title: "Neither"},
		{ID: "cam", Kind: "camera", Title: "Not a stop"},
	}

	got := stopsFrom(entities)

	// Two, not three: an entity with no resolvable stop number is dropped
	// rather than listed. Offering a stop the arrivals endpoint cannot be
	// asked about would be a row that silently returns nothing.
	if len(got) != 2 {
		t.Fatalf("stopsFrom returned %d stops: %+v", len(got), got)
	}
	if got[0].ID != "401" {
		t.Errorf("the payload stop number was ignored: %q", got[0].ID)
	}
	if got[1].ID != "233" {
		t.Errorf("the fallback did not read the entity id: %q", got[1].ID)
	}
	for _, s := range got {
		if s.Name == "Neither" {
			t.Error("a stop with no number on the pole was listed anyway")
		}
	}
}

// Kind decides the glyph and severity decides the colour, so a serious incident
// still looks like an incident.
func TestGlyphFor(t *testing.T) {
	t.Parallel()

	if glyph, colour := glyphFor(feature{topic: "air"}); glyph != '▲' || colour != render.ColorCyan {
		t.Errorf("air = %q/%v", glyph, colour)
	}
	if glyph, colour := glyphFor(feature{topic: "nothing known"}); glyph != '·' || colour != render.ColorSlate {
		t.Errorf("an unknown topic = %q/%v", glyph, colour)
	}
	if _, colour := glyphFor(feature{topic: "traffic", severity: observation.SeverityCritical}); colour != render.ColorCrimson {
		t.Errorf("a critical observation is not drawn as one: %v", colour)
	}
	if glyph, colour := glyphFor(feature{topic: "traffic", inventory: true}); glyph != '□' || colour != render.ColorSlate {
		t.Errorf("inventory competes with live observations: %q/%v", glyph, colour)
	}
}

func TestShortAgeAndDuration(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 3, 15, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		at   time.Time
		want string
	}{
		{name: "never", at: time.Time{}, want: "—"},
		{name: "seconds", at: now.Add(-30 * time.Second), want: "30s"},
		{name: "minutes", at: now.Add(-30 * time.Minute), want: "30m"},
		{name: "hours", at: now.Add(-5 * time.Hour), want: "5h"},
		{name: "days", at: now.Add(-72 * time.Hour), want: "3d"},
		{name: "the future is marked as the future", at: now.Add(10 * time.Minute), want: "in 10m"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := shortAge(tc.at, now); got != tc.want {
				t.Errorf("shortAge = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPluralAndOrDash(t *testing.T) {
	t.Parallel()

	if got := plural(1, "record", "records"); got != "1 record" {
		t.Errorf("plural(1) = %q", got)
	}
	if got := plural(0, "record", "records"); got != "0 records" {
		t.Errorf("plural(0) = %q", got)
	}
	if got := orDash("  "); got != "—" {
		t.Errorf("orDash(blank) = %q", got)
	}
}

// A path is identified by its tail, so that is the half that survives.
func TestTailOfKeepsTheEnd(t *testing.T) {
	t.Parallel()

	if got := tailOf("/home/someone/.config/eye/sources.yaml", 20); !strings.HasSuffix(got, "sources.yaml") {
		t.Errorf("tailOf dropped the filename: %q", got)
	}
	if got := tailOf("short", 20); got != "short" {
		t.Errorf("tailOf shortened a short path: %q", got)
	}
}
