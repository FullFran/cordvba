package overpass_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/FullFran/cordvba/apps/eye/internal/httpx"
	"github.com/FullFran/cordvba/apps/eye/internal/provider/infrastructure/overpass"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// serveFixture returns a recorded Overpass answer, and records what was asked.
func serveFixture(t *testing.T, name string, asked *string) *httptest.Server {
	t.Helper()

	body, err := os.ReadFile("../../../../testdata/overpass/" + name) // #nosec G304 -- fixture path built from a test literal
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err == nil && asked != nil {
			*asked = r.Form.Get("data")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// stopsSource builds a registry entry pointing at a test server.
func stopsSource(url string, options map[string]string) source.Source {
	if options == nil {
		options = map[string]string{
			"kind":  "bus_stop",
			"query": `node["highway"="bus_stop"]({{bbox}});`,
			"bbox":  "37.80,-4.90,37.98,-4.65",
		}
	}
	return source.Source{
		ID: "osm-bus-stops", Authority: "OpenStreetMap contributors", Topic: "transport",
		URL: url, Format: "overpass-json", License: "ODbL-1.0",
		Access: source.AccessDocumentedAPI, Automation: source.AutomationEnabled,
		Interval: 168 * time.Hour, Options: options,
	}
}

func TestEntitiesMapsNamedStops(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t, "cordoba-bus-stops.json", nil)
	p := overpass.New(stopsSource(srv.URL, nil), httpx.New())

	entities, err := p.Entities(context.Background())
	if err != nil {
		t.Fatalf("Entities() = %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("expected entities")
	}

	var found bool
	for _, e := range entities {
		if e.Title != "Virgen Angustias 3ª DC" {
			continue
		}
		found = true

		if e.Kind != "bus_stop" {
			t.Errorf("kind = %q, want bus_stop", e.Kind)
		}
		if e.Position == nil {
			t.Fatal("stop has no position; a stop eye cannot place is worthless on a map")
		}
		if got := e.Position.Lat; got < 37.89 || got > 37.91 {
			t.Errorf("lat = %v, outside Cordoba", got)
		}
		if !strings.HasPrefix(e.ID, "osm-bus-stops:node/") {
			t.Errorf("id = %q, want the OSM element type and id so it can be looked up", e.ID)
		}
		if e.Provenance.License != "ODbL-1.0" {
			t.Errorf("license = %q; ODbL is share-alike and must survive normalization", e.Provenance.License)
		}
		if e.Provenance.Publisher != "OpenStreetMap contributors" {
			t.Errorf("publisher = %q", e.Provenance.Publisher)
		}
		if e.Provenance.RawHash == "" {
			t.Error("no raw hash: the observation cannot be replayed from its evidence")
		}
	}
	if !found {
		t.Fatal("the named stop in the fixture did not survive mapping")
	}
}

// TestEntitiesSkipsUnnamedStops checks eye does not invent a label. An unnamed
// OSM node is a real stop, but it is not one a person can search for, and
// calling it "bus_stop 914978713" pretends to a name it does not have.
func TestEntitiesSkipsUnnamedStops(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t, "cordoba-bus-stops.json", nil)
	p := overpass.New(stopsSource(srv.URL, nil), httpx.New())

	entities, err := p.Entities(context.Background())
	if err != nil {
		t.Fatalf("Entities() = %v", err)
	}
	for _, e := range entities {
		if strings.TrimSpace(e.Title) == "" {
			t.Errorf("entity %s has an empty title", e.ID)
		}
		if strings.Contains(e.Title, "914978713") {
			t.Errorf("entity %s was given a fabricated title %q", e.ID, e.Title)
		}
	}
}

// TestPollSubstitutesTheBoundingBox checks the registry's bbox reaches the
// query. Overpass has no default area, and a query sent without one either
// fails or returns the planet.
func TestPollSubstitutesTheBoundingBox(t *testing.T) {
	t.Parallel()

	var asked string
	srv := serveFixture(t, "cordoba-bus-stops.json", &asked)
	p := overpass.New(stopsSource(srv.URL, nil), httpx.New())

	if _, err := p.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if !strings.Contains(asked, "37.80,-4.90,37.98,-4.65") {
		t.Errorf("query sent = %q, want the registry bounding box substituted", asked)
	}
	if strings.Contains(asked, "{{bbox}}") {
		t.Error("the {{bbox}} placeholder was sent literally")
	}
	if !strings.Contains(asked, "[out:json]") {
		t.Errorf("query sent = %q, want a JSON output header", asked)
	}
}

// TestPollReportsTheInventoryOnce checks the poll emits an inventory refresh
// rather than one record per stop. A bus stop is a persistent thing; where it
// is does not change every poll.
func TestPollReportsTheInventoryOnce(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t, "cordoba-bus-stops.json", nil)
	p := overpass.New(stopsSource(srv.URL, nil), httpx.New())

	records, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want exactly one inventory refresh", len(records))
	}
	if records[0].Kind != "inventory_refresh" {
		t.Errorf("kind = %q, want inventory_refresh", records[0].Kind)
	}
	if err := records[0].Validate(); err != nil {
		t.Errorf("record invalid: %v", err)
	}

	var payload struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(records[0].Payload, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if payload.Count == 0 {
		t.Error("payload reports no stops")
	}
}

// TestPollRefusesWithoutAQuery checks the registry, not the adapter, decides
// what is asked for. An Overpass adapter with a built-in query is a scraper
// with an opinion.
func TestPollRefusesWithoutAQuery(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t, "cordoba-bus-stops.json", nil)
	src := stopsSource(srv.URL, map[string]string{"kind": "bus_stop"})

	if _, err := overpass.New(src, httpx.New()).Poll(context.Background()); err == nil {
		t.Fatal("expected an error when the registry declares no query")
	}
}

// TestPollSurfacesARefusal checks a server error is reported, never swallowed
// into an empty inventory that looks like "Cordoba has no bus stops".
func TestPollSurfacesARefusal(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)

	if _, err := overpass.New(stopsSource(srv.URL, nil), httpx.New()).Poll(context.Background()); err == nil {
		t.Fatal("expected an error on 429")
	}
}
