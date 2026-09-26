package render

import (
	"math"
	"strings"
)

// Braille is the highest-resolution drawing surface a terminal offers without
// a graphics protocol. U+2800..U+28FF packs a 2x4 dot grid into a single
// character, so an ordinary 100x30 terminal becomes a 200x120 pixel canvas —
// enough to draw a city, and it costs the dependency budget nothing.
//
// The bit order is historical rather than logical. Braille was a six-dot
// alphabet before the eight-dot computer extension was added, so the fourth
// row's two dots were appended in the high bits instead of interleaved:
//
//	dot 1 (bit 0)   dot 4 (bit 3)
//	dot 2 (bit 1)   dot 5 (bit 4)
//	dot 3 (bit 2)   dot 6 (bit 5)
//	dot 7 (bit 6)   dot 8 (bit 7)
const brailleBase = 0x2800

// dotBit maps a position inside a cell to the bit it sets. Indexed [x][y].
var dotBit = [2][4]uint8{
	{0x01, 0x02, 0x04, 0x40},
	{0x08, 0x10, 0x20, 0x80},
}

// Color names a canvas colour. It is an enumeration rather than an RGB triple
// so the palette stays the one the rest of eye uses: phosphor for live data,
// amber for anything the reader must not take at face value, red for errors.
type Color uint8

// The canvas palette.
const (
	// ColorDefault leaves the cell unstyled, taking the terminal's colour.
	ColorDefault Color = iota
	// ColorPhosphor is live, sourced data.
	ColorPhosphor
	// ColorAmber is data the reader should not take at face value.
	ColorAmber
	// ColorCrimson is an error or an alert.
	ColorCrimson
	// ColorCyan is a label or a reference feature.
	ColorCyan
	// ColorSlate is chrome: graticules, borders, the frame.
	ColorSlate
)

// sequence returns the ANSI escape for a colour.
func (c Color) sequence() string {
	switch c {
	case ColorPhosphor:
		return phosphor
	case ColorAmber:
		return amber
	case ColorCrimson:
		return crimson
	case ColorCyan:
		return cyan
	case ColorSlate:
		return slate
	default:
		return ""
	}
}

// AnsiTheme returns a theme with styling forced on.
//
// NewTheme asks the writer whether it is a terminal, which is the right
// question for a command's stdout and the wrong one for a frame that is
// composed in memory and written out later. The full-screen views build their
// frame in a buffer, so they have to decide about colour themselves.
func AnsiTheme() Theme { return Theme{enabled: true} }

// Canvas is a braille pixel grid drawn over a grid of character cells.
//
// Coordinates are in dots, with the origin at the top left. Colour is recorded
// per cell rather than per dot, because a terminal cell has exactly one
// foreground colour — two differently coloured dots cannot share one, and the
// last one written wins.
type Canvas struct {
	cols, rows int
	dots       []uint8
	colors     []Color
	glyphs     []rune

	// pen is the colour Set, Line and Rect draw with.
	pen Color
}

// NewCanvas builds a blank canvas of cols x rows character cells, which is
// 2*cols by 4*rows dots.
func NewCanvas(cols, rows int) *Canvas {
	if cols < 0 {
		cols = 0
	}
	if rows < 0 {
		rows = 0
	}
	return &Canvas{
		cols:   cols,
		rows:   rows,
		dots:   make([]uint8, cols*rows),
		colors: make([]Color, cols*rows),
		glyphs: make([]rune, cols*rows),
	}
}

// Width is the canvas width in dots.
func (c *Canvas) Width() int { return c.cols * 2 }

// Height is the canvas height in dots.
func (c *Canvas) Height() int { return c.rows * 4 }

// Cols is the canvas width in character cells.
func (c *Canvas) Cols() int { return c.cols }

// Rows is the canvas height in character cells.
func (c *Canvas) Rows() int { return c.rows }

// Pen sets the colour subsequent Set, Line and Rect calls draw with.
func (c *Canvas) Pen(col Color) { c.pen = col }

// Set lights a dot in the current pen colour. Dots outside the canvas are
// dropped rather than wrapped: a feature off the edge of the map is off the
// map, not on the other side of it.
func (c *Canvas) Set(x, y int) { c.SetColor(x, y, c.pen) }

// SetColor lights a dot and gives its cell a colour.
func (c *Canvas) SetColor(x, y int, col Color) {
	i, bit, ok := c.index(x, y)
	if !ok {
		return
	}
	c.dots[i] |= bit
	if col != ColorDefault {
		c.colors[i] = col
	}
}

// Glyph replaces a whole cell with a character, overriding the braille dots
// underneath it.
//
// Braille draws position; a glyph draws kind. A map that plots everything as
// identical dots answers "where is there something" and never "what". Glyphs
// are per cell because a cell is the smallest thing a terminal can put a
// character in.
func (c *Canvas) Glyph(col, row int, r rune, colour Color) {
	if col < 0 || row < 0 || col >= c.cols || row >= c.rows {
		return
	}
	i := row*c.cols + col
	c.glyphs[i] = r
	if colour != ColorDefault {
		c.colors[i] = colour
	}
}

