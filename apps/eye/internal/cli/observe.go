package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	application "github.com/FullFran/eye/internal/observation/application"
	observation "github.com/FullFran/eye/internal/observation/domain"
)

// observeOptions are the flags every "go and look" command shares.
type observeOptions struct {
	limit    int
	since    time.Duration
	text     string
	sourceID string
	asJSON   bool
	offline  bool
	registry string
	dataDir  string
}

// bind registers the shared flags on a flag set.
func (o *observeOptions) bind(fs *flag.FlagSet, defaultLimit int, defaultSince time.Duration) {
	fs.IntVar(&o.limit, "limit", defaultLimit, "maximum entries to show")
	fs.DurationVar(&o.since, "since", defaultSince, "only entries observed within this window")
	fs.StringVar(&o.text, "text", "", "only entries whose title or description contains this")
	fs.StringVar(&o.sourceID, "source", "", "only this source id")
	fs.BoolVar(&o.asJSON, "json", false, "output as JSON")
	fs.BoolVar(&o.offline, "offline", false, "answer from the local store without polling any source")
	fs.StringVar(&o.registry, "registry", "", "path to an alternative sources.yaml")
	fs.StringVar(&o.dataDir, "data-dir", "", "override where the store and raw cache live")
}

// filter turns the flags into a domain filter.
func (o *observeOptions) filter(topics []string, now time.Time) observation.Filter {
	f := observation.Filter{Topics: topics, Text: o.text, Limit: o.limit}
	if o.sourceID != "" {
		f.Sources = []string{o.sourceID}
	}
	if o.since > 0 {
		since := now.Add(-o.since)
		f.Since = &since
	}
	return f
}

// recordView is the JSON shape of an observation.
type recordView struct {
	ID         string          `json:"id"`
	Source     string          `json:"source"`
	Publisher  string          `json:"publisher"`
	Topic      string          `json:"topic"`
	Kind       string          `json:"kind"`
	Title      string          `json:"title"`
	Summary    string          `json:"summary,omitempty"`
	ObservedAt time.Time       `json:"observed_at"`
	FetchedAt  time.Time       `json:"fetched_at"`
	ValidFrom  *time.Time      `json:"valid_from,omitempty"`
	LatencyS   float64         `json:"source_latency_seconds"`
	Severity   int             `json:"severity"`
	Quality    string          `json:"quality"`
	License    string          `json:"license"`
	URL        string          `json:"url"`
	Position   *position       `json:"position,omitempty"`
	Payload    json.RawMessage `json:"payload,omitempty"`
}

// position is the JSON projection of a coordinate.
type position struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// toView projects a record for machine consumption, keeping provenance visible.
func toView(r observation.Record) recordView {
	v := recordView{
		ID:         r.ID,
		Source:     r.Source,
		Publisher:  r.Provenance.Publisher,
		Topic:      r.Topic,
		Kind:       r.Kind,
		Title:      r.Title,
		Summary:    r.Description,
		ObservedAt: r.ObservedAt,
		FetchedAt:  r.FetchedAt,
		ValidFrom:  r.ValidFrom,
		LatencyS:   r.Latency().Seconds(),
		Severity:   int(r.Severity),
		Quality:    string(r.Quality),
		License:    r.Provenance.License,
		URL:        r.Provenance.SourceURL,
		Payload:    r.Payload,
	}
	if r.Position != nil {
		v.Position = &position{Lat: r.Position.Lat, Lon: r.Position.Lon}
	}
	return v
}

// toViews projects a slice of records.
func toViews(records []observation.Record) []recordView {
	out := make([]recordView, 0, len(records))
	for _, r := range records {
		out = append(out, toView(r))
	}
	return out
}

// observe is the shared body of the topic commands: poll the live sources for
// those topics, then answer from what came back.
func observe(ctx context.Context, opts *observeOptions, topics []string, stdout, stderr io.Writer, render func(io.Writer, []observation.Record, []application.Result, time.Time) error) error {
	rt, err := newRuntime(runtimeOptions{registry: opts.registry, dataDir: opts.dataDir})
	if err != nil {
		return err
	}
	defer func() { _ = rt.Close() }()

	var results []application.Result

	if opts.offline {
		// Answer from what earlier runs collected. Useful on a train,
		// and the only way to ask a question without adding traffic to
		// a public service.
		if records, _ := rt.query(ctx, observation.Filter{Topics: topics, Limit: 1}); len(records) == 0 {
			return fmt.Errorf("the local store holds nothing for %s — run without --offline first, or start eye daemon",
				strings.Join(topics, ", "))
		}
	} else {
		ps, noAdapter := rt.pollable(topics...)
		if len(ps) == 0 {
			return fmt.Errorf("no live source for %s (%d enabled but unreadable by this build)",
				strings.Join(topics, ", "), len(noAdapter))
		}
		results = rt.collect(ctx, ps)
		reportFailures(stderr, results)
	}

	now := time.Now().UTC()
	records, err := rt.query(ctx, opts.filter(topics, now))
	if err != nil {
		return err
	}

	if opts.asJSON {
		return writeJSON(stdout, toViews(records))
	}
	return render(stdout, records, results, now)
}

// reportFailures writes one line per source that could not be polled. A public
// service being down is an ordinary Tuesday; hiding it is not acceptable.
func reportFailures(w io.Writer, results []application.Result) {
	for _, r := range results {
		if r.OK() {
			continue
		}
		_, _ = fmt.Fprintf(w, "warning: %s unavailable: %v\n", r.Source, r.Err)
	}
}

// renderFeed prints a dated list of observations: press, civic documents, and
// anything else whose natural shape is a timeline.
func renderFeed(w io.Writer, records []observation.Record, results []application.Result, now time.Time) error {
	if len(records) == 0 {
		_, _ = fmt.Fprintln(w, "Nothing in the selected window.")
		return nil
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, r := range records {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n",
			ageOf(r.ObservedAt, now),
			ellipsis(r.Provenance.Publisher, 22),
			ellipsis(r.Title, 88),
		)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	return renderProvenanceFooter(w, records, results)
}

// renderProvenanceFooter closes every human-readable listing with where the
// figures came from. It is the property that separates eye from a rumour.
func renderProvenanceFooter(w io.Writer, records []observation.Record, results []application.Result) error {
	sources := map[string]bool{}
	for _, r := range records {
		sources[r.Provenance.Publisher] = true
	}

	var ok, failed int
	for _, r := range results {
		if r.OK() {
			ok++
		} else {
			failed++
		}
	}

	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	sortStrings(names)

	_, _ = fmt.Fprintf(w, "\n%s from %s\n",
		plural(len(records), "entry", "entries"),
		strings.Join(names, " · "))

	if failed > 0 {
		_, _ = fmt.Fprintf(w, "%d of %d sources answered.\n", ok, ok+failed)
	}
	return nil
}

// sortStrings sorts in place; kept local so render code has no import churn.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
