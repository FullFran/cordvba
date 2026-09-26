package scheduler_test

import (
	"testing"
	"time"

	"github.com/FullFran/eye/internal/scheduler"
)

func TestBackoffNext(t *testing.T) {
	t.Parallel()

	b := scheduler.Backoff{Base: time.Second, Max: time.Minute, Factor: 2}

	cases := []struct {
		name   string
		errors int
		want   time.Duration
	}{
		{name: "no errors means no backoff", errors: 0, want: 0},
		{name: "negative errors means no backoff", errors: -1, want: 0},
		{name: "first failure waits the base", errors: 1, want: time.Second},
		{name: "second doubles", errors: 2, want: 2 * time.Second},
		{name: "third doubles again", errors: 3, want: 4 * time.Second},
		{name: "caps at max", errors: 10, want: time.Minute},
		// A long outage must not overflow into a negative duration, which
		// would turn a backoff into a busy loop.
		{name: "a very long outage still caps", errors: 10_000, want: time.Minute},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := b.Next(tc.errors); got != tc.want {
				t.Errorf("Next(%d) = %v, want %v", tc.errors, got, tc.want)
			}
		})
	}
}

func TestBackoffZeroValueUsesDefaults(t *testing.T) {
	t.Parallel()

	var b scheduler.Backoff
	got := b.Next(1)

	if got != scheduler.DefaultBackoff.Base {
		t.Errorf("Next(1) on a zero Backoff = %v, want the default base %v", got, scheduler.DefaultBackoff.Base)
	}
	if b.Next(100) > scheduler.DefaultBackoff.Max {
		t.Error("zero-value backoff exceeded the default cap")
	}
}

func TestJitter(t *testing.T) {
	t.Parallel()

	base := 100 * time.Second

	cases := []struct {
		name     string
		d        time.Duration
		fraction float64
		random   float64
		want     time.Duration
	}{
		{name: "midpoint leaves the delay unchanged", d: base, fraction: 0.2, random: 0.5, want: base},
		{name: "zero random is the lower bound", d: base, fraction: 0.2, random: 0, want: 80 * time.Second},
		{name: "one random is the upper bound", d: base, fraction: 0.2, random: 1, want: 120 * time.Second},
		{name: "no fraction is a no-op", d: base, fraction: 0, random: 0, want: base},
		{name: "zero delay stays zero", d: 0, fraction: 0.5, random: 0, want: 0},
		{name: "fraction above one is clamped", d: base, fraction: 5, random: 1, want: 200 * time.Second},
		{name: "never negative", d: time.Second, fraction: 1, random: 0, want: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := scheduler.Jitter(tc.d, tc.fraction, tc.random); got != tc.want {
				t.Errorf("Jitter(%v, %v, %v) = %v, want %v", tc.d, tc.fraction, tc.random, got, tc.want)
			}
		})
	}
}

func TestBreakerState(t *testing.T) {
	t.Parallel()

	b := scheduler.Breaker{Threshold: 3, Cooldown: 10 * time.Minute}
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name        string
		errors      int
		lastAttempt time.Time
		want        scheduler.BreakerState
	}{
		{name: "healthy", errors: 0, lastAttempt: now, want: scheduler.BreakerClosed},
		{name: "below threshold", errors: 2, lastAttempt: now, want: scheduler.BreakerClosed},
		{name: "at threshold trips", errors: 3, lastAttempt: now, want: scheduler.BreakerOpen},
		{name: "still cooling", errors: 9, lastAttempt: now.Add(-5 * time.Minute), want: scheduler.BreakerOpen},
		{name: "cooldown elapsed allows a probe", errors: 9, lastAttempt: now.Add(-11 * time.Minute), want: scheduler.BreakerHalfOpen},
		{name: "never attempted allows a probe", errors: 9, lastAttempt: time.Time{}, want: scheduler.BreakerHalfOpen},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := b.State(tc.errors, tc.lastAttempt, now); got != tc.want {
				t.Errorf("State() = %v, want %v", got, tc.want)
			}
			if got, want := b.Allow(tc.errors, tc.lastAttempt, now), tc.want != scheduler.BreakerOpen; got != want {
				t.Errorf("Allow() = %v, want %v", got, want)
			}
		})
	}
}

func TestBreakerZeroValueUsesDefaults(t *testing.T) {
	t.Parallel()

	var b scheduler.Breaker
	now := time.Now()

	if !b.Allow(scheduler.DefaultBreaker.Threshold-1, now, now) {
		t.Error("a zero Breaker blocked a source below the default threshold")
	}
	if b.Allow(scheduler.DefaultBreaker.Threshold, now, now) {
		t.Error("a zero Breaker allowed a source at the default threshold")
	}
}
