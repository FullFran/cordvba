// Package dcat reads a tabular DCAT distribution as observations.
//
// MITECO publishes the national air-quality index (ICA) as a DCAT dataset whose
// distributions are hourly CSV tables, one row per measuring station. The
// catalogue names the distribution; this adapter reads it.
//
// The index is not a measurement. It is the worst category among the pollutants
// the station could account for, and the publisher states that when it was
// computed from fewer pollutants than usual it is reported as the same category
// multiplied by ten. That caveat is preserved rather than normalized away: a
// partial index is real information about how much the number can be trusted.
package dcat

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/FullFran/cordvba/apps/eye/internal/httpx"
	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	provider "github.com/FullFran/cordvba/apps/eye/internal/provider/domain"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// ErrDCAT is returned when a distribution answers something eye cannot use.
var ErrDCAT = errors.New("dcat")

// defaultTTL is how long an hourly index reading is kept. The distribution is
// replaced every hour; eye keeps a season of it, not a national archive.
const defaultTTL = 30 * 24 * time.Hour

// icaLayout is the timestamp the distribution carries. The data dictionary
// states it is UTC, and the value itself says so nowhere.
const icaLayout = "2006-01-02T15:04:05"

// categories names the six steps of the index, as the publisher defines them.
var categories = map[int]string{
	1: "buena",
	2: "razonablemente buena",
	3: "regular",
	4: "desfavorable",
	5: "muy desfavorable",
	6: "extremadamente desfavorable",
}

// AirQualityProvider reads a national air-quality index distribution.
type AirQualityProvider struct {
	src    source.Source
	client *httpx.Client
}

// NewAirQuality builds the adapter for a registry entry.
func NewAirQuality(src source.Source, c *httpx.Client) *AirQualityProvider {
	return &AirQualityProvider{src: src, client: c}
}

// Info implements provider.Provider.
func (p *AirQualityProvider) Info() source.Source { return p.src }

// station is one row of the distribution.
type station struct {
	Code     string
	Name     string
	Kind     string
	Position observation.Point
	Active   bool
	Reading  time.Time
	Index    int
	// Reported is false when the station published no index at all, which
	// the distribution writes as an empty cell. The publisher documents a
	// zero for "no data received" and says nothing about the blank, so the
	// two are kept apart rather than merged into one silence.
	Reported bool
	DueTo    string
}

// Poll returns one record per station the distribution reports on.
func (p *AirQualityProvider) Poll(ctx context.Context) ([]observation.Record, error) {
	rows, resp, err := p.read(ctx)
	if err != nil {
		return nil, err
	}

	sum := sha256.Sum256(resp.Body)
	prov := observation.Provenance{
		Publisher: p.src.Authority,
		SourceURL: p.distribution(),
		License:   p.src.License,
		FetchedAt: resp.FetchedAt,
		RawHash:   hex.EncodeToString(sum[:]),
	}

	ttl := p.ttl()
	records := make([]observation.Record, 0, len(rows))

	for _, s := range rows {
		category, partial := describe(s.Index)
		expires := s.Reading.Add(ttl)

		payload, err := json.Marshal(map[string]any{
			"station":        s.Code,
			"station_type":   s.Kind,
			"index":          s.Index,
			"index_reported": s.Reported,
			"category":       category,
			"partial":        partial,
			"active":         s.Active,
			"due_to":         s.DueTo,
		})
		if err != nil {
			continue
		}

		rec := observation.Record{
			ID:         p.src.ID + ":" + s.Code + ":" + s.Reading.Format(time.RFC3339),
			Source:     p.src.ID,
			Kind:       "air_quality_index",
			Topic:      p.src.Topic,
			ObservedAt: s.Reading,
			FetchedAt:  resp.FetchedAt,
			Position:   &s.Position,
			Title:      title(s, category),
			Severity:   severityOf(s.Index),
			Confidence: 1,
			Quality:    qualityOf(s, partial),
			// The station is the thing; the index is what changes about
			// it every hour.
			LocalKey:   s.Code,
			DedupeKey:  p.src.ID + ":" + s.Code + ":" + s.Reading.Format(time.RFC3339),
			ExpiresAt:  &expires,
			Payload:    payload,
			Provenance: prov,
		}
		if err := rec.Validate(); err != nil {
			continue
		}
		records = append(records, rec)
	}
	return records, nil
}

