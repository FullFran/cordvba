package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"sort"
	"time"

	application "github.com/FullFran/eye/internal/observation/application"
	observation "github.com/FullFran/eye/internal/observation/domain"
	"github.com/FullFran/eye/internal/render"
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
				dataDir   = fs.String("data-dir", "", "override where the store and raw cache live")
			)
			if err := fs.Parse(args); err != nil {
				return err
			}

			rt, err := newRuntime(runtimeOptions{registry: *registryP, dataDir: *dataDir})
			if err != nil {
				return err
			}
			defer func() { _ = rt.Close() }()

			ps, noAdapter := rt.pollable()
			started := time.Now()
			results := rt.collect(ctx, ps)
			elapsed := time.Since(started)

			if _, err := rt.prune(ctx); err != nil {
				return err
			}

			records, err := rt.query(ctx, observation.Filter{})
			if err != nil {
				return err
			}
			_, entityCount, err := rt.store.Counts(ctx)
			if err != nil {
				return err
			}

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

// renderStatus writes the situation board.
func renderStatus(w io.Writer, rt *runtime, results []application.Result, noAdapter []source.Source, records []observation.Record, entities int, elapsed time.Duration) error {
	theme := render.NewTheme(w)
	now := time.Now()

	theme.Banner(w, fmt.Sprintf("CÓRDOBA · %s", now.Format("02 Jan 2006 · 15:04:05")))

	_, _ = fmt.Fprintln(w, theme.Section("observations", boardWidth))
	if err := renderTopics(w, theme, records, entities, now); err != nil {
		return err
	}

	_, _ = fmt.Fprintln(w, "\n"+theme.Section("sources", boardWidth))
	renderSourceHealth(w, theme, rt, results, noAdapter)

	_, _ = fmt.Fprintln(w, "\n"+theme.Rule(boardWidth))
	_, _ = fmt.Fprintf(w, "%s  polled in %s · registry %s\n",
		theme.Indicator("live"), elapsed.Round(time.Millisecond), rt.registryPath)
	_, _ = fmt.Fprintln(w, theme.Dim("every figure above traces back to a public source · --json for the evidence"))
	return nil
}

// boardWidth is the situation board's column width.
const boardWidth = 66

// renderTopics writes one line per topic, with how fresh its newest
// observation is. A count without an age is not worth reading.
func renderTopics(w io.Writer, theme render.Theme, records []observation.Record, entities int, now time.Time) error {
	byTopic := map[string]int{}
	newest := map[string]time.Time{}
	for _, r := range records {
		byTopic[r.Topic]++
		if r.ObservedAt.After(newest[r.Topic]) {
			newest[r.Topic] = r.ObservedAt
		}
	}

	if len(byTopic) == 0 {
		_, _ = fmt.Fprintln(w, theme.Alert("  no source answered — nothing here is a claim about the city"))
		return nil
	}

	topics := make([]string, 0, len(byTopic))
	for topic := range byTopic {
		topics = append(topics, topic)
	}
	sort.Strings(topics)

	for _, topic := range topics {
		age := ageOf(newest[topic], now.UTC())

		// Freshness is the one thing a situation board must not flatter.
		// Anything over a day is amber whatever else it says.
		shown := theme.Live(age)
		if now.UTC().Sub(newest[topic]) > 24*time.Hour {
			shown = theme.Warn(age)
		}

		_, _ = fmt.Fprintf(w, "  %s %s %s %s\n",
			theme.Indicator("live"),
			theme.Label(theme.Pad(topic, 12)),
			theme.Dim(theme.Pad(fmt.Sprint(byTopic[topic]), 6)),
			shown)
	}
	if entities > 0 {
		_, _ = fmt.Fprintf(w, "  %s %s %s %s\n",
			theme.Indicator("live"),
			theme.Label(theme.Pad("inventory", 12)),
			theme.Dim(theme.Pad(fmt.Sprint(entities), 6)),
			theme.Dim("mapped"))
	}
	return nil
}

// renderSourceHealth reports how many feeds answered, how many are held, and
// how many this build cannot yet read. All three are different problems.
func renderSourceHealth(w io.Writer, theme render.Theme, rt *runtime, results []application.Result, noAdapter []source.Source) {
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

	row := func(state, label string, count int, note string) {
		var styled string
		switch state {
		case "failing":
			styled = theme.Alert(theme.Pad(label, 12))
		case "held":
			styled = theme.Warn(theme.Pad(label, 12))
		case "none":
			styled = theme.Dim(theme.Pad(label, 12))
		default:
			styled = theme.Label(theme.Pad(label, 12))
		}
		_, _ = fmt.Fprintf(w, "  %s %s %s %s\n",
			theme.Indicator(state), styled,
			theme.Dim(theme.Pad(fmt.Sprint(count), 6)), theme.Dim(note))
	}

	row("live", "answered", answered, "reading now")
	if failed > 0 {
		row("failing", "failed", failed, "see below")
	}
	if held > 0 {
		row("held", "held", held, "reuse terms unresolved")
	}
	if len(noAdapter) > 0 {
		row("none", "no adapter", len(noAdapter), "permitted, not yet written")
	}

	for _, r := range results {
		if !r.OK() {
			_, _ = fmt.Fprintf(w, "  %s %s %s\n",
				theme.Indicator("failing"), theme.Alert(r.Source), theme.Dim(ellipsis(r.Err.Error(), 46)))
		}
	}
}
