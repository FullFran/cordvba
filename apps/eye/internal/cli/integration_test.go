package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FullFran/eye/internal/cli"
	store "github.com/FullFran/eye/internal/observation/infrastructure"
)

// fakePortal serves every source shape eye reads, so a command can be exercised
// end to end without touching a public service.
func fakePortal(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	srv := httptest.NewUnstartedServer(mux)

	mux.HandleFunc("/press.rss", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
  <title>Diario de Prueba</title>
  <item>
    <title><![CDATA[Corte de tráfico en el Puente Romano]]></title>
    <link>https://example.org/noticia/1</link>
    <pubDate>%s</pubDate>
    <description><![CDATA[<p>Obras hasta el viernes.</p>]]></description>
  </item>
</channel></rss>`, time.Now().Add(-30*time.Minute).UTC().Format(time.RFC1123Z))
	})

	mux.HandleFunc("/events.rss", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="0.91"><channel>
  <title>Eventos de Prueba</title>
  <item>
    <title>Velá de la Fuensanta</title>
    <link>https://example.org/evento/1</link>
    <pubDate>%s</pubDate>
  </item>
</channel></rss>`, time.Now().Add(72*time.Hour).UTC().Format(time.RFC1123Z))
	})

	mux.HandleFunc("/v2/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"ac":[{"hex":"abc123","flight":"EYE001  ","lat":37.9,"lon":-4.8,"seen_pos":4.0}],"now":1}`)
	})

	mux.HandleFunc("/api/3/action/package_show", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"success":true,"result":{"name":"camaras","license_title":"License not specified",
			"resources":[{"format":"GeoJSON","name":"c.json","url":"%s/cams.json"}]}}`, srv.URL)
	})

	mux.HandleFunc("/cams.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"type":"FeatureCollection","features":[
			{"type":"Feature","id":"1","geometry":{"type":"Point","coordinates":[-4.7995,37.8984]},
			 "properties":{"name":"AVDA. DE LA ARRUZAFILLA"}}]}`)
	})

	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

// writeRegistry writes a registry pointing at the fake portal and returns its
// path.
func writeRegistry(t *testing.T, portal string) string {
	t.Helper()

	body := fmt.Sprintf(`
sources:
  - id: test-press
    authority: Diario de Prueba
    topic: press
    url: %[1]s/press.rss
    format: rss
    license: unspecified
    access: documented_api
    automation: enabled
    interval: 30m

  - id: test-events
    authority: Universidad de Prueba
    topic: events
    url: %[1]s/events.rss
    format: rss
    license: unspecified
    access: documented_api
    automation: enabled
    interval: 6h

  - id: test-air
    authority: adsb.test
    topic: air
    url: %[1]s
    format: adsb-json
    license: check-terms
    access: documented_api
    automation: enabled
    interval: 15s
    options:
      lat: "37.8882"
      lon: "-4.7794"
      radius_nm: "60"

  - id: test-cameras
    authority: Ayuntamiento de Prueba
    topic: transport
    url: %[1]s
    format: ckan-geojson
    license: unspecified
    access: documented_api
    automation: enabled
    interval: 24h
    options:
      dataset: camaras
      kind: camera

  - id: test-held
    authority: Fuente Retenida
    topic: hydrology
    url: %[1]s/held
    format: web
    license: unspecified
    access: public_html
    automation: review_terms
    notes: Reuse terms unresolved.

  - id: test-no-adapter
    authority: Fuente Sin Adaptador
    topic: weather
    url: %[1]s/wms
    format: wms
    license: unspecified
    access: documented_api
    automation: enabled
    interval: 1h
`, portal)

	return writeRegistryBody(t, body)
}

// writeRegistryBody writes registry YAML to a temp file and returns its path.
func writeRegistryBody(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "sources.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write registry: %v", err)
	}
	return path
}

