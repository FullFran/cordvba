// Package tui is the full-screen cockpit: one terminal view over everything
// eye knows about Cordoba.
//
// It is built out of the standard library and the drawing primitives in
// internal/render, with no terminal framework behind it. That is ADR-0004
// applied rather than quoted: the dependency budget has three entries and none
// of them is a UI toolkit, so raw mode is a call to stty, input is decoded from
// bytes here, and the map is drawn in braille.
//
// The package is split so that almost all of it can be tested with a
// bytes.Buffer. Model holds the state, Update is a pure transition over
// Events, and View renders to an io.Writer at a given size. Only terminal.go
// touches the operating system.
package tui

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/FullFran/cordvba/apps/eye/internal/render"
)

// ViewID names one of the cockpit's screens.
type ViewID int

// The screens, in the order Tab visits them.
const (
	ViewDashboard ViewID = iota
	ViewMap
	ViewFeed
	ViewSources
	ViewTransit
)

// viewNames are the labels shown in the header. They are also what the number
// keys select, in this order.
var viewNames = []string{"DASHBOARD", "MAP", "FEED", "SOURCES", "TRANSIT"}

// ParseView resolves a screen name, so a command can open straight onto one.
func ParseView(name string) (ViewID, error) {
	for i, view := range viewNames {
		if strings.EqualFold(name, view) {
			return ViewID(i), nil
		}
	}
	return 0, fmt.Errorf("no such view %q: one of %s",
		name, strings.ToLower(strings.Join(viewNames, ", ")))
}

// String renders a view's name.
func (v ViewID) String() string {
	if int(v) < 0 || int(v) >= len(viewNames) {
		return "UNKNOWN"
	}
	return viewNames[v]
}

// Size is the terminal size in character cells.
type Size struct {
	Cols int
	Rows int
}

// EventKind names what happened.
type EventKind int

// The events the cockpit reacts to.
const (
	// EventKey is a keystroke.
	EventKey EventKind = iota
	// EventData is a fresh read of the store.
	EventData
	// EventResize is a new terminal size.
	EventResize
	// EventTick is the clock advancing, with no new data behind it.
	EventTick
	// EventArrivals is a live arrivals reading coming back.
	EventArrivals
	// EventNote is the outcome of something the operator asked for.
	EventNote
)

// Event is one thing that happened, delivered to Update.
type Event struct {
	Kind EventKind
	Key  Key
	At   time.Time
	Size Size

	// Data carries a store read for EventData.
	Data *Data
	// Arrivals carries a live reading for EventArrivals.
	Arrivals []Arrival
	// Note is a one-line result to show in the status bar.
	Note string
	// Err is a failure to report next to the note.
	Err error
}

// Action is what the model asks the runner to do next.
//
// The model never performs I/O itself; it says what it needs and the runner —
// which owns the store, the registry and the HTTP client — goes and does it.
// That is what keeps every transition testable.
type Action int

// The actions a view can ask for.
const (
	// ActionNone means carry on.
	ActionNone Action = iota
	// ActionQuit ends the session and restores the terminal.
	ActionQuit
	// ActionRefresh re-reads the store. It touches no network.
	ActionRefresh
	// ActionPollSource polls one source live. Target is its id.
	ActionPollSource
	// ActionArrivals fetches live arrivals. Target is a stop number.
	ActionArrivals
)

// Command is the model's answer to an event.
type Command struct {
	Action Action
	Target string
}

// Options configure a cockpit.
type Options struct {
	// Version is the build identity, shown so a frame can be attributed to
	// a binary.
	Version string
	// Registry is the path the source registry was loaded from.
	Registry string
	// Theme decides colour. Use render.PlainTheme in tests.
	Theme render.Theme
	// Interval is how often the runner re-reads the store.
	Interval time.Duration
	// StartedAt is when the session began, for the uptime counter.
	StartedAt time.Time
	// Now seeds the clock before the first tick.
	Now time.Time
	// Start is the screen the cockpit opens on.
	Start ViewID
	// Interactive says whether a keyboard is attached.
	Interactive bool
	// InputNote explains why it is not, when it is not.
	InputNote string
}

