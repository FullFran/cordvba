package infrastructure_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domain "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
	registry "github.com/FullFran/cordvba/apps/eye/internal/source/infrastructure"
)

// overridesFixtureRegistry mirrors the shape overrides.yaml is validated
// against in these tests: an enabled source, one the registry holds, and an
// undocumented personal one.
func overridesFixtureRegistry(t *testing.T) []domain.Source {
	t.Helper()

	sources, err := registry.Parse([]byte(`
sources:
  - id: metar-cordoba
    authority: NOAA
    topic: weather
    url: https://aviationweather.gov/api/data/metar
    format: metar-json
    license: us-government-public-domain
    access: documented_api
    automation: enabled
    interval: 20m
  - id: miteco-ica
    authority: MITECO
    topic: air_quality
    url: https://www.miteco.gob.es/es/calidad-y-evaluacion-ambiental/temas/atmosfera-y-calidad-del-aire/
    format: dcat-air-quality
    license: unspecified
    access: documented_api
    automation: enabled
    interval: 1h
  - id: renfe-positions
    authority: RENFE
    topic: transport
    url: https://data.renfe.com
    format: gtfs-rt
    license: unspecified
    access: documented_api
    automation: enabled
    interval: 60s
  - id: saih-guadalquivir
    authority: CHG
    topic: hydrology
    url: https://www.chguadalquivir.es/saih/
    format: web-viewer
    license: unspecified
    access: public_html
    automation: review_terms
    notes: Reuse terms unresolved.
  - id: aucorsa-arrivals
    authority: AUCORSA
    topic: transport
    url: https://lightapi.aucorsa.es/wp-json/aucorsa/v1
    format: aucorsa-arrivals
    license: unspecified
    access: undocumented_personal
    automation: enabled
    interval: 60s
    notes: Undocumented endpoint the operator reads for themselves.
`))
	if err != nil {
		t.Fatalf("Parse() fixture registry = %v", err)
	}
	return sources
}

func TestParseOverridesEmptyIsZeroValue(t *testing.T) {
	t.Parallel()

	o, err := registry.ParseOverrides([]byte(``), overridesFixtureRegistry(t))
	if err != nil {
		t.Fatalf("ParseOverrides() = %v", err)
	}
	if !o.Permits(domain.Source{ID: "anything"}) {
		t.Error("an empty overrides document must restrict nothing")
	}
}

func TestParseOverridesOnlyAndDisable(t *testing.T) {
	t.Parallel()

	sources := overridesFixtureRegistry(t)
	o, err := registry.ParseOverrides([]byte(`
sources:
  only: [metar-cordoba, miteco-ica]
`), sources)
	if err != nil {
		t.Fatalf("ParseOverrides() = %v", err)
	}
	if !o.Permits(bySourceID(sources, "metar-cordoba")) {
		t.Error("metar-cordoba should be permitted")
	}
	if o.Permits(bySourceID(sources, "renfe-positions")) {
		t.Error("renfe-positions is not in only, and must not be permitted")
	}
}

func TestParseOverridesTopicsOnly(t *testing.T) {
	t.Parallel()

	sources := overridesFixtureRegistry(t)
	o, err := registry.ParseOverrides([]byte(`
sources:
  topics_only: [weather, air_quality]
`), sources)
	if err != nil {
		t.Fatalf("ParseOverrides() = %v", err)
	}
	if !o.Permits(bySourceID(sources, "miteco-ica")) {
		t.Error("miteco-ica is topic air_quality and should be permitted")
	}
	if o.Permits(bySourceID(sources, "renfe-positions")) {
		t.Error("renfe-positions is topic transport, not named, and must not be permitted")
	}
}

func TestParseOverridesRetentionHistorical(t *testing.T) {
	t.Parallel()

	sources := overridesFixtureRegistry(t)
	o, err := registry.ParseOverrides([]byte(`
retention:
  metar-cordoba: historical
`), sources)
	if err != nil {
		t.Fatalf("ParseOverrides() = %v", err)
	}

	kind, ttl, overridden := o.EffectiveRetention(bySourceID(sources, "metar-cordoba"))
	if !overridden || kind != domain.RetentionHistorical || ttl != 0 {
		t.Errorf("EffectiveRetention() = (%v, %v, %v), want (historical, 0, true)", kind, ttl, overridden)
	}
}

