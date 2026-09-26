package ctan_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/FullFran/cordvba/apps/eye/internal/httpx"
	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	"github.com/FullFran/cordvba/apps/eye/internal/provider/infrastructure/ctan"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// serveFixture returns a recorded CTAN answer.
func serveFixture(t *testing.T, name string) *httptest.Server {
	t.Helper()

	body, err := os.ReadFile("../../../../testdata/ctan/" + name) // #nosec G304 -- fixture path built from a test literal
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func ctanSource(url, format string) source.Source {
	return source.Source{
		ID: "ctan-cordoba-notices", Authority: "Consorcio de Transporte Metropolitano del Area de Cordoba",
		Topic: "transport", URL: url, Format: format,
		License: "reuse-authorised-ley-37-2007",
		Access:  source.AccessDocumentedAPI, Automation: source.AutomationEnabled,
		Interval: 30 * time.Minute,
	}
}

func TestNoticesMapsAServiceNotice(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t, "noticias.json")
	records, err := ctan.NewNotices(ctanSource(srv.URL, "ctan-notices"), httpx.New()).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) == 0 {
		t.Fatal("expected records")
	}

	var found bool
	for _, r := range records {
		if r.LocalKey != "161" {
			continue
		}
		found = true

		if r.Kind != "service_notice" {
			t.Errorf("kind = %q, want service_notice", r.Kind)
		}
		if r.Title == "" {
			t.Error("empty title")
		}
		// "Parada no operativa" is a stop being cancelled. It has to
		// outrank a procurement announcement or the board is useless.
		if r.Severity != observation.SeverityModerate {
			t.Errorf("severity = %d, want %d for a cancelled stop", r.Severity, observation.SeverityModerate)
		}
		if r.ValidFrom == nil || r.ValidUntil == nil {
			t.Fatal("a notice with a start and an end date must carry both")
		}
		if !r.ValidFrom.Before(*r.ValidUntil) {
			t.Errorf("valid_from %v is not before valid_until %v", r.ValidFrom, r.ValidUntil)
		}
		if r.Quality != observation.QualityOfficial {
			t.Errorf("quality = %q; the consortium announcing its own service is official", r.Quality)
		}
		if err := r.Validate(); err != nil {
			t.Errorf("record invalid: %v", err)
		}
	}
	if !found {
		t.Fatal("notice 161 did not survive mapping")
	}
}

// TestNoticesRanksCategoriesApart checks the severity mapping is a mapping and
// not a constant: a competition announcement is not a cancelled stop.
func TestNoticesRanksCategoriesApart(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t, "noticias.json")
	records, err := ctan.NewNotices(ctanSource(srv.URL, "ctan-notices"), httpx.New()).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}

	seen := map[observation.Severity]bool{}
	for _, r := range records {
		seen[r.Severity] = true
	}
	if len(seen) < 2 {
		t.Errorf("every notice got severity %v; the category is being ignored", seen)
	}
}

// TestNoticesKeepsBothTimestampsApart is the invariant the whole project rests
// on: when the consortium said it, and when eye read it, are different facts.
func TestNoticesKeepsBothTimestampsApart(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t, "noticias.json")
	records, err := ctan.NewNotices(ctanSource(srv.URL, "ctan-notices"), httpx.New()).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	for _, r := range records {
		if r.ObservedAt.IsZero() || r.FetchedAt.IsZero() {
			t.Fatalf("record %s has a zero timestamp", r.ID)
		}
		if r.Provenance.License != "reuse-authorised-ley-37-2007" {
			t.Errorf("license = %q; CTAN's authorisation requires attribution", r.Provenance.License)
		}
		if r.Provenance.RawHash == "" {
			t.Errorf("record %s cannot be replayed from its evidence", r.ID)
		}
	}
}

func TestStopsMapsCoordinates(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t, "paradas.json")
	src := ctanSource(srv.URL, "ctan-stops")
	src.ID = "ctan-cordoba-stops"
	src.Options = map[string]string{"kind": "bus_stop"}

	entities, err := ctan.NewStops(src, httpx.New()).Entities(context.Background())
	if err != nil {
		t.Fatalf("Entities() = %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("expected entities")
	}

	for _, e := range entities {
		if e.Position == nil {
			t.Fatalf("stop %s has no position; coordinates are the only reason to read this endpoint", e.ID)
		}
		if !e.Position.Valid() {
			t.Errorf("stop %s position outside WGS84", e.ID)
		}
		if e.Kind != "bus_stop" {
			t.Errorf("kind = %q, want bus_stop", e.Kind)
		}
		if err := e.Validate(); err != nil {
			t.Errorf("entity invalid: %v", err)
		}
	}
}

// TestSurfacesARefusal checks a server error is never swallowed into an empty
// poll that reads as "the consortium has announced nothing".
func TestSurfacesARefusal(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	if _, err := ctan.NewNotices(ctanSource(srv.URL, "ctan-notices"), httpx.New()).Poll(context.Background()); err == nil {
		t.Fatal("expected an error on 503")
	}
}

// TestRejectsAMalformedBody checks rubbish is an error, not silence.
func TestRejectsAMalformedBody(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html>not json</html>"))
	}))
	t.Cleanup(srv.Close)

	if _, err := ctan.NewNotices(ctanSource(srv.URL, "ctan-notices"), httpx.New()).Poll(context.Background()); err == nil {
		t.Fatal("expected a parse error")
	}
}
