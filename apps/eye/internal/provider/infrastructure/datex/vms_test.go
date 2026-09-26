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

	"github.com/FullFran/cordvba/apps/eye/internal/httpx"
	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	"github.com/FullFran/cordvba/apps/eye/internal/provider/infrastructure/datex"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// serveVms returns the recorded VmsPublication: twelve sign controllers, four
// of them showing nothing at all.
func serveVms(t *testing.T) *httptest.Server {
	t.Helper()

	body, err := os.ReadFile("../../../../testdata/datex/vms.xml") // #nosec G304 -- fixture path is a literal
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

// vmsSource builds the registry entry for dgt-vms.
func vmsSource(url string) source.Source {
	return source.Source{
		ID: "dgt-vms", Authority: "Direccion General de Trafico", Topic: "transport",
		URL: url, Format: "datex2-vms", License: "CC-BY-4.0",
		Access: source.AccessDocumentedAPI, Automation: source.AutomationEnabled, Interval: 5 * time.Minute,
	}
}

// pollVms fetches the fixture and indexes the result by LocalKey.
func pollVms(t *testing.T) map[string]observation.Record {
	t.Helper()

	srv := serveVms(t)
	records, err := datex.NewVms(vmsSource(srv.URL), httpx.New()).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}

	byKey := make(map[string]observation.Record, len(records))
	for _, r := range records {
		byKey[r.LocalKey] = r
	}
	if len(byKey) != len(records) {
		t.Fatalf("indexed %d of %d records; local keys collide", len(byKey), len(records))
	}
	return byKey
}

// A sign displaying nothing is not news. Four of the twelve recorded
// controllers are dark: two publish no message at all, two publish a message
// whose only content is the blank pictogram.
func TestVmsPollEmitsOnlySignsDisplayingSomething(t *testing.T) {
	t.Parallel()

	byKey := pollVms(t)
	if len(byKey) != 8 {
		t.Fatalf("records = %d, want 8", len(byKey))
	}

	for _, dark := range []string{"167937:1", "61444:1", "167938:1", "167939:1"} {
		if _, found := byKey[dark]; found {
			t.Errorf("blank sign %s produced a record", dark)
		}
	}
	for _, lit := range []string{"61441:1", "61455:1", "61471:1", "61475:1", "167940:1", "61523:1"} {
		if _, found := byKey[lit]; !found {
			t.Errorf("sign %s is displaying something but produced no record", lit)
		}
	}
	for _, rec := range byKey {
		if rec.Kind != "vms_message" {
			t.Errorf("kind = %q, want vms_message", rec.Kind)
		}
		if err := rec.Validate(); err != nil {
			t.Errorf("record %s does not validate: %v", rec.ID, err)
		}
	}
}

func TestVmsRecordCarriesTheDisplayedText(t *testing.T) {
	t.Parallel()

	byKey := pollVms(t)

	rec, ok := byKey["61441:1"]
	if !ok {
		t.Fatal("sign 61441:1 is missing")
	}
	if rec.Title != "VELOCIDAD CONTROLADA POR RADAR" {
		t.Errorf("title = %q", rec.Title)
	}
	if !strings.Contains(rec.Description, "VELOCIDAD") {
		t.Errorf("description = %q", rec.Description)
	}

	// A sign that alternates two messages keeps both.
	alternating, ok := byKey["61471:1"]
	if !ok {
		t.Fatal("sign 61471:1 is missing")
	}
	if !strings.Contains(alternating.Title, "CORTE ACCESO") || !strings.Contains(alternating.Title, "ALTERNATIVA") {
		t.Errorf("title = %q, want both alternating messages", alternating.Title)
	}

	// A sign showing only a pictogram is still saying something, and the
	// code it is showing is the only honest title available.
	pictogram, ok := byKey["167940:1"]
	if !ok {
		t.Fatal("sign 167940:1 is missing")
	}
	if !strings.Contains(pictogram.Title, "XDGT") {
		t.Errorf("title = %q, want the pictogram code", pictogram.Title)
	}

	var payload struct {
		ControllerID string   `json:"controller_id"`
		Pictograms   []string `json:"pictograms"`
		TextLines    []string `json:"text_lines"`
	}
	if err := json.Unmarshal(rec.Payload, &payload); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	if payload.ControllerID != "61441" {
		t.Errorf("payload controller_id = %q", payload.ControllerID)
	}
	if len(payload.Pictograms) == 0 || len(payload.TextLines) == 0 {
		t.Errorf("payload lost the sign's content: %+v", payload)
	}
}

// The VmsPublication is a status feed: it references each sign by controller
// id and carries no coordinates whatsoever. Inventing a position would be
// worse than having none.
func TestVmsRecordHasNoInventedPosition(t *testing.T) {
	t.Parallel()

	for _, rec := range pollVms(t) {
		if rec.Position != nil {
			t.Errorf("record %s has position %v, but the feed states none", rec.ID, *rec.Position)
		}
	}
}

func TestVmsTimestampsAreDistinct(t *testing.T) {
	t.Parallel()

	rec, ok := pollVms(t)["61441:1"]
	if !ok {
		t.Fatal("sign 61441:1 is missing")
	}
	if rec.ObservedAt.IsZero() || rec.FetchedAt.IsZero() {
		t.Fatalf("observed=%v fetched=%v", rec.ObservedAt, rec.FetchedAt)
	}
	if rec.ObservedAt.Equal(rec.FetchedAt) {
		t.Error("observed_at and fetched_at were merged")
	}
	// timeLastSet, in UTC.
	want := time.Date(2026, time.September, 2, 22, 48, 10, 0, time.UTC)
	if !rec.ObservedAt.Equal(want) {
		t.Errorf("observed_at = %s, want %s", rec.ObservedAt, want)
	}
}

