package geojson_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	"github.com/FullFran/cordvba/apps/eye/internal/provider/infrastructure/geojson"
)

// opts returns mapping options for a test.
func opts(kind string) geojson.Options {
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	return geojson.Options{
		SourceID: "cordoba-patios", Kind: kind, Topic: "events", SeenAt: now,
		Provenance: observation.Provenance{
			Publisher: "Ayuntamiento de Cordoba", SourceURL: "https://ide.cordoba.es/geoserver/ows",
			License: "unspecified", FetchedAt: now,
		},
	}
}

// fixture loads a recorded WFS response.
func fixture(t *testing.T, name string) []byte {
	t.Helper()

	body, err := os.ReadFile("../../../../testdata/wfs/" + name) // #nosec G304 -- fixture path is a literal
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return body
}

// Cordoba's Patios layer publishes the name, telephone and email of the person
// responsible for each courtyard. eye keeps where a patio is; who owns it and
// how to telephone them is none of its business.
//
// This is the sharpest test of ADR-0007 in the codebase: the data is public,
// reachable, and trivially easy to take.
func TestPersonalDataIsNeverStored(t *testing.T) {
	t.Parallel()

	fc, err := geojson.Parse(fixture(t, "patios.json"))
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}

	// The fixture must actually contain the fields, or this test passes
	// while proving nothing.
	dropped := geojson.DroppedPersonalFields(fc.Features[0].Properties)
	if len(dropped) == 0 {
		t.Fatal("the fixture carries no personal fields; this test would be vacuous")
	}
	t.Logf("refused: %s", strings.Join(dropped, ", "))

	entities := fc.Entities(opts("patio"))
	if len(entities) == 0 {
		t.Fatal("no entities produced")
	}

	for _, e := range entities {
		var payload map[string]any
		if err := json.Unmarshal(e.Payload, &payload); err != nil {
			t.Fatalf("payload is not JSON: %v", err)
		}
		for _, forbidden := range []string{"respons", "telefono", "email", "propietario", "contacto"} {
			if _, present := payload[forbidden]; present {
				t.Errorf("%q survived into the payload", forbidden)
			}
		}
		// Nor may it leak through the title.
		if strings.Contains(strings.ToLower(e.Title), "@") {
			t.Errorf("title looks like an email address: %q", e.Title)
		}
	}
}

func TestDroppedPersonalFieldsNamesThem(t *testing.T) {
	t.Parallel()

	got := geojson.DroppedPersonalFields(map[string]any{
		"name": "Calle Martín de Roa, 7", "TELEFONO": "x", "Email": "y", "respons": "z", "itin.": "6",
	})

	if len(got) != 3 {
		t.Fatalf("dropped = %v, want three personal fields", got)
	}
	for _, want := range []string{"Email", "TELEFONO", "respons"} {
		var found bool
		for _, g := range got {
			if g == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%q was not reported as dropped; matching must be case-insensitive", want)
		}
	}
}

func TestEntitiesKeepsTheUsefulPatioFields(t *testing.T) {
	t.Parallel()

	fc, _ := geojson.Parse(fixture(t, "patios.json"))
	entities := fc.Entities(opts("patio"))

	e := entities[0]
	if e.Kind != "patio" || e.Topic != "events" {
		t.Errorf("kind/topic = %q/%q", e.Kind, e.Topic)
	}
	if e.Position == nil || e.Position.Lat < 37 || e.Position.Lat > 38 {
		t.Errorf("position = %+v, want a point in Cordoba", e.Position)
	}
	if !strings.Contains(e.Title, "Calle") && !strings.Contains(e.Title, "Plaza") {
		t.Errorf("title = %q, want the street address", e.Title)
	}

	var payload map[string]any
	_ = json.Unmarshal(e.Payload, &payload)
	// The itinerary is what makes a patio findable on a route.
	if _, ok := payload["nomb itine"]; !ok {
		t.Errorf("the itinerary name was dropped: %v", payload)
	}
	// The internal UNC path must not survive.
	if _, ok := payload["imagen"]; ok {
		t.Error("the internal file path survived as \"imagen\"")
	}
}

// Half the municipal layers are polygons. Dropping them would be a worse answer
// than placing a park at the centre of its bounding box and keeping the shape.
func TestPolygonsBecomeEntitiesWithTheirGeometryKept(t *testing.T) {
	t.Parallel()

	fc, err := geojson.Parse(fixture(t, "zonas_verdes.json"))
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}
	if fc.Features[0].Geometry.Type == "Point" {
		t.Skip("fixture is not a polygon layer")
	}

	entities := fc.Entities(opts("green_space"))
	if len(entities) == 0 {
		t.Fatal("a polygon layer produced no entities")
	}

	e := entities[0]
	if e.Position == nil {
		t.Fatal("no representative point")
	}
	if e.Position.Lat < 37 || e.Position.Lat > 38 || e.Position.Lon < -5.5 || e.Position.Lon > -4 {
		t.Errorf("representative point %+v is not in Cordoba", e.Position)
	}
	if len(e.Geometry) == 0 {
		t.Error("the polygon itself was not kept")
	}

	var geom map[string]any
	if err := json.Unmarshal(e.Geometry, &geom); err != nil {
		t.Fatalf("stored geometry is not JSON: %v", err)
	}
	if geom["type"] == "Point" {
		t.Error("the stored geometry was flattened to a point")
	}
}

