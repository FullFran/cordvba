package aucorsa

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FullFran/eye/internal/httpx"
	observation "github.com/FullFran/eye/internal/observation/domain"
	provider "github.com/FullFran/eye/internal/provider/domain"
	source "github.com/FullFran/eye/internal/source/domain"
)

// tokenPattern matches the value AUCORSA prints into every public page.
//
// It is a WordPress nonce. For an anonymous visitor that is a CSRF token and
// not a credential: it carries no account, identifies nobody, and every visitor
// is handed the same string — verified by comparing what a browser received
// with what a plain HTTP GET of the same page returns. It rotates on
// WordPress's twelve-hour tick.
//
// eye reads it from the public page it is printed on, which is what a browser
// does, and re-reads it when it stops working. Nothing is guessed, forged or
// bypassed: an endpoint that refuses without it simply refuses, and eye reports
// that rather than trying anything else.
var tokenPattern = regexp.MustCompile(`"(?:ajax_nonce|wpRestNonce)"\s*:\s*"([a-f0-9]{6,16})"`)

// tokenRefresh is how long a fetched token is reused before being re-read.
// WordPress ticks every twelve hours; refreshing at six leaves ample margin and
// keeps eye from fetching the page more often than it needs to.
const tokenRefresh = 6 * time.Hour

// arrivalTTL is how long an estimate is kept. A prediction about the next few
// minutes is worthless an hour later, and keeping it would only pad the store.
const arrivalTTL = time.Hour

// RealtimeProvider reads arrival estimates for a set of stops.
//
// The endpoint is undocumented, so this source is registered as access
// `undocumented_personal` under ADR-0008. That has two consequences enforced in
// code rather than remembered: the registry alone cannot enable it — the
// operator's machine must set EYE_ALLOW_PERSONAL_SOURCES — and its records
// never leave that machine through `eye serve`.
type RealtimeProvider struct {
	src    source.Source
	client *httpx.Client

	mu         sync.Mutex
	token      string
	tokenTaken time.Time
}

// NewRealtime builds an arrivals reader for a registry entry.
func NewRealtime(src source.Source, client *httpx.Client) *RealtimeProvider {
	return &RealtimeProvider{src: src, client: client}
}

// Info implements provider.Provider.
func (p *RealtimeProvider) Info() source.Source { return p.src }

// The endpoint answers with a JSON string containing an HTML fragment — a
// rendered popup rather than data. These patterns take it apart.
//
// Parsing markup is not how anyone would choose to read a timetable, but this
// is the shape the operator serves, and pretending otherwise would mean
// inventing an API they do not have.
var (
	// stopLabelPattern captures "Parada 401: Ctra. Trassierra (Anna Pavlova)".
	stopLabelPattern = regexp.MustCompile(`ppp-stop-label"[^>]*>Parada\s*(\d+)\s*:\s*([^<]+)<`)
	// linePattern captures the line code and its route within a block.
	lineNumberPattern = regexp.MustCompile(`ppp-line-number"[^>]*>([^<]{1,6})<`)
	lineRoutePattern  = regexp.MustCompile(`ppp-line-route"[^>]*>([^<]+)<`)
	// minutesPattern captures each "<strong>2 minutos</strong>".
	minutesPattern = regexp.MustCompile(`<strong>\s*(\d+)\s*minuto`)
	// occupancyPattern captures the alt text of the occupancy icon.
	occupancyPattern = regexp.MustCompile(`imgocupationbus[^>]*alt="([^"]+)"`)
)

// estimate is one predicted arrival for one line at one stop.
type estimate struct {
	Line      string
	Route     string
	Minutes   int
	Occupancy string
	// Position is where this estimate sits in the line's queue: the first is
	// the next bus, the second the one after it.
	Position int
}

