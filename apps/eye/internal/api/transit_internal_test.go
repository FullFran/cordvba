package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	observation "github.com/FullFran/eye/internal/observation/domain"
	source "github.com/FullFran/eye/internal/source/domain"
)

// countingTransit answers instantly and remembers how often it was asked.
type countingTransit struct{ calls int }

func (c *countingTransit) Arrivals(context.Context, []string) ([]observation.Record, error) {
	c.calls++
	return nil, nil
}

func (c *countingTransit) ResolveStops(context.Context, string) ([]string, error) {
	return nil, nil
}

func (c *countingTransit) Departures(context.Context, string, int) ([]observation.Record, error) {
	return nil, nil
}

// The cache has to expire, or a stop shows the same bus for the rest of the
// afternoon. A clock is injected rather than slept on: a test that waits
// twenty seconds is a test nobody runs.
func TestArrivalsCacheExpires(t *testing.T) {
	t.Parallel()

	transit := &countingTransit{}
	server := New(emptyStore{}, []source.Source{{
		ID: "aucorsa-arrivals", Access: source.AccessUndocumentedPersonal,
	}}, nil, WithTransit(transit), WithPersonalSources(true), WithToken("s3cret"))

	clock := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	server.now = func() time.Time { return clock }

	handler := server.Handler()
	ask := func() {
		t.Helper()

		w := httptest.NewRecorder()
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/transit/arrivals?stop=101", nil)
		r.Header.Set("Authorization", "Bearer s3cret")
		handler.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", w.Code, w.Body.String())
		}
	}

	ask()
	clock = clock.Add(arrivalsTTL / 2)
	ask()
	if transit.calls != 1 {
		t.Fatalf("calls = %d within the cache window, want 1", transit.calls)
	}

	clock = clock.Add(arrivalsTTL)
	ask()
	if transit.calls != 2 {
		t.Errorf("calls = %d after the cache expired, want a fresh read", transit.calls)
	}
}
