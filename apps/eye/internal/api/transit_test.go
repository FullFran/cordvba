package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/FullFran/eye/internal/api"
	"github.com/FullFran/eye/internal/logging"
	observation "github.com/FullFran/eye/internal/observation/domain"
)

// fakeTransit is a live reader that never leaves the process.
type fakeTransit struct {
	mu         sync.Mutex
	calls      int
	stopsAsked []string
	resolved   []string
	arrivals   []observation.Record
	departures []observation.Record
	err        error
}

func (f *fakeTransit) Arrivals(_ context.Context, stops []string) ([]observation.Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.stopsAsked = stops
	return f.arrivals, f.err
}

func (f *fakeTransit) ResolveStops(_ context.Context, _ string) ([]string, error) {
	return f.resolved, f.err
}

func (f *fakeTransit) Departures(_ context.Context, _ string, _ int) ([]observation.Record, error) {
	return f.departures, f.err
}

func (f *fakeTransit) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// arrivalRecord is one live estimate in the shape the AUCORSA reader produces.
func arrivalRecord(stop, line string, minutes int) observation.Record {
	r := record("arrival:"+stop+":"+line, "transport")
	r.Source = "aucorsa-arrivals"
	r.Kind = "bus_arrival"
	payload, _ := json.Marshal(map[string]any{
		"stop_id": stop, "stop_name": "Parada " + stop,
		"line": line, "route": "Ronda de los Tejares", "minutes": minutes,
	})
	r.Payload = payload
	return r
}

// openTransit builds a server with a live transit port, personal sources
// allowed and a token configured — the only combination that may serve live
// arrivals at all.
func openTransit(t *testing.T, transit api.Transit) *httptest.Server {
	t.Helper()

	return serveWithPersonalSource(t, &fakeStore{},
		api.WithTransit(transit),
		api.WithPersonalSources(true),
		api.WithToken("s3cret"))
}

// authorized adds the bearer header the guarded endpoints need.
func authorized() map[string]string {
	return map[string]string{"Authorization": "Bearer s3cret"}
}

func TestArrivalsPassThroughLive(t *testing.T) {
	t.Parallel()

	transit := &fakeTransit{arrivals: []observation.Record{arrivalRecord("101", "3", 7)}}
	srv := openTransit(t, transit)

	resp := do(t, srv, http.MethodGet, "/v1/transit/arrivals?stop=101", authorized())
	if resp.Status != http.StatusOK {
		t.Fatalf("status = %d", resp.Status)
	}

	body := resp.Body
	if body["count"] != float64(1) {
		t.Fatalf("count = %v", body["count"])
	}
	if body["stale"] != false {
		t.Errorf("stale = %v, want false on a fresh read", body["stale"])
	}

	arrivals, _ := body["arrivals"].([]any)
	first, _ := arrivals[0].(map[string]any)
	for _, field := range []string{"stop_id", "stop_name", "line", "destination", "eta_minutes", "observed_at"} {
		if _, ok := first[field]; !ok {
			t.Errorf("arrival is missing %q: %v", field, first)
		}
	}
	if first["eta_minutes"] != float64(7) {
		t.Errorf("eta_minutes = %v", first["eta_minutes"])
	}
	if len(transit.stopsAsked) != 1 || transit.stopsAsked[0] != "101" {
		t.Errorf("stops asked = %v", transit.stopsAsked)
	}
}

// A browser on a refresh loop must not turn one reader into a load generator
// against somebody else's server.
func TestArrivalsAreCachedBriefly(t *testing.T) {
	t.Parallel()

	transit := &fakeTransit{arrivals: []observation.Record{arrivalRecord("101", "3", 7)}}
	srv := openTransit(t, transit)

	for range 4 {
		resp := do(t, srv, http.MethodGet, "/v1/transit/arrivals?stop=101", authorized())
		if resp.Status != http.StatusOK {
			t.Fatalf("status = %d", resp.Status)
		}
	}
	if got := transit.count(); got != 1 {
		t.Errorf("the operator was asked %d times for four requests, want 1", got)
	}

	// A different stop is a different question, and is asked.
	do(t, srv, http.MethodGet, "/v1/transit/arrivals?stop=202", authorized())
	if got := transit.count(); got != 2 {
		t.Errorf("calls = %d, want a second stop to be a second question", got)
	}
}

