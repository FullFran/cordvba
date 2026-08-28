// Package scheduler runs providers on their declared intervals without turning
// eye into a rate-limit benchmark.
//
// The policy in this file is pure and deterministic: backoff, jitter and the
// circuit breaker are functions of state, not of the clock. The loop that uses
// them lives in scheduler.go. Keeping them apart is what makes the interesting
// behaviour testable without a single sleep.
package scheduler

import (
	"math"
	"time"
)

// Backoff computes how long to wait after consecutive failures.
type Backoff struct {
	// Base is the delay after the first failure.
	Base time.Duration
	// Max caps the delay however long the outage lasts.
	Max time.Duration
	// Factor is the multiplier per additional failure.
	Factor float64
}

// DefaultBackoff is deliberately patient. A public administration having a bad
// morning is not something to hammer through.
var DefaultBackoff = Backoff{Base: 30 * time.Second, Max: 30 * time.Minute, Factor: 2}

// Next returns the delay for a given number of consecutive errors. Zero errors
// means no backoff: the caller uses the source's own interval.
func (b Backoff) Next(consecutiveErrors int) time.Duration {
	if consecutiveErrors <= 0 {
		return 0
	}

	base, maxDelay, factor := b.Base, b.Max, b.Factor
	if base <= 0 {
		base = DefaultBackoff.Base
	}
	if maxDelay <= 0 {
		maxDelay = DefaultBackoff.Max
	}
	if factor < 1 {
		factor = DefaultBackoff.Factor
	}

	// Cap the exponent before it is applied. math.Pow on a large exponent
	// overflows to +Inf, and +Inf nanoseconds is a negative duration.
	exponent := math.Min(float64(consecutiveErrors-1), 32)
	delay := float64(base) * math.Pow(factor, exponent)

	if delay > float64(maxDelay) || math.IsInf(delay, 1) {
		return maxDelay
	}
	return time.Duration(delay)
}

// Jitter spreads a delay by up to fraction, so twenty providers registered at
// once do not all fire on the same second forever after.
//
// randomValue is in [0,1). It is a parameter rather than a package-level source
// so that the function stays pure and testable.
func Jitter(d time.Duration, fraction float64, randomValue float64) time.Duration {
	if d <= 0 || fraction <= 0 {
		return d
	}
	if fraction > 1 {
		fraction = 1
	}

	// Spread symmetrically around d: [d-f·d, d+f·d].
	offset := (randomValue*2 - 1) * fraction * float64(d)
	jittered := time.Duration(float64(d) + offset)

	if jittered < 0 {
		return 0
	}
	return jittered
}

// BreakerState is where a source sits in the circuit breaker.
type BreakerState string

// The three breaker states.
const (
	// BreakerClosed is normal operation.
	BreakerClosed BreakerState = "closed"
	// BreakerOpen means the source is failing and is not being called.
	BreakerOpen BreakerState = "open"
	// BreakerHalfOpen means the cooldown has elapsed and one probe is
	// allowed through.
	BreakerHalfOpen BreakerState = "half-open"
)

// Breaker stops eye from calling a source that is consistently failing.
type Breaker struct {
	// Threshold is how many consecutive errors trip it.
	Threshold int
	// Cooldown is how long it stays open before allowing a probe.
	Cooldown time.Duration
}

// DefaultBreaker trips after five consecutive failures and probes every ten
// minutes.
var DefaultBreaker = Breaker{Threshold: 5, Cooldown: 10 * time.Minute}

// State reports where a source sits, given its failure count and when it was
// last attempted.
func (b Breaker) State(consecutiveErrors int, lastAttempt, now time.Time) BreakerState {
	threshold := b.Threshold
	if threshold <= 0 {
		threshold = DefaultBreaker.Threshold
	}
	if consecutiveErrors < threshold {
		return BreakerClosed
	}

	cooldown := b.Cooldown
	if cooldown <= 0 {
		cooldown = DefaultBreaker.Cooldown
	}
	if lastAttempt.IsZero() || now.Sub(lastAttempt) >= cooldown {
		return BreakerHalfOpen
	}
	return BreakerOpen
}

// Allow reports whether a call may go out.
func (b Breaker) Allow(consecutiveErrors int, lastAttempt, now time.Time) bool {
	return b.State(consecutiveErrors, lastAttempt, now) != BreakerOpen
}
