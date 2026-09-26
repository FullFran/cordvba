package cli

import (
	"context"
	"testing"
	"time"

	"github.com/FullFran/cordvba/apps/eye/internal/logging"
	domain "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// fakeSinkProvider is a minimal provider.Provider for exercising daemonSink
// directly, without a scheduler loop or a live source.
type fakeSinkProvider struct {
	info    source.Source
	records []domain.Record
}

func (f fakeSinkProvider) Info() source.Source { return f.info }

func (f fakeSinkProvider) Poll(context.Context) ([]domain.Record, error) { return f.records, nil }

// #132: the daemon's own scheduler loop persists through daemonSink.Store,
// which is a different code path than Collector.collectOne — the seam #73
// assumed was the only one. Before the fix this test proves, a historical
// source's records reached the store through daemonSink.Store still carrying
// the adapter's own ExpiresAt.
func TestDaemonSinkStoreAppliesSourceRetention(t *testing.T) {
	t.Parallel()

	rt, err := newRuntime(runtimeOptions{registry: minimalRegistry(t), dataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("newRuntime() = %v", err)
	}
	defer func() { _ = rt.Close() }()

	sink := newDaemonSink(rt, logging.Discard())

	staleExpiry := time.Now().UTC().Add(30 * 24 * time.Hour)
	rec := testRecord("historical-1", "", &staleExpiry)
	p := fakeSinkProvider{
		info: source.Source{
			ID: "metar-cordoba", Topic: "weather", Format: "fake",
			Retention: source.RetentionHistorical,
		},
		records: []domain.Record{rec},
	}

	if _, err := sink.Store(context.Background(), p, p.records); err != nil {
		t.Fatalf("Store() = %v", err)
	}

	got, err := rt.store.Query(context.Background(), domain.Filter{})
	if err != nil {
		t.Fatalf("Query() = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("stored = %d records, want 1", len(got))
	}
	if got[0].ExpiresAt != nil {
		t.Errorf("ExpiresAt = %v, want nil: the daemon's own store path must apply retention exactly like the collector", got[0].ExpiresAt)
	}
}

// The composed retention flows through the daemon path too: an operator's
// overrides.yaml (#125) pinning a registry-ephemeral source to historical
// must be honoured here, not only in the collector.
func TestDaemonSinkStoreAppliesOverriddenRetention(t *testing.T) {
	t.Parallel()

	rt, err := newRuntime(runtimeOptions{registry: minimalRegistry(t), dataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("newRuntime() = %v", err)
	}
	defer func() { _ = rt.Close() }()
	rt.overrides = source.Overrides{Retention: map[string]source.RetentionOverride{
		"renfe-positions": {Kind: source.RetentionHistorical},
	}}

	sink := newDaemonSink(rt, logging.Discard())

	staleExpiry := time.Now().UTC().Add(24 * time.Hour)
	rec := testRecord("override-1", "", &staleExpiry)
	p := fakeSinkProvider{
		info:    source.Source{ID: "renfe-positions", Topic: "transport", Format: "fake"}, // ephemeral in the registry
		records: []domain.Record{rec},
	}

	if _, err := sink.Store(context.Background(), p, p.records); err != nil {
		t.Fatalf("Store() = %v", err)
	}

	got, err := rt.store.Query(context.Background(), domain.Filter{})
	if err != nil {
		t.Fatalf("Query() = %v", err)
	}
	if len(got) != 1 || got[0].ExpiresAt != nil {
		t.Errorf("got %+v, want one record with ExpiresAt nil once the override pins it to historical", got)
	}
}