// changeGlow is how long the header indicator marks a change.
//
// It exists so the indicator means something. A block that blinks on a timer
// is decoration; one that blinks when the store actually moved is an
// instrument, and the difference is the whole point of the board.
const changeGlow = 30 * time.Second

// Model is the cockpit's state. Every transition goes through Update.
type Model struct {
	opts  Options
	theme render.Theme

	view ViewID
	size Size
	now  time.Time
	tick int
	quit bool
	help bool

	data      Data
	signature string
	changedAt time.Time
	status    string
	statusErr bool

	// Map state.
	box     render.GeoBox
	autoFit bool

	// Feed state.
	feedSel    int
	filter     string
	filtering  bool
	showDetail bool

	// Registry state.
	sourceSel int

	// Transit state.
	stopSel    int
	stopQuery  string
	stopTyping bool
	arrivals   []Arrival
}

// cordoba is the viewport the map opens on: the city and the ring of motorway
// around it.
//
// It does NOT re-fit to the data on its own, and that is the important part.
// Two sources in the registry are national — DGT publishes 1948 cameras from
// Galicia to Almería, IGN publishes earthquakes across Iberia and out into the
// Atlantic — so fitting to everything held produces a viewport spanning most of
// western Europe with Córdoba as three dots in it, under a panel headed
// CÓRDOBA. `f` fits to the data for anyone who wants that on purpose.
var cordoba = render.GeoBox{West: -5.05, South: 37.79, East: -4.65, North: 37.95}

// New builds a cockpit.
func New(opts Options) *Model {
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	if opts.StartedAt.IsZero() {
		opts.StartedAt = opts.Now
	}
	if opts.Interval <= 0 {
		opts.Interval = 5 * time.Second
	}

	return &Model{
		opts:    opts,
		theme:   opts.Theme,
		now:     opts.Now,
		view:    opts.Start,
		box:     cordoba,
		autoFit: false,
	}
}

// ViewID reports which screen is showing.
func (m *Model) ViewID() ViewID { return m.view }

// Size reports the last size the model was told about.
func (m *Model) Size() Size { return m.size }

// Quitting reports whether the session should end.
func (m *Model) Quitting() bool { return m.quit }

// Filter reports the feed's incremental filter.
func (m *Model) Filter() string { return m.filter }

// FeedSelection reports which row of the feed is selected.
func (m *Model) FeedSelection() int { return m.feedSel }

// MapBox reports the map's current viewport.
func (m *Model) MapBox() render.GeoBox { return m.box }

// MapAutoFit reports whether the map re-fits itself to the data each tick.
func (m *Model) MapAutoFit() bool { return m.autoFit }

// Changed reports whether the store moved recently enough for the header
// indicator to say so.
func (m *Model) Changed() bool {
	return !m.changedAt.IsZero() && m.now.Sub(m.changedAt) < changeGlow
}

// Update applies an event and returns what the runner should do next.
func (m *Model) Update(e Event) Command {
	if !e.At.IsZero() {
		m.now = e.At
	}

	switch e.Kind {
	case EventTick:
		m.tick++
		return Command{}
	case EventResize:
		m.size = e.Size
		return Command{}
	case EventData:
		m.applyData(e)
		return Command{}
	case EventArrivals:
		m.arrivals = e.Arrivals
		m.setNote(e)
		return Command{}
	case EventNote:
		m.setNote(e)
		return Command{}
	case EventKey:
		return m.key(e.Key)
	default:
		return Command{}
	}
}

// applyData takes a store read, notices whether anything moved, and re-fits the
// map when it is following the data.
func (m *Model) applyData(e Event) {
	if e.Data == nil {
		return
	}
	m.data = *e.Data

	if sig := signatureOf(m.data); sig != m.signature {
		m.signature = sig
		m.changedAt = m.now
	}
	if m.autoFit {
		m.box = fitToData(m.data, m.box)
	}
	m.clampSelections()
}

