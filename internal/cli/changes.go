package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	application "github.com/FullFran/eye/internal/observation/application"
	observation "github.com/FullFran/eye/internal/observation/domain"
)

// changesCommand reports what moved since eye last looked.
//
// This is the difference between a reader and a watcher. Everything else in eye
// answers "what is there"; this one answers "what is different", which is the
// question you actually have when you already looked an hour ago.
func changesCommand() Command {
	return Command{
		Name:    "changes",
		Summary: "What appeared, moved or stopped being published since eye last looked",
		Run: func(ctx context.Context, args []string, stdout, stderr io.Writer) error {
			fs := flag.NewFlagSet("changes", flag.ContinueOnError)
			fs.SetOutput(stderr)

			opts := &observeOptions{}
			opts.bind(fs, 40, 24*time.Hour)
			var (
				topics  = fs.String("topic", "", "only these topics, comma separated")
				onlyNew = fs.Bool("new", false, "only things that appeared")
				onlyUpd = fs.Bool("changed", false, "only things that moved")
				onlyOld = fs.Bool("gone", false, "only things that stopped being published")
			)
			if err := fs.Parse(args); err != nil {
				return err
			}

			rt, err := newRuntime(runtimeOptions{registry: opts.registry, dataDir: opts.dataDir})
			if err != nil {
				return err
			}
			defer func() { _ = rt.Close() }()

			selected := splitCSV(*topics)

			// Asked BEFORE polling: the poll below stores a baseline, and
			// after that every source reads as known. The question is
			// whether eye had one when the user asked.
			everLooked := rt.everLooked(ctx)

			var results []application.Result
			if !opts.offline {
				// Polling is what produces changes: each poll is compared
				// against the previous one. Reading without polling shows
				// what earlier runs already found.
				ps, _ := rt.pollable(selected...)
				results = rt.collect(ctx, ps)
				reportFailures(stderr, results)
			}

			now := time.Now()
			filter := opts.filter(selected, now)
			filter.Kinds = []string{observation.ChangeKindRecord}

			records, err := rt.query(ctx, filter)
			if err != nil {
				return err
			}

			changes := decodeChanges(records)
			changes = keepKinds(changes, wantedKinds(*onlyNew, *onlyUpd, *onlyOld))

			if opts.asJSON {
				return writeJSON(stdout, changes)
			}
			return renderChanges(stdout, changes, everLooked, opts.since, now)
		},
	}
}

// changeView pairs a change with the observation it was recorded on.
type changeView struct {
	Kind      string                    `json:"kind"`
	Title     string                    `json:"title"`
	Topic     string                    `json:"topic"`
	Source    string                    `json:"source"`
	Publisher string                    `json:"publisher"`
	NoticedAt time.Time                 `json:"noticed_at"`
	RecordID  string                    `json:"record_id"`
	Identity  string                    `json:"identity"`
	Fields    []observation.FieldChange `json:"fields,omitempty"`
}

// decodeChanges reads the change payloads back out of the records.
func decodeChanges(records []observation.Record) []changeView {
	out := make([]changeView, 0, len(records))
	for _, r := range records {
		var c observation.Change
		if err := json.Unmarshal(r.Payload, &c); err != nil {
			// A change eye can no longer read is not a change eye can
			// report. Skipping beats printing a blank line.
			continue
		}
		out = append(out, changeView{
			Kind: c.Kind, Title: r.Title, Topic: r.Topic,
			Source: r.Source, Publisher: r.Provenance.Publisher,
			NoticedAt: r.ObservedAt, RecordID: c.RecordID,
			Identity: c.Identity, Fields: c.Fields,
		})
	}
	return out
}

// wantedKinds turns the three filter flags into the set to keep. None of them
// set means all of them.
func wantedKinds(new, changed, gone bool) map[string]bool {
	want := map[string]bool{}
	if new {
		want[observation.ChangeAppeared] = true
	}
	if changed {
		want[observation.ChangeUpdated] = true
	}
	if gone {
		want[observation.ChangeDisappeared] = true
	}
	return want
}

// keepKinds narrows the list, treating an empty set as no restriction.
func keepKinds(changes []changeView, want map[string]bool) []changeView {
	if len(want) == 0 {
		return changes
	}

	out := make([]changeView, 0, len(changes))
	for _, c := range changes {
		if want[c.Kind] {
			out = append(out, c)
		}
	}
	return out
}

