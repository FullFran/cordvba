package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/FullFran/cordvba/apps/eye/internal/logging"
	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	"github.com/FullFran/cordvba/apps/eye/internal/render"
)

// Defaults for the live board.
const (
	defaultWatchInterval = 60 * time.Second
	defaultWatchRows     = 12
	// newFor is how long an observation is marked as new after eye first
	// sees it. Long enough to notice on the next tick, short enough that
	// the board is not permanently green.
	newFor = 5 * time.Minute
)

// watchCommand keeps a live board of the city on screen.
//
// This is the mode the whole project is for. Everything else answers a question
// you thought to ask; this one tells you what happened while you were not
// asking.
func watchCommand() Command {
	return Command{
		Name:    "watch",
		Summary: "Keep a live board of Cordoba on screen",
		Run: func(ctx context.Context, args []string, stdout, stderr io.Writer) error {
			fs := flag.NewFlagSet("watch", flag.ContinueOnError)
			fs.SetOutput(stderr)
			var (
				registryP = fs.String("registry", "", "path to an alternative sources.yaml")
				dataDir   = fs.String("data-dir", "", "override where the store and raw cache live")
				every     = fs.Duration("every", defaultWatchInterval, "how often to poll")
				rows      = fs.Int("rows", defaultWatchRows, "how many recent observations to show")
				offline   = fs.Bool("offline", false, "read the store without polling — for use beside eye daemon")
				once      = fs.Bool("once", false, "draw a single frame and exit")
			)
			if err := fs.Parse(args); err != nil {
				return err
			}

			rt, err := newRuntime(runtimeOptions{registry: *registryP, dataDir: *dataDir})
			if err != nil {
				return err
			}
			defer func() { _ = rt.Close() }()

			ps, _ := rt.pollable()
			if len(ps) == 0 && !*offline {
				return errors.New("no live source in the registry")
			}

			screen := render.NewScreen(stdout)
			screen.Enter()
			defer screen.Leave()

			board := &liveBoard{
				theme: render.NewTheme(stdout),
				rows:  *rows,
				seen:  map[string]time.Time{},
			}

			for tick := 0; ; tick++ {
				if !*offline {
					// Logs would tear the board apart, so the poll is
					// silent here; eye daemon is where you watch it work.
					_ = logging.Discard()
					board.results = rt.collect(ctx, ps)
				}
				if _, err := rt.prune(ctx); err != nil {
					return err
				}

				records, err := rt.query(ctx, observation.Filter{Limit: 400})
				if err != nil {
					return err
				}
				_, entities, err := rt.store.Counts(ctx)
				if err != nil {
					return err
				}

				screen.Frame(func(w io.Writer) {
					board.draw(w, records, entities, rt, tick)
				})
				screen.Separator()

				if *once {
					return nil
				}
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(*every):
				}
			}
		},
	}
}

// liveBoard renders the repeating frame and remembers what it has shown, so
// arrivals can be marked.
type liveBoard struct {
	theme   render.Theme
	rows    int
	results []observationResult

	// seen maps a record id to when this board first displayed it. Without
	// it every observation would look new on the first tick and never
	// again, which is the opposite of useful.
	seen map[string]time.Time
}

// observationResult aliases the collector result so the board does not depend
// on the application package's name in its signature.
type observationResult = applicationResult

// draw renders one frame.
func (b *liveBoard) draw(w io.Writer, records []observation.Record, entities int, rt *runtime, tick int) {
	now := time.Now()
	t := b.theme

	t.Banner(w, fmt.Sprintf("CÓRDOBA · %s · tick %d", now.Format("02 Jan · 15:04:05"), tick))

	// Changes are observations too, so they come back from the same query.
	// Showing them in the feed alongside the thing they describe would print
	// every headline twice, so they get their own board.
	observations, changes := splitChanges(records)

	b.drawChanges(w, changes, now)
	_, _ = fmt.Fprintln(w)
	b.drawFeed(w, observations, now)
	_, _ = fmt.Fprintln(w)
	b.drawTopics(w, observations, entities, now)
	_, _ = fmt.Fprintln(w)
	b.drawSources(w, rt)

	_, _ = fmt.Fprintln(w, "\n"+t.Rule(boardWidth))
	_, _ = fmt.Fprintln(w, t.Dim("ctrl-c to stop · every figure traces back to a public source"))
}

