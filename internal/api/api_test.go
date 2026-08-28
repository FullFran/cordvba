package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FullFran/eye/internal/api"
	"github.com/FullFran/eye/internal/logging"
	observation "github.com/FullFran/eye/internal/observation/domain"
	source "github.com/FullFran/eye/internal/source/domain"
)

// fakeStore records the filter it was given, so the tests can assert that a
// query string became the filter it should have.
type fakeStore struct {
	lastFilter observation.Filter
	records    []observation.Record
	entities   []observation.Entity
	err        error
}

func (f *fakeStore) Query(_ context.Context, filter observation.Filter) ([]observation.Record, error) {
	f.lastFilter = filter
	return f.records, f.err
}

func (f *fakeStore) Entities(_ context.Context, filter observation.Filter) ([]observation.Entity, error) {
	f.lastFilter = filter
	return f.entities, f.err
}

func (f *fakeStore) Counts(context.Context) (int, int, error) {
	return len(f.records), len(f.entities), f.err
}

// record builds a valid observation.
func record(id, topic string) observation.Record {
	at := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	return observation.Record{
		ID: id, Source: "diario-cordoba", Kind: "news_item", Topic: topic,
		ObservedAt: at, FetchedAt: at.Add(7 * time.Minute),
		Title: "Titular " + id, Quality: observation.QualityOfficial, Confidence: 1,
		Provenance: observation.Provenance{
			Publisher: "Diario Cordoba", SourceURL: "https://example.org/1",
			License: "unspecified", FetchedAt: at,
		},
	}
}

// serve starts a test server over a store.
func serve(t *testing.T, store *fakeStore) *httptest.Server {
	t.Helper()

	sources := []source.Source{{
		ID: "diario-cordoba", Authority: "Diario Cordoba", Topic: "press",
		URL: "https://www.diariocordoba.com/rss/", Format: "rss", License: "unspecified",
		Access: source.AccessDocumentedAPI, Automation: source.AutomationEnabled, Interval: time.Minute,
	}, {
		ID: "saih-guadalquivir", Authority: "CHG", Topic: "hydrology",
		URL: "https://www.chguadalquivir.es/saih/", Format: "web", License: "unspecified",
		Access: source.AccessPublicHTML, Automation: source.AutomationReviewTerms,
		Notes: "Reuse terms unresolved.",
	}}

	srv := httptest.NewServer(api.New(store, sources, logging.Discard()).Handler())
	t.Cleanup(srv.Close)
	return srv
}

// get performs a request and decodes the JSON body.
func get(t *testing.T, srv *httptest.Server, path string) (int, map[string]any) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatalf("build request %s: %v", path, err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return resp.StatusCode, body
}

func TestHealth(t *testing.T) {
	t.Parallel()

	srv := serve(t, &fakeStore{records: []observation.Record{record("1", "press")}})
	status, body := get(t, srv, "/health")

	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if body["status"] != "ok" || body["records"] != float64(1) {
		t.Errorf("health = %v", body)
	}
}

// The whole point of the gateway: a record arrives with its provenance intact.
func TestRecordsKeepProvenance(t *testing.T) {
	t.Parallel()

	srv := serve(t, &fakeStore{records: []observation.Record{record("1", "press")}})
	_, body := get(t, srv, "/v1/records")

	records, _ := body["records"].([]any)
	if len(records) != 1 {
		t.Fatalf("records = %v", body)
	}

	first, _ := records[0].(map[string]any)
	prov, _ := first["provenance"].(map[string]any)
	for _, field := range []string{"publisher", "source_url", "license"} {
		if prov[field] == "" || prov[field] == nil {
			t.Errorf("provenance is missing %q: %v", field, prov)
		}
	}
	// Both timestamps, so a consumer can compute source latency itself.
	if first["observed_at"] == first["fetched_at"] {
		t.Error("the two timestamps were collapsed")
	}
}

func TestRecordsFilterMapping(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	srv := serve(t, store)

	get(t, srv, "/v1/records?topic=press,civic&source=boe&kind=news_item&text=feria&min_severity=3&limit=42") //nolint:dogsled // the assertion is on the recorded filter

	f := store.lastFilter
	if len(f.Topics) != 2 || f.Topics[0] != "press" {
		t.Errorf("topics = %v", f.Topics)
	}
	if len(f.Sources) != 1 || f.Sources[0] != "boe" {
		t.Errorf("sources = %v", f.Sources)
	}
	if f.Text != "feria" {
		t.Errorf("text = %q", f.Text)
	}
	if f.MinSeverity != 3 {
		t.Errorf("min severity = %d", f.MinSeverity)
	}
	if f.Limit != 42 {
		t.Errorf("limit = %d", f.Limit)
	}
}

// A person has "the last two hours"; a script has a timestamp. Both are natural
// and both are accepted.
func TestSinceAcceptsDurationAndTimestamp(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	srv := serve(t, store)

	get(t, srv, "/v1/records?since=2h") //nolint:dogsled // asserting on the filter
	if store.lastFilter.Since == nil {
		t.Fatal("since=2h produced no bound")
	}
	if age := time.Since(*store.lastFilter.Since); age < 100*time.Minute || age > 140*time.Minute {
		t.Errorf("since=2h resolved to %v ago", age)
	}

	get(t, srv, "/v1/records?since=2026-08-28T10:00:00Z") //nolint:dogsled // asserting on the filter
	if store.lastFilter.Since == nil || store.lastFilter.Since.Hour() != 10 {
		t.Errorf("since as RFC3339 = %v", store.lastFilter.Since)
	}
}

