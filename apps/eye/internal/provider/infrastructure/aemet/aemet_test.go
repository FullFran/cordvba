package aemet_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FullFran/cordvba/apps/eye/internal/httpx"
	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	"github.com/FullFran/cordvba/apps/eye/internal/provider/infrastructure/aemet"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// fixture reads a recorded response. Tests never reach the network.
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile("../../../../testdata/aemet/" + name) // #nosec G304 -- fixture path built from a test literal
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return body
}

// capArchive builds the gtar payload AEMET serves for avisos_cap out of the
// recorded CAP messages, which are kept as XML so a reviewer can read them.
func capArchive(t *testing.T, names ...string) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, name := range names {
		body := fixture(t, filepath.Join("cap", name))
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatalf("tar header: %v", err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatalf("tar write: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

// twoStep serves an AEMET product: an envelope pointing at a datos URL on the
// same server, and the payload behind it.
func twoStep(t *testing.T, envelope []byte, data []byte, dataType string) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("/opendata/sh/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", dataType)
		_, _ = w.Write(data)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("api_key") == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write(fixture(t, "unauthorized.json"))
			return
		}
		// The recorded envelope points at opendata.aemet.es; the test
		// server has to answer for itself.
		body := strings.ReplaceAll(string(envelope), "https://opendata.aemet.es", srv.URL)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
	return srv
}

func src(t *testing.T, id, base string, opts map[string]string) source.Source {
	t.Helper()
	return source.Source{
		ID: id, Authority: "AEMET", Topic: "weather", URL: base + "/opendata",
		License: "reuse-with-attribution", Format: "rest-json-two-step",
		Access: source.AccessDocumentedAPI, Automation: source.AutomationEnabled,
		Interval: 5 * time.Minute, Options: opts,
	}
}

// A source with no credential must say so by name. A poll that quietly returns
// nothing looks exactly like a quiet city.
func TestMissingAPIKeyIsNamed(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		poll func(context.Context) ([]observation.Record, error)
	}{
		{
			name: "warnings",
			poll: aemet.NewWarnings(src(t, "aemet-warnings", "https://opendata.aemet.es", nil), httpx.New(), "").Poll,
		},
		{
			name: "observation",
			poll: aemet.NewObservation(src(t, "aemet-observation", "https://opendata.aemet.es", nil), httpx.New(), "").Poll,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := tc.poll(context.Background())
			if !errors.Is(err, aemet.ErrMissingAPIKey) {
				t.Fatalf("Poll() error = %v, want ErrMissingAPIKey", err)
			}
			if got != nil {
				t.Errorf("Poll() = %d records, want none", len(got))
			}
			if !strings.Contains(err.Error(), "AEMET_API_KEY") {
				t.Errorf("error %q does not name the environment variable", err)
			}
		})
	}
}

// warningRecords polls the recorded bulletin and indexes it by zone.
func warningRecords(t *testing.T) map[string]observation.Record {
	t.Helper()

	srv := twoStep(t,
		fixture(t, "warnings_step1.json"),
		capArchive(t,
			"Z_CAP_C_LEMM_20260902230000_AFAZ611402ATTA030000.xml",
			"Z_CAP_C_LEMM_20260902230000_AFAZ61VV61TOTO040000.xml",
		),
		"application/x-gzip")

	p := aemet.NewWarnings(src(t, "aemet-warnings", srv.URL, nil), httpx.New(), "test.key.value")
	records, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}

	// One orange warning for the Campina, plus two "no warning" zones from
	// the multi-area minor message. Silence has to be reported too.
	if len(records) != 3 {
		t.Fatalf("Poll() = %d records, want 3", len(records))
	}

	byZone := map[string]observation.Record{}
	for _, r := range records {
		if err := r.Validate(); err != nil {
			t.Errorf("record %s invalid: %v", r.ID, err)
		}
		byZone[r.LocalKey] = r
	}
	return byZone
}

// requireZone returns the record for a zone and phenomenon.
func requireZone(t *testing.T, byZone map[string]observation.Record, key string) observation.Record {
	t.Helper()

	rec, ok := byZone[key]
	if !ok {
		t.Fatalf("no record keyed %q; got %v", key, keys(byZone))
	}
	return rec
}

