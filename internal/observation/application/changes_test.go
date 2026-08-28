package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	application "github.com/FullFran/eye/internal/observation/application"
	domain "github.com/FullFran/eye/internal/observation/domain"
	store "github.com/FullFran/eye/internal/observation/infrastructure"
	provider "github.com/FullFran/eye/internal/provider/domain"
	source "github.com/FullFran/eye/internal/source/domain"
)

var tick = time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

// scripted is a provider that returns whatever the test sets next.
type scripted struct {
	src     source.Source
	records []domain.Record
	err     error
}

func (p *scripted) Info() source.Source { return p.src }

func (p *scripted) Poll(context.Context) ([]domain.Record, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.records, nil
}

// observation builds one record as a source would report it.
func observation(title string, severity domain.Severity) domain.Record {
	return domain.Record{
		ID: "feed:" + title, Source: "feed", Kind: "event", Topic: "events",
		ObservedAt: tick, FetchedAt: tick,
		Title: title, Severity: severity, Confidence: 1,
		Quality: domain.QualityOfficial,
		Provenance: domain.Provenance{
			Publisher: "IMAE", SourceURL: "https://example.test",
			License: "unspecified", FetchedAt: tick, RawHash: "abc",
		},
	}
}

// collectorOver builds a collector with change detection wired up.
func collectorOver(t *testing.T) (*application.Collector, *store.MemStore) {
	t.Helper()

	mem := store.NewMemStore()
	c := application.NewCollector(mem, mem)
	c.Snapshots = mem
	c.Now = func() time.Time { return tick }
	return c, mem
}

// storedChanges reads the change records back out.
func storedChanges(t *testing.T, mem *store.MemStore) []domain.Change {
	t.Helper()

	records, err := mem.Query(context.Background(), domain.Filter{Kinds: []string{domain.ChangeKindRecord}})
	if err != nil {
		t.Fatal(err)
	}

	out := make([]domain.Change, 0, len(records))
	for _, r := range records {
		var c domain.Change
		if err := json.Unmarshal(r.Payload, &c); err != nil {
			t.Fatalf("change payload: %v", err)
		}
		out = append(out, c)
	}
	return out
}

// A fresh install has nothing to compare against. Announcing every record in
// Cordoba as new would be both wrong and useless.
func TestCollectSaysNothingChangedOnTheFirstEverPoll(t *testing.T) {
	t.Parallel()

	c, mem := collectorOver(t)
	p := &scripted{
		src:     source.Source{ID: "feed", Topic: "events"},
		records: []domain.Record{observation("Concierto A", domain.SeverityInfo)},
	}

	c.Collect(context.Background(), []provider.Provider{p})

	if changes := storedChanges(t, mem); len(changes) != 0 {
		t.Errorf("first poll produced %d changes:\n%+v", len(changes), changes)
	}
}

func TestCollectReportsWhatArrivedAfterTheFirstPoll(t *testing.T) {
	t.Parallel()

	c, mem := collectorOver(t)
	p := &scripted{
		src:     source.Source{ID: "feed", Topic: "events"},
		records: []domain.Record{observation("Concierto A", domain.SeverityInfo)},
	}
	c.Collect(context.Background(), []provider.Provider{p})

	p.records = append(p.records, observation("Concierto B", domain.SeverityInfo))
	c.Collect(context.Background(), []provider.Provider{p})

	changes := storedChanges(t, mem)
	if len(changes) != 1 {
		t.Fatalf("changes = %d, want 1:\n%+v", len(changes), changes)
	}
	if changes[0].Kind != domain.ChangeAppeared {
		t.Errorf("kind = %q, want appeared", changes[0].Kind)
	}
}

// The point of the whole feature: a source going quiet is not an ending.
func TestCollectDoesNotTurnAFailedPollIntoDisappearances(t *testing.T) {
	t.Parallel()

	c, mem := collectorOver(t)
	p := &scripted{
		src:     source.Source{ID: "feed", Topic: "events"},
		records: []domain.Record{observation("Concierto A", domain.SeverityInfo)},
	}
	c.Collect(context.Background(), []provider.Provider{p})

	// The feed breaks.
	p.err = errors.New("connection refused")
	c.Collect(context.Background(), []provider.Provider{p})

	if changes := storedChanges(t, mem); len(changes) != 0 {
		t.Errorf("a broken feed produced %d changes:\n%+v", len(changes), changes)
	}

	// And when it recovers unchanged, still nothing happened. The snapshot
	// must have survived the failure, or the recovery would read as the
	// whole feed appearing from nothing.
	p.err = nil
	c.Collect(context.Background(), []provider.Provider{p})

	if changes := storedChanges(t, mem); len(changes) != 0 {
		t.Errorf("recovery produced %d changes:\n%+v", len(changes), changes)
	}
}

func TestCollectReportsADisappearanceFromAWorkingSource(t *testing.T) {
	t.Parallel()

	c, mem := collectorOver(t)
	p := &scripted{
		src: source.Source{ID: "feed", Topic: "events"},
		records: []domain.Record{
			observation("Concierto A", domain.SeverityInfo),
			observation("Concierto B", domain.SeverityInfo),
		},
	}
	c.Collect(context.Background(), []provider.Provider{p})

	p.records = p.records[:1]
	c.Collect(context.Background(), []provider.Provider{p})

	changes := storedChanges(t, mem)
	if len(changes) != 1 {
		t.Fatalf("changes = %d, want 1:\n%+v", len(changes), changes)
	}
	if changes[0].Kind != domain.ChangeDisappeared {
		t.Errorf("kind = %q, want disappeared", changes[0].Kind)
	}
}

func TestCollectReportsAFieldMoving(t *testing.T) {
	t.Parallel()

	c, mem := collectorOver(t)
	p := &scripted{
		src:     source.Source{ID: "feed", Topic: "events"},
		records: []domain.Record{observation("Concierto A", domain.SeverityInfo)},
	}
	c.Collect(context.Background(), []provider.Provider{p})

	p.records = []domain.Record{observation("Concierto A", domain.SeverityHigh)}
	c.Collect(context.Background(), []provider.Provider{p})

	changes := storedChanges(t, mem)
	if len(changes) != 1 {
		t.Fatalf("changes = %d, want 1:\n%+v", len(changes), changes)
	}
	if changes[0].Kind != domain.ChangeUpdated {
		t.Fatalf("kind = %q, want updated", changes[0].Kind)
	}
	if len(changes[0].Fields) == 0 || changes[0].Fields[0].Field != "severity" {
		t.Errorf("fields = %+v, want the severity move", changes[0].Fields)
	}
}

// Change detection is optional wiring. A collector without it must still
// collect, rather than panic on a nil store.
func TestCollectWithoutChangeDetection(t *testing.T) {
	t.Parallel()

	mem := store.NewMemStore()
	c := application.NewCollector(mem, mem)
	c.Now = func() time.Time { return tick }

	p := &scripted{
		src:     source.Source{ID: "feed", Topic: "events"},
		records: []domain.Record{observation("Concierto A", domain.SeverityInfo)},
	}
	results := c.Collect(context.Background(), []provider.Provider{p})

	if len(results) != 1 || !results[0].OK() {
		t.Fatalf("results = %+v", results)
	}
	if changes := storedChanges(t, mem); len(changes) != 0 {
		t.Errorf("changes = %d with no snapshot store", len(changes))
	}
}
