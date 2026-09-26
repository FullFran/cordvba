package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	application "github.com/FullFran/cordvba/apps/eye/internal/observation/application"
	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
)

// newsCommand reads the local press. Newspapers catch what every official feed
// missed, which is exactly why they are worth reading alongside them.
func newsCommand() Command {
	return Command{
		Name:    "news",
		Summary: "What the Cordoba press is publishing right now",
		Run: func(ctx context.Context, args []string, stdout, stderr io.Writer) error {
			fs := flag.NewFlagSet("news", flag.ContinueOnError)
			fs.SetOutput(stderr)
			opts := &observeOptions{}
			opts.bind(fs, 25, 24*time.Hour)
			if err := fs.Parse(args); err != nil {
				return err
			}
			return observe(ctx, opts, []string{"press"}, stdout, stderr, renderFeed)
		},
	}
}

// civicCommand reads official publications.
func civicCommand() Command {
	return Command{
		Name:    "civic",
		Summary: "Official publications: BOE, and the open-data catalogs",
		Run: func(ctx context.Context, args []string, stdout, stderr io.Writer) error {
			fs := flag.NewFlagSet("civic", flag.ContinueOnError)
			fs.SetOutput(stderr)
			opts := &observeOptions{}
			opts.bind(fs, 25, 48*time.Hour)
			if err := fs.Parse(args); err != nil {
				return err
			}
			return observe(ctx, opts, []string{"civic", "city"}, stdout, stderr, renderFeed)
		},
	}
}

// eventsCommand lists what Cordoba is about to do.
func eventsCommand() Command {
	return Command{
		Name:    "events",
		Summary: "What is scheduled in Cordoba",
		Run: func(ctx context.Context, args []string, stdout, stderr io.Writer) error {
			fs := flag.NewFlagSet("events", flag.ContinueOnError)
			fs.SetOutput(stderr)
			opts := &observeOptions{}
			opts.bind(fs, 30, 0)
			if err := fs.Parse(args); err != nil {
				return err
			}
			return observe(ctx, opts, []string{"events"}, stdout, stderr, renderEvents)
		},
	}
}

// renderEvents prints events by start time rather than by publication time,
// because for an event the interesting date is the one in the future.
func renderEvents(w io.Writer, records []observation.Record, results []application.Result, now time.Time) error {
	if len(records) == 0 {
		_, _ = fmt.Fprintln(w, "No scheduled events from the live sources.")
		return nil
	}

	upcoming := make([]observation.Record, 0, len(records))
	for _, r := range records {
		if r.ValidFrom == nil || !r.ValidFrom.Before(now) {
			upcoming = append(upcoming, r)
		}
	}
	if len(upcoming) == 0 {
		upcoming = records
	}

	// Earliest first: a listing of what is coming reads forwards.
	for i := 1; i < len(upcoming); i++ {
		for j := i; j > 0 && startOf(upcoming[j]).Before(startOf(upcoming[j-1])); j-- {
			upcoming[j], upcoming[j-1] = upcoming[j-1], upcoming[j]
		}
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, r := range upcoming {
		when := "date unknown"
		if r.ValidFrom != nil {
			when = r.ValidFrom.Local().Format("Mon 02 Jan 15:04")
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n",
			when,
			ageOf(startOfOrZero(r), now),
			ellipsis(r.Title, 82),
		)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	return renderProvenanceFooter(w, upcoming, results)
}

// startOf is an event's start, falling back to when we observed it so that
// sorting stays stable for entries with no published time.
func startOf(r observation.Record) time.Time {
	if r.ValidFrom != nil {
		return *r.ValidFrom
	}
	return r.ObservedAt
}

// startOfOrZero returns the published start, or the zero time when the source
// published none. A missing date renders as missing, never as a guess.
func startOfOrZero(r observation.Record) time.Time {
	if r.ValidFrom != nil {
		return *r.ValidFrom
	}
	return time.Time{}
}

// skyCommand shows the aircraft currently over the configured viewport.
func skyCommand() Command {
	return Command{
		Name:    "sky",
		Summary: "Aircraft currently over Cordoba",
		Run: func(ctx context.Context, args []string, stdout, stderr io.Writer) error {
			fs := flag.NewFlagSet("sky", flag.ContinueOnError)
			fs.SetOutput(stderr)
			opts := &observeOptions{}
			opts.bind(fs, 40, time.Hour)
			if err := fs.Parse(args); err != nil {
				return err
			}
			return observe(ctx, opts, []string{"air"}, stdout, stderr, renderSky)
		},
	}
}

// renderSky prints the current air picture.
func renderSky(w io.Writer, records []observation.Record, results []application.Result, now time.Time) error {
	if len(records) == 0 {
		_, _ = fmt.Fprintln(w, "No aircraft in the configured viewport.")
		return nil
	}

	centre := observation.Point{Lat: 37.8882, Lon: -4.7794}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "CALLSIGN\tDISTANCE\tPOSITION\tAGE")
	for _, r := range records {
		distance := "—"
		coords := "—"
		if r.Position != nil {
			distance = fmt.Sprintf("%.0f km", centre.DistanceKm(*r.Position))
			coords = fmt.Sprintf("%.3f, %.3f", r.Position.Lat, r.Position.Lon)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			ellipsis(r.Title, 12), distance, coords, ageOf(r.ObservedAt, now))
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(w, "\n%s · positions expire from the store within 72h by design\n",
		plural(len(records), "aircraft", "aircraft"))
	return renderProvenanceFooter(w, records, results)
}

// camerasCommand lists the municipal traffic camera inventory.
//
// Positions and names only. There is no media path in eye, by design.
func camerasCommand() Command {
	return Command{
		Name:    "cameras",
		Summary: "The municipal traffic camera inventory (positions only)",
		Run: func(ctx context.Context, args []string, stdout, stderr io.Writer) error {
			fs := flag.NewFlagSet("cameras", flag.ContinueOnError)
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

			ps, _ := rt.pollable("transport")
			results := rt.collect(ctx, ps)
			reportFailures(stderr, results)

			entities, err := rt.store.Entities(ctx, observation.Filter{Kinds: []string{"camera"}})
			if err != nil {
				return err
			}
			if len(entities) == 0 {
				return fmt.Errorf("no camera inventory available from the live sources")
			}

			if *asJSON {
				return writeJSON(stdout, entities)
			}

			centre := observation.Point{Lat: 37.8882, Lon: -4.7794}
			tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "LOCATION\tPOSITION\tFROM CENTRE")
			for _, e := range entities {
				coords, distance := "—", "—"
				if e.Position != nil {
					coords = fmt.Sprintf("%.4f, %.4f", e.Position.Lat, e.Position.Lon)
					distance = fmt.Sprintf("%.1f km", centre.DistanceKm(*e.Position))
				}
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", ellipsis(e.Title, 52), coords, distance)
			}
			if err := tw.Flush(); err != nil {
				return err
			}

			_, _ = fmt.Fprintf(stdout, "\n%s · %s · licence: %s\n",
				plural(len(entities), "camera", "cameras"),
				entities[0].Provenance.Publisher,
				entities[0].Provenance.License)
			_, _ = fmt.Fprintln(stdout, "Inventory only. eye maps cameras; it does not stream them.")
			return nil
		},
	}
}