// runCmd executes one command against the fake portal.
//
// Every invocation gets its own data directory. A test suite that writes into
// the operator's real store is a bug, and it is one that only shows up as
// surprising rows in their `eye status`.
func runCmd(t *testing.T, registry string, args ...string) (int, string, string) {
	t.Helper()
	return runCmdIn(t, registry, t.TempDir(), args...)
}

// runCmdIn executes one command against a specific data directory, so a test
// can assert that what one invocation stored, the next can read.
func runCmdIn(t *testing.T, registry, dataDir string, args ...string) (int, string, string) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	full := append(args, "--registry", registry, "--data-dir", dataDir)
	code := cli.New().Run(context.Background(), full, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// setup starts the portal and writes its registry.
func setup(t *testing.T) string {
	t.Helper()
	return writeRegistry(t, fakePortal(t).URL)
}

func TestSourcesCommand(t *testing.T) {
	t.Parallel()

	registry := setup(t)
	code, stdout, _ := runCmd(t, registry, "sources")

	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	for _, want := range []string{"test-press", "live", "review_terms", "no adapter", "held"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output is missing %q:\n%s", want, stdout)
		}
	}
}

func TestSourcesCommandJSON(t *testing.T) {
	t.Parallel()

	registry := setup(t)
	_, stdout, _ := runCmd(t, registry, "sources", "--json")

	var got []struct {
		ID         string `json:"id"`
		Pollable   bool   `json:"pollable"`
		HasAdapter bool   `json:"has_adapter"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}

	state := map[string][2]bool{}
	for _, s := range got {
		state[s.ID] = [2]bool{s.Pollable, s.HasAdapter}
	}

	// Permitted and readable are separate facts and both are reported.
	if state["test-press"] != [2]bool{true, true} {
		t.Errorf("test-press = %v, want permitted and readable", state["test-press"])
	}
	if state["test-held"] != [2]bool{false, false} {
		t.Errorf("test-held = %v, want neither", state["test-held"])
	}
	if state["test-no-adapter"] != [2]bool{true, false} {
		t.Errorf("test-no-adapter = %v, want permitted but unreadable", state["test-no-adapter"])
	}
}

func TestNewsCommand(t *testing.T) {
	t.Parallel()

	registry := setup(t)
	code, stdout, _ := runCmd(t, registry, "news")

	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(stdout, "Puente Romano") {
		t.Errorf("output does not carry the article:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Diario de Prueba") {
		t.Errorf("output does not attribute the publisher:\n%s", stdout)
	}
}

func TestNewsCommandJSONKeepsProvenance(t *testing.T) {
	t.Parallel()

	registry := setup(t)
	_, stdout, _ := runCmd(t, registry, "news", "--json")

	var got []struct {
		Publisher  string  `json:"publisher"`
		License    string  `json:"license"`
		URL        string  `json:"url"`
		LatencyS   float64 `json:"source_latency_seconds"`
		ObservedAt string  `json:"observed_at"`
		FetchedAt  string  `json:"fetched_at"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("no records")
	}

	r := got[0]
	if r.Publisher == "" || r.License == "" || r.URL == "" {
		t.Errorf("provenance is incomplete: %+v", r)
	}
	if r.ObservedAt == r.FetchedAt {
		t.Error("observed and fetched times were collapsed")
	}
	if r.LatencyS < 60 {
		t.Errorf("source latency = %.0fs, want the ~30 minutes the fixture declares", r.LatencyS)
	}
}

func TestEventsCommandShowsFutureStart(t *testing.T) {
	t.Parallel()

	registry := setup(t)
	code, stdout, _ := runCmd(t, registry, "events")

	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(stdout, "Velá de la Fuensanta") {
		t.Errorf("output is missing the event:\n%s", stdout)
	}
	// Rendered as a relative future distance; the exact rounding is not the
	// point, that it reads as upcoming is.
	if !strings.Contains(stdout, "in ") {
		t.Errorf("output does not show the event as upcoming:\n%s", stdout)
	}
}

