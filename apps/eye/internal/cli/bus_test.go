package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/FullFran/cordvba/apps/eye/internal/config"
	"github.com/FullFran/cordvba/apps/eye/internal/httpx"
	observation "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	"github.com/FullFran/cordvba/apps/eye/internal/provider/infrastructure/aucorsa"
	source "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
)

// arrivalRecord builds a record shaped as the adapter writes them.
func arrivalRecord(t *testing.T, line string, minutes int, occupancy string) observation.Record {
	t.Helper()

	payload, err := json.Marshal(arrival{
		StopID: "116", StopName: "Ronda Tejares", Line: line,
		Route: "SANTA ROSA", Minutes: minutes, Occupancy: occupancy,
	})
	if err != nil {
		t.Fatal(err)
	}
	return observation.Record{Payload: payload, ObservedAt: time.Now().UTC()}
}

func TestRenderArrivalsPutsTheSoonestFirst(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	err := renderArrivals(&out, nil, []observation.Record{
		arrivalRecord(t, "4", 21, ""),
		arrivalRecord(t, "6", 3, "Ocupación Baja"),
		arrivalRecord(t, "2", 12, ""),
	}, time.Now())
	if err != nil {
		t.Fatalf("renderArrivals() = %v", err)
	}

	got := out.String()
	first, second, third := strings.Index(got, "3 min"), strings.Index(got, "12 min"), strings.Index(got, "21 min")
	if first < 0 || second < 0 || third < 0 {
		t.Fatalf("not every arrival was printed:\n%s", got)
	}
	if first > second || second > third {
		t.Errorf("arrivals are not ordered by wait:\n%s", got)
	}

	// An unreported occupancy must read as absent, never as blank space that
	// could be mistaken for an empty bus.
	if !strings.Contains(got, "—") {
		t.Errorf("a missing occupancy is not marked as missing:\n%s", got)
	}
	if !strings.Contains(got, "estimate") {
		t.Errorf("the output does not say these are estimates:\n%s", got)
	}
}

func TestRenderArrivalsWithNoBuses(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	stops := []aucorsa.Stop{{ID: "116", Name: "Ronda Tejares"}}
	if err := renderArrivals(&out, stops, nil, time.Now()); err != nil {
		t.Fatalf("renderArrivals() = %v", err)
	}

	got := out.String()
	// The stop that was asked about is still named: an empty answer about a
	// known stop is information, and an empty screen is not.
	if !strings.Contains(got, "Ronda Tejares") || !strings.Contains(got, "116") {
		t.Errorf("the stop asked about is not named:\n%s", got)
	}
	if !strings.Contains(got, "No buses due") {
		t.Errorf("output = %q", got)
	}
}

// busRuntime builds just enough runtime for the reader, with no store.
func busRuntime(access source.Access, allowPersonal bool) *runtime {
	return &runtime{
		cfg: config.Config{AllowPersonalSources: allowPersonal},
		sources: []source.Source{
			{ID: "aucorsa-lines", Format: "aucorsa-lines", Automation: source.AutomationEnabled},
			{
				ID: "aucorsa-arrivals", Format: arrivalsFormat, URL: "https://example.invalid",
				Access: access, Automation: source.AutomationEnabled,
			},
		},
		client: httpx.New(),
	}
}

// The gate is the whole point of ADR-0008, so the refusal has to be tested and
// it has to say which switch is off.
func TestArrivalsReaderRefusesWithoutTheOptIn(t *testing.T) {
	t.Parallel()

	_, err := busRuntime(source.AccessUndocumentedPersonal, false).arrivalsReader()
	if err == nil {
		t.Fatal("arrivalsReader() built a personal source with no opt-in")
	}
	if !strings.Contains(err.Error(), "EYE_ALLOW_PERSONAL_SOURCES") {
		t.Errorf("the refusal does not name the switch: %v", err)
	}
}

func TestArrivalsReaderBuildsWithTheOptIn(t *testing.T) {
	t.Parallel()

	if _, err := busRuntime(source.AccessUndocumentedPersonal, true).arrivalsReader(); err != nil {
		t.Fatalf("arrivalsReader() = %v", err)
	}
}

func TestArrivalsReaderWithoutTheSource(t *testing.T) {
	t.Parallel()

	rt := &runtime{cfg: config.Config{}, client: httpx.New()}
	if _, err := rt.arrivalsReader(); err == nil {
		t.Fatal("arrivalsReader() succeeded against a registry with no arrivals source")
	}
}

func TestResolveStopsPrefersTheExplicitNumbers(t *testing.T) {
	t.Parallel()

	src := source.Source{URL: "https://example.invalid", Options: map[string]string{"stops": "1,2"}}
	reader := aucorsa.NewRealtime(src, httpx.New())

	// A term is present, but --stop is explicit and must win without any
	// lookup reaching the network.
	ids, matched, err := resolveStops(context.Background(), reader, "tendillas", "116, 456")
	if err != nil {
		t.Fatalf("resolveStops() = %v", err)
	}
	if len(matched) != 0 {
		t.Errorf("matched = %v, want no directory lookup", matched)
	}
	if strings.Join(ids, ",") != "116,456" {
		t.Errorf("ids = %v", ids)
	}
}

func TestResolveStopsFallsBackToTheWatchedStops(t *testing.T) {
	t.Parallel()

	src := source.Source{URL: "https://example.invalid", Options: map[string]string{"stops": "116,456"}}
	reader := aucorsa.NewRealtime(src, httpx.New())

	ids, _, err := resolveStops(context.Background(), reader, "", "")
	if err != nil {
		t.Fatalf("resolveStops() = %v", err)
	}
	if strings.Join(ids, ",") != "116,456" {
		t.Errorf("ids = %v, want the registry's stops", ids)
	}
}
