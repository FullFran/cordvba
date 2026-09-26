package api

import (
	"encoding/json"
	"net/http"
	"time"

	observation "github.com/FullFran/eye/internal/observation/domain"
)

// featureCollection is an RFC 7946 FeatureCollection.
//
// It is spelled out rather than assembled from maps because the format is a
// contract: a map library that meets "coordinates":[lat,lon] renders Córdoba
// in the Indian Ocean and reports no error at all.
type featureCollection struct {
	Type     string    `json:"type"`
	Features []feature `json:"features"`
}

// feature is one member of the collection.
type feature struct {
	Type       string          `json:"type"`
	ID         string          `json:"id,omitempty"`
	Geometry   json.RawMessage `json:"geometry"`
	Properties map[string]any  `json:"properties"`
}

// point is a GeoJSON Point. Longitude comes first: RFC 7946 §3.1.1.
type point struct {
	Type        string     `json:"type"`
	Coordinates [2]float64 `json:"coordinates"`
}

// handleGeoJSON serves records with a location as a FeatureCollection.
func (s *Server) handleGeoJSON(w http.ResponseWriter, r *http.Request) {
	filter, err := filterFrom(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	records, err := s.store.Query(r.Context(), filter)
	if err != nil {
		s.log.Error("geojson query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	records = s.redistributable(records)
	features := make([]feature, 0, len(records))

	for _, rec := range records {
		geometry, ok := geometryOf(rec.Geometry, rec.Position)
		if !ok {
			// eye does not invent coordinates to satisfy a query.
			continue
		}
		features = append(features, feature{
			Type: "Feature", ID: rec.ID, Geometry: geometry,
			Properties: map[string]any{
				"id":          rec.ID,
				"source":      rec.Source,
				"kind":        rec.Kind,
				"topic":       rec.Topic,
				"title":       rec.Title,
				"severity":    int(rec.Severity),
				"observed_at": rec.ObservedAt.UTC().Format(time.RFC3339),
				"license":     rec.Provenance.License,
			},
		})
	}

	writeJSON(w, http.StatusOK, featureCollection{Type: "FeatureCollection", Features: features})
}

// handleEntitiesGeoJSON serves inventory with a location as a
// FeatureCollection.
func (s *Server) handleEntitiesGeoJSON(w http.ResponseWriter, r *http.Request) {
	filter, err := filterFrom(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	entities, err := s.store.Entities(r.Context(), filter)
	if err != nil {
		s.log.Error("entity geojson query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	features := make([]feature, 0, len(entities))
	for _, ent := range entities {
		if s.nonRedistributable[ent.Source] {
			continue
		}
		geometry, ok := geometryOf(ent.Geometry, ent.Position)
		if !ok {
			continue
		}
		features = append(features, feature{
			Type: "Feature", ID: ent.ID, Geometry: geometry,
			Properties: map[string]any{
				"id":     ent.ID,
				"source": ent.Source,
				"kind":   ent.Kind,
				"topic":  ent.Topic,
				"title":  ent.Title,
				// An entity has no severity of its own; it is carried so
				// one client code path can style both collections.
				"severity":   0,
				"first_seen": ent.FirstSeen.UTC().Format(time.RFC3339),
				"last_seen":  ent.LastSeen.UTC().Format(time.RFC3339),
				// observed_at mirrors last_seen, for the same reason.
				"observed_at": ent.LastSeen.UTC().Format(time.RFC3339),
				"license":     ent.Provenance.License,
			},
		})
	}

	writeJSON(w, http.StatusOK, featureCollection{Type: "FeatureCollection", Features: features})
}

// geometryOf picks the geometry to render.
//
// A stored geometry wins: collapsing a road segment or a fire perimeter to its
// centre would throw away the shape that is the reason it was stored as a
// geometry. Absent that, a point. Absent both, nothing — which is not the same
// as a point at 0,0 off the coast of Ghana.
func geometryOf(geometry json.RawMessage, position *observation.Point) (json.RawMessage, bool) {
	if len(geometry) > 0 {
		return geometry, true
	}
	if position == nil || !position.Valid() {
		return nil, false
	}

	encoded, err := json.Marshal(point{Type: "Point", Coordinates: [2]float64{position.Lon, position.Lat}})
	if err != nil {
		return nil, false
	}
	return encoded, true
}
