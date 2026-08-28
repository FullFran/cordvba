// Package geojson maps GeoJSON feature collections onto eye entities.
//
// Both the CKAN adapter and the WFS adapter end up holding a FeatureCollection,
// and the rules for turning one into inventory — which geometries are usable,
// where the human label comes from, which properties are worth keeping — are
// the same either way. They live here so the two adapters cannot drift.
package geojson

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	observation "github.com/FullFran/eye/internal/observation/domain"
)

// FeatureCollection is the subset of GeoJSON eye reads.
type FeatureCollection struct {
	Type     string    `json:"type"`
	Features []Feature `json:"features"`
}

// Feature is one geometry with its properties.
type Feature struct {
	ID       string `json:"id"`
	Geometry struct {
		Type        string          `json:"type"`
		Coordinates json.RawMessage `json:"coordinates"`
	} `json:"geometry"`
	Properties map[string]any `json:"properties"`
}

// Parse decodes a feature collection.
func Parse(body []byte) (*FeatureCollection, error) {
	var fc FeatureCollection
	if err := json.Unmarshal(body, &fc); err != nil {
		return nil, fmt.Errorf("geojson: %w", err)
	}
	return &fc, nil
}

// Options control how features become entities.
type Options struct {
	// SourceID prefixes every entity id.
	SourceID string
	// Kind is the entity kind, such as "camera" or "patio".
	Kind string
	// Topic is the entity topic.
	Topic string
	// SeenAt is when the collection was fetched.
	SeenAt time.Time
	// Provenance is attached to every entity.
	Provenance observation.Provenance
}

// internalProperties are dropped rather than stored.
//
// Municipal exports carry the internal file path a layer was built from — a UNC
// share on somebody's office network. That is operational noise at best and
// information about their internal systems at worst, and eye has no use for it
// either way.
var internalProperties = map[string]bool{
	"path": true, "ruta": true, "fichero": true, "imagen": true,
	"shape_area": true, "shape_len": true, "shape.area": true, "shape.len": true,
	"objectid": true, "gid": true,
}

// personalProperties are dropped unconditionally, before anything is stored.
//
// This is not a configuration option and must not become one. Cordoba's Patios
// layer publishes the name, telephone number and email address of the person
// responsible for each courtyard — real contact details for private citizens,
// in an open WFS anyone can query. The data being reachable does not make
// collecting it acceptable, and ADR-0007 exists precisely for the moment when
// taking it would be trivially easy.
//
// eye keeps where a patio is. Who owns it, and how to telephone them, is none
// of its business.
var personalProperties = map[string]bool{
	// Spanish, as the municipal layers spell them.
	"respons": true, "responsable": true, "propietario": true, "titular": true,
	"telefono": true, "tlf": true, "movil": true, "contacto": true,
	"email": true, "correo": true, "dni": true, "nif": true, "nombre_contacto": true,
	// English, for any source that arrives later.
	"owner": true, "phone": true, "mobile": true, "contact": true,
	"contact_name": true, "contact_email": true, "contact_phone": true,
}

