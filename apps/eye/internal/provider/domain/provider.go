// Package domain defines the ports every source adapter implements. Adapters
// live in internal/provider/infrastructure/<source>/ and are the only place in
// eye that knows about DATEX II, GTFS, CKAN or ICS.
package domain

import (
	"context"
	"time"

	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// Provider is a pull adapter: eye asks, the source answers. This covers the
// large majority of public feeds, whose publication cadence is measured in
// minutes or hours.
type Provider interface {
	// Info describes the source and its polling contract.
	Info() source.Source

	// Poll fetches the current state and normalizes it into records. It
	// must not retry internally; backoff belongs to the scheduler.
	Poll(ctx context.Context) ([]observation.Record, error)
}

// StreamingProvider is a push adapter for sources that genuinely stream, such
// as GTFS-RT vehicle positions or an ADS-B feed. Forcing these to look like a
// CKAN catalog that changes once a week helps nobody.
type StreamingProvider interface {
	Provider

	// Stream runs until ctx is cancelled, emitting records as they arrive.
	Stream(ctx context.Context, out chan<- observation.Record) error
}

// EntityProvider is implemented by adapters that also publish an inventory of
// persistent things, such as the position of every DGT camera.
type EntityProvider interface {
	Provider

	// Entities fetches the current inventory.
	Entities(ctx context.Context) ([]observation.Entity, error)
}

// Health is the scheduler's view of one provider.
type Health struct {
	SourceID string `json:"source_id"`

	LastAttempt time.Time `json:"last_attempt"`
	LastSuccess time.Time `json:"last_success"`

	// ConsecutiveErrors drives backoff and the circuit breaker.
	ConsecutiveErrors int `json:"consecutive_errors"`
	// LastError is the most recent failure message, if any.
	LastError string `json:"last_error,omitempty"`

	// Records is how many records the last successful poll produced.
	Records int `json:"records"`
}

// Stale reports whether the last success is older than the tolerance allowed
// for this source. A stale source must be shown as stale, never silently
// rendered as if it were live.
func (h Health) Stale(now time.Time, tolerance time.Duration) bool {
	if h.LastSuccess.IsZero() {
		return true
	}
	return now.Sub(h.LastSuccess) > tolerance
}
