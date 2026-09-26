package datex_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/FullFran/cordvba/apps/eye/internal/httpx"
	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	"github.com/FullFran/cordvba/apps/eye/internal/provider/infrastructure/datex"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// serveSituations returns the recorded SituationPublication: 20 situations and
// 22 situation records cut verbatim out of the national feed.
func serveSituations(t *testing.T) *httptest.Server {
	t.Helper()

	body, err := os.ReadFile("../../../../testdata/datex/situations.xml") // #nosec G304 -- fixture path is a literal
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"cached"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		w.Header().Set("ETag", `"cached"`)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// situationSource builds the registry entry for dgt-incidents.
func situationSource(url string) source.Source {
	return source.Source{
		ID: "dgt-incidents", Authority: "Direccion General de Trafico", Topic: "transport",
		URL: url, Format: "datex2-situations", License: "CC-BY-4.0",
		Access: source.AccessDocumentedAPI, Automation: source.AutomationEnabled, Interval: 5 * time.Minute,
	}
}

// pollSituations fetches the fixture and indexes the result by LocalKey, which
// is the publisher's own situationRecord id.
func pollSituations(t *testing.T) map[string]observation.Record {
	t.Helper()

	srv := serveSituations(t)
	records, err := datex.NewSituations(situationSource(srv.URL), httpx.New()).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}

	byKey := make(map[string]observation.Record, len(records))
	for _, r := range records {
		if _, dup := byKey[r.LocalKey]; dup {
			t.Errorf("duplicate local key %q", r.LocalKey)
		}
		byKey[r.LocalKey] = r
	}
	if len(byKey) != len(records) {
		t.Fatalf("indexed %d of %d records", len(byKey), len(records))
	}
	return byKey
}

// The fixture holds 22 situation records. One of them is a
// GeneralInstructionOrMessageToRoadUsers, which eye deliberately does not
// carry, so 21 records come out.
func TestSituationsPollEmitsOneRecordPerCarriedSituationRecord(t *testing.T) {
	t.Parallel()

	srv := serveSituations(t)
	records, err := datex.NewSituations(situationSource(srv.URL), httpx.New()).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) != 21 {
		t.Fatalf("records = %d, want 21", len(records))
	}
	for _, r := range records {
		if err := r.Validate(); err != nil {
			t.Errorf("record %s does not validate: %v", r.ID, err)
		}
	}
}

// A record type eye has no honest shape for is dropped whole rather than
// squeezed into the wrong kind.
func TestSituationsPollSkipsRecordTypesItCannotCarry(t *testing.T) {
	t.Parallel()

	byKey := pollSituations(t)
	if _, found := byKey["21706463"]; found {
		t.Error("the GeneralInstructionOrMessageToRoadUsers record was mapped instead of skipped")
	}
}

// situationCase is one expected mapping, checked against the recorded feed.
//
// The three tests below walk this same table instead of asserting everything in
// one pass. Splitting them costs a few lines and buys a failure that names the
// property that broke — the kind, the position or the payload — rather than a
// line number inside a wall of assertions.
type situationCase struct {
	name     string
	localKey string
	kind     string
	severity observation.Severity
	lat, lon float64
	road     string
	province string
	// linear marks a record whose location is a segment rather than a
	// point, and which must therefore carry a geometry.
	linear bool
}

// situationCases covers every kind the adapter emits, both location shapes, and
// both severity values the publisher actually uses.
func situationCases() []situationCase {
	return []situationCase{
		{
			name: "road closed by rockfall in Cordoba", localKey: "20711965",
			kind: "road_closure", severity: observation.SeverityCritical,
			lat: 37.35811, lon: -4.19934, road: "A-4154", province: "Córdoba", linear: true,
		},
		{
			name: "carriageway closed for roadworks in Cordoba", localKey: "22293349",
			kind: "road_closure", severity: observation.SeverityInfo,
			lat: 37.43573, lon: -4.0973, road: "CO-8204", province: "Córdoba", linear: true,
		},
		{
			name: "narrow lanes for roadworks", localKey: "18811074",
			kind: "roadworks", severity: observation.SeverityInfo,
			lat: 39.993732, lon: -3.6074991, road: "N-400", province: "Toledo", linear: true,
		},
		{
			name: "accident reported by 112", localKey: "27433286",
			kind: "road_incident", severity: observation.SeverityInfo,
			lat: 39.136375, lon: -0.48124817, road: "CV-550", province: "València/Valencia",
		},
		{
			name: "poor environment conditions", localKey: "27470296",
			kind: "weather_hazard", severity: observation.SeverityModerate,
			lat: 43.454296, lon: -7.3220057, road: "A-8", province: "Lugo", linear: true,
		},
		{
			name: "flooding is a weather hazard", localKey: "26710501",
			kind: "weather_hazard", severity: observation.SeverityInfo,
			lat: 37.458626, lon: -3.6391692, road: "N-323a", province: "Granada",
		},
		{
			name: "slow traffic is an incident", localKey: "21204838",
			kind: "road_incident", severity: observation.SeverityModerate,
			lat: 43.105568, lon: -6.259333, road: "AS-227", province: "Asturias", linear: true,
		},
		{
			name: "lane management with no roadworks cause", localKey: "27289502",
			kind: "road_incident", severity: observation.SeverityInfo,
			lat: 37.262863, lon: -5.5284524, road: "A-92", province: "Sevilla",
		},
	}
}

