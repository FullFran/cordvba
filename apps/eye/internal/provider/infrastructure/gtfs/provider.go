package gtfs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/FullFran/cordvba/apps/eye/internal/httpx"
	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	provider "github.com/FullFran/cordvba/apps/eye/internal/provider/domain"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// departureHorizon is how far ahead departures are emitted. A day of
// timetable is what you plan around; a year of it is a database dump.
const departureHorizon = 24 * time.Hour

// Provider reads a GTFS archive and emits the stations it watches as entities
// and their upcoming departures as records.
//
// A full national feed is tens of thousands of trips, so the registry names the
// stops that matter. eye is a model of Córdoba, not a timetable mirror.
type Provider struct {
	src    source.Source
	client *httpx.Client

	validators httpx.Validators
}

// New builds a GTFS provider for a registry entry.
func New(src source.Source, client *httpx.Client) *Provider {
	return &Provider{src: src, client: client}
}

// Info implements provider.Provider.
func (p *Provider) Info() source.Source { return p.src }

// Poll returns the next departures from the watched stops.
func (p *Provider) Poll(ctx context.Context) ([]observation.Record, error) {
	feed, prov, err := p.fetch(ctx)
	if err != nil || feed == nil {
		return nil, err
	}

	watched := p.watchedStops(feed)
	if len(watched) == 0 {
		return nil, fmt.Errorf("%w: none of the configured stops are in this feed", ErrGTFS)
	}

	now := time.Now()
	departures := feed.Departures(watched, now, departureHorizon)

	records := make([]observation.Record, 0, len(departures))
	for _, d := range departures {
		rec, ok := p.departureRecord(d, prov)
		if !ok {
			continue
		}
		records = append(records, rec)
	}
	return records, nil
}

// Entities returns the watched stops as inventory.
func (p *Provider) Entities(ctx context.Context) ([]observation.Entity, error) {
	feed, prov, err := p.fetch(ctx)
	if err != nil || feed == nil {
		return nil, err
	}

	watched := p.watchedStops(feed)
	entities := make([]observation.Entity, 0, len(watched))

	for id := range watched {
		stop := feed.Stops[id]
		pos := observation.Point{Lat: stop.Lat, Lon: stop.Lon}
		if !pos.Valid() || (pos.Lat == 0 && pos.Lon == 0) {
			continue
		}

		payload, _ := json.Marshal(map[string]any{"stop_id": stop.ID})
		ent := observation.Entity{
			ID: p.src.ID + ":" + stop.ID, Source: p.src.ID,
			Kind: p.src.Option("kind", "station"), Topic: p.src.Topic,
			Title: stop.Name, Position: &pos,
			FirstSeen: prov.FetchedAt, LastSeen: prov.FetchedAt,
			Payload: payload, Provenance: prov,
		}
		if err := ent.Validate(); err != nil {
			continue
		}
		entities = append(entities, ent)
	}

	sort.Slice(entities, func(i, j int) bool { return entities[i].Title < entities[j].Title })
	return entities, nil
}

// fetch downloads and parses the archive, returning nil when unchanged.
func (p *Provider) fetch(ctx context.Context) (*Feed, observation.Provenance, error) {
	resp, err := p.client.Get(ctx, p.src.URL, p.validators)
	if err != nil {
		if isNotModified(err) {
			p.validators = resp.Validators
			return nil, observation.Provenance{}, nil
		}
		return nil, observation.Provenance{}, fmt.Errorf("%w: %w", ErrGTFS, err)
	}
	p.validators = resp.Validators

	feed, err := Parse(resp.Body)
	if err != nil {
		return nil, observation.Provenance{}, err
	}

	sum := sha256.Sum256(resp.Body)
	return feed, observation.Provenance{
		Publisher: p.src.Authority,
		SourceURL: p.src.URL,
		License:   p.src.License,
		FetchedAt: resp.FetchedAt,
		RawHash:   hex.EncodeToString(sum[:]),
	}, nil
}

// watchedStops resolves the configured stop ids, dropping any the feed does
// not contain so a renamed station fails loudly rather than silently.
func (p *Provider) watchedStops(feed *Feed) map[string]bool {
	out := map[string]bool{}
	for _, id := range strings.Split(p.src.Option("stops", ""), ",") {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := feed.Stops[id]; ok {
			out[id] = true
		}
	}
	return out
}

// departureRecord maps one scheduled departure.
func (p *Provider) departureRecord(d Departure, prov observation.Provenance) (observation.Record, bool) {
	when := d.When
	pos := observation.Point{Lat: d.Stop.Lat, Lon: d.Stop.Lon}

	// RENFE leaves trip_headsign empty, so the destination falls back to the
	// route and then to the trip id. A departure with no label at all is
	// worse than one labelled by its line.
	destination := firstNonEmpty(d.Headsign, d.RouteName, d.TripID)
	title := fmt.Sprintf("%s · %s → %s", d.Departure.Clock(), d.Stop.Name, destination)
	payload, _ := json.Marshal(map[string]any{
		"stop_id": d.Stop.ID, "stop_name": d.Stop.Name,
		"trip_id": d.TripID, "route": d.RouteName,
		"headsign": d.Headsign, "scheduled": d.Departure.String(),
	})

	rec := observation.Record{
		ID:     fmt.Sprintf("%s:%s:%s:%s", p.src.ID, d.Stop.ID, d.TripID, when.Format("20060102T1504")),
		Source: p.src.ID, Kind: "scheduled_departure", Topic: p.src.Topic,
		// We learned this now; the departure itself is in the future.
		ObservedAt: prov.FetchedAt, FetchedAt: prov.FetchedAt,
		ValidFrom: &when,
		Title:     title,
		Severity:  observation.SeverityNone, Confidence: 1,
		Quality: observation.QualityOfficial,
		// A departure is a stop and a trip. Its time is what moves.
		LocalKey:  fmt.Sprintf("%s:%s", d.Stop.ID, d.TripID),
		DedupeKey: fmt.Sprintf("%s:%s:%s", d.Stop.ID, d.TripID, when.Format("20060102T1504")),
		Payload:   payload, Provenance: prov,
	}
	if pos.Valid() && !(pos.Lat == 0 && pos.Lon == 0) {
		rec.Position = &pos
	}

	if err := rec.Validate(); err != nil {
		return observation.Record{}, false
	}
	return rec, true
}

// firstNonEmpty returns the first value that is not blank.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// isNotModified reports a 304, which is a healthy empty poll.
func isNotModified(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not modified")
}

var (
	_ provider.Provider       = (*Provider)(nil)
	_ provider.EntityProvider = (*Provider)(nil)
)
