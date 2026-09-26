// Package api serves eye's observations over HTTP.
//
// This is what makes eye a gateway rather than a command: providers are written
// once, and a web map, a chatbot, a notebook or a shell script all read the same
// normalized records with the same provenance attached.
//
// It is one adapter among several and holds no privileges. Nothing in the
// application layer knows it exists, and eye works completely without it.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
	"github.com/FullFran/cordvba/apps/eye/internal/version"
)

// maxLimit caps how many records one request may take. A gateway that will
// serialise its whole store on request is a denial-of-service waiting for a
// curious client.
const maxLimit = 5000

// defaultLimit applies when a caller does not ask.
const defaultLimit = 200

// Store is what the API needs from the observation store. It is a narrow port
// rather than the concrete type, so the handlers are testable without a
// database and cannot reach for anything they should not.
type Store interface {
	Query(ctx context.Context, f observation.Filter) ([]observation.Record, error)
	Entities(ctx context.Context, f observation.Filter) ([]observation.Entity, error)
	Counts(ctx context.Context) (records, entities int, err error)
	// States returns what eye remembers about polling each source. It is
	// what turns the registry view into an honest one: a source eye is
	// allowed to read and has not managed to read for a week is not the
	// same as one that works.
	States(ctx context.Context) ([]SourceState, error)
}

// SourceState is the stored polling health of one source, as the API serves it.
//
// It is the API's own type rather than the store's: the port is what the
// handlers need, not what the database happens to keep.
type SourceState struct {
	SourceID          string
	LastAttempt       time.Time
	LastSuccess       time.Time
	LastError         string
	Records           int
	ConsecutiveErrors int
}

// Aggregate is a summary of everything the store holds. It is computed by the
// store rather than by counting a page of records, because a count that is
// capped at a page size is a wrong count.
type Aggregate struct {
	Records  int
	Entities int
	ByTopic  map[string]int
	BySource map[string]int
	ByKind   map[string]int
	Oldest   time.Time
	Newest   time.Time
}

// Aggregator is the optional port for /v1/stats. A store that cannot aggregate
// still serves everything else; the endpoint says so instead of guessing.
type Aggregator interface {
	Aggregate(ctx context.Context) (Aggregate, error)
}

// Sizer is the optional port for the size of the store on disk. An in-memory
// store has no bytes to report, and reports none.
type Sizer interface {
	StoreBytes() (int64, error)
}

// Server serves the read API.
type Server struct {
	store   Store
	sources []source.Source
	log     *slog.Logger
	started time.Time

	// token guards /v1 when it is set. Empty means the API is open, which
	// is the right default for a loopback bind and never for a public one —
	// `eye serve --public` refuses without a token.
	token string
	// corsOrigin is what the API answers in Access-Control-Allow-Origin.
	corsOrigin string

	// transit is the live passthrough port. It is nil when this build was
	// wired without one, and the transit endpoints say so rather than
	// pretending the operator's server is down.
	transit Transit
	// allowPersonalSources mirrors EYE_ALLOW_PERSONAL_SOURCES. The live
	// arrivals endpoint needs it AND a token: see ADR-0008.
	allowPersonalSources bool
	// arrivals caches live estimates for a few seconds, so a browser on a
	// refresh loop cannot turn one reader into a load generator against
	// somebody else's server.
	arrivals *arrivalsCache

	// ui is the embedded web console, when this build was wired with one.
	// It takes the root, and every path no API route claims, because that
	// is where a person types a URL. Machines read /v1 and /openapi.json,
	// which are routes of their own and stay exactly where they were.
	ui http.Handler

	// now exists so the cache can be tested without sleeping.
	now func() time.Time

	// nonRedistributable is the set of source ids whose records eye must not
	// serve to anyone else: undocumented personal sources, and anything read
	// under terms that permit personal use only.
	//
	// Filtering here rather than trusting every future handler is the point.
	// A rule that lives in one place cannot be forgotten in the next one.
	nonRedistributable map[string]bool
}

// Option configures a Server at construction.
type Option func(*Server)

// WithToken guards every /v1 endpoint with a bearer token.
func WithToken(token string) Option {
	return func(s *Server) { s.token = strings.TrimSpace(token) }
}

