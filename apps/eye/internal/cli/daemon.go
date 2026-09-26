package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/FullFran/cordvba/apps/eye/internal/logging"
	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	store "github.com/FullFran/cordvba/apps/eye/internal/observation/infrastructure"
	provider "github.com/FullFran/cordvba/apps/eye/internal/provider/domain"
	"github.com/FullFran/cordvba/apps/eye/internal/scheduler"
)

// daemonCommand runs the scheduler until interrupted.
//
// This is the mode that turns eye from a thing you ask into a thing that
// watches. Change detection needs continuous observation, and continuous
// observation needs a process that stays up.
func daemonCommand() Command {
	return Command{
		Name:    "daemon",
		Summary: "Poll every live source continuously and persist what they return",
		Run: func(ctx context.Context, args []string, stdout, stderr io.Writer) error {
			fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
			fs.SetOutput(stderr)
			var (
				registryP = fs.String("registry", "", "path to an alternative sources.yaml")
				dataDir   = fs.String("data-dir", "", "override where the store and raw cache live")
				pruneFreq = fs.Duration("prune-every", time.Hour, "how often to enforce retention")
				once      = fs.Bool("once", false, "poll every source a single time and exit")
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
			if len(ps) == 0 {
				return errors.New("no live source in the registry")
			}

			log := logging.New(logging.Options{Level: rt.cfg.LogLevel, Writer: stderr})
			log.Info("starting",
				"sources", len(ps),
				"awaiting_adapter", len(noAdapter),
				"store", rt.store.Path(),
				"raw_cache", rt.cache.Root(),
				"registry", rt.registryPath)

			if *once {
				return runOnce(ctx, rt, ps, stdout)
			}

			// One immediate pass before the scheduler takes over. An
			// operator who starts the daemon should have the full
			// picture within a second, not after a random stagger,
			// and a source polled every twelve hours should not make
			// them wait to find out whether it works at all.
			primed := rt.collect(ctx, ps)
			var ok int
			for _, r := range primed {
				if r.OK() {
					ok++
					continue
				}
				log.Warn("initial poll failed", "source", r.Source, "error", r.Err)
			}
			log.Info("initial pass complete", "answered", ok, "of", len(primed))

			sink := newDaemonSink(rt, log)
			sch := scheduler.New(ps, sink, scheduler.Options{
				Backoff: scheduler.DefaultBackoff,
				Breaker: scheduler.DefaultBreaker,
				Logger:  log,
			})

			// A restart during an outage must not reset a tripped
			// breaker and hammer a source that is already struggling.
			states, err := rt.store.States(ctx)
			if err != nil {
				return err
			}
			for id, st := range states {
				sch.Seed(id, st.ConsecutiveErrors, st.LastAttempt)
			}

			go pruneLoop(ctx, rt, *pruneFreq, log)

			// A deadline is as legitimate a way to stop as a signal:
			// systemd timeouts and `timeout 30s eye daemon` both
			// arrive here, and neither is a failure.
			if err := sch.Run(ctx); err != nil &&
				!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				return err
			}

			log.Info("stopped")
			return nil
		},
	}
}

// runOnce polls every source a single time. It is what a systemd timer or a
// cron entry would call, for people who would rather not supervise a process.
func runOnce(ctx context.Context, rt *runtime, ps []provider.Provider, stdout io.Writer) error {
	results := rt.collect(ctx, ps)

	pr, err := rt.prune(ctx)
	if err != nil {
		return err
	}

	var stored, failed int
	for _, r := range results {
		stored += r.Records
		if !r.OK() {
			failed++
		}
	}

	records, entities, err := rt.store.Counts(ctx)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stdout, "%s polled · %s new · %s pruned · %s pruned (%s)\n",
		plural(len(results), "source", "sources"),
		plural(stored, "record", "records"),
		plural(pr.Records, "expired record", "expired records"),
		plural(pr.Payloads, "raw payload", "raw payloads"),
		formatBytes(pr.PayloadBytes))
	if failed > 0 {
		_, _ = fmt.Fprintf(stdout, "%d failed\n", failed)
	}
	_, _ = fmt.Fprintf(stdout, "store holds %s and %s · %s\n",
		plural(records, "record", "records"),
		plural(entities, "asset", "assets"),
		rt.store.Path())
	_, _ = fmt.Fprintf(stdout, "raw cache holds %s\n", formatBytes(pr.RawBytes))
	return nil
}

// pruneLoop enforces retention on a timer.
func pruneLoop(ctx context.Context, rt *runtime, every time.Duration, log *loggerAlias) {
	if every <= 0 {
		return
	}

	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pr, err := rt.prune(ctx)
			if err != nil {
				log.Error("prune failed", "error", err)
				continue
			}
			// Logged every cycle, not only when something was removed: the
			// remaining sizes are exactly what tells an operator the raw
			// cache is staying bounded, or is not.
			log.Info("retention enforced",
				"expired_records", pr.Records,
				"payloads_removed", pr.Payloads,
				"payload_bytes_freed", pr.PayloadBytes,
				"store_bytes", pr.StoreBytes,
				"raw_cache_bytes", pr.RawBytes)
		}
	}
}

// formatBytes renders a byte count the way an operator reads a directory
// listing, without pulling in a dependency for it.
func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// daemonSink persists what the scheduler collects and remembers each source's
// polling state between runs.
type daemonSink struct {
	rt  *runtime
	log *loggerAlias
}

// newDaemonSink builds the scheduler's sink.
func newDaemonSink(rt *runtime, log *loggerAlias) *daemonSink {
	return &daemonSink{rt: rt, log: log}
}

// Store persists records, and the inventory when the provider publishes one.
func (s *daemonSink) Store(ctx context.Context, p provider.Provider, records []observation.Record) (int, error) {
	stored, err := s.rt.store.Append(ctx, records)
	if err != nil {
		return 0, err
	}

	if ep, ok := p.(provider.EntityProvider); ok {
		entities, err := ep.Entities(ctx)
		if err != nil {
			// An inventory failure is worth reporting but must not
			// discard the observations that already came back.
			s.log.Warn("inventory refresh failed", "source", p.Info().ID, "error", err)
			return stored, nil
		}
		if _, err := s.rt.store.Upsert(ctx, entities); err != nil {
			return stored, err
		}
	}
	return stored, nil
}

// Report persists the source's polling state so a restart resumes where it
// left off instead of re-downloading everything.
func (s *daemonSink) Report(ctx context.Context, o scheduler.Outcome) {
	if o.Skipped() {
		return
	}

	st := store.SourceState{
		SourceID:          o.SourceID,
		LastAttempt:       o.Attempted,
		ConsecutiveErrors: o.ConsecutiveErrors,
		Records:           o.Records,
	}
	if o.OK() {
		st.LastSuccess = o.Attempted
	} else if o.Err != nil {
		st.LastError = o.Err.Error()
	}

	if err := s.rt.store.SaveState(ctx, st); err != nil {
		s.log.Error("could not persist source state", "source", o.SourceID, "error", err)
	}
}

var _ scheduler.Sink = (*daemonSink)(nil)
