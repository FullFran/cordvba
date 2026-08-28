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

	"github.com/FullFran/eye/internal/httpx"
	"github.com/FullFran/eye/internal/provider/infrastructure/datex"
	source "github.com/FullFran/eye/internal/source/domain"
)

// serveFixture returns the recorded DevicePublication.
func serveFixture(t *testing.T) *httptest.Server {
	t.Helper()

	body, err := os.ReadFile("../../../../testdata/datex/devices.xml") // #nosec G304 -- fixture path is a literal
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

// deviceSource builds a registry entry pointing at a test server.
func deviceSource(url string) source.Source {
	return source.Source{
		ID: "dgt-cameras", Authority: "Direccion General de Trafico", Topic: "transport",
		URL: url, Format: "datex2-devices", License: "free-of-charge-nap-terms",
		Access: source.AccessDocumentedAPI, Automation: source.AutomationEnabled, Interval: time.Hour,
	}
}

func TestEntitiesParsesRealFeed(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t)
	entities, err := datex.NewDevices(deviceSource(srv.URL), httpx.New()).Entities(context.Background())
	if err != nil {
		t.Fatalf("Entities() = %v", err)
	}
	if len(entities) == 0 {
		t.Fatal("no cameras parsed")
	}

	e := entities[0]
	if e.Kind != "camera" {
		t.Errorf("kind = %q", e.Kind)
	}
	if e.Position == nil {
		t.Fatal("camera has no position")
	}
	// Inside Spain, generously bounded.
	if e.Position.Lat < 27 || e.Position.Lat > 44 {
		t.Errorf("latitude %.4f is outside Spain", e.Position.Lat)
	}
	if !strings.Contains(e.Title, "km") {
		t.Errorf("title %q does not carry the kilometre point", e.Title)
	}
	if err := e.Validate(); err != nil {
		t.Errorf("entity does not validate: %v", err)
	}
}

// The image URL is stored as a pointer to where a frame can be fetched. eye
// never fetches or stores frames while polling.
func TestEntitiesCarryTheImageURLWithoutFetchingIt(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t)
	entities, err := datex.NewDevices(deviceSource(srv.URL), httpx.New()).Entities(context.Background())
	if err != nil {
		t.Fatalf("Entities() = %v", err)
	}

	var payload struct {
		ImageURL string `json:"image_url"`
		Road     string `json:"road"`
		Province string `json:"province"`
	}
	if err := json.Unmarshal(entities[0].Payload, &payload); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}

	if !strings.HasSuffix(payload.ImageURL, ".jpg") {
		t.Errorf("image_url = %q, want a JPEG endpoint", payload.ImageURL)
	}
	if payload.Road == "" || payload.Province == "" {
		t.Errorf("road metadata is missing: %+v", payload)
	}
}

func TestPollSummarizesTheInventory(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t)
	records, err := datex.NewDevices(deviceSource(srv.URL), httpx.New()).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want a single inventory summary", len(records))
	}
	if records[0].Kind != "inventory_refresh" {
		t.Errorf("kind = %q", records[0].Kind)
	}
	if records[0].Provenance.RawHash == "" {
		t.Error("no raw hash; the evidence chain is broken")
	}
}

func TestEntitiesHandlesNotModified(t *testing.T) {
	t.Parallel()

	srv := serveFixture(t)
	p := datex.NewDevices(deviceSource(srv.URL), httpx.New())

	if _, err := p.Entities(context.Background()); err != nil {
		t.Fatalf("first Entities() = %v", err)
	}
	// A 304 is a healthy, empty result — not a failure.
	got, err := p.Entities(context.Background())
	if err != nil {
		t.Fatalf("second Entities() = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("second Entities() returned %d entities, want 0", len(got))
	}
}

// A schema addition by DGT must not take the provider down.
func TestEntitiesIgnoresUnknownElements(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<d2:payload xmlns:d2="d" xmlns:ns2="n" xmlns:loc="l" xmlns:fse="f" xmlns:xsi="x">
  <ns2:somethingBrandNew><ns2:nested>ignore me</ns2:nested></ns2:somethingBrandNew>
  <ns2:device id="1">
    <ns2:typeOfDevice>camera</ns2:typeOfDevice>
    <ns2:futureField>whatever</ns2:futureField>
    <ns2:pointLocation><loc:tpegPointLocation><loc:point><loc:pointCoordinates>
      <loc:latitude>37.89</loc:latitude><loc:longitude>-4.74</loc:longitude>
    </loc:pointCoordinates></loc:point></loc:tpegPointLocation></ns2:pointLocation>
    <fse:deviceUrl>https://example.org/1.jpg</fse:deviceUrl>
  </ns2:device>
</d2:payload>`))
	}))
	defer srv.Close()

	entities, err := datex.NewDevices(deviceSource(srv.URL), httpx.New()).Entities(context.Background())
	if err != nil {
		t.Fatalf("Entities() = %v", err)
	}
	if len(entities) != 1 {
		t.Fatalf("entities = %d, want 1", len(entities))
	}
}

func TestEntitiesSkipsNonCamerasAndBadPositions(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<d2:payload xmlns:d2="d" xmlns:ns2="n" xmlns:loc="l" xmlns:fse="f" xmlns:xsi="x">
  <ns2:device id="1"><ns2:typeOfDevice>vms</ns2:typeOfDevice>
    <ns2:pointLocation><loc:tpegPointLocation><loc:point><loc:pointCoordinates>
      <loc:latitude>37.89</loc:latitude><loc:longitude>-4.74</loc:longitude>
    </loc:pointCoordinates></loc:point></loc:tpegPointLocation></ns2:pointLocation></ns2:device>
  <ns2:device id="2"><ns2:typeOfDevice>camera</ns2:typeOfDevice>
    <ns2:pointLocation><loc:tpegPointLocation><loc:point><loc:pointCoordinates>
      <loc:latitude>0</loc:latitude><loc:longitude>0</loc:longitude>
    </loc:pointCoordinates></loc:point></loc:tpegPointLocation></ns2:pointLocation></ns2:device>
  <ns2:device id="3"><ns2:typeOfDevice>camera</ns2:typeOfDevice>
    <ns2:pointLocation><loc:tpegPointLocation><loc:point><loc:pointCoordinates>
      <loc:latitude>37.89</loc:latitude><loc:longitude>-4.74</loc:longitude>
    </loc:pointCoordinates></loc:point></loc:tpegPointLocation></ns2:pointLocation>
    <fse:deviceUrl>https://example.org/3.jpg</fse:deviceUrl></ns2:device>
</d2:payload>`))
	}))
	defer srv.Close()

	entities, err := datex.NewDevices(deviceSource(srv.URL), httpx.New()).Entities(context.Background())
	if err != nil {
		t.Fatalf("Entities() = %v", err)
	}

	// The VMS panel and the null-island camera are both dropped.
	if len(entities) != 1 {
		t.Fatalf("entities = %d, want 1", len(entities))
	}
	if !strings.HasSuffix(entities[0].ID, ":3") {
		t.Errorf("kept the wrong device: %s", entities[0].ID)
	}
}

func TestPollReportsAnUnreachableSource(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	if _, err := datex.NewDevices(deviceSource(srv.URL), httpx.New()).Poll(context.Background()); err == nil {
		t.Fatal("Poll() against a failing source = nil error")
	}
}

func TestInfoReturnsTheRegistryEntry(t *testing.T) {
	t.Parallel()

	s := deviceSource("https://example.org/devices.xml")
	if got := datex.NewDevices(s, httpx.New()).Info(); got.ID != s.ID {
		t.Errorf("Info().ID = %q", got.ID)
	}
}
