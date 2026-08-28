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

	observation "github.com/FullFran/eye/internal/observation/domain"
	source "github.com/FullFran/eye/internal/source/domain"
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
}

// Server serves the read API.
type Server struct {
	store   Store
	sources []source.Source
	log     *slog.Logger
	started time.Time
}

// New builds a server over a store and the registry.
func New(store Store, sources []source.Source, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{store: store, sources: sources, log: log, started: time.Now().UTC()}
}

// Handler returns the routed handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /v1/records", s.handleRecords)
	mux.HandleFunc("GET /v1/entities", s.handleEntities)
	mux.HandleFunc("GET /v1/sources", s.handleSources)
	mux.HandleFunc("GET /", s.handleIndex)

	return s.withCommonHeaders(mux)
}

// withCommonHeaders applies the headers every response needs.
func (s *Server) withCommonHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read-only API, so a permissive CORS policy costs nothing and lets
		// a browser-based map consume it directly.
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Cache-Control", "no-store")
		// Every response says where the data came from, in the same place a
		// machine client will look for it.
		w.Header().Set("X-Eye-Provenance", "public sources; see the license field on every record")
		next.ServeHTTP(w, r)
	})
}

// handleIndex describes the API, so a person who curls the root learns what to
// ask for instead of getting a 404.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		writeError(w, http.StatusNotFound, "no such endpoint")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"service": "eye",
		"about":   "A live, source-verifiable model of Córdoba, from public sources only.",
		"endpoints": map[string]string{
			"GET /health":      "liveness and store counts",
			"GET /v1/records":  "observations; topic, source, kind, since, until, bbox, near, radius_km, text, min_severity, limit",
			"GET /v1/entities": "inventory; kind, source, topic, near, radius_km, text, limit",
			"GET /v1/sources":  "the registry: what eye may read and what it can read",
		},
		"note": "Every record carries its publisher, licence and both timestamps. " +
			"Camera images are never served through this API.",
	})
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

// handleSources serves the registry, including what is held and why.
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
		URL        string `json:"url"`
		Notes      string `json:"notes,omitempty"`
	}

	out := make([]view, 0, len(s.sources))
	for _, src := range s.sources {
		out = append(out, view{
			ID: src.ID, Authority: src.Authority, Topic: src.Topic, Format: src.Format,
			License: src.License, Access: string(src.Access), Automation: string(src.Automation),
			Pollable: src.Automation.Pollable(), URL: src.URL, Notes: src.Notes,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"count": len(out), "sources": out})
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

// min returns the smaller of two ints.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
