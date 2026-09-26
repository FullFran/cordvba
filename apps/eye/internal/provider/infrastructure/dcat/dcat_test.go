package dcat_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/FullFran/eye/internal/httpx"
	observation "github.com/FullFran/eye/internal/observation/domain"
	"github.com/FullFran/eye/internal/provider/infrastructure/dcat"
	source "github.com/FullFran/eye/internal/source/domain"
)

// icaHeader is the distribution's column row, as published.
const icaHeader = "cod_estacion,nombre,tipo,latitud,longitud,activa,fecha,indice,debido_a\n"

// fixture reads a recorded response. Tests never reach the network.
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile("../../../../testdata/miteco/" + name) // #nosec G304 -- fixture path built from a test literal
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return body
}

func serve(t *testing.T, status int, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func src(url string, opts map[string]string) source.Source {
	return source.Source{
		ID: "miteco-ica", Authority: "MITECO", Topic: "air_quality", URL: url,
		License: "CC-BY-4.0", Format: "csv-dcat",
		Access: source.AccessDocumentedAPI, Automation: source.AutomationEnabled,
		Interval: 20 * time.Minute, PublishedEvery: time.Hour, Options: opts,
	}
}

// readings polls the recorded national table.
func readings(t *testing.T) []observation.Record {
	t.Helper()

	srv := serve(t, http.StatusOK, fixture(t, "ica-ultima-hora.csv"))
	records, err := dcat.NewAirQuality(src(srv.URL, nil), httpx.New()).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) != 624 {
		t.Fatalf("Poll() = %d records, want one per station row", len(records))
	}
	return records
}

// Every row of the published hour has to survive normalization, including the
// 63 stations that reported no index at all.
func TestPollNormalizesEveryRow(t *testing.T) {
	t.Parallel()

	for _, r := range readings(t) {
		if err := r.Validate(); err != nil {
			t.Fatalf("record %s invalid: %v", r.ID, err)
		}
		if r.Kind != "air_quality_index" {
			t.Fatalf("kind = %q", r.Kind)
		}
		if r.ObservedAt.Equal(r.FetchedAt) {
			t.Fatal("ObservedAt and FetchedAt were merged")
		}
		if r.Provenance.RawHash == "" || r.Provenance.Publisher != "MITECO" ||
			r.Provenance.License == "" || r.Provenance.SourceURL == "" {
			t.Fatalf("incomplete provenance: %+v", r.Provenance)
		}
	}
}

func TestPoll(t *testing.T) {
	t.Parallel()

	first := readings(t)[0]

	if first.LocalKey != "1022001" {
		t.Errorf("LocalKey = %q, want the national station code", first.LocalKey)
	}
	if !strings.Contains(first.Title, "EL CIEGO") {
		t.Errorf("title = %q, want the station name", first.Title)
	}
	want := time.Date(2026, time.September, 2, 21, 0, 0, 0, time.UTC)
	if !first.ObservedAt.Equal(want) {
		t.Errorf("ObservedAt = %s, want %s", first.ObservedAt, want)
	}
	if first.Position == nil || !first.Position.Valid() {
		t.Error("a station reading with no position is not placeable")
	}
	if first.ExpiresAt == nil {
		t.Error("an hourly index reading must carry a retention deadline")
	}

	var payload map[string]any
	if err := json.Unmarshal(first.Payload, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	for _, key := range []string{"index", "category", "due_to", "station_type", "active"} {
		if _, ok := payload[key]; !ok {
			t.Errorf("payload missing %q: %v", key, payload)
		}
	}
}

// The index is a six-step ladder the publisher defines. Flattening it would
// throw away the only thing the distribution actually says.
func TestSeverityFollowsTheIndex(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		index string
		want  observation.Severity
	}{
		{name: "no data received", index: "0", want: observation.SeverityNone},
		{name: "buena", index: "1", want: observation.SeverityInfo},
		{name: "razonablemente buena", index: "2", want: observation.SeverityInfo},
		{name: "regular", index: "3", want: observation.SeverityLow},
		{name: "desfavorable", index: "4", want: observation.SeverityModerate},
		{name: "muy desfavorable", index: "5", want: observation.SeverityHigh},
		{name: "extremadamente desfavorable", index: "6", want: observation.SeverityCritical},
		{name: "partial desfavorable keeps its rank", index: "40", want: observation.SeverityModerate},
		{name: "partial extremadamente keeps its rank", index: "60", want: observation.SeverityCritical},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			body := icaHeader + "1401001,CORDOBA ASOMADILLA,FONDO,37.89,-4.78,true,2026-09-02T21:00:00," + tc.index + ",O3\n"
			srv := serve(t, http.StatusOK, []byte(body))

			records, err := dcat.NewAirQuality(src(srv.URL, nil), httpx.New()).Poll(context.Background())
			if err != nil {
				t.Fatalf("Poll() = %v", err)
			}
			if len(records) != 1 {
				t.Fatalf("Poll() = %d records, want 1", len(records))
			}
			if records[0].Severity != tc.want {
				t.Errorf("severity = %d, want %d", records[0].Severity, tc.want)
			}
		})
	}
}

