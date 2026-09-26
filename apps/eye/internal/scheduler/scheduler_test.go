package scheduler_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/FullFran/eye/internal/logging"
	observation "github.com/FullFran/eye/internal/observation/domain"
	provider "github.com/FullFran/eye/internal/provider/domain"
	"github.com/FullFran/eye/internal/scheduler"
	source "github.com/FullFran/eye/internal/source/domain"
)

// instantClock fires every wait immediately and advances a virtual clock, so a
// loop that would take hours of wall time runs in microseconds.
type instantClock struct {
	mu    sync.Mutex
	now   time.Time
	slept []time.Duration
}

func newInstantClock() *instantClock {
	return &instantClock{now: time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)}
}

func (c *instantClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *instantClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.slept = append(c.slept, d)
	fire := c.now
	c.mu.Unlock()

	ch := make(chan time.Time, 1)
	ch <- fire
	return ch
}

// waits returns the durations the loop asked to sleep for.
func (c *instantClock) waits() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]time.Duration(nil), c.slept...)
}

// countingProvider returns a record per poll and can be made to fail.
type countingProvider struct {
	id  string
	url string

	mu     sync.Mutex
	polls  int
	failFn func(poll int) error
}

func (p *countingProvider) Info() source.Source {
	return source.Source{
		ID: p.id, Authority: "Test", Topic: "press", URL: p.url,
		Format: "rss", License: "unspecified",
		Access: source.AccessDocumentedAPI, Automation: source.AutomationEnabled,
		Interval: time.Minute,
	}
}

func (p *countingProvider) Poll(context.Context) ([]observation.Record, error) {
	p.mu.Lock()
	p.polls++
	n := p.polls
	fail := p.failFn
	p.mu.Unlock()

	if fail != nil {
		if err := fail(n); err != nil {
			return nil, err
		}
	}

	at := time.Now().UTC()
	return []observation.Record{{
		ID:     p.id + ":" + time.Now().Format(time.RFC3339Nano),
		Source: p.id, Kind: "news_item", Topic: "press",
		ObservedAt: at, FetchedAt: at, Title: "t",
		Quality: observation.QualityOfficial, Confidence: 1,
		Provenance: observation.Provenance{
			Publisher: "Test", SourceURL: "https://example.org", License: "unspecified", FetchedAt: at,
		},
	}}, nil
}

func (p *countingProvider) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.polls
}

// recordingSink collects outcomes and cancels the loop once it has seen
// enough. With an instant clock the goroutines are not fairly interleaved, so
// a test that needs output from several sources says so explicitly rather than
// counting on the scheduler to take turns.
type recordingSink struct {
	mu       sync.Mutex
	outcomes []scheduler.Outcome
	target   int
	until    func([]scheduler.Outcome) bool
	cancel   context.CancelFunc
}

func (s *recordingSink) Store(_ context.Context, _ provider.Provider, records []observation.Record) (int, error) {
	return len(records), nil
}

func (s *recordingSink) Report(_ context.Context, o scheduler.Outcome) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.outcomes = append(s.outcomes, o)
	if s.cancel == nil {
		return
	}
	if s.until != nil {
		if s.until(s.outcomes) {
			s.cancel()
		}
		return
	}
	if s.target > 0 && len(s.outcomes) >= s.target {
		s.cancel()
	}
}

func (s *recordingSink) all() []scheduler.Outcome {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]scheduler.Outcome(nil), s.outcomes...)
}

// options returns deterministic scheduler options for a test.
func options(clock scheduler.Clock) scheduler.Options {
	return scheduler.Options{
		Backoff:        scheduler.Backoff{Base: time.Minute, Max: 10 * time.Minute, Factor: 2},
		Breaker:        scheduler.Breaker{Threshold: 3, Cooldown: 10 * time.Minute},
		JitterFraction: 0.1,
		Clock:          clock,
		Logger:         logging.Discard(),
		Rand:           func() float64 { return 0.5 }, // midpoint: jitter is a no-op
	}
}

