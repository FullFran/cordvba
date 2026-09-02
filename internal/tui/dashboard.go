package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/FullFran/eye/internal/render"
)

// summaryPanelRows is the height of the topic and rate panels together, border
// included.
const summaryPanelRows = 6

// dashboardView is the overview: what this binary is, what the store holds,
// how each source is doing, and what has been arriving.
func (m *Model) dashboardView(cols, rows int) []string {
	lines := []string{m.summaryLine(cols)}
	if m.data.Err != nil {
		lines = append(lines, m.theme.Alert(render.Clip(
			"could not read the store: "+m.data.Err.Error(), cols)))
	}
	lines = append(lines, "")

	// The health grid takes what it needs; the summary panels take what is
	// left, which is what keeps the board usable in a short terminal.
	room := rows - len(lines) - summaryPanelRows - 1
	lines = append(lines, m.healthGrid(cols, room)...)

	if rows-len(lines) >= summaryPanelRows {
		lines = append(lines, "")
		lines = append(lines, m.summaryPanels(cols)...)
	}
	return lines
}

// summaryLine is the one-line identity of what is on screen: which build,
// which registry, how much is in the store and how fresh the read is.
func (m *Model) summaryLine(cols int) string {
	t := m.theme

	read := "never"
	if !m.data.ReadAt.IsZero() {
		read = shortAge(m.data.ReadAt, m.now) + " ago"
	}

	// The counts come before the registry path so that a long path is what
	// gets clipped on a narrow terminal, rather than the figures.
	line := strings.Join([]string{
		t.Label("build ") + t.Live(m.opts.Version),
		t.Live(fmt.Sprintf("%d records", m.data.RecordCount)),
		t.Live(fmt.Sprintf("%d entities", m.data.EntityCount)),
		t.Dim("read " + read),
		t.Label("registry ") + t.Live(tailOf(orDash(m.data.Registry), max(cols/3, 16))),
	}, t.Chrome(" · "))

	return render.Clip(" "+line, cols)
}

// tailOf shortens a path from the left, because the end of a path is the half
// that identifies it.
func tailOf(s string, width int) string {
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	return "…" + string(runes[len(runes)-width+1:])
}

// healthCardWidth is the narrowest a card can be and still say something
// useful. Below it the grid drops to one column.
const healthCardWidth = 34

// healthGrid draws one card per source, laid out in as many columns as fit.
//
// A card rather than a table row because the interesting fields are of
// different kinds — a state, an age, a count and an error — and a table makes
// the error either unreadable or the whole grid too wide to scan.
func (m *Model) healthGrid(cols, rows int) []string {
	t := m.theme
	header := t.Section("source health", min(cols, 100))

	if len(m.data.Sources) == 0 {
		return []string{header, t.Dim("  the registry is empty")}
	}

	perRow := max(1, (cols+1)/(healthCardWidth+1))
	width := (cols - (perRow - 1)) / perRow

	sources := append([]SourceRow(nil), m.data.Sources...)
	sort.SliceStable(sources, func(i, j int) bool { return healthRank(sources[i]) < healthRank(sources[j]) })

	out := []string{header}
	for i := 0; i < len(sources); i += perRow {
		// cardRows is the fixed height of one card; stopping on a partial
		// row would clip a border and leave a dangling wall.
		if rows-len(out) < cardRows {
			out = append(out, t.Dim(fmt.Sprintf("  … %d more sources · press 4", len(sources)-i)))
			return out
		}

		row := make([][]string, 0, perRow)
		for _, s := range sources[i:min(i+perRow, len(sources))] {
			row = append(row, m.healthCard(s, width))
		}
		out = append(out, beside(1, row...)...)
	}
	return out
}

// cardRows is how tall one health card is, border included.
const cardRows = 6

// healthCard renders one source's state.
//
// The panel title is the topic, and the source id is the first line inside it.
// A box title is a label and gets upper-cased; an id is a value and must be
// shown exactly as the registry spells it.
func (m *Model) healthCard(s SourceRow, width int) []string {
	t := m.theme
	inner := width - 4

	state := s.State()
	styled := t.Live(state)
	switch s.indicator() {
	case "failing":
		styled = t.Alert(state)
	case "stale", "none":
		styled = t.Warn(state)
	}

	last := "never"
	if !s.LastSuccess.IsZero() {
		last = "ok " + shortAge(s.LastSuccess, m.now)
	}

	id := t.Bold(t.Live(render.Ellipsis(s.ID, inner)))
	state = render.PadVisible(t.Indicator(s.indicator())+" "+styled,
		max(inner-render.VisibleWidth(last), 1)) + t.Dim(last)
	records := t.Dim(fmt.Sprintf("%s · %s", plural(s.Records, "record", "records"), orDash(s.Automation)))

	failure := t.Dim("—")
	if s.LastError != "" {
		failure = t.Alert(render.Ellipsis(s.LastError, inner))
	}

	return m.panel(orDash(s.Topic), width, []string{id, state, records, failure})
}