// setNote records a one-line result for the status bar.
func (m *Model) setNote(e Event) {
	if e.Err != nil {
		m.status, m.statusErr = e.Err.Error(), true
		return
	}
	if e.Note != "" {
		m.status, m.statusErr = e.Note, false
	}
}

// signatureOf fingerprints a read, so a redraw can tell an unchanged store from
// a moving one without diffing every record.
func signatureOf(d Data) string {
	newest := ""
	if len(d.Records) > 0 {
		newest = d.Records[0].ID
	}
	return fmt.Sprintf("%d/%d/%s/%d", d.RecordCount, d.EntityCount, newest, len(d.Records))
}

// key dispatches a keystroke: the active view's text fields first, then the
// view's own bindings, then the global ones.
//
// The order matters. A filter that is open owns the keyboard, or typing "quit"
// into a search box quits.
func (m *Model) key(k Key) Command {
	if m.filtering && m.view == ViewFeed {
		return m.feedFilterKey(k)
	}
	if m.stopTyping && m.view == ViewTransit {
		return m.stopSearchKey(k)
	}

	if cmd, handled := m.viewKey(k); handled {
		return cmd
	}
	return m.globalKey(k)
}

// globalKey handles the bindings that work on every screen.
func (m *Model) globalKey(k Key) Command {
	switch k.Code {
	case KeyCtrlC:
		m.quit = true
		return Command{Action: ActionQuit}
	case KeyTab:
		m.selectView((m.view + 1) % ViewID(len(viewNames)))
		return Command{}
	case KeyBackTab:
		m.selectView((m.view + ViewID(len(viewNames)) - 1) % ViewID(len(viewNames)))
		return Command{}
	case KeyEscape:
		m.help = false
		return Command{}
	case KeyRune:
		return m.globalRune(k.Rune)
	default:
		return Command{}
	}
}

// globalRune handles the printable global keys.
func (m *Model) globalRune(r rune) Command {
	switch {
	case r == 'q':
		m.quit = true
		return Command{Action: ActionQuit}
	case r == '?':
		m.help = !m.help
		return Command{}
	case r == 'r':
		return Command{Action: ActionRefresh}
	case r >= '1' && r <= '9':
		if n := int(r - '1'); n < len(viewNames) {
			m.selectView(ViewID(n))
		}
		return Command{}
	default:
		return Command{}
	}
}

// selectView switches screen and closes anything modal.
func (m *Model) selectView(v ViewID) {
	m.view = v
	m.help = false
	m.showDetail = false
}

// viewKey gives the active screen first refusal on a key.
func (m *Model) viewKey(k Key) (Command, bool) {
	switch m.view {
	case ViewMap:
		return m.mapKey(k)
	case ViewFeed:
		return m.feedKey(k)
	case ViewSources:
		return m.sourcesKey(k)
	case ViewTransit:
		return m.transitKey(k)
	default:
		return Command{}, false
	}
}

// clampSelections keeps every cursor inside its list after the data changes.
func (m *Model) clampSelections() {
	m.feedSel = clamp(m.feedSel, 0, len(m.visibleRecords())-1)
	m.sourceSel = clamp(m.sourceSel, 0, len(m.data.Sources)-1)
	m.stopSel = clamp(m.stopSel, 0, len(m.visibleStops())-1)
}

// View renders one frame at the given size.
//
// It takes a writer and a size rather than reading either from the process,
// which is what lets the whole cockpit be asserted on in a test.
func (m *Model) View(w io.Writer, size Size) {
	if size.Cols < minCols || size.Rows < minRows {
		_, _ = io.WriteString(w, render.Clip("eye: terminal too small", size.Cols))
		return
	}

	lines := m.header(size.Cols)
	body := size.Rows - len(lines) - footerRows
	lines = append(lines, m.body(size.Cols, body)...)
	lines = append(lines, m.footer(size.Cols)...)

	for i, line := range lines {
		if i >= size.Rows {
			break
		}
		if i > 0 {
			_, _ = io.WriteString(w, "\n")
		}
		_, _ = io.WriteString(w, render.Clip(line, size.Cols))
	}
}