func TestSchedulerPollsRepeatedly(t *testing.T) {
	t.Parallel()

	clock := newInstantClock()
	p := &countingProvider{id: "press", url: "https://a.example/rss"}
	sink := &recordingSink{target: 4}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sink.cancel = cancel

	if err := scheduler.New([]provider.Provider{p}, sink, options(clock)).Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() = %v, want context.Canceled", err)
	}

	if p.count() < 4 {
		t.Errorf("provider polled %d times, want at least 4", p.count())
	}
	for _, o := range sink.all() {
		if !o.OK() {
			t.Errorf("unexpected failure: %v", o.Err)
		}
		if o.Stored != 1 {
			t.Errorf("stored = %d, want 1", o.Stored)
		}
	}
}

// A failing source must back off rather than retry on its normal interval.
func TestSchedulerBacksOffOnFailure(t *testing.T) {
	t.Parallel()

	clock := newInstantClock()
	p := &countingProvider{
		id: "flaky", url: "https://b.example/rss",
		failFn: func(int) error { return errors.New("503 from upstream") },
	}
	sink := &recordingSink{target: 3}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sink.cancel = cancel

	_ = scheduler.New([]provider.Provider{p}, sink, options(clock)).Run(ctx)

	outcomes := sink.all()
	if len(outcomes) < 2 {
		t.Fatalf("outcomes = %d, want at least 2", len(outcomes))
	}
	if outcomes[0].ConsecutiveErrors != 1 || outcomes[1].ConsecutiveErrors != 2 {
		t.Errorf("failure count did not accumulate: %d then %d",
			outcomes[0].ConsecutiveErrors, outcomes[1].ConsecutiveErrors)
	}

	// The interval is one minute; the second wait must exceed it because
	// backoff has taken over.
	waits := clock.waits()
	var overInterval int
	for _, w := range waits {
		if w > 90*time.Second {
			overInterval++
		}
	}
	if overInterval == 0 {
		t.Errorf("no wait exceeded the source interval; backoff never applied: %v", waits)
	}
}

// Once the breaker trips, the source must stop being called.
func TestSchedulerBreakerStopsCallingAFailingSource(t *testing.T) {
	t.Parallel()

	clock := newInstantClock()
	p := &countingProvider{
		id: "down", url: "https://c.example/rss",
		failFn: func(int) error { return errors.New("connection refused") },
	}
	sink := &recordingSink{target: 6}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sink.cancel = cancel

	_ = scheduler.New([]provider.Provider{p}, sink, options(clock)).Run(ctx)

	var skipped int
	for _, o := range sink.all() {
		if o.Skipped() {
			skipped++
			if o.Breaker != scheduler.BreakerOpen {
				t.Errorf("a skipped outcome reported breaker %q", o.Breaker)
			}
		}
	}

	if skipped == 0 {
		t.Fatalf("the breaker never skipped a poll; provider was called %d times for %d outcomes",
			p.count(), len(sink.all()))
	}
	// A skip must not be counted as another failure against the source.
	if p.count() >= len(sink.all()) {
		t.Errorf("provider polled %d times for %d outcomes; skips still called the source",
			p.count(), len(sink.all()))
	}
}

// A success after failures clears the count, so one bad morning does not leave
// a source permanently penalised.
func TestSchedulerRecoversAfterSuccess(t *testing.T) {
	t.Parallel()

	clock := newInstantClock()
	p := &countingProvider{
		id: "recovering", url: "https://d.example/rss",
		failFn: func(poll int) error {
			if poll <= 2 {
				return errors.New("temporary outage")
			}
			return nil
		},
	}
	sink := &recordingSink{target: 4}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sink.cancel = cancel

	_ = scheduler.New([]provider.Provider{p}, sink, options(clock)).Run(ctx)

	outcomes := sink.all()
	last := outcomes[len(outcomes)-1]
	if !last.OK() {
		t.Fatalf("final outcome failed: %v", last.Err)
	}
	if last.ConsecutiveErrors != 0 {
		t.Errorf("ConsecutiveErrors = %d after recovery, want 0", last.ConsecutiveErrors)
	}
}

