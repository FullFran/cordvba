package tui_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	"github.com/FullFran/cordvba/apps/eye/internal/render"
	"github.com/FullFran/cordvba/apps/eye/internal/tui"
)

// testNow is a fixed clock, so a frame is the same on every run.
var testNow = time.Date(2026, 9, 3, 15, 4, 5, 0, time.UTC)

// newTestModel builds a model with styling off, so assertions match plain text.
func newTestModel(t *testing.T) *tui.Model {
	t.Helper()

	return tui.New(tui.Options{
		Version:     "dev (abc1234)",
		Registry:    "(embedded)",
		Theme:       render.PlainTheme(),
		Interval:    5 * time.Second,
		StartedAt:   testNow.Add(-12 * time.Minute),
		Now:         testNow,
		Interactive: true,
	})
}

// press feeds one printable key.
func press(m *tui.Model, r rune) tui.Command {
	return m.Update(tui.Event{Kind: tui.EventKey, Key: tui.Key{Code: tui.KeyRune, Rune: r}, At: testNow})
}

// hit feeds one named key.
func hit(m *tui.Model, code tui.KeyCode) tui.Command {
	return m.Update(tui.Event{Kind: tui.EventKey, Key: tui.Key{Code: code}, At: testNow})
}

// feed delivers a store read.
func feed(m *tui.Model, d tui.Data) tui.Command {
	return m.Update(tui.Event{Kind: tui.EventData, Data: &d, At: testNow})
}

// testRecord builds a positioned observation.
func testRecord(id, source, topic, title string, minutesAgo int, pos *observation.Point) observation.Record {
	at := testNow.Add(-time.Duration(minutesAgo) * time.Minute)
	return observation.Record{
		ID: id, Source: source, Kind: "k", Topic: topic,
		ObservedAt: at, FetchedAt: at.Add(30 * time.Second),
		Title: title, Position: pos,
		Severity: observation.SeverityModerate,
		Quality:  observation.QualityOfficial, Confidence: 1,
		Provenance: observation.Provenance{
			Publisher: "Publisher of " + source,
			SourceURL: "https://example.org/" + source,
			License:   "CC BY 4.0",
			FetchedAt: at.Add(30 * time.Second),
			RawHash:   "deadbeefcafe",
		},
	}
}

func at(lat, lon float64) *observation.Point { return &observation.Point{Lat: lat, Lon: lon} }

// testData is a small but complete picture of the city.
func testData() tui.Data {
	return tui.Data{
		ReadAt:      testNow,
		Registry:    "(embedded)",
		RecordCount: 4,
		EntityCount: 2,
		Records: []observation.Record{
			testRecord("r1", "dgt-incidents", "traffic", "A-4 km 399 obras", 3, at(37.88, -4.78)),
			testRecord("r2", "diario-cordoba", "press", "El Ayuntamiento aprueba el presupuesto", 20, nil),
			testRecord("r3", "adsb-lol", "air", "RYR1234", 1, at(37.92, -4.70)),
			testRecord("r4", "saih-guadalquivir", "water", "Embalse de Iznajar", 90, at(37.26, -4.31)),
		},
		Entities: []observation.Entity{
			{
				ID: "cam-1", Source: "dgt-cameras", Kind: "camera", Topic: "traffic",
				Title: "A-4 km 399.1 · CÓRDOBA", Position: at(37.86, -4.80),
				FirstSeen: testNow, LastSeen: testNow,
				Provenance: observation.Provenance{Publisher: "DGT", SourceURL: "https://dgt.es", License: "CC BY 4.0"},
			},
			{
				ID: "stop-401", Source: "aucorsa-lines", Kind: "bus_stop", Topic: "transport",
				Title: "Ctra. Trassierra (Anna Pavlova)", Position: at(37.89, -4.81),
				FirstSeen: testNow, LastSeen: testNow,
				Payload:    json.RawMessage(`{"stop_id":"401"}`),
				Provenance: observation.Provenance{Publisher: "AUCORSA", SourceURL: "https://aucorsa.es", License: "unspecified"},
			},
		},
		Sources: []tui.SourceRow{
			{
				ID: "dgt-incidents", Authority: "DGT", Topic: "traffic", Format: "datex2",
				License: "CC BY 4.0", Access: "documented_api", Automation: "enabled",
				Pollable: true, HasAdapter: true, Interval: 5 * time.Minute,
				LastSuccess: testNow.Add(-2 * time.Minute), LastAttempt: testNow.Add(-2 * time.Minute),
				Records: 42,
			},
			{
				ID: "saih-guadalquivir", Authority: "SAIH", Topic: "water", Format: "wfs",
				License: "unspecified", Access: "documented_api", Automation: "enabled",
				Pollable: true, HasAdapter: true, Interval: time.Hour,
				LastAttempt: testNow.Add(-time.Minute), LastError: "503 from upstream",
			},
			{
				ID: "held-source", Authority: "Alguien", Topic: "city", Format: "rss",
				License: "unspecified", Access: "public_html", Automation: "review_terms",
			},
		},
	}
}

