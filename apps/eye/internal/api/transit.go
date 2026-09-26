package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
)

// arrivalsTTL is how long a live arrivals answer is reused.
//
// Twenty seconds is short enough that nobody watching a bus stop notices, and
// long enough that a browser on a refresh loop cannot turn one reader into a
// load generator against somebody else's server. The endpoint eye reads here
// is the operator's own, undocumented and unmetered by us; being gentle with
// it is part of the deal in ADR-0008.
const arrivalsTTL = 20 * time.Second

// maxResolvedStops caps how many directory matches one search will poll. A
// vague term matches many stops, and quietly firing a request at each of them
// is not a reasonable thing to do to somebody else's server.
const maxResolvedStops = 5

// defaultStation is the station /v1/transit/departures answers for.
const defaultStation = "CORDOBA"

// ErrTransitUnavailable is what a Transit adapter returns when it cannot
// answer on this machine at all: no such source in the registry, or the
// personal-source gate closed.
//
// It is distinct from a reader that tried and failed. "The operator's endpoint
// did not answer" and "this deployment has no reader for that" are different
// facts, and reporting the second as the first sends somebody to look at
// AUCORSA's servers when the problem is their own configuration.
var ErrTransitUnavailable = errors.New("transit not configured")

// Transit is the live transit passthrough.
//
// It is a port rather than a provider because the API must not know that
// AUCORSA exists, let alone how its token rotates. The adapter lives in
// internal/cli, next to the command that already reads the same source.
type Transit interface {
	// Arrivals returns live estimates for explicit stop numbers.
	Arrivals(ctx context.Context, stops []string) ([]observation.Record, error)
	// ResolveStops turns a name into stop numbers, capped by the adapter.
	ResolveStops(ctx context.Context, text string) ([]string, error)
	// Departures returns published departures from a station.
	Departures(ctx context.Context, station string, limit int) ([]observation.Record, error)
}

// arrivalsCache holds recent live answers, keyed by the stops asked about.
type arrivalsCache struct {
	mu      sync.Mutex
	entries map[string]arrivalsEntry
	ttl     time.Duration
	now     func() time.Time
}

// arrivalsEntry is one cached answer.
type arrivalsEntry struct {
	records []observation.Record
	at      time.Time
}

// newArrivalsCache builds a cache with the given lifetime.
func newArrivalsCache(ttl time.Duration, now func() time.Time) *arrivalsCache {
	return &arrivalsCache{entries: map[string]arrivalsEntry{}, ttl: ttl, now: now}
}

// get returns a cached answer while it is still fresh.
func (c *arrivalsCache) get(key string) ([]observation.Record, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok || c.now().Sub(entry.at) >= c.ttl {
		return nil, false
	}
	return entry.records, true
}

// put stores an answer.
func (c *arrivalsCache) put(key string, records []observation.Record) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = arrivalsEntry{records: records, at: c.now()}
}

// arrivalView is one live estimate as this API serves it.
type arrivalView struct {
	StopID   string `json:"stop_id"`
	StopName string `json:"stop_name"`
	Line     string `json:"line"`
	// Destination is the route the operator prints on the bus.
	Destination string `json:"destination"`
	// ETAMinutes is the operator's own prediction, not a measurement.
	ETAMinutes int       `json:"eta_minutes"`
	ObservedAt time.Time `json:"observed_at"`
	Occupancy  string    `json:"occupancy,omitempty"`
}

// arrivalPayload is the shape the AUCORSA reader records.
type arrivalPayload struct {
	StopID    string `json:"stop_id"`
	StopName  string `json:"stop_name"`
	Line      string `json:"line"`
	Route     string `json:"route"`
	Minutes   int    `json:"minutes"`
	Occupancy string `json:"occupancy"`
}

