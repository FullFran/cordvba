// Package infrastructure loads the source registry from disk.
//
// The registry is the legal contract of the project, so a malformed entry stops
// the program rather than degrading quietly: a source eye cannot describe is a
// source eye must not fetch.
package infrastructure

import (
	"errors"
	"fmt"
	"os"
	"time"

	domain "github.com/FullFran/cordvba/apps/eye/internal/source/domain"
	"gopkg.in/yaml.v3"
)

// ErrRegistry is returned when the registry file cannot be turned into a valid
// set of sources.
var ErrRegistry = errors.New("registry")

// yamlSource mirrors one entry of configs/sources.yaml. Durations arrive as
// strings ("60s", "1h") because that is what a human writes.
type yamlSource struct {
	ID             string            `yaml:"id"`
	Authority      string            `yaml:"authority"`
	Topic          string            `yaml:"topic"`
	URL            string            `yaml:"url"`
	Format         string            `yaml:"format"`
	License        string            `yaml:"license"`
	Access         string            `yaml:"access"`
	Automation     string            `yaml:"automation"`
	Interval       string            `yaml:"interval"`
	PublishedEvery string            `yaml:"published_every"`
	SampledKinds   []string          `yaml:"sampled_kinds"`
	Notes          string            `yaml:"notes"`
	Options        map[string]string `yaml:"options"`
}

// yamlRegistry is the top-level document.
type yamlRegistry struct {
	Sources []yamlSource `yaml:"sources"`
}

// knownAccess and knownAutomation exist so that a typo becomes a load error
// rather than a silently unrecognised value.
var (
	knownAccess = map[domain.Access]bool{
		domain.AccessDocumentedAPI:        true,
		domain.AccessDocumentedDownload:   true,
		domain.AccessPublicHTML:           true,
		domain.AccessUndocumentedBackend:  true,
		domain.AccessUndocumentedPersonal: true,
	}
	knownAutomation = map[domain.AutomationStatus]bool{
		domain.AutomationEnabled:     true,
		domain.AutomationReviewTerms: true,
		domain.AutomationManualLink:  true,
		domain.AutomationDisabled:    true,
	}
)

// LoadFile reads and validates the registry at path.
func LoadFile(path string) ([]domain.Source, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- operator-supplied registry path, never request input
	if err != nil {
		return nil, fmt.Errorf("%w: read %s: %w", ErrRegistry, path, err)
	}
	return Parse(data)
}

// Parse turns registry bytes into validated sources.
func Parse(data []byte) ([]domain.Source, error) {
	var doc yamlRegistry
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRegistry, err)
	}
	if len(doc.Sources) == 0 {
		return nil, fmt.Errorf("%w: no sources declared", ErrRegistry)
	}

	seen := make(map[string]bool, len(doc.Sources))
	sources := make([]domain.Source, 0, len(doc.Sources))

	for i, entry := range doc.Sources {
		src, err := convert(entry)
		if err != nil {
			return nil, fmt.Errorf("%w: entry %d (%s): %w", ErrRegistry, i+1, entry.ID, err)
		}
		if seen[src.ID] {
			return nil, fmt.Errorf("%w: duplicate source id %q", ErrRegistry, src.ID)
		}
		seen[src.ID] = true
		sources = append(sources, src)
	}
	return sources, nil
}

// convert maps one YAML entry onto the domain type and validates it.
func convert(e yamlSource) (domain.Source, error) {
	interval, err := parseDuration(e.Interval)
	if err != nil {
		return domain.Source{}, fmt.Errorf("interval: %w", err)
	}
	published, err := parseDuration(e.PublishedEvery)
	if err != nil {
		return domain.Source{}, fmt.Errorf("published_every: %w", err)
	}

	src := domain.Source{
		ID:             e.ID,
		Authority:      e.Authority,
		Topic:          e.Topic,
		URL:            e.URL,
		Format:         e.Format,
		License:        e.License,
		Access:         domain.Access(e.Access),
		Automation:     domain.AutomationStatus(e.Automation),
		Interval:       interval,
		PublishedEvery: published,
		SampledKinds:   e.SampledKinds,
		Notes:          e.Notes,
		Options:        e.Options,
	}

	if !knownAccess[src.Access] {
		return domain.Source{}, fmt.Errorf("unknown access %q", e.Access)
	}
	if !knownAutomation[src.Automation] {
		return domain.Source{}, fmt.Errorf("unknown automation %q", e.Automation)
	}
	if err := src.Validate(); err != nil {
		return domain.Source{}, err
	}
	return src, nil
}

// parseDuration accepts an empty value as "not declared".
func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, err
	}
	if d < 0 {
		return 0, fmt.Errorf("negative duration %q", s)
	}
	return d, nil
}
