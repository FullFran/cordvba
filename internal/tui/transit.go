package tui

import (
	"fmt"
	"sort"

	observation "github.com/FullFran/eye/internal/observation/domain"
	"github.com/FullFran/eye/internal/render"
)

// Panel heights for the transit view. The stop list and the arrivals share the
// screen with the train board, and all three have to stay readable.
const (
	stopRows    = 4
	arrivalRows = 5
	trainRows   = 4
)

// visibleStops is the stop directory after the search term.
func (m *Model) visibleStops() []Stop {
	stops := stopsFrom(m.data.Entities)
	if m.stopQuery == "" {
		return stops
	}

	// Folded, word-order-free matching, so "trassierra pavlova" finds the
	// stop whatever order the pole prints them in.
	f := observation.Filter{Text: m.stopQuery}
	out := make([]Stop, 0, len(stops))
	for _, s := range stops {
		if f.MatchEntity(observation.Entity{Title: s.Name}) {
			out = append(out, s)
		}
	}
	return out
}

// transitKey handles the transit view's bindings.
func (m *Model) transitKey(k Key) (Command, bool) {
	stops := m.visibleStops()

	switch k.Code {
	case KeyUp:
		m.stopSel = clamp(m.stopSel-1, 0, len(stops)-1)
	case KeyDown:
		m.stopSel = clamp(m.stopSel+1, 0, len(stops)-1)
	case KeyHome:
		m.stopSel = 0
	case KeyEnd:
		m.stopSel = max(len(stops)-1, 0)
	case KeyEnter:
		return m.askForArrivals(stops)
	case KeyRune:
		return m.transitRune(k.Rune, stops)
	default:
		return Command{}, false
	}
	return Command{}, true
}

// transitRune handles the printable transit keys.
func (m *Model) transitRune(r rune, stops []Stop) (Command, bool) {
	switch r {
	case '/':
		m.stopTyping = true
		return Command{}, true
	case 'r':
		return m.askForArrivals(stops)
	default:
		return Command{}, false
	}
}

// askForArrivals requests a live reading for the selected stop.
//
// This is the only view that reaches the network without an explicit r, and it
// still does so only when asked: no tick ever polls the operator.
func (m *Model) askForArrivals(stops []Stop) (Command, bool) {
	if len(stops) == 0 {
		m.status, m.statusErr = "no stop selected", true
		return Command{}, true
	}

	stop := stops[clamp(m.stopSel, 0, len(stops)-1)]
	m.status, m.statusErr = "asking about "+stop.Name+"…", false
	return Command{Action: ActionArrivals, Target: stop.ID}, true
}

// stopSearchKey is the keyboard while the stop search is open.
func (m *Model) stopSearchKey(k Key) Command {
	switch k.Code {
	case KeyEscape:
		m.stopQuery, m.stopTyping = "", false
	case KeyEnter:
		m.stopTyping = false
	case KeyBackspace:
		if runes := []rune(m.stopQuery); len(runes) > 0 {
			m.stopQuery = string(runes[:len(runes)-1])
		}
	case KeyCtrlC:
		m.quit = true
		return Command{Action: ActionQuit}
	case KeyRune:
		m.stopQuery += string(k.Rune)
	}
	m.stopSel = clamp(m.stopSel, 0, len(m.visibleStops())-1)
	return Command{}
}

// transitView draws the stop directory, the live arrivals and the train board.
func (m *Model) transitView(cols, rows int) []string {
	width := cols
	out := m.stopsPanel(width)
	out = append(out, m.arrivalsPanel(width)...)

	if rows-len(out) >= trainRows+2 {
		out = append(out, m.trainsPanel(width, rows-len(out))...)
	}
	return out
}

// stopsPanel lists the bus stops eye holds, narrowed by the search term.
//
// The directory comes from the store, not from the operator's server: typing
// in a search box should not send a request per keystroke to somebody else's
// service.
func (m *Model) stopsPanel(width int) []string {
	t := m.theme
	stops := m.visibleStops()
	inner := width - 4

	body := []string{m.stopSearchLine(inner)}
	if len(stops) == 0 {
		body = append(body, t.Warn("no bus stop in the store matches · run eye daemon to build the directory"))
	}

	top := scrollTop(m.stopSel, stopRows, len(stops))
	for i := top; i < len(stops) && i < top+stopRows; i++ {
		marker := " "
		name := t.Live(stops[i].Name)
		if i == m.stopSel {
			marker = t.Live("▸")
			name = t.Bold(t.Live(stops[i].Name))
		}
		body = append(body, render.Clip(
			marker+t.Label(render.PadVisible("parada "+stops[i].ID, 14))+
				render.Ellipsis(name, max(inner-15, 8)), inner))
	}

	return m.panel(fmt.Sprintf("stops · %d of %d", len(stops), len(stopsFrom(m.data.Entities))), width, body)
}

