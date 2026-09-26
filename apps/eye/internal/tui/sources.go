package tui

import (
	"fmt"

	"github.com/FullFran/eye/internal/render"
)

// Column widths for the registry table. They are fixed rather than computed by
// a tabwriter because one long licence string should not be able to push the
// automation column off the screen.
const (
	colID         = 21
	colAuthority  = 17
	colTopic      = 10
	colFormat     = 14
	colLicence    = 14
	colAccess     = 21
	colAutomation = 13
	colLastOK     = 10
)

// columnSet says which of the optional columns fit in the terminal.
//
// The table degrades by dropping columns rather than by clipping them: half of
// "documented_backe" is worse than nothing there, because it reads as a value.
// What never goes is the id, the topic, whether eye may poll it, and when it
// last worked.
type columnSet struct {
	authority bool
	format    bool
	licence   bool
	access    bool
}

// fixedColumns is the width the never-dropped columns take.
const fixedColumns = 1 + colID + colTopic + colAutomation + colLastOK

// columnsFor decides what fits, widest first.
func columnsFor(width int) columnSet {
	set := columnSet{}
	for _, step := range []struct {
		cost int
		on   *bool
	}{
		{cost: colAuthority, on: &set.authority},
		{cost: colFormat, on: &set.format},
		{cost: colLicence, on: &set.licence},
		{cost: colAccess, on: &set.access},
	} {
		if fixedColumns+columnWidth(set)+step.cost > width {
			break
		}
		*step.on = true
	}
	return set
}

// columnWidth is what the optional columns currently take.
func columnWidth(set columnSet) int {
	width := 0
	for _, c := range []struct {
		on    bool
		width int
	}{
		{set.authority, colAuthority},
		{set.format, colFormat},
		{set.licence, colLicence},
		{set.access, colAccess},
	} {
		if c.on {
			width += c.width
		}
	}
	return width
}

// sourcesKey handles the registry view's bindings.
func (m *Model) sourcesKey(k Key) (Command, bool) {
	count := len(m.data.Sources)

	switch k.Code {
	case KeyUp:
		m.sourceSel = clamp(m.sourceSel-1, 0, count-1)
	case KeyDown:
		m.sourceSel = clamp(m.sourceSel+1, 0, count-1)
	case KeyPageUp:
		m.sourceSel = clamp(m.sourceSel-pageStep, 0, count-1)
	case KeyPageDown:
		m.sourceSel = clamp(m.sourceSel+pageStep, 0, count-1)
	case KeyHome:
		m.sourceSel = 0
	case KeyEnd:
		m.sourceSel = max(count-1, 0)
	case KeyRune:
		if k.Rune != 'r' || count == 0 {
			return Command{}, false
		}
		// The one place in the cockpit where a keypress reaches the
		// network, and it is one source, once, because somebody asked.
		selected := m.data.Sources[m.sourceSel]
		m.status, m.statusErr = "polling "+selected.ID+"…", false
		return Command{Action: ActionPollSource, Target: selected.ID}, true
	default:
		return Command{}, false
	}
	return Command{}, true
}

// sourcesView draws the whole registry: what eye may read, what it can read,
// and what happened last time it tried.
func (m *Model) sourcesView(cols, rows int) []string {
	t := m.theme
	width := cols
	inner := width - 4

	visible := max(rows-6, 1)
	top := scrollTop(m.sourceSel, visible, len(m.data.Sources))

	set := columnsFor(inner)

	body := []string{t.Dim(render.Clip(sourcesHeader(set), inner))}
	if len(m.data.Sources) == 0 {
		body = append(body, t.Alert("the registry is empty"))
	}
	for i := top; i < len(m.data.Sources) && i < top+visible; i++ {
		body = append(body, render.Clip(m.sourceRow(m.data.Sources[i], set, i == m.sourceSel), inner))
	}

	out := m.panel(fmt.Sprintf("sources · %d in the registry", len(m.data.Sources)), width, body)
	return append(out, m.selectedSourceLines(cols)...)
}

// sourcesHeader is the column strip for the columns that fit.
func sourcesHeader(set columnSet) string {
	out := " " + render.PadVisible("SOURCE", colID)
	if set.authority {
		out += render.PadVisible("AUTHORITY", colAuthority)
	}
	out += render.PadVisible("TOPIC", colTopic)
	if set.format {
		out += render.PadVisible("FORMAT", colFormat)
	}
	if set.licence {
		out += render.PadVisible("LICENCE", colLicence)
	}
	if set.access {
		out += render.PadVisible("ACCESS", colAccess)
	}
	return out + render.PadVisible("AUTOMATION", colAutomation) + "LAST OK"
}

// sourceRow renders one registry entry.
func (m *Model) sourceRow(s SourceRow, set columnSet, selected bool) string {
	t := m.theme

	marker := " "
	if selected {
		marker = t.Live("▸")
	}

	last := t.Dim("never")
	switch {
	case !s.LastSuccess.IsZero() && s.Stale:
		last = t.Warn(shortAge(s.LastSuccess, m.now) + " stale")
	case !s.LastSuccess.IsZero():
		last = t.Live(shortAge(s.LastSuccess, m.now))
	case s.LastError != "":
		last = t.Alert("failing")
	}

	automation := t.Live(s.Automation)
	if s.Automation != "enabled" {
		automation = t.Warn(s.Automation)
	}

	row := marker + t.Label(render.PadVisible(render.Ellipsis(s.ID, colID-1), colID))
	if set.authority {
		row += render.PadVisible(render.Ellipsis(s.Authority, colAuthority-1), colAuthority)
	}
	row += t.Dim(render.PadVisible(render.Ellipsis(s.Topic, colTopic-1), colTopic))
	if set.format {
		row += render.PadVisible(render.Ellipsis(s.Format, colFormat-1), colFormat)
	}
	if set.licence {
		row += t.Dim(render.PadVisible(render.Ellipsis(s.License, colLicence-1), colLicence))
	}
	if set.access {
		row += render.PadVisible(render.Ellipsis(s.Access, colAccess-1), colAccess)
	}
	return row + render.PadVisible(automation, colAutomation) + last
}

// selectedSourceLines explain the highlighted entry: why it is or is not
// pollable, and what went wrong last time.
func (m *Model) selectedSourceLines(cols int) []string {
	t := m.theme
	if len(m.data.Sources) == 0 {
		return []string{"", ""}
	}

	s := m.data.Sources[clamp(m.sourceSel, 0, len(m.data.Sources)-1)]

	interval := "—"
	if s.Interval > 0 {
		interval = shortDuration(s.Interval)
	}

	adapter := t.Live("adapter present")
	if !s.HasAdapter {
		adapter = t.Warn("no adapter in this build")
	}
	pollable := t.Live("eye will poll it")
	if !s.Pollable {
		pollable = t.Warn("held out of the scheduler")
	}

	first := fmt.Sprintf(" %s %s · %s · %s · %s · %s",
		t.Label("selected"), t.Bold(t.Live(s.ID)),
		adapter, pollable,
		t.Dim("every "+interval),
		t.Dim(fmt.Sprintf("%d records last time", s.Records)))

	second := t.Dim(" r polls this source now · held sources are a licence question, not a failure")
	if s.LastError != "" {
		second = " " + t.Alert("last error: "+s.LastError)
	}

	return []string{render.Clip(first, cols), render.Clip(second, cols)}
}
