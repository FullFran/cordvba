package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"net/url"
	"sync"
	"time"

	observation "github.com/FullFran/eye/internal/observation/domain"
	provider "github.com/FullFran/eye/internal/provider/domain"
)

// Defaults for the polling loop.
const (
	// DefaultJitterFraction spreads each interval by ±15%.
	DefaultJitterFraction = 0.15
	// DefaultHostConcurrency is how many requests may be in flight against
	// one host. Several Cordoba feeds share a publisher.
	DefaultHostConcurrency = 2
	// MinInterval floors any configured interval. A registry typo of "1s"
	// should not become a denial-of-service against a public service.
	MinInterval = 5 * time.Second
	// MaxStartupStagger bounds how long a source waits for its first poll.
	//
	// Staggering startup avoids a thundering herd, but it must not be
	// proportional to the interval: a source polled every twelve hours
	// would then take up to a day to say anything, and a daemon that
	// starts and does nothing is not a daemon.
	MaxStartupStagger = 30 * time.Second
)

// ErrBreakerOpen reports a poll that was deliberately not made because the
// source is in an outage. It is a skip, not a failure of the source: counting
// it as an error would inflate the failure count the breaker itself is reading.
var ErrBreakerOpen = errors.New("scheduler: breaker open, source left alone")

// Sink receives the outcome of every poll. The daemon uses it to persist
// records and source state; tests use it to observe the loop.
type Sink interface {
	// Store persists what a poll returned and reports how many records
	// were new.
	Store(ctx context.Context, p provider.Provider, records []observation.Record) (int, error)
	// Report records the outcome of one poll, successful or not.
	Report(ctx context.Context, outcome Outcome)
}

// Outcome is what one poll produced.
type Outcome struct {
	SourceID string
	Topic    string
	// Attempted is when the poll started.
	Attempted time.Time
	// Records is how many observations the source returned.
	Records int
	// Stored is how many of those were new.
	Stored int
	// Err is nil on success.
	Err error
	// ConsecutiveErrors is the running failure count after this poll.
	ConsecutiveErrors int
	// Breaker is where the source sat when the poll was decided.
	Breaker BreakerState
}

// OK reports whether the poll succeeded.
func (o Outcome) OK() bool { return o.Err == nil }

// Skipped reports whether the poll was deliberately not made. A skip is
// neither a success nor a failure and must not be counted as either.
func (o Outcome) Skipped() bool { return errors.Is(o.Err, ErrBreakerOpen) }

// Clock is the scheduler's view of time. Injecting it is what lets the loop be
// tested in microseconds instead of minutes.
type Clock interface {
	Now() time.Time
	// After returns a channel that fires once, after d.
	After(d time.Duration) <-chan time.Time
}

// systemClock is the real implementation.
type systemClock struct{}