// Entities returns the measuring stations behind the readings.
func (p *AirQualityProvider) Entities(ctx context.Context) ([]observation.Entity, error) {
	rows, resp, err := p.read(ctx)
	if err != nil {
		return nil, err
	}

	sum := sha256.Sum256(resp.Body)
	prov := observation.Provenance{
		Publisher: p.src.Authority,
		SourceURL: p.distribution(),
		License:   p.src.License,
		FetchedAt: resp.FetchedAt,
		RawHash:   hex.EncodeToString(sum[:]),
	}

	seen := make(map[string]bool, len(rows))
	entities := make([]observation.Entity, 0, len(rows))

	for _, s := range rows {
		if seen[s.Code] {
			continue
		}
		seen[s.Code] = true

		payload, _ := json.Marshal(map[string]any{
			"station":      s.Code,
			"station_type": s.Kind,
			"active":       s.Active,
		})
		position := s.Position
		e := observation.Entity{
			ID:         p.src.ID + ":station:" + s.Code,
			Source:     p.src.ID,
			Kind:       "air_quality_station",
			Topic:      p.src.Topic,
			Title:      s.Name,
			Position:   &position,
			FirstSeen:  resp.FetchedAt,
			LastSeen:   resp.FetchedAt,
			Payload:    payload,
			Provenance: prov,
		}
		if err := e.Validate(); err != nil {
			continue
		}
		entities = append(entities, e)
	}
	return entities, nil
}

// read fetches the distribution and parses the rows eye can place.
func (p *AirQualityProvider) read(ctx context.Context) ([]station, *httpx.Response, error) {
	resp, err := p.client.Get(ctx, p.distribution(), httpx.Validators{})
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s: %w", ErrDCAT, p.src.ID, err)
	}

	rows, err := parseTable(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("poll %s: %w", p.src.ID, err)
	}

	west, south, east, north, filtering := p.viewport()
	kept := make([]station, 0, len(rows))
	for _, row := range rows {
		s, ok := toStation(row)
		if !ok {
			continue
		}
		if filtering && (s.Position.Lon < west || s.Position.Lon > east ||
			s.Position.Lat < south || s.Position.Lat > north) {
			continue
		}
		kept = append(kept, s)
	}
	return kept, resp, nil
}

// distribution is the table to read. The registry may point at the catalogue,
// in which case the operator names the distribution the catalogue publishes.
func (p *AirQualityProvider) distribution() string {
	return p.src.Option("distribution", p.src.URL)
}

// ttl resolves the retention window for a reading.
func (p *AirQualityProvider) ttl() time.Duration {
	if d, err := time.ParseDuration(p.src.Option("ttl", "")); err == nil && d > 0 {
		return d
	}
	return defaultTTL
}

// viewport reads an optional west,south,east,north filter. A national table is
// a lot of rows for a city gateway, and narrowing it is eye's own decision:
// the rows kept are exactly as published.
func (p *AirQualityProvider) viewport() (west, south, east, north float64, ok bool) {
	parts := strings.Split(p.src.Option("bbox", ""), ",")
	if len(parts) != 4 {
		return 0, 0, 0, 0, false
	}
	values := make([]float64, 0, 4)
	for _, part := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return 0, 0, 0, 0, false
		}
		values = append(values, v)
	}
	return values[0], values[1], values[2], values[3], true
}

