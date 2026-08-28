package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

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
	store   *store.MemStore
	client  *httpx.Client

	// registryPath describes where the registry came from, so the user can
	// tell whether they are running against their own file or the one
	// compiled into the binary.
	registryPath string
}

// newRuntime resolves configuration and loads the registry.
func newRuntime(sourcesOverride string) (*runtime, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	path, data, err := resolveRegistry(cfg, sourcesOverride)
	if err != nil {
		return nil, err
	}

	sources, err := registry.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	return &runtime{
		cfg:          cfg,
		sources:      sources,
		store:        store.NewMemStore(),
		client:       httpx.New(),
		registryPath: path,
	}, nil
}

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
	return providers.BuildPollable(selected, r.client)
}

// collect polls the given adapters into the runtime store.
func (r *runtime) collect(ctx context.Context, ps []provider.Provider) []observation.Result {
	return observation.NewCollector(r.store, r.store).Collect(ctx, ps)
}

// query reads back from the runtime store.
func (r *runtime) query(ctx context.Context, f observationdomain.Filter) ([]observationdomain.Record, error) {
	return r.store.Query(ctx, f)
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