// frame renders one frame to a string.
func frame(m *tui.Model, cols, rows int) string {
	var sb strings.Builder
	m.View(&sb, tui.Size{Cols: cols, Rows: rows})
	return sb.String()
}

func TestTabCyclesThroughEveryView(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	want := []tui.ViewID{tui.ViewMap, tui.ViewFeed, tui.ViewSources, tui.ViewTransit, tui.ViewDashboard}

	for i, expect := range want {
		hit(m, tui.KeyTab)
		if got := m.ViewID(); got != expect {
			t.Fatalf("after %d tabs the view is %v, want %v", i+1, got, expect)
		}
	}
}

func TestBackTabGoesTheOtherWay(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	hit(m, tui.KeyBackTab)

	if got := m.ViewID(); got != tui.ViewTransit {
		t.Errorf("back-tab from the dashboard went to %v, want the last view", got)
	}
}

func TestNumberKeysJumpStraightToAView(t *testing.T) {
	t.Parallel()

	cases := map[rune]tui.ViewID{
		'1': tui.ViewDashboard,
		'2': tui.ViewMap,
		'3': tui.ViewFeed,
		'4': tui.ViewSources,
		'5': tui.ViewTransit,
	}

	for key, want := range cases {
		m := newTestModel(t)
		press(m, key)
		if got := m.ViewID(); got != want {
			t.Errorf("%q selected %v, want %v", key, got, want)
		}
	}
}

func TestQuitKeys(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		send func(*tui.Model) tui.Command
	}{
		{name: "q", send: func(m *tui.Model) tui.Command { return press(m, 'q') }},
		{name: "ctrl-c", send: func(m *tui.Model) tui.Command { return hit(m, tui.KeyCtrlC) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m := newTestModel(t)
			if got := tc.send(m); got.Action != tui.ActionQuit {
				t.Errorf("%s returned %v, want ActionQuit", tc.name, got.Action)
			}
			if !m.Quitting() {
				t.Errorf("%s did not set the model to quit", tc.name)
			}
		})
	}
}

func TestHelpListsTheKeys(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	feed(m, testData())
	press(m, '?')

	out := frame(m, 100, 30)
	for _, want := range []string{"KEYS", "tab", "quit"} {
		if !strings.Contains(out, want) {
			t.Errorf("the help pane is missing %q:\n%s", want, out)
		}
	}

	press(m, '?')
	if strings.Contains(frame(m, 100, 30), "KEYS") {
		t.Error("? did not close the help pane")
	}
}

// The filter turns the keyboard into a text field. While it is open, q types a
// q; a cockpit that quits mid-search is a cockpit nobody searches in.
func TestFeedFilterCapturesTypingIncludingQ(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	feed(m, testData())
	press(m, '3')
	press(m, '/')

	for _, r := range "presupuesto q" {
		if got := press(m, r); got.Action == tui.ActionQuit {
			t.Fatalf("typing %q in the filter quit the cockpit", r)
		}
	}
	if got := m.Filter(); got != "presupuesto q" {
		t.Errorf("filter = %q, want %q", got, "presupuesto q")
	}

	hit(m, tui.KeyBackspace)
	hit(m, tui.KeyBackspace)
	if got := m.Filter(); got != "presupuesto" {
		t.Errorf("after two backspaces the filter is %q", got)
	}
}

func TestEscapeClearsTheFilter(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	feed(m, testData())
	press(m, '3')
	press(m, '/')
	press(m, 'x')
	hit(m, tui.KeyEscape)

	if m.Filter() != "" {
		t.Errorf("escape left the filter as %q", m.Filter())
	}
	if got := press(m, 'q'); got.Action != tui.ActionQuit {
		t.Error("q no longer quits after the filter was closed")
	}
}

func TestFeedFilterNarrowsWhatIsShown(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	feed(m, testData())
	press(m, '3')
	press(m, '/')
	for _, r := range "presupuesto" {
		press(m, r)
	}

	out := frame(m, 110, 30)
	if !strings.Contains(out, "presupuesto") {
		t.Errorf("the matching record is not shown:\n%s", out)
	}
	if strings.Contains(out, "RYR1234") {
		t.Errorf("a record that does not match the filter is still shown:\n%s", out)
	}
}

