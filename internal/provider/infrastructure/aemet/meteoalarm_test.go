package aemet_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/FullFran/eye/internal/httpx"
	observation "github.com/FullFran/eye/internal/observation/domain"
	"github.com/FullFran/eye/internal/provider/infrastructure/aemet"
	source "github.com/FullFran/eye/internal/source/domain"
)

// serveMeteoAlarm returns the recorded Spanish warning feed.
func serveMeteoAlarm(t *testing.T) *httptest.Server {
	t.Helper()

	body, err := os.ReadFile("../../../../testdata/meteoalarm/spain-warnings.xml") // #nosec G304 -- fixture path built from a test literal
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func meteoAlarmSource(url string, opts map[string]string) source.Source {
	return source.Source{
		ID: "aemet-warnings", Authority: "AEMET. Agencia Estatal de Meteorologia",
		Topic: "weather", URL: url, Format: "meteoalarm-atom",
		License: "cc-by-4.0-equivalent-meteoalarm-terms",
		Access:  source.AccessDocumentedAPI, Automation: source.AutomationEnabled,
		Interval: 5 * time.Minute, Options: opts,
	}
}

// TestMeteoAlarmNeedsNoAPIKey is the whole reason this provider exists.
func TestMeteoAlarmNeedsNoAPIKey(t *testing.T) {
	t.Parallel()

	srv := serveMeteoAlarm(t)
	records, err := aemet.NewMeteoAlarm(meteoAlarmSource(srv.URL, nil), httpx.New()).
		Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) == 0 {
		t.Fatal("the keyless feed produced no warnings")
	}
	for _, r := range records {
		if err := r.Validate(); err != nil {
			t.Errorf("record invalid: %v", err)
		}
		if r.Kind != "weather_warning" {
			t.Errorf("kind = %q, want weather_warning", r.Kind)
		}
	}
}

// TestMeteoAlarmMapsACordobaWarning checks the fields a person standing in
// Cordoba actually needs: which zone, how bad, and when it starts and ends.
func TestMeteoAlarmMapsACordobaWarning(t *testing.T) {
	t.Parallel()

	srv := serveMeteoAlarm(t)
	records, err := aemet.NewMeteoAlarm(meteoAlarmSource(srv.URL, nil), httpx.New()).
		Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}

	var found bool
	for _, r := range records {
		var pay struct {
			Area      string `json:"area"`
			Zone      string `json:"zone"`
			Issuer    string `json:"issuing_authority"`
			RelayedBy string `json:"relayed_by"`
			Severity  string `json:"cap_severity"`
		}
		if err := json.Unmarshal(r.Payload, &pay); err != nil {
			t.Fatalf("payload: %v", err)
		}
		if pay.Area != "Campiña cordobesa" {
			continue
		}
		found = true

		// The fixture holds a Severe high-temperature warning for that zone.
		if r.Severity != observation.SeverityHigh {
			t.Errorf("severity = %d, want %d for a Severe warning", r.Severity, observation.SeverityHigh)
		}
		if r.ValidFrom == nil || r.ValidUntil == nil {
			t.Fatal("a warning with an onset and an expiry must carry both")
		}
		if !r.ValidFrom.Before(*r.ValidUntil) {
			t.Errorf("valid_from %v is not before valid_until %v", r.ValidFrom, r.ValidUntil)
		}
		if r.ExpiresAt != nil {
			t.Error("a warning must not be given a retention deadline; an expired warning is still the record of what was said")
		}
		if pay.Zone == "" {
			t.Error("the EMMA_ID zone was lost")
		}
		// The relay must be visible, not laundered into looking direct.
		if !strings.Contains(pay.Issuer, "AEMET") {
			t.Errorf("issuing authority = %q, want AEMET", pay.Issuer)
		}
		if pay.RelayedBy != "meteoalarm.org" {
			t.Errorf("relayed_by = %q; the relay must be recorded", pay.RelayedBy)
		}
		if r.Provenance.SourceURL != srv.URL {
			t.Errorf("source URL = %q, want what eye actually fetched", r.Provenance.SourceURL)
		}
	}
	if !found {
		t.Fatal("the Campiña cordobesa warning in the fixture did not survive mapping")
	}
}

// TestMeteoAlarmKeepsBothTimestampsApart is the invariant the project rests on.
func TestMeteoAlarmKeepsBothTimestampsApart(t *testing.T) {
	t.Parallel()

	srv := serveMeteoAlarm(t)
	records, err := aemet.NewMeteoAlarm(meteoAlarmSource(srv.URL, nil), httpx.New()).
		Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	for _, r := range records {
		if r.ObservedAt.IsZero() || r.FetchedAt.IsZero() {
			t.Fatalf("record %s has a zero timestamp", r.ID)
		}
		if r.ObservedAt.Equal(r.FetchedAt) {
			t.Errorf("record %s was dated at fetch time; the issue time was lost", r.ID)
		}
		if r.Provenance.RawHash == "" {
			t.Errorf("record %s cannot be replayed from its evidence", r.ID)
		}
	}
}

// TestMeteoAlarmIngestsEverySpanishWarningByDefault checks eye does not narrow
// at ingestion. A warning dropped here is one no later query can recover, and
// missing a red warning because a zone was renamed is not a trade worth making.
func TestMeteoAlarmIngestsEverySpanishWarningByDefault(t *testing.T) {
	t.Parallel()

	srv := serveMeteoAlarm(t)
	records, err := aemet.NewMeteoAlarm(meteoAlarmSource(srv.URL, nil), httpx.New()).
		Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}

	var outsideCordoba bool
	for _, r := range records {
		if strings.Contains(r.Title, "Valencia") {
			outsideCordoba = true
		}
	}
	if !outsideCordoba {
		t.Error("warnings outside Cordoba were dropped at ingestion")
	}
}

// TestMeteoAlarmZonesFilterIsOptIn checks the filter works when asked for, by
// EMMA_ID or by a folded substring of the area name.
func TestMeteoAlarmZonesFilterIsOptIn(t *testing.T) {
	t.Parallel()

	srv := serveMeteoAlarm(t)
	records, err := aemet.NewMeteoAlarm(
		meteoAlarmSource(srv.URL, map[string]string{"zones": "subbetica cordobesa"}),
		httpx.New(),
	).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) == 0 {
		t.Fatal("the accent-folded zone name matched nothing")
	}
	for _, r := range records {
		if !strings.Contains(r.Description, "Subbética cordobesa") {
			t.Errorf("record %q leaked past the zone filter", r.Description)
		}
	}
}

// TestMeteoAlarmRejectsRubbish checks a malformed body is an error, not silence.
func TestMeteoAlarmRejectsRubbish(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<<< not xml"))
	}))
	t.Cleanup(srv.Close)

	if _, err := aemet.NewMeteoAlarm(meteoAlarmSource(srv.URL, nil), httpx.New()).
		Poll(context.Background()); err == nil {
		t.Fatal("expected a parse error")
	}
}
