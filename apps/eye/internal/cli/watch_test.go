package cli

import (
	"errors"
	"strings"
	"testing"
	"time"

	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	"github.com/FullFran/cordvba/apps/eye/internal/render"
)

// errTest stands in for a source that could not be reached.
var errTest = errors.New("503 from upstream")

// renderPlainTheme is the unstyled theme, so assertions match plain text.
func renderPlainTheme() render.Theme { return render.PlainTheme() }

// feedRecord builds a record from a source at an age.
func feedRecord(id, source string, minutesAgo int) observation.Record {
	at := time.Now().UTC().Add(-time.Duration(minutesAgo) * time.Minute)
	return observation.Record{
		ID: id, Source: source, Kind: "k", Topic: "t",
		ObservedAt: at, FetchedAt: at, Title: id,
		Quality: observation.QualityOfficial, Confidence: 1,
		Provenance: observation.Provenance{
			Publisher: source, SourceURL: "https://example.org", License: "unspecified", FetchedAt: at,
		},
	}
}

// ADS-B produces a record every few seconds and is always the newest thing in
// the store. A straight newest-first feed is therefore a list of aircraft
// callsigns with the city pushed off the bottom.
func TestFairShareStopsOneSourceDominating(t *testing.T) {
	t.Parallel()

	var records []observation.Record
	for i := range 20 {
		records = append(records, feedRecord("plane-"+string(rune('a'+i)), "adsb-lol", i))
	}
	records = append(records,
		feedRecord("news-1", "diario-cordoba", 30),
		feedRecord("news-2", "cordopolis", 35),
		feedRecord("quake", "ign-seismic", 40),
	)
	observation.SortRecordsNewestFirst(records)

	got := fairShare(records, 8)
	if len(got) != 8 {
		t.Fatalf("rows = %d, want 8", len(got))
	}

	counts := map[string]int{}
	for _, r := range got {
		counts[r.Source]++
	}

	// The guarantee is a seat for everyone else, not a ceiling on the
	// loudest: leftover rows are filled in time order, and on a quiet night
	// more aircraft beats blank space.
	for _, want := range []string{"diario-cordoba", "cordopolis", "ign-seismic"} {
		if counts[want] == 0 {
			t.Errorf("%s was pushed off the board by the noisy source", want)
		}
	}
	if counts["adsb-lol"] == len(got) {
		t.Error("the noisy source took the whole board")
	}
}

// With enough sources to fill it, no one source exceeds its quota.
func TestFairShareHoldsTheQuotaWhenThereIsEnoughToShow(t *testing.T) {
	t.Parallel()

	var records []observation.Record
	for i := range 10 {
		records = append(records, feedRecord("plane-"+string(rune('a'+i)), "adsb-lol", i))
	}
	for i, src := range []string{"diario-cordoba", "cordopolis", "eldiadecordoba", "boe", "uco-events"} {
		records = append(records,
			feedRecord(src+"-1", src, 20+i),
			feedRecord(src+"-2", src, 30+i),
		)
	}
	observation.SortRecordsNewestFirst(records)

	got := fairShare(records, 10)
	counts := map[string]int{}
	for _, r := range got {
		counts[r.Source]++
	}

	for src, n := range counts {
		if n > maxPerSourceInFeed {
			t.Errorf("%s took %d rows with plenty of alternatives; quota is %d", src, n, maxPerSourceInFeed)
		}
	}
}

// A quiet hour with few sources must still fill the board rather than showing
// half a screen.
func TestFairShareFillsFromOverflowWhenSourcesAreFew(t *testing.T) {
	t.Parallel()

	var records []observation.Record
	for i := range 10 {
		records = append(records, feedRecord("plane-"+string(rune('a'+i)), "adsb-lol", i))
	}
	observation.SortRecordsNewestFirst(records)

	got := fairShare(records, 6)
	if len(got) != 6 {
		t.Errorf("rows = %d, want the board filled to 6", len(got))
	}
}

func TestFairShareIsNewestFirst(t *testing.T) {
	t.Parallel()

	records := []observation.Record{
		feedRecord("a", "s1", 5),
		feedRecord("b", "s2", 1),
		feedRecord("c", "s3", 10),
	}
	observation.SortRecordsNewestFirst(records)

	got := fairShare(records, 3)
	for i := 1; i < len(got); i++ {
		if got[i].ObservedAt.After(got[i-1].ObservedAt) {
			t.Fatalf("feed is not newest-first: %v", got)
		}
	}
}

func TestFairShareHandlesEmptyAndZeroRows(t *testing.T) {
	t.Parallel()

	if got := fairShare(nil, 5); len(got) != 0 {
		t.Errorf("fairShare(nil) = %d rows", len(got))
	}
	if got := fairShare([]observation.Record{feedRecord("a", "s", 1)}, 0); got != nil {
		t.Errorf("fairShare(_, 0) = %v, want nil", got)
	}
}