// Cell returns the character cell a dot falls in.
func (c *Canvas) Cell(x, y int) (col, row int) { return x / 2, y / 4 }

// Get reports whether a dot is lit.
func (c *Canvas) Get(x, y int) bool {
	i, bit, ok := c.index(x, y)
	return ok && c.dots[i]&bit != 0
}

// Clear erases every dot and every colour.
func (c *Canvas) Clear() {
	for i := range c.dots {
		c.dots[i] = 0
		c.colors[i] = ColorDefault
		c.glyphs[i] = 0
	}
}

// Line draws a straight line between two dots using Bresenham's algorithm,
// which needs no floating point and lands exactly on both endpoints.
func (c *Canvas) Line(x0, y0, x1, y1 int) {
	dx := abs(x1 - x0)
	dy := -abs(y1 - y0)
	sx, sy := step(x0, x1), step(y0, y1)
	err := dx + dy

	for {
		c.Set(x0, y0)
		if x0 == x1 && y0 == y1 {
			return
		}
		// The doubled error decides which axis advances, so a shallow
		// line steps horizontally most of the time and a steep one
		// vertically, with no gaps either way.
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

// Rect draws the outline of a rectangle. It is an outline and not a fill
// because at braille resolution a filled box hides everything underneath it.
func (c *Canvas) Rect(x0, y0, x1, y1 int) {
	c.Line(x0, y0, x1, y0)
	c.Line(x1, y0, x1, y1)
	c.Line(x1, y1, x0, y1)
	c.Line(x0, y1, x0, y0)
}

// Lines returns the canvas as one string per character row, unstyled.
func (c *Canvas) Lines() []string {
	out := make([]string, 0, c.rows)
	for row := range c.rows {
		var sb strings.Builder
		sb.Grow(c.cols * 3)
		for col := range c.cols {
			sb.WriteRune(c.cell(row*c.cols + col))
		}
		out = append(out, sb.String())
	}
	return out
}

// String returns the whole canvas, unstyled, rows separated by newlines.
func (c *Canvas) String() string { return strings.Join(c.Lines(), "\n") }

// StyledLines returns one string per character row with per-cell colour
// applied, or the plain rows when the theme has styling off.
//
// Runs of the same colour share one escape sequence. A 200x120 canvas is 6000
// cells, and one sequence per cell would be more escape bytes than picture.
func (c *Canvas) StyledLines(t Theme) []string {
	if !t.Enabled() {
		return c.Lines()
	}

	out := make([]string, 0, c.rows)
	for row := range c.rows {
		var sb strings.Builder
		open := ColorDefault

		for col := range c.cols {
			i := row*c.cols + col
			if want := c.colors[i]; want != open {
				if open != ColorDefault {
					sb.WriteString(reset)
				}
				sb.WriteString(want.sequence())
				open = want
			}
			sb.WriteRune(c.cell(i))
		}
		if open != ColorDefault {
			sb.WriteString(reset)
		}
		out = append(out, sb.String())
	}
	return out
}

// Render returns the whole canvas with colour applied.
func (c *Canvas) Render(t Theme) string { return strings.Join(c.StyledLines(t), "\n") }

// cell returns the character a cell renders as: its glyph when it has one, and
// the braille pattern of its dots otherwise.
func (c *Canvas) cell(i int) rune {
	if c.glyphs[i] != 0 {
		return c.glyphs[i]
	}
	// The dot mask is a byte, so adding it to the block base can never leave
	// the braille range; converting the byte rather than a widened int is
	// what makes that obvious to a reader and to the compiler.
	return brailleBase + rune(c.dots[i])
}

// index resolves a dot to its cell and bit.
func (c *Canvas) index(x, y int) (i int, bit uint8, ok bool) {
	if x < 0 || y < 0 || x >= c.Width() || y >= c.Height() {
		return 0, 0, false
	}
	return (y/4)*c.cols + x/2, dotBit[x%2][y%4], true
}

// GeoBox is a WGS84 bounding box in the order a map thinks in.
//
// It repeats observation.BBox rather than importing it on purpose: render
// draws pixels and knows nothing about observations, and a drawing package
// that imports the domain is a drawing package the domain will eventually
// import back.
type GeoBox struct {
	West  float64
	South float64
	East  float64
	North float64
}

// Pad grows a box by a fraction of its own size on every side, and gives a
// degenerate box an extent so a single point still has a map around it.
func (b GeoBox) Pad(fraction float64) GeoBox {
	dx := (b.East - b.West) * fraction
	dy := (b.North - b.South) * fraction

	// minimumSpan is roughly a kilometre at this latitude. A box with no
	// extent divides by zero in the projection, and one point is a perfectly
	// ordinary thing for the store to hold.
	const minimumSpan = 0.01
	if dx <= 0 {
		dx = minimumSpan
	}
	if dy <= 0 {
		dy = minimumSpan
	}

	return GeoBox{
		West: b.West - dx, East: b.East + dx,
		South: b.South - dy, North: b.North + dy,
	}
}

// FitAspect widens a box so that it fills a canvas of the given pixel size
// without distorting what is drawn on it.
//
// It only ever grows the box. Cropping to fit would hide data that the caller
// asked to see, and stretching would draw Cordoba as an ellipse.
func (b GeoBox) FitAspect(width, height int) GeoBox {
	if width <= 0 || height <= 0 {
		return b
	}

	// The comparison happens in Mercator space, in the same units on both
	// axes. Comparing degrees of longitude against projected latitude is the
	// bug this function exists to prevent, so it must not make it itself:
	// longitude goes to radians, which is what the projected y already is.
	south, north := mercator(b.South), mercator(b.North)
	spanX, spanY := rad(b.East-b.West), north-south
	if spanX <= 0 || spanY <= 0 {
		return b
	}

	want := float64(width) / float64(height)
	if have := spanX / spanY; have < want {
		grow := deg((spanY*want - spanX) / 2)
		b.West -= grow
		b.East += grow
	} else {
		grow := (spanX/want - spanY) / 2
		b.South = inverseMercator(south - grow)
		b.North = inverseMercator(north + grow)
	}
	return b
}

// Centre is the middle of the box.
func (b GeoBox) Centre() (lon, lat float64) {
	return (b.West + b.East) / 2, (b.South + b.North) / 2
}

// Zoom scales a box about its centre. A factor below 1 zooms in.
func (b GeoBox) Zoom(factor float64) GeoBox {
	if factor <= 0 {
		return b
	}
	lon, lat := b.Centre()
	dx := (b.East - b.West) * factor / 2
	dy := (b.North - b.South) * factor / 2
	return GeoBox{West: lon - dx, East: lon + dx, South: lat - dy, North: lat + dy}
}

// Shift moves a box by a fraction of its own width and height.
func (b GeoBox) Shift(dx, dy float64) GeoBox {
	w, h := b.East-b.West, b.North-b.South
	return GeoBox{
		West: b.West + w*dx, East: b.East + w*dx,
		South: b.South + h*dy, North: b.North + h*dy,
	}
}

// Projection maps WGS84 coordinates onto canvas dots and back.
//
// Latitude goes through Web Mercator so that a braille dot is very nearly
// square in ground terms and the city keeps its shape. Longitude is linear,
// which Mercator also is.
type Projection struct {
	box           GeoBox
	width, height int

	// south and north are the box's latitudes already in Mercator space,
	// cached because every projected point needs them.
	south, north float64
}

// NewProjection builds a projection of a box onto a canvas of width by height
// dots.
func NewProjection(box GeoBox, width, height int) Projection {
	return Projection{
		box: box, width: width, height: height,
		south: mercator(box.South), north: mercator(box.North),
	}
}

// Box returns the bounding box the projection covers.
func (p Projection) Box() GeoBox { return p.box }

// Project maps a coordinate to a dot. ok is false when the point falls outside
// the box: eye draws what it can see and says nothing about the rest.
func (p Projection) Project(lon, lat float64) (x, y int, ok bool) {
	if p.width <= 1 || p.height <= 1 {
		return 0, 0, false
	}
	if lon < p.box.West || lon > p.box.East || lat < p.box.South || lat > p.box.North {
		return 0, 0, false
	}

	spanX := p.box.East - p.box.West
	spanY := p.north - p.south
	if spanX <= 0 || spanY <= 0 {
		return 0, 0, false
	}

	x = int(math.Round((lon - p.box.West) / spanX * float64(p.width-1)))
	y = int(math.Round((p.north - mercator(lat)) / spanY * float64(p.height-1)))
	return x, y, true
}

// Invert maps a dot back to the coordinate at its centre, which is what turns
// a cursor on the map into a position a person can look up.
func (p Projection) Invert(x, y int) (lon, lat float64) {
	if p.width <= 1 || p.height <= 1 {
		return p.box.Centre()
	}

	lon = p.box.West + (p.box.East-p.box.West)*float64(x)/float64(p.width-1)
	merc := p.north - (p.north-p.south)*float64(y)/float64(p.height-1)
	return lon, inverseMercator(merc)
}

// mercator is the Web Mercator y for a latitude, in radians of the projected
// plane. Latitudes are clamped short of the poles, where the projection runs
// off to infinity.
func mercator(lat float64) float64 {
	const limit = 85.05112878
	lat = math.Max(-limit, math.Min(limit, lat))
	return math.Log(math.Tan(math.Pi/4 + rad(lat)/2))
}

// inverseMercator turns a projected y back into a latitude.
func inverseMercator(y float64) float64 {
	return deg(2*math.Atan(math.Exp(y)) - math.Pi/2)
}

func rad(d float64) float64 { return d * math.Pi / 180 }
func deg(r float64) float64 { return r * 180 / math.Pi }

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// step returns the direction to walk from a towards b.
func step(a, b int) int {
	if a < b {
		return 1
	}
	return -1
}
