package firms_test

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

	"github.com/FullFran/cordvba/apps/eye/internal/httpx"
	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	"github.com/FullFran/cordvba/apps/eye/internal/provider/infrastructure/firms"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// fixture reads a recorded response. Tests never reach the network.
func fixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile("../../../../testdata/firms/" + name) // #nosec G304 -- fixture path built from a test literal
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return body
}

// serve answers every request with one recorded body.
func serve(t *testing.T, status int, body []byte, path *string) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if path != nil {
			*path = r.URL.Path
		}
		w.Header().Set("Content-Type", "text/csv")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func src(base string, opts map[string]string) source.Source {
	if opts == nil {
		opts = map[string]string{}
	}
	if _, ok := opts["bbox"]; !ok {
		// Wide enough to contain every row of the recorded fixture, which
		// reaches 38.96. These tests are about PARSING, and coupling them
		// to the bounding box would make a filtering change look like a
		// parsing regression. TestArchiveFiltersToTheBoundingBox is where
		// the box itself is tested.
		opts["bbox"] = "-7.6,36.0,-2.0,39.2"
	}
	return source.Source{
		ID: "nasa-firms", Authority: "NASA FIRMS", Topic: "fire", URL: base + "/api/area/",
		License: "nasa-open-data", Format: "rest-csv",
		Access: source.AccessDocumentedAPI, Automation: source.AutomationEnabled,
		Interval: 10 * time.Minute, Options: opts,
	}
}

// A build with no credential must say which variable is missing. Returning an
// empty slice would report "no fires" during a fire.
func TestMissingMapKeyIsNamed(t *testing.T) {
	t.Parallel()

	got, err := firms.New(src("https://firms.modaps.eosdis.nasa.gov", nil), httpx.New(), "").
		Poll(context.Background())
	if !errors.Is(err, firms.ErrMissingMapKey) {
		t.Fatalf("Poll() = %v, want ErrMissingMapKey", err)
	}
	if !strings.Contains(err.Error(), "FIRMS_MAP_KEY") {
		t.Errorf("error %q does not name the environment variable", err)
	}
	if got != nil {
		t.Errorf("Poll() = %d records, want none", len(got))
	}
}

// detections polls the recorded overpass, reporting the path that was asked for.
func detections(t *testing.T) ([]observation.Record, string) {
	t.Helper()

	var gotPath string
	srv := serve(t, http.StatusOK, fixture(t, "andalucia_24h.csv"), &gotPath)

	p := firms.New(src(srv.URL, map[string]string{"source": "VIIRS_SNPP_NRT", "days": "1"}),
		httpx.New(), testMapKey)
	records, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) != 29 {
		t.Fatalf("Poll() = %d records, want 29 detections", len(records))
	}
	return records, gotPath
}

func TestPoll(t *testing.T) {
	t.Parallel()

	records, gotPath := detections(t)

	if !strings.Contains(gotPath, "VIIRS_SNPP_NRT") || !strings.Contains(gotPath, "-7.6,36.0,-2.0,39.2") {
		t.Errorf("request path = %q, want the product and the bounding box", gotPath)
	}

	r := records[0]
	if err := r.Validate(); err != nil {
		t.Fatalf("record invalid: %v", err)
	}
	if r.Quality != observation.QualityPreliminary {
		t.Errorf("quality = %q, want preliminary: NRT detections are not validated", r.Quality)
	}
	if r.Kind != "fire_detection" {
		t.Errorf("kind = %q", r.Kind)
	}
	if r.Position == nil || !r.Position.Valid() {
		t.Error("a detection with no position is not a detection")
	}
}

func TestPollTimestampsAndRetention(t *testing.T) {
	t.Parallel()

	records, _ := detections(t)
	r := records[0]

	want := time.Date(2026, time.September, 1, 1, 41, 0, 0, time.UTC)
	if !r.ObservedAt.Equal(want) {
		t.Errorf("ObservedAt = %s, want %s from acq_date and acq_time", r.ObservedAt, want)
	}
	if r.ObservedAt.Equal(r.FetchedAt) {
		t.Error("ObservedAt and FetchedAt were merged")
	}
	if r.ExpiresAt == nil {
		t.Error("a near-real-time detection must carry a retention deadline")
	}
}

// The map key is a path segment, so provenance has to be built from a URL that
// does not carry it.
func TestPollProvenanceCarriesNoCredential(t *testing.T) {
	t.Parallel()

	records, _ := detections(t)
	r := records[0]

	if r.Provenance.RawHash == "" || r.Provenance.Publisher == "" ||
		r.Provenance.License == "" || r.Provenance.SourceURL == "" {
		t.Errorf("incomplete provenance: %+v", r.Provenance)
	}
	if strings.Contains(r.Provenance.SourceURL, testMapKey) {
		t.Errorf("the map key leaked into provenance: %q", r.Provenance.SourceURL)
	}
}