// parseEstimates takes apart the rendered fragment.
func parseEstimates(fragment string) (stopID, stopName string, out []estimate) {
	if m := stopLabelPattern.FindStringSubmatch(fragment); m != nil {
		stopID, stopName = m[1], clean(m[2])
	}

	for _, body := range lineBlocks(fragment) {
		var line, route string
		if m := lineNumberPattern.FindStringSubmatch(body); m != nil {
			line = clean(m[1])
		}
		if m := lineRoutePattern.FindStringSubmatch(body); m != nil {
			route = clean(m[1])
		}
		if line == "" {
			continue
		}

		occupancy := ""
		if m := occupancyPattern.FindStringSubmatch(body); m != nil {
			occupancy = clean(m[1])
		}

		for i, m := range minutesPattern.FindAllStringSubmatch(body, -1) {
			minutes, err := strconv.Atoi(m[1])
			if err != nil {
				continue
			}
			out = append(out, estimate{
				Line: line, Route: route, Minutes: minutes,
				// Occupancy is reported for the next bus only.
				Occupancy: occupancyFor(occupancy, i),
				Position:  i,
			})
		}
	}
	return stopID, stopName, out
}

// containerMarker opens each per-line block in the rendered fragment.
const containerMarker = `<div class="ppp-container">`

// favoritesMarker opens the block that follows the last line, which belongs to
// the page's own furniture rather than to any arrival.
const favoritesMarker = `<div class="ppp-favorited`

// lineBlocks cuts the fragment into one body per line calling at the stop.
//
// Splitting rather than matching between delimiters is deliberate: a regexp
// that ends on the next opening tag consumes it, so every block after the first
// disappears — and a stop served by several lines is the normal case, not the
// exception.
func lineBlocks(fragment string) []string {
	parts := strings.Split(fragment, containerMarker)
	if len(parts) < 2 {
		return nil
	}

	out := make([]string, 0, len(parts)-1)
	for _, body := range parts[1:] {
		// The tail after the last line block is the favourites widget.
		if i := strings.Index(body, favoritesMarker); i >= 0 {
			body = body[:i]
		}
		out = append(out, body)
	}
	return out
}

// occupancyFor returns the occupancy reading, which the operator gives for the
// imminent bus and not for the ones behind it.
func occupancyFor(occupancy string, position int) string {
	if position == 0 {
		return occupancy
	}
	return ""
}

// Poll fetches arrival estimates for every stop the registry configures.
func (p *RealtimeProvider) Poll(ctx context.Context) ([]observation.Record, error) {
	stops := p.WatchedStops()
	if len(stops) == 0 {
		return nil, fmt.Errorf("%w: source %s watches no stops", ErrAucorsa, p.src.ID)
	}
	return p.Arrivals(ctx, stops)
}

// WatchedStops is the stop list the registry configures for this source.
func (p *RealtimeProvider) WatchedStops() []string {
	return splitList(p.src.Option("stops", ""))
}

// Arrivals fetches estimates for an explicit set of stops, which is what lets a
// command ask about a stop the registry does not watch.
func (p *RealtimeProvider) Arrivals(ctx context.Context, stops []string) ([]observation.Record, error) {
	if len(stops) == 0 {
		return nil, nil
	}

	token, err := p.currentToken(ctx)
	if err != nil {
		return nil, err
	}

	now := nowUTC()
	var (
		records []observation.Record
		refused int
	)

	for _, stop := range stops {
		body, err := p.fetchEstimates(ctx, stop, token)
		if err != nil {
			// One stop that will not answer is not a reason to lose the
			// rest, but if every stop refuses that is worth reporting.
			refused++
			continue
		}
		records = append(records, p.toRecords(stop, body, now)...)
	}

	if refused == len(stops) {
		return nil, fmt.Errorf("%w: all %d stops refused; the endpoint or its token has changed",
			ErrAucorsa, len(stops))
	}
	return records, nil
}

// fetchEstimates calls the arrivals endpoint for one stop.
func (p *RealtimeProvider) fetchEstimates(ctx context.Context, stop, token string) ([]byte, error) {
	url := fmt.Sprintf("%s/estimations/stop?stop_id=%s&_wpnonce=%s",
		strings.TrimSuffix(p.src.URL, "/"), stop, token)

	resp, err := p.client.Get(ctx, url, httpx.Validators{})
	if err != nil {
		return nil, fmt.Errorf("%w: stop %s: %w", ErrAucorsa, stop, err)
	}

	// The endpoint answers 200 with the body "-1" when it declines. A status
	// code that always says OK is exactly why this has to be checked, and
	// why a refusal must be surfaced rather than parsed as empty data.
	if strings.TrimSpace(string(resp.Body)) == "-1" {
		p.invalidateToken()
		return nil, fmt.Errorf("%w: stop %s declined", ErrAucorsa, stop)
	}
	return resp.Body, nil
}