// Provenance is part of the data, not decoration, so the detail pane has to
// show all of it — including the two timestamps kept apart.
func TestFeedDetailShowsFullProvenance(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	feed(m, testData())
	press(m, '3')
	hit(m, tui.KeyEnter)

	out := frame(m, 110, 34)
	for _, want := range []string{
		"observed", "fetched", "licence", "publisher", "raw hash", "deadbeefcafe", "CC BY 4.0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the detail pane is missing %q:\n%s", want, out)
		}
	}

	hit(m, tui.KeyEscape)
	if strings.Contains(frame(m, 110, 34), "raw hash") {
		t.Error("escape did not close the detail pane")
	}
}

func TestFeedScrolls(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	d := testData()
	for i := range 40 {
		d.Records = append(d.Records, testRecord(
			"bulk-"+string(rune('a'+i%26))+string(rune('0'+i/26)),
			"bulk", "city", "bulk entry", 200+i, nil))
	}
	feed(m, d)
	press(m, '3')

	before := m.FeedSelection()
	hit(m, tui.KeyDown)
	hit(m, tui.KeyDown)
	if m.FeedSelection() != before+2 {
		t.Errorf("two downs moved the selection from %d to %d", before, m.FeedSelection())
	}

	hit(m, tui.KeyPageDown)
	if m.FeedSelection() <= before+2 {
		t.Error("page down did not move the selection")
	}

	hit(m, tui.KeyHome)
	if m.FeedSelection() != 0 {
		t.Errorf("home left the selection at %d", m.FeedSelection())
	}

	// The selection must never run off either end of the list.
	for range 5 {
		hit(m, tui.KeyUp)
	}
	if m.FeedSelection() != 0 {
		t.Errorf("up past the top left the selection at %d", m.FeedSelection())
	}

	hit(m, tui.KeyEnd)
	for range 5 {
		hit(m, tui.KeyDown)
	}
	if got := m.FeedSelection(); got != len(d.Records)-1 {
		t.Errorf("down past the bottom left the selection at %d of %d", got, len(d.Records))
	}
}

func TestMapArrowsPanAndPlusMinusZoom(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	feed(m, testData())
	press(m, '2')

	start := m.MapBox()
	hit(m, tui.KeyRight)
	east := m.MapBox()
	if east.West <= start.West || east.East <= start.East {
		t.Errorf("right did not pan east: %+v -> %+v", start, east)
	}

	hit(m, tui.KeyUp)
	north := m.MapBox()
	if north.North <= east.North {
		t.Errorf("up did not pan north: %+v -> %+v", east, north)
	}

	wide := north.East - north.West
	press(m, '+')
	if in := m.MapBox(); in.East-in.West >= wide {
		t.Errorf("+ did not zoom in: %f wide, was %f", in.East-in.West, wide)
	}

	press(m, '-')
	press(m, '-')
	if out := m.MapBox(); out.East-out.West <= wide {
		t.Errorf("- did not zoom out: %f wide, was %f", out.East-out.West, wide)
	}
}

// The map opens on Córdoba, whatever else is in the store.
//
// This is not a preference. Two of eye's sources are national: DGT publishes
// 1948 cameras from Galicia to Almería, and IGN publishes earthquakes across
// Iberia and the Atlantic. Fitting to all of that puts the viewport somewhere
// around 25°W to 12°E and renders Córdoba as three dots, under a panel titled
// CÓRDOBA. `f` fits to the data for anyone who wants the wider picture.
func TestTheMapOpensOnCordoba(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	press(m, '2')
	feed(m, testData())

	box := m.MapBox()
	if m.MapAutoFit() {
		t.Error("the map starts auto-fitted, so one national source moves it off the city")
	}
	if box.West < -6 || box.East > -3.5 || box.South < 37 || box.North > 38.6 {
		t.Errorf("opening viewport %v is not Córdoba", box)
	}
}

// Panning has to switch auto-fit off, or the next tick drags the view back and
// the map fights the person using it.
func TestPanningStopsAutoFitAndFRestoresIt(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	feed(m, testData())
	press(m, '2')

	press(m, 'f')
	if !m.MapAutoFit() {
		t.Fatal("f did not fit the map to the data")
	}

	hit(m, tui.KeyLeft)
	if m.MapAutoFit() {
		t.Error("panning left auto-fit on, so the next tick will undo the pan")
	}

	press(m, 'f')
	if !m.MapAutoFit() {
		t.Error("f did not restore auto-fit")
	}
}