func TestWarningsPollNormalizesAnOrangeWarning(t *testing.T) {
	t.Parallel()

	orange := requireZone(t, warningRecords(t), "611402:AT")

	if orange.Severity != observation.SeverityHigh {
		t.Errorf("naranja severity = %d, want High (%d)", orange.Severity, observation.SeverityHigh)
	}
	if orange.Quality != observation.QualityOfficial {
		t.Errorf("quality = %q, want official", orange.Quality)
	}
	if !strings.Contains(orange.Title, "Campiña cordobesa") {
		t.Errorf("title = %q, want the zone name", orange.Title)
	}
	if orange.ExpiresAt != nil {
		t.Error("an official warning must not be scheduled for deletion")
	}
	if orange.Position == nil || !orange.Position.Valid() {
		t.Error("the CAP polygon produced no usable position")
	}
	if len(orange.Geometry) == 0 {
		t.Error("the CAP polygon produced no geometry")
	}
}

func TestWarningsPollKeepsTheTwoTimestampsApart(t *testing.T) {
	t.Parallel()

	orange := requireZone(t, warningRecords(t), "611402:AT")

	if !orange.ObservedAt.Equal(time.Date(2026, time.September, 2, 23, 0, 0, 0, time.UTC)) {
		t.Errorf("ObservedAt = %s, want the CAP sent time", orange.ObservedAt)
	}
	if orange.ObservedAt.Equal(orange.FetchedAt) {
		t.Error("ObservedAt and FetchedAt were merged")
	}
	if orange.ValidFrom == nil || orange.ValidUntil == nil {
		t.Fatal("a warning must carry ValidFrom and ValidUntil")
	}
	wantFrom := time.Date(2026, time.September, 3, 11, 0, 0, 0, time.UTC)
	if !orange.ValidFrom.Equal(wantFrom) {
		t.Errorf("ValidFrom = %s, want %s (onset 13:00+02:00)", orange.ValidFrom, wantFrom)
	}
}

func TestWarningsPollRecordsProvenanceAndPayload(t *testing.T) {
	t.Parallel()

	orange := requireZone(t, warningRecords(t), "611402:AT")

	if orange.Provenance.RawHash == "" || orange.Provenance.Publisher != "AEMET" ||
		orange.Provenance.License == "" || orange.Provenance.SourceURL == "" {
		t.Errorf("incomplete provenance: %+v", orange.Provenance)
	}

	var payload map[string]any
	if err := json.Unmarshal(orange.Payload, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	for _, want := range []string{"cap_identifier", "level", "phenomenon", "zone", "parameter"} {
		if _, ok := payload[want]; !ok {
			t.Errorf("payload missing %q: %v", want, payload)
		}
	}
}

// The bulletin says which zones have no warning in force, and that is news in
// its own right: a source that only speaks up for bad news cannot distinguish
// calm from an outage.
func TestWarningsPollKeepsTheNoWarningZones(t *testing.T) {
	t.Parallel()

	byZone := warningRecords(t)
	for _, zone := range []string{"611401:TO", "611403:TO"} {
		green := requireZone(t, byZone, zone)
		if green.Severity != observation.SeverityInfo {
			t.Errorf("%s verde severity = %d, want Info", zone, green.Severity)
		}
		if !strings.Contains(green.Title, "Sin aviso") {
			t.Errorf("%s title = %q", zone, green.Title)
		}
	}
}

// The credential must never travel in the URL, where every error message and
// health row would quote it back.
func TestWarningsSendsKeyInHeader(t *testing.T) {
	t.Parallel()

	var gotHeader, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("api_key")
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write(fixture(t, "no_data.json"))
	}))
	defer srv.Close()

	p := aemet.NewWarnings(src(t, "aemet-warnings", srv.URL, nil), httpx.New(), "secret.jwt.value")
	_, _ = p.Poll(context.Background())

	if gotHeader != "secret.jwt.value" {
		t.Errorf("api_key header = %q", gotHeader)
	}
	if strings.Contains(gotQuery, "secret") {
		t.Errorf("the key leaked into the query string: %q", gotQuery)
	}
}

