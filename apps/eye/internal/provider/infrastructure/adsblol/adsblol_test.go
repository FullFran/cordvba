package adsblol_test

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
	"github.com/FullFran/cordvba/apps/eye/internal/provider/infrastructure/adsblol"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// viewportSource builds a registry entry pointing at a test server.
func viewportSource(url string, opts map[string]string) source.Source {
	if opts == nil {
		opts = map[string]string{"lat": "37.8882", "lon": "-4.7794", "radius_nm": "60"}
	}
	return source.Source{
		ID: "adsb-lol", Authority: "adsb.lol", Topic: "air", URL: url,
		Format: "adsb-json", License: "check-terms",
		Access: source.AccessDocumentedAPI, Automation: source.AutomationEnabled,
		Interval: 15 * time.Second, Options: opts,
	}
}

// serveViewport returns the recorded viewport payload.
func serveViewport(t *testing.T) (*httptest.Server, *string) {
	t.Helper()

	body, err := os.ReadFile("../../../../testdata/adsblol/viewport.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, &path
}

func TestPollBuildsTheViewportPath(t *testing.T) {
	t.Parallel()

	srv, path := serveViewport(t)
	if _, err := adsblol.New(viewportSource(srv.URL, nil), httpx.New()).Poll(context.Background()); err != nil {
		t.Fatalf("Poll() = %v", err)
	}

	if want := "/v2/lat/37.8882/lon/-4.7794/dist/60"; *path != want {
		t.Errorf("requested %q, want %q", *path, want)
	}
}

func TestPollSkipsAircraftWithNoPosition(t *testing.T) {
	t.Parallel()

	srv, _ := serveViewport(t)
	records, err := adsblol.New(viewportSource(srv.URL, nil), httpx.New()).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}

	// The fixture has four aircraft; one sits at 0,0 with no real fix.
	if len(records) != 3 {
		t.Fatalf("records = %d, want 3", len(records))
	}
	for _, r := range records {
		if r.Position == nil {
			t.Error("record with no position was kept")
		}
		if strings.Contains(r.Title, "GROUND") {
			t.Error("the positionless aircraft was not skipped")
		}
	}
}

// Movement data is short-lived by design, and the mechanism is the field, not
// an intention written in a document.
func TestPollSetsExpiry(t *testing.T) {
	t.Parallel()

	srv, _ := serveViewport(t)
	src := viewportSource(srv.URL, map[string]string{
		"lat": "37.8882", "lon": "-4.7794", "radius_nm": "60", "ttl": "24h",
	})

	records, err := adsblol.New(src, httpx.New()).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}

	for _, r := range records {
		if r.ExpiresAt == nil {
			t.Fatal("an aircraft position was stored with no expiry")
		}
		got := r.ExpiresAt.Sub(r.FetchedAt)
		if got < 23*time.Hour || got > 25*time.Hour {
			t.Errorf("ttl = %v, want about 24h", got)
		}
	}
}

// seen_pos is how long ago the position was received. Using fetch time as the
// observation time would advertise a stale fix as live.
func TestPollUsesPositionAgeAsObservationTime(t *testing.T) {
	t.Parallel()

	srv, _ := serveViewport(t)
	records, _ := adsblol.New(viewportSource(srv.URL, nil), httpx.New()).Poll(context.Background())

	var found bool
	for _, r := range records {
		if !strings.HasPrefix(r.Title, "EXS32TE") {
			continue
		}
		found = true
		if latency := r.Latency(); latency < 12*time.Second || latency > 13*time.Second {
			t.Errorf("latency = %v, want about 12.4s from seen_pos", latency)
		}
	}
	if !found {
		t.Error("the aircraft with a known position age was not returned")
	}
}

func TestPollFlagsEmergency(t *testing.T) {
	t.Parallel()

	srv, _ := serveViewport(t)
	records, _ := adsblol.New(viewportSource(srv.URL, nil), httpx.New()).Poll(context.Background())

	for _, r := range records {
		if strings.HasPrefix(r.Title, "MAYDAY1") {
			if r.Severity != observation.SeverityHigh {
				t.Errorf("severity = %d, want high for a declared emergency", r.Severity)
			}
			return
		}
	}
	t.Error("the emergency aircraft was not returned")
}

func TestPollKeepsRawFieldsInPayload(t *testing.T) {
	t.Parallel()

	srv, _ := serveViewport(t)
	records, _ := adsblol.New(viewportSource(srv.URL, nil), httpx.New()).Poll(context.Background())

	var payload map[string]any
	if err := json.Unmarshal(records[0].Payload, &payload); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	for _, key := range []string{"hex", "callsign", "registration", "aircraft_type", "track"} {
		if _, ok := payload[key]; !ok {
			t.Errorf("payload is missing %q", key)
		}
	}
}

func TestPollRequiresAViewport(t *testing.T) {
	t.Parallel()

	srv, _ := serveViewport(t)
	src := viewportSource(srv.URL, map[string]string{"radius_nm": "60"})

	if _, err := adsblol.New(src, httpx.New()).Poll(context.Background()); err == nil {
		t.Fatal("Poll() without lat/lon = nil error")
	}
}

func TestPollReportsBadPayload(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	if _, err := adsblol.New(viewportSource(srv.URL, nil), httpx.New()).Poll(context.Background()); err == nil {
		t.Fatal("Poll() on a bad payload = nil error")
	}
}
