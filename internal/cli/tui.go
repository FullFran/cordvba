package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/FullFran/eye/internal/logging"
	observation "github.com/FullFran/eye/internal/observation/domain"
	providers "github.com/FullFran/eye/internal/provider/infrastructure"
	"github.com/FullFran/eye/internal/render"
	source "github.com/FullFran/eye/internal/source/domain"
	"github.com/FullFran/eye/internal/tui"
	"github.com/FullFran/eye/internal/version"
)

// Defaults for the cockpit.
const (
	// defaultTUIInterval is how often the store is re-read. It is a local
	// read, so it can be brisk; nothing on this timer touches the network.
	defaultTUIInterval = 5 * time.Second
	// defaultFallbackSize is the terminal to assume when there is none to
	// ask, which is every pipe and every test.
	defaultFallbackSize = "80x24"

	// tuiRecordLimit and tuiEntityLimit cap one read. A store with a
	// fortnight of ADS-B in it should not make the board take a second to
	// draw, and no screen shows more than this at once.
	tuiRecordLimit = 600
	tuiEntityLimit = 4000
)

// tuiCommand opens the full-screen cockpit.
//
// Every other command answers a question you thought to ask, and `eye watch`
// keeps one board on screen. This is the whole system at once: the store, the
// registry, the map and the transit boards, in one terminal you can steer.
func tuiCommand() Command {
	return Command{
		Name:    "tui",
		Summary: "A full-screen cockpit over everything eye knows",
		Run: func(ctx context.Context, args []string, stdout, stderr io.Writer) error {
			fs := flag.NewFlagSet("tui", flag.ContinueOnError)
			fs.SetOutput(stderr)
			var (
				registryP = fs.String("registry", "", "path to an alternative sources.yaml")
				dataDir   = fs.String("data-dir", "", "override where the store and raw cache live")
				interval  = fs.Duration("interval", defaultTUIInterval, "how often to re-read the store")
				fallback  = fs.String("fallback-size", defaultFallbackSize,
					"terminal size to assume when it cannot be queried, as WxH")
				view = fs.String("view", "dashboard",
					"the screen to open on: dashboard, map, feed, sources, transit")
				once = fs.Bool("once", false, "draw a single frame and exit")
			)
			if err := fs.Parse(args); err != nil {
				return err
			}

			size, err := parseSize(*fallback)
			if err != nil {
				return fmt.Errorf("--fallback-size: %w", err)
			}
			start, err := tui.ParseView(*view)
			if err != nil {
				return fmt.Errorf("--view: %w", err)
			}

			rt, err := newRuntime(runtimeOptions{registry: *registryP, dataDir: *dataDir})
			if err != nil {
				return err
			}
			defer func() { _ = rt.Close() }()

			// Log lines would tear the board apart, so the cockpit is silent
			// on stderr; `eye daemon` is where you watch the polling work.
			_ = logging.Discard()

			now := time.Now()
			return tui.Run(ctx, tui.RunOptions{
				Options: tui.Options{
					Version:   version.String(),
					Registry:  rt.registryPath,
					Theme:     render.NewTheme(stdout),
					Interval:  *interval,
					StartedAt: now,
					Now:       now,
					Start:     start,
				},
				Out:      stdout,
				Reader:   &storeReader{rt: rt},
				Fallback: size,
				Once:     *once,
			})
		},
	}
}

// parseSize reads a WxH terminal size.
func parseSize(s string) (tui.Size, error) {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == 'x' || r == 'X' })
	if len(fields) != 2 {
		return tui.Size{}, fmt.Errorf("%q is not a size: write it as 120x40", s)
	}

	cols, colErr := strconv.Atoi(strings.TrimSpace(fields[0]))
	rows, rowErr := strconv.Atoi(strings.TrimSpace(fields[1]))
	if colErr != nil || rowErr != nil {
		return tui.Size{}, fmt.Errorf("%q is not a size: both halves must be numbers", s)
	}
	if cols <= 0 || rows <= 0 {
		return tui.Size{}, fmt.Errorf("%q is not a size: a terminal has a positive width and height", s)
	}
	return tui.Size{Cols: cols, Rows: rows}, nil
}

// storeReader is the cockpit's window onto the runtime.
//
// It is the only thing in the cockpit that performs I/O, and it is deliberately
// three named operations rather than a handle to the store: Read is local and
// runs on a timer, Poll and Arrivals touch the network and run only when
// somebody asks for them.
type storeReader struct {
	rt *runtime
}