// AEMET answers "no warnings in force" with a 404, which is an empty poll and
// not a failure the scheduler should back off from.
func TestWarningsNoDataIsAnEmptyPoll(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write(fixture(t, "no_data.json"))
	}))
	defer srv.Close()

	got, err := aemet.NewWarnings(src(t, "aemet-warnings", srv.URL, nil), httpx.New(), "k.k.k").
		Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v, want a quiet empty poll", err)
	}
	if len(got) != 0 {
		t.Errorf("Poll() = %d records, want none", len(got))
	}
}

// A datos URL that has expired comes back with a 200 and an envelope saying so.
func TestWarningsExpiredDatosURL(t *testing.T) {
	t.Parallel()

	srv := twoStep(t, fixture(t, "warnings_step1.json"), fixture(t, "datos_expired.json"), "application/json")

	got, err := aemet.NewWarnings(src(t, "aemet-warnings", srv.URL, nil), httpx.New(), "k.k.k").
		Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v, want an empty poll", err)
	}
	if len(got) != 0 {
		t.Errorf("Poll() = %d records, want none", len(got))
	}
}

func TestWarningsRejectedKey(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write(fixture(t, "unauthorized.json"))
	}))
	defer srv.Close()

	p := aemet.NewWarnings(src(t, "aemet-warnings", srv.URL, nil), httpx.New(), "stale.jwt.value")
	_, err := p.Poll(context.Background())
	if !errors.Is(err, aemet.ErrUnauthorized) {
		t.Fatalf("Poll() = %v, want ErrUnauthorized", err)
	}
}

func TestWarningsMalformedArchive(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body []byte
	}{
		{name: "not gzip", body: []byte("<html>maintenance</html>")},
		{name: "empty body", body: nil},
		{name: "gzip of nonsense", body: gzipped(t, []byte("not a tar at all"))},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := twoStep(t, fixture(t, "warnings_step1.json"), tc.body, "application/x-gzip")
			_, err := aemet.NewWarnings(src(t, "aemet-warnings", srv.URL, nil), httpx.New(), "k.k.k").
				Poll(context.Background())
			if err == nil {
				t.Fatal("a malformed payload was accepted")
			}
			if !errors.Is(err, aemet.ErrAEMET) {
				t.Errorf("error = %v, want it wrapped in ErrAEMET", err)
			}
		})
	}
}

// observationRecords polls the recorded station payload.
func observationRecords(t *testing.T) []observation.Record {
	t.Helper()

	srv := twoStep(t, fixture(t, "observation_step1.json"),
		fixture(t, "observation_datos.json"), "application/json; charset=utf-8")

	p := aemet.NewObservation(src(t, "aemet-observation", srv.URL, map[string]string{"station": "5402"}),
		httpx.New(), "k.k.k")
	records, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("Poll() = %d records, want one per published hour", len(records))
	}
	return records
}

func TestObservationPoll(t *testing.T) {
	t.Parallel()

	r := observationRecords(t)[0]
	if err := r.Validate(); err != nil {
		t.Fatalf("record invalid: %v", err)
	}
	if r.Quality != observation.QualityPreliminary {
		t.Errorf("quality = %q, want preliminary: these are unreviewed automatic readings", r.Quality)
	}
	if r.LocalKey != "5402" {
		t.Errorf("LocalKey = %q, want the station indicative", r.LocalKey)
	}
	if r.Position == nil || r.Position.Lat < 37 || r.Position.Lat > 38 {
		t.Errorf("position = %+v, want the Cordoba station", r.Position)
	}
	if !strings.Contains(r.Title, "CORDOBA AEROPUERTO") {
		t.Errorf("title = %q, want the station name", r.Title)
	}
}

func TestObservationPollTimestampsAndRetention(t *testing.T) {
	t.Parallel()

	r := observationRecords(t)[0]

	want := time.Date(2026, time.September, 2, 21, 0, 0, 0, time.UTC)
	if !r.ObservedAt.Equal(want) {
		t.Errorf("ObservedAt = %s, want %s (fint is UTC)", r.ObservedAt, want)
	}
	if r.ObservedAt.Equal(r.FetchedAt) {
		t.Error("ObservedAt and FetchedAt were merged")
	}
	if r.ExpiresAt == nil {
		t.Fatal("an automatic sensor sample must carry a retention deadline")
	}
	if !r.ExpiresAt.After(r.ObservedAt) {
		t.Errorf("ExpiresAt %s is not after ObservedAt %s", r.ExpiresAt, r.ObservedAt)
	}
}

