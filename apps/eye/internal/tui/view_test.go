package tui_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	"github.com/FullFran/cordvba/apps/eye/internal/render"
	"github.com/FullFran/cordvba/apps/eye/internal/tui"
)

// errRead stands in for a store that could not be read.
var errRead = errors.New("database is locked")

// Every frame carries the header, whatever view is on. Losing it means losing
// the clock and the counts, which is what makes the thing a board.
func TestEveryViewDrawsTheHeaderAndFooter(t *testing.T) {
	t.Parallel()

	views := map[rune]string{
		'1': "DASHBOARD",
		'2': "MAP",
		'3': "FEED",
		'4': "SOURCES",
		'5': "TRANSIT",
	}

	for key, name := range views {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			m := newTestModel(t)
			feed(m, testData())
			press(m, key)

			out := frame(m, 110, 34)
			for _, want := range []string{"EYE", name, "15:04:05", "? help", "q quit"} {
				if !strings.Contains(out, want) {
					t.Errorf("the %s frame is missing %q:\n%s", name, want, out)
				}
			}
		})
	}
}

// No line may be wider than the terminal, or it wraps and the whole layout
// slides down the screen a row at a time.
func TestNoLineIsWiderThanTheTerminal(t *testing.T) {
	t.Parallel()

	sizes := []tui.Size{{Cols: 80, Rows: 24}, {Cols: 110, Rows: 34}, {Cols: 200, Rows: 60}, {Cols: 40, Rows: 12}}

	for _, key := range []rune{'1', '2', '3', '4', '5'} {
		for _, size := range sizes {
			m := tui.New(tui.Options{
				Version: "dev", Registry: "(embedded)", Theme: render.AnsiTheme(),
				Interval: 5 * time.Second, StartedAt: testNow, Now: testNow, Interactive: true,
			})
			feed(m, testData())
			press(m, key)

			var sb strings.Builder
			m.View(&sb, size)

			for i, line := range strings.Split(strings.TrimSuffix(sb.String(), "\n"), "\n") {
				if got := render.VisibleWidth(line); got > size.Cols {
					t.Fatalf("view %q at %dx%d: line %d is %d columns wide: %q",
						key, size.Cols, size.Rows, i, got, line)
				}
			}
		}
	}
}

// A frame taller than the terminal scrolls the top off, which on an alternate
// screen means the header disappears.
func TestAFrameNeverOverflowsTheTerminalHeight(t *testing.T) {
	t.Parallel()

	for _, key := range []rune{'1', '2', '3', '4', '5'} {
		m := newTestModel(t)
		feed(m, testData())
		press(m, key)

		out := frame(m, 100, 20)
		if got := strings.Count(strings.TrimSuffix(out, "\n"), "\n") + 1; got > 20 {
			t.Errorf("view %q drew %d rows into a 20-row terminal", key, got)
		}
	}
}

// A terminal too small to draw in must say so rather than panic or draw
// nonsense.
func TestATinyTerminalIsHandled(t *testing.T) {
	t.Parallel()

	for _, size := range []tui.Size{{Cols: 0, Rows: 0}, {Cols: 5, Rows: 2}, {Cols: 20, Rows: 4}} {
		m := newTestModel(t)
		feed(m, testData())
		var sb strings.Builder
		m.View(&sb, size)
	}
}

func TestDashboardShowsHealthTopicsAndARateHistogram(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	feed(m, testData())

	out := frame(m, 110, 34)
	for _, want := range []string{
		"dev (abc1234)", // build identity
		"up 12m",        // uptime
		"4 records",     // store counts
		"2 entities",
		"SOURCE HEALTH",
		"dgt-incidents",
		"saih-guadalquivir",
		"503 from upstream", // the last error, verbatim
		"BY TOPIC",
		"traffic",
		"press",
		"RECORDS PER HOUR",
		"(embedded)", // the registry in force
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the dashboard is missing %q:\n%s", want, out)
		}
	}
}

// A source that has never answered, one that answered two minutes ago and one
// held out of the scheduler are three different states, and the board must not
// flatten them into one.
func TestDashboardDistinguishesHeldFromFailing(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	feed(m, testData())

	out := frame(m, 110, 34)
	if !strings.Contains(out, "review_terms") {
		t.Errorf("a held source is not shown as held:\n%s", out)
	}
	if !strings.Contains(out, "failing") {
		t.Errorf("a source that has never succeeded is not shown as failing:\n%s", out)
	}
}

func TestMapPlotsFeaturesAndReportsTheCursor(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	feed(m, testData())
	press(m, '2')

	out := frame(m, 110, 34)
	if !strings.Contains(out, "ON SCREEN") {
		t.Errorf("the map has no side panel:\n%s", out)
	}
	// The cursor position is what makes the map answerable rather than
	// decorative.
	if !strings.Contains(out, "cursor") {
		t.Errorf("the map does not report the cursor position:\n%s", out)
	}
	if !strings.Contains(out, "37.") || !strings.Contains(out, "-4.") {
		t.Errorf("the cursor position is not in WGS84:\n%s", out)
	}
	// Braille is how the map is drawn; without it there is no map.
	if !strings.ContainsAny(out, "⠁⠂⠄⡀⠈⠐⠠⢀⣿") {
		t.Errorf("nothing was plotted on the canvas:\n%s", out)
	}
}

