package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/FullFran/eye/configs"
	"github.com/FullFran/eye/internal/config"
	"github.com/FullFran/eye/internal/httpx"
	observation "github.com/FullFran/eye/internal/observation/application"
	observationdomain "github.com/FullFran/eye/internal/observation/domain"
	store "github.com/FullFran/eye/internal/observation/infrastructure"
	provider "github.com/FullFran/eye/internal/provider/domain"
	providers "github.com/FullFran/eye/internal/provider/infrastructure"
	source "github.com/FullFran/eye/internal/source/domain"
	registry "github.com/FullFran/eye/internal/source/infrastructure"
)

// runtime is everything a command needs to answer a question: the resolved
// configuration, the source registry, a store and an HTTP client.
type runtime struct {
	cfg     config.Config
	sources []source.Source
	store   *store.SQLiteStore
	cache   *store.RawCache
	client  *httpx.Client

	// registryPath describes where the registry came from, so the user can
	// tell whether they are running against their own file or the one
	// compiled into the binary.
	registryPath string
}

// runtimeOptions are the per-invocation overrides a command may pass.
type runtimeOptions struct {
	// registry is an explicit sources.yaml path.
	registry string
	// dataDir overrides where the store and raw cache live. Tests set it
	// so a test run can never write into the operator's real store.
	dataDir string
}

// newRuntime resolves configuration, loads the registry and opens the store.
//
// The caller must Close it. Everything a command collects is persisted, so a
// later --offline query can answer without touching the network.
func newRuntime(opts runtimeOptions) (*runtime, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if opts.dataDir != "" {
		cfg.DataDir = opts.dataDir
	}
	sourcesOverride := opts.registry

	path, data, err := resolveRegistry(cfg, sourcesOverride)
	if err != nil {
		return nil, err
	}

	sources, err := registry.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	db, err := store.OpenSQLite(cfg.StorePath())
	if err != nil {
		return nil, err
	}

	cache := store.NewRawCache(cfg.RawCachePath())

	// NewWithOptions rather than New: a bad EYE_EXTRA_CA_FILE has to stop the
	// command with the path in the message. Silently falling back to the
	// system pool would turn a typo into a source that fails at TLS for a
	// reason nobody would think to look for.
	client, err := httpx.NewWithOptions(
		httpx.WithRecorder(cache),
		httpx.WithExtraCAFile(cfg.ExtraCAFile),
	)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	return &runtime{
		cfg:          cfg,
		sources:      sources,
		store:        db,
		cache:        cache,
		client:       client,
		registryPath: path,
	}, nil
}

// Close releases the store.
func (r *runtime) Close() error { return r.store.Close() }

// resolveRegistry applies the lookup order: an explicit override, the user's
// config directory, a registry in the working tree, then the embedded default.
func resolveRegistry(cfg config.Config, override string) (path string, data []byte, err error) {
	candidates := []string{override, cfg.SourcesPath, filepath.Join("configs", "sources.yaml")}

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		body, readErr := os.ReadFile(candidate) // #nosec G304 -- operator-supplied registry path, never request input
		if readErr == nil {
			return candidate, body, nil
		}
		if override != "" && candidate == override {
			return "", nil, fmt.Errorf("read registry %s: %w", override, readErr)
		}
	}
	return "(embedded)", configs.DefaultSources, nil
}

// pollable returns the adapters eye may and can run, plus the enabled sources
// this build has no adapter for.
func (r *runtime) pollable(topics ...string) ([]provider.Provider, []source.Source) {
	selected := r.sources
	if len(topics) > 0 {
		selected = filterByTopic(r.sources, topics)
	}
	return providers.BuildPollable(selected, r.client, providers.Options{
		AllowPersonal: r.cfg.AllowPersonalSources,
		// The two sources that need a credential get it from the machine,
		// never from the registry. An absent key does not hide the
		// source; the adapter reports it by name on the first poll.
		AEMETAPIKey: r.cfg.AEMETAPIKey,
		FIRMSMapKey: r.cfg.FIRMSMapKey,
	})
}

// collect polls the given adapters into the runtime store and records what
// each source did.
//
// Health is persisted here rather than only in the daemon, so that whichever
// command last polled a source, `eye sources` can say when it last worked.
func (r *runtime) collect(ctx context.Context, ps []provider.Provider) []observation.Result {
	collector := observation.NewCollector(r.store, r.store)
	// Change detection compares this poll against the previous one. Every
	// command that polls feeds it, so whichever one ran last, the next knows
	// what moved.
	collector.Snapshots = r.store

	results := collector.Collect(ctx, ps)
	r.saveStates(ctx, results)
	return results
}

// saveStates persists the outcome of a collection pass.
func (r *runtime) saveStates(ctx context.Context, results []observation.Result) {
	for _, res := range results {
		st := store.SourceState{
			SourceID:    res.Source,
			LastAttempt: res.Health.LastAttempt,
			Records:     res.Health.Records,
		}
		if res.OK() {
			st.LastSuccess = res.Health.LastSuccess
		} else {
			st.ConsecutiveErrors = 1
			if res.Err != nil {
				st.LastError = res.Err.Error()
			}
		}
		// A failure to record health must not fail the command that was
		// actually asked for.
		_ = r.store.SaveState(ctx, st)
	}
}

// query reads back from the store.
func (r *runtime) query(ctx context.Context, f observationdomain.Filter) ([]observationdomain.Record, error) {
	return r.store.Query(ctx, f)
}

// prune enforces retention. Expired movement records are deleted because a
// DELETE runs, not because a document says they should be.
func (r *runtime) prune(ctx context.Context) (int, error) {
	return r.store.Prune(ctx, time.Now().UTC())
}

// filterByTopic narrows a registry to the given topics.
func filterByTopic(sources []source.Source, topics []string) []source.Source {
	want := make(map[string]bool, len(topics))
	for _, t := range topics {
		want[t] = true
	}

	out := make([]source.Source, 0, len(sources))
	for _, s := range sources {
		if want[s.Topic] {
			out = append(out, s)
		}
	}
	return out
}