// mustRecord fetches one record by the publisher's own identifier.
func mustRecord(t *testing.T, byKey map[string]observation.Record, localKey string) observation.Record {
	t.Helper()

	rec, ok := byKey[localKey]
	if !ok {
		t.Fatalf("no record with local key %q", localKey)
	}
	return rec
}

func TestSituationsPollMapsKindAndSeverity(t *testing.T) {
	t.Parallel()

	byKey := pollSituations(t)
	for _, tc := range situationCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := mustRecord(t, byKey, tc.localKey)
			if rec.Kind != tc.kind {
				t.Errorf("kind = %q, want %q", rec.Kind, tc.kind)
			}
			if rec.Severity != tc.severity {
				t.Errorf("severity = %d, want %d", rec.Severity, tc.severity)
			}
			// DGT declaring the state of its own network is as official
			// as a road condition gets.
			if rec.Quality != observation.QualityOfficial {
				t.Errorf("quality = %q, want official", rec.Quality)
			}
		})
	}
}

// Every record sits on a point the publisher stated, and a segment keeps the
// shape a single point cannot express.
func TestSituationsPollPlacesRecordsWhereThePublisherSaid(t *testing.T) {
	t.Parallel()

	byKey := pollSituations(t)
	for _, tc := range situationCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := mustRecord(t, byKey, tc.localKey)
			if rec.Position == nil {
				t.Fatal("record has no position")
			}
			if rec.Position.Lat != tc.lat || rec.Position.Lon != tc.lon {
				t.Errorf("position = %v, want {%v %v}", *rec.Position, tc.lat, tc.lon)
			}
			if tc.linear && len(rec.Geometry) == 0 {
				t.Error("a linear extent lost its geometry")
			}
			if !tc.linear && len(rec.Geometry) != 0 {
				t.Errorf("a point location gained a geometry: %s", rec.Geometry)
			}
		})
	}
}

// The wire format stops at the adapter, but the road and the province have to
// survive it: they are what a Córdoba query filters on.
func TestSituationsPollPayloadNamesTheRoadAndTheRawType(t *testing.T) {
	t.Parallel()

	byKey := pollSituations(t)
	for _, tc := range situationCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rec := mustRecord(t, byKey, tc.localKey)

			var payload map[string]any
			if err := json.Unmarshal(rec.Payload, &payload); err != nil {
				t.Fatalf("payload is not JSON: %v", err)
			}
			if payload["road_name"] != tc.road {
				t.Errorf("payload road_name = %v, want %q", payload["road_name"], tc.road)
			}
			if payload["province"] != tc.province {
				t.Errorf("payload province = %v, want %q", payload["province"], tc.province)
			}
			if payload["datex_type"] == nil || payload["datex_type"] == "" {
				t.Error("payload does not carry the raw DATEX type")
			}
		})
	}
}

// A linear extent keeps its shape as a GeoJSON LineString, and the record is
// placed at the segment's stated start rather than at an invented midpoint.
func TestSituationsPollEmitsALineStringForALinearExtent(t *testing.T) {
	t.Parallel()

	rec, ok := pollSituations(t)["20276492"]
	if !ok {
		t.Fatal("record 20276492 is missing")
	}

	var geometry struct {
		Type        string       `json:"type"`
		Coordinates [][2]float64 `json:"coordinates"`
	}
	if err := json.Unmarshal(rec.Geometry, &geometry); err != nil {
		t.Fatalf("geometry is not GeoJSON: %v", err)
	}
	if geometry.Type != "LineString" {
		t.Errorf("geometry type = %q, want LineString", geometry.Type)
	}
	if len(geometry.Coordinates) != 2 {
		t.Fatalf("coordinates = %d, want 2", len(geometry.Coordinates))
	}
	// RFC 7946: longitude first.
	if geometry.Coordinates[0] != [2]float64{-4.5017304, 37.41493} {
		t.Errorf("start = %v", geometry.Coordinates[0])
	}
	if geometry.Coordinates[1] != [2]float64{-4.3549824, 37.31576} {
		t.Errorf("end = %v", geometry.Coordinates[1])
	}
	if rec.Position.Lat != geometry.Coordinates[0][1] || rec.Position.Lon != geometry.Coordinates[0][0] {
		t.Errorf("position %v is not the stated start of the segment", *rec.Position)
	}
}

