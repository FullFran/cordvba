package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
	"time"

	application "github.com/FullFran/eye/internal/observation/application"
	observation "github.com/FullFran/eye/internal/observation/domain"
	source "github.com/FullFran/eye/internal/source/domain"
)

// statusCommand polls every live source and reports the state of the city and
// of the sources themselves.
//
// The source health block is not decoration. A number is only worth reading if
// you can see how fresh it is and how many feeds actually answered.
func statusCommand() Command {
	return Command{
		Name:    "status",
		Summary: "Poll every live source and report the state of Cordoba",
		Run: func(ctx context.Context, args []string, stdout, stderr io.Writer) error {
			fs := flag.NewFlagSet("status", flag.ContinueOnError)
			fs.SetOutput(stderr)
			var (
				asJSON    = fs.Bool("json", false, "output as JSON")
				registryP = fs.String("registry", "", "path to an alternative sources.yaml")
			)
			if err := fs.Parse(args); err != nil {
				return err
			}

			rt, err := newRuntime(*registryP)
			if err != nil {
				return err
			}

			ps, noAdapter := rt.pollable()
			started := time.Now()
			results := rt.collect(ctx, ps)
			elapsed := time.Since(started)

			records, err := rt.query(ctx, observation.Filter{})
			if err != nil {
				return err
			}
			_, entityCount := rt.store.Len()

			if *asJSON {
				return writeJSON(stdout, statusView(rt, results, noAdapter, records, entityCount, elapsed))
			}
			return renderStatus(stdout, rt, results, noAdapter, records, entityCount, elapsed)
		},
	}
}

// statusReport is the JSON shape of the status command.
type statusReport struct {
	GeneratedAt time.Time            `json:"generated_at"`
	Registry    string               `json:"registry"`
	ElapsedMS   int64                `json:"elapsed_ms"`
	Sources     statusSourceCounts   `json:"sources"`
	Records     int                  `json:"records"`
	Entities    int                  `json:"entities"`
	ByTopic     map[string]int       `json:"records_by_topic"`
	Results     []statusSourceResult `json:"results"`
	Held        []string             `json:"held"`
	NoAdapter   []string             `json:"awaiting_adapter"`
}

// statusSourceCounts summarises the registry.
type statusSourceCounts struct {
	Total    int `json:"total"`
	Live     int `json:"live"`
	Answered int `json:"answered"`
	Failed   int `json:"failed"`
	Held     int `json:"held"`
}

// statusSourceResult is one source's outcome.
type statusSourceResult struct {
	Source  string `json:"source"`
	Topic   string `json:"topic"`
	OK      bool   `json:"ok"`
	Records int    `json:"records"`
	Error   string `json:"error,omitempty"`
}

// statusView builds the machine-readable report.
func statusView(rt *runtime, results []application.Result, noAdapter []source.Source, records []observation.Record, entities int, elapsed time.Duration) statusReport {
	rep := statusReport{
		GeneratedAt: time.Now().UTC(),
		Registry:    rt.registryPath,
		ElapsedMS:   elapsed.Milliseconds(),
		Records:     len(records),
		Entities:    entities,
		ByTopic:     map[string]int{},
	}

	for _, r := range records {
		rep.ByTopic[r.Topic]++
	}

	for _, res := range results {
		item := statusSourceResult{Source: res.Source, Topic: res.Topic, OK: res.OK(), Records: res.Health.Records}
		if res.Err != nil {
			item.Error = res.Err.Error()
			rep.Sources.Failed++
		} else {
			rep.Sources.Answered++
		}
		rep.Results = append(rep.Results, item)
	}

	for _, s := range rt.sources {
		if !s.Automation.Pollable() {
			rep.Sources.Held++
			rep.Held = append(rep.Held, s.ID)
		}
	}
	for _, s := range noAdapter {
		rep.NoAdapter = append(rep.NoAdapter, s.ID)
	}

	rep.Sources.Total = len(rt.sources)
	rep.Sources.Live = len(results)
	return rep
}

// renderStatus writes the human-readable city summary.
func renderStatus(w io.Writer, rt *runtime, results []application.Result, noAdapter []source.Source, records []observation.Record, entities int, elapsed time.Duration) error {
	now := time.Now()
	_, _ = fmt.Fprintf(w, "CÓRDOBA · %s\n\n", now.Format("02 Jan 2006 15:04"))

	if err := renderTopics(w, records, entities, now); err != nil {
		return err
	}
	renderSourceHealth(w, rt, results, noAdapter)

	_, _ = fmt.Fprintf(w, "\nPolled in %s · registry: %s\n", elapsed.Round(time.Millisecond), rt.registryPath)
	_, _ = fmt.Fprintln(w, "Every figure above traces back to a public source. Run with --json for the evidence.")
	return nil
}

// renderTopics writes one line per topic, with how fresh its newest
// observation is. A count without an age is not worth reading.
func renderTopics(w io.Writer, records []observation.Record, entities int, now time.Time) error {
	byTopic := map[string]int{}
	newest := map[string]time.Time{}
	for _, r := range records {
		byTopic[r.Topic]++
		if r.ObservedAt.After(newest[r.Topic]) {
			newest[r.Topic] = r.ObservedAt
		}
	}

	if len(byTopic) == 0 {
		_, _ = fmt.Fprintln(w, "No source answered. Nothing below is a claim about the city.")
		return nil
	}

	topics := make([]string, 0, len(byTopic))
	for topic := range byTopic {
		topics = append(topics, topic)
	}
	sort.Strings(topics)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, topic := range topics {
		_, _ = fmt.Fprintf(tw, "  %s\t%s\tnewest %s\n",
			topic, plural(byTopic[topic], "observation", "observations"),
			ageOf(newest[topic], now.UTC()))
	}
	if entities > 0 {
		_, _ = fmt.Fprintf(tw, "  %s\t%s\t\n", "inventory", plural(entities, "asset mapped", "assets mapped"))
	}
	return tw.Flush()
}

// renderSourceHealth reports how many feeds answered, how many are held, and
// how many this build cannot yet read. All three are different problems.
func renderSourceHealth(w io.Writer, rt *runtime, results []application.Result, noAdapter []source.Source) {
	var answered, failed int
	for _, r := range results {
		if r.OK() {
			answered++
		} else {
			failed++
		}
	}

	var held int
	for _, s := range rt.sources {
		if !s.Automation.Pollable() {
			held++
		}
	}

	_, _ = fmt.Fprintf(w, "\nSources\n")
	_, _ = fmt.Fprintf(w, "  %d answered\n", answered)
	if failed > 0 {
		_, _ = fmt.Fprintf(w, "  %d failed\n", failed)
	}
	if held > 0 {
		_, _ = fmt.Fprintf(w, "  %d held: reuse terms unresolved or no documented interface\n", held)
	}
	if len(noAdapter) > 0 {
		_, _ = fmt.Fprintf(w, "  %d awaiting an adapter in this build\n", len(noAdapter))
	}

	for _, r := range results {
		if !r.OK() {
			_, _ = fmt.Fprintf(w, "\n  %s: %v\n", r.Source, r.Err)
		}
	}
}