func TestArrivalsResolveAStopByName(t *testing.T) {
	t.Parallel()

	transit := &fakeTransit{
		resolved: []string{"101", "102"},
		arrivals: []observation.Record{arrivalRecord("101", "3", 4)},
	}
	srv := openTransit(t, transit)

	resp := do(t, srv, http.MethodGet, "/v1/transit/arrivals?text=tejares", authorized())
	if resp.Status != http.StatusOK {
		t.Fatalf("status = %d", resp.Status)
	}
	if len(transit.stopsAsked) != 2 {
		t.Errorf("stops asked = %v, want the resolved ones", transit.stopsAsked)
	}
}

func TestArrivalsNeedAStop(t *testing.T) {
	t.Parallel()

	srv := openTransit(t, &fakeTransit{})
	resp := do(t, srv, http.MethodGet, "/v1/transit/arrivals", authorized())

	if resp.Status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.Status)
	}
}

// ADR-0008: an undocumented personal source is never redistributed. Serving
// live arrivals over HTTP is redistribution unless the deployment is both
// opted in and private.
func TestArrivalsAreRefusedUnlessBothGatesAreOpen(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		personal bool
		token    string
	}{
		{name: "no opt-in and no token"},
		{name: "opted in but open to anyone", personal: true},
		{name: "private but not opted in", token: "s3cret"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := []api.Option{
				api.WithTransit(&fakeTransit{}),
				api.WithPersonalSources(tc.personal),
			}
			if tc.token != "" {
				opts = append(opts, api.WithToken(tc.token))
			}
			srv := serveWithPersonalSource(t, &fakeStore{}, opts...)

			headers := map[string]string{}
			if tc.token != "" {
				headers["Authorization"] = "Bearer " + tc.token
			}

			resp := do(t, srv, http.MethodGet, "/v1/transit/arrivals?stop=101", headers)
			if resp.Status != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", resp.Status)
			}

			body := resp.Body
			if body["error"] != "personal source not served" {
				t.Errorf("error = %v", body["error"])
			}
			if body["detail"] == nil {
				t.Error("the refusal says nothing about which gate is closed")
			}
		})
	}
}

// A build wired without a live reader says so, rather than reporting the
// operator's server as broken.
func TestArrivalsWithoutATransitPortAre503(t *testing.T) {
	t.Parallel()

	srv := serveWithPersonalSource(t, &fakeStore{},
		api.WithPersonalSources(true), api.WithToken("s3cret"))

	resp := do(t, srv, http.MethodGet, "/v1/transit/arrivals?stop=101", authorized())
	if resp.Status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.Status)
	}
	if body := resp.Body; body["error"] != "transit not configured" {
		t.Errorf("error = %v", body["error"])
	}
}

// The operator's endpoint fails often; that is the deal with an undocumented
// source. It must fail as a gateway error, not as an eye bug.
func TestArrivalsSurfaceAReaderFailure(t *testing.T) {
	t.Parallel()

	srv := openTransit(t, &fakeTransit{err: errors.New("all stops refused")})
	resp := do(t, srv, http.MethodGet, "/v1/transit/arrivals?stop=101", authorized())

	if resp.Status != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.Status)
	}
}

func TestDepartures(t *testing.T) {
	t.Parallel()

	departure := record("renfe:1", "transport")
	departure.Source = "renfe-cordoba-departures"
	departure.Kind = "scheduled_departure"
	when := time.Date(2026, time.August, 28, 13, 30, 0, 0, time.UTC)
	departure.ValidFrom = &when
	departure.Payload = json.RawMessage(
		`{"stop_id":"50500","stop_name":"Cordoba","route":"AVE","headsign":"Madrid"}`)

	transit := &fakeTransit{departures: []observation.Record{departure}}
	srv := openTransit(t, transit)

	resp := do(t, srv, http.MethodGet, "/v1/transit/departures?station=CORDOBA&limit=5", authorized())
	if resp.Status != http.StatusOK {
		t.Fatalf("status = %d", resp.Status)
	}

	body := resp.Body
	if body["count"] != float64(1) {
		t.Fatalf("count = %v", body["count"])
	}
	departures, _ := body["departures"].([]any)
	first, _ := departures[0].(map[string]any)
	if first["destination"] != "Madrid" {
		t.Errorf("destination = %v", first["destination"])
	}
	if first["station"] != "Cordoba" {
		t.Errorf("station = %v", first["station"])
	}
	if first["scheduled_at"] != when.Format(time.RFC3339) {
		t.Errorf("scheduled_at = %v", first["scheduled_at"])
	}
}