func TestSourcesRPollsTheSelectedSource(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	feed(m, testData())
	press(m, '4')

	got := press(m, 'r')
	if got.Action != tui.ActionPollSource {
		t.Fatalf("r in the registry view returned %v, want ActionPollSource", got.Action)
	}
	if got.Target != "dgt-incidents" {
		t.Errorf("polled %q, want the selected source", got.Target)
	}

	hit(m, tui.KeyDown)
	if got := press(m, 'r'); got.Target != "saih-guadalquivir" {
		t.Errorf("after moving down, r polls %q", got.Target)
	}
}

// Elsewhere r is a re-read of the store, which touches no network at all.
func TestRRereadsTheStoreOutsideTheRegistryView(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	feed(m, testData())

	if got := press(m, 'r'); got.Action != tui.ActionRefresh {
		t.Errorf("r on the dashboard returned %v, want ActionRefresh", got.Action)
	}
}

func TestTransitSearchesStopsAndAsksForArrivals(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	feed(m, testData())
	press(m, '5')
	press(m, '/')
	for _, r := range "trassierra" {
		press(m, r)
	}

	out := frame(m, 110, 30)
	if !strings.Contains(out, "Trassierra") {
		t.Errorf("the matching stop is not listed:\n%s", out)
	}

	hit(m, tui.KeyEnter)
	got := hit(m, tui.KeyEnter)
	if got.Action != tui.ActionArrivals {
		t.Fatalf("enter on a selected stop returned %v, want ActionArrivals", got.Action)
	}
	if got.Target != "401" {
		t.Errorf("asked for arrivals at %q, want the number on the pole", got.Target)
	}
}

// A prediction read ten minutes ago is not live. Saying so is the difference
// between a board and a rumour.
func TestArrivalsOlderThanTheirCadenceAreMarkedStale(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	feed(m, testData())
	press(m, '5')

	m.Update(tui.Event{
		Kind: tui.EventArrivals,
		At:   testNow,
		Arrivals: []tui.Arrival{
			{StopID: "401", StopName: "Ctra. Trassierra", Line: "3", Route: "Villarrubia", Minutes: 4, ReadAt: testNow},
		},
	})

	fresh := frame(m, 110, 30)
	if !strings.Contains(fresh, "Villarrubia") {
		t.Fatalf("the arrival is not shown:\n%s", fresh)
	}
	if strings.Contains(fresh, "STALE") {
		t.Errorf("an estimate read this second is marked stale:\n%s", fresh)
	}

	// Move the clock on past the cadence without a new reading.
	m.Update(tui.Event{Kind: tui.EventTick, At: testNow.Add(10 * time.Minute)})
	aged := frame(m, 110, 30)
	if !strings.Contains(aged, "STALE") {
		t.Errorf("a ten-minute-old estimate is still presented as live:\n%s", aged)
	}
}

// The indicator blinks to say something moved. If it blinks anyway it is
// decoration, and the board stops meaning anything.
func TestTheIndicatorOnlyBlinksWhenSomethingChanged(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	feed(m, testData())
	if !m.Changed() {
		t.Error("the first store read did not register as a change")
	}

	// The same data again is not news.
	m.Update(tui.Event{Kind: tui.EventTick, At: testNow.Add(time.Hour)})
	d := testData()
	d.ReadAt = testNow.Add(time.Hour)
	m.Update(tui.Event{Kind: tui.EventData, Data: &d, At: testNow.Add(time.Hour)})
	if m.Changed() {
		t.Error("an unchanged store read still counts as a change")
	}

	// A new record is.
	d2 := testData()
	d2.ReadAt = testNow.Add(2 * time.Hour)
	d2.RecordCount++
	d2.Records = append(d2.Records, testRecord("r5", "boe", "civic", "Nueva resolución", 0, nil))
	m.Update(tui.Event{Kind: tui.EventData, Data: &d2, At: testNow.Add(2 * time.Hour)})
	if !m.Changed() {
		t.Error("a new record did not register as a change")
	}
}

func TestResizeIsRemembered(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	m.Update(tui.Event{Kind: tui.EventResize, Size: tui.Size{Cols: 132, Rows: 43}, At: testNow})

	if got := m.Size(); got.Cols != 132 || got.Rows != 43 {
		t.Errorf("Size() = %+v after a resize", got)
	}
}

func TestAStoreErrorIsShownRatherThanSwallowed(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	d := testData()
	d.Err = errRead
	feed(m, d)

	if out := frame(m, 100, 30); !strings.Contains(out, "could not read the store") {
		t.Errorf("a failed read is not reported:\n%s", out)
	}
}