func (systemClock) Now() time.Time                         { return time.Now().UTC() }
func (systemClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// Options configure a Scheduler.
type Options struct {
	Backoff         Backoff
	Breaker         Breaker
	JitterFraction  float64
	HostConcurrency int
	Clock           Clock
	Logger          *slog.Logger
	// Rand returns a value in [0,1). Injectable so jitter is deterministic
	// under test.
	Rand func() float64
}

// Scheduler polls a set of providers, each on its own interval.
type Scheduler struct {
	providers []provider.Provider
	sink      Sink
	opts      Options

	hosts *hostLimiter

	mu    sync.Mutex
	state map[string]*sourceRuntime
}

// sourceRuntime is the scheduler's per-source memory.
type sourceRuntime struct {
	consecutiveErrors int
	lastAttempt       time.Time
}

// New builds a scheduler over the given providers.
func New(providers []provider.Provider, sink Sink, opts Options) *Scheduler {
	if opts.Clock == nil {
		opts.Clock = systemClock{}
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Rand == nil {
		opts.Rand = rand.Float64
	}
	if opts.JitterFraction <= 0 {
		opts.JitterFraction = DefaultJitterFraction
	}
	if opts.HostConcurrency <= 0 {
		opts.HostConcurrency = DefaultHostConcurrency
	}

	return &Scheduler{
		providers: providers,
		sink:      sink,
		opts:      opts,
		hosts:     newHostLimiter(opts.HostConcurrency),
		state:     make(map[string]*sourceRuntime, len(providers)),
	}
}

// Seed primes the scheduler with persisted failure counts, so a restart during
// an outage does not reset a tripped breaker and hammer a struggling source.
func (s *Scheduler) Seed(sourceID string, consecutiveErrors int, lastAttempt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state[sourceID] = &sourceRuntime{consecutiveErrors: consecutiveErrors, lastAttempt: lastAttempt}
}

// Run polls every provider on its own schedule until ctx is cancelled.
//
// Each provider gets a goroutine that sleeps between its own polls. That is a
// goroutine per source rather than a central heap, which for twenty sources is
// simpler to read and to reason about than the alternative.
func (s *Scheduler) Run(ctx context.Context) error {
	var wg sync.WaitGroup

	for _, p := range s.providers {
		wg.Add(1)
		go func(p provider.Provider) {
			defer wg.Done()
			s.runOne(ctx, p)
		}(p)
	}

	wg.Wait()
	return ctx.Err()
}

// runOne is the loop for a single provider.
func (s *Scheduler) runOne(ctx context.Context, p provider.Provider) {
	info := p.Info()
	log := s.opts.Logger.With("source", info.ID, "topic", info.Topic)

	// Stagger the first poll so twenty sources do not all start together,
	// but bound it: everything should have been polled within the first
	// half-minute of the daemon coming up.
	if !s.wait(ctx, s.startupStagger()) {
		return
	}

	for {
		delay := s.pollOnce(ctx, p, log)
		if !s.wait(ctx, delay) {
			return
		}
	}
}

// pollOnce performs one cycle and returns how long to wait before the next.
func (s *Scheduler) pollOnce(ctx context.Context, p provider.Provider, log *slog.Logger) time.Duration {
	info := p.Info()
	now := s.opts.Clock.Now()

	errorCount, lastAttempt := s.snapshot(info.ID)
	breaker := s.opts.Breaker.State(errorCount, lastAttempt, now)

	if breaker == BreakerOpen {
		// Do not call, and do not count it as another failure: the whole
		// point of an open breaker is that the source is left alone.
		log.Debug("breaker open, skipping poll", "consecutive_errors", errorCount)
		s.sink.Report(ctx, Outcome{
			SourceID: info.ID, Topic: info.Topic, Attempted: now,
			ConsecutiveErrors: errorCount, Breaker: breaker,
			Err: ErrBreakerOpen,
		})
		return s.nextDelay(info.Interval, errorCount)
	}

	release, ok := s.hosts.acquire(ctx, hostOf(info.URL))
	if !ok {
		return s.nextDelay(info.Interval, errorCount)
	}

	started := s.opts.Clock.Now()
	records, err := p.Poll(ctx)
	release()

	outcome := Outcome{
		SourceID: info.ID, Topic: info.Topic, Attempted: started,
		Records: len(records), Err: err, Breaker: breaker,
	}

	if err != nil {
		errorCount = s.recordFailure(info.ID, started)
		outcome.ConsecutiveErrors = errorCount
		log.Warn("poll failed", "error", err, "consecutive_errors", errorCount,
			"retry_in", s.nextDelay(info.Interval, errorCount).String())
		s.sink.Report(ctx, outcome)
		return s.nextDelay(info.Interval, errorCount)
	}

	stored, storeErr := s.sink.Store(ctx, p, records)
	if storeErr != nil {
		errorCount = s.recordFailure(info.ID, started)
		outcome.ConsecutiveErrors = errorCount
		outcome.Err = storeErr
		log.Error("store failed", "error", storeErr)
		s.sink.Report(ctx, outcome)
		return s.nextDelay(info.Interval, errorCount)
	}

	s.recordSuccess(info.ID, started)
	outcome.Stored = stored
	outcome.ConsecutiveErrors = 0

	log.Info("poll complete", "records", len(records), "new", stored,
		"took", s.opts.Clock.Now().Sub(started).String())
	s.sink.Report(ctx, outcome)

	return s.nextDelay(info.Interval, 0)
}

// startupStagger is a short, bounded random delay before a source's first poll.
func (s *Scheduler) startupStagger() time.Duration {
	return time.Duration(s.opts.Rand() * float64(MaxStartupStagger))
}

// nextDelay is the source's interval, or the backoff when it is failing, with
// jitter applied either way.
func (s *Scheduler) nextDelay(configured time.Duration, consecutiveErrors int) time.Duration {
	delay := interval(configured)
	if backoff := s.opts.Backoff.Next(consecutiveErrors); backoff > delay {
		delay = backoff
	}
	return Jitter(delay, s.opts.JitterFraction, s.opts.Rand())
}

// wait blocks for d, reporting false when the context ends first.
//
// Cancellation is checked before the select rather than only inside it. A
// select over a ready timer and a closed Done channel picks at random, so
// without this a shutdown can lose the coin toss and run one more poll.
func (s *Scheduler) wait(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	default:
	}

	if d <= 0 {
		return true
	}

	select {
	case <-ctx.Done():
		return false
	case <-s.opts.Clock.After(d):
		return true
	}
}

// snapshot reads a source's failure state.
func (s *Scheduler) snapshot(sourceID string) (consecutiveErrors int, lastAttempt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rt, ok := s.state[sourceID]
	if !ok {
		return 0, time.Time{}
	}
	return rt.consecutiveErrors, rt.lastAttempt
}

// recordFailure increments the failure count and returns the new value.
func (s *Scheduler) recordFailure(sourceID string, at time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	rt, ok := s.state[sourceID]
	if !ok {
		rt = &sourceRuntime{}
		s.state[sourceID] = rt
	}
	rt.consecutiveErrors++
	rt.lastAttempt = at
	return rt.consecutiveErrors
}

// recordSuccess clears the failure count.
func (s *Scheduler) recordSuccess(sourceID string, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state[sourceID] = &sourceRuntime{lastAttempt: at}
}

// interval floors a configured interval, so a registry typo cannot become a
// denial-of-service against a public service.
func interval(d time.Duration) time.Duration {
	if d < MinInterval {
		return MinInterval
	}
	return d
}

// hostOf extracts the host for concurrency limiting. Several sources share a
// publisher, and the limit belongs to the host rather than to the source id.
func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	return u.Host
}
