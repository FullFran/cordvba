package infrastructure

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	domain "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
)

// scanner is satisfied by *sql.Row and *sql.Rows.
type scanner interface{ Scan(dest ...any) error }

// recordPredicates renders the WHERE clause for a record query.
//
// Every value goes through a placeholder. The only text this function
// concatenates is its own, which is what keeps a filter from being an
// injection vector.
func recordPredicates(f domain.Filter) (string, []any) {
	var (
		clauses []string
		args    []any
	)

	addIn := func(column string, values []string) {
		if len(values) == 0 {
			return
		}
		clauses = append(clauses, fmt.Sprintf("LOWER(%s) IN (%s)", column, placeholders(len(values))))
		for _, v := range values {
			args = append(args, strings.ToLower(v))
		}
	}

	addIn("topic", f.Topics)
	addIn("source", f.Sources)
	addIn("kind", f.Kinds)

	if f.Since != nil {
		clauses = append(clauses, "observed_at >= ?")
		args = append(args, micros(*f.Since))
	}
	if f.Until != nil {
		clauses = append(clauses, "observed_at <= ?")
		args = append(args, micros(*f.Until))
	}
	if f.MinSeverity > 0 {
		clauses = append(clauses, "severity >= ?")
		args = append(args, int(f.MinSeverity))
	}
	// Text is deliberately NOT pushed into SQL. SQLite's LIKE does not fold
	// accents, so "cordoba" would miss "CÓRDOBA" — and the sources eye reads
	// are Spanish. The filter is applied in Go instead, which costs a scan of
	// the rows the other predicates already narrowed.

	clauses, args = appendSpatial(clauses, args, f)

	return renderWhere(clauses), args
}

// entityPredicates renders the WHERE clause for an entity query.
func entityPredicates(f domain.Filter) (string, []any) {
	var (
		clauses []string
		args    []any
	)

	addIn := func(column string, values []string) {
		if len(values) == 0 {
			return
		}
		clauses = append(clauses, fmt.Sprintf("LOWER(%s) IN (%s)", column, placeholders(len(values))))
		for _, v := range values {
			args = append(args, strings.ToLower(v))
		}
	}

	addIn("topic", f.Topics)
	addIn("source", f.Sources)
	addIn("kind", f.Kinds)

	// Text is applied in Go, for the accent-folding reason above.

	clauses, args = appendSpatial(clauses, args, f)

	return renderWhere(clauses), args
}

// appendSpatial adds the box predicate. A radius filter is narrowed here to its
// enclosing box and refined in Go, because a great-circle distance cannot be
// indexed without PostGIS.
func appendSpatial(clauses []string, args []any, f domain.Filter) ([]string, []any) {
	box := f.BBox
	if box == nil && f.Near != nil && f.RadiusKm > 0 {
		enclosing := boundingBoxAround(*f.Near, f.RadiusKm)
		box = &enclosing
	}
	if box == nil {
		return clauses, args
	}

	clauses = append(clauses,
		"lat IS NOT NULL AND lat BETWEEN ? AND ? AND lon BETWEEN ? AND ?")
	args = append(args, box.South, box.North, box.West, box.East)

	return clauses, args
}

// degreesPerKmLat is the latitude change of one kilometre. Longitude degrees
// shrink with latitude, hence the cosine below.
const degreesPerKmLat = 1.0 / 111.32

// boundingBoxAround returns the smallest box that certainly contains the circle
// of the given radius. It over-selects slightly, which is exactly what a
// prefilter should do: the Go-side haversine check then discards the corners.
func boundingBoxAround(centre domain.Point, radiusKm float64) domain.BBox {
	latDelta := radiusKm * degreesPerKmLat

	// Guard against the poles, where the cosine collapses and the longitude
	// span becomes the whole circle.
	cos := math.Cos(centre.Lat * math.Pi / 180)
	lonDelta := 180.0
	if cos > 1e-6 {
		lonDelta = math.Min(180.0, latDelta/cos)
	}

	return domain.BBox{
		South: math.Max(-90, centre.Lat-latDelta),
		North: math.Min(90, centre.Lat+latDelta),
		West:  math.Max(-180, centre.Lon-lonDelta),
		East:  math.Min(180, centre.Lon+lonDelta),
	}
}