func TestMapSaysSoWhenNothingHasAPosition(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	feed(m, tui.Data{
		ReadAt: testNow,
		Records: []observation.Record{
			testRecord("r1", "boe", "civic", "Resolución", 5, nil),
		},
	})
	press(m, '2')

	if out := frame(m, 110, 34); !strings.Contains(out, "nothing on the map") {
		t.Errorf("an empty map does not explain itself:\n%s", out)
	}
}

func TestFeedListsTheNewestFirstWithItsColumns(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	feed(m, testData())
	press(m, '3')

	out := frame(m, 110, 34)
	for _, want := range []string{"TIME", "SOURCE", "SEV", "TITLE", "RYR1234", "adsb-lol"} {
		if !strings.Contains(out, want) {
			t.Errorf("the feed is missing %q:\n%s", want, out)
		}
	}

	// Newest first: the one-minute-old aircraft above the ninety-minute-old
	// reservoir.
	air := strings.Index(out, "RYR1234")
	water := strings.Index(out, "Iznajar")
	if air < 0 || water < 0 || air > water {
		t.Errorf("the feed is not newest-first:\n%s", out)
	}
}

func TestSourcesShowsTheWholeRegistryEntry(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	feed(m, testData())
	press(m, '4')

	out := frame(m, 130, 34)
	for _, want := range []string{
		"SOURCE", "AUTHORITY", "TOPIC", "FORMAT", "LICENCE", "ACCESS", "AUTOMATION", "LAST OK",
		"dgt-incidents", "DGT", "datex2", "CC BY 4.0", "documented_api", "enabled",
		"held-source", "review_terms",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the registry view is missing %q:\n%s", want, out)
		}
	}
}

func TestSourcesShowsTheLastErrorOfTheSelectedSource(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	feed(m, testData())
	press(m, '4')
	hit(m, tui.KeyDown)

	if out := frame(m, 130, 34); !strings.Contains(out, "503 from upstream") {
		t.Errorf("the selected source's last error is not shown:\n%s", out)
	}
}

func TestTransitListsStopsAndTrains(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	d := testData()
	departure := testNow.Add(25 * time.Minute)
	train := testRecord("t1", "renfe-gtfs", "transport", "MD 13072 → Sevilla", 1, nil)
	train.Kind = "scheduled_departure"
	train.ValidFrom = &departure
	d.Records = append(d.Records, train)
	feed(m, d)
	press(m, '5')

	out := frame(m, 110, 34)
	for _, want := range []string{"STOPS", "ARRIVALS", "TRAINS", "Trassierra", "Sevilla"} {
		if !strings.Contains(out, want) {
			t.Errorf("the transit view is missing %q:\n%s", want, out)
		}
	}
}

func TestTransitSaysArrivalsAreOperatorEstimates(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	feed(m, testData())
	press(m, '5')

	// The distinction between a measurement and somebody's prediction has to
	// be on the screen that shows the prediction.
	if out := frame(m, 110, 34); !strings.Contains(out, "operator estimate") {
		t.Errorf("the arrivals panel does not say what it is showing:\n%s", out)
	}
}

// Without a keyboard the cockpit still refreshes, and it must say that is what
// it is doing rather than looking frozen.
func TestNonInteractiveModeSaysSo(t *testing.T) {
	t.Parallel()

	m := tui.New(tui.Options{
		Version: "dev", Registry: "(embedded)", Theme: render.PlainTheme(),
		Interval: 5 * time.Second, StartedAt: testNow, Now: testNow,
		Interactive: false, InputNote: "no /dev/tty",
	})
	feed(m, testData())

	out := frame(m, 100, 30)
	if !strings.Contains(out, "no keyboard") {
		t.Errorf("the frame does not say input is unavailable:\n%s", out)
	}
	if !strings.Contains(out, "no /dev/tty") {
		t.Errorf("the frame does not say why input is unavailable:\n%s", out)
	}
}

// Styling on must not change what the frame says, only how it looks.
func TestStylingChangesNothingButTheColours(t *testing.T) {
	t.Parallel()

	plain := newTestModel(t)
	feed(plain, testData())

	styled := tui.New(tui.Options{
		Version: "dev (abc1234)", Registry: "(embedded)", Theme: render.AnsiTheme(),
		Interval: 5 * time.Second, StartedAt: testNow.Add(-12 * time.Minute), Now: testNow,
		Interactive: true,
	})
	feed(styled, testData())

	var got strings.Builder
	styled.View(&got, tui.Size{Cols: 110, Rows: 34})

	if !strings.Contains(got.String(), "\x1b[") {
		t.Fatal("the styled frame carries no escape sequences")
	}
	if strip(got.String()) != frame(plain, 110, 34) {
		t.Error("styling changed the text of the frame, not just its colours")
	}
}

// strip removes escape sequences so two frames can be compared as text.
func strip(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); {
		if s[i] != 0x1b {
			sb.WriteByte(s[i])
			i++
			continue
		}
		j := i + 2
		for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
			j++
		}
		i = j + 1
	}
	return sb.String()
}