// The publisher states that an index computed from fewer pollutants is reported
// as the same category multiplied by ten. That caveat must reach the UI rather
// than dissolve into an identical-looking number.
func TestPartialIndexIsNotOfficial(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		row         string
		wantQuality observation.Quality
		wantPartial bool
		wantActive  bool
	}{
		{
			name:        "a full reading is the authority's own index",
			row:         "1401001,CORDOBA ASOMADILLA,FONDO,37.89,-4.78,true,2026-09-02T21:00:00,2,O3",
			wantQuality: observation.QualityOfficial,
			wantActive:  true,
		},
		{
			name:        "fewer pollutants is preliminary",
			row:         "1401001,CORDOBA ASOMADILLA,FONDO,37.89,-4.78,true,2026-09-02T21:00:00,20,O3",
			wantQuality: observation.QualityPreliminary,
			wantPartial: true,
			wantActive:  true,
		},
		{
			name:        "a station out of service is preliminary whatever it emits",
			row:         "1401001,CORDOBA ASOMADILLA,FONDO,37.89,-4.78,false,2026-09-02T21:00:00,2,O3",
			wantQuality: observation.QualityPreliminary,
			wantActive:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := serve(t, http.StatusOK, []byte(icaHeader+tc.row+"\n"))
			records, err := dcat.NewAirQuality(src(srv.URL, nil), httpx.New()).Poll(context.Background())
			if err != nil {
				t.Fatalf("Poll() = %v", err)
			}
			if len(records) != 1 {
				t.Fatalf("Poll() = %d records, want 1", len(records))
			}
			r := records[0]
			if r.Quality != tc.wantQuality {
				t.Errorf("quality = %q, want %q", r.Quality, tc.wantQuality)
			}

			var payload map[string]any
			if err := json.Unmarshal(r.Payload, &payload); err != nil {
				t.Fatalf("payload: %v", err)
			}
			if payload["partial"] != tc.wantPartial {
				t.Errorf("payload partial = %v, want %v", payload["partial"], tc.wantPartial)
			}
			if payload["active"] != tc.wantActive {
				t.Errorf("payload active = %v, want %v", payload["active"], tc.wantActive)
			}
		})
	}
}

// A national table narrowed to a viewport is still the national table; the
// filter is eye's, and the rows it keeps are unchanged.
func TestBoundingBoxFilter(t *testing.T) {
	t.Parallel()

	srv := serve(t, http.StatusOK, fixture(t, "ica-ultima-hora.csv"))
	p := dcat.NewAirQuality(src(srv.URL, map[string]string{"bbox": "-5.7,37.2,-4.0,38.8"}), httpx.New())

	records, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) == 0 {
		t.Fatal("the Cordoba viewport matched no station")
	}
	if len(records) > 40 {
		t.Fatalf("the viewport kept %d stations, which is the whole country", len(records))
	}
	for _, r := range records {
		if r.Position.Lon < -5.7 || r.Position.Lon > -4.0 || r.Position.Lat < 37.2 || r.Position.Lat > 38.8 {
			t.Errorf("%s is outside the viewport: %+v", r.Title, r.Position)
		}
	}
}

