// Package aucorsa reads Córdoba's city bus operator.
//
// AUCORSA publishes no GTFS and no open-data feed. What it does publish is an
// ordinary server-rendered page per line, carrying every stop with its id and
// name and the service hours for each day type. That is what this reader uses:
// public HTML, fetched the way a browser fetches it, with nothing to work
// around.
//
// The real-time arrivals live behind the same site's REST namespace and are
// handled separately in realtime.go, under a different access kind.
package aucorsa

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"

	"github.com/FullFran/cordvba/apps/eye/internal/httpx"
	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	provider "github.com/FullFran/cordvba/apps/eye/internal/provider/domain"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// ErrAucorsa is returned when a page cannot be read.
var ErrAucorsa = errors.New("aucorsa")

// Patterns over the rendered page. Regular expressions rather than a DOM
// parser because that would be a dependency, and because these two shapes are
// simple and stable: an anchor to a stop, and a line link on the index.
var (
	// stopPattern matches an element carrying a stop id followed by its name.
	stopPattern = regexp.MustCompile(`stop-id-(\d+)[^>]*>\s*([^<]{2,80})`)
	// linePattern matches a line link on the index page.
	linePattern = regexp.MustCompile(`aucorsa\.es/linea/([A-Za-z0-9]{1,4})/`)
	// hoursPattern matches a service window such as "6:30 a 23:00".
	hoursPattern = regexp.MustCompile(`(\d{1,2}:\d{2})\s*a\s*(\d{1,2}:\d{2})`)
	// tagPattern strips markup from an extracted fragment.
	tagPattern = regexp.MustCompile(`<[^>]*>`)
	// spacePattern collapses the whitespace that stripping leaves.
	spacePattern = regexp.MustCompile(`\s+`)
)

// Provider reads AUCORSA's public line pages.
type Provider struct {
	src    source.Source
	client *httpx.Client
}

// New builds a line reader for a registry entry.
func New(src source.Source, client *httpx.Client) *Provider {
	return &Provider{src: src, client: client}
}

// Info implements provider.Provider.
func (p *Provider) Info() source.Source { return p.src }

// Poll emits one record per line carrying its service hours.
func (p *Provider) Poll(ctx context.Context) ([]observation.Record, error) {
	lines, prov, err := p.readLines(ctx)
	if err != nil {
		return nil, err
	}

	records := make([]observation.Record, 0, len(lines))
	for _, line := range lines {
		payload, _ := json.Marshal(map[string]any{
			"line":     line.Code,
			"stops":    len(line.Stops),
			"schedule": line.Hours,
		})

		rec := observation.Record{
			ID:     p.src.ID + ":line:" + line.Code,
			Source: p.src.ID, Kind: "bus_line", Topic: p.src.Topic,
			ObservedAt: prov.FetchedAt, FetchedAt: prov.FetchedAt,
			Title:       "Línea " + line.Code + " · " + line.ServiceSummary(),
			Description: strings.Join(line.Hours, " · "),
			Severity:    observation.SeverityNone, Confidence: 1,
			Quality: observation.QualityOfficial,
			// The service hours are in the title and in the old key, so
			// a timetable change looked like a different line.
			LocalKey:  "line:" + line.Code,
			DedupeKey: p.src.ID + ":line:" + line.Code + ":" + strings.Join(line.Hours, "|"),
			Payload:   payload, Provenance: prov,
		}
		if err := rec.Validate(); err != nil {
			continue
		}
		records = append(records, rec)
	}
	return records, nil
}

// Entities emits every stop across every line.
func (p *Provider) Entities(ctx context.Context) ([]observation.Entity, error) {
	lines, prov, err := p.readLines(ctx)
	if err != nil {
		return nil, err
	}

	// A stop is served by several lines, so it is emitted once with the
	// lines recorded against it rather than once per line.
	byID := map[string]*stopEntity{}
	for _, line := range lines {
		for id, name := range line.Stops {
			entry, ok := byID[id]
			if !ok {
				entry = &stopEntity{ID: id, Name: name}
				byID[id] = entry
			}
			entry.Lines = append(entry.Lines, line.Code)
		}
	}

	entities := make([]observation.Entity, 0, len(byID))
	for _, s := range byID {
		payload, _ := json.Marshal(map[string]any{"stop_id": s.ID, "lines": s.Lines})

		// AUCORSA's pages carry no coordinates. The municipal cartography
		// layer does, and matching the two is its own piece of work — so
		// these are recorded without a position rather than with a guess.
		ent := observation.Entity{
			ID: p.src.ID + ":" + s.ID, Source: p.src.ID,
			Kind: "bus_stop", Topic: p.src.Topic,
			Title:     s.Name,
			FirstSeen: prov.FetchedAt, LastSeen: prov.FetchedAt,
			Payload: payload, Provenance: prov,
		}
		if err := ent.Validate(); err != nil {
			continue
		}
		entities = append(entities, ent)
	}
	return entities, nil
}