// stopSearchLine draws the search field.
func (m *Model) stopSearchLine(width int) string {
	t := m.theme
	if m.stopTyping {
		return render.Clip(t.Label("/")+t.Live(m.stopQuery)+t.Live("▮"), width)
	}
	if m.stopQuery != "" {
		return render.Clip(t.Label("/")+t.Live(m.stopQuery)+t.Dim("  esc clears it"), width)
	}
	return render.Clip(t.Dim("/ to search the stop directory · enter asks the operator for arrivals"), width)
}

// arrivalsPanel shows the live estimates, counting down, and says plainly when
// they have outlived their own cadence.
func (m *Model) arrivalsPanel(width int) []string {
	t := m.theme
	inner := width - 4

	if len(m.arrivals) == 0 {
		return m.panel("arrivals", width, []string{
			t.Dim("press enter on a stop to ask the operator for its next buses"),
			t.Dim("nothing is fetched on a tick · this is the operator estimate, not a measurement"),
		})
	}

	arrivals := append([]Arrival(nil), m.arrivals...)
	sort.SliceStable(arrivals, func(i, j int) bool {
		return arrivals[i].ETA(m.now) < arrivals[j].ETA(m.now)
	})

	body := []string{t.Dim(render.PadVisible("ETA", 10) + render.PadVisible("LINE", 7) +
		render.PadVisible("ROUTE", 30) + "OCCUPANCY")}

	stale := false
	for i, a := range arrivals {
		if i >= arrivalRows-2 {
			break
		}
		stale = stale || a.Stale(m.now)
		body = append(body, render.Clip(
			render.PadVisible(m.etaCell(a), 10)+
				t.Label(render.PadVisible(a.Line, 7))+
				t.Live(render.PadVisible(render.Clip(a.Route, 29), 30))+
				t.Dim(orDash(a.Occupancy)), inner))
	}

	note := t.Dim(fmt.Sprintf("operator estimate · read %s ago · not a measurement",
		shortAge(arrivals[0].ReadAt, m.now)))
	if stale {
		note = t.Warn(fmt.Sprintf("STALE · read %s ago, past the %s the operator refreshes in · press r",
			shortAge(arrivals[0].ReadAt, m.now), shortDuration(arrivalCadence)))
	}
	body = append(body, render.Clip(note, inner))

	return m.panel("arrivals · "+arrivals[0].StopName, width, body)
}

// etaCell counts an estimate down, and refuses to keep counting once the
// reading is older than the cadence it was published at.
func (m *Model) etaCell(a Arrival) string {
	t := m.theme
	if a.Stale(m.now) {
		return t.Warn(fmt.Sprintf("%dm STALE", a.Minutes))
	}

	left := a.ETA(m.now)
	if left <= 0 {
		return t.Live("due")
	}
	return t.Live(shortDuration(left))
}

// trainsPanel shows the next scheduled departures from the watched stations.
func (m *Model) trainsPanel(width, rows int) []string {
	t := m.theme
	inner := width - 4

	departures := departuresFrom(m.data.Records, m.now)
	if len(departures) == 0 {
		return m.panel("trains", width, []string{
			t.Dim("no scheduled departure in the store for the watched stations"),
		})
	}

	body := make([]string, 0, rows)
	for i, r := range departures {
		if i >= min(rows-2, trainRows) {
			break
		}
		body = append(body, render.Clip(
			t.Label(render.PadVisible(r.ValidFrom.Local().Format("15:04"), 8))+
				t.Dim(render.PadVisible("in "+shortDuration(r.ValidFrom.Sub(m.now)), 8))+
				t.Live(r.Title), inner))
	}
	body = append(body, t.Dim(fmt.Sprintf("%d scheduled in the next day · timetable, not a live position",
		len(departures))))

	return m.panel("trains", width, body)
}