// Departures are a published timetable, not a personal source: they need no
// opt-in, only a wired reader.
func TestDeparturesNeedNoPersonalOptIn(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(api.New(&fakeStore{}, nil, logging.Discard(),
		api.WithTransit(&fakeTransit{})).Handler())
	t.Cleanup(srv.Close)

	resp := do(t, srv, http.MethodGet, "/v1/transit/departures", nil)
	if resp.Status != http.StatusOK {
		t.Fatalf("status = %d", resp.Status)
	}
}

func TestTransitLinesComeFromTheStore(t *testing.T) {
	t.Parallel()

	line := record("aucorsa-lines:line:3", "transport")
	line.Source = "aucorsa-lines"
	line.Kind = "bus_line"
	line.Title = "Línea 3 · 41 paradas · 06:30"
	line.Payload = json.RawMessage(`{"line":"3","stops":41,"schedule":["06:30 a 23:00"]}`)

	srv := serve(t, &fakeStore{records: []observation.Record{line}})
	status, body := get(t, srv, "/v1/transit/lines")

	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	lines, _ := body["lines"].([]any)
	if len(lines) != 1 {
		t.Fatalf("lines = %v", body)
	}

	first, _ := lines[0].(map[string]any)
	if first["code"] != "3" {
		t.Errorf("code = %v", first["code"])
	}
	if first["stops"] != float64(41) {
		t.Errorf("stops = %v", first["stops"])
	}
	hours, _ := first["service_hours"].([]any)
	if len(hours) != 1 {
		t.Errorf("service_hours = %v", first["service_hours"])
	}
	if first["name"] == "" || first["name"] == nil {
		t.Errorf("name = %v", first["name"])
	}
}

func TestTransitStopsComeFromTheStore(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	stop := observation.Entity{
		ID: "aucorsa-lines:101", Source: "aucorsa-lines", Kind: "bus_stop", Topic: "transport",
		Title: "Ronda de los Tejares", FirstSeen: at, LastSeen: at,
		Payload: json.RawMessage(`{"stop_id":"101","lines":["3","5"]}`),
		Provenance: observation.Provenance{
			Publisher: "AUCORSA", SourceURL: "https://example.org", License: "unspecified", FetchedAt: at,
		},
	}

	store := &fakeStore{entities: []observation.Entity{stop}}
	srv := serve(t, store)

	status, body := get(t, srv, "/v1/transit/stops?text=tejares&limit=3")
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}

	// The store is asked for stops, not for everything.
	if len(store.lastFilter.Kinds) == 0 {
		t.Error("the store was not asked for stop entities")
	}
	if store.lastFilter.Text != "tejares" || store.lastFilter.Limit != 3 {
		t.Errorf("filter = %+v", store.lastFilter)
	}

	stops, _ := body["stops"].([]any)
	if len(stops) != 1 {
		t.Fatalf("stops = %v", body)
	}
	first, _ := stops[0].(map[string]any)
	if first["id"] != "101" {
		t.Errorf("id = %v, want the number on the pole", first["id"])
	}
	if first["name"] != "Ronda de los Tejares" {
		t.Errorf("name = %v", first["name"])
	}
}

// "The operator's endpoint did not answer" and "this deployment has no reader
// for that" are different facts. Reporting the second as the first sends
// somebody to look at AUCORSA's servers over their own configuration.
func TestArrivalsSeparateAMissingReaderFromAFailingOne(t *testing.T) {
	t.Parallel()

	srv := openTransit(t, &fakeTransit{err: api.ErrTransitUnavailable})
	resp := do(t, srv, http.MethodGet, "/v1/transit/arrivals?stop=101", authorized())

	if resp.Status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.Status)
	}
	if resp.Body["error"] != "transit not configured" {
		t.Errorf("error = %v", resp.Body["error"])
	}
}

func TestDeparturesSayWhenNoReaderIsWired(t *testing.T) {
	t.Parallel()

	srv := openTransit(t, &fakeTransit{err: api.ErrTransitUnavailable})
	resp := do(t, srv, http.MethodGet, "/v1/transit/departures", authorized())

	if resp.Status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.Status)
	}
}
