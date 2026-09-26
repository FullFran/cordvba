package aucorsa

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FullFran/cordvba/apps/eye/internal/httpx"
	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// testNonce stands in for the value AUCORSA prints on its public pages. It is
// a CSRF nonce, not a credential: see the note on tokenPattern.
const testNonce = "a7559dc20c" // #nosec G101 -- a CSRF nonce fixture, not a credential

// operator stands in for AUCORSA: it prints a token on its public page and
// refuses every call that does not carry it, which is the behaviour this
// adapter exists to cope with.
type operator struct {
	token     string
	arrivals  string
	directory string

	pageReads atomic.Int32
	refuse    atomic.Bool
}

func (o *operator) serve(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/page", func(w http.ResponseWriter, _ *http.Request) {
		o.pageReads.Add(1)
		_, _ = w.Write([]byte(`<script>var ajax_vars = {"ajax_nonce":"` + o.token + `"};</script>`))
	})

	// Both data routes answer 200 whatever happens, exactly as the real one
	// does; refusal is in the body.
	answer := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if o.refuse.Load() || r.URL.Query().Get("_wpnonce") != o.token {
				_, _ = w.Write([]byte("-1"))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}
	}
	mux.HandleFunc("/estimations/stop", answer(o.arrivals))
	mux.HandleFunc("/autocompletion/stop", answer(o.directory))

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// arrivalsSource builds the registry entry as the real one is declared.
func arrivalsSource(base, stops string) source.Source {
	return source.Source{
		ID: "aucorsa-arrivals", Authority: "AUCORSA", Topic: "transport",
		URL: base, Format: "aucorsa-arrivals", License: "unspecified",
		Access: source.AccessUndocumentedPersonal, Automation: source.AutomationEnabled,
		Interval: time.Minute,
		Options:  map[string]string{"stops": stops, "token_page": base + "/page"},
		Notes:    "test entry",
	}
}

const oneArrival = `"<div class=\"ppp-content\"><div class=\"ppp-stop-label\">Parada 116: Ronda Tejares<\/div>` +
	`<div class=\"ppp-container\"><div class=\"ppp-line-number\">6<\/div>` +
	`<div class=\"ppp-line-route\">SANTA ROSA<\/div>` +
	`<div class=\"ppp-estimation\"><strong>4 minutos<\/strong>` +
	`<img class=\"imgocupationbus\" alt=\"Ocupaci&oacute;n Baja\"><\/div><\/div>` +
	`<div class=\"ppp-favorited\"><\/div><\/div>"`

func TestPollReadsTheTokenThenTheArrivals(t *testing.T) {
	t.Parallel()

	op := &operator{token: testNonce, arrivals: oneArrival}
	srv := op.serve(t)
	p := NewRealtime(arrivalsSource(srv.URL, "116"), httpx.New())

	records, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}

	r := records[0]
	if !strings.Contains(r.Title, "Línea 6") || !strings.Contains(r.Title, "4 min") {
		t.Errorf("title = %q", r.Title)
	}
	// An estimate is the operator's prediction. Recording it as anything
	// firmer would misrepresent what the source actually knows.
	if r.Quality != observation.QualityPreliminary {
		t.Errorf("quality = %v, want preliminary", r.Quality)
	}
	// The bus is due in the future; the reading happened now.
	if r.ValidFrom == nil || !r.ValidFrom.After(r.ObservedAt) {
		t.Error("the departure is not recorded as later than the observation")
	}
	if r.ExpiresAt == nil {
		t.Error("an estimate with no expiry would outlive its usefulness in the store")
	}
	if r.Provenance.RawHash == "" || r.Provenance.Publisher != "AUCORSA" {
		t.Errorf("provenance = %+v", r.Provenance)
	}

	// A second poll must reuse the cached token rather than re-reading the
	// public page on every tick.
	if _, err := p.Poll(context.Background()); err != nil {
		t.Fatalf("second Poll() = %v", err)
	}
	if got := op.pageReads.Load(); got != 1 {
		t.Errorf("public page read %d times, want 1", got)
	}
}

