package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	observation "github.com/FullFran/eye/internal/observation/domain"
	"github.com/FullFran/eye/internal/provider/infrastructure/aucorsa"
	source "github.com/FullFran/eye/internal/source/domain"
)

// arrivalsFormat is the registry format this command reads.
const arrivalsFormat = "aucorsa-arrivals"

// maxResolvedStops caps how many directory matches a single search will poll.
// A vague term matches many stops, and quietly firing a request at each of them
// is not a reasonable thing to do to somebody else's server.
const maxResolvedStops = 5

// busCommand shows live arrival estimates at a stop.
//
// Stops are searched by the name on the pole. The operator's own pages carry a
// second, unrelated number, and configuring that one returns no arrivals with
// no error at all — so the search exists to keep anybody from having to guess.
func busCommand() Command {
	return Command{
		Name:    "bus",
		Summary: "Live bus arrivals at a Cordoba stop",
		Run: func(ctx context.Context, args []string, stdout, stderr io.Writer) error {
			fs := flag.NewFlagSet("bus", flag.ContinueOnError)
			fs.SetOutput(stderr)
			var (
				asJSON    = fs.Bool("json", false, "output as JSON")
				stopIDs   = fs.String("stop", "", "comma-separated stop numbers, as printed on the pole")
				registryP = fs.String("registry", "", "path to an alternative sources.yaml")
				dataDir   = fs.String("data-dir", "", "override where the store and raw cache live")
			)
			term, err := parseInterspersed(fs, args)
			if err != nil {
				return err
			}

			rt, err := newRuntime(runtimeOptions{registry: *registryP, dataDir: *dataDir})
			if err != nil {
				return err
			}
			defer func() { _ = rt.Close() }()

			reader, err := rt.arrivalsReader()
			if err != nil {
				return err
			}

			stops, matched, err := resolveStops(ctx, reader, strings.Join(term, " "), *stopIDs)
			if err != nil {
				return err
			}
			if len(stops) == 0 {
				return fmt.Errorf("no stop to ask about: pass a name, or --stop with the number on the pole")
			}

			records, err := reader.Arrivals(ctx, stops)
			if err != nil {
				return err
			}

			if *asJSON {
				return writeJSON(stdout, records)
			}
			return renderArrivals(stdout, matched, records, time.Now())
		},
	}
}

// arrivalsReader builds the live arrivals adapter from the registry, refusing
// clearly when the source exists but this machine has not opted in.
//
// The refusal is the point of the gate, so it says which switch is off rather
// than reporting the source as missing.
func (r *runtime) arrivalsReader() (*aucorsa.RealtimeProvider, error) {
	var src source.Source
	for _, s := range r.sources {
		if s.Format == arrivalsFormat {
			src = s
			break
		}
	}
	if src.ID == "" {
		return nil, fmt.Errorf("this registry has no %s source", arrivalsFormat)
	}
	if !src.Automation.Pollable() {
		return nil, fmt.Errorf("source %s is %s in the registry", src.ID, src.Automation)
	}
	if src.Access.Personal() && !r.cfg.AllowPersonalSources {
		return nil, fmt.Errorf(
			"source %s is an undocumented personal source and this machine has not opted in;\n"+
				"set EYE_ALLOW_PERSONAL_SOURCES=1 to enable it — see docs/adr/0008-undocumented-personal-sources.md",
			src.ID)
	}
	return aucorsa.NewRealtime(src, r.client), nil
}

// resolveStops turns what the user asked for into stop numbers.
func resolveStops(ctx context.Context, reader *aucorsa.RealtimeProvider, term, explicit string) (ids []string, matched []aucorsa.Stop, err error) {
	if explicit != "" {
		return splitCSV(explicit), nil, nil
	}
	if term == "" {
		return reader.WatchedStops(), nil, nil
	}

	matched, err = reader.LookupStops(ctx, term)
	if err != nil {
		return nil, nil, err
	}
	if len(matched) == 0 {
		return nil, nil, fmt.Errorf("no stop in the operator's directory matches %q", term)
	}
	if len(matched) > maxResolvedStops {
		matched = matched[:maxResolvedStops]
	}

	ids = make([]string, 0, len(matched))
	for _, s := range matched {
		ids = append(ids, s.ID)
	}
	return ids, matched, nil
}

// arrival is one estimate as the adapter recorded it.
type arrival struct {
	StopID    string `json:"stop_id"`
	StopName  string `json:"stop_name"`
	Line      string `json:"line"`
	Route     string `json:"route"`
	Minutes   int    `json:"minutes"`
	Occupancy string `json:"occupancy"`
	Position  int    `json:"position"`
}

// renderArrivals prints the estimates soonest first.
func renderArrivals(w io.Writer, matched []aucorsa.Stop, records []observation.Record, now time.Time) error {
	for _, s := range matched {
		_, _ = fmt.Fprintf(w, "· %s (parada %s)\n", s.Name, s.ID)
	}
	if len(matched) > 0 {
		_, _ = fmt.Fprintln(w)
	}

	arrivals := make([]arrival, 0, len(records))
	var observedAt time.Time
	for _, r := range records {
		var a arrival
		if err := json.Unmarshal(r.Payload, &a); err != nil {
			continue
		}
		arrivals = append(arrivals, a)
		if r.ObservedAt.After(observedAt) {
			observedAt = r.ObservedAt
		}
	}

	if len(arrivals) == 0 {
		_, _ = fmt.Fprintln(w, "No buses due. The operator publishes nothing for these stops right now,")
		_, _ = fmt.Fprintln(w, "which is also what it answers outside service hours.")
		return nil
	}

	sort.SliceStable(arrivals, func(i, j int) bool { return arrivals[i].Minutes < arrivals[j].Minutes })

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "DUE\tLINE\tROUTE\tSTOP\tOCCUPANCY")
	for _, a := range arrivals {
		_, _ = fmt.Fprintf(tw, "%d min\t%s\t%s\t%s\t%s\n",
			a.Minutes, a.Line, ellipsis(a.Route, 34), ellipsis(a.StopName, 32),
			orDash(a.Occupancy))
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	// These are the operator's own predictions, not measurements, and the
	// difference is worth printing next to them every time.
	_, _ = fmt.Fprintf(w, "\n%s · operator estimate, read %s ago\n",
		plural(len(arrivals), "arrival", "arrivals"), ageOf(observedAt, now))
	return nil
}

// orDash renders an absent reading as absent rather than as blank.
func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