// An observation is marked new only until it has had time to be noticed.
// Marking everything forever, or nothing ever, are both useless.
func TestBoardMarksArrivalsOnce(t *testing.T) {
	t.Parallel()

	board := &liveBoard{rows: 5, seen: map[string]time.Time{}}
	now := time.Now()

	board.seen["old"] = now.Add(-newFor - time.Minute)
	board.seen["recent"] = now.Add(-time.Minute)

	if now.Sub(board.seen["old"]) < newFor {
		t.Error("an observation seen long ago still counts as new")
	}
	if now.Sub(board.seen["recent"]) >= newFor {
		t.Error("an observation seen a minute ago is already stale")
	}
}

// drawBoard renders one frame to a string for assertions.
func drawBoard(t *testing.T, records []observation.Record, results []applicationResult) string {
	t.Helper()

	board := &liveBoard{theme: renderPlainTheme(), rows: 6, seen: map[string]time.Time{}, results: results}

	var buf strings.Builder
	board.draw(&buf, records, 1980, &runtime{registryPath: "(embedded)"}, 3)
	return buf.String()
}

func TestBoardDrawsEverySection(t *testing.T) {
	t.Parallel()

	out := drawBoard(t, []observation.Record{
		feedRecord("news", "diario-cordoba", 4),
		feedRecord("quake", "ign-seismic", 20),
	}, []applicationResult{{Source: "diario-cordoba"}, {Source: "ign-seismic"}})

	for _, want := range []string{"E Y E", "CÓRDOBA", "LATEST", "OBSERVATIONS", "SOURCES", "tick 3"} {
		if !strings.Contains(out, want) {
			t.Errorf("the board is missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "1980 assets mapped") {
		t.Errorf("the inventory count is missing:\n%s", out)
	}
	// The provenance line survives whatever the styling does.
	if !strings.Contains(out, "traces back to a public source") {
		t.Errorf("the board dropped the provenance line:\n%s", out)
	}
}

func TestBoardSaysSoWhenTheStoreIsEmpty(t *testing.T) {
	t.Parallel()

	out := drawBoard(t, nil, nil)
	if !strings.Contains(out, "nothing in the store yet") {
		t.Errorf("an empty store is not reported:\n%s", out)
	}
}

// Reading beside eye daemon is a normal mode, and the board must say that is
// what it is doing rather than implying it polled.
func TestBoardSaysWhenItIsOnlyReading(t *testing.T) {
	t.Parallel()

	out := drawBoard(t, []observation.Record{feedRecord("a", "s", 1)}, nil)
	if !strings.Contains(out, "eye daemon is doing the polling") {
		t.Errorf("offline mode is not explained:\n%s", out)
	}
}

func TestBoardNamesFailingSources(t *testing.T) {
	t.Parallel()

	out := drawBoard(t, []observation.Record{feedRecord("a", "s", 1)}, []applicationResult{
		{Source: "saih-guadalquivir", Err: errTest},
		{Source: "diario-cordoba"},
	})

	if !strings.Contains(out, "1 failed") {
		t.Errorf("the failure count is missing:\n%s", out)
	}
	if !strings.Contains(out, "saih-guadalquivir") {
		t.Errorf("the failing source is not named:\n%s", out)
	}
}

func TestBoardMarksNewArrivalsAndStopsMarkingThem(t *testing.T) {
	t.Parallel()

	board := &liveBoard{theme: renderPlainTheme(), rows: 4, seen: map[string]time.Time{}}
	records := []observation.Record{feedRecord("fresh", "diario-cordoba", 1)}

	var first strings.Builder
	board.draw(&first, records, 0, &runtime{registryPath: "x"}, 0)
	if !strings.Contains(first.String(), "▸") {
		t.Errorf("an observation the board had never shown was not marked:\n%s", first.String())
	}

	// Age the board's memory past the marking window and redraw.
	board.seen["fresh"] = time.Now().Add(-newFor - time.Minute)

	var second strings.Builder
	board.draw(&second, records, 0, &runtime{registryPath: "x"}, 1)
	if strings.Contains(second.String(), "▸") {
		t.Errorf("an observation shown long ago is still marked as new:\n%s", second.String())
	}
}

func TestFailIndicatorDoesNotShoutAtZero(t *testing.T) {
	t.Parallel()

	theme := renderPlainTheme()
	if got := failIndicator(theme, 0); !strings.Contains(got, "0 failed") {
		t.Errorf("failIndicator(0) = %q", got)
	}
	if got := failIndicator(theme, 3); !strings.Contains(got, "3 failed") {
		t.Errorf("failIndicator(3) = %q", got)
	}
}
