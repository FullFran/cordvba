package tui

import (
	"fmt"
	"strings"

	observation "github.com/FullFran/eye/internal/observation/domain"
	"github.com/FullFran/eye/internal/render"
)

// pageStep is how far page up and page down move a list.
const pageStep = 10

// visibleRecords is the feed after the filter, newest first.
func (m *Model) visibleRecords() []observation.Record {
	records := m.data.Records
	if m.filter == "" {
		return records
	}

	// The same accent-folding, word-order-free matching the query flags use,
	// so searching the cockpit and searching the command line agree.
	f := observation.Filter{Text: m.filter}
	out := make([]observation.Record, 0, len(records))
	for _, r := range records {
		if f.MatchRecord(r) {
			out = append(out, r)
		}
	}
	return out
}

// feedKey handles the feed's own bindings.
func (m *Model) feedKey(k Key) (Command, bool) {
	count := len(m.visibleRecords())

	switch k.Code {
	case KeyUp:
		m.feedSel = clamp(m.feedSel-1, 0, count-1)
	case KeyDown:
		m.feedSel = clamp(m.feedSel+1, 0, count-1)
	case KeyPageUp:
		m.feedSel = clamp(m.feedSel-pageStep, 0, count-1)
	case KeyPageDown:
		m.feedSel = clamp(m.feedSel+pageStep, 0, count-1)
	case KeyHome:
		m.feedSel = 0
	case KeyEnd:
		m.feedSel = max(count-1, 0)
	case KeyEnter:
		m.showDetail = count > 0
	case KeyEscape:
		if m.showDetail {
			m.showDetail = false
			return Command{}, true
		}
		return Command{}, false
	case KeyRune:
		if k.Rune != '/' {
			return Command{}, false
		}
		m.filtering = true
	default:
		return Command{}, false
	}
	return Command{}, true
}

// feedFilterKey is the keyboard while the filter is open.
//
// It takes everything printable, q included. A cockpit that quits because
// somebody searched for "quarry" is a cockpit nobody searches in.
func (m *Model) feedFilterKey(k Key) Command {
	switch k.Code {
	case KeyEscape:
		m.filter, m.filtering = "", false
	case KeyEnter:
		m.filtering = false
	case KeyBackspace:
		if runes := []rune(m.filter); len(runes) > 0 {
			m.filter = string(runes[:len(runes)-1])
		}
	case KeyCtrlC:
		m.quit = true
		return Command{Action: ActionQuit}
	case KeyRune:
		m.filter += string(k.Rune)
	}
	m.feedSel = clamp(m.feedSel, 0, len(m.visibleRecords())-1)
	return Command{}
}

// feedView draws the scrolling table, or the detail pane over it.
func (m *Model) feedView(cols, rows int) []string {
	records := m.visibleRecords()
	if m.showDetail && m.feedSel < len(records) {
		return m.detailPane(cols, rows, records[m.feedSel])
	}

	t := m.theme
	width := cols
	inner := width - 4

	// The rows the table can show, after its two borders, its column strip
	// and the filter line under it.
	visible := max(rows-4, 1)
	top := scrollTop(m.feedSel, visible, len(records))

	header := t.Dim(render.PadVisible("TIME", 7) + render.PadVisible("SOURCE", 20) +
		render.PadVisible("SEV", 5) + "TITLE")
	body := []string{header}

	if len(records) == 0 {
		body = append(body, t.Warn("nothing in the store matches"))
	}
	for i := top; i < len(records) && i < top+visible; i++ {
		body = append(body, render.Clip(m.feedRow(records[i], i == m.feedSel, inner), inner))
	}

	out := m.panel(m.feedTitle(len(records)), width, body)
	return append(out, m.filterLine(cols))
}

// feedTitle names the panel and says how much of the store it is showing.
func (m *Model) feedTitle(shown int) string {
	if m.filter != "" {
		return fmt.Sprintf("feed · %d of %d match", shown, len(m.data.Records))
	}
	return fmt.Sprintf("feed · %d newest", shown)
}

