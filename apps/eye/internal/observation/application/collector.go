// Package application holds the observation use cases: collecting from
// providers and querying what was collected. It depends only on domain
// interfaces, and knows nothing about HTTP, YAML or terminals.
package application

import (
	"context"
	"sync"
	"time"

	domain "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	provider "github.com/FullFran/cordvba/apps/eye/internal/provider/domain"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// DefaultConcurrency bounds how many sources are polled at once. Public
// administrations are not a load-testing target.
const DefaultConcurrency = 6

// Collector polls providers and writes what they return into the stores.
type Collector struct {
	records  domain.RecordStore
	entities domain.EntityStore

	// Snapshots enables change detection. It is optional: without it eye
	// still collects, it just cannot say what moved since last time.
	Snapshots domain.SnapshotStore

	// Concurrency bounds parallel polls. Zero means DefaultConcurrency.
	Concurrency int
	// Now is injectable so tests do not depend on the wall clock.
	Now func() time.Time
}

// NewCollector builds a collector over the given stores.
func NewCollector(records domain.RecordStore, entities domain.EntityStore) *Collector {
	return &Collector{records: records, entities: entities, Now: func() time.Time { return time.Now().UTC() }}
}

// Result is the outcome of polling one source.
type Result struct {
	Health   provider.Health
	Source   string
	Topic    string
	Records  int
	Entities int
	Err      error
}

// OK reports whether the poll succeeded.
func (r Result) OK() bool { return r.Err == nil }

// Collect polls every provider and stores what comes back.
//
// One failing source never stops the others: a public service being down is an
// ordinary Tuesday, and the report says so per source rather than aborting.
func (c *Collector) Collect(ctx context.Context, providers []provider.Provider) []Result {
	concurrency := c.Concurrency
	if concurrency <= 0 {
		concurrency = DefaultConcurrency
	}

	results := make([]Result, len(providers))
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, p := range providers {
		wg.Add(1)
		go func(i int, p provider.Provider) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[i] = Result{Source: p.Info().ID, Topic: p.Info().Topic, Err: ctx.Err()}
				return
			}

			results[i] = c.collectOne(ctx, p)
		}(i, p)
	}

	wg.Wait()
	return results
}

// collectOne polls a single provider and persists its output.
func (c *Collector) collectOne(ctx context.Context, p provider.Provider) Result {
	info := p.Info()
	now := c.Now()

	res := Result{
		Source: info.ID,
		Topic:  info.Topic,
		Health: provider.Health{SourceID: info.ID, LastAttempt: now},
	}

	records, err := p.Poll(ctx)
	if err != nil {
		res.Err = err
		res.Health.ConsecutiveErrors = 1
		res.Health.LastError = err.Error()
		return res
	}

	applyRetention(info, records)

	if stored, err := c.records.Append(ctx, records); err == nil {
		res.Records = stored
	} else {
		res.Err = err
		return res
	}

	c.detectChanges(ctx, info, records, now)

	// An inventory provider also refreshes the entity store.
	if ep, ok := p.(provider.EntityProvider); ok {
		if entities, err := ep.Entities(ctx); err == nil {
			if stored, err := c.entities.Upsert(ctx, entities); err == nil {
				res.Entities = stored
			}
		}
	}

	res.Health.LastSuccess = c.Now()
	res.Health.Records = len(records)
	return res
}

// applyRetention overrides ExpiresAt for a historical source's freshly polled
// records, in place.
//
// Every adapter still computes its own ExpiresAt from its own TTL — that code
// is unchanged, on purpose: rewriting three adapters to each ask the registry
// "am I historical?" would mean the same policy re-implemented three times,
// with a fourth mistake waiting the next time a source is added. The
// collector is instead the single seam every source's Poll() result passes
// through before it reaches the store, regardless of adapter, so the
// registry's retention decision is enforced exactly once, here, for every
// source there is or ever will be.
//
// Change records never reach this function: they are produced by
// detectChanges below, through a separate Append call, and keep their own
// fixed 30-day TTL untouched — that belongs to the change-feed ADR, not to a
// source's retention policy.
func applyRetention(info source.Source, records []domain.Record) {
	if !info.Historical() {
		return
	}
	for i := range records {
		records[i].ExpiresAt = nil
	}
}

// detectChanges compares this poll against the previous one and stores what
// moved.
//
// It runs only after a SUCCESSFUL poll, which is what makes a disappearance
// trustworthy: a record missing from a source that answered is a record the
// source no longer reports, while a record missing because the poll failed is
// nothing at all. A failed poll returns before reaching here and leaves the
// previous snapshot untouched, so the next success compares against the last
// state eye actually saw.
//
// Failures here are deliberately silent in the Result. The command the user
// asked for was "collect", and losing this tick's changes must not turn a
// successful poll into a reported failure.
func (c *Collector) detectChanges(ctx context.Context, info source.Source, records []domain.Record, now time.Time) {
	if c.Snapshots == nil {
		return
	}

	previous, known, err := c.Snapshots.Snapshot(ctx, info.ID)
	if err != nil {
		return
	}

	// Samples are dropped before both the comparison and the snapshot: a
	// position is different every time by definition, so diffing two of them
	// reports the reading back as news and buries everything else.
	watched := watchable(info, records)

	// A first sighting has nothing to compare against, so nothing may be
	// claimed to have appeared. The snapshot is still taken, so the next
	// poll has a baseline.
	if known {
		if changes := domain.DetectChanges(previous, watched, now, true); len(changes) > 0 {
			_, _ = c.records.Append(ctx, changes)
		}
	}

	_ = c.Snapshots.SaveSnapshot(ctx, info.ID, watched)
}

// watchable drops the records whose kind this source publishes as samples.
//
// Per kind rather than per source because one feed does both: RENFE's real-time
// feed carries train positions, which are samples, alongside trip delays, which
// are exactly what somebody wants to be told about.
func watchable(info source.Source, records []domain.Record) []domain.Record {
	if len(info.SampledKinds) == 0 {
		return records
	}

	out := make([]domain.Record, 0, len(records))
	for _, r := range records {
		if info.SamplesKind(r.Kind) {
			continue
		}
		out = append(out, r)
	}
	return out
}