// healthRank orders the grid so the problems are at the top. A board where the
// broken source is the last thing you scroll to is a board you stop trusting.
func healthRank(s SourceRow) int {
	switch s.State() {
	case "failing":
		return 0
	case "stale":
		return 1
	case "never polled":
		return 2
	case "live":
		return 3
	default:
		return 4
	}
}

// summaryPanels draws the topic breakdown beside the arrival rate, or stacks
// them when the terminal is narrow.
func (m *Model) summaryPanels(cols int) []string {
	if cols < 90 {
		width := min(cols, 80)
		out := m.panel("by topic", width, m.topicLines(width-4))
		return append(out, m.panel("records per hour", width, m.rateLines(width-4))...)
	}

	left := (cols - 1) / 2
	right := cols - 1 - left

	// Both panels are grown to the same content height before they are
	// boxed. Side by side, a short box next to a tall one closes halfway up
	// the screen and reads as a broken frame rather than as an empty panel.
	topics, rate := levelled(m.topicLines(left-4), m.rateLines(right-4))

	return beside(1,
		m.panel("by topic", left, topics),
		m.panel("records per hour", right, rate))
}

// levelled pads two blocks of content to the same height.
func levelled(a, b []string) ([]string, []string) {
	for len(a) < len(b) {
		a = append(a, "")
	}
	for len(b) < len(a) {
		b = append(b, "")
	}
	return a, b
}

// topicRows is how many topic lines the panel shows before summarising.
const topicRows = 3

// topicLines counts what the store holds by topic, with how fresh each is.
func (m *Model) topicLines(width int) []string {
	t := m.theme

	counts := map[string]int{}
	newest := map[string]time.Time{}
	for _, r := range m.data.Records {
		counts[r.Topic]++
		if r.ObservedAt.After(newest[r.Topic]) {
			newest[r.Topic] = r.ObservedAt
		}
	}
	if len(counts) == 0 {
		return []string{t.Dim("nothing in the store yet")}
	}

	topics := make([]string, 0, len(counts))
	for topic := range counts {
		topics = append(topics, topic)
	}
	// Busiest first, then alphabetically. The tiebreak is not cosmetic: with
	// a plain count comparison two topics of equal size swap places between
	// frames, and a board whose rows move on their own is unreadable.
	sort.Slice(topics, func(i, j int) bool {
		if counts[topics[i]] != counts[topics[j]] {
			return counts[topics[i]] > counts[topics[j]]
		}
		return topics[i] < topics[j]
	})

	out := make([]string, 0, topicRows)
	for i := 0; i < len(topics) && i < topicRows; i++ {
		topic := topics[i]
		age := shortAge(newest[topic], m.now)

		// Over a day old is amber whatever else it says. Freshness is the
		// one thing a situation board must not flatter.
		fresh := t.Live(age)
		if m.now.Sub(newest[topic]) > 24*time.Hour {
			fresh = t.Warn(age)
		}

		out = append(out, render.Clip(
			t.Label(render.PadVisible(topic, 14))+
				t.Dim(render.PadVisible(fmt.Sprint(counts[topic]), 7))+fresh, width))
	}
	if extra := len(topics) - topicRows; extra > 0 {
		noun := "topics"
		if extra == 1 {
			noun = "topic"
		}
		out = append(out, t.Dim(fmt.Sprintf("… and %d more %s", extra, noun)))
	}
	return out
}

// rateHours is how far back the histogram looks.
const rateHours = 24

// rateLines draws arrivals per hour over the last day.
//
// It is drawn from ObservedAt rather than FetchedAt: the question is when the
// city did something, not when eye got round to looking.
func (m *Model) rateLines(width int) []string {
	t := m.theme

	buckets := make([]int, rateHours)
	total := 0
	for _, r := range m.data.Records {
		hours := int(m.now.Sub(r.ObservedAt).Hours())
		if hours < 0 || hours >= rateHours {
			continue
		}
		buckets[rateHours-1-hours]++
		total++
	}

	peak := 0
	for _, v := range buckets {
		peak = max(peak, v)
	}

	bars := render.Bars(buckets)
	axis := render.PadVisible("-24h", max(len(buckets)-3, 0)) + "now"

	return []string{
		render.Clip(t.Live(bars), width),
		render.Clip(t.Dim(axis), width),
		render.Clip(t.Dim(fmt.Sprintf("%d in the last day · peak %d an hour", total, peak)), width),
	}
}