// FIRMS publishes a per-detection `confidence`. It is product metadata about
// the algorithm, not eye's confidence in the observation, and AGENTS.md names
// this exact mistake.
func TestConfidenceStaysInThePayload(t *testing.T) {
	t.Parallel()

	srv := serve(t, http.StatusOK, fixture(t, "andalucia_24h.csv"), nil)
	records, err := firms.New(src(srv.URL, nil), httpx.New(), testMapKey).
		Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}

	var sawNominal, sawHigh bool
	for _, r := range records {
		var payload map[string]any
		if err := json.Unmarshal(r.Payload, &payload); err != nil {
			t.Fatalf("payload: %v", err)
		}
		got, ok := payload["confidence"]
		if !ok {
			t.Fatalf("payload lost the FIRMS confidence: %v", payload)
		}
		switch got {
		case "nominal":
			sawNominal = true
		case "high":
			sawHigh = true
		}
		// Every record must carry eye's own full confidence, never the
		// product's grade rendered as a number.
		if r.Confidence != 1 {
			t.Errorf("Record.Confidence = %v for a %v detection, want 1", r.Confidence, got)
		}
		for _, key := range []string{"frp", "bright_ti4", "satellite", "daynight", "version"} {
			if _, ok := payload[key]; !ok {
				t.Errorf("payload dropped %q: %v", key, payload)
			}
		}
	}
	if !sawNominal || !sawHigh {
		t.Error("the fixture no longer exercises both confidence grades")
	}
}

