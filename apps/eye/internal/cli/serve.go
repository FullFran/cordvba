package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/FullFran/eye/internal/api"
	"github.com/FullFran/eye/internal/logging"
	observationdomain "github.com/FullFran/eye/internal/observation/domain"
	store "github.com/FullFran/eye/internal/observation/infrastructure"
	"github.com/FullFran/eye/internal/provider/infrastructure/aucorsa"
	"github.com/FullFran/eye/internal/web"
)

// serveTimeouts keep a stalled client from holding a connection forever.
const (
	serveReadTimeout  = 15 * time.Second
	serveWriteTimeout = 60 * time.Second
	serveIdleTimeout  = 2 * time.Minute
	serveShutdownWait = 10 * time.Second
)

// serveCommand exposes the store over HTTP.
//
// This is the adapter that makes eye a gateway: providers are written once, and
// anything that speaks HTTP reads the same normalized records with the same
// provenance attached.
func serveCommand() Command {
	return Command{
		Name:    "serve",
		Summary: "Serve observations over HTTP, so other programs can read them",
		Run: func(ctx context.Context, args []string, stdout, stderr io.Writer) error {
			fs := flag.NewFlagSet("serve", flag.ContinueOnError)
			fs.SetOutput(stderr)
			var (
				registryP = fs.String("registry", "", "path to an alternative sources.yaml")
				dataDir   = fs.String("data-dir", "", "override where the store and raw cache live")
				addr      = fs.String("addr", "127.0.0.1:8787", "address to listen on")
				public    = fs.Bool("public", false, "allow binding to a non-loopback address")
				tokenFile = fs.String("token-file", "", "read the API token from this file instead of EYE_API_TOKEN")
				probeURL  = fs.String("probe", "", "check that an eye server answers at this URL, then exit")
				noUI      = fs.Bool("no-ui", false, "serve only the JSON API, without the web console")
			)
			if err := fs.Parse(args); err != nil {
				return err
			}

			// A container health check runs this: a distroless image has
			// no shell and no curl, so the probe is eye itself. It opens
			// no store and binds nothing — a health check that contends
			// for the database is a health check that causes outages.
			if *probeURL != "" {
				return probe(ctx, *probeURL, stdout)
			}

			if err := checkBind(*addr, *public); err != nil {
				return err
			}

			rt, err := newRuntime(runtimeOptions{registry: *registryP, dataDir: *dataDir})
			if err != nil {
				return err
			}
			defer func() { _ = rt.Close() }()

			token, err := resolveToken(*tokenFile, rt.cfg.APIToken)
			if err != nil {
				return err
			}
			if err := checkPublicAuth(*public, token); err != nil {
				return err
			}

			log := logging.New(logging.Options{Level: rt.cfg.LogLevel, Writer: stderr})

			options := []api.Option{
				api.WithToken(token),
				api.WithCORSOrigin(rt.cfg.CORSOrigin),
				api.WithPersonalSources(rt.cfg.AllowPersonalSources),
			}
			// The console is compiled into the binary, so serving it costs
			// nothing to deploy and --no-ui exists for the deployment that
			// wants an API and no web page on its root.
			if !*noUI {
				options = append(options, api.WithUI(web.Handler()))
			}
			server := &http.Server{
				Addr: *addr,
				Handler: api.New(apiStore{rt.store}, rt.sources, log,
					append(options, api.WithTransit(rt.transitReader()))...,
				).Handler(),
				ReadTimeout:  serveReadTimeout,
				WriteTimeout: serveWriteTimeout,
				IdleTimeout:  serveIdleTimeout,
			}

			// ListenConfig rather than net.Listen: the listener then
			// respects the same context that stops the server.
			var lc net.ListenConfig
			listener, err := lc.Listen(ctx, "tcp", *addr)
			if err != nil {
				return fmt.Errorf("listen on %s: %w", *addr, err)
			}

			_, _ = fmt.Fprintf(stdout, "eye is serving on http://%s\n", listener.Addr())
			if !*noUI {
				_, _ = fmt.Fprintf(stdout, "  console: http://%s/\n", listener.Addr())
			}
			_, _ = fmt.Fprintf(stdout, "  /v1   /v1/records   /v1/entities   /v1/sources   /v1/stats\n")
			_, _ = fmt.Fprintf(stdout, "  /v1/changes   /v1/geojson   /v1/transit/...   /openapi.json   /health\n")
			_, _ = fmt.Fprintf(stdout, "  store: %s\n", rt.store.Path())
			if token != "" {
				_, _ = fmt.Fprintln(stdout, "  auth:  /v1 needs Authorization: Bearer <EYE_API_TOKEN>")
			} else {
				_, _ = fmt.Fprintln(stdout, "  auth:  none — /v1 is open to anything that can reach this address")
			}
			_, _ = fmt.Fprintln(stdout, "\nThis serves observations. It never serves camera images:")
			_, _ = fmt.Fprintln(stdout, "their licence permits personal use only.")

			errs := make(chan error, 1)
			go func() {
				if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
					errs <- err
				}
			}()

			select {
			case err := <-errs:
				return err
			case <-ctx.Done():
			}

			shutdownCtx, cancel := context.WithTimeout(context.Background(), serveShutdownWait)
			defer cancel()

			_, _ = fmt.Fprintln(stdout, "\nshutting down")
			return server.Shutdown(shutdownCtx)
		},
	}
}