// feedRow renders one observation.
func (m *Model) feedRow(r observation.Record, selected bool, width int) string {
	t := m.theme

	marker := " "
	title := t.Live(r.Title)
	if selected {
		marker = t.Live("▸")
		title = t.Bold(t.Live(r.Title))
	}

	return marker + t.Dim(render.PadVisible(shortAge(r.ObservedAt, m.now), 6)) +
		t.Label(render.PadVisible(render.Ellipsis(r.Source, 19), 20)) +
		render.PadVisible(m.severityMark(r.Severity), 5) +
		render.Ellipsis(title, max(width-32, 8))
}

// severityMark draws the coarse impact scale as a bar, coloured by how much it
// matters. The scale is small on purpose: a river level and a road closure can
// share a list without pretending to be the same kind of thing.
func (m *Model) severityMark(s observation.Severity) string {
	if s <= observation.SeverityNone {
		return m.theme.Dim("·")
	}

	bar := strings.Repeat("▪", int(s))
	switch {
	case s >= observation.SeverityHigh:
		return m.theme.Alert(bar)
	case s >= observation.SeverityModerate:
		return m.theme.Warn(bar)
	default:
		return m.theme.Live(bar)
	}
}

// filterLine draws the incremental filter, with a cursor while it is open.
func (m *Model) filterLine(cols int) string {
	t := m.theme
	if m.filtering {
		return render.Clip(" "+t.Label("/")+t.Live(m.filter)+t.Live("▮"), cols)
	}
	if m.filter != "" {
		return render.Clip(" "+t.Label("/")+t.Live(m.filter)+t.Dim("  esc clears it"), cols)
	}
	return render.Clip(" "+t.Dim("/ to filter · enter for the full record and its provenance"), cols)
}

// detailPane shows one record whole, provenance included.
//
// Provenance is the part that makes an observation checkable rather than a
// rumour, so it is not folded away behind another keypress: publisher, licence,
// both timestamps and the hash of the bytes it was normalised from.
func (m *Model) detailPane(cols, rows int, r observation.Record) []string {
	t := m.theme
	width := min(cols, 100)
	inner := width - 4

	field := func(name, value string) string {
		return render.Clip(t.Label(render.PadVisible(name, 12))+t.Live(value), inner)
	}

	position := "—"
	if r.Position != nil {
		position = fmt.Sprintf("%.5f, %.5f", r.Position.Lat, r.Position.Lon)
	}

	body := []string{
		render.Clip(t.Bold(t.Live(r.Title)), inner),
		"",
		field("source", r.Source),
		field("topic", r.Topic+" · "+orDash(r.Kind)),
		field("severity", fmt.Sprintf("%d of 5", int(r.Severity))),
		field("quality", string(r.Quality)),
		field("position", position),
		"",
		render.Clip(t.Chrome("provenance"), inner),
		field("publisher", orDash(r.Provenance.Publisher)),
		field("licence", orDash(r.Provenance.License)),
		field("observed", r.ObservedAt.Format("2006-01-02 15:04:05 MST")),
		field("fetched", r.FetchedAt.Format("2006-01-02 15:04:05 MST")+
			fmt.Sprintf("  (latency %s)", shortDuration(r.Latency()))),
		field("raw hash", orDash(r.Provenance.RawHash)),
		field("url", orDash(r.Provenance.SourceURL)),
	}
	if r.Description != "" {
		body = append(body, "", render.Clip(t.Dim(r.Description), inner))
	}

	out := m.panel("record", width, body)
	if len(out) > rows-1 {
		out = out[:max(rows-1, 1)]
	}
	return append(out, t.Dim(" esc closes this · observed and fetched are never merged"))
}

// scrollTop keeps the selected row on screen without jumping the whole list
// around it.
func scrollTop(selected, visible, total int) int {
	if total <= visible {
		return 0
	}
	top := selected - visible/2
	return clamp(top, 0, total-visible)
}