func TestVmsPollCarriesCompleteProvenance(t *testing.T) {
	t.Parallel()

	srv := serveVms(t)
	src := vmsSource(srv.URL)
	records, err := datex.NewVms(src, httpx.New()).Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() = %v", err)
	}
	if len(records) == 0 {
		t.Fatal("no records")
	}

	for _, rec := range records {
		prov := rec.Provenance
		if err := prov.Validate(); err != nil {
			t.Fatalf("provenance of %s: %v", rec.ID, err)
		}
		if prov.Publisher != src.Authority || prov.SourceURL != src.URL || prov.License != src.License {
			t.Fatalf("provenance = %+v", prov)
		}
		if prov.FetchedAt.IsZero() {
			t.Fatal("provenance has no fetched_at")
		}
		if len(prov.RawHash) != 64 {
			t.Fatalf("raw_hash = %q, want a hex sha256", prov.RawHash)
		}
	}
}

// A publication in which every sign is dark is a valid, quiet answer, not a
// failure — but a publication eye cannot parse is a failure.
func TestVmsPollDistinguishesQuietFromBroken(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name: "every sign is dark",
			body: `<?xml version="1.0"?>
<d2:payload xsi:type="vms:VmsPublication" xmlns:d2="d" xmlns:vms="v" xmlns:com="c" xmlns:xsi="x">
  <com:publicationTime>2026-09-03T00:51:57.000+02:00</com:publicationTime>
  <vms:vmsControllerStatus>
    <vms:vmsControllerReference targetClass="vms:VmsController" id="1"/>
    <vms:vmsStatus vmsIndex="1"><vms:vmsStatus/></vms:vmsStatus>
  </vms:vmsControllerStatus>
  <vms:vmsControllerStatus>
    <vms:vmsControllerReference targetClass="vms:VmsController" id="2"/>
    <vms:vmsStatus vmsIndex="1"><vms:vmsStatus><vms:vmsMessage messageIndex="1"><vms:vmsMessage>
      <vms:timeLastSet>2026-09-02T14:30:59.000+02:00</vms:timeLastSet>
      <vms:displayAreaSettings displayAreaIndex="1"><vms:displayAreaSettings xsi:type="vms:PictogramDisplay">
        <vms:pictogram xsi:type="vms:RegularPictogram"><vms:customPictogramCode>0</vms:customPictogramCode></vms:pictogram>
      </vms:displayAreaSettings></vms:displayAreaSettings>
    </vms:vmsMessage></vms:vmsMessage></vms:vmsStatus></vms:vmsStatus>
  </vms:vmsControllerStatus>
  <vms:vmsControllerStatus>
    <vms:vmsControllerReference targetClass="vms:VmsController" id="3"/>
    <vms:vmsStatus vmsIndex="1"><vms:vmsStatus><vms:vmsMessage messageIndex="1"><vms:vmsMessage>
      <vms:timeLastSet>2026-09-02T14:30:59.000+02:00</vms:timeLastSet>
      <vms:displayAreaSettings displayAreaIndex="1"><vms:displayAreaSettings xsi:type="vms:TextDisplay">
        <vms:textLine lineIndex="1"><vms:textLine><vms:textLine>   </vms:textLine></vms:textLine></vms:textLine>
      </vms:displayAreaSettings></vms:displayAreaSettings>
    </vms:vmsMessage></vms:vmsMessage></vms:vmsStatus></vms:vmsStatus>
  </vms:vmsControllerStatus>
</d2:payload>`,
		},
		{name: "not xml at all", body: `{"signs": []}`, wantErr: true},
		{name: "empty body", body: ``, wantErr: true},
		{
			name:    "truncated mid sign",
			body:    `<?xml version="1.0"?><d2:payload xmlns:d2="d" xmlns:vms="v"><vms:vmsControllerStatus><vms:vmsStatus`,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/xml; charset=utf-8")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			records, err := datex.NewVms(vmsSource(srv.URL), httpx.New()).Poll(context.Background())
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Poll() = %d records, nil error; want an error", len(records))
				}
				return
			}
			if err != nil {
				t.Fatalf("Poll() = %v", err)
			}
			if len(records) != 0 {
				t.Errorf("records = %d, want 0 for a feed of dark signs", len(records))
			}
		})
	}
}

func TestVmsPollReportsAnUnreachableSource(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	if _, err := datex.NewVms(vmsSource(srv.URL), httpx.New()).Poll(context.Background()); err == nil {
		t.Fatal("Poll() against a 404 = nil error")
	}
}

func TestVmsPollHandlesNotModified(t *testing.T) {
	t.Parallel()

	srv := serveVms(t)
	p := datex.NewVms(vmsSource(srv.URL), httpx.New())

	if _, err := p.Poll(context.Background()); err != nil {
		t.Fatalf("first Poll() = %v", err)
	}
	got, err := p.Poll(context.Background())
	if err != nil {
		t.Fatalf("second Poll() = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Errorf("second Poll() returned %d records, want 0", len(got))
	}
}

func TestVmsInfoReturnsTheRegistryEntry(t *testing.T) {
	t.Parallel()

	s := vmsSource("https://example.org/vms.xml")
	if got := datex.NewVms(s, httpx.New()).Info(); got.ID != s.ID {
		t.Errorf("Info().ID = %q", got.ID)
	}
}