// Layout constants. Below the minimum there is no board to draw, only a
// message saying so.
const (
	minCols    = 40
	minRows    = 10
	headerRows = 2
	footerRows = 2
)

// header draws the masthead: who is watching, when, and which screen.
func (m *Model) header(cols int) []string {
	t := m.theme

	left := fmt.Sprintf("%s %s %s  %s · up %s",
		t.Live("◉"), t.Bold(t.Live("EYE")), m.indicatorBlock(),
		t.Label(m.now.Format("02 Jan · 15:04:05")),
		t.Dim(shortDuration(m.now.Sub(m.opts.StartedAt))))

	right := m.tabs(cols)

	gap := cols - render.VisibleWidth(left) - render.VisibleWidth(right)
	if gap < 1 {
		// No room for the tab strip: the active screen's name is the part
		// that must survive.
		right = t.Bold(t.Live("‹" + m.view.String() + "›"))
		gap = cols - render.VisibleWidth(left) - render.VisibleWidth(right)
	}
	if gap < 1 {
		gap = 1
	}

	return []string{
		left + strings.Repeat(" ", gap) + right,
		t.Scanline(cols),
	}
}

// tabs renders the screen selector, with the active one picked out.
func (m *Model) tabs(cols int) string {
	if cols < 96 {
		return m.theme.Bold(m.theme.Live("‹" + m.view.String() + "›"))
	}

	var parts []string
	for i, name := range viewNames {
		label := fmt.Sprintf("%d %s", i+1, name)
		if ViewID(i) == m.view {
			parts = append(parts, m.theme.Bold(m.theme.Live("▸"+label)))
			continue
		}
		parts = append(parts, m.theme.Dim(" "+label))
	}
	return strings.Join(parts, m.theme.Chrome(" "))
}

// indicatorBlock is the cursor block beside the name.
//
// It blinks only while something has actually changed. A board that performs
// activity it does not have is worse than a static one.
func (m *Model) indicatorBlock() string {
	if !m.Changed() {
		return m.theme.Dim("▯")
	}
	if m.tick%2 == 0 {
		return m.theme.Live("▮")
	}
	return m.theme.Dim("▯")
}

// body renders the active screen, or the help pane over it.
func (m *Model) body(cols, rows int) []string {
	if rows < 1 {
		return nil
	}

	var lines []string
	switch {
	case m.help:
		lines = m.helpPane(cols)
	case m.view == ViewMap:
		lines = m.mapView(cols, rows)
	case m.view == ViewFeed:
		lines = m.feedView(cols, rows)
	case m.view == ViewSources:
		lines = m.sourcesView(cols, rows)
	case m.view == ViewTransit:
		lines = m.transitView(cols, rows)
	default:
		lines = m.dashboardView(cols, rows)
	}

	if len(lines) > rows {
		return lines[:rows]
	}
	for len(lines) < rows {
		lines = append(lines, "")
	}
	return lines
}

// footer draws the key hints and the standing caveat.
func (m *Model) footer(cols int) []string {
	t := m.theme

	left := t.Dim(m.keyHints())
	right := t.Dim("traces back to a public source")
	if m.status != "" {
		right = t.Live(render.Clip(m.status, cols/2))
		if m.statusErr {
			right = t.Alert(render.Clip(m.status, cols/2))
		}
	}
	if !m.opts.Interactive {
		left = t.Warn(render.Clip("no keyboard · "+m.opts.InputNote, cols-2))
	}

	gap := cols - render.VisibleWidth(left) - render.VisibleWidth(right)
	if gap < 1 {
		right = ""
		gap = cols - render.VisibleWidth(left)
	}
	if gap < 0 {
		gap = 0
	}

	return []string{t.Scanline(cols), left + strings.Repeat(" ", gap) + right}
}

