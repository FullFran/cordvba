// Package metar reads surface weather observations from the NOAA Aviation
// Weather Center.
//
// It exists because Córdoba had no live weather reading at all. AEMET publishes
// station 5402 — Córdoba Aeropuerto — through an API that needs a personal key,
// and until an operator gets one eye knows nothing about the weather outside.
//
// This is the SAME PHYSICAL STATION, reported through a different channel.
// LEBA is the ICAO identifier of Córdoba Airport, and the feed answers with the
// aerodrome's own coordinates and elevation. What is different is the publisher,
// the purpose and the variable set: METAR is written for pilots, it reports
// fewer quantities than AEMET does, and NOAA is not the competent meteorological
// authority for Spain. Every one of those differences is recorded on the record
// rather than smoothed away — see the quality and the payload.
package metar

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/FullFran/cordvba/apps/eye/internal/httpx"
	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	provider "github.com/FullFran/cordvba/apps/eye/internal/provider/domain"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// ErrMETAR wraps every failure this adapter reports.
var ErrMETAR = errors.New("metar")

// defaultStation is Córdoba Airport.
const defaultStation = "LEBA"

// defaultTTL is how long a surface observation is kept. A reading is a sample of
// a moving signal, not a statement that stays true.
const defaultTTL = 30 * 24 * time.Hour

// Provider reads one or more METAR stations.
type Provider struct {
	src    source.Source
	client *httpx.Client
}

// New builds a provider for a registry entry.
func New(src source.Source, c *httpx.Client) *Provider {
	return &Provider{src: src, client: c}
}

// Info implements provider.Provider.
func (p *Provider) Info() source.Source { return p.src }

// report is one observation as the Aviation Weather Center serialises it.
//
// Several fields are typed `any` on purpose: the API returns a number when it
// has one and a string when the value is qualified, so `visib` arrives as 10 on
// a clear day and as "6+" when the observer means "at least six". Forcing that
// into a float would silently drop the qualification.
type report struct {
	ICAO        string   `json:"icaoId"`
	Name        string   `json:"name"`
	ObsTime     int64    `json:"obsTime"`
	ReportTime  string   `json:"reportTime"`
	ReceiptTime string   `json:"receiptTime"`
	Temp        *float64 `json:"temp"`
	Dewpoint    *float64 `json:"dewp"`
	WindDir     any      `json:"wdir"`
	WindSpeed   *float64 `json:"wspd"`
	WindGust    *float64 `json:"wgst"`
	Visibility  any      `json:"visib"`
	Altimeter   *float64 `json:"altim"`
	Cover       string   `json:"cover"`
	Raw         string   `json:"rawOb"`
	Type        string   `json:"metarType"`
	Lat         float64  `json:"lat"`
	Lon         float64  `json:"lon"`
	Elevation   *float64 `json:"elev"`
}

// Poll fetches the configured stations and normalizes each report.
func (p *Provider) Poll(ctx context.Context) ([]observation.Record, error) {
	endpoint := p.endpoint()
	resp, err := p.client.Get(ctx, endpoint, httpx.Validators{})
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrMETAR, p.src.ID, err)
	}

	var reports []report
	if err := json.Unmarshal(resp.Body, &reports); err != nil {
		return nil, fmt.Errorf("%w: %s: parse: %w", ErrMETAR, p.src.ID, err)
	}

	sum := sha256.Sum256(resp.Body)
	prov := observation.Provenance{
		Publisher: p.src.Authority,
		SourceURL: endpoint,
		License:   p.src.License,
		FetchedAt: resp.FetchedAt,
		RawHash:   hex.EncodeToString(sum[:]),
	}

	records := make([]observation.Record, 0, len(reports))
	for _, r := range reports {
		if rec, ok := p.toRecord(r, resp.FetchedAt, prov); ok {
			records = append(records, rec)
		}
	}
	return records, nil
}

// Entities publishes the stations themselves, which are persistent things.
func (p *Provider) Entities(ctx context.Context) ([]observation.Entity, error) {
	endpoint := p.endpoint()
	resp, err := p.client.Get(ctx, endpoint, httpx.Validators{})
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrMETAR, p.src.ID, err)
	}

	var reports []report
	if err := json.Unmarshal(resp.Body, &reports); err != nil {
		return nil, fmt.Errorf("%w: %s: parse: %w", ErrMETAR, p.src.ID, err)
	}

	sum := sha256.Sum256(resp.Body)
	prov := observation.Provenance{
		Publisher: p.src.Authority,
		SourceURL: endpoint,
		License:   p.src.License,
		FetchedAt: resp.FetchedAt,
		RawHash:   hex.EncodeToString(sum[:]),
	}

	out := make([]observation.Entity, 0, len(reports))
	for _, r := range reports {
		pos := observation.Point{Lat: r.Lat, Lon: r.Lon}
		if !pos.Valid() || (pos.Lat == 0 && pos.Lon == 0) {
			continue
		}
		payload, _ := json.Marshal(map[string]any{"icao": r.ICAO, "elevation_m": r.Elevation})

		ent := observation.Entity{
			ID:         p.src.ID + ":" + strings.TrimSpace(r.ICAO),
			Source:     p.src.ID,
			Kind:       p.src.Option("kind", "weather_station"),
			Topic:      p.src.Topic,
			Title:      strings.TrimSpace(firstNonBlank(r.Name, r.ICAO)),
			Position:   &pos,
			FirstSeen:  resp.FetchedAt,
			LastSeen:   resp.FetchedAt,
			Payload:    payload,
			Provenance: prov,
		}
		if err := ent.Validate(); err != nil {
			continue
		}
		out = append(out, ent)
	}
	return out, nil
}

