package tui

import (
	"fmt"
	"sort"
	"strings"

	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	"github.com/FullFran/cordvba/apps/eye/internal/render"
)

// Map interaction constants.
const (
	// panStep is how far one arrow press moves the viewport, as a fraction
	// of what is on screen.
	panStep = 0.2
	// zoomStep is the factor one +/- press applies.
	zoomStep = 0.7
	// fitPadding is the margin auto-fit leaves around the data, so a feature
	// on the edge is not drawn half off it.
	fitPadding = 0.1
	// sidePanelWidth is the width of the legend beside the map.
	sidePanelWidth = 30
)

// feature is one plottable thing, already reduced to what the map needs.
type feature struct {
	title    string
	topic    string
	position observation.Point
	// inventory marks a persistent thing rather than an observation of one.
	inventory bool
	severity  observation.Severity
}

// topicGlyph is the character each topic is drawn with. The map is small, so
// the alphabet is small: shape carries kind, colour carries state.
var topicGlyph = map[string]rune{
	"traffic":   '●',
	"air":       '▲',
	"water":     '≈',
	"transport": '■',
	"events":    '◆',
	"weather":   '✳',
	"emergency": '✖',
}

// topicColor is the palette the map draws each topic in.
var topicColor = map[string]render.Color{
	"traffic":   render.ColorAmber,
	"air":       render.ColorCyan,
	"water":     render.ColorPhosphor,
	"transport": render.ColorPhosphor,
	"events":    render.ColorCyan,
	"weather":   render.ColorCyan,
	"emergency": render.ColorCrimson,
}

// glyphFor returns the character and colour a feature is drawn with.
func glyphFor(f feature) (rune, render.Color) {
	glyph, ok := topicGlyph[f.topic]
	if !ok {
		glyph = '·'
	}
	colour, ok := topicColor[f.topic]
	if !ok {
		colour = render.ColorSlate
	}

	if f.inventory {
		// Inventory is furniture: it says what is installed, not what is
		// happening, and it must not compete with live observations.
		return '□', render.ColorSlate
	}
	if f.severity >= observation.SeverityHigh {
		return glyph, render.ColorCrimson
	}
	return glyph, colour
}

// mapKey handles the map's own bindings.
func (m *Model) mapKey(k Key) (Command, bool) {
	switch k.Code {
	case KeyLeft:
		m.pan(-panStep, 0)
	case KeyRight:
		m.pan(panStep, 0)
	case KeyUp:
		m.pan(0, panStep)
	case KeyDown:
		m.pan(0, -panStep)
	case KeyRune:
		return m.mapRune(k.Rune)
	default:
		return Command{}, false
	}
	return Command{}, true
}

// mapRune handles the printable map keys.
func (m *Model) mapRune(r rune) (Command, bool) {
	switch r {
	case '+', '=':
		m.autoFit = false
		m.box = m.box.Zoom(zoomStep)
	case '-', '_':
		m.autoFit = false
		m.box = m.box.Zoom(1 / zoomStep)
	case 'f':
		m.autoFit = true
		m.box = fitToData(m.data, cordoba)
	default:
		return Command{}, false
	}
	return Command{}, true
}

// pan moves the viewport and stops the map following the data.
//
// Without that second half the next tick would re-fit and drag the view back,
// so the map would fight the person using it.
func (m *Model) pan(dx, dy float64) {
	m.autoFit = false
	m.box = m.box.Shift(dx, dy)
}

// fitToData returns a box around everything that has a position, or the
// fallback when nothing does.
func fitToData(d Data, fallback render.GeoBox) render.GeoBox {
	features := featuresOf(d)
	if len(features) == 0 {
		return fallback
	}

	box := render.GeoBox{
		West: features[0].position.Lon, East: features[0].position.Lon,
		South: features[0].position.Lat, North: features[0].position.Lat,
	}
	for _, f := range features[1:] {
		box.West = min(box.West, f.position.Lon)
		box.East = max(box.East, f.position.Lon)
		box.South = min(box.South, f.position.Lat)
		box.North = max(box.North, f.position.Lat)
	}
	return box.Pad(fitPadding)
}

// featuresOf reduces a read to everything that can be drawn.
func featuresOf(d Data) []feature {
	out := make([]feature, 0, len(d.Records)+len(d.Entities))
	for _, r := range d.Records {
		if r.Position == nil {
			continue
		}
		out = append(out, feature{
			title: r.Title, topic: r.Topic, position: *r.Position, severity: r.Severity,
		})
	}
	for _, e := range d.Entities {
		if e.Position == nil {
			continue
		}
		out = append(out, feature{
			title: e.Title, topic: e.Topic, position: *e.Position, inventory: true,
		})
	}
	return out
}

