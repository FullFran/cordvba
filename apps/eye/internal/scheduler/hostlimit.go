package scheduler

import (
	"context"
	"sync"
)

// hostLimiter caps how many requests are in flight against one host.
//
// The limit is per host rather than global because that is where the courtesy
// matters: three Cordoba newspapers on three different hosts can be polled at
// once, but three feeds from the same publisher should not be.
type hostLimiter struct {
	limit int

	mu   sync.Mutex
	sems map[string]chan struct{}
}

// newHostLimiter builds a limiter allowing `limit` concurrent requests per host.
func newHostLimiter(limit int) *hostLimiter {
	if limit <= 0 {
		limit = DefaultHostConcurrency
	}
	return &hostLimiter{limit: limit, sems: make(map[string]chan struct{})}
}

// acquire blocks until a slot for host is free. It returns a release function
// and false when the context ended first.
func (l *hostLimiter) acquire(ctx context.Context, host string) (release func(), ok bool) {
	sem := l.semFor(host)

	select {
	case sem <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-sem }) }, true
	case <-ctx.Done():
		return func() {}, false
	}
}

// semFor returns the semaphore for a host, creating it on first use.
func (l *hostLimiter) semFor(host string) chan struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()

	sem, ok := l.sems[host]
	if !ok {
		sem = make(chan struct{}, l.limit)
		l.sems[host] = sem
	}
	return sem
}
