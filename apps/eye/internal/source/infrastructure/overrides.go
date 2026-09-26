package infrastructure

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	domain "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
	"gopkg.in/yaml.v3"
)

// ErrOverrides is returned when the optional deployment overrides file cannot
// be turned into a valid restriction of the registry.
var ErrOverrides = errors.New("overrides")

// yamlOverrides mirrors the optional overrides file (EYE_OVERRIDES_FILE).
type yamlOverrides struct {
	Sources struct {
		Only       []string `yaml:"only"`
		Disable    []string `yaml:"disable"`
		TopicsOnly []string `yaml:"topics_only"`
	} `yaml:"sources"`
	// Retention values arrive as strings ("historical", "24h") because that
	// is what a human writes; parseRetentionOverride turns each into the two
	// mutually exclusive forms Retention distinguishes.
	Retention map[string]string `yaml:"retention"`
}

// LoadOverridesFile reads and validates the optional overrides file at path
// against the given registry.
//
// A missing file is not an error: it is today's behaviour, unchanged, which
// is the point of the file being optional. Any other failure to read it, or a
// present file that fails to parse or validate, stops the program — the same
// fail-closed posture the registry itself takes, because an overrides file
// eye cannot describe is one it must not act on.
func LoadOverridesFile(path string, sources []domain.Source) (domain.Overrides, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- operator-supplied overrides path, never request input
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return domain.Overrides{}, nil
		}
		return domain.Overrides{}, fmt.Errorf("%w: read %s: %w", ErrOverrides, path, err)
	}
	overrides, err := ParseOverrides(data, sources)
	if err != nil {
		return domain.Overrides{}, fmt.Errorf("%s: %w", path, err)
	}
	return overrides, nil
}

// ParseOverrides turns overrides bytes into a validated Overrides value.
func ParseOverrides(data []byte, sources []domain.Source) (domain.Overrides, error) {
	var doc yamlOverrides
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return domain.Overrides{}, fmt.Errorf("%w: %w", ErrOverrides, err)
	}

	out := domain.Overrides{
		Sources: domain.SourceOverrides{
			Only:       doc.Sources.Only,
			Disable:    doc.Sources.Disable,
			TopicsOnly: doc.Sources.TopicsOnly,
		},
	}

	if len(doc.Retention) > 0 {
		out.Retention = make(map[string]domain.RetentionOverride, len(doc.Retention))
		for id, raw := range doc.Retention {
			ov, err := parseRetentionOverride(raw)
			if err != nil {
				return domain.Overrides{}, fmt.Errorf("%w: retention %s: %w", ErrOverrides, id, err)
			}
			out.Retention[id] = ov
		}
	}

	if err := out.Validate(sources); err != nil {
		return domain.Overrides{}, fmt.Errorf("%w: %w", ErrOverrides, err)
	}
	return out, nil
}

// parseRetentionOverride accepts exactly the two forms retention: describes:
// the literal "historical", or a positive Go duration — an ephemeral source
// pinned to that TTL in place of whatever the adapter would otherwise
// compute. Anything else, including a non-positive duration, is rejected: the
// same discipline the registry's own retention option already enforces.
func parseRetentionOverride(raw string) (domain.RetentionOverride, error) {
	raw = strings.TrimSpace(raw)
	if domain.Retention(raw) == domain.RetentionHistorical {
		return domain.RetentionOverride{Kind: domain.RetentionHistorical}, nil
	}

	d, err := time.ParseDuration(raw)
	if err != nil {
		return domain.RetentionOverride{}, fmt.Errorf("must be %q or a duration: %w", domain.RetentionHistorical, err)
	}
	if d <= 0 {
		return domain.RetentionOverride{}, fmt.Errorf("duration %q must be positive", raw)
	}
	return domain.RetentionOverride{Kind: domain.RetentionEphemeral, TTL: d}, nil
}
