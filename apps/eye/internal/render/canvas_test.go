package render_test

import (
	"math"
	"strings"
	"testing"

	"github.com/FullFran/cordvba/apps/eye/internal/render"
)

// A braille cell packs a 2x4 dot grid into one character, and the bit order is
// not the obvious one: the fourth row was bolted onto a six-dot alphabet, so
// its two dots live in the high bits. Getting this wrong draws a map that looks
// almost right, which is the worst kind of wrong.
func TestCanvasPacksEveryDotPosition(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		x, y int
		want rune
	}{
		{name: "dot 1 top left", x: 0, y: 0, want: '⠁'},
		{name: "dot 2", x: 0, y: 1, want: '⠂'},
		{name: "dot 3", x: 0, y: 2, want: '⠄'},
		{name: "dot 7 bottom left", x: 0, y: 3, want: '⡀'},
		{name: "dot 4 top right", x: 1, y: 0, want: '⠈'},
		{name: "dot 5", x: 1, y: 1, want: '⠐'},
		{name: "dot 6", x: 1, y: 2, want: '⠠'},
		{name: "dot 8 bottom right", x: 1, y: 3, want: '⢀'},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := render.NewCanvas(1, 1)
			c.Set(tc.x, tc.y)
			if got := c.String(); got != string(tc.want) {
				t.Errorf("Set(%d,%d) = %q (U+%04X), want %q (U+%04X)",
					tc.x, tc.y, got, []rune(got)[0], string(tc.want), tc.want)
			}
		})
	}
}

func TestCanvasFullCellIsTheLastBrailleGlyph(t *testing.T) {
	t.Parallel()

	c := render.NewCanvas(1, 1)
	for y := range 4 {
		for x := range 2 {
			c.Set(x, y)
		}
	}
	if got := c.String(); got != "⣿" {
		t.Errorf("a full cell = %q, want U+28FF", got)
	}
}

// An empty cell is the blank braille pattern, not a space: every row must be
// the same number of columns wide or the side panel beside the map shifts.
func TestCanvasBlankCellKeepsItsColumn(t *testing.T) {
	t.Parallel()

	c := render.NewCanvas(3, 2)
	lines := c.Lines()
	if len(lines) != 2 {
		t.Fatalf("Lines() = %d rows, want 2", len(lines))
	}
	for i, line := range lines {
		if line != "⠀⠀⠀" {
			t.Errorf("row %d = %q, want three blank braille cells", i, line)
		}
	}
}

func TestCanvasSizeIsDotsNotCells(t *testing.T) {
	t.Parallel()

	c := render.NewCanvas(10, 5)
	if w, h := c.Width(), c.Height(); w != 20 || h != 20 {
		t.Errorf("canvas is %dx%d dots, want 20x20", w, h)
	}
}

func TestCanvasIgnoresDotsOutsideItself(t *testing.T) {
	t.Parallel()

	c := render.NewCanvas(1, 1)
	for _, p := range [][2]int{{-1, 0}, {0, -1}, {2, 0}, {0, 4}} {
		c.Set(p[0], p[1])
	}
	if got := c.String(); got != "⠀" {
		t.Errorf("an out-of-range dot was drawn: %q", got)
	}
}

func TestCanvasClearErasesEverything(t *testing.T) {
	t.Parallel()

	c := render.NewCanvas(2, 1)
	c.Set(0, 0)
	c.SetColor(3, 3, render.ColorAmber)
	c.Clear()

	if got := c.String(); got != "⠀⠀" {
		t.Errorf("Clear() left %q", got)
	}
	if c.Get(0, 0) {
		t.Error("Clear() left a dot set")
	}
}

// Bresenham has to land exactly on both endpoints. A line that stops one dot
// short is invisible at this scale and impossible to debug later.
func TestCanvasLineHitsBothEndpoints(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		x0, y0, x1, y1 int
	}{
		{name: "horizontal", x0: 0, y0: 0, x1: 19, y1: 0},
		{name: "vertical", x0: 3, y0: 0, x1: 3, y1: 19},
		{name: "diagonal", x0: 0, y0: 0, x1: 19, y1: 19},
		{name: "shallow", x0: 0, y0: 2, x1: 19, y1: 5},
		{name: "steep", x0: 2, y0: 0, x1: 5, y1: 19},
		{name: "backwards", x0: 19, y0: 19, x1: 0, y1: 0},
		{name: "single dot", x0: 7, y0: 7, x1: 7, y1: 7},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := render.NewCanvas(10, 5)
			c.Line(tc.x0, tc.y0, tc.x1, tc.y1)

			if !c.Get(tc.x0, tc.y0) {
				t.Errorf("the line missed its start (%d,%d)", tc.x0, tc.y0)
			}
			if !c.Get(tc.x1, tc.y1) {
				t.Errorf("the line missed its end (%d,%d)", tc.x1, tc.y1)
			}
		})
	}
}