// The stations are an inventory in their own right, published with every poll.
func TestEntities(t *testing.T) {
	t.Parallel()

	srv := serve(t, http.StatusOK, fixture(t, "ica-ultima-hora.csv"))
	entities, err := dcat.NewAirQuality(src(srv.URL, map[string]string{"bbox": "-5.7,37.2,-4.0,38.8"}), httpx.New()).
		Entities(context.Background())
	if err != nil {
		t.Fatalf("Entities() = %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("Entities() returned nothing")
	}
	for _, e := range entities {
		if err := e.Validate(); err != nil {
			t.Fatalf("entity %s invalid: %v", e.ID, err)
		}
		if e.Kind != "air_quality_station" {
			t.Fatalf("kind = %q", e.Kind)
		}
		if e.Position == nil {
			t.Fatalf("station %s has no position", e.Title)
		}
	}
}

func TestErrorEmptyAndMalformed(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		status  int
		body    []byte
		wantErr bool
		want    int
	}{
		{name: "header only", status: http.StatusOK, body: []byte(icaHeader)},
		{name: "server error", status: http.StatusBadGateway, body: []byte("upstream failed"), wantErr: true},
		{name: "empty body", status: http.StatusOK, body: nil, wantErr: true},
		{
			name: "an html page instead of a table", status: http.StatusOK,
			body: []byte("<html><body>Portal en mantenimiento</body></html>"), wantErr: true,
		},
		{
			name:   "a row with an unreadable coordinate is skipped",
			status: http.StatusOK,
			body: []byte(icaHeader +
				"1401001,SIN COORDENADAS,FONDO,,,true,2026-09-02T21:00:00,2,O3\n" +
				"1401002,CORDOBA LEPANTO,TRAFICO,37.89,-4.77,true,2026-09-02T21:00:00,2,NO2\n"),
			want: 1,
		},
		{
			name:   "a row with an unreadable timestamp is skipped",
			status: http.StatusOK,
			body: []byte(icaHeader +
				"1401001,SIN FECHA,FONDO,37.89,-4.78,true,,2,O3\n"),
			want: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := serve(t, tc.status, tc.body)
			got, err := dcat.NewAirQuality(src(srv.URL, nil), httpx.New()).Poll(context.Background())

			if tc.wantErr {
				if err == nil {
					t.Fatal("a malformed response was accepted")
				}
				if !errors.Is(err, dcat.ErrDCAT) {
					t.Errorf("error = %v, want it wrapped in ErrDCAT", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Poll() = %v", err)
			}
			if len(got) != tc.want {
				t.Errorf("Poll() = %d records, want %d", len(got), tc.want)
			}
		})
	}
}

// The registry points at the catalogue; the table lives at a distribution URL
// the catalogue names. Whichever the operator configures, the adapter must read
// a table and say so plainly when it is handed something else.
func TestDistributionOptionOverridesTheRegistryURL(t *testing.T) {
	t.Parallel()

	table := serve(t, http.StatusOK, []byte(icaHeader+
		"1401001,CORDOBA ASOMADILLA,FONDO,37.89,-4.78,true,2026-09-02T21:00:00,2,O3\n"))

	s := src("https://catalogo.datosabiertos.miteco.gob.es/", map[string]string{"distribution": table.URL})
	records, err := dcat.NewAirQuality(s, httpx.New()).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("Poll() = %d records, want 1", len(records))
	}
	if records[0].Provenance.SourceURL != table.URL {
		t.Errorf("SourceURL = %q, want the distribution actually read", records[0].Provenance.SourceURL)
	}
}

// 63 of the 624 stations in the recorded hour published an empty index cell.
// The publisher documents a zero for "no data received" and nothing at all for
// the blank, so eye keeps both rather than quietly shrinking the country.
func TestStationsThatPublishedNoIndexAreKept(t *testing.T) {
	t.Parallel()

	srv := serve(t, http.StatusOK, []byte(icaHeader+
		"1401001,CORDOBA ASOMADILLA,FONDO,37.89,-4.78,true,2026-09-02T21:00:00,,\n"+
		"1401002,CORDOBA LEPANTO,TRAFICO,37.89,-4.77,true,2026-09-02T21:00:00,0,\n"))

	records, err := dcat.NewAirQuality(src(srv.URL, nil), httpx.New()).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("Poll() = %d records, want both stations", len(records))
	}

	for i, wantReported := range []bool{false, true} {
		var payload map[string]any
		if err := json.Unmarshal(records[i].Payload, &payload); err != nil {
			t.Fatalf("payload: %v", err)
		}
		if payload["index_reported"] != wantReported {
			t.Errorf("station %d index_reported = %v, want %v",
				i, payload["index_reported"], wantReported)
		}
		if records[i].Severity != observation.SeverityNone {
			t.Errorf("station %d severity = %d, want None", i, records[i].Severity)
		}
		if records[i].Quality != observation.QualityPreliminary {
			t.Errorf("station %d quality = %q, want preliminary", i, records[i].Quality)
		}
		if !strings.Contains(records[i].Title, "sin datos") {
			t.Errorf("station %d title = %q, want it to say there is no reading", i, records[i].Title)
		}
	}
}

// A non-numeric index is a format change, not a quiet station, and must not be
// read as one.
func TestNonNumericIndexIsDropped(t *testing.T) {
	t.Parallel()

	srv := serve(t, http.StatusOK, []byte(icaHeader+
		"1401001,CORDOBA ASOMADILLA,FONDO,37.89,-4.78,true,2026-09-02T21:00:00,buena,O3\n"))

	records, err := dcat.NewAirQuality(src(srv.URL, nil), httpx.New()).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) != 0 {
		t.Errorf("Poll() = %d records, want the unreadable row dropped", len(records))
	}
}
