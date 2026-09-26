package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FullFran/eye/internal/api"
	"github.com/FullFran/eye/internal/logging"
	source "github.com/FullFran/eye/internal/source/domain"
)

// aggregatingStore answers the aggregate port as a real store would.
type aggregatingStore struct {
	fakeStore
	aggregate api.Aggregate
	err       error
}

func (a *aggregatingStore) Aggregate(context.Context) (api.Aggregate, error) {
	return a.aggregate, a.err
}

func (a *aggregatingStore) StoreBytes() (int64, error) { return 4096, nil }

// A count computed from one page of records is a wrong count, so the store
// aggregates and the handler renders.
func TestStatsAggregates(t *testing.T) {
	t.Parallel()

	oldest := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	newest := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)

	store := &aggregatingStore{aggregate: api.Aggregate{
		Records: 12, Entities: 3,
		ByTopic:  map[string]int{"press": 7, "traffic": 5},
		BySource: map[string]int{"diario-cordoba": 12},
		ByKind:   map[string]int{"news_item": 12},
		Oldest:   oldest, Newest: newest,
	}}

	srv := httptest.NewServer(api.New(store, nil, logging.Discard()).Handler())
	t.Cleanup(srv.Close)

	status, body := get(t, srv, "/v1/stats")
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}

	byTopic, _ := body["by_topic"].(map[string]any)
	if byTopic["press"] != float64(7) || byTopic["traffic"] != float64(5) {
		t.Errorf("by_topic = %v", byTopic)
	}
	bySource, _ := body["by_source"].(map[string]any)
	if bySource["diario-cordoba"] != float64(12) {
		t.Errorf("by_source = %v", bySource)
	}
	byKind, _ := body["by_kind"].(map[string]any)
	if byKind["news_item"] != float64(12) {
		t.Errorf("by_kind = %v", byKind)
	}
	if body["oldest"] != oldest.Format(time.RFC3339) {
		t.Errorf("oldest = %v", body["oldest"])
	}
	if body["newest"] != newest.Format(time.RFC3339) {
		t.Errorf("newest = %v", body["newest"])
	}
	if body["store_bytes"] != float64(4096) {
		t.Errorf("store_bytes = %v", body["store_bytes"])
	}
	if body["records"] != float64(12) {
		t.Errorf("records = %v", body["records"])
	}
}

// A store that cannot aggregate still serves everything else. Saying so beats
// answering with a plausible zero.
func TestStatsSaysSoWhenTheStoreCannotAggregate(t *testing.T) {
	t.Parallel()

	srv := serve(t, &fakeStore{})
	status, body := get(t, srv, "/v1/stats")

	if status != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", status)
	}
	if body["error"] == nil {
		t.Errorf("body = %v, want a reason", body)
	}
}

// "We have not managed to read this for a week" is the useful half of a
// registry view, and it comes from the store rather than from a guess.
func TestSourcesCarryStoredHealth(t *testing.T) {
	t.Parallel()

	success := time.Date(2026, time.August, 28, 11, 0, 0, 0, time.UTC)
	store := &fakeStore{states: []api.SourceState{{
		SourceID: "saih-guadalquivir", LastSuccess: success,
		LastError: "connection refused", Records: 42, ConsecutiveErrors: 3,
	}}}

	srv := serve(t, store)
	status, body := get(t, srv, "/v1/sources")
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}

	sources, _ := body["sources"].([]any)
	var found bool
	for _, s := range sources {
		entry, _ := s.(map[string]any)
		if entry["id"] != "saih-guadalquivir" {
			continue
		}
		found = true
		if entry["last_success"] != success.Format(time.RFC3339) {
			t.Errorf("last_success = %v", entry["last_success"])
		}
		if entry["last_error"] != "connection refused" {
			t.Errorf("last_error = %v", entry["last_error"])
		}
		if entry["records"] != float64(42) {
			t.Errorf("records = %v", entry["records"])
		}
		if entry["consecutive_errors"] != float64(3) {
			t.Errorf("consecutive_errors = %v", entry["consecutive_errors"])
		}
	}
	if !found {
		t.Fatalf("sources = %v", body)
	}
}

// A source eye has never polled has no health, and reporting a zero timestamp
// as a last success would be a lie with a date on it.
func TestSourcesWithoutHealthOmitIt(t *testing.T) {
	t.Parallel()

	srv := serve(t, &fakeStore{})
	_, body := get(t, srv, "/v1/sources")

	sources, _ := body["sources"].([]any)
	for _, s := range sources {
		entry, _ := s.(map[string]any)
		if _, ok := entry["last_success"]; ok {
			t.Errorf("an unpolled source reports a last success: %v", entry)
		}
	}
}

// serveWithPersonalSource builds a server whose registry holds an
// undocumented personal source.
func serveWithPersonalSource(t *testing.T, store api.Store, opts ...api.Option) *httptest.Server {
	t.Helper()

	sources := []source.Source{
		{
			ID: "diario-cordoba", Authority: "Diario Cordoba", Topic: "press",
			URL: "https://example.org/rss", Format: "rss", License: "unspecified",
			Access: source.AccessDocumentedAPI, Automation: source.AutomationEnabled,
			Interval: time.Minute,
		},
		{
			ID: "aucorsa-arrivals", Authority: "AUCORSA", Topic: "transport",
			URL: "https://example.org/live", Format: "aucorsa-arrivals", License: "undocumented",
			Access: source.AccessUndocumentedPersonal, Automation: source.AutomationEnabled,
			Interval: time.Minute, Notes: "Operator's own undocumented source.",
		},
	}

	srv := httptest.NewServer(api.New(store, sources, logging.Discard(), opts...).Handler())
	t.Cleanup(srv.Close)
	return srv
}
