package ckan_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/FullFran/eye/internal/httpx"
	observation "github.com/FullFran/eye/internal/observation/domain"
	"github.com/FullFran/eye/internal/provider/infrastructure/ckan"
	source "github.com/FullFran/eye/internal/source/domain"
)

// portal serves a minimal but faithful CKAN instance: package_search,
// package_show, and the GeoJSON distribution behind a resource URL.
func portal(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	srv := httptest.NewUnstartedServer(mux)

	mux.HandleFunc("/api/3/action/package_search", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"success": true,
			"result": map[string]any{
				"count": 2,
				"results": []map[string]any{
					{
						"name": "camaras-de-trafico", "title": "Cámaras de tráfico",
						"notes": "Localización de las cámaras.", "license_title": "License not specified",
						"metadata_modified": "2024-02-12T08:16:15.485475",
						"resources":         []map[string]any{{"format": "CSV"}, {"format": "GeoJSON"}},
					},
					{
						"name": "accidentes", "title": "Accidentes de tráfico",
						"license_title":     "CC BY 4.0",
						"metadata_modified": "2026-08-01T10:00:00.000000",
						"resources":         []map[string]any{{"format": "CSV"}},
					},
				},
			},
		})
	})

	mux.HandleFunc("/api/3/action/package_show", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id") != "camaras-de-trafico" {
			writeJSON(w, map[string]any{"success": false})
			return
		}
		writeJSON(w, map[string]any{
			"success": true,
			"result": map[string]any{
				"name": "camaras-de-trafico", "title": "Cámaras de tráfico",
				"license_title": "License not specified",
				"resources": []map[string]any{
					{"format": "CSV", "name": "camaras.csv", "url": srv.URL + "/download/camaras.csv"},
					{"format": "GeoJSON", "name": "camarastrafico.json", "url": srv.URL + "/download/cams.json"},
				},
			},
		})
	})

	mux.HandleFunc("/download/cams.json", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"type": "FeatureCollection",
			"features": []map[string]any{
				{
					"type": "Feature", "id": "num.0",
					"geometry":   map[string]any{"type": "Point", "coordinates": []float64{-4.79954, 37.89841}},
					"properties": map[string]any{"name": "AVDA. DE LA ARRUZAFILLA"},
				},
				{
					"type": "Feature", "id": "num.1",
					"geometry":   map[string]any{"type": "LineString", "coordinates": []float64{-4.7, 37.8}},
					"properties": map[string]any{"name": "not a point"},
				},
				{
					"type": "Feature", "id": "num.2",
					"geometry":   map[string]any{"type": "Point", "coordinates": []float64{-999, 999}},
					"properties": map[string]any{"name": "out of range"},
				},
			},
		})
	})

	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

// writeJSON is the test server's response helper.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// portalSource builds a registry entry for a CKAN portal.
func portalSource(url, format string, opts map[string]string) source.Source {
	return source.Source{
		ID: "cordoba-ckan", Authority: "Ayuntamiento de Cordoba", Topic: "city", URL: url,
		Format: format, License: "unspecified",
		Access: source.AccessDocumentedAPI, Automation: source.AutomationEnabled,
		Interval: 12 * time.Hour, Options: opts,
	}
}

func TestCatalogPollEmitsDatasetUpdates(t *testing.T) {
	t.Parallel()

	srv := portal(t)
	records, err := ckan.NewCatalog(portalSource(srv.URL, "ckan-catalog", nil), httpx.New()).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}

	for _, r := range records {
		if r.Kind != "dataset_update" {
			t.Errorf("kind = %q", r.Kind)
		}
		if err := r.Validate(); err != nil {
			t.Errorf("record does not validate: %v", err)
		}
	}
}

// CKAN says "License not specified" when nobody declared one. That is a fact
// worth keeping, not a licence and not a blank.
func TestCatalogPreservesUnspecifiedLicence(t *testing.T) {
	t.Parallel()

	srv := portal(t)
	records, _ := ckan.NewCatalog(portalSource(srv.URL, "ckan-catalog", nil), httpx.New()).Poll(context.Background())

	byName := map[string]string{}
	for _, r := range records {
		byName[r.Title] = r.Provenance.License
	}

	if got := byName["Cámaras de tráfico"]; got != "unspecified" {
		t.Errorf("undeclared licence stored as %q, want %q", got, "unspecified")
	}
	if got := byName["Accidentes de tráfico"]; got != "CC BY 4.0" {
		t.Errorf("declared licence stored as %q", got)
	}
}

func TestGeoJSONEntities(t *testing.T) {
	t.Parallel()

	srv := portal(t)
	src := portalSource(srv.URL, "ckan-geojson", map[string]string{
		"dataset": "camaras-de-trafico", "kind": "camera",
	})

	entities, err := ckan.NewGeoJSON(src, httpx.New()).Entities(context.Background())
	if err != nil {
		t.Fatalf("Entities() = %v", err)
	}

	// One valid point; a LineString and an out-of-range point are dropped.
	if len(entities) != 1 {
		t.Fatalf("entities = %d, want 1", len(entities))
	}

	e := entities[0]
	if e.Kind != "camera" {
		t.Errorf("kind = %q", e.Kind)
	}
	if e.Title != "AVDA. DE LA ARRUZAFILLA" {
		t.Errorf("title = %q", e.Title)
	}
	// GeoJSON is lon, lat. Reversing it puts every camera in the ocean.
	if e.Position == nil || e.Position.Lat < 37 || e.Position.Lat > 38 {
		t.Errorf("position = %+v, want a latitude near Cordoba", e.Position)
	}
	if e.Provenance.License != "unspecified" {
		t.Errorf("licence = %q", e.Provenance.License)
	}
	if e.Provenance.RawHash == "" {
		t.Error("no raw hash; the evidence chain is broken")
	}
}

func TestGeoJSONPollSummarizesInventory(t *testing.T) {
	t.Parallel()

	srv := portal(t)
	src := portalSource(srv.URL, "ckan-geojson", map[string]string{"dataset": "camaras-de-trafico", "kind": "camera"})

	records, err := ckan.NewGeoJSON(src, httpx.New()).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want a single inventory summary", len(records))
	}
	if records[0].Kind != "inventory_refresh" {
		t.Errorf("kind = %q", records[0].Kind)
	}
	if !strings.Contains(records[0].Title, "camera") {
		t.Errorf("title = %q, want it to name the asset kind", records[0].Title)
	}
}

func TestGeoJSONRequiresADataset(t *testing.T) {
	t.Parallel()

	srv := portal(t)
	_, err := ckan.NewGeoJSON(portalSource(srv.URL, "ckan-geojson", nil), httpx.New()).Entities(context.Background())
	if err == nil {
		t.Fatal("Entities() with no dataset option = nil error")
	}
}

func TestCallReportsPortalFailure(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"success": false})
	}))
	defer srv.Close()

	_, err := ckan.NewCatalog(portalSource(srv.URL, "ckan-catalog", nil), httpx.New()).Poll(context.Background())
	if err == nil {
		t.Fatal("Poll() against a failing portal = nil error")
	}
}

var _ = observation.Record{}