// toRecords normalizes the estimates for one stop.
func (p *RealtimeProvider) toRecords(stop string, body []byte, now time.Time) []observation.Record {
	// The body is a JSON string wrapping the HTML, so it is unwrapped once
	// before the markup is read.
	var fragment string
	if err := json.Unmarshal(body, &fragment); err != nil {
		fragment = string(body)
	}

	_, stopName, estimates := parseEstimates(fragment)
	if stopName == "" {
		stopName = "parada " + stop
	}

	sum := sha256.Sum256(body)
	prov := observation.Provenance{
		Publisher: p.src.Authority,
		SourceURL: strings.TrimSuffix(p.src.URL, "/") + "/estimations/stop",
		License:   p.src.License,
		FetchedAt: now,
		RawHash:   hex.EncodeToString(sum[:]),
	}

	expires := now.Add(arrivalTTL)
	out := make([]observation.Record, 0, len(estimates))

	for _, e := range estimates {
		payload, _ := json.Marshal(map[string]any{
			"stop_id":   stop,
			"stop_name": stopName,
			"line":      e.Line,
			"route":     e.Route,
			"minutes":   e.Minutes,
			"occupancy": e.Occupancy,
			"position":  e.Position,
		})

		title := fmt.Sprintf("Línea %s → %s · %d min · %s", e.Line, e.Route, e.Minutes, stopName)
		if e.Occupancy != "" {
			title += " · " + e.Occupancy
		}

		// The departure is in the future; the observation is now. Keeping
		// them apart is what lets a consumer see how stale an estimate is.
		due := now.Add(time.Duration(e.Minutes) * time.Minute)

		rec := observation.Record{
			ID:     fmt.Sprintf("%s:%s:%s:%d:%s", p.src.ID, stop, e.Line, e.Position, now.Format("20060102T150405")),
			Source: p.src.ID, Kind: "bus_arrival", Topic: p.src.Topic,
			ObservedAt: now, FetchedAt: now,
			ValidFrom:  &due,
			Title:      title,
			Severity:   observation.SeverityNone,
			Confidence: 1,
			// An arrival estimate is the operator's own prediction, not a
			// measurement, and it is recorded as one.
			Quality: observation.QualityPreliminary,
			// The minutes are the prediction, not the bus. Keeping them
			// out is what turns "21 min" becoming "18 min" into a
			// change instead of a different bus.
			LocalKey:   fmt.Sprintf("%s:%s:%d", stop, e.Line, e.Position),
			DedupeKey:  fmt.Sprintf("%s:%s:%s:%d:%d", p.src.ID, stop, e.Line, e.Position, e.Minutes),
			ExpiresAt:  &expires,
			Payload:    payload,
			Provenance: prov,
		}
		if err := rec.Validate(); err != nil {
			continue
		}
		out = append(out, rec)
	}
	return out
}

// currentToken returns a usable token, reading the public page when the cached
// one is stale or absent.
func (p *RealtimeProvider) currentToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	cached, taken := p.token, p.tokenTaken
	p.mu.Unlock()

	if cached != "" && time.Since(taken) < tokenRefresh {
		return cached, nil
	}

	page := p.src.Option("token_page", "https://aucorsa.es/")
	resp, err := p.client.Get(ctx, page, httpx.Validators{})
	if err != nil {
		return "", fmt.Errorf("%w: read %s: %w", ErrAucorsa, page, err)
	}

	m := tokenPattern.FindStringSubmatch(resp.DecodeUTF8())
	if m == nil {
		return "", fmt.Errorf("%w: %s no longer carries the value this reader expects; the site has changed",
			ErrAucorsa, page)
	}

	p.mu.Lock()
	p.token, p.tokenTaken = m[1], time.Now()
	p.mu.Unlock()

	return m[1], nil
}

// invalidateToken forces the next poll to re-read the page.
func (p *RealtimeProvider) invalidateToken() {
	p.mu.Lock()
	p.token, p.tokenTaken = "", time.Time{}
	p.mu.Unlock()
}

// splitList parses a comma-separated option.
func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

var _ provider.Provider = (*RealtimeProvider)(nil)