// The source's own timestamp and eye's fetch time are the two halves of the
// latency figure; merging them would sell stale data as live.
func TestSituationsPollKeepsObservedAndFetchedApart(t *testing.T) {
	t.Parallel()

	rec, ok := pollSituations(t)["20711965"]
	if !ok {
		t.Fatal("record 20711965 is missing")
	}
	if rec.ObservedAt.IsZero() || rec.FetchedAt.IsZero() {
		t.Fatalf("observed=%v fetched=%v", rec.ObservedAt, rec.FetchedAt)
	}
	if rec.ObservedAt.Equal(rec.FetchedAt) {
		t.Error("observed_at and fetched_at were merged")
	}
	// situationRecordVersionTime, in UTC.
	want := time.Date(2026, time.February, 19, 12, 19, 14, 0, time.UTC)
	if !rec.ObservedAt.Equal(want) {
		t.Errorf("observed_at = %s, want %s", rec.ObservedAt, want)
	}
	if rec.Latency() <= 0 {
		t.Errorf("latency = %s, want a positive figure", rec.Latency())
	}
}

func TestSituationsPollReadsTheValidityWindow(t *testing.T) {
	t.Parallel()

	byKey := pollSituations(t)

	// A roadworks closure that states both ends of its window.
	bounded, ok := byKey["22293349"]
	if !ok {
		t.Fatal("record 22293349 is missing")
	}
	if bounded.ValidFrom == nil || bounded.ValidUntil == nil {
		t.Fatalf("valid_from=%v valid_until=%v", bounded.ValidFrom, bounded.ValidUntil)
	}
	wantFrom := time.Date(2026, time.May, 27, 5, 34, 0, 0, time.UTC)
	wantUntil := time.Date(2026, time.December, 31, 22, 59, 0, 0, time.UTC)
	if !bounded.ValidFrom.Equal(wantFrom) {
		t.Errorf("valid_from = %s, want %s", bounded.ValidFrom, wantFrom)
	}
	if !bounded.ValidUntil.Equal(wantUntil) {
		t.Errorf("valid_until = %s, want %s", bounded.ValidUntil, wantUntil)
	}

	// An open-ended closure states a start and no end. Inventing one would
	// make eye claim the road reopens at a time nobody announced.
	open, ok := byKey["20711965"]
	if !ok {
		t.Fatal("record 20711965 is missing")
	}
	if open.ValidFrom == nil {
		t.Error("valid_from is missing")
	}
	if open.ValidUntil != nil {
		t.Errorf("valid_until = %s, want nil for an open-ended closure", open.ValidUntil)
	}
}

// The publisher's own free text beats anything eye composes.
func TestSituationsPollPrefersThePublishersOwnText(t *testing.T) {
	t.Parallel()

	byKey := pollSituations(t)

	written, ok := byKey["22986875"]
	if !ok {
		t.Fatal("record 22986875 is missing")
	}
	if written.Description != "LUNADA" {
		t.Errorf("description = %q, want the publisher's locationDescription", written.Description)
	}

	composed, ok := byKey["18811074"]
	if !ok {
		t.Fatal("record 18811074 is missing")
	}
	if !strings.Contains(composed.Title, "N-400") {
		t.Errorf("title %q does not name the road", composed.Title)
	}
	if !strings.Contains(composed.Description, "N-400") {
		t.Errorf("description %q does not name the road", composed.Description)
	}
	if !strings.Contains(strings.ToLower(composed.Description), "narrow lanes") {
		t.Errorf("description %q does not name the situation type", composed.Description)
	}
}

func TestSituationsPollCarriesCompleteProvenance(t *testing.T) {
	t.Parallel()

	srv := serveSituations(t)
	src := situationSource(srv.URL)
	records, err := datex.NewSituations(src, httpx.New()).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}

	for _, rec := range records {
		prov := rec.Provenance
		if err := prov.Validate(); err != nil {
			t.Fatalf("provenance of %s: %v", rec.ID, err)
		}
		if prov.Publisher != src.Authority || prov.SourceURL != src.URL || prov.License != src.License {
			t.Fatalf("provenance = %+v", prov)
		}
		if prov.FetchedAt.IsZero() {
			t.Fatal("provenance has no fetched_at")
		}
		if len(prov.RawHash) != 64 {
			t.Fatalf("raw_hash = %q, want a hex sha256", prov.RawHash)
		}
	}
}