// checkBind refuses a non-loopback address unless it was asked for explicitly.
//
// eye reads sources under licences that in several cases permit personal use
// only, and the store holds an aircraft position history. Exposing that to a
// network should be a decision somebody made on purpose, not the default that
// came with a config file.
func checkBind(addr string, public bool) error {
	if public {
		return nil
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("addr %q: %w", addr, err)
	}

	if host == "" || host == "0.0.0.0" || host == "::" {
		return errors.New("refusing to listen on every interface: pass --public if that is what you mean")
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() {
		return fmt.Errorf("refusing to listen on the non-loopback address %s: pass --public if that is what you mean", host)
	}
	if ip := net.ParseIP(host); ip == nil && !strings.EqualFold(host, "localhost") {
		return fmt.Errorf("refusing to listen on %q: pass --public if that is what you mean", host)
	}
	return nil
}

// checkPublicAuth refuses a public bind that nothing authenticates.
//
// A loopback bind with no token is somebody's own machine, and asking them to
// invent a credential to read their own store is ceremony. A public bind with
// no token is the same store offered to whoever finds the port, and eye reads
// sources under licences that in several cases permit personal use only. That
// is not a choice worth defaulting into.
func checkPublicAuth(public bool, token string) error {
	if !public || token != "" {
		return nil
	}
	return errors.New(
		"refusing to serve publicly with no token: set EYE_API_TOKEN, or pass --token-file <path>")
}

// resolveToken picks the API token, preferring an explicit file.
//
// A file is the form a container or a systemd unit can actually deliver: a
// secret in an environment variable shows up in a process listing and in every
// crash report that dumps the environment.
func resolveToken(path, fromEnv string) (string, error) {
	if path == "" {
		return fromEnv, nil
	}

	body, err := os.ReadFile(path) // #nosec G304 -- operator-supplied path, never request input
	if err != nil {
		return "", fmt.Errorf("read token file %s: %w", path, err)
	}

	// Trailing whitespace is what every editor and every `echo` writes, and
	// a token carrying a newline fails in a way nobody can see.
	token := strings.TrimSpace(string(body))
	if token == "" {
		return "", fmt.Errorf("token file %s is empty", path)
	}
	return token, nil
}

// apiStore adapts the SQLite store to the API's read ports.
//
// The store speaks its own state type and the port speaks the API's. This is
// the one place the two meet, which is what keeps the store from having to
// know an HTTP adapter exists.
type apiStore struct{ *store.SQLiteStore }

// States projects stored polling health onto the API's port, in a stable
// order so two identical deployments answer identically.
func (s apiStore) States(ctx context.Context) ([]api.SourceState, error) {
	states, err := s.SQLiteStore.States(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]api.SourceState, 0, len(states))
	for _, st := range states {
		out = append(out, api.SourceState{
			SourceID: st.SourceID, LastAttempt: st.LastAttempt, LastSuccess: st.LastSuccess,
			LastError: st.LastError, Records: st.Records, ConsecutiveErrors: st.ConsecutiveErrors,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SourceID < out[j].SourceID })
	return out, nil
}

// Aggregate projects the store's own summary onto the API's port.
func (s apiStore) Aggregate(ctx context.Context) (api.Aggregate, error) {
	agg, err := s.SQLiteStore.Aggregate(ctx)
	if err != nil {
		return api.Aggregate{}, err
	}
	return api.Aggregate{
		Records: agg.Records, Entities: agg.Entities,
		ByTopic: agg.ByTopic, BySource: agg.BySource, ByKind: agg.ByKind,
		Oldest: agg.Oldest, Newest: agg.Newest,
	}, nil
}

// StoreBytes reports how much disk the store occupies.
func (s apiStore) StoreBytes() (int64, error) {
	info, err := os.Stat(s.Path())
	if err != nil {
		return 0, fmt.Errorf("stat store: %w", err)
	}
	return info.Size(), nil
}

// transitReader wires the live transit endpoints to the readers eye already
// has: AUCORSA for live arrivals, and the stored RENFE timetable for
// departures.
//
// It returns nil when this machine cannot read live arrivals at all — no such
// source in the registry, or the personal-source gate closed — and the API
// answers 503 rather than pretending the operator's server is down.
func (r *runtime) transitReader() api.Transit {
	reader, err := r.arrivalsReader()
	if err != nil {
		return busTransit{store: r.store}
	}
	return busTransit{arrivals: reader, store: r.store}
}

// busTransit adapts eye's readers to the API's transit port.
type busTransit struct {
	arrivals *aucorsa.RealtimeProvider
	store    *store.SQLiteStore
}

// Arrivals passes a live request through to the operator's endpoint.
func (b busTransit) Arrivals(ctx context.Context, stops []string) ([]observationdomain.Record, error) {
	if b.arrivals == nil {
		return nil, fmt.Errorf("%w: live arrivals are not available on this machine", api.ErrTransitUnavailable)
	}
	return b.arrivals.Arrivals(ctx, stops)
}

// ResolveStops turns a stop name into the numbers printed on the poles.
func (b busTransit) ResolveStops(ctx context.Context, text string) ([]string, error) {
	if b.arrivals == nil {
		return nil, fmt.Errorf("%w: the stop directory is not available on this machine", api.ErrTransitUnavailable)
	}

	matched, err := b.arrivals.LookupStops(ctx, text)
	if err != nil {
		return nil, err
	}
	if len(matched) > maxResolvedStops {
		matched = matched[:maxResolvedStops]
	}

	ids := make([]string, 0, len(matched))
	for _, s := range matched {
		ids = append(ids, s.ID)
	}
	return ids, nil
}

// Departures answers from the store rather than from the network.
//
// The timetable is a GTFS archive of the whole AV/LD network; downloading it
// on every request would be absurd, and the daemon already keeps it fresh. So
// this reads what eye holds, soonest first, dropping departures that have
// already left.
func (b busTransit) Departures(ctx context.Context, station string, limit int) ([]observationdomain.Record, error) {
	records, err := b.store.Query(ctx, observationdomain.Filter{
		Kinds: []string{departureKind},
		Text:  station,
	})
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	upcoming := make([]observationdomain.Record, 0, len(records))
	for _, rec := range records {
		if rec.ValidFrom == nil || rec.ValidFrom.Before(now) {
			continue
		}
		upcoming = append(upcoming, rec)
	}

	// Soonest first: a timetable ordered by when eye fetched it is not a
	// timetable, it is a log.
	sort.SliceStable(upcoming, func(i, j int) bool {
		return upcoming[i].ValidFrom.Before(*upcoming[j].ValidFrom)
	})
	return observationdomain.ApplyLimit(upcoming, limit), nil
}

// departureKind is the record kind the GTFS provider emits for a scheduled
// call at a watched station.
const departureKind = "scheduled_departure"

// probeTimeout bounds a health probe. A probe that can hang forever is a
// container that never restarts.
const probeTimeout = 5 * time.Second

// probe asks an eye server whether it is answering.
//
// It exists because a distroless image has no shell and no curl, and shipping
// one to run a health check would undo the reason the image is distroless. The
// binary that serves is the binary that probes.
func probe(ctx context.Context, url string, stdout io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("probe %s: %w", url, err)
	}

	client := &http.Client{Timeout: probeTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("probe %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	// The body is read and capped so the connection can be reused and a
	// misconfigured URL cannot stream a gigabyte into a health check.
	body, _ := io.ReadAll(io.LimitReader(resp.Body, probeBodyLimit))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("probe %s: %s", url, resp.Status)
	}
	_, _ = fmt.Fprintf(stdout, "%s %s\n", resp.Status, strings.TrimSpace(string(body)))
	return nil
}

// probeBodyLimit caps what a probe will read back.
const probeBodyLimit = 4 << 10
