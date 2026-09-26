package application_test

import (
	"context"
	"errors"
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
	id       string
	topic    string
	records  []domain.Record
	entities []domain.Entity
	err      error
	delay    time.Duration
}

func (f *fakeProvider) Info() source.Source {
	return source.Source{ID: f.id, Topic: f.topic, Format: "fake"}
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