// parseTable reads the distribution by column name.
func parseTable(body []byte) ([]map[string]string, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, fmt.Errorf("%w: the distribution was empty", ErrDCAT)
	}

	r := csv.NewReader(bytes.NewReader(body))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true

	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("%w: unreadable header: %w", ErrDCAT, err)
	}
	for i := range header {
		header[i] = strings.ToLower(strings.TrimSpace(header[i]))
	}
	for _, required := range []string{"cod_estacion", "latitud", "longitud", "fecha", "indice"} {
		if !contains(header, required) {
			return nil, fmt.Errorf("%w: the response is not an index table: column %q is missing (columns: %v)",
				ErrDCAT, required, header)
		}
	}

	var rows []map[string]string
	for {
		record, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			// One ragged line must not discard the country.
			continue
		}
		row := make(map[string]string, len(header))
		for i, name := range header {
			if i < len(record) {
				row[name] = strings.TrimSpace(record[i])
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// toStation reads one row, dropping anything eye cannot place in space or time.
func toStation(row map[string]string) (station, bool) {
	lat, errLat := strconv.ParseFloat(row["latitud"], 64)
	lon, errLon := strconv.ParseFloat(row["longitud"], 64)
	if errLat != nil || errLon != nil || row["cod_estacion"] == "" {
		return station{}, false
	}
	pos := observation.Point{Lat: lat, Lon: lon}
	if !pos.Valid() {
		return station{}, false
	}

	reading, err := time.Parse(icaLayout, row["fecha"])
	if err != nil {
		return station{}, false
	}

	// A blank index is a station that published nothing this hour. It is
	// still a station eye watches, and dropping the row would quietly
	// shrink the country every time a sensor went quiet.
	index, reported := 0, false
	if raw := row["indice"]; raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return station{}, false
		}
		index, reported = parsed, true
	}

	return station{
		Code:     row["cod_estacion"],
		Name:     firstNonEmpty(row["nombre"], row["cod_estacion"]),
		Kind:     row["tipo"],
		Position: pos,
		Active:   strings.EqualFold(row["activa"], "true"),
		Reading:  reading.UTC(),
		Index:    index,
		Reported: reported,
		DueTo:    row["debido_a"],
	}, true
}

// describe splits the published code into its category and whether it was
// computed from fewer pollutants than the station can measure.
//
// The publisher encodes the shortfall by multiplying the category by ten, so
// 40 is "desfavorable, but with gaps" and not a fortieth step on the ladder.
func describe(index int) (category string, partial bool) {
	if index >= 10 && index%10 == 0 {
		return categories[index/10], true
	}
	return categories[index], false
}

// severityOf ranks the index on eye's cross-source scale. A partial index keeps
// the rank of the category it reports; what it loses is not severity but
// certainty, and that is what Quality carries.
func severityOf(index int) observation.Severity {
	step := index
	if step >= 10 && step%10 == 0 {
		step /= 10
	}
	switch step {
	case 1, 2:
		return observation.SeverityInfo
	case 3:
		return observation.SeverityLow
	case 4:
		return observation.SeverityModerate
	case 5:
		return observation.SeverityHigh
	case 6:
		return observation.SeverityCritical
	default:
		// Zero is the publisher saying no data arrived from the station.
		return observation.SeverityNone
	}
}

// qualityOf reports how far the index can be trusted.
//
// The full index is the competent authority's own published statement. An index
// short of pollutants, or one from a station the publisher marks out of
// service, is not: it is preliminary, and saying otherwise would launder the
// publisher's own caveat out of the data.
func qualityOf(s station, partial bool) observation.Quality {
	if partial || !s.Active || !s.Reported || s.Index == 0 {
		return observation.QualityPreliminary
	}
	return observation.QualityOfficial
}

// title names the station and what its air was like.
func title(s station, category string) string {
	if category == "" {
		return s.Name + ": sin datos"
	}
	return s.Name + ": calidad del aire " + category
}

// contains reports whether a column name is present.
func contains(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
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

var (
	_ provider.Provider       = (*AirQualityProvider)(nil)
	_ provider.EntityProvider = (*AirQualityProvider)(nil)
)