// drawFeed lists the newest observations, marking the ones that just arrived.
func (b *liveBoard) drawFeed(w io.Writer, records []observation.Record, now time.Time) {
	t := b.theme
	_, _ = fmt.Fprintln(w, t.Section("latest", boardWidth))

	if len(records) == 0 {
		_, _ = fmt.Fprintln(w, t.Alert("  nothing in the store yet"))
		return
	}

	for _, r := range fairShare(records, b.rows) {
		firstSeen, known := b.seen[r.ID]
		if !known {
			firstSeen = now
			b.seen[r.ID] = firstSeen
		}

		marker := t.Dim(" ")
		if now.Sub(firstSeen) < newFor {
			marker = t.Live("▸")
		}

		age := ageOf(r.ObservedAt, now.UTC())
		_, _ = fmt.Fprintf(w, " %s %s %s %s\n",
			marker,
			t.Dim(t.Pad(age, 5)),
			t.Label(t.Pad(ellipsis(r.Provenance.Publisher, 18), 18)),
			ellipsis(r.Title, boardWidth-30))
	}
}

// changeRows is how much of the board the change panel may take. It is small
// on purpose: this is the "what moved" line, not the timeline.
const changeRows = 4

// splitChanges separates derived change records from the observations they were
// derived from.
func splitChanges(records []observation.Record) (observations, changes []observation.Record) {
	for _, r := range records {
		if r.Kind == observation.ChangeKindRecord {
			changes = append(changes, r)
			continue
		}
		observations = append(observations, r)
	}
	return observations, changes
}

// drawChanges renders what moved since eye last looked.
//
// This is the panel that makes the board worth watching rather than reading:
// everything else says what is there, and this says what is different.
func (b *liveBoard) drawChanges(w io.Writer, changes []observation.Record, now time.Time) {
	t := b.theme
	_, _ = fmt.Fprintln(w, t.Section("what moved", boardWidth))

	if len(changes) == 0 {
		_, _ = fmt.Fprintln(w, t.Dim("  nothing has moved"))
		return
	}

	observation.SortRecordsNewestFirst(changes)
	shown := observation.ApplyLimit(changes, changeRows)

	for _, r := range shown {
		var c observation.Change
		if err := json.Unmarshal(r.Payload, &c); err != nil {
			continue
		}

		mark, label := t.Live("+"), "new"
		switch c.Kind {
		case observation.ChangeUpdated:
			mark, label = t.Warn("~"), "changed"
		case observation.ChangeDisappeared:
			mark, label = t.Alert("-"), "gone"
		}

		_, _ = fmt.Fprintf(w, " %s %s %s %s\n",
			mark,
			t.Dim(t.Pad(ageOf(r.ObservedAt, now.UTC()), 5)),
			t.Label(t.Pad(label, 8)),
			ellipsis(r.Title, boardWidth-24))

		// One field, the first, so the panel stays a summary. `eye changes`
		// is where the whole diff lives.
		if len(c.Fields) > 0 {
			f := c.Fields[0]
			_, _ = fmt.Fprintf(w, "   %s\n",
				t.Dim(fmt.Sprintf("%s %s → %s", f.Field,
					ellipsis(orDash(f.From), 24), ellipsis(orDash(f.To), 24))))
		}
	}

	if len(changes) > len(shown) {
		_, _ = fmt.Fprintf(w, "   %s\n", t.Dim(fmt.Sprintf("… %d more · eye changes", len(changes)-len(shown))))
	}
}

// maxPerSourceInFeed is each source's guaranteed share of the feed.
//
// It is a floor for everyone else, not a ceiling for the loudest. ADS-B
// produces a record every few seconds and is always the newest thing in the
// store, so a straight newest-first feed is a list of aircraft callsigns with
// the city pushed off the bottom.
//
// The quota runs first, so every source that has anything to say gets a seat.
// Whatever rows are left over are then filled in time order, which may well be
// more aircraft — because on a quiet night more aircraft is a better answer
// than blank space.
const maxPerSourceInFeed = 2