// stopEntity accumulates a stop across the lines that serve it.
type stopEntity struct {
	ID    string
	Name  string
	Lines []string
}

// Line is one bus line as the page describes it.
type Line struct {
	Code  string
	Stops map[string]string
	Hours []string
}

// ServiceSummary renders the first and last service of the day.
func (l Line) ServiceSummary() string {
	if len(l.Hours) == 0 {
		return fmt.Sprintf("%d paradas", len(l.Stops))
	}
	return fmt.Sprintf("%d paradas · %s", len(l.Stops), l.Hours[0])
}

// readLines fetches the index and every line page.
func (p *Provider) readLines(ctx context.Context) ([]Line, observation.Provenance, error) {
	indexURL := strings.TrimSuffix(p.src.URL, "/") + "/lineas-y-horarios/"

	resp, err := p.client.Get(ctx, indexURL, httpx.Validators{})
	if err != nil {
		return nil, observation.Provenance{}, fmt.Errorf("%w: index: %w", ErrAucorsa, err)
	}

	sum := sha256.Sum256(resp.Body)
	prov := observation.Provenance{
		Publisher: p.src.Authority, SourceURL: indexURL, License: p.src.License,
		FetchedAt: resp.FetchedAt, RawHash: hex.EncodeToString(sum[:]),
	}

	codes := lineCodes(resp.DecodeUTF8())
	if len(codes) == 0 {
		return nil, prov, fmt.Errorf("%w: no lines on the index page", ErrAucorsa)
	}

	lines := make([]Line, 0, len(codes))
	for _, code := range codes {
		line, err := p.readLine(ctx, code)
		if err != nil {
			// One line page failing must not lose the other thirty-one.
			continue
		}
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		return nil, prov, fmt.Errorf("%w: %d lines listed but none could be read", ErrAucorsa, len(codes))
	}
	return lines, prov, nil
}

// readLine fetches and parses one line page.
func (p *Provider) readLine(ctx context.Context, code string) (Line, error) {
	url := strings.TrimSuffix(p.src.URL, "/") + "/linea/" + code + "/"

	resp, err := p.client.Get(ctx, url, httpx.Validators{})
	if err != nil {
		return Line{}, fmt.Errorf("%w: line %s: %w", ErrAucorsa, code, err)
	}

	body := resp.DecodeUTF8()
	line := Line{Code: code, Stops: parseStops(body), Hours: parseHours(body)}

	if len(line.Stops) == 0 {
		return Line{}, fmt.Errorf("%w: line %s has no stops", ErrAucorsa, code)
	}
	return line, nil
}

// lineCodes extracts the line identifiers from the index page.
func lineCodes(body string) []string {
	seen := map[string]bool{}
	var out []string

	for _, m := range linePattern.FindAllStringSubmatch(body, -1) {
		code := strings.ToLower(m[1])
		if !seen[code] {
			seen[code] = true
			out = append(out, code)
		}
	}
	return out
}

// parseStops extracts the stop ids and names from a line page.
func parseStops(body string) map[string]string {
	out := map[string]string{}

	for _, m := range stopPattern.FindAllStringSubmatch(body, -1) {
		name := clean(m[2])
		if name == "" {
			continue
		}
		out[m[1]] = name
	}
	return out
}

// parseHours extracts the service windows, such as "6:30 a 23:00".
func parseHours(body string) []string {
	seen := map[string]bool{}
	var out []string

	for _, m := range hoursPattern.FindAllStringSubmatch(body, -1) {
		window := m[1] + "–" + m[2]
		if !seen[window] {
			seen[window] = true
			out = append(out, window)
		}
	}
	return out
}

// clean turns an HTML fragment into a plain label.
func clean(s string) string {
	s = tagPattern.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return strings.TrimSpace(spacePattern.ReplaceAllString(s, " "))
}

// nowUTC exists so the reader has one clock source.
func nowUTC() time.Time { return time.Now().UTC() }

var (
	_ provider.Provider       = (*Provider)(nil)
	_ provider.EntityProvider = (*Provider)(nil)
	_                         = nowUTC
)