// Fire radiative power is what separates a smouldering pixel from a front.
func TestSeverityTracksFireRadiativePower(t *testing.T) {
	t.Parallel()

	header := "latitude,longitude,bright_ti4,scan,track,acq_date,acq_time,satellite,confidence,version,bright_ti5,frp,daynight\n"
	cases := []struct {
		name string
		frp  string
		want observation.Severity
	}{
		{name: "a faint pixel", frp: "0.6", want: observation.SeverityLow},
		{name: "an ordinary detection", frp: "12.0", want: observation.SeverityModerate},
		{name: "a strong detection", frp: "180.0", want: observation.SeverityHigh},
		{name: "a fire front", frp: "900.0", want: observation.SeverityCritical},
		{name: "no power reported", frp: "", want: observation.SeverityLow},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			body := header + "37.8,-4.8,340.0,0.4,0.4,2026-09-01,1301,N,nominal,2.0NRT,300.0," + tc.frp + ",D\n"
			srv := serve(t, http.StatusOK, []byte(body), nil)

			records, err := firms.New(src(srv.URL, nil), httpx.New(), testMapKey).
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

// The area API adds an `instrument` column the public archive does not carry.
// Reading by column name rather than by position is what makes both parse.
func TestReadsColumnsByName(t *testing.T) {
	t.Parallel()

	body := "latitude,longitude,bright_ti4,scan,track,acq_date,acq_time,satellite,instrument,confidence,version,bright_ti5,frp,daynight\n" +
		"37.85,-4.79,338.4,0.45,0.42,2026-09-01,1302,N,VIIRS,high,2.0NRT,302.1,44.2,D\n"
	srv := serve(t, http.StatusOK, []byte(body), nil)

	records, err := firms.New(src(srv.URL, nil), httpx.New(), testMapKey).
		Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("Poll() = %d records, want 1", len(records))
	}

	var payload map[string]any
	if err := json.Unmarshal(records[0].Payload, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if payload["instrument"] != "VIIRS" {
		t.Errorf("instrument = %v, want the extra column kept", payload["instrument"])
	}
}

func TestErrorEmptyAndMalformed(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		status  int
		body    []byte
		wantErr error
		want    int
	}{
		{
			name:    "rejected map key",
			status:  http.StatusBadRequest,
			body:    fixture(t, "invalid_map_key.txt"),
			wantErr: firms.ErrRejectedMapKey,
		},
		{
			name:    "rejected map key with a 200",
			status:  http.StatusOK,
			body:    fixture(t, "invalid_map_key.txt"),
			wantErr: firms.ErrRejectedMapKey,
		},
		{
			name:   "header only means no fires",
			status: http.StatusOK,
			body:   fixture(t, "empty.csv"),
		},
		{
			name:    "a completely empty body",
			status:  http.StatusOK,
			body:    nil,
			wantErr: firms.ErrFIRMS,
		},
		{
			name:    "html error page",
			status:  http.StatusOK,
			body:    []byte("<html><body>Service unavailable</body></html>"),
			wantErr: firms.ErrFIRMS,
		},
		{
			name:   "a row with an unreadable coordinate is skipped",
			status: http.StatusOK,
			body: []byte("latitude,longitude,bright_ti4,scan,track,acq_date,acq_time,satellite,confidence,version,bright_ti5,frp,daynight\n" +
				"not-a-number,-4.8,340.0,0.4,0.4,2026-09-01,1301,N,nominal,2.0NRT,300.0,5.0,D\n" +
				"37.8,-4.8,340.0,0.4,0.4,2026-09-01,1301,N,nominal,2.0NRT,300.0,5.0,D\n"),
			want: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := serve(t, tc.status, tc.body, nil)
			got, err := firms.New(src(srv.URL, nil), httpx.New(), testMapKey).
				Poll(context.Background())

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Poll() = %v, want %v", err, tc.wantErr)
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

// A source with no viewport must say so rather than asking FIRMS for the world.
func TestMissingBoundingBox(t *testing.T) {
	t.Parallel()

	s := src("https://firms.modaps.eosdis.nasa.gov", map[string]string{"bbox": " "})
	_, err := firms.New(s, httpx.New(), testMapKey).Poll(context.Background())
	if !errors.Is(err, firms.ErrFIRMS) {
		t.Fatalf("Poll() = %v, want ErrFIRMS", err)
	}
	if !strings.Contains(err.Error(), "bbox") {
		t.Errorf("error %q does not name the missing option", err)
	}
}

// FIRMS puts the credential in the URL path, and the transport quotes URLs back
// in its errors. That error is persisted as the source's last failure, so the
// key must not survive the trip.
// testMapKey stands in for a FIRMS credential.
//
// It is deliberately not key-shaped. A plausible-looking hex string here was
// reported by the repository's secret scanner as a leaked credential, and a
// scanner that cries wolf on test fixtures is a scanner people start ignoring.
// Nothing under test cares about the shape: redaction is a string replacement.
const testMapKey = "FIRMS-TEST-KEY-NOT-A-CREDENTIAL"

func TestTransportErrorsDoNotLeakTheMapKey(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := firms.New(src(srv.URL, nil), httpx.New(), testMapKey).Poll(context.Background())
	if err == nil {
		t.Fatal("a 500 was accepted")
	}
	if strings.Contains(err.Error(), testMapKey) {
		t.Fatalf("the map key leaked into the error: %v", err)
	}
	if !strings.Contains(err.Error(), "[MAP_KEY]") {
		t.Errorf("error %q does not show where the key was redacted", err)
	}
	if !errors.Is(err, httpx.ErrStatus) {
		t.Errorf("redaction broke the error chain: %v", err)
	}
}

// archiveSource points the adapter at NASA's keyless NRT archive rather than
// the credentialed area API.
func archiveSource(url string, opts map[string]string) source.Source {
	if opts == nil {
		opts = map[string]string{}
	}
	opts["archive"] = url
	s := src(url, opts)
	s.URL = url
	return s
}

// TestArchiveNeedsNoMapKey is the point of the archive mode.
//
// NASA publishes the same detections twice: through an area API that needs a
// credential, and through a public NRT archive that needs none. The archive is
// the one a second person running eye can actually reproduce, which is worth
// more to this project than the area API's server-side filtering.
func TestArchiveNeedsNoMapKey(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("../../../../testdata/firms/andalucia_24h.csv")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	// No map key at all — the third argument is empty.
	records, err := firms.New(archiveSource(srv.URL, nil), httpx.New(), "").
		Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) == 0 {
		t.Fatal("the keyless archive produced no records")
	}
	for _, r := range records {
		if err := r.Validate(); err != nil {
			t.Errorf("record invalid: %v", err)
		}
	}
}

// The archive is continental, so the bounding box has to be applied here. The
// area API filters server-side; asking NASA's static file to do that is not an
// option, and storing every fire in Europe under a Cordoba source would be a
// quiet lie about what eye is watching.
func TestArchiveFiltersToTheBoundingBox(t *testing.T) {
	t.Parallel()

	const continental = "latitude,longitude,bright_ti4,scan,track,acq_date,acq_time,satellite,confidence,version,bright_ti5,frp,daynight\n" +
		"37.9,-4.8,300.1,0.4,0.4,2026-09-02,1427,N,n,2.0NRT,290.0,5.2,D\n" + // Cordoba
		"52.5,13.4,310.0,0.4,0.4,2026-09-02,1427,N,n,2.0NRT,295.0,7.1,D\n" + // Berlin
		"41.9,12.5,305.0,0.4,0.4,2026-09-02,1427,N,n,2.0NRT,292.0,6.0,D\n" // Rome

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(continental))
	}))
	t.Cleanup(srv.Close)

	records, err := firms.New(
		archiveSource(srv.URL, map[string]string{"bbox": "-5.8,37.2,-4.0,38.8"}),
		httpx.New(), "",
	).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want only the one inside the box", len(records))
	}
	if got := records[0].Position; got == nil || got.Lat != 37.9 {
		t.Errorf("kept the wrong detection: %+v", got)
	}
}

// Provenance must name the archive file, not the area API with a placeholder
// key: a reader has to be able to fetch exactly what eye fetched.
func TestArchiveProvenanceNamesTheFile(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("../../../../testdata/firms/andalucia_24h.csv")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	records, err := firms.New(archiveSource(srv.URL, nil), httpx.New(), "").
		Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) == 0 {
		t.Fatal("expected records")
	}
	if got := records[0].Provenance.SourceURL; got != srv.URL {
		t.Errorf("source URL = %q, want the archive file %q", got, srv.URL)
	}
	if strings.Contains(records[0].Provenance.SourceURL, "MAP_KEY") {
		t.Error("the archive provenance mentions a credential it never used")
	}
}