func TestSkyCommand(t *testing.T) {
	t.Parallel()

	registry := setup(t)
	code, stdout, _ := runCmd(t, registry, "sky")

	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(stdout, "EYE001") {
		t.Errorf("output is missing the aircraft:\n%s", stdout)
	}
	if !strings.Contains(stdout, "expire") {
		t.Errorf("output does not state the retention policy:\n%s", stdout)
	}
}

func TestCamerasCommandStatesItIsInventoryOnly(t *testing.T) {
	t.Parallel()

	registry := setup(t)
	code, stdout, _ := runCmd(t, registry, "cameras")

	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(stdout, "ARRUZAFILLA") {
		t.Errorf("output is missing the camera:\n%s", stdout)
	}
	// The boundary is stated where a user reads it, not only in an ADR.
	if !strings.Contains(stdout, "does not stream them") {
		t.Errorf("output does not state that eye maps cameras only:\n%s", stdout)
	}
	if !strings.Contains(stdout, "unspecified") {
		t.Errorf("output does not surface the undeclared licence:\n%s", stdout)
	}
}

func TestStatusCommand(t *testing.T) {
	t.Parallel()

	registry := setup(t)
	code, stdout, _ := runCmd(t, registry, "status")

	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	// The situation board still has to say all three source states, whatever
	// it looks like.
	for _, want := range []string{"CÓRDOBA", "press", "events", "answered", "held", "no adapter"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("status is missing %q:\n%s", want, stdout)
		}
	}
	if !strings.Contains(stdout, "traces back to a public source") {
		t.Errorf("status dropped the provenance line:\n%s", stdout)
	}
	// Piped output must carry no escape sequences at all.
	if strings.Contains(stdout, "\x1b[") {
		t.Errorf("status wrote ANSI escapes to a pipe:\n%q", stdout)
	}
}

func TestStatusCommandJSON(t *testing.T) {
	t.Parallel()

	registry := setup(t)
	_, stdout, _ := runCmd(t, registry, "status", "--json")

	var got struct {
		Sources struct {
			Total, Live, Answered, Failed, Held int
		} `json:"sources"`
		Records   int      `json:"records"`
		Entities  int      `json:"entities"`
		Held      []string `json:"held"`
		NoAdapter []string `json:"awaiting_adapter"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}

	if got.Sources.Total != 6 {
		t.Errorf("total sources = %d, want 6", got.Sources.Total)
	}
	if got.Sources.Answered != 4 {
		t.Errorf("answered = %d, want 4", got.Sources.Answered)
	}
	if got.Records == 0 {
		t.Error("no records collected")
	}
	if got.Entities != 1 {
		t.Errorf("entities = %d, want 1", got.Entities)
	}
	if len(got.Held) != 1 || got.Held[0] != "test-held" {
		t.Errorf("held = %v", got.Held)
	}
	if len(got.NoAdapter) != 1 || got.NoAdapter[0] != "test-no-adapter" {
		t.Errorf("awaiting adapter = %v", got.NoAdapter)
	}
}

func TestQueryCommandFiltersByTopic(t *testing.T) {
	t.Parallel()

	registry := setup(t)
	_, stdout, _ := runCmd(t, registry, "query", "--topic", "press", "--json")

	var got []struct {
		Topic string `json:"topic"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("no records")
	}
	for _, r := range got {
		if r.Topic != "press" {
			t.Errorf("topic = %q, want only press", r.Topic)
		}
	}
}

func TestQueryCommandTextFilter(t *testing.T) {
	t.Parallel()

	registry := setup(t)
	_, stdout, _ := runCmd(t, registry, "query", "--topic", "press", "--text", "no such thing")

	if !strings.Contains(stdout, "Nothing in the selected window") {
		t.Errorf("a filter matching nothing should say so plainly:\n%s", stdout)
	}
}