// A refusal answers 200, so it must be surfaced as a failure rather than
// recorded as a stop with no buses.
func TestPollSurfacesARefusal(t *testing.T) {
	t.Parallel()

	op := &operator{token: testNonce, arrivals: oneArrival}
	op.refuse.Store(true)
	srv := op.serve(t)
	p := NewRealtime(arrivalsSource(srv.URL, "116,456"), httpx.New())

	records, err := p.Poll(context.Background())
	if err == nil {
		t.Fatalf("Poll() = %d records, nil error; want a refusal", len(records))
	}
	if !strings.Contains(err.Error(), "refused") {
		t.Errorf("error = %v, want it to name the refusal", err)
	}

	// The cached token is dropped, so the next attempt re-reads the page
	// rather than repeating a value the endpoint has stopped accepting.
	op.refuse.Store(false)
	if _, err := p.Poll(context.Background()); err != nil {
		t.Fatalf("Poll() after recovery = %v", err)
	}
	if got := op.pageReads.Load(); got != 2 {
		t.Errorf("public page read %d times, want a re-read after the refusal", got)
	}
}

// One unanswering stop must not cost the others their arrivals.
func TestPollKeepsTheStopsThatDoAnswer(t *testing.T) {
	t.Parallel()

	// One stop refuses; the other answers.
	mux := http.NewServeMux()
	mux.HandleFunc("/page", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ajax_nonce":"a1b2c3d4"}`))
	})
	mux.HandleFunc("/estimations/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("stop_id") == "999" {
			_, _ = w.Write([]byte("-1"))
			return
		}
		_, _ = w.Write([]byte(oneArrival))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	p := NewRealtime(arrivalsSource(srv.URL, "999,116"), httpx.New())
	records, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) != 1 {
		t.Errorf("records = %d, want the one stop that answered", len(records))
	}
}

func TestLookupStopsReadsTheDirectory(t *testing.T) {
	t.Parallel()

	directory, err := json.Marshal([]map[string]string{
		{"id": "456", "label": "Claudio Marcelo (Tendillas) (456)", "link": "https://aucorsa.es/parada/claudio-marcelo-tendillas/"},
		{"id": "271", "label": "Claudio Marcelo (Tendillas) (271)", "link": ""},
	})
	if err != nil {
		t.Fatal(err)
	}

	op := &operator{token: "a1b2c3d4", directory: string(directory)}
	srv := op.serve(t)
	p := NewRealtime(arrivalsSource(srv.URL, ""), httpx.New())

	stops, err := p.LookupStops(context.Background(), "tendillas")
	if err != nil {
		t.Fatalf("LookupStops() = %v", err)
	}
	if len(stops) != 2 {
		t.Fatalf("stops = %d, want 2", len(stops))
	}
	if stops[0].ID != "456" {
		t.Errorf("id = %q, want 456", stops[0].ID)
	}
	// The directory repeats the number inside the label; carrying it into the
	// name would print it twice.
	if stops[0].Name != "Claudio Marcelo (Tendillas)" {
		t.Errorf("name = %q, want the number stripped", stops[0].Name)
	}
}

func TestLookupStopsSurfacesARefusal(t *testing.T) {
	t.Parallel()

	op := &operator{token: "a1b2c3d4", directory: `[]`}
	op.refuse.Store(true)
	srv := op.serve(t)
	p := NewRealtime(arrivalsSource(srv.URL, ""), httpx.New())

	if _, err := p.LookupStops(context.Background(), "tendillas"); err == nil {
		t.Fatal("LookupStops() accepted a refusal as an empty directory")
	}
}

func TestLookupStopsRefusesAnEmptyTerm(t *testing.T) {
	t.Parallel()

	p := NewRealtime(arrivalsSource("http://127.0.0.1:0", ""), httpx.New())
	if _, err := p.LookupStops(context.Background(), "   "); err == nil {
		t.Fatal("an empty search term should not reach the network")
	}
}

// A source that watches no stops is a configuration mistake, and saying so is
// more useful than polling nothing and reporting success.
func TestPollWithoutStops(t *testing.T) {
	t.Parallel()

	p := NewRealtime(arrivalsSource("http://127.0.0.1:0", ""), httpx.New())
	if _, err := p.Poll(context.Background()); err == nil {
		t.Fatal("Poll() succeeded with no stops configured")
	}
}