// keyHints lists the keys that do something on the active screen.
func (m *Model) keyHints() string {
	common := "tab views · 1-5 jump · ? help · q quit"
	switch m.view {
	case ViewMap:
		return "↑↓←→ pan · +/- zoom · f fit · " + common
	case ViewFeed:
		return "↑↓ scroll · / filter · enter detail · " + common
	case ViewSources:
		return "↑↓ select · r poll it · " + common
	case ViewTransit:
		return "/ find a stop · enter arrivals · r refresh · " + common
	default:
		return "r re-read · " + common
	}
}

// helpPane lists every binding, for the times the footer is not enough.
func (m *Model) helpPane(cols int) []string {
	t := m.theme
	width := min(cols, 74)

	rows := [][2]string{
		{"tab / shift-tab", "next and previous screen"},
		{"1 2 3 4 5", "dashboard, map, feed, sources, transit"},
		{"r", "re-read the store; on SOURCES, poll the selected source live"},
		{"↑ ↓ pgup pgdn home end", "move within a list"},
		{"enter", "open the selected record, or ask for live arrivals"},
		{"esc", "close a pane, or clear a filter"},
		{"/", "filter the feed, or search the stop directory"},
		{"← → ↑ ↓", "pan the map"},
		{"+ -", "zoom the map in and out"},
		{"f", "fit the map to the data"},
		{"?", "open and close this list"},
		{"q / ctrl-c", "quit and give the terminal back exactly as it was"},
	}

	out := []string{t.BoxTop("keys", width)}
	for _, row := range rows {
		out = append(out, t.BoxRow(t.Label(render.PadVisible(row[0], 24))+t.Dim(row[1]), width))
	}
	out = append(out, t.BoxBottom(width))
	out = append(out, "", t.Dim("  Nothing here is fetched on a timer except the store read;"))
	out = append(out, t.Dim("  the network is touched only when you press r or ask for arrivals."))
	return out
}

// panel wraps content lines in a titled box.
func (m *Model) panel(title string, width int, inner []string) []string {
	out := make([]string, 0, len(inner)+2)
	out = append(out, m.theme.BoxTop(title, width))
	for _, line := range inner {
		out = append(out, m.theme.BoxRow(line, width))
	}
	return append(out, m.theme.BoxBottom(width))
}

// beside lays blocks out side by side, padding each to its own width.
func beside(gap int, blocks ...[]string) []string {
	widths := make([]int, len(blocks))
	height := 0
	for i, block := range blocks {
		for _, line := range block {
			widths[i] = max(widths[i], render.VisibleWidth(line))
		}
		height = max(height, len(block))
	}

	out := make([]string, 0, height)
	for row := range height {
		var sb strings.Builder
		for i, block := range blocks {
			if i > 0 {
				sb.WriteString(strings.Repeat(" ", gap))
			}
			line := ""
			if row < len(block) {
				line = block[row]
			}
			sb.WriteString(render.PadVisible(line, widths[i]))
		}
		out = append(out, strings.TrimRight(sb.String(), " "))
	}
	return out
}

// shortAge renders how old something is, compactly, without inventing a
// precision the source does not have.
func shortAge(t, now time.Time) string {
	if t.IsZero() {
		return "—"
	}

	d := now.Sub(t)
	future := d < 0
	if future {
		d = -d
	}

	s := shortDuration(d)
	if future {
		return "in " + s
	}
	return s
}

// shortDuration renders a duration the way somebody would say it.
func shortDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// clamp keeps a value inside a range, treating an empty range as zero.
func clamp(v, low, high int) int {
	if high < low {
		return low
	}
	return max(low, min(v, high))
}

// plural renders a count with the right noun.
func plural(n int, singular, many string) string {
	if n == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %s", n, many)
}

// orDash renders an absent value as absent rather than as blank.
func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}