// DroppedPersonalFields reports which personal fields a property set carried,
// so a provider can say that it refused them rather than silently discarding.
func DroppedPersonalFields(props map[string]any) []string {
	var out []string
	for k := range props {
		if personalProperties[strings.ToLower(strings.TrimSpace(k))] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// titleKeys are the property names publishers actually use for a human label,
// in the order they are tried.
var titleKeys = []string{
	"name", "nombre", "title", "titulo", "denominacion", "descripcion",
	"direccion", "rotulo", "texto",
}

// Entities maps a collection onto inventory, skipping anything eye cannot place.
func (fc *FeatureCollection) Entities(opts Options) []observation.Entity {
	out := make([]observation.Entity, 0, len(fc.Features))

	for i, f := range fc.Features {
		point, ok := f.position()
		if !ok {
			continue
		}

		id := f.ID
		if id == "" {
			id = strconv.Itoa(i)
		}

		payload, _ := json.Marshal(cleanProperties(f.Properties))

		ent := observation.Entity{
			ID:         opts.SourceID + ":" + id,
			Source:     opts.SourceID,
			Kind:       opts.Kind,
			Topic:      opts.Topic,
			Title:      Title(f.Properties, opts.Kind+" "+id),
			Position:   &point,
			Geometry:   f.rawGeometry(),
			FirstSeen:  opts.SeenAt,
			LastSeen:   opts.SeenAt,
			Payload:    payload,
			Provenance: opts.Provenance,
		}
		if err := ent.Validate(); err != nil {
			continue
		}
		out = append(out, ent)
	}
	return out
}

// position returns a single coordinate for a feature.
//
// A Point gives its own. Anything else — a park, a block, a road segment —
// gets the centre of its bounding box, and the full geometry is kept alongside
// it so nothing is lost. Half the municipal layers are polygons, and dropping
// them would be a worse answer than placing a park at its middle and saying so.
func (f Feature) position() (observation.Point, bool) {
	coords := f.flatCoordinates()
	if len(coords) == 0 {
		return observation.Point{}, false
	}

	if strings.EqualFold(f.Geometry.Type, "Point") {
		return coords[0], coords[0].Valid() && !isNullIsland(coords[0])
	}

	minLat, maxLat := coords[0].Lat, coords[0].Lat
	minLon, maxLon := coords[0].Lon, coords[0].Lon
	for _, c := range coords[1:] {
		minLat, maxLat = math.Min(minLat, c.Lat), math.Max(maxLat, c.Lat)
		minLon, maxLon = math.Min(minLon, c.Lon), math.Max(maxLon, c.Lon)
	}

	centre := observation.Point{Lat: (minLat + maxLat) / 2, Lon: (minLon + maxLon) / 2}
	return centre, centre.Valid() && !isNullIsland(centre)
}

// flatCoordinates walks a geometry of any nesting depth and returns every
// coordinate pair it contains.
//
// GeoJSON nests differently per type — Point is [x,y], LineString is [[x,y]…],
// Polygon adds a ring level, Multi* adds another. Walking generically avoids
// four near-identical decoders.
func (f Feature) flatCoordinates() []observation.Point {
	var raw any
	if err := json.Unmarshal(f.Geometry.Coordinates, &raw); err != nil {
		return nil
	}

	var out []observation.Point
	var walk func(v any)
	walk = func(v any) {
		list, ok := v.([]any)
		if !ok || len(list) == 0 {
			return
		}
		// A coordinate pair is the innermost level: two or three numbers.
		if lon, lonOK := list[0].(float64); lonOK && len(list) >= 2 {
			if lat, latOK := list[1].(float64); latOK {
				// GeoJSON is lon, lat. Reversing it puts every
				// Cordoba feature in the Indian Ocean.
				out = append(out, observation.Point{Lon: lon, Lat: lat})
				return
			}
		}
		for _, item := range list {
			walk(item)
		}
	}
	walk(raw)

	return out
}

// rawGeometry returns the feature's geometry as GeoJSON, so a polygon survives
// even though the entity is placed at a single point.
func (f Feature) rawGeometry() json.RawMessage {
	if strings.EqualFold(f.Geometry.Type, "Point") || len(f.Geometry.Coordinates) == 0 {
		return nil
	}
	raw, err := json.Marshal(map[string]any{
		"type":        f.Geometry.Type,
		"coordinates": f.Geometry.Coordinates,
	})
	if err != nil {
		return nil
	}
	return raw
}

// isNullIsland reports the 0,0 coordinate that missing data decays into.
func isNullIsland(p observation.Point) bool { return p.Lat == 0 && p.Lon == 0 }

// Title picks a human label out of whatever the publisher called the field.
func Title(props map[string]any, fallback string) string {
	for _, key := range titleKeys {
		for name, value := range props {
			if !strings.EqualFold(name, key) {
				continue
			}
			// A title must never be sourced from a personal field, even
			// if a publisher names one of them "nombre".
			if personalProperties[strings.ToLower(strings.TrimSpace(name))] {
				continue
			}
			if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return fallback
}

// cleanProperties drops personal data, internal fields and empty values.
func cleanProperties(props map[string]any) map[string]any {
	out := make(map[string]any, len(props))
	for k, v := range props {
		key := strings.ToLower(strings.TrimSpace(k))
		if personalProperties[key] || internalProperties[key] {
			continue
		}
		if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
			continue
		}
		if v == nil {
			continue
		}
		out[k] = v
	}
	return out
}