// toRecord normalizes one report.
func (p *Provider) toRecord(r report, fetchedAt time.Time,
	prov observation.Provenance,
) (observation.Record, bool) {
	icao := strings.TrimSpace(r.ICAO)
	if icao == "" || r.ObsTime <= 0 {
		return observation.Record{}, false
	}

	// ObservedAt is the observation time the station stamped, never the
	// moment eye fetched it. Their difference is this reading's age, and a
	// METAR that is two hours old must be visible as two hours old.
	observedAt := time.Unix(r.ObsTime, 0).UTC()
	expires := observedAt.Add(p.ttl())

	pos := observation.Point{Lat: r.Lat, Lon: r.Lon}
	position := &pos
	if !pos.Valid() || (pos.Lat == 0 && pos.Lon == 0) {
		position = nil
	}

	payload, _ := json.Marshal(map[string]any{
		"icao":               icao,
		"station":            strings.TrimSpace(r.Name),
		"temperature_c":      r.Temp,
		"dewpoint_c":         r.Dewpoint,
		"wind_direction_deg": r.WindDir,
		"wind_speed_kt":      r.WindSpeed,
		"wind_gust_kt":       r.WindGust,
		"visibility":         r.Visibility,
		"altimeter_hpa":      r.Altimeter,
		"cloud_cover":        strings.TrimSpace(r.Cover),
		"report_type":        strings.TrimSpace(r.Type),
		"raw":                strings.TrimSpace(r.Raw),
		// The reading is an aerodrome observation relayed by NOAA. It is not
		// AEMET speaking, even though the instrument sits at the same airport,
		// and a reader comparing it against an official figure has to know.
		"note": "aerodrome METAR relayed by the NOAA Aviation Weather Center; " +
			"not a statement by the competent national meteorological authority",
	})

	rec := observation.Record{
		ID:          p.src.ID + ":" + icao + ":" + observedAt.Format(time.RFC3339),
		Source:      p.src.ID,
		Kind:        p.src.Option("record_kind", "weather_observation"),
		Topic:       p.src.Topic,
		ObservedAt:  observedAt,
		FetchedAt:   fetchedAt,
		Position:    position,
		Title:       observationTitle(r),
		Description: strings.TrimSpace(r.Raw),
		Severity:    observation.SeverityNone,
		Confidence:  1,
		// An automatic instrument reading nobody has reviewed. It is good
		// data and it is not a validated declaration, and flattening that
		// difference is how a number starts carrying more authority than it
		// earned.
		Quality: observation.QualityPreliminary,
		// The station is the persistent thing; the moment makes the reading.
		LocalKey:   icao,
		DedupeKey:  p.src.ID + ":" + icao + ":" + observedAt.Format(time.RFC3339),
		ExpiresAt:  &expires,
		Payload:    payload,
		Provenance: prov,
	}
	if err := rec.Validate(); err != nil {
		return observation.Record{}, false
	}
	return rec, true
}

// observationTitle is a one-line summary a person can read without decoding a
// METAR string.
func observationTitle(r report) string {
	name := strings.TrimSpace(firstNonBlank(r.Name, r.ICAO))
	parts := make([]string, 0, 3)
	if r.Temp != nil {
		parts = append(parts, strconv.FormatFloat(*r.Temp, 'f', -1, 64)+" °C")
	}
	if r.WindSpeed != nil {
		parts = append(parts, strconv.FormatFloat(*r.WindSpeed, 'f', -1, 64)+" kt")
	}
	if cover := strings.TrimSpace(r.Cover); cover != "" {
		parts = append(parts, cover)
	}
	if len(parts) == 0 {
		return name
	}
	return name + " · " + strings.Join(parts, " · ")
}

// endpoint builds the request for the configured stations.
func (p *Provider) endpoint() string {
	base := strings.TrimSpace(p.src.URL)
	if base == "" {
		base = "https://aviationweather.gov/api/data/metar"
	}
	stations := strings.TrimSpace(p.src.Option("stations", defaultStation))
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return base + sep + "ids=" + stations + "&format=json"
}

// ttl resolves the retention window for a reading.
func (p *Provider) ttl() time.Duration {
	if d, err := time.ParseDuration(p.src.Option("ttl", "")); err == nil && d > 0 {
		return d
	}
	return defaultTTL
}

// firstNonBlank returns the first value that is not whitespace.
func firstNonBlank(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// Interface checks.
var (
	_ provider.Provider       = (*Provider)(nil)
	_ provider.EntityProvider = (*Provider)(nil)
)
