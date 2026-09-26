package application_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	application "github.com/FullFran/cordvba/apps/eye/internal/observation/application"
	domain "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	store "github.com/FullFran/cordvba/apps/eye/internal/observation/infrastructure"
	provider "github.com/FullFran/cordvba/apps/eye/internal/provider/domain"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// fakeProvider is a provider under test control.
type fakeProvider struct {
	id        string
	topic     string
	retention source.Retention
	records   []domain.Record
	entities  []domain.Entity
	err       error
	delay     time.Duration
}

func (f *fakeProvider) Info() source.Source {
	return source.Source{ID: f.id, Topic: f.topic, Format: "fake", Retention: f.retention}
}

func (f *fakeProvider) Poll(ctx context.Context) ([]domain.Record, error) {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.records, f.err
}

// fakeEntityProvider also publishes an inventory.
type fakeEntityProvider struct{ fakeProvider }

func (f *fakeEntityProvider) Entities(context.Context) ([]domain.Entity, error) {
	return f.entities, nil
}

// sampleRecord builds a valid record.
func sampleRecord(id string) domain.Record {
	now := time.Now().UTC()
	return domain.Record{
		ID: id, Source: "fake", Kind: "news_item", Topic: "press",
		ObservedAt: now, FetchedAt: now, Title: "t", Quality: domain.QualityOfficial, Confidence: 1,
		Provenance: domain.Provenance{
			Publisher: "P", SourceURL: "https://example.org", License: "unspecified", FetchedAt: now,
		},
	}
}

func TestCollectStoresRecords(t *testing.T) {
	t.Parallel()

	s := store.NewMemStore()
	c := application.NewCollector(s, s)

	results := c.Collect(context.Background(), []provider.Provider{
		&fakeProvider{id: "a", topic: "press", records: []domain.Record{sampleRecord("1"), sampleRecord("2")}},
	})

	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if !results[0].OK() {
		t.Fatalf("result error: %v", results[0].Err)
	}
	if results[0].Records != 2 {
		t.Errorf("stored = %d, want 2", results[0].Records)
	}
	if results[0].Health.LastSuccess.IsZero() {
		t.Error("health has no LastSuccess after a successful poll")
	}
}

// One public service being down is an ordinary Tuesday. It must not take the
// rest of the collection with it.
func TestCollectIsolatesFailures(t *testing.T) {
	t.Parallel()

	s := store.NewMemStore()
	c := application.NewCollector(s, s)

	results := c.Collect(context.Background(), []provider.Provider{
		&fakeProvider{id: "broken", topic: "press", err: errors.New("503 from upstream")},
		&fakeProvider{id: "healthy", topic: "press", records: []domain.Record{sampleRecord("1")}},
	})

	if results[0].OK() {
		t.Error("the broken source should report an error")
	}
	if results[0].Health.ConsecutiveErrors != 1 {
		t.Errorf("ConsecutiveErrors = %d, want 1", results[0].Health.ConsecutiveErrors)
	}
	if results[0].Health.LastError == "" {
		t.Error("health should carry the error message")
	}
	if !results[1].OK() || results[1].Records != 1 {
		t.Errorf("the healthy source should still have been collected: %+v", results[1])
	}
}

func TestCollectStoresEntities(t *testing.T) {
	t.Parallel()

	s := store.NewMemStore()
	c := application.NewCollector(s, s)

	now := time.Now().UTC()
	ep := &fakeEntityProvider{fakeProvider{
		id: "inv", topic: "transport",
		entities: []domain.Entity{{
			ID: "cam-1", Source: "inv", Kind: "camera", Topic: "transport",
			Title: "Puente", FirstSeen: now, LastSeen: now,
			Provenance: domain.Provenance{
				Publisher: "P", SourceURL: "https://example.org", License: "unspecified", FetchedAt: now,
			},
		}},
	}}

	results := c.Collect(context.Background(), []provider.Provider{ep})
	if results[0].Entities != 1 {
		t.Errorf("entities = %d, want 1", results[0].Entities)
	}

	_, entities := s.Len()
	if entities != 1 {
		t.Errorf("store holds %d entities, want 1", entities)
	}
}

func TestCollectRespectsCancellation(t *testing.T) {
	t.Parallel()

	s := store.NewMemStore()
	c := application.NewCollector(s, s)
	c.Concurrency = 1

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results := c.Collect(ctx, []provider.Provider{
		&fakeProvider{id: "slow", topic: "press", delay: time.Minute},
		&fakeProvider{id: "slower", topic: "press", delay: time.Minute},
	})

	for _, r := range results {
		if r.OK() {
			t.Errorf("%s succeeded despite a cancelled context", r.Source)
		}
	}
}