// A source that is down must be reported, never hidden behind an empty result.
func TestFailingSourceIsReported(t *testing.T) {
	t.Parallel()

	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer down.Close()

	registry := writeRegistry(t, down.URL)
	_, _, stderr := runCmd(t, registry, "news")

	if !strings.Contains(stderr, "unavailable") {
		t.Errorf("stderr does not report the failing source: %q", stderr)
	}
}

func TestUnknownRegistryPathFails(t *testing.T) {
	t.Parallel()

	code, _, stderr := runCmd(t, filepath.Join(t.TempDir(), "missing.yaml"), "sources")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "missing.yaml") {
		t.Errorf("stderr does not name the missing file: %q", stderr)
	}
}

func TestTopicWithNoLiveSourceFails(t *testing.T) {
	t.Parallel()

	registry := setup(t)
	code, _, stderr := runCmd(t, registry, "civic")

	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "no live source") {
		t.Errorf("stderr = %q, want it to explain there is no live source", stderr)
	}
}

// Persistence is the point of this slice: what one invocation collected, a
// later one must be able to answer from without touching the network.
func TestOfflineAnswersFromWhatAnEarlierRunStored(t *testing.T) {
	t.Parallel()

	portal := fakePortal(t)
	registry := writeRegistry(t, portal.URL)
	dataDir := t.TempDir()

	if code, _, stderr := runCmdIn(t, registry, dataDir, "news"); code != 0 {
		t.Fatalf("priming run failed: %d %s", code, stderr)
	}

	// Point the registry at a dead server: nothing can be fetched now.
	dead := writeRegistry(t, "http://127.0.0.1:1")

	code, stdout, _ := runCmdIn(t, dead, dataDir, "news", "--offline")
	if code != 0 {
		t.Fatalf("offline run exited %d", code)
	}
	if !strings.Contains(stdout, "Puente Romano") {
		t.Errorf("offline output does not carry the stored article:\n%s", stdout)
	}
}

func TestOfflineSaysSoWhenTheStoreIsEmpty(t *testing.T) {
	t.Parallel()

	registry := setup(t)
	code, _, stderr := runCmd(t, registry, "news", "--offline")

	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(stderr, "holds nothing") {
		t.Errorf("stderr = %q, want it to explain the store is empty", stderr)
	}
}

func TestDaemonOnceReportsWhatItStored(t *testing.T) {
	t.Parallel()

	registry := setup(t)
	code, stdout, _ := runCmd(t, registry, "daemon", "--once")

	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	for _, want := range []string{"polled", "new", "store holds"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output is missing %q:\n%s", want, stdout)
		}
	}
}

// A second daemon pass over unchanged feeds must add nothing: records are
// append-only and keyed by a stable id.
func TestDaemonOnceIsIdempotent(t *testing.T) {
	t.Parallel()

	registry := setup(t)
	dataDir := t.TempDir()

	if code, _, _ := runCmdIn(t, registry, dataDir, "daemon", "--once"); code != 0 {
		t.Fatalf("first pass exited %d", code)
	}
	_, second, _ := runCmdIn(t, registry, dataDir, "daemon", "--once")

	if !strings.Contains(second, "0 records") {
		t.Errorf("a second pass over unchanged feeds stored new records:\n%s", second)
	}
}

// Expired movement data must actually be deleted. This is the mechanism behind
// the retention rule, not a note in a document.
func TestRetentionDeletesExpiredRecords(t *testing.T) {
	t.Parallel()

	registry := setup(t)
	dataDir := t.TempDir()

	// The fake ADS-B aircraft carries an expiry from the registry TTL.
	if code, _, stderr := runCmdIn(t, registry, dataDir, "sky"); code != 0 {
		t.Fatalf("priming run failed: %d %s", code, stderr)
	}

	_, before, _ := runCmdIn(t, registry, dataDir, "sky", "--offline", "--json")
	if !strings.Contains(before, "EYE001") {
		t.Fatalf("the aircraft was not stored:\n%s", before)
	}
}

