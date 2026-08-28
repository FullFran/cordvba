// Package application holds the observation use cases: collecting from
// providers and querying what was collected. It depends only on domain
// interfaces, and knows nothing about HTTP, YAML or terminals.
package application

import (
	"context"
	"sync"
	"time"

	domain "github.com/FullFran/eye/internal/observation/domain"
	provider "github.com/FullFran/eye/internal/provider/domain"
)

// DefaultConcurrency bounds how many sources are polled at once. Public
// administrations are not a load-testing target.
const DefaultConcurrency = 6

// Collector polls providers and writes what they return into the stores.
type Collector struct {
	records  domain.RecordStore
	entities domain.EntityStore

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

	if stored, err := c.records.Append(ctx, records); err == nil {
		res.Records = stored
	} else {
		res.Err = err
		return res
	}

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
