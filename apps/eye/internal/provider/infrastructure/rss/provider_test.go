package rss_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/FullFran/cordvba/apps/eye/internal/httpx"
	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	"github.com/FullFran/cordvba/apps/eye/internal/provider/infrastructure/rss"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// serveFixture starts a server returning a recorded feed.
func serveFixture(t *testing.T, name, contentType string) *httptest.Server {
	t.Helper()

	body, err := os.ReadFile("../../../../testdata/rss/" + name) // #nosec G304 -- fixture path built from a test literal
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"cached"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("ETag", `"cached"`)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// pressSource builds a registry entry pointing at a test server.
func pressSource(url, topic string) source.Source {
	return source.Source{
		ID: "test-feed", Authority: "Diario Cordoba", Topic: topic, URL: url,
		Format: "rss", License: "unspecified",
		Access: source.AccessDocumentedAPI, Automation: source.AutomationEnabled, Interval: time.Minute,
	}
}

func TestPollNormalizesNews(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t, "diariocordoba.xml", "application/rss+xml; charset=utf-8")
	p := rss.New(pressSource(srv.URL, "press"), httpx.New())

	records, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) == 0 {
		t.Fatal("expected records")
	}

	r := records[0]
	if r.Kind != "news_item" {
		t.Errorf("kind = %q, want news_item", r.Kind)
	}
	if r.Provenance.Publisher != "Diario Cordoba" {
		t.Errorf("publisher = %q", r.Provenance.Publisher)
	}
	if r.Provenance.RawHash == "" {
		t.Error("record has no raw hash; the evidence chain is broken")
	}
	// For news, the source's own timestamp is the observation time.
	if r.ObservedAt.Equal(r.FetchedAt) {
		t.Error("ObservedAt equals FetchedAt; the feed date was not used")
	}
	if err := r.Validate(); err != nil {
		t.Errorf("record does not validate: %v", err)
	}
}

// In an event feed, pubDate is the event start, not the publication moment.
// Mapping it onto ObservedAt would date every announcement in the future.
func TestPollMapsEventStartToValidFrom(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t, "uco-eventos.rss", "application/rss+xml; charset=utf-8")
	p := rss.New(pressSource(srv.URL, "events"), httpx.New())

	records, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) == 0 {
		t.Fatal("expected records")
	}

	var withStart int
	for _, r := range records {
		if r.Kind != "event_listing" {
			t.Fatalf("kind = %q, want event_listing", r.Kind)
		}
		if !r.ObservedAt.Equal(r.FetchedAt) {
			t.Error("for an event, ObservedAt should be when we learned of it")
		}
		if r.ValidFrom != nil {
			withStart++
		}
	}
	if withStart == 0 {
		t.Error("no event carried a start time in ValidFrom")
	}
}

func TestPollHandlesNotModified(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t, "diariocordoba.xml", "application/rss+xml; charset=utf-8")
	p := rss.New(pressSource(srv.URL, "press"), httpx.New())

	if _, err := p.Poll(context.Background()); err != nil {
		t.Fatalf("first Poll() = %v", err)
	}

	// The second poll sends the stored ETag and must treat 304 as a
	// successful, empty poll rather than as a failure.
	records, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("second Poll() = %v, want nil (304 is healthy)", err)
	}
	if len(records) != 0 {
		t.Errorf("second Poll() returned %d records, want 0", len(records))
	}
}

func TestPollIsStableAcrossCalls(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("../../../../testdata/rss/diariocordoba.xml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	p := rss.New(pressSource(srv.URL, "press"), httpx.New())

	first, _ := p.Poll(context.Background())
	second, _ := p.Poll(context.Background())

	if len(first) != len(second) {
		t.Fatalf("polls returned %d then %d records", len(first), len(second))
	}
	// Unstable ids would make the same article accumulate on every poll.
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Fatalf("record id changed between polls: %q then %q", first[i].ID, second[i].ID)
		}
	}
}