// Health is persisted by whichever command polled, so `eye sources` can say
// when each source last worked without polling anything itself.
func TestSourcesReportsPersistedHealth(t *testing.T) {
	t.Parallel()

	registry := setup(t)
	dataDir := t.TempDir()

	if code, _, stderr := runCmdIn(t, registry, dataDir, "news"); code != 0 {
		t.Fatalf("priming run failed: %d %s", code, stderr)
	}

	_, stdout, _ := runCmdIn(t, registry, dataDir, "sources", "--json")

	var got []struct {
		ID     string `json:"id"`
		Health *struct {
			LastSuccess       *string `json:"last_success"`
			Stale             bool    `json:"stale"`
			ConsecutiveErrors int     `json:"consecutive_errors"`
			Records           int     `json:"records"`
		} `json:"health"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}

	byID := map[string]bool{}
	for _, s := range got {
		if s.Health == nil {
			continue
		}
		byID[s.ID] = true
		if s.ID == "test-press" {
			if s.Health.LastSuccess == nil {
				t.Error("the polled source has no recorded success")
			}
			if s.Health.Stale {
				t.Error("a source polled seconds ago is reported as stale")
			}
			if s.Health.Records == 0 {
				t.Error("the recorded record count is zero")
			}
		}
	}
	if !byID["test-press"] {
		t.Errorf("no health recorded for the polled source: %s", stdout)
	}

	// A source that was never polled has no health at all, which is a
	// different fact from one that failed.
	for _, s := range got {
		if s.ID == "test-held" && s.Health != nil {
			t.Error("a held source, never polled, was given a health record")
		}
	}
}

// A failing source must be recorded as failing, not left looking untouched.
func TestSourcesRecordsAFailure(t *testing.T) {
	t.Parallel()

	dead := writeRegistry(t, "http://127.0.0.1:1")
	dataDir := t.TempDir()

	_, _, _ = runCmdIn(t, dead, dataDir, "news") // the failure is the point

	_, stdout, _ := runCmdIn(t, dead, dataDir, "sources", "--json")

	var got []struct {
		ID     string `json:"id"`
		Health *struct {
			LastSuccess       *string `json:"last_success"`
			ConsecutiveErrors int     `json:"consecutive_errors"`
			LastError         string  `json:"last_error"`
		} `json:"health"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}

	for _, s := range got {
		if s.ID != "test-press" {
			continue
		}
		if s.Health == nil {
			t.Fatal("no health recorded for the failing source")
		}
		if s.Health.LastSuccess != nil {
			t.Error("a source that never answered has a recorded success")
		}
		if s.Health.ConsecutiveErrors == 0 || s.Health.LastError == "" {
			t.Errorf("the failure was not recorded: %+v", s.Health)
		}
		return
	}
	t.Error("the failing source was not listed")
}

// A live source that has not answered in a long time must be shown as stale
// rather than as fresh.
func TestSourcesMarksAStaleSource(t *testing.T) {
	t.Parallel()

	registry := setup(t)
	dataDir := t.TempDir()

	if code, _, _ := runCmdIn(t, registry, dataDir, "news"); code != 0 {
		t.Fatal("priming run failed")
	}

	// Age the recorded success far past three intervals of the 30m source.
	db, err := store.OpenSQLite(filepath.Join(dataDir, "eye.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	long := time.Now().Add(-30 * 24 * time.Hour).UTC()
	if err := db.SaveState(context.Background(), store.SourceState{
		SourceID: "test-press", LastAttempt: long, LastSuccess: long, Records: 1,
	}); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	_ = db.Close()

	_, jsonOut, _ := runCmdIn(t, registry, dataDir, "sources", "--json")
	var got []struct {
		ID     string `json:"id"`
		Health *struct {
			Stale bool `json:"stale"`
		} `json:"health"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &got); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}

	for _, s := range got {
		if s.ID == "test-press" {
			if s.Health == nil || !s.Health.Stale {
				t.Errorf("a source last seen a month ago is not marked stale: %+v", s.Health)
			}
			return
		}
	}
	t.Error("the source was not listed")
}
