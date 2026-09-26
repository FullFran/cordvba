package aemet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/FullFran/eye/internal/httpx"
	observation "github.com/FullFran/eye/internal/observation/domain"
	provider "github.com/FullFran/eye/internal/provider/domain"
	source "github.com/FullFran/eye/internal/source/domain"
)

// observationPath is the documented endpoint for the last twelve hours of one
// automatic station.
const observationPath = "/api/observacion/convencional/datos/estacion/"

// defaultStation is Cordoba Aeropuerto. eye is a Cordoba gateway, so a registry
// entry that names no station gets the city's own.
const defaultStation = "5402"

// defaultObservationTTL is how long a station reading is kept. These are
// unreviewed automatic samples of a continuous signal, not statements: eye is a
// live gateway, not a climate archive.
const defaultObservationTTL = 30 * 24 * time.Hour

// fintLayout is the timestamp AEMET writes into every reading. It carries no
// zone marker and is UTC, per the product's own field dictionary.
const fintLayout = "2006-01-02T15:04:05"

// ObservationProvider reads the hourly output of AEMET automatic stations.
//
// The readings carry the station's own identity and position, so the inventory
// comes out of the same response. Asking a second endpoint for it would spend
// AEMET's request budget to learn what eye already has.
type ObservationProvider struct {
	client client
}

// NewObservation builds the station-observation adapter for a registry entry.
func NewObservation(src source.Source, c *httpx.Client, apiKey string) *ObservationProvider {
	return &ObservationProvider{client: client{src: src, http: c, apiKey: apiKey}}
}

// Info implements provider.Provider.
func (p *ObservationProvider) Info() source.Source { return p.client.src }

// reading is the subset of an AEMET observation eye normalizes. Every other
// field stays in Payload exactly as received.
type reading struct {
	Idema string   `json:"idema"`
	Ubi   string   `json:"ubi"`
	Fint  string   `json:"fint"`
	Lat   *float64 `json:"lat"`
	Lon   *float64 `json:"lon"`
	Alt   *float64 `json:"alt"`
	Ta    *float64 `json:"ta"`
	Hr    *float64 `json:"hr"`
	Prec  *float64 `json:"prec"`
	Vv    *float64 `json:"vv"`
}

// Poll returns one record per hourly reading the station published.
func (p *ObservationProvider) Poll(ctx context.Context) ([]observation.Record, error) {
	rows, pay, err := p.readings(ctx)
	if err != nil {
		return nil, err
	}

	ttl := p.client.ttl(defaultObservationTTL)
	records := make([]observation.Record, 0, len(rows))

	for _, row := range rows {
		var r reading
		if err := json.Unmarshal(row, &r); err != nil {
			continue
		}
		observedAt, ok := parseFint(r.Fint)
		if r.Idema == "" || !ok {
			// A reading with no station or no timestamp is not an
			// observation of anything eye can place in time.
			continue
		}

		expires := observedAt.Add(ttl)
		rec := observation.Record{
			ID:         p.client.src.ID + ":" + r.Idema + ":" + r.Fint,
			Source:     p.client.src.ID,
			Kind:       "weather_observation",
			Topic:      p.client.src.Topic,
			ObservedAt: observedAt,
			FetchedAt:  pay.fetchedAt,
			Title:      readingTitle(r),
			Severity:   observation.SeverityInfo,
			Confidence: 1,
			// AEMET's automatic stations publish before anyone reviews
			// them. Calling that official would launder a raw sensor
			// reading into a validated statement.
			Quality: observation.QualityPreliminary,
			// The station is the thing. Its temperature is what
			// changes, so identity built from a reading would report
			// one station disappearing and another arriving hourly.
			LocalKey:  r.Idema,
			DedupeKey: p.client.src.ID + ":" + r.Idema + ":" + r.Fint,
			ExpiresAt: &expires,
			Payload:   row,
			Provenance: observation.Provenance{
				Publisher: p.client.src.Authority,
				SourceURL: pay.endpoint,
				License:   p.client.src.License,
				FetchedAt: pay.fetchedAt,
				RawHash:   pay.rawHash,
			},
		}
		if pos, ok := positionOf(r); ok {
			rec.Position = &pos
		}

		if err := rec.Validate(); err != nil {
			continue
		}
		records = append(records, rec)
	}
	return records, nil
}

// Entities returns the stations behind the readings.
func (p *ObservationProvider) Entities(ctx context.Context) ([]observation.Entity, error) {
	rows, pay, err := p.readings(ctx)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	entities := make([]observation.Entity, 0, 1)

	for _, row := range rows {
		var r reading
		if err := json.Unmarshal(row, &r); err != nil || r.Idema == "" || seen[r.Idema] {
			continue
		}
		seen[r.Idema] = true

		payload, _ := json.Marshal(map[string]any{"indicative": r.Idema, "altitude_m": r.Alt})
		e := observation.Entity{
			ID:        p.client.src.ID + ":station:" + r.Idema,
			Source:    p.client.src.ID,
			Kind:      "weather_station",
			Topic:     p.client.src.Topic,
			Title:     firstNonEmpty(r.Ubi, r.Idema),
			FirstSeen: pay.fetchedAt,
			LastSeen:  pay.fetchedAt,
			Payload:   payload,
			Provenance: observation.Provenance{
				Publisher: p.client.src.Authority,
				SourceURL: pay.endpoint,
				License:   p.client.src.License,
				FetchedAt: pay.fetchedAt,
				RawHash:   pay.rawHash,
			},
		}
		if pos, ok := positionOf(r); ok {
			e.Position = &pos
		}
		if err := e.Validate(); err != nil {
			continue
		}
		entities = append(entities, e)
	}
	return entities, nil
}

// readings performs one two-step fetch and returns the rows untouched.
func (p *ObservationProvider) readings(ctx context.Context) ([]json.RawMessage, payload, error) {
	pay, err := p.client.fetch(ctx, observationPath+p.station())
	switch {
	case errors.Is(err, ErrNoData):
		return nil, payload{}, nil
	case err != nil:
		return nil, payload{}, err
	}

	var rows []json.RawMessage
	if err := json.Unmarshal([]byte(pay.decodeUTF8()), &rows); err != nil {
		return nil, payload{}, fmt.Errorf("%w: %s: the payload is not a list of readings: %w",
			ErrAEMET, p.client.src.ID, err)
	}
	return rows, pay, nil
}

// station is the climatological indicative to fetch.
func (p *ObservationProvider) station() string {
	return p.client.src.Option("station", defaultStation)
}

// readingTitle names the station and its headline measurement.
func readingTitle(r reading) string {
	name := firstNonEmpty(r.Ubi, r.Idema)
	if r.Ta == nil {
		return name
	}
	return fmt.Sprintf("%s: %.1f ºC", name, *r.Ta)
}

// positionOf reads the station's coordinates, which every reading carries.
func positionOf(r reading) (observation.Point, bool) {
	if r.Lat == nil || r.Lon == nil {
		return observation.Point{}, false
	}
	pos := observation.Point{Lat: *r.Lat, Lon: *r.Lon}
	return pos, pos.Valid()
}

// parseFint reads AEMET's observation timestamp, which is UTC and says so
// nowhere in the value itself.
func parseFint(s string) (time.Time, bool) {
	if t, err := time.Parse(fintLayout, s); err == nil {
		return t.UTC(), true
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), true
	}
	return time.Time{}, false
}

var (
	_ provider.Provider       = (*ObservationProvider)(nil)
	_ provider.EntityProvider = (*ObservationProvider)(nil)
)