// fairShare returns the newest observations with no single source allowed to
// dominate, falling back to filling the remaining rows in time order.
func fairShare(records []observation.Record, rows int) []observation.Record {
	if rows <= 0 {
		return nil
	}

	perSource := map[string]int{}
	out := make([]observation.Record, 0, rows)
	var overflow []observation.Record

	for _, r := range records {
		if len(out) == rows {
			break
		}
		if perSource[r.Source] >= maxPerSourceInFeed {
			overflow = append(overflow, r)
			continue
		}
		perSource[r.Source]++
		out = append(out, r)
	}

	// If the quota left the board short — few sources, quiet hour — fill
	// the rest rather than showing half a screen.
	for _, r := range overflow {
		if len(out) == rows {
			break
		}
		out = append(out, r)
	}

	observation.SortRecordsNewestFirst(out)
	return out
}

// drawTopics summarises what the store holds, by topic.
func (b *liveBoard) drawTopics(w io.Writer, records []observation.Record, entities int, now time.Time) {
	t := b.theme
	_, _ = fmt.Fprintln(w, t.Section("observations", boardWidth))

	counts := map[string]int{}
	newest := map[string]time.Time{}
	for _, r := range records {
		counts[r.Topic]++
		if r.ObservedAt.After(newest[r.Topic]) {
			newest[r.Topic] = r.ObservedAt
		}
	}

	topics := make([]string, 0, len(counts))
	for topic := range counts {
		topics = append(topics, topic)
	}
	sort.Strings(topics)

	var line strings.Builder
	for i, topic := range topics {
		if i > 0 {
			line.WriteString(t.Chrome(" · "))
		}
		age := ageOf(newest[topic], now.UTC())
		shown := t.Live(age)
		if now.UTC().Sub(newest[topic]) > 24*time.Hour {
			shown = t.Warn(age)
		}
		fmt.Fprintf(&line, "%s %s", t.Label(topic), shown)
	}
	_, _ = fmt.Fprintf(w, "  %s\n", line.String())
	_, _ = fmt.Fprintf(w, "  %s\n", t.Dim(fmt.Sprintf("%d observations · %d assets mapped", len(records), entities)))
}

// drawSources shows which feeds answered on this tick.
func (b *liveBoard) drawSources(w io.Writer, rt *runtime) {
	t := b.theme
	_, _ = fmt.Fprintln(w, t.Section("sources", boardWidth))

	if len(b.results) == 0 {
		_, _ = fmt.Fprintf(w, "  %s\n", t.Dim("reading the store; eye daemon is doing the polling"))
		return
	}

	var answered, failed int
	var broken []string
	for _, r := range b.results {
		if r.OK() {
			answered++
			continue
		}
		failed++
		broken = append(broken, r.Source)
	}

	var held int
	for _, s := range rt.sources {
		if !s.Automation.Pollable() {
			held++
		}
	}

	failMarker := t.Indicator("failing")
	if failed == 0 {
		// A red cross next to "0 failed" reads as an alarm. It is not one.
		failMarker = t.Dim("·")
	}

	_, _ = fmt.Fprintf(w, "  %s %s   %s %s   %s %s\n",
		t.Indicator("live"), t.Live(fmt.Sprintf("%d answered", answered)),
		failMarker, failIndicator(t, failed),
		t.Indicator("held"), t.Warn(fmt.Sprintf("%d held", held)))

	if len(broken) > 0 {
		_, _ = fmt.Fprintf(w, "  %s\n", t.Alert("down: "+ellipsis(strings.Join(broken, ", "), boardWidth-8)))
	}
}

// failIndicator keeps a zero failure count from shouting in red.
func failIndicator(t render.Theme, failed int) string {
	text := fmt.Sprintf("%d failed", failed)
	if failed == 0 {
		return t.Dim(text)
	}
	return t.Alert(text)
}