// WithUI serves a web console at the root instead of the JSON service
// description.
//
// The handler receives every request no API route matched, assets included, so
// it must 404 on its own unknown paths rather than answering everything. Go's
// ServeMux gives a longer pattern priority over "/", which is what keeps
// /v1/records an API call and not a web page — there is a test for exactly
// that, because it is the mistake this option invites.
func WithUI(ui http.Handler) Option {
	return func(s *Server) { s.ui = ui }
}

// WithCORSOrigin sets the Access-Control-Allow-Origin the API answers.
func WithCORSOrigin(origin string) Option {
	return func(s *Server) {
		if origin = strings.TrimSpace(origin); origin != "" {
			s.corsOrigin = origin
		}
	}
}

// WithTransit wires the live transit passthrough.
func WithTransit(t Transit) Option {
	return func(s *Server) { s.transit = t }
}

// WithPersonalSources records that this machine opted into undocumented
// personal sources. It is one half of the gate on live arrivals; the other is
// a configured token.
func WithPersonalSources(allowed bool) Option {
	return func(s *Server) { s.allowPersonalSources = allowed }
}

// New builds a server over a store and the registry.
func New(store Store, sources []source.Source, log *slog.Logger, opts ...Option) *Server {
	if log == nil {
		log = slog.Default()
	}
	blocked := map[string]bool{}
	for _, src := range sources {
		if !src.Redistributable() {
			blocked[src.ID] = true
		}
	}

	s := &Server{
		store: store, sources: sources, log: log,
		started: time.Now().UTC(), nonRedistributable: blocked,
		corsOrigin: "*",
		now:        func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(s)
	}
	s.arrivals = newArrivalsCache(arrivalsTTL, func() time.Time { return s.now() })

	return s
}

// redistributable drops records from sources eye may not serve onward.
func (s *Server) redistributable(records []observation.Record) []observation.Record {
	if len(s.nonRedistributable) == 0 {
		return records
	}

	out := make([]observation.Record, 0, len(records))
	for _, r := range records {
		if s.nonRedistributable[r.Source] {
			continue
		}
		out = append(out, r)
	}
	return out
}

// Handler returns the routed handler.
//
// Every route comes from the same table the OpenAPI document is generated
// from, so an endpoint cannot exist in one and be missing from the other.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	for _, rt := range s.routes() {
		mux.HandleFunc(rt.Method+" "+rt.Path, rt.handler)
	}

	return s.withCommonHeaders(s.withAuth(mux))
}

// withCommonHeaders applies the headers every response needs.
func (s *Server) withCommonHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read-only API, so a permissive CORS policy costs nothing and lets
		// a browser-based map consume it directly. An operator who fronts
		// it with an application of their own narrows it.
		s.writeCORS(w)
		w.Header().Set("Cache-Control", "no-store")
		// Every response says where the data came from, in the same place a
		// machine client will look for it.
		w.Header().Set("X-Eye-Provenance", "public sources; see the license field on every record")

		// The preflight is answered before anything else looks at the
		// request: a browser sends it before it sends the token, so a
		// guarded preflight fails every cross-origin call before it is
		// made.
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleRoot answers the two audiences the root has.
//
// A person who opens it in a browser gets the console, when this build has one.
// A script that curls it gets the service description, which is also what a
// build without a console answers — the machine-readable index at /v1 and the
// OpenAPI document are unaffected either way.
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if s.ui != nil {
		s.ui.ServeHTTP(w, r)
		return
	}
	s.handleIndex(w, r)
}

// handleIndex describes the API, so a person who curls the root learns what to
// ask for instead of getting a 404.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		writeError(w, http.StatusNotFound, "no such endpoint")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"service":   "eye",
		"version":   version.Version,
		"about":     "A live, source-verifiable model of Córdoba, from public sources only.",
		"index":     "/v1",
		"openapi":   "/openapi.json",
		"endpoints": s.endpointIndex(),
		"note": "Every record carries its publisher, licence and both timestamps. " +
			"Camera images are never served through this API.",
	})
}

// handleV1Index describes the versioned API.
func (s *Server) handleV1Index(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version":       "v1",
		"authenticated": s.token != "",
		"endpoints":     s.endpointIndex(),
		"openapi":       "/openapi.json",
	})
}