func TestPointGeometryStoresNoRedundantShape(t *testing.T) {
	t.Parallel()

	fc, _ := geojson.Parse(fixture(t, "patios.json"))
	e := fc.Entities(opts("patio"))[0]

	// Position already carries it; repeating it as geometry is noise.
	if len(e.Geometry) != 0 {
		t.Errorf("a point feature stored a redundant geometry: %s", e.Geometry)
	}
}

func TestEntitiesSkipsUnusableFeatures(t *testing.T) {
	t.Parallel()

	fc, err := geojson.Parse([]byte(`{"type":"FeatureCollection","features":[
		{"type":"Feature","id":"a","geometry":{"type":"Point","coordinates":[0,0]},"properties":{"name":"null island"}},
		{"type":"Feature","id":"b","geometry":null,"properties":{"name":"no geometry"}},
		{"type":"Feature","id":"c","geometry":{"type":"Point","coordinates":[-999,999]},"properties":{"name":"off world"}},
		{"type":"Feature","id":"d","geometry":{"type":"Point","coordinates":[-4.7794,37.8882]},"properties":{"name":"Tendillas"}}
	]}`))
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}

	entities := fc.Entities(opts("landmark"))
	if len(entities) != 1 {
		t.Fatalf("entities = %d, want only the usable one", len(entities))
	}
	if entities[0].Title != "Tendillas" {
		t.Errorf("kept the wrong feature: %q", entities[0].Title)
	}
}

func TestTitleFallsBackWhenNothingIsNamed(t *testing.T) {
	t.Parallel()

	got := geojson.Title(map[string]any{"cod": 17, "tip": "x"}, "green_space 3")
	if got != "green_space 3" {
		t.Errorf("Title() = %q, want the fallback", got)
	}
}

func TestParseRejectsNonJSON(t *testing.T) {
	t.Parallel()

	if _, err := geojson.Parse([]byte(`<ExceptionReport/>`)); err == nil {
		t.Fatal("Parse() on XML = nil error")
	}
}

// Municipal exports store a rendered popup where a name belongs. Córdoba's bus
// layer holds "ACERA DE GUERRITA<br><br>DIRECCIÓN: CENTRO CIUDAD", tabs and
// all, and a label with markup in it is not a label.
func TestTitleStripsEmbeddedMarkup(t *testing.T) {
	t.Parallel()

	cases := []struct{ name, in, want string }{
		{
			name: "br tags and tabs",
			in:   "ACERA  DE GUERRITA\t\t\t   <br><br>DIRECCIÓN: CENTRO CIUDAD\t\t",
			want: "ACERA DE GUERRITA DIRECCIÓN: CENTRO CIUDAD",
		},
		{
			name: "html entities",
			in:   "Pla&ccedil;a &amp; Mercado",
			want: "Plaça & Mercado",
		},
		{
			name: "already clean",
			in:   "Campus Universitario de Rabanales",
			want: "Campus Universitario de Rabanales",
		},
		{
			name: "markup only falls through to the fallback",
			in:   "<br><br>",
			want: "fallback",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := geojson.Title(map[string]any{"name": tc.in}, "fallback"); got != tc.want {
				t.Errorf("Title() = %q, want %q", got, tc.want)
			}
		})
	}
}

// A layer mixing stop points with route geometry must yield both, not silently
// drop half of what the municipality published.
func TestLineStringsBecomeEntitiesToo(t *testing.T) {
	t.Parallel()

	fc, err := geojson.Parse([]byte(`{"type":"FeatureCollection","features":[
		{"type":"Feature","id":"stop","geometry":{"type":"Point","coordinates":[-4.7794,37.8882]},
		 "properties":{"name":"ACERA DE GUERRITA<br>DIRECCIÓN: CENTRO"}},
		{"type":"Feature","id":"route","geometry":{"type":"LineString","coordinates":[[-4.78,37.88],[-4.77,37.89]]},
		 "properties":{"name":"Línea 3"}}
	]}`))
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}

	entities := fc.Entities(opts("bus"))
	if len(entities) != 2 {
		t.Fatalf("entities = %d, want both the stop and the route", len(entities))
	}

	byTitle := map[string]bool{}
	for _, e := range entities {
		byTitle[e.Title] = len(e.Geometry) > 0
	}
	if hasGeom, ok := byTitle["ACERA DE GUERRITA DIRECCIÓN: CENTRO"]; !ok || hasGeom {
		t.Error("the stop is missing, or stored a redundant geometry")
	}
	if hasGeom, ok := byTitle["Línea 3"]; !ok || !hasGeom {
		t.Error("the route is missing, or its shape was dropped")
	}
}