// The reading reaches the payload as received. Re-typing every field into the
// domain would quietly discard the ones eye does not model yet.
func TestObservationPollKeepsTheReadingAsReceived(t *testing.T) {
	t.Parallel()

	r := observationRecords(t)[0]

	if r.Provenance.RawHash == "" {
		t.Error("provenance carries no raw hash")
	}

	var payload map[string]any
	if err := json.Unmarshal(r.Payload, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	for field, want := range map[string]any{"ta": 27.4, "hr": 41.0, "pres_nmar": 1016.8, "idema": "5402"} {
		if payload[field] != want {
			t.Errorf("payload %s = %v, want %v", field, payload[field], want)
		}
	}
}

// The station's identity and position are published with every reading, so the
// inventory needs no second call against AEMET's request budget.
func TestObservationEntities(t *testing.T) {
	t.Parallel()

	srv := twoStep(t, fixture(t, "observation_step1.json"),
		fixture(t, "observation_datos.json"), "application/json; charset=utf-8")

	entities, err := aemet.NewObservation(src(t, "aemet-observation", srv.URL, nil), httpx.New(), "k.k.k").
		Entities(context.Background())
	if err != nil {
		t.Fatalf("Entities() = %v", err)
	}
	if len(entities) != 1 {
		t.Fatalf("Entities() = %d, want one station", len(entities))
	}
	e := entities[0]
	if err := e.Validate(); err != nil {
		t.Fatalf("entity invalid: %v", err)
	}
	if e.Kind != "weather_station" {
		t.Errorf("kind = %q", e.Kind)
	}
	if e.Position == nil {
		t.Error("a station with no position is not an inventory entry")
	}
	if e.Provenance.Publisher != "AEMET" || e.Provenance.License == "" {
		t.Errorf("incomplete provenance: %+v", e.Provenance)
	}
}

// AEMET serves several products as ISO-8859-15; decoding those as UTF-8 turns
// every accented Spanish name into a replacement character.
func TestObservationDecodesLatin1(t *testing.T) {
	t.Parallel()

	body := []byte(`[{"idema":"5402","lat":37.84,"lon":-4.85,"alt":90,"ubi":"C` +
		"\xd3" + `RDOBA AEROPUERTO","fint":"2026-09-02T21:00:00","ta":27.4}]`)
	srv := twoStep(t, fixture(t, "observation_step1.json"), body, "text/plain;charset=ISO-8859-15")

	records, err := aemet.NewObservation(src(t, "aemet-observation", srv.URL, nil), httpx.New(), "k.k.k").
		Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("Poll() = %d records, want 1", len(records))
	}
	if !strings.Contains(records[0].Title, "CÓRDOBA") {
		t.Errorf("title = %q, want the accent decoded", records[0].Title)
	}
}

// AEMET started writing fint with a numeric offset ("+0000", not RFC 3339's
// "+00:00"). Those readings must be stored, and a batch in which every reading
// has an unreadable timestamp must fail loudly instead of reporting success
// with zero records (#143).
func TestObservationTimestampFormats(t *testing.T) {
	t.Parallel()

	reading := func(fint string) string {
		return `{"idema":"5402","ubi":"CORDOBA AEROPUERTO","lat":37.8442,"lon":-4.8464,"ta":37.5,"fint":"` + fint + `"}`
	}
	cases := []struct {
		name     string
		body     string
		wantErr  bool
		want     int
		wantTime time.Time
	}{
		{
			name: "numeric offset", body: `[` + reading("2026-09-26T15:00:00+0000") + `]`, want: 1,
			wantTime: time.Date(2026, 9, 26, 15, 0, 0, 0, time.UTC),
		},
		{
			name: "legacy layout without zone", body: `[` + reading("2026-09-26T15:00:00") + `]`, want: 1,
			wantTime: time.Date(2026, 9, 26, 15, 0, 0, 0, time.UTC),
		},
		{name: "every timestamp unreadable", body: `[` + reading("26/09/2026 15h") + `,` + reading("") + `]`, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := twoStep(t, fixture(t, "observation_step1.json"), []byte(tc.body), "application/json")
			got, err := aemet.NewObservation(src(t, "aemet-observation", srv.URL, nil), httpx.New(), "k.k.k").
				Poll(context.Background())
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Poll() accepted %d readings with unreadable timestamps silently", len(got))
				}
				if !errors.Is(err, aemet.ErrAEMET) {
					t.Errorf("error = %v, want it wrapped in ErrAEMET", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Poll() = %v", err)
			}
			if len(got) != tc.want {
				t.Fatalf("Poll() = %d records, want %d", len(got), tc.want)
			}
			if !got[0].ObservedAt.Equal(tc.wantTime) {
				t.Errorf("ObservedAt = %v, want %v", got[0].ObservedAt, tc.wantTime)
			}
		})
	}
}

