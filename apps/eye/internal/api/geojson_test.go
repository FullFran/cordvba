package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	observation "github.com/FullFran/eye/internal/observation/domain"
)

// positioned returns a record with a point location.
func positioned(id string) observation.Record {
	r := record(id, "traffic")
	r.Position = &observation.Point{Lat: 37.8882, Lon: -4.7794}
	r.Severity = observation.SeverityHigh
	return r
}

// shaped returns a record whose location is a geometry rather than a point.
func shaped(id string) observation.Record {
	r := record(id, "traffic")
	r.Geometry = json.RawMessage(`{"type":"LineString","coordinates":[[-4.78,37.88],[-4.77,37.89]]}`)
	return r
}

// A FeatureCollection that is not RFC 7946 is a file every map library
// refuses, which is the entire audience for this endpoint.
func TestGeoJSONIsARealFeatureCollection(t *testing.T) {
	t.Parallel()

	store := &fakeStore{records: []observation.Record{positioned("1")}}
	srv := serve(t, store)

	status, body := get(t, srv, "/v1/geojson")
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if body["type"] != "FeatureCollection" {
		t.Fatalf("type = %v", body["type"])
	}

	features, _ := body["features"].([]any)
	if len(features) != 1 {
		t.Fatalf("features = %v", body["features"])
	}

	feature, _ := features[0].(map[string]any)
	if feature["type"] != "Feature" {
		t.Errorf("feature type = %v", feature["type"])
	}

	geometry, _ := feature["geometry"].(map[string]any)
	if geometry["type"] != "Point" {
		t.Fatalf("geometry = %v", geometry)
	}
	coords, _ := geometry["coordinates"].([]any)
	// RFC 7946 is longitude first. Getting this backwards puts Córdoba in
	// the Indian Ocean and every map renders it without complaining.
	if len(coords) != 2 || coords[0] != -4.7794 || coords[1] != 37.8882 {
		t.Errorf("coordinates = %v, want [lon, lat]", coords)
	}

	props, _ := feature["properties"].(map[string]any)
	for _, field := range []string{"id", "source", "kind", "topic", "title", "severity", "observed_at", "license"} {
		if _, ok := props[field]; !ok {
			t.Errorf("properties are missing %q: %v", field, props)
		}
	}
}

// A record that already carries a geometry keeps it: collapsing a road segment
// or a fire perimeter to its point would throw away the shape that matters.
func TestGeoJSONKeepsAnExistingGeometry(t *testing.T) {
	t.Parallel()

	srv := serve(t, &fakeStore{records: []observation.Record{shaped("1")}})
	_, body := get(t, srv, "/v1/geojson")

	features, _ := body["features"].([]any)
	if len(features) != 1 {
		t.Fatalf("features = %v", body["features"])
	}
	feature, _ := features[0].(map[string]any)
	geometry, _ := feature["geometry"].(map[string]any)
	if geometry["type"] != "LineString" {
		t.Errorf("geometry = %v, want the record's own", geometry)
	}
}

// eye does not invent coordinates to satisfy a query, so a record with no
// location is absent rather than dropped at 0,0 off the coast of Ghana.
func TestGeoJSONSkipsRecordsWithNoLocation(t *testing.T) {
	t.Parallel()

	store := &fakeStore{records: []observation.Record{positioned("1"), record("2", "press")}}
	srv := serve(t, store)

	_, body := get(t, srv, "/v1/geojson")
	features, _ := body["features"].([]any)
	if len(features) != 1 {
		t.Fatalf("features = %d, want only the located one", len(features))
	}
}

func TestGeoJSONAppliesTheSameFilters(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	srv := serve(t, store)

	get(t, srv, "/v1/geojson?topic=traffic&since=2h&limit=7") //nolint:dogsled // asserting on the filter
	if f := store.lastFilter; len(f.Topics) != 1 || f.Topics[0] != "traffic" || f.Limit != 7 || f.Since == nil {
		t.Errorf("filter = %+v", f)
	}
}

// The redistribution rule is not a property of one handler.
func TestGeoJSONNeverServesAPersonalSource(t *testing.T) {
	t.Parallel()

	personal := positioned("secret")
	personal.Source = "aucorsa-arrivals"

	srv := serveWithPersonalSource(t, &fakeStore{
		records: []observation.Record{positioned("public"), personal},
	})

	_, body := get(t, srv, "/v1/geojson")
	features, _ := body["features"].([]any)
	if len(features) != 1 {
		t.Fatalf("features = %d, want only the redistributable one", len(features))
	}
}

func TestEntitiesGeoJSON(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	entity := observation.Entity{
		ID: "cordoba-cameras:1", Source: "cordoba-cameras", Kind: "camera", Topic: "traffic",
		Title: "Ronda de los Tejares", Position: &observation.Point{Lat: 37.89, Lon: -4.78},
		FirstSeen: at, LastSeen: at,
		Provenance: observation.Provenance{
			Publisher: "Ayuntamiento de Cordoba", SourceURL: "https://example.org/1",
			License: "CC-BY-4.0", FetchedAt: at,
		},
	}

	srv := serve(t, &fakeStore{entities: []observation.Entity{entity}})
	status, body := get(t, srv, "/v1/entities.geojson")

	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if body["type"] != "FeatureCollection" {
		t.Fatalf("type = %v", body["type"])
	}

	features, _ := body["features"].([]any)
	if len(features) != 1 {
		t.Fatalf("features = %v", body["features"])
	}
	feature, _ := features[0].(map[string]any)
	props, _ := feature["properties"].(map[string]any)
	if props["title"] != "Ronda de los Tejares" || props["license"] != "CC-BY-4.0" {
		t.Errorf("properties = %v", props)
	}
}