// handleTransitArrivals serves live arrival estimates.
//
// This is the one endpoint that leaves the machine on request, and the one
// that reads a source eye may not redistribute. Both facts are enforced here
// rather than documented somewhere else.
func (s *Server) handleTransitArrivals(w http.ResponseWriter, r *http.Request) {
	if !s.mayServePersonalSources() {
		writeErrorDetail(w, http.StatusForbidden, "personal source not served",
			"Live arrivals come from an undocumented personal source (ADR-0008). "+
				"Serving them needs EYE_ALLOW_PERSONAL_SOURCES=1 and EYE_API_TOKEN set, "+
				"so the data cannot leave an authenticated deployment.")
		return
	}
	if s.transit == nil {
		writeError(w, http.StatusServiceUnavailable, "transit not configured")
		return
	}

	stops, err := s.stopsAskedFor(r)
	if err != nil {
		if errors.Is(err, ErrTransitUnavailable) {
			writeError(w, http.StatusServiceUnavailable, "transit not configured")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(stops) == 0 {
		writeError(w, http.StatusBadRequest,
			"stop is required: pass stop=<pole number> or text=<stop name>")
		return
	}

	key := strings.Join(stops, ",")
	records, cached := s.arrivals.get(key)
	if !cached {
		records, err = s.transit.Arrivals(r.Context(), stops)
		if errors.Is(err, ErrTransitUnavailable) {
			writeError(w, http.StatusServiceUnavailable, "transit not configured")
			return
		}
		if err != nil {
			s.log.Warn("live arrivals failed", "stops", key, "error", err)
			writeErrorDetail(w, http.StatusBadGateway, "arrivals unavailable",
				"The operator's endpoint did not answer. It is undocumented and may "+
					"change without notice; that is the deal recorded in ADR-0008.")
			return
		}
		s.arrivals.put(key, records)
	}

	arrivals := make([]arrivalView, 0, len(records))
	for _, rec := range records {
		var payload arrivalPayload
		if err := json.Unmarshal(rec.Payload, &payload); err != nil {
			continue
		}
		arrivals = append(arrivals, arrivalView{
			StopID: payload.StopID, StopName: payload.StopName,
			Line: payload.Line, Destination: payload.Route,
			ETAMinutes: payload.Minutes, ObservedAt: rec.ObservedAt.UTC(),
			Occupancy: payload.Occupancy,
		})
	}
	sort.SliceStable(arrivals, func(i, j int) bool {
		return arrivals[i].ETAMinutes < arrivals[j].ETAMinutes
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"count":    len(arrivals),
		"arrivals": arrivals,
		// Whether this answer came from the cache rather than from the
		// operator just now. A reader deciding how much to trust an
		// estimate should not have to guess.
		"stale": cached,
		"note": "Operator estimates, not measurements. Personal source: " +
			"never redistributed beyond this deployment.",
	})
}

// mayServePersonalSources reports whether both gates of ADR-0008 are open.
//
// The opt-in says this machine may READ the source. The token says the
// deployment is private, which is what makes serving it something other than
// redistribution. Either alone is not enough.
func (s *Server) mayServePersonalSources() bool {
	return s.allowPersonalSources && s.token != ""
}

// stopsAskedFor resolves the stops one request is about.
func (s *Server) stopsAskedFor(r *http.Request) ([]string, error) {
	q := r.URL.Query()

	if explicit := splitList(q.Get("stop")); len(explicit) > 0 {
		return explicit, nil
	}

	text := strings.TrimSpace(q.Get("text"))
	if text == "" {
		return nil, nil
	}

	resolved, err := s.transit.ResolveStops(r.Context(), text)
	if err != nil {
		return nil, err
	}
	if len(resolved) > maxResolvedStops {
		resolved = resolved[:maxResolvedStops]
	}
	return resolved, nil
}

// departureView is one published departure as this API serves it.
type departureView struct {
	Station     string    `json:"station"`
	StopID      string    `json:"stop_id,omitempty"`
	Destination string    `json:"destination"`
	Route       string    `json:"route,omitempty"`
	TripID      string    `json:"trip_id,omitempty"`
	ScheduledAt time.Time `json:"scheduled_at"`
}

// departurePayload is the shape the GTFS provider records.
type departurePayload struct {
	StopID   string `json:"stop_id"`
	StopName string `json:"stop_name"`
	TripID   string `json:"trip_id"`
	Route    string `json:"route"`
	Headsign string `json:"headsign"`
}

// handleTransitDepartures serves the published timetable for a station.
//
// Unlike arrivals this is an open, licensed feed, so it needs no opt-in: only
// a reader wired to answer it.
func (s *Server) handleTransitDepartures(w http.ResponseWriter, r *http.Request) {
	if s.transit == nil {
		writeError(w, http.StatusServiceUnavailable, "transit not configured")
		return
	}

	q := r.URL.Query()
	station := strings.TrimSpace(q.Get("station"))
	if station == "" {
		station = defaultStation
	}

	limit, err := intParam(q.Get("limit"), defaultLimit)
	if err != nil {
		writeError(w, http.StatusBadRequest, "limit: "+err.Error())
		return
	}

	records, err := s.transit.Departures(r.Context(), station, min(limit, maxLimit))
	if errors.Is(err, ErrTransitUnavailable) {
		writeError(w, http.StatusServiceUnavailable, "transit not configured")
		return
	}
	if err != nil {
		s.log.Warn("departures failed", "station", station, "error", err)
		writeError(w, http.StatusBadGateway, "departures unavailable")
		return
	}

	departures := make([]departureView, 0, len(records))
	for _, rec := range records {
		var payload departurePayload
		if err := json.Unmarshal(rec.Payload, &payload); err != nil {
			continue
		}

		view := departureView{
			Station: payload.StopName, StopID: payload.StopID,
			Destination: firstNonEmpty(payload.Headsign, payload.Route, payload.TripID),
			Route:       payload.Route, TripID: payload.TripID,
		}
		if rec.ValidFrom != nil {
			view.ScheduledAt = rec.ValidFrom.UTC()
		}
		departures = append(departures, view)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"station": station, "count": len(departures), "departures": departures,
	})
}

// lineKinds are the entity kinds a bus line may be stored under.
var lineKinds = []string{"line", "bus_line"}

// stopKinds are the entity kinds a bus stop may be stored under. eye holds
// AUCORSA's own directory and the municipal cartography layer, and they do not
// agree on a name.
var stopKinds = []string{"stop", "bus_stop", "bus"}

// lineView is one bus line as this API serves it.
type lineView struct {
	Code         string   `json:"code"`
	Name         string   `json:"name"`
	ServiceHours []string `json:"service_hours"`
	Stops        int      `json:"stops"`
	Source       string   `json:"source"`
}

// linePayload is the shape the AUCORSA lines provider records.
type linePayload struct {
	Line     string   `json:"line"`
	Stops    int      `json:"stops"`
	Schedule []string `json:"schedule"`
}

// handleTransitLines serves the bus lines eye holds.
//
// Lines are read from the store, not from the operator: they change a few
// times a year, and asking somebody's website on every request for something
// that changes in April is not reading, it is hammering.
func (s *Server) handleTransitLines(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, err := intParam(q.Get("limit"), defaultLimit)
	if err != nil {
		writeError(w, http.StatusBadRequest, "limit: "+err.Error())
		return
	}

	filter := observation.Filter{
		Kinds: lineKinds,
		Text:  strings.TrimSpace(q.Get("text")),
		Limit: min(limit, maxLimit),
	}

	records, err := s.store.Query(r.Context(), filter)
	if err != nil {
		s.log.Error("line query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	records = s.redistributable(records)

	lines := make([]lineView, 0, len(records))
	seen := map[string]bool{}

	for _, rec := range records {
		var payload linePayload
		if err := json.Unmarshal(rec.Payload, &payload); err != nil {
			continue
		}
		if payload.Line == "" || seen[payload.Line] {
			continue
		}
		seen[payload.Line] = true

		lines = append(lines, lineView{
			Code: payload.Line, Name: rec.Title,
			ServiceHours: orEmptyList(payload.Schedule),
			Stops:        payload.Stops, Source: rec.Source,
		})
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].Code < lines[j].Code })

	writeJSON(w, http.StatusOK, map[string]any{"count": len(lines), "lines": lines})
}

// stopView is one bus stop as this API serves it.
type stopView struct {
	// ID is the number printed on the pole, which is also what the live
	// arrivals endpoint expects. The operator's pages carry a second,
	// unrelated number, and using that one silently returns nothing.
	ID       string             `json:"id"`
	EntityID string             `json:"entity_id"`
	Name     string             `json:"name"`
	Lines    []string           `json:"lines,omitempty"`
	Position *observation.Point `json:"position,omitempty"`
	Source   string             `json:"source"`
}

// stopPayload is the shape the stop providers record.
type stopPayload struct {
	StopID string   `json:"stop_id"`
	Lines  []string `json:"lines"`
}

// handleTransitStops serves the stops eye holds.
func (s *Server) handleTransitStops(w http.ResponseWriter, r *http.Request) {
	filter, err := filterFrom(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	filter.Kinds = stopKinds

	entities, err := s.store.Entities(r.Context(), filter)
	if err != nil {
		s.log.Error("stop query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	stops := make([]stopView, 0, len(entities))
	for _, ent := range entities {
		if s.nonRedistributable[ent.Source] {
			continue
		}

		var payload stopPayload
		_ = json.Unmarshal(ent.Payload, &payload)

		id := payload.StopID
		if id == "" {
			id = ent.ID
		}
		stops = append(stops, stopView{
			ID: id, EntityID: ent.ID, Name: ent.Title,
			Lines: payload.Lines, Position: ent.Position, Source: ent.Source,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"count": len(stops), "stops": stops})
}

// orEmptyList renders an absent list as an empty one.
func orEmptyList(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// firstNonEmpty returns the first value that is not blank.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