// Read takes a picture of the store. It polls nothing.
func (r *storeReader) Read(ctx context.Context) (tui.Data, error) {
	records, err := r.rt.query(ctx, observation.Filter{Limit: tuiRecordLimit})
	if err != nil {
		return tui.Data{}, err
	}
	entities, err := r.rt.store.Entities(ctx, observation.Filter{Limit: tuiEntityLimit})
	if err != nil {
		return tui.Data{}, err
	}
	recordCount, entityCount, err := r.rt.store.Counts(ctx)
	if err != nil {
		return tui.Data{}, err
	}
	states, err := r.rt.store.States(ctx)
	if err != nil {
		return tui.Data{}, err
	}

	now := time.Now().UTC()
	rows := make([]tui.SourceRow, 0, len(r.rt.sources))
	for _, s := range r.rt.sources {
		row := tui.SourceRow{
			ID: s.ID, Authority: s.Authority, Topic: s.Topic, Format: s.Format,
			License: s.License, Access: string(s.Access), Automation: string(s.Automation),
			Pollable:   pollableNow(s, r.rt.cfg.AllowPersonalSources),
			HasAdapter: providers.Supported(s.Format),
			Interval:   s.Interval,
		}
		if st, ok := states[s.ID]; ok {
			health := healthView(st, s, now)
			row.LastAttempt = st.LastAttempt
			row.LastSuccess = st.LastSuccess
			row.LastError = st.LastError
			row.Records = st.Records
			row.Stale = health.Stale
		}
		rows = append(rows, row)
	}

	return tui.Data{
		ReadAt:      time.Now(),
		Registry:    r.rt.registryPath,
		Records:     records,
		Entities:    entities,
		Sources:     rows,
		RecordCount: recordCount,
		EntityCount: entityCount,
	}, nil
}

// Poll fetches one source live, because somebody pressed r.
//
// The refusals are as important as the fetch: a source that is held, or that
// this build has no adapter for, or that needs the machine's own opt-in, must
// say which of those it is rather than silently returning nothing.
func (r *storeReader) Poll(ctx context.Context, id string) (string, error) {
	var src source.Source
	for _, s := range r.rt.sources {
		if s.ID == id {
			src = s
			break
		}
	}
	if src.ID == "" {
		return "", fmt.Errorf("no source %q in this registry", id)
	}
	if !src.Automation.Pollable() {
		return "", fmt.Errorf("%s is %s in the registry, so eye will not fetch it", id, src.Automation)
	}
	if !providers.Supported(src.Format) {
		return "", fmt.Errorf("%s is permitted but this build has no %s adapter", id, src.Format)
	}
	if src.Access.Personal() && !r.rt.cfg.AllowPersonalSources {
		return "", fmt.Errorf("%s is an undocumented personal source and this machine has not opted in", id)
	}

	// The same options the scheduler polls with, credentials included: a
	// source polled from the cockpit must behave exactly as it does from
	// `eye daemon`, or `r` becomes a different kind of poll.
	ps, _ := providers.BuildPollable([]source.Source{src}, r.rt.client, providers.Options{
		AllowPersonal: r.rt.cfg.AllowPersonalSources,
		AEMETAPIKey:   r.rt.cfg.AEMETAPIKey,
		FIRMSMapKey:   r.rt.cfg.FIRMSMapKey,
	})
	if len(ps) == 0 {
		return "", fmt.Errorf("%s could not be built as an adapter", id)
	}

	started := time.Now()
	results := r.rt.collect(ctx, ps)
	elapsed := time.Since(started).Round(time.Millisecond)

	// One source in, so at most one result out. An empty slice means the
	// collector had nothing to run, which is not the same as a source that
	// answered with nothing.
	if len(results) == 0 {
		return "", fmt.Errorf("%s was not polled: the collector had nothing to run", id)
	}

	res := results[0]
	if !res.OK() {
		return "", fmt.Errorf("%s: %w", id, res.Err)
	}
	return fmt.Sprintf("%s · %s in %s", id,
		plural(res.Health.Records, "record", "records"), elapsed), nil
}

// Arrivals asks the operator about one stop.
func (r *storeReader) Arrivals(ctx context.Context, stop string) ([]tui.Arrival, error) {
	reader, err := r.rt.arrivalsReader()
	if err != nil {
		return nil, err
	}

	records, err := reader.Arrivals(ctx, []string{stop})
	if err != nil {
		return nil, err
	}

	out := make([]tui.Arrival, 0, len(records))
	for _, rec := range records {
		var a arrival
		if err := json.Unmarshal(rec.Payload, &a); err != nil {
			// An estimate eye can no longer read is not an estimate eye can
			// show. Skipping beats printing a blank row.
			continue
		}
		out = append(out, tui.Arrival{
			StopID: a.StopID, StopName: a.StopName,
			Line: a.Line, Route: a.Route,
			Minutes: a.Minutes, Occupancy: a.Occupancy,
			// ObservedAt is when the operator computed the estimate, which
			// is what decides whether it may still be called live.
			ReadAt: rec.ObservedAt,
		})
	}
	return out, nil
}