func TestCollectHandlesNoProviders(t *testing.T) {
	t.Parallel()

	s := store.NewMemStore()
	if got := application.NewCollector(s, s).Collect(context.Background(), nil); len(got) != 0 {
		t.Errorf("Collect(nil) = %d results, want 0", len(got))
	}
}

// sampleRecordWithExpiry builds a valid record carrying an adapter-computed
// ExpiresAt, the shape every provider's Poll returns today.
func sampleRecordWithExpiry(id string, expires time.Time) domain.Record {
	r := sampleRecord(id)
	r.ExpiresAt = &expires
	return r
}

// A historical source is an environmental series eye cannot re-fetch once the
// window has passed: the adapter may still compute an ExpiresAt (every current
// adapter does), but the collector — the one seam every source's records pass
// through before they reach the store — must clear it before persisting.
func TestCollectClearsExpiryForHistoricalSources(t *testing.T) {
	t.Parallel()

	s := store.NewMemStore()
	c := application.NewCollector(s, s)

	expires := time.Now().UTC().Add(30 * 24 * time.Hour)
	results := c.Collect(context.Background(), []provider.Provider{
		&fakeProvider{
			id: "aemet-observation", topic: "weather", retention: source.RetentionHistorical,
			records: []domain.Record{sampleRecordWithExpiry("1", expires)},
		},
	})
	if !results[0].OK() {
		t.Fatalf("result error: %v", results[0].Err)
	}

	got, err := s.Query(context.Background(), domain.Filter{})
	if err != nil {
		t.Fatalf("Query() = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("stored = %d records, want 1", len(got))
	}
	if got[0].ExpiresAt != nil {
		t.Errorf("ExpiresAt = %v, want nil for a historical source", got[0].ExpiresAt)
	}
}

// An ephemeral source (the default, and every source that never mentions
// retention) keeps behaving exactly as it always has: the adapter's own TTL
// survives untouched.
func TestCollectKeepsExpiryForEphemeralSources(t *testing.T) {
	t.Parallel()

	s := store.NewMemStore()
	c := application.NewCollector(s, s)

	expires := time.Now().UTC().Add(24 * time.Hour)
	results := c.Collect(context.Background(), []provider.Provider{
		&fakeProvider{
			id: "adsb-lol", topic: "aviation",
			records: []domain.Record{sampleRecordWithExpiry("1", expires)},
		},
	})
	if !results[0].OK() {
		t.Fatalf("result error: %v", results[0].Err)
	}

	got, err := s.Query(context.Background(), domain.Filter{})
	if err != nil {
		t.Fatalf("Query() = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("stored = %d records, want 1", len(got))
	}
	if got[0].ExpiresAt == nil || !got[0].ExpiresAt.Equal(expires) {
		t.Errorf("ExpiresAt = %v, want %v unchanged for an ephemeral source", got[0].ExpiresAt, expires)
	}
}

// This is the end-to-end proof #73 asks for: a historical source's records
// carry a nil ExpiresAt all the way into the durable store, and the daemon's
// own prune mechanism (SQLiteStore.Prune) never touches them — even when the
// adapter computed an ExpiresAt already in the past. An ephemeral source
// polled alongside it is pruned exactly as before: nothing about #73 changes
// what already worked.
func TestHistoricalRecordsSurvivePruneAlongsideEphemeralOnes(t *testing.T) {
	t.Parallel()

	s, err := store.OpenSQLite(filepath.Join(t.TempDir(), "eye.db"))
	if err != nil {
		t.Fatalf("OpenSQLite() = %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	c := application.NewCollector(s, s)
	now := time.Now().UTC()
	longPastExpiry := now.Add(-72 * time.Hour) // what a 30-day TTL adapter would have computed weeks ago

	results := c.Collect(context.Background(), []provider.Provider{
		&fakeProvider{
			id: "aemet-observation", topic: "weather", retention: source.RetentionHistorical,
			records: []domain.Record{sampleRecordWithExpiry("historical-1", longPastExpiry)},
		},
		&fakeProvider{
			id: "adsb-lol", topic: "aviation",
			records: []domain.Record{sampleRecordWithExpiry("ephemeral-1", longPastExpiry)},
		},
	})
	for _, r := range results {
		if !r.OK() {
			t.Fatalf("%s: %v", r.Source, r.Err)
		}
	}

	pruned, err := s.Prune(context.Background(), now)
	if err != nil {
		t.Fatalf("Prune() = %v", err)
	}
	if pruned != 1 {
		t.Errorf("pruned = %d, want 1 (only the ephemeral record)", pruned)
	}

	got, err := s.Query(context.Background(), domain.Filter{})
	if err != nil {
		t.Fatalf("Query() = %v", err)
	}
	if len(got) != 1 || got[0].ID != "historical-1" {
		t.Fatalf("after prune the store holds %+v, want only historical-1", got)
	}
}

// #125: an operator's overrides.yaml can pin a source's retention to
// "historical", even though the registry itself says ephemeral. The
// collector is the single seam every source's records pass through — see
// applyRetention — so this is where the composition (registry ∘ overrides)
// has to happen, exactly once, for every source there is or ever will be.
func TestCollectRetentionOverrideHistoricalWinsOverRegistryEphemeral(t *testing.T) {
	t.Parallel()

	s := store.NewMemStore()
	c := application.NewCollector(s, s)
	c.Overrides = source.Overrides{Retention: map[string]source.RetentionOverride{
		"metar-cordoba": {Kind: source.RetentionHistorical},
	}}

	expires := time.Now().UTC().Add(24 * time.Hour)
	results := c.Collect(context.Background(), []provider.Provider{
		&fakeProvider{
			id: "metar-cordoba", topic: "weather", // ephemeral in the registry
			records: []domain.Record{sampleRecordWithExpiry("1", expires)},
		},
	})
	if !results[0].OK() {
		t.Fatalf("result error: %v", results[0].Err)
	}

	got, err := s.Query(context.Background(), domain.Filter{})
	if err != nil {
		t.Fatalf("Query() = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("stored = %d records, want 1", len(got))
	}
	if got[0].ExpiresAt != nil {
		t.Errorf("ExpiresAt = %v, want nil once the override pins this source to historical", got[0].ExpiresAt)
	}
}

// The opposite composition: a historical registry entry, pinned back to a
// specific ephemeral TTL by the operator. The override's TTL must replace
// whatever the adapter itself computed, not merely coexist with it.
func TestCollectRetentionOverrideEphemeralReplacesAdapterTTL(t *testing.T) {
	t.Parallel()

	s := store.NewMemStore()
	c := application.NewCollector(s, s)
	c.Overrides = source.Overrides{Retention: map[string]source.RetentionOverride{
		"renfe-positions": {Kind: source.RetentionEphemeral, TTL: 24 * time.Hour},
	}}
	c.Now = func() time.Time { return time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC) }

	adapterComputedExpiry := time.Now().UTC().Add(30 * 24 * time.Hour)
	rec := sampleRecordWithExpiry("1", adapterComputedExpiry)
	rec.FetchedAt = c.Now()

	results := c.Collect(context.Background(), []provider.Provider{
		&fakeProvider{
			id: "renfe-positions", topic: "transport", retention: source.RetentionHistorical,
			records: []domain.Record{rec},
		},
	})
	if !results[0].OK() {
		t.Fatalf("result error: %v", results[0].Err)
	}

	got, err := s.Query(context.Background(), domain.Filter{})
	if err != nil {
		t.Fatalf("Query() = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("stored = %d records, want 1", len(got))
	}
	want := rec.FetchedAt.Add(24 * time.Hour)
	if got[0].ExpiresAt == nil || !got[0].ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %v, want %v (the override's TTL from FetchedAt)", got[0].ExpiresAt, want)
	}
}

// A source with no matching entry in Overrides.Retention keeps behaving
// exactly as it did before #125: the registry's own retention decides.
func TestCollectRetentionWithoutOverrideUnaffected(t *testing.T) {
	t.Parallel()

	s := store.NewMemStore()
	c := application.NewCollector(s, s)
	c.Overrides = source.Overrides{Retention: map[string]source.RetentionOverride{
		"some-other-source": {Kind: source.RetentionHistorical},
	}}

	expires := time.Now().UTC().Add(24 * time.Hour)
	results := c.Collect(context.Background(), []provider.Provider{
		&fakeProvider{
			id: "adsb-lol", topic: "aviation",
			records: []domain.Record{sampleRecordWithExpiry("1", expires)},
		},
	})
	if !results[0].OK() {
		t.Fatalf("result error: %v", results[0].Err)
	}

	got, err := s.Query(context.Background(), domain.Filter{})
	if err != nil {
		t.Fatalf("Query() = %v", err)
	}
	if len(got) != 1 || got[0].ExpiresAt == nil || !got[0].ExpiresAt.Equal(expires) {
		t.Errorf("an unrelated override entry must not touch adsb-lol's ExpiresAt: got %+v", got)
	}
}