// renderWhere joins clauses into a WHERE fragment.
func renderWhere(clauses []string) string {
	if len(clauses) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(clauses, " AND ")
}

// scanRecord materializes one row.
func scanRecord(s scanner) (domain.Record, error) {
	var (
		r                                             domain.Record
		validFrom, validUntil, expiresAt              sql.NullInt64
		lat, lon                                      sql.NullFloat64
		geometry, entityID, localKey, dedupe, payload sql.NullString
		observedAt, fetchedAt                         int64
		severity                                      int
		quality                                       string
	)

	if err := s.Scan(
		&r.ID, &r.Source, &r.Kind, &r.Topic,
		&observedAt, &fetchedAt, &validFrom, &validUntil, &expiresAt,
		&lat, &lon, &geometry,
		&r.Title, &r.Description,
		&severity, &r.Confidence, &quality,
		&entityID, &localKey, &dedupe, &payload,
		&r.Provenance.Publisher, &r.Provenance.SourceURL, &r.Provenance.License, &r.Provenance.RawHash,
	); err != nil {
		return domain.Record{}, fmt.Errorf("%w: scan record: %w", ErrStore, err)
	}

	r.ObservedAt = timeFromMicros(observedAt)
	r.FetchedAt = timeFromMicros(fetchedAt)
	r.Severity = domain.Severity(severity)
	r.Quality = domain.Quality(quality)
	r.Provenance.FetchedAt = r.FetchedAt

	r.ValidFrom = timePtrFromNull(validFrom)
	r.ValidUntil = timePtrFromNull(validUntil)
	r.ExpiresAt = timePtrFromNull(expiresAt)

	if lat.Valid && lon.Valid {
		r.Position = &domain.Point{Lat: lat.Float64, Lon: lon.Float64}
	}
	if geometry.Valid {
		r.Geometry = json.RawMessage(geometry.String)
	}
	if entityID.Valid {
		id := entityID.String
		r.EntityID = &id
	}
	if localKey.Valid {
		r.LocalKey = localKey.String
	}
	if dedupe.Valid {
		r.DedupeKey = dedupe.String
	}
	if payload.Valid {
		r.Payload = json.RawMessage(payload.String)
	}

	return r, nil
}

// scanEntity materializes one inventory row.
func scanEntity(s scanner) (domain.Entity, error) {
	var (
		e                   domain.Entity
		lat, lon            sql.NullFloat64
		geometry, payload   sql.NullString
		firstSeen, lastSeen int64
	)

	if err := s.Scan(
		&e.ID, &e.Source, &e.Kind, &e.Topic, &e.Title, &e.Description,
		&lat, &lon, &geometry, &firstSeen, &lastSeen, &payload,
		&e.Provenance.Publisher, &e.Provenance.SourceURL, &e.Provenance.License, &e.Provenance.RawHash,
	); err != nil {
		return domain.Entity{}, fmt.Errorf("%w: scan entity: %w", ErrStore, err)
	}

	e.FirstSeen = timeFromMicros(firstSeen)
	e.LastSeen = timeFromMicros(lastSeen)
	e.Provenance.FetchedAt = e.LastSeen

	if lat.Valid && lon.Valid {
		e.Position = &domain.Point{Lat: lat.Float64, Lon: lon.Float64}
	}
	if geometry.Valid {
		e.Geometry = json.RawMessage(geometry.String)
	}
	if payload.Valid {
		e.Payload = json.RawMessage(payload.String)
	}

	return e, nil
}

// timePtrFromNull converts a nullable column into an optional time.
func timePtrFromNull(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := timeFromMicros(v.Int64)
	return &t
}