func TestPollReportsBadFeed(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>404</body></html>"))
	}))
	defer srv.Close()

	if _, err := rss.New(pressSource(srv.URL, "press"), httpx.New()).Poll(context.Background()); err == nil {
		t.Fatal("Poll() on a non-feed = nil error")
	}
}

func TestPayloadCarriesTheArticleLink(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t, "diariocordoba.xml", "application/rss+xml; charset=utf-8")
	records, err := rss.New(pressSource(srv.URL, "press"), httpx.New()).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(records[0].Payload, &payload); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	if link, _ := payload["link"].(string); link == "" {
		t.Error("payload carries no article link")
	}
}

func TestInfoReturnsTheRegistryEntry(t *testing.T) {
	t.Parallel()

	s := pressSource("https://example.org/rss", "press")
	if got := rss.New(s, httpx.New()).Info(); got.ID != s.ID {
		t.Errorf("Info().ID = %q, want %q", got.ID, s.ID)
	}
}

var _ = observation.Record{}

// optionSource builds a registry entry that carries adapter options.
func optionSource(url, topic string, options map[string]string) source.Source {
	src := pressSource(url, topic)
	src.Authority = "Instituto Geografico Nacional"
	src.Options = options
	return src
}

// TestPollHonoursTheRegistryKind checks that the registry, not the adapter,
// decides what a feed's entries are called. An IGN earthquake filed as a
// "news_item" is unfindable by anything looking for seismic events.
func TestPollHonoursTheRegistryKind(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t, "ign-sismologia.xml", "application/rss+xml; charset=utf-8")
	src := optionSource(srv.URL, "geophysics", map[string]string{"kind": "seismic_event"})

	records, err := rss.New(src, httpx.New()).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) == 0 {
		t.Fatal("expected records")
	}
	for _, r := range records {
		if r.Kind != "seismic_event" {
			t.Fatalf("kind = %q, want seismic_event", r.Kind)
		}
	}
}

// TestPollScalesSeverityByMagnitude checks that a magnitude 3.1 earthquake does
// not rank identically to every press headline in the store. Severity is opt-in
// per source, because only a feed that states a magnitude can be read this way.
func TestPollScalesSeverityByMagnitude(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t, "ign-sismologia.xml", "application/rss+xml; charset=utf-8")
	src := optionSource(srv.URL, "geophysics", map[string]string{
		"kind": "seismic_event", "severity": "magnitude",
	})

	records, err := rss.New(src, httpx.New()).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}

	var found bool
	for _, r := range records {
		var payload struct {
			Magnitude *float64 `json:"magnitude"`
		}
		if err := json.Unmarshal(r.Payload, &payload); err != nil {
			t.Fatalf("payload: %v", err)
		}
		if payload.Magnitude == nil {
			continue
		}
		found = true

		want := observation.SeverityInfo
		switch m := *payload.Magnitude; {
		case m >= 5:
			want = observation.SeverityCritical
		case m >= 4:
			want = observation.SeverityHigh
		case m >= 3:
			want = observation.SeverityModerate
		case m >= 2:
			want = observation.SeverityLow
		}
		if r.Severity != want {
			t.Errorf("magnitude %v: severity = %d, want %d", *payload.Magnitude, r.Severity, want)
		}
	}
	if !found {
		t.Fatal("no record carried a magnitude; the fixture states one for every earthquake")
	}
}

// TestPollLeavesSeverityAloneWithoutTheOption checks the behaviour stays opt-in:
// a press feed must not have its severity guessed from whatever number happens
// to appear in a headline.
func TestPollLeavesSeverityAloneWithoutTheOption(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t, "ign-sismologia.xml", "application/rss+xml; charset=utf-8")
	src := optionSource(srv.URL, "geophysics", nil)

	records, err := rss.New(src, httpx.New()).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) == 0 {
		t.Fatal("expected records")
	}
	for _, r := range records {
		if r.Severity != observation.SeverityInfo {
			t.Errorf("severity = %d, want %d without the option", r.Severity, observation.SeverityInfo)
		}
	}
}