func TestCanvasLineIsContinuous(t *testing.T) {
	t.Parallel()

	c := render.NewCanvas(10, 5)
	c.Line(0, 0, 19, 9)

	// Every column between the endpoints must carry at least one dot, or
	// the line is dashed rather than drawn.
	for x := range 20 {
		found := false
		for y := range 20 {
			if c.Get(x, y) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("column %d has no dot: the line has a gap", x)
		}
	}
}

func TestCanvasRectDrawsAnOutlineNotAFill(t *testing.T) {
	t.Parallel()

	c := render.NewCanvas(10, 5)
	c.Rect(2, 2, 8, 8)

	for _, corner := range [][2]int{{2, 2}, {8, 2}, {2, 8}, {8, 8}} {
		if !c.Get(corner[0], corner[1]) {
			t.Errorf("corner (%d,%d) is missing", corner[0], corner[1])
		}
	}
	if c.Get(5, 5) {
		t.Error("Rect filled its interior; it must draw an outline")
	}
}

// The map must be able to turn a cursor position back into coordinates, so the
// projection has to be invertible to within one pixel.
func TestProjectionRoundTrip(t *testing.T) {
	t.Parallel()

	box := render.GeoBox{West: -5.05, South: 37.79, East: -4.65, North: 37.95}
	p := render.NewProjection(box, 200, 120)

	cases := []struct {
		name     string
		lon, lat float64
	}{
		{name: "centre of Cordoba", lon: -4.7794, lat: 37.8882},
		{name: "north west corner", lon: -5.05, lat: 37.95},
		{name: "south east corner", lon: -4.65, lat: 37.79},
		{name: "off centre", lon: -4.9, lat: 37.82},
	}

	// One pixel is the honest tolerance: the projection quantises to a dot
	// grid, so a round trip cannot be more precise than the grid is.
	lonStep := (box.East - box.West) / 199
	latStep := (box.North - box.South) / 119

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			x, y, ok := p.Project(tc.lon, tc.lat)
			if !ok {
				t.Fatalf("Project(%f,%f) reported the point as off-canvas", tc.lon, tc.lat)
			}

			lon, lat := p.Invert(x, y)
			if math.Abs(lon-tc.lon) > lonStep {
				t.Errorf("longitude round trip: %f -> %f (tolerance %f)", tc.lon, lon, lonStep)
			}
			if math.Abs(lat-tc.lat) > latStep {
				t.Errorf("latitude round trip: %f -> %f (tolerance %f)", tc.lat, lat, latStep)
			}
		})
	}
}

func TestProjectionRejectsPointsOutsideTheBox(t *testing.T) {
	t.Parallel()

	p := render.NewProjection(render.GeoBox{West: -5, South: 37, East: -4, North: 38}, 100, 100)
	if _, _, ok := p.Project(-3.5, 37.5); ok {
		t.Error("a point east of the box was projected onto the canvas")
	}
	if _, _, ok := p.Project(-4.5, 39); ok {
		t.Error("a point north of the box was projected onto the canvas")
	}
}

// Web Mercator, not a linear ramp: on a linear projection the midpoint of a
// latitude span lands exactly halfway, and shapes come out squashed.
func TestProjectionUsesWebMercatorForLatitude(t *testing.T) {
	t.Parallel()

	// A tall box makes the difference measurable at this resolution.
	box := render.GeoBox{West: -5, South: 0, East: -4, North: 60}
	p := render.NewProjection(box, 100, 1000)

	_, y, ok := p.Project(-4.5, 30)
	if !ok {
		t.Fatal("the midpoint of the box is off-canvas")
	}

	linear := 1000 / 2
	if math.Abs(float64(y-linear)) < 5 {
		t.Errorf("latitude 30 landed at row %d, which is the linear answer: Mercator was not applied", y)
	}
}

// A bounding box drawn on a canvas of a different shape must be widened, not
// stretched, or Cordoba comes out as an ellipse.
func TestGeoBoxFitAspectWidensRatherThanStretches(t *testing.T) {
	t.Parallel()

	box := render.GeoBox{West: -4.8, South: 37.85, East: -4.75, North: 37.90}
	fitted := box.FitAspect(200, 50)

	if fitted.West > box.West || fitted.East < box.East {
		t.Errorf("FitAspect narrowed the box: %+v -> %+v", box, fitted)
	}
	if fitted.South > box.South || fitted.North < box.North {
		t.Errorf("FitAspect cropped the box vertically: %+v -> %+v", box, fitted)
	}

	// A square drawn inside the fitted box must come out square on the
	// canvas, which is the whole point of fitting.
	p := render.NewProjection(fitted, 200, 50)
	x0, y0, _ := p.Project(fitted.West, box.South)
	x1, _, _ := p.Project(fitted.East, box.South)
	_, y1, _ := p.Project(fitted.West, fitted.North)

	wide := float64(x1 - x0)
	tall := float64(y0 - y1)
	if wide <= 0 || tall <= 0 {
		t.Fatalf("degenerate projection: %f x %f", wide, tall)
	}
	if ratio := wide / tall; ratio < 3.5 || ratio > 4.5 {
		t.Errorf("the fitted box maps to a %.2f:1 rectangle on a 4:1 canvas", ratio)
	}
}