// mapView draws the canvas, the legend beside it and the cursor readout under
// it.
func (m *Model) mapView(cols, rows int) []string {
	features := featuresOf(m.data)

	side := sidePanelWidth
	if cols < 92 {
		side = 0
	}
	mapWidth := cols - side
	if side > 0 {
		mapWidth--
	}

	// Two rows go to the readout under the map, two to the canvas border.
	canvasRows := rows - 2 - 2
	canvasCols := mapWidth - 4
	if canvasCols < 4 || canvasRows < 2 {
		return []string{m.theme.Warn("the terminal is too small to draw the map")}
	}

	canvas := render.NewCanvas(canvasCols, canvasRows)
	box := m.box.FitAspect(canvas.Width(), canvas.Height())
	projection := render.NewProjection(box, canvas.Width(), canvas.Height())

	drawn, offscreen := plot(canvas, features, projection)
	crosshair(canvas)

	block := m.panel("map · córdoba", mapWidth, canvas.StyledLines(m.theme))
	if side > 0 {
		block = beside(1, block, m.legend(side, features, drawn))
	}

	return append(block, m.cursorLines(cols, projection, features, drawn, offscreen)...)
}

// plot draws every feature it can reach, and counts the ones it cannot.
//
// Something off the edge of the viewport is not drawn on the edge of it: a map
// that clamps out-of-range features to its border invents positions, which is
// the one thing a map must never do.
func plot(canvas *render.Canvas, features []feature, p render.Projection) (drawn, offscreen int) {
	for _, f := range features {
		x, y, ok := p.Project(f.position.Lon, f.position.Lat)
		if !ok {
			offscreen++
			continue
		}
		glyph, colour := glyphFor(f)
		col, row := canvas.Cell(x, y)
		canvas.Glyph(col, row, glyph, colour)
		drawn++
	}
	return drawn, offscreen
}

// crosshair marks the centre of the viewport, which is the point the readout
// reports.
func crosshair(canvas *render.Canvas) {
	cx, cy := canvas.Width()/2, canvas.Height()/2
	canvas.Pen(render.ColorSlate)

	// A gap in the middle, so the crosshair never hides what it points at.
	for d := 3; d <= 9; d++ {
		canvas.Set(cx-d, cy)
		canvas.Set(cx+d, cy)
		canvas.Set(cx, cy-d)
		canvas.Set(cx, cy+d)
	}
	canvas.Pen(render.ColorDefault)
}

// legend lists what kinds are on screen and how many of each.
func (m *Model) legend(width int, features []feature, drawn int) []string {
	t := m.theme

	counts := map[string]int{}
	for _, f := range features {
		counts[f.topic]++
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

	inner := make([]string, 0, len(topics)+2)
	for _, topic := range topics {
		glyph, _ := glyphFor(feature{topic: topic})
		inner = append(inner, render.Clip(
			t.Live(string(glyph))+" "+t.Label(render.PadVisible(topic, 12))+
				t.Dim(fmt.Sprint(counts[topic])), width-4))
	}
	if len(inner) == 0 {
		inner = append(inner, t.Warn("nothing on the map"))
	}
	inner = append(inner,
		t.Chrome(strings.Repeat("─", max(width-4, 0))),
		t.Dim(fmt.Sprintf("%d of %d plotted", drawn, len(features))),
		t.Dim("□ inventory · others live"))

	return m.panel("on screen", width, inner)
}

// cursorLines report where the crosshair is and what is nearest to it.
func (m *Model) cursorLines(cols int, p render.Projection, features []feature, drawn, offscreen int) []string {
	t := m.theme
	box := p.Box()

	lon, lat := box.Centre()
	line := fmt.Sprintf(" %s %s", t.Label("cursor"),
		t.Live(fmt.Sprintf("%.4f, %.4f", lon, lat)))

	if len(features) == 0 {
		line += t.Chrome(" · ") + t.Warn("nothing on the map: no source in this read published a position")
	} else if near, distance, ok := nearest(features, observation.Point{Lat: lat, Lon: lon}); ok {
		line += t.Chrome(" · ") + t.Label("nearest ") +
			t.Live(render.Clip(near.title, max(cols/3, 12))) +
			t.Dim(fmt.Sprintf(" (%.2f km)", distance))
	}

	extent := fmt.Sprintf(" %s %.3f,%.3f → %.3f,%.3f · %s",
		t.Label("box"), box.West, box.South, box.East, box.North,
		t.Dim(fmt.Sprintf("%d drawn · %d outside the viewport", drawn, offscreen)))

	return []string{render.Clip(line, cols), render.Clip(extent, cols)}
}

// nearest returns the feature closest to a point.
func nearest(features []feature, to observation.Point) (feature, float64, bool) {
	if len(features) == 0 {
		return feature{}, 0, false
	}

	best, bestDistance := features[0], to.DistanceKm(features[0].position)
	for _, f := range features[1:] {
		if d := to.DistanceKm(f.position); d < bestDistance {
			best, bestDistance = f, d
		}
	}
	return best, bestDistance, true
}