// everLooked reports whether eye holds a baseline for any source at all.
//
// It exists so an empty result can say which empty it is. "Nothing changed" and
// "this is the first time eye has looked, so there was nothing to compare
// against" are very different answers, and a tool that prints the first when it
// means the second is lying quietly.
func (r *runtime) everLooked(ctx context.Context) bool {
	for _, s := range r.sources {
		if _, known, err := r.store.Snapshot(ctx, s.ID); err == nil && known {
			return true
		}
	}
	return false
}

// titleWidth and valueWidth keep the table inside an ordinary terminal. A
// change nobody can read on screen is a change nobody reads.
const (
	titleWidth = 46
	valueWidth = 44
)

// changeLabel is how each kind reads in the table.
var changeLabel = map[string]string{
	observation.ChangeAppeared:    "NEW",
	observation.ChangeUpdated:     "CHANGED",
	observation.ChangeDisappeared: "GONE",
}

// changeOrder groups the output: what arrived, what moved, what stopped.
var changeOrder = []string{
	observation.ChangeAppeared,
	observation.ChangeUpdated,
	observation.ChangeDisappeared,
}

// renderChanges prints the changes grouped by what kind of change they are.
func renderChanges(w io.Writer, changes []changeView, everLooked bool, window time.Duration, now time.Time) error {
	if len(changes) == 0 {
		if !everLooked {
			_, _ = fmt.Fprintln(w, "Nothing to compare against yet: this is the first time eye has looked.")
			_, _ = fmt.Fprintln(w, "That baseline is now stored. Run this again after the next poll.")
			return nil
		}
		_, _ = fmt.Fprintf(w, "Nothing changed in the last %s.\n", shortDuration(window))
		return nil
	}

	byKind := map[string][]changeView{}
	for _, c := range changes {
		byKind[c.Kind] = append(byKind[c.Kind], c)
	}

	for _, kind := range changeOrder {
		group := byKind[kind]
		if len(group) == 0 {
			continue
		}
		sort.SliceStable(group, func(i, j int) bool {
			return group[i].NoticedAt.After(group[j].NoticedAt)
		})

		_, _ = fmt.Fprintf(w, "\n%s\n", changeLabel[kind])
		for _, c := range group {
			// Fixed widths rather than a tabwriter: a field diff can be
			// long, and letting it set a shared column width pushes the
			// whole table off the screen.
			_, _ = fmt.Fprintf(w, "  %-12s  %-9s  %-*s  %s\n",
				c.NoticedAt.Local().Format("02 Jan 15:04"),
				ellipsis(c.Topic, 9),
				titleWidth, ellipsis(c.Title, titleWidth),
				ellipsis(c.Publisher, 24))

			for _, f := range c.Fields {
				_, _ = fmt.Fprintf(w, "  %-12s  %-9s    %s: %s → %s\n",
					"", "", f.Field,
					ellipsis(orDash(f.From), valueWidth),
					ellipsis(orDash(f.To), valueWidth))
			}
		}
	}

	_, _ = fmt.Fprintf(w, "\n%s · %s\n", plural(len(changes), "change", "changes"), summaryOfChanges(changes))
	// The distinction that makes this trustworthy, stated where it is read.
	_, _ = fmt.Fprintf(w, "over the last %s · GONE means a WORKING source stopped publishing it;\n",
		shortDuration(window))
	_, _ = fmt.Fprintln(w, "a source that failed or went stale produces silence here, never an ending.")
	return nil
}

// summaryOfChanges counts each kind, for the boards that show a total.
func summaryOfChanges(changes []changeView) string {
	counts := map[string]int{}
	for _, c := range changes {
		counts[c.Kind]++
	}

	var parts []string
	for _, kind := range changeOrder {
		if counts[kind] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[kind], strings.ToLower(changeLabel[kind])))
		}
	}
	if len(parts) == 0 {
		return "no changes"
	}
	return strings.Join(parts, " · ")
}

// shortDuration renders a window the way somebody would say it out loud.
func shortDuration(d time.Duration) string {
	switch {
	case d <= 0:
		return "all of recorded time"
	case d >= 24*time.Hour && d%(24*time.Hour) == 0:
		return plural(int(d/(24*time.Hour)), "day", "days")
	case d >= time.Hour && d%time.Hour == 0:
		return plural(int(d/time.Hour), "hour", "hours")
	default:
		return d.String()
	}
}