func TestParseOverridesRetentionDuration(t *testing.T) {
	t.Parallel()

	sources := overridesFixtureRegistry(t)
	o, err := registry.ParseOverrides([]byte(`
retention:
  renfe-positions: 24h
`), sources)
	if err != nil {
		t.Fatalf("ParseOverrides() = %v", err)
	}

	kind, ttl, overridden := o.EffectiveRetention(bySourceID(sources, "renfe-positions"))
	if !overridden || kind != domain.RetentionEphemeral || ttl != 24*time.Hour {
		t.Errorf("EffectiveRetention() = (%v, %v, %v), want (ephemeral, 24h, true)", kind, ttl, overridden)
	}
}

func TestParseOverridesRejectsUnknownEntries(t *testing.T) {
	t.Parallel()

	sources := overridesFixtureRegistry(t)
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "unknown id in only",
			yaml: "sources:\n  only: [does-not-exist]\n",
			want: "does-not-exist",
		},
		{
			name: "unknown topic",
			yaml: "sources:\n  topics_only: [aviation]\n",
			want: "aviation",
		},
		{
			name: "same id in only and disable",
			yaml: "sources:\n  only: [metar-cordoba]\n  disable: [metar-cordoba]\n",
			want: "metar-cordoba",
		},
		{
			name: "malformed retention value",
			yaml: "retention:\n  metar-cordoba: sometimes\n",
			want: "metar-cordoba",
		},
		{
			name: "non-positive retention duration",
			yaml: "retention:\n  metar-cordoba: 0h\n",
			want: "metar-cordoba",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := registry.ParseOverrides([]byte(tc.yaml), sources)
			if !errors.Is(err, registry.ErrOverrides) {
				t.Fatalf("ParseOverrides() = %v, want ErrOverrides", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

// The restrict-only rule, proven from the file all the way down: an operator
// cannot use overrides.yaml to switch on a held or ungated source, whatever
// the file says.
func TestParseOverridesCannotEnableAHeldOrPersonalSource(t *testing.T) {
	t.Parallel()

	sources := overridesFixtureRegistry(t)
	o, err := registry.ParseOverrides([]byte(`
sources:
  only: [saih-guadalquivir, aucorsa-arrivals]
`), sources)
	if err != nil {
		t.Fatalf("ParseOverrides() = %v", err)
	}

	if o.Schedulable(bySourceID(sources, "saih-guadalquivir"), true) {
		t.Error("a review_terms source must never become schedulable through overrides")
	}
	if o.Schedulable(bySourceID(sources, "aucorsa-arrivals"), false) {
		t.Error("an undocumented_personal source without the machine opt-in must never become schedulable through overrides")
	}
}

func TestLoadOverridesFileMissingIsTodaysBehaviour(t *testing.T) {
	t.Parallel()

	sources := overridesFixtureRegistry(t)
	path := filepath.Join(t.TempDir(), "overrides.yaml")

	o, err := registry.LoadOverridesFile(path, sources)
	if err != nil {
		t.Fatalf("LoadOverridesFile() = %v, want nil for a missing file", err)
	}
	for _, s := range sources {
		if !o.Permits(s) {
			t.Errorf("a missing overrides file must restrict nothing, but %s is not permitted", s.ID)
		}
	}
}

func TestLoadOverridesFilePresentAndInvalidFailsNamingTheEntry(t *testing.T) {
	t.Parallel()

	sources := overridesFixtureRegistry(t)
	path := filepath.Join(t.TempDir(), "overrides.yaml")
	if err := os.WriteFile(path, []byte("sources:\n  only: [does-not-exist]\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := registry.LoadOverridesFile(path, sources)
	if !errors.Is(err, registry.ErrOverrides) {
		t.Fatalf("LoadOverridesFile() = %v, want ErrOverrides", err)
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("error = %q, want it to name the bad entry", err)
	}
}

func TestLoadOverridesFilePresentAndValid(t *testing.T) {
	t.Parallel()

	sources := overridesFixtureRegistry(t)
	path := filepath.Join(t.TempDir(), "overrides.yaml")
	if err := os.WriteFile(path, []byte("sources:\n  only: [metar-cordoba]\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	o, err := registry.LoadOverridesFile(path, sources)
	if err != nil {
		t.Fatalf("LoadOverridesFile() = %v", err)
	}
	if !o.Permits(bySourceID(sources, "metar-cordoba")) {
		t.Error("metar-cordoba should be permitted")
	}
	if o.Permits(bySourceID(sources, "miteco-ica")) {
		t.Error("miteco-ica is not in only, and must not be permitted")
	}
}

// bySourceID finds a fixture source by id, failing loudly if the fixture
// itself is wrong rather than returning a zero value a caller could confuse
// for a real result.
func bySourceID(sources []domain.Source, id string) domain.Source {
	for _, s := range sources {
		if s.ID == id {
			return s
		}
	}
	panic("fixture registry has no source " + id)
}