func TestObservationEmptyAndMalformed(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		body    []byte
		wantErr bool
		want    int
	}{
		{name: "empty array", body: []byte(`[]`)},
		{name: "not json", body: []byte(`<html>down for maintenance</html>`), wantErr: true},
		{name: "json object instead of array", body: []byte(`{"estado":200}`), wantErr: true},
		{name: "row with no station", body: []byte(`[{"fint":"2026-09-02T21:00:00"}]`)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := twoStep(t, fixture(t, "observation_step1.json"), tc.body, "application/json")
			got, err := aemet.NewObservation(src(t, "aemet-observation", srv.URL, nil), httpx.New(), "k.k.k").
				Poll(context.Background())
			if tc.wantErr {
				if err == nil {
					t.Fatal("a malformed payload was accepted")
				}
				if !errors.Is(err, aemet.ErrAEMET) {
					t.Errorf("error = %v, want it wrapped in ErrAEMET", err)
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

func gzipped(t *testing.T, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(body); err != nil {
		t.Fatalf("gzip: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func keys(m map[string]observation.Record) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// Asked without a credential at all, AEMET answers 200 with a zero-byte body.
// An adapter that shrugs at that reports a calm Andalucia during a red warning.
func TestEmptyBodyIsNotAnEmptyPoll(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	got, err := aemet.NewWarnings(src(t, "aemet-warnings", srv.URL, nil), httpx.New(), "k.k.k").
		Poll(context.Background())
	if err == nil {
		t.Fatal("a 200 with an empty body was read as a successful poll")
	}
	if !errors.Is(err, aemet.ErrAEMET) {
		t.Errorf("error = %v, want it wrapped in ErrAEMET", err)
	}
	if len(got) != 0 {
		t.Errorf("Poll() = %d records", len(got))
	}
}

// AEMET publishes a request budget. Spending it is a distinct outcome from a
// wrong key, and the scheduler backs off differently for each.
func TestRateLimitIsItsOwnOutcome(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := aemet.NewObservation(src(t, "aemet-observation", srv.URL, nil), httpx.New(), "k.k.k").
		Poll(context.Background())
	if !errors.Is(err, aemet.ErrRateLimited) {
		t.Fatalf("Poll() = %v, want ErrRateLimited", err)
	}
	if errors.Is(err, aemet.ErrUnauthorized) {
		t.Error("a spent budget was reported as a rejected key")
	}
}

// The archive holds one message per zone and phenomenon; a bulletin the
// adapter cannot key to a zone is dropped rather than stored unfollowable.
func TestCAPWithoutZoneIsDropped(t *testing.T) {
	t.Parallel()

	body := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<alert xmlns="urn:oasis:names:tc:emergency:cap:1.2">
  <identifier>2.49.0.0.724.0.ES.20260902230000.NOZONE</identifier>
  <sender>http://www.aemet.es</sender>
  <sent>2026-09-02T23:00:00-00:00</sent>
  <status>Actual</status><msgType>Alert</msgType><scope>Public</scope>
  <info>
    <language>es-ES</language><category>Met</category>
    <event>Aviso de vientos de nivel amarillo</event>
    <severity>Moderate</severity><certainty>Likely</certainty>
    <onset>2026-09-03T10:00:00+02:00</onset>
    <expires>2026-09-03T20:00:00+02:00</expires>
    <headline>Aviso de vientos de nivel amarillo</headline>
    <parameter><valueName>AEMET-Meteoalerta nivel</valueName><value>amarillo</value></parameter>
    <area><areaDesc>Zona sin codigo</areaDesc></area>
  </info>
</alert>`)

	alert, err := aemet.ParseCAP(body)
	if err != nil {
		t.Fatalf("ParseCAP() = %v", err)
	}
	info, ok := alert.Localized("es-ES")
	if !ok {
		t.Fatal("no Spanish info block")
	}
	if len(info.Areas) != 1 || info.Areas[0].Zone() != "" {
		t.Fatalf("fixture does not exercise the missing zone: %+v", info.Areas)
	}
	if got := info.Level(); got != "amarillo" {
		t.Errorf("Level() = %q", got)
	}
}

func TestParseCAPRejectsRubbish(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
	}{
		{name: "empty", body: ""},
		{name: "html", body: "<html><body>down</body></html>"},
		{name: "alert with no info", body: `<alert xmlns="urn:oasis:names:tc:emergency:cap:1.2"><identifier>x</identifier></alert>`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := aemet.ParseCAP([]byte(tc.body)); err == nil {
				t.Fatal("a malformed CAP message was accepted")
			}
		})
	}
}

// The Meteoalerta colour is the authority's own ladder. CAP's severity is the
// fallback for a message that omits the colour, and the plan defines them as
// the same four steps.
func TestSeverityMapsBothLadders(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		level    string
		capLevel string
		want     observation.Severity
	}{
		{name: "rojo", level: "rojo", want: observation.SeverityCritical},
		{name: "naranja", level: "naranja", want: observation.SeverityHigh},
		{name: "amarillo", level: "amarillo", want: observation.SeverityModerate},
		{name: "verde", level: "verde", want: observation.SeverityInfo},
		{name: "no colour falls back to Extreme", capLevel: "Extreme", want: observation.SeverityCritical},
		{name: "no colour falls back to Severe", capLevel: "Severe", want: observation.SeverityHigh},
		{name: "no colour falls back to Moderate", capLevel: "Moderate", want: observation.SeverityModerate},
		{name: "no colour falls back to Minor", capLevel: "Minor", want: observation.SeverityInfo},
		{name: "nothing at all is information, not silence", want: observation.SeverityInfo},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var parameter string
			if tc.level != "" {
				parameter = `<parameter><valueName>AEMET-Meteoalerta nivel</valueName><value>` +
					tc.level + `</value></parameter>`
			}
			body := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<alert xmlns="urn:oasis:names:tc:emergency:cap:1.2">
  <identifier>2.49.0.0.724.0.ES.20260902230000.LADDER</identifier>
  <sender>http://www.aemet.es</sender>
  <sent>2026-09-02T23:00:00-00:00</sent>
  <status>Actual</status><msgType>Alert</msgType><scope>Public</scope>
  <info>
    <language>es-ES</language><category>Met</category>
    <event>Aviso</event><severity>` + tc.capLevel + `</severity>
    <headline>Aviso</headline>` + parameter + `
    <eventCode><valueName>AEMET-Meteoalerta fenomeno</valueName><value>VI;Vientos</value></eventCode>
    <area><areaDesc>Campiña cordobesa</areaDesc>
      <geocode><valueName>AEMET-Meteoalerta zona</valueName><value>611402</value></geocode>
    </area>
  </info>
</alert>`)

			srv := twoStep(t, fixture(t, "warnings_step1.json"), tarGzip(t, body), "application/x-gzip")
			records, err := aemet.NewWarnings(src(t, "aemet-warnings", srv.URL, nil), httpx.New(), "k.k.k").
				Poll(context.Background())
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

// AEMET publishes each warning in Spanish and English. Which one eye reads is
// the registry's choice, and an unknown language must not empty the bulletin.
func TestWarningsLanguageSelection(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		language string
		want     string
	}{
		{name: "the default is Spanish", want: "Aviso de temperaturas máximas de nivel naranja"},
		{name: "English is published too", language: "en-GB", want: "Severe high temperature warning"},
		{name: "a language family still matches", language: "en", want: "Severe high temperature warning"},
		{name: "an unpublished language falls back", language: "fr-FR", want: "Aviso de temperaturas máximas"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := map[string]string{}
			if tc.language != "" {
				opts["language"] = tc.language
			}
			srv := twoStep(t, fixture(t, "warnings_step1.json"),
				capArchive(t, "Z_CAP_C_LEMM_20260902230000_AFAZ611402ATTA030000.xml"), "application/x-gzip")

			records, err := aemet.NewWarnings(src(t, "aemet-warnings", srv.URL, opts), httpx.New(), "k.k.k").
				Poll(context.Background())
			if err != nil {
				t.Fatalf("Poll() = %v", err)
			}
			if len(records) != 1 {
				t.Fatalf("Poll() = %d records, want 1", len(records))
			}
			if !strings.Contains(records[0].Title, tc.want) {
				t.Errorf("title = %q, want it to contain %q", records[0].Title, tc.want)
			}
		})
	}
}

// A CAP polygon is written as "lat,lon"; GeoJSON wants [lon, lat]. Getting that
// backwards would put every Cordoba warning in the Indian Ocean.
func TestPolygonBecomesGeoJSON(t *testing.T) {
	t.Parallel()

	orange := requireZone(t, warningRecords(t), "611402:AT")

	var geometry struct {
		Type        string        `json:"type"`
		Coordinates [][][]float64 `json:"coordinates"`
	}
	if err := json.Unmarshal(orange.Geometry, &geometry); err != nil {
		t.Fatalf("geometry: %v", err)
	}
	if geometry.Type != "Polygon" {
		t.Fatalf("geometry type = %q", geometry.Type)
	}
	ring := geometry.Coordinates[0]
	if len(ring) < 4 {
		t.Fatalf("ring has %d points", len(ring))
	}
	if ring[0][0] != ring[len(ring)-1][0] || ring[0][1] != ring[len(ring)-1][1] {
		t.Error("the ring is not closed")
	}
	for _, point := range ring {
		if point[0] > 0 || point[0] < -6 {
			t.Fatalf("longitude %v is not in western Andalucia; lat and lon are swapped", point[0])
		}
		if point[1] < 36 || point[1] > 39 {
			t.Fatalf("latitude %v is not in Andalucia", point[1])
		}
	}
	if orange.Position.Lat < 37 || orange.Position.Lat > 39 || orange.Position.Lon > -4 || orange.Position.Lon < -6 {
		t.Errorf("centroid %+v is outside the zone", orange.Position)
	}
}

// A zone drawn as several disjoint pieces is a MultiPolygon, not a lie about
// one of them.
func TestSeveralPolygonsBecomeAMultiPolygon(t *testing.T) {
	t.Parallel()

	body := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<alert xmlns="urn:oasis:names:tc:emergency:cap:1.2">
  <identifier>2.49.0.0.724.0.ES.20260902230000.MULTI</identifier>
  <sender>http://www.aemet.es</sender>
  <sent>2026-09-02T23:00:00-00:00</sent>
  <status>Actual</status><msgType>Alert</msgType><scope>Public</scope>
  <info>
    <language>es-ES</language><category>Met</category>
    <event>Aviso de vientos de nivel amarillo</event><severity>Moderate</severity>
    <headline>Aviso de vientos de nivel amarillo</headline>
    <parameter><valueName>AEMET-Meteoalerta nivel</valueName><value>amarillo</value></parameter>
    <area><areaDesc>Campiña cordobesa</areaDesc>
      <polygon>38.0,-5.1 37.9,-4.5 37.7,-4.8 38.0,-5.1</polygon>
      <polygon>37.5,-4.9 37.4,-4.6 37.3,-4.8 37.5,-4.9</polygon>
      <polygon>this one is not coordinates at all</polygon>
      <geocode><valueName>AEMET-Meteoalerta zona</valueName><value>611402</value></geocode>
    </area>
  </info>
</alert>`)

	srv := twoStep(t, fixture(t, "warnings_step1.json"), tarGzip(t, body), "application/x-gzip")
	records, err := aemet.NewWarnings(src(t, "aemet-warnings", srv.URL, nil), httpx.New(), "k.k.k").
		Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("Poll() = %d records, want 1", len(records))
	}

	var geometry struct {
		Type        string          `json:"type"`
		Coordinates [][][][]float64 `json:"coordinates"`
	}
	if err := json.Unmarshal(records[0].Geometry, &geometry); err != nil {
		t.Fatalf("geometry: %v", err)
	}
	if geometry.Type != "MultiPolygon" {
		t.Errorf("geometry type = %q, want MultiPolygon", geometry.Type)
	}
	if len(geometry.Coordinates) != 2 {
		t.Errorf("kept %d rings, want the two readable ones", len(geometry.Coordinates))
	}
}

// The registry entry points at the portal; the API lives one segment below it.
// A source that already names the API root must not gain a second one.
func TestAPIRootIsResolvedFromTheRegistryURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		url  string
	}{
		{name: "the portal home", url: "https://opendata.aemet.es/"},
		{name: "the API root itself", url: "https://opendata.aemet.es/opendata"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				w.WriteHeader(http.StatusNotFound)
			}))
			defer srv.Close()

			s := src(t, "aemet-warnings", srv.URL, map[string]string{"base": tc.url})
			// Point the base at the test server while keeping the shape
			// of the URL under test.
			s.Options["base"] = srv.URL + strings.TrimPrefix(tc.url, "https://opendata.aemet.es")
			_, _ = aemet.NewWarnings(s, httpx.New(), "k.k.k").Poll(context.Background())

			if gotPath != "/opendata/api/avisos_cap/ultimoelaborado/area/61" {
				t.Errorf("asked for %q, want the documented path exactly once", gotPath)
			}
		})
	}
}

// A registry entry may pin the retention window; the default is a month.
func TestObservationRetentionIsConfigurable(t *testing.T) {
	t.Parallel()

	srv := twoStep(t, fixture(t, "observation_step1.json"),
		fixture(t, "observation_datos.json"), "application/json")

	records, err := aemet.NewObservation(src(t, "aemet-observation", srv.URL,
		map[string]string{"ttl": "48h"}), httpx.New(), "k.k.k").Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) == 0 {
		t.Fatal("Poll() returned nothing")
	}
	if got := records[0].ExpiresAt.Sub(records[0].ObservedAt); got != 48*time.Hour {
		t.Errorf("retention = %s, want the configured 48h", got)
	}
}

// The two-step format names a transport, not a product. Reading warnings under
// the observation source's id would be worse than not polling at all.
func TestProductResolution(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		product string
		entity  bool
		wantErr error
	}{
		{name: "warnings", product: "warnings"},
		{name: "the Spanish name works too", product: "avisos"},
		{name: "observation", product: "observation", entity: true},
		{name: "the Spanish name for observation", product: "observacion", entity: true},
		{name: "unnamed refuses to poll", wantErr: aemet.ErrUnspecifiedProduct},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := map[string]string{}
			if tc.product != "" {
				opts["product"] = tc.product
			}
			s := src(t, "aemet-two-step", "https://opendata.aemet.es", opts)
			p := aemet.NewProduct(s, httpx.New(), "")

			if p.Info().ID != "aemet-two-step" {
				t.Errorf("Info() = %q", p.Info().ID)
			}

			_, err := p.Poll(context.Background())
			want := tc.wantErr
			if want == nil {
				want = aemet.ErrMissingAPIKey
			}
			if !errors.Is(err, want) {
				t.Fatalf("Poll() = %v, want %v", err, want)
			}

			if _, ok := p.(interface {
				Entities(context.Context) ([]observation.Entity, error)
			}); ok != tc.entity {
				t.Errorf("publishes an inventory = %v, want %v", ok, tc.entity)
			}
		})
	}
}

// tarGzip wraps one CAP message in the archive AEMET serves.
func tarGzip(t *testing.T, body []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "Z_CAP_C_LEMM_20260902230000_AFAZ.xml", Mode: 0o644, Size: int64(len(body))}); err != nil {
		t.Fatalf("tar header: %v", err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatalf("tar write: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}
