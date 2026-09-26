package metar_test

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
	"github.com/FullFran/cordvba/apps/eye/internal/provider/infrastructure/metar"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// serveFixture returns the recorded LEBA report and records what was asked.
func serveFixture(t *testing.T, asked *string) *httptest.Server {
	t.Helper()

	body, err := os.ReadFile("../../../../testdata/metar/leba.json") // #nosec G304 -- fixture path built from a test literal
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if asked != nil {
			*asked = r.URL.RawQuery
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func metarSource(url string, opts map[string]string) source.Source {
	return source.Source{
		ID: "metar-cordoba", Authority: "NOAA Aviation Weather Center",
		Topic: "weather", URL: url, Format: "metar-json",
		License: "us-government-public-domain",
		Access:  source.AccessDocumentedAPI, Automation: source.AutomationEnabled,
		Interval: 20 * time.Minute, Options: opts,
	}
}

func TestPollReadsTheCordobaStation(t *testing.T) {
	t.Parallel()

	var asked string
	srv := serveFixture(t, &asked)
	records, err := metar.New(metarSource(srv.URL, nil), httpx.New()).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want one report", len(records))
	}
	if !strings.Contains(asked, "ids=LEBA") {
		t.Errorf("query = %q, want the LEBA station", asked)
	}

	r := records[0]
	if r.LocalKey != "LEBA" {
		t.Errorf("local key = %q, want the ICAO id", r.LocalKey)
	}
	if r.Position == nil || !r.Position.Valid() {
		t.Fatal("the aerodrome position was lost")
	}
	if got := r.Position.Lat; got < 37.7 || got > 38.0 {
		t.Errorf("lat = %v, not Cordoba airport", got)
	}
	if err := r.Validate(); err != nil {
		t.Errorf("record invalid: %v", err)
	}
}

// A reading nobody has reviewed must not be filed as an official declaration,
// and it must expire: it is a sample of a moving signal, not a statement that
// stays true.
func TestPollMarksReadingsPreliminaryAndShortLived(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t, nil)
	records, err := metar.New(metarSource(srv.URL, nil), httpx.New()).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}

	r := records[0]
	if r.Quality != observation.QualityPreliminary {
		t.Errorf("quality = %q, want preliminary", r.Quality)
	}
	if r.ExpiresAt == nil {
		t.Fatal("a surface reading was stored without a retention deadline")
	}
	if !r.ExpiresAt.After(r.ObservedAt) {
		t.Errorf("expires_at %v is not after observed_at %v", r.ExpiresAt, r.ObservedAt)
	}
}

// The observation time is the station's, never the fetch time. Their difference
// is the age of the reading, and a METAR two hours old must look two hours old.
func TestPollKeepsTheStationTimeApartFromTheFetchTime(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t, nil)
	records, err := metar.New(metarSource(srv.URL, nil), httpx.New()).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}

	r := records[0]
	if r.ObservedAt.IsZero() || r.FetchedAt.IsZero() {
		t.Fatal("a timestamp is missing")
	}
	if r.ObservedAt.Equal(r.FetchedAt) {
		t.Error("the reading was dated at fetch time; its age was lost")
	}
	if r.Provenance.RawHash == "" {
		t.Error("the reading cannot be replayed from its evidence")
	}
}

// The payload must say what this reading is and is not. The instrument sits at
// the same airport AEMET reports as station 5402, and a reader comparing the
// two has to know they are different publishers with different purposes.
func TestPollRecordsWhatTheReadingIsNot(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t, nil)
	records, err := metar.New(metarSource(srv.URL, nil), httpx.New()).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}

	var pay struct {
		ICAO string   `json:"icao"`
		Temp *float64 `json:"temperature_c"`
		Raw  string   `json:"raw"`
		Note string   `json:"note"`
	}
	if err := json.Unmarshal(records[0].Payload, &pay); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if pay.ICAO != "LEBA" || pay.Temp == nil {
		t.Errorf("payload lost the reading: %+v", pay)
	}
	if !strings.Contains(pay.Raw, "METAR LEBA") {
		t.Errorf("the raw report was not preserved: %q", pay.Raw)
	}
	if !strings.Contains(pay.Note, "not a statement by the competent") {
		t.Errorf("the caveat about the publisher was dropped: %q", pay.Note)
	}
}

// The station is a persistent thing and belongs in inventory, not only in the
// time series.
func TestEntitiesPublishesTheStation(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t, nil)
	entities, err := metar.New(metarSource(srv.URL, nil), httpx.New()).Entities(context.Background())
	if err != nil {
		t.Fatalf("Entities() = %v", err)
	}
	if len(entities) != 1 {
		t.Fatalf("entities = %d, want one station", len(entities))
	}
	if entities[0].Kind != "weather_station" {
		t.Errorf("kind = %q, want weather_station", entities[0].Kind)
	}
	if entities[0].Position == nil {
		t.Error("the station has no position")
	}
}

// A visibility of "6+" is a qualified value the publisher chose to express in
// words. Forcing it into a number would drop the qualification silently.
func TestPollPreservesAQualifiedVisibility(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t, nil)
	records, err := metar.New(metarSource(srv.URL, nil), httpx.New()).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}

	var pay struct {
		Visibility any `json:"visibility"`
	}
	if err := json.Unmarshal(records[0].Payload, &pay); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if pay.Visibility == nil {
		t.Error("visibility was dropped")
	}
}

func TestPollRejectsRubbish(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html>not json</html>"))
	}))
	t.Cleanup(srv.Close)

	if _, err := metar.New(metarSource(srv.URL, nil), httpx.New()).Poll(context.Background()); err == nil {
		t.Fatal("expected a parse error")
	}
}