// endpointIndex renders the route table as "METHOD /path" to a description.
func (s *Server) endpointIndex() map[string]string {
	out := make(map[string]string, len(s.routes()))
	for _, rt := range s.routes() {
		out[rt.Method+" "+rt.Path] = rt.Summary
	}
	return out
}

// handleHealth reports liveness and how much the store holds.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	records, entities, err := s.store.Counts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store unavailable")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"uptime_s": int(time.Since(s.started).Seconds()),
		"records":  records,
		"entities": entities,
		"sources":  len(s.sources),
		"version":  version.Version,
	})
}

// handleRecords serves observations.
func (s *Server) handleRecords(w http.ResponseWriter, r *http.Request) {
	filter, err := filterFrom(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	records, err := s.store.Query(r.Context(), filter)
	if err != nil {
		s.log.Error("query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	records = s.redistributable(records)
	writeJSON(w, http.StatusOK, map[string]any{
		"count":   len(records),
		"records": records,
	})
}

// handleEntities serves inventory.
func (s *Server) handleEntities(w http.ResponseWriter, r *http.Request) {
	filter, err := filterFrom(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	entities, err := s.store.Entities(r.Context(), filter)
	if err != nil {
		s.log.Error("entity query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"count":    len(entities),
		"entities": entities,
	})
}

// handleSources serves the registry, including what is held and why, and how
// each source is actually doing.
//
// The registry alone says what eye is ALLOWED to read. Stored health says what
// it has MANAGED to read, and a source that has been failing for a week looks
// exactly like a working one without it.
func (s *Server) handleSources(w http.ResponseWriter, r *http.Request) {
	type view struct {
		ID         string `json:"id"`
		Authority  string `json:"authority"`
		Topic      string `json:"topic"`
		Format     string `json:"format"`
		License    string `json:"license"`
		Access     string `json:"access"`
		Automation string `json:"automation"`
		Pollable   bool   `json:"pollable"`
		// Redistributable is false for sources eye reads for the operator
		// only. Their records never leave this deployment.
		Redistributable bool   `json:"redistributable"`
		URL             string `json:"url"`
		Notes           string `json:"notes,omitempty"`

		// Stored health. Absent rather than zero when eye has never polled
		// the source: a zero timestamp is a lie with a date on it.
		LastAttempt       *string `json:"last_attempt,omitempty"`
		LastSuccess       *string `json:"last_success,omitempty"`
		LastError         string  `json:"last_error,omitempty"`
		Records           int     `json:"records"`
		ConsecutiveErrors int     `json:"consecutive_errors"`
	}

	health := s.sourceHealth(r.Context())

	out := make([]view, 0, len(s.sources))
	for _, src := range s.sources {
		entry := view{
			ID: src.ID, Authority: src.Authority, Topic: src.Topic, Format: src.Format,
			License: src.License, Access: string(src.Access), Automation: string(src.Automation),
			Pollable: src.Automation.Pollable(), Redistributable: src.Redistributable(),
			URL: src.URL, Notes: src.Notes,
		}
		if state, known := health[src.ID]; known {
			entry.LastAttempt = timestamp(state.LastAttempt)
			entry.LastSuccess = timestamp(state.LastSuccess)
			entry.LastError = state.LastError
			entry.Records = state.Records
			entry.ConsecutiveErrors = state.ConsecutiveErrors
		}
		out = append(out, entry)
	}

	writeJSON(w, http.StatusOK, map[string]any{"count": len(out), "sources": out})
}

// sourceHealth reads stored polling health, keyed by source id.
//
// A store that cannot answer costs the health columns and nothing else: the
// registry is worth serving on its own, and failing the whole endpoint because
// one optional column is missing would be the wrong trade.
func (s *Server) sourceHealth(ctx context.Context) map[string]SourceState {
	states, err := s.store.States(ctx)
	if err != nil {
		s.log.Warn("source health unavailable", "error", err)
		return nil
	}

	out := make(map[string]SourceState, len(states))
	for _, state := range states {
		out[state.SourceID] = state
	}
	return out
}

// timestamp renders an optional time, absent when it was never set.
func timestamp(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	rendered := t.UTC().Format(time.RFC3339)
	return &rendered
}

// filterFrom builds a domain filter from query parameters.
func filterFrom(r *http.Request) (observation.Filter, error) {
	q := r.URL.Query()

	f := observation.Filter{
		Topics:  splitList(q.Get("topic")),
		Sources: splitList(q.Get("source")),
		Kinds:   splitList(q.Get("kind")),
		Text:    strings.TrimSpace(q.Get("text")),
	}

	limit, err := intParam(q.Get("limit"), defaultLimit)
	if err != nil {
		return f, fmt.Errorf("limit: %w", err)
	}
	f.Limit = min(limit, maxLimit)

	if v := q.Get("since"); v != "" {
		since, err := timeParam(v)
		if err != nil {
			return f, fmt.Errorf("since: %w", err)
		}
		f.Since = &since
	}
	if v := q.Get("until"); v != "" {
		until, err := timeParam(v)
		if err != nil {
			return f, fmt.Errorf("until: %w", err)
		}
		f.Until = &until
	}
	if v := q.Get("min_severity"); v != "" {
		severity, err := intParam(v, 0)
		if err != nil {
			return f, fmt.Errorf("min_severity: %w", err)
		}
		f.MinSeverity = observation.Severity(severity)
	}
	if v := q.Get("bbox"); v != "" {
		box, err := bboxParam(v)
		if err != nil {
			return f, fmt.Errorf("bbox: %w", err)
		}
		f.BBox = &box
	}
	if v := q.Get("near"); v != "" {
		point, err := pointParam(v)
		if err != nil {
			return f, fmt.Errorf("near: %w", err)
		}
		f.Near = &point

		radius, err := floatParam(q.Get("radius_km"), 10)
		if err != nil {
			return f, fmt.Errorf("radius_km: %w", err)
		}
		f.RadiusKm = radius
	}

	return f, nil
}

// splitList parses a comma-separated parameter.
func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// intParam parses an integer parameter with a default.
func intParam(s string, fallback int) (int, error) {
	if strings.TrimSpace(s) == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, errors.New("must be a non-negative integer")
	}
	return n, nil
}

// floatParam parses a float parameter with a default.
func floatParam(s string, fallback float64) (float64, error) {
	if strings.TrimSpace(s) == "" {
		return fallback, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v <= 0 {
		return 0, errors.New("must be a positive number")
	}
	return v, nil
}

// timeParam accepts an RFC 3339 timestamp or a Go duration meaning "ago".
//
// Both forms are supported because both are natural: a script has a timestamp,
// a person has "the last two hours".
func timeParam(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return time.Now().UTC().Add(-d), nil
	}
	return time.Time{}, errors.New("must be an RFC 3339 timestamp or a duration such as 2h")
}

// pointParam parses "lat,lon".
func pointParam(s string) (observation.Point, error) {
	lat, lon, ok := strings.Cut(s, ",")
	if !ok {
		return observation.Point{}, errors.New("must be lat,lon")
	}

	latVal, err1 := strconv.ParseFloat(strings.TrimSpace(lat), 64)
	lonVal, err2 := strconv.ParseFloat(strings.TrimSpace(lon), 64)
	if err1 != nil || err2 != nil {
		return observation.Point{}, errors.New("must be numeric lat,lon")
	}

	p := observation.Point{Lat: latVal, Lon: lonVal}
	if !p.Valid() {
		return observation.Point{}, errors.New("is outside WGS84")
	}
	return p, nil
}

// bboxParam parses "west,south,east,north", the order the source APIs use.
func bboxParam(s string) (observation.BBox, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return observation.BBox{}, errors.New("must be west,south,east,north")
	}

	var values [4]float64
	for i, p := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return observation.BBox{}, errors.New("must be four numbers")
		}
		values[i] = v
	}

	box := observation.BBox{West: values[0], South: values[1], East: values[2], North: values[3]}
	if !box.Valid() {
		return observation.BBox{}, errors.New("is not a valid box")
	}
	return box, nil
}

// writeJSON renders a response body.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// writeError renders an error body in the same shape as everything else.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message})
}

// writeErrorDetail renders an error that needs an explanation as well as a
// name. A refusal a caller cannot act on is a refusal they will file as a bug.
func writeErrorDetail(w http.ResponseWriter, status int, message, detail string) {
	writeJSON(w, status, map[string]any{"error": message, "detail": detail})
}

// min returns the smaller of two ints.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
