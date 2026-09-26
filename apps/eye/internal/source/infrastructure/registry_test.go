package infrastructure_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	domain "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
	registry "github.com/FullFran/cordvba/apps/eye/internal/source/infrastructure"
)

// The registry shipped with the repository must always load. It is a contract,
// not a sample: an invalid entry means eye would fetch something it cannot
// describe.
func TestShippedRegistryLoads(t *testing.T) {
	t.Parallel()

	sources, err := registry.LoadFile("../../../configs/sources.yaml")
	if err != nil {
		t.Fatalf("LoadFile() = %v", err)
	}
	if len(sources) < 20 {
		t.Errorf("loaded %d sources, expected the full registry", len(sources))
	}

	var enabled int
	for _, s := range sources {
		if s.Automation.Pollable() {
			enabled++
			if s.Format == "" {
				t.Errorf("%s: enabled source with no format", s.ID)
			}
		}
	}
	if enabled == 0 {
		t.Error("no source is enabled; eye would fetch nothing")
	}
	t.Logf("%d sources, %d enabled", len(sources), enabled)
}

func TestParseValidRegistry(t *testing.T) {
	t.Parallel()

	sources, err := registry.Parse([]byte(`
sources:
  - id: diario-cordoba
    authority: Diario Cordoba
    topic: press
    url: https://www.diariocordoba.com/rss/
    format: rss
    license: unspecified
    access: documented_api
    automation: enabled
    interval: 30m
    options:
      rows: "50"
`))
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}

	if len(sources) != 1 {
		t.Fatalf("sources = %d, want 1", len(sources))
	}

	s := sources[0]
	if s.Interval != 30*time.Minute {
		t.Errorf("interval = %v, want 30m", s.Interval)
	}
	if s.Option("rows", "") != "50" {
		t.Errorf("options not carried through: %v", s.Options)
	}
}

func TestParseRejects(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "empty document",
			yaml: "sources: []",
			want: "no sources declared",
		},
		{
			name: "unknown access",
			yaml: base(`access: telepathy`),
			want: "unknown access",
		},
		{
			name: "unknown automation",
			yaml: base(`automation: probably-fine`),
			want: "unknown automation",
		},
		{
			name: "unparsable interval",
			yaml: base(`interval: soon`),
			want: "interval",
		},
		{
			name: "enabled without interval",
			yaml: base(`interval: ""`),
			want: "positive interval",
		},
		{
			name: "missing format",
			yaml: base(`format: ""`),
			want: "format",
		},
		{
			name: "missing license",
			yaml: base(`license: ""`),
			want: "license",
		},
		{
			name: "undocumented backend enabled",
			yaml: base(`access: undocumented_backend`),
			want: "undocumented backends are never pollable",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := registry.Parse([]byte(tc.yaml))
			if err == nil {
				t.Fatal("Parse() = nil error, want a failure")
			}
			if !errors.Is(err, registry.ErrRegistry) {
				t.Errorf("error does not wrap ErrRegistry: %v", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestParseRejectsDuplicateIDs(t *testing.T) {
	t.Parallel()

	_, err := registry.Parse([]byte(`
sources:
  - id: dup
    authority: A
    topic: press
    url: https://a.example/rss
    format: rss
    license: unspecified
    access: documented_api
    automation: enabled
    interval: 30m
  - id: dup
    authority: B
    topic: press
    url: https://b.example/rss
    format: rss
    license: unspecified
    access: documented_api
    automation: enabled
    interval: 30m
`))
	if err == nil || !strings.Contains(err.Error(), "duplicate source id") {
		t.Fatalf("Parse() = %v, want a duplicate id error", err)
	}
}

func TestParseAllowsHeldSourceWithoutInterval(t *testing.T) {
	t.Parallel()

	sources, err := registry.Parse([]byte(`
sources:
  - id: infoca-active
    authority: Junta de Andalucia
    topic: fire
    url: https://www.juntadeandalucia.es/organismos/ema/
    format: web-viewer
    license: unspecified
    access: public_html
    automation: manual_link
`))
	if err != nil {
		t.Fatalf("Parse() = %v", err)
	}
	if sources[0].Automation.Pollable() {
		t.Error("a manual_link source must never be pollable")
	}
}

// base renders a valid entry with one field overridden.
func base(override string) string {
	fields := map[string]string{
		"id":         "test-source",
		"authority":  "Test Authority",
		"topic":      "press",
		"url":        "https://example.org/rss",
		"format":     "rss",
		"license":    "unspecified",
		"access":     "documented_api",
		"automation": "enabled",
		"interval":   "30m",
	}

	key, value, _ := strings.Cut(override, ":")
	fields[strings.TrimSpace(key)] = strings.TrimSpace(value)

	var sb strings.Builder
	sb.WriteString("sources:\n  - ")
	first := true
	for _, k := range []string{"id", "authority", "topic", "url", "format", "license", "access", "automation", "interval"} {
		if !first {
			sb.WriteString("    ")
		}
		first = false
		sb.WriteString(k + ": " + fields[k] + "\n")
	}
	return sb.String()
}

var _ = domain.AutomationEnabled