func TestGeoBoxPadGrowsAroundTheData(t *testing.T) {
	t.Parallel()

	box := render.GeoBox{West: -5, South: 37, East: -4, North: 38}
	padded := box.Pad(0.1)

	if padded.West != -5.1 || padded.East != -3.9 {
		t.Errorf("Pad widened to %f..%f, want -5.1..-3.9", padded.West, padded.East)
	}
	if padded.South != 36.9 || padded.North != 38.1 {
		t.Errorf("Pad heightened to %f..%f, want 36.9..38.1", padded.South, padded.North)
	}
}

// A single point has no extent, and a zero-sized box divides by zero. It has to
// become a small box around that point instead.
func TestGeoBoxPadGivesADegenerateBoxAnExtent(t *testing.T) {
	t.Parallel()

	box := render.GeoBox{West: -4.78, South: 37.88, East: -4.78, North: 37.88}
	padded := box.Pad(0.1)

	if padded.East <= padded.West || padded.North <= padded.South {
		t.Fatalf("a single-point box stayed degenerate: %+v", padded)
	}

	p := render.NewProjection(padded, 40, 40)
	if _, _, ok := p.Project(-4.78, 37.88); !ok {
		t.Error("the point the box was built around is off its own canvas")
	}
}

// Colour is per cell, so a coloured map still costs one escape sequence per
// character rather than per dot.
func TestCanvasRendersColourPerCell(t *testing.T) {
	t.Parallel()

	c := render.NewCanvas(3, 1)
	c.SetColor(0, 0, render.ColorAmber)
	c.SetColor(2, 0, render.ColorAmber)
	c.SetColor(4, 0, render.ColorCrimson)

	plain := c.String()
	if plain != "⠁⠁⠁" {
		t.Fatalf("String() = %q, want the unstyled dots", plain)
	}

	styled := c.Render(render.AnsiTheme())
	if render.VisibleWidth(styled) != 3 {
		t.Errorf("Render() changed the cell count: %q", styled)
	}

	// Two adjacent amber cells share one sequence and one reset; the third
	// cell is a different colour and gets its own. Per-cell sequences would
	// be more escape bytes than picture on a full-screen map.
	if got := strings.Count(styled, "\x1b[0m"); got != 2 {
		t.Errorf("Render() emitted %d resets for two colour runs: %q", got, styled)
	}
}

func TestCanvasRenderIsPlainForAnUnstyledTheme(t *testing.T) {
	t.Parallel()

	c := render.NewCanvas(2, 1)
	c.SetColor(0, 0, render.ColorAmber)

	if got := c.Render(render.PlainTheme()); strings.Contains(got, "\x1b") {
		t.Errorf("a plain theme still produced escape sequences: %q", got)
	}
}

// Braille says where; a glyph says what. A map that plots everything as
// identical dots cannot tell an aircraft from a reservoir.
func TestCanvasGlyphOverridesTheDotsUnderIt(t *testing.T) {
	t.Parallel()

	c := render.NewCanvas(2, 1)
	c.Set(0, 0)
	c.Set(2, 0)
	c.Glyph(1, 0, '▲', render.ColorCyan)

	if got := c.String(); got != "⠁▲" {
		t.Errorf("String() = %q, want the glyph in the second cell", got)
	}
	if !c.Get(2, 0) {
		t.Error("the glyph erased the dot underneath instead of covering it")
	}
}

func TestCanvasGlyphOutsideTheCanvasIsDropped(t *testing.T) {
	t.Parallel()

	c := render.NewCanvas(1, 1)
	for _, p := range [][2]int{{-1, 0}, {0, -1}, {1, 0}, {0, 1}} {
		c.Glyph(p[0], p[1], '▲', render.ColorCyan)
	}
	if got := c.String(); got != "⠀" {
		t.Errorf("an out-of-range glyph was drawn: %q", got)
	}
}

func TestCanvasCellLocatesADot(t *testing.T) {
	t.Parallel()

	c := render.NewCanvas(4, 3)
	if col, row := c.Cell(5, 9); col != 2 || row != 2 {
		t.Errorf("Cell(5,9) = (%d,%d), want (2,2)", col, row)
	}
}