// A restart during an outage must not reset a tripped breaker.
func TestSchedulerSeedRestoresFailureState(t *testing.T) {
	t.Parallel()

	clock := newInstantClock()
	p := &countingProvider{id: "still-down", url: "https://e.example/rss"}
	sink := &recordingSink{target: 1}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sink.cancel = cancel

	s := scheduler.New([]provider.Provider{p}, sink, options(clock))
	s.Seed("still-down", 9, clock.Now())
	_ = s.Run(ctx)

	outcomes := sink.all()
	if len(outcomes) == 0 {
		t.Fatal("no outcome")
	}
	if !outcomes[0].Skipped() {
		t.Errorf("first outcome = %+v, want a breaker skip from the seeded failure count", outcomes[0])
	}
	if p.count() != 0 {
		t.Errorf("provider was polled %d times despite a seeded open breaker", p.count())
	}
}

func TestSchedulerRunsMultipleProviders(t *testing.T) {
	t.Parallel()

	clock := newInstantClock()
	a := &countingProvider{id: "a", url: "https://a.example/rss"}
	b := &countingProvider{id: "b", url: "https://b.example/rss"}
	sink := &recordingSink{until: func(outcomes []scheduler.Outcome) bool {
		seen := map[string]bool{}
		for _, o := range outcomes {
			seen[o.SourceID] = true
		}
		return seen["a"] && seen["b"]
	}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sink.cancel = cancel

	_ = scheduler.New([]provider.Provider{a, b}, sink, options(clock)).Run(ctx)

	seen := map[string]bool{}
	for _, o := range sink.all() {
		seen[o.SourceID] = true
	}
	if !seen["a"] || !seen["b"] {
		t.Errorf("not every provider ran: %v", seen)
	}
}

func TestSchedulerStopsOnContextCancellation(t *testing.T) {
	t.Parallel()

	clock := newInstantClock()
	p := &countingProvider{id: "p", url: "https://f.example/rss"}
	sink := &recordingSink{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() { done <- scheduler.New([]provider.Provider{p}, sink, options(clock)).Run(ctx) }()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Run() = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return after cancellation")
	}
}

// A registry typo of "1s" must not become a denial-of-service.
func TestSchedulerFloorsTinyIntervals(t *testing.T) {
	t.Parallel()

	clock := newInstantClock()
	p := &tinyIntervalProvider{countingProvider{id: "eager", url: "https://g.example/rss"}}
	sink := &recordingSink{target: 3}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sink.cancel = cancel

	_ = scheduler.New([]provider.Provider{p}, sink, options(clock)).Run(ctx)

	for _, w := range clock.waits() {
		// Jitter is a no-op here (Rand is fixed at the midpoint), so any
		// wait below the floor means the floor was not applied.
		if w > 0 && w < scheduler.MinInterval {
			t.Errorf("waited %v, below the %v floor", w, scheduler.MinInterval)
		}
	}
}

// tinyIntervalProvider declares an interval below the floor.
type tinyIntervalProvider struct{ countingProvider }

func (p *tinyIntervalProvider) Info() source.Source {
	info := p.countingProvider.Info()
	info.Interval = time.Millisecond
	return info
}

// A daemon that starts and does nothing for hours is not a daemon. The first
// poll must be soon regardless of how long the source's interval is.
func TestSchedulerStartupStaggerIsBounded(t *testing.T) {
	t.Parallel()

	clock := newInstantClock()
	p := &slowIntervalProvider{countingProvider{id: "twice-daily", url: "https://h.example/rss"}}
	sink := &recordingSink{target: 1}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sink.cancel = cancel

	opts := options(clock)
	opts.Rand = func() float64 { return 1 } // worst case: the longest stagger
	_ = scheduler.New([]provider.Provider{p}, sink, opts).Run(ctx)

	waits := clock.waits()
	if len(waits) == 0 {
		t.Fatal("the scheduler never waited")
	}
	if waits[0] > scheduler.MaxStartupStagger {
		t.Errorf("first wait = %v, want at most %v — a twelve-hour source would take half a day to speak",
			waits[0], scheduler.MaxStartupStagger)
	}
	if p.count() == 0 {
		t.Error("the source was never polled")
	}
}

// slowIntervalProvider declares a twelve-hour interval.
type slowIntervalProvider struct{ countingProvider }

func (p *slowIntervalProvider) Info() source.Source {
	info := p.countingProvider.Info()
	info.Interval = 12 * time.Hour
	return info
}