// A body eye cannot parse is a failure, never a quiet empty poll: an empty
// success would report "no incidents in Spain" and mean "the feed broke".
func TestSituationsPollFailsOnAMalformedBody(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
	}{
		{name: "not xml at all", body: `{"situations": []}`},
		{name: "empty body", body: ``},
		{
			name: "truncated mid record",
			body: `<?xml version="1.0"?><d2:payload xmlns:d2="d" xmlns:sit="s"><sit:situation id="1">` +
				`<sit:situationRecord id="2"><sit:situationRecordVersionTime>2026-09-03T00:00:00Z`,
		},
		{
			name: "a different publication",
			body: `<?xml version="1.0"?><html><body>403 Forbidden</body></html>`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/xml; charset=utf-8")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			records, err := datex.NewSituations(situationSource(srv.URL), httpx.New()).Poll(context.Background())
			if err == nil {
				t.Fatalf("Poll() = %d records, nil error; want an error", len(records))
			}
			if len(records) != 0 {
				t.Errorf("Poll() returned %d records alongside the error", len(records))
			}
		})
	}
}

func TestSituationsPollReportsAnUnreachableSource(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	if _, err := datex.NewSituations(situationSource(srv.URL), httpx.New()).Poll(context.Background()); err == nil {
		t.Fatal("Poll() against a 403 = nil error")
	}
}

func TestSituationsPollHandlesNotModified(t *testing.T) {
	t.Parallel()

	srv := serveSituations(t)
	p := datex.NewSituations(situationSource(srv.URL), httpx.New())

	if _, err := p.Poll(context.Background()); err != nil {
		t.Fatalf("first Poll() = %v", err)
	}
	got, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("second Poll() = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("second Poll() returned %d records, want 0", len(got))
	}
}

// DGT extends its profile from time to time; a schema addition must not take
// the provider down.
func TestSituationsPollIgnoresUnknownElements(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<d2:payload xsi:type="sit:SituationPublication" xmlns:d2="d" xmlns:sit="s" xmlns:com="c" xmlns:loc="l" xmlns:lse="e" xmlns:xsi="x">
  <com:publicationTime>2026-09-03T00:52:41.560+02:00</com:publicationTime>
  <sit:somethingBrandNew><sit:nested>ignore me</sit:nested></sit:somethingBrandNew>
  <sit:situation id="1">
    <sit:situationRecord xsi:type="sit:GenericSituationRecord" id="9" version="1">
      <sit:situationRecordVersionTime>2026-09-02T06:32:54.000+02:00</sit:situationRecordVersionTime>
      <sit:futureField>whatever</sit:futureField>
      <sit:cause><sit:causeType>accident</sit:causeType>
        <sit:detailedCauseType><sit:accidentType>accident</sit:accidentType></sit:detailedCauseType></sit:cause>
      <sit:locationReference xsi:type="loc:PointLocation">
        <loc:supplementaryPositionalDescription><loc:roadInformation><loc:roadName>N-432</loc:roadName>
        </loc:roadInformation></loc:supplementaryPositionalDescription>
        <loc:tpegPointLocation><loc:point><loc:pointCoordinates>
          <loc:latitude>37.89</loc:latitude><loc:longitude>-4.74</loc:longitude>
        </loc:pointCoordinates></loc:point></loc:tpegPointLocation>
      </sit:locationReference>
    </sit:situationRecord>
  </sit:situation>
</d2:payload>`))
	}))
	defer srv.Close()

	records, err := datex.NewSituations(situationSource(srv.URL), httpx.New()).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if records[0].Kind != "road_incident" {
		t.Errorf("kind = %q", records[0].Kind)
	}
}

// eye ingests the whole national feed and filters at query time, so the
// province has to survive normalization for a Córdoba filter to be possible.
func TestSituationsPollKeepsEveryProvinceAndMakesItFilterable(t *testing.T) {
	t.Parallel()

	cordoba := 0
	for _, rec := range pollSituations(t) {
		var payload struct {
			Province string `json:"province"`
		}
		if err := json.Unmarshal(rec.Payload, &payload); err != nil {
			t.Fatalf("payload is not JSON: %v", err)
		}
		if payload.Province == "Córdoba" {
			cordoba++
		}
	}
	if cordoba != 4 {
		t.Errorf("Córdoba records = %d, want 4", cordoba)
	}
}

func TestSituationsInfoReturnsTheRegistryEntry(t *testing.T) {
	t.Parallel()

	s := situationSource("https://example.org/situations.xml")
	if got := datex.NewSituations(s, httpx.New()).Info(); got.ID != s.ID {
		t.Errorf("Info().ID = %q", got.ID)
	}
}