func TestSpatialParameters(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	srv := serve(t, store)

	get(t, srv, "/v1/entities?near=37.8882,-4.7794&radius_km=15") //nolint:dogsled // asserting on the filter
	if store.lastFilter.Near == nil || store.lastFilter.RadiusKm != 15 {
		t.Errorf("near/radius = %v / %v", store.lastFilter.Near, store.lastFilter.RadiusKm)
	}

	get(t, srv, "/v1/records?bbox=-5.6,37.1,-4.0,38.7") //nolint:dogsled // asserting on the filter
	if store.lastFilter.BBox == nil || store.lastFilter.BBox.West != -5.6 {
		t.Errorf("bbox = %v", store.lastFilter.BBox)
	}
}

// A gateway that serialises its whole store on request is a denial-of-service
// waiting for a curious client.
func TestLimitIsCapped(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	srv := serve(t, store)

	get(t, srv, "/v1/records?limit=999999") //nolint:dogsled // asserting on the filter
	if store.lastFilter.Limit > 5000 {
		t.Errorf("limit = %d, want it capped", store.lastFilter.Limit)
	}
}

func TestBadParametersAreRejectedWithAReason(t *testing.T) {
	t.Parallel()

	srv := serve(t, &fakeStore{})

	cases := []struct{ name, path, wants string }{
		{name: "bad since", path: "/v1/records?since=yesterday", wants: "since"},
		{name: "bad bbox", path: "/v1/records?bbox=1,2,3", wants: "bbox"},
		{name: "inverted bbox", path: "/v1/records?bbox=0,10,-10,0", wants: "bbox"},
		{name: "bad near", path: "/v1/entities?near=north", wants: "near"},
		{name: "near outside wgs84", path: "/v1/entities?near=91,0", wants: "near"},
		{name: "bad limit", path: "/v1/records?limit=-1", wants: "limit"},
		{name: "bad radius", path: "/v1/entities?near=37.8,-4.7&radius_km=0", wants: "radius"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			status, body := get(t, srv, tc.path)
			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", status)
			}
			msg, _ := body["error"].(string)
			if msg == "" {
				t.Fatal("no error message")
			}
			if !contains(msg, tc.wants) {
				t.Errorf("error = %q, want it to name %q", msg, tc.wants)
			}
		})
	}
}

// The registry is served including what is held and why, because "we do not
// have this and here is the reason" is itself information.
func TestSourcesIncludeHeldOnesAndTheirReason(t *testing.T) {
	t.Parallel()

	srv := serve(t, &fakeStore{})
	_, body := get(t, srv, "/v1/sources")

	sources, _ := body["sources"].([]any)
	if len(sources) != 2 {
		t.Fatalf("sources = %v", body)
	}

	var foundHeld bool
	for _, s := range sources {
		entry, _ := s.(map[string]any)
		if entry["id"] == "saih-guadalquivir" {
			foundHeld = true
			if entry["pollable"] != false {
				t.Error("a held source is reported as pollable")
			}
			if entry["notes"] == "" || entry["notes"] == nil {
				t.Error("a held source carries no reason")
			}
		}
	}
	if !foundHeld {
		t.Error("held sources are not served")
	}
}

func TestIndexDescribesTheAPI(t *testing.T) {
	t.Parallel()

	srv := serve(t, &fakeStore{})
	status, body := get(t, srv, "/")

	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if body["service"] != "eye" {
		t.Errorf("index = %v", body)
	}
	// Somebody who curls the root should learn what to ask for.
	endpoints, _ := body["endpoints"].(map[string]any)
	if len(endpoints) < 4 {
		t.Errorf("endpoints = %v", endpoints)
	}
}

func TestUnknownPathIs404(t *testing.T) {
	t.Parallel()

	srv := serve(t, &fakeStore{})
	if status, _ := get(t, srv, "/v1/nope"); status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
}

func TestBrowsersCanReadIt(t *testing.T) {
	t.Parallel()

	srv := serve(t, &fakeStore{})
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/health", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Error("no CORS header; a browser map could not consume this")
	}
	if resp.Header.Get("X-Eye-Provenance") == "" {
		t.Error("no provenance header")
	}
}

// contains is a tiny helper so the table above reads cleanly.
func contains(haystack, needle string) bool {
	return len(needle) == 0 || len(haystack) >= len(needle) &&
		(haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// eye must never serve onward what it was not entitled to redistribute. The
// filter lives in one place because a rule spread across handlers is one that
// gets forgotten in the next one.
func TestPersonalSourceRecordsAreNeverServed(t *testing.T) {
	t.Parallel()

	personal := record("secret", "transport")
	personal.Source = "adif-live"

	store := &fakeStore{records: []observation.Record{record("public", "press"), personal}}

	sources := []source.Source{
		{
			ID: "diario-cordoba", Authority: "Diario Cordoba", Topic: "press",
			URL: "https://example.org/rss", Format: "rss", License: "unspecified",
			Access: source.AccessDocumentedAPI, Automation: source.AutomationEnabled,
			Interval: time.Minute,
		},
		{
			ID: "adif-live", Authority: "ADIF", Topic: "transport",
			URL: "https://example.org/live", Format: "json", License: "undocumented",
			Access: source.AccessUndocumentedPersonal, Automation: source.AutomationEnabled,
			Interval: time.Minute, Notes: "Operator's own undocumented source.",
		},
	}

	srv := httptest.NewServer(api.New(store, sources, logging.Discard()).Handler())
	defer srv.Close()

	status, body := get(t, srv, "/v1/records")
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}

	records, _ := body["records"].([]any)
	if len(records) != 1 {
		t.Fatalf("served %d records, want only the redistributable one", len(records))
	}
	first, _ := records[0].(map[string]any)
	if first["source"] == "adif-live" {
		t.Error("a personal source's record was served")
	}
	if body["count"] != float64(1) {
		t.Errorf("count = %v, want it to match what was actually served", body["count"])
	}
}
