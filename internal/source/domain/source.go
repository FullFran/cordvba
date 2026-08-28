// Package domain models the source registry: which public feeds eye is allowed
// to read, how often, and on what legal footing.
package domain

import (
	"errors"
	"time"
)

// ErrInvalidSource is returned when a registry entry breaks an invariant.
var ErrInvalidSource = errors.New("source: invalid")

// Access describes the kind of public interface a source exposes. It is the
// input to the automation decision, not the decision itself.
type Access string

// The access kinds eye distinguishes.
const (
	// AccessDocumentedAPI is a published, machine-readable contract:
	// CKAN, DATEX II, GTFS-RT, AEMET OpenData, FIRMS.
	AccessDocumentedAPI Access = "documented_api"
	// AccessDocumentedDownload is a published file resource with stable
	// terms, such as a GTFS zip in a national access point.
	AccessDocumentedDownload Access = "documented_download"
	// AccessPublicHTML is a public page with no documented reuse contract.
	AccessPublicHTML Access = "public_html"
	// AccessUndocumentedBackend is an internal endpoint discovered behind a
	// public viewer. eye never automates these.
	AccessUndocumentedBackend Access = "undocumented_backend"
)

// AutomationStatus is the gate that decides whether the scheduler may poll a
// source at all. It exists so that "it worked because we called
// /internal/v3/mapData until they banned us" can never happen by accident.
type AutomationStatus string

// The automation gates, from permitted to forbidden.
const (
	// AutomationEnabled: poll it, the publisher documented the interface.
	AutomationEnabled AutomationStatus = "enabled"
	// AutomationReviewTerms: valuable but the reuse terms are unclear.
	// Held out of the scheduler until a human resolves the licence.
	AutomationReviewTerms AutomationStatus = "review_terms"
	// AutomationManualLink: eye links to the public viewer and ingests
	// nothing.
	AutomationManualLink AutomationStatus = "manual_link"
	// AutomationDisabled: never fetched automatically.
	AutomationDisabled AutomationStatus = "disabled"
)

// Pollable reports whether the scheduler is allowed to fetch this source.
// Only an explicit "enabled" passes; every other state, including an unknown
// one, fails closed.
func (a AutomationStatus) Pollable() bool { return a == AutomationEnabled }

// Source is one entry of the registry in configs/sources.yaml.
type Source struct {
	ID        string `json:"id"`
	Authority string `json:"authority"`
	Topic     string `json:"topic"`
	URL       string `json:"url"`
	License   string `json:"license"`

	// Format selects the adapter. It is the registry, not the code, that
	// decides which parser a source gets, so adding a feed of a format eye
	// already speaks is a configuration change.
	Format string `json:"format"`

	// Options carries adapter-specific settings, such as a CKAN dataset id
	// or a viewport radius. Keeping them here means a new source of a known
	// format needs no new Go code.
	Options map[string]string `json:"options,omitempty"`

	Access     Access           `json:"access"`
	Automation AutomationStatus `json:"automation"`

	// Interval is the engineering poll interval, which is eye's decision.
	Interval time.Duration `json:"interval"`
	// PublishedEvery is the refresh cadence the publisher declares, when it
	// declares one. Polling faster than this buys nothing but rate limits.
	PublishedEvery time.Duration `json:"published_every,omitempty"`

	// Notes carries the human reason for a non-enabled automation status.
	Notes string `json:"notes,omitempty"`
}

// Validate enforces the invariants every registry entry must satisfy.
func (s Source) Validate() error {
	switch {
	case s.ID == "":
		return errors.Join(ErrInvalidSource, errors.New("empty id"))
	case s.Authority == "":
		return errors.Join(ErrInvalidSource, errors.New("empty authority"))
	case s.Topic == "":
		return errors.Join(ErrInvalidSource, errors.New("empty topic"))
	case s.URL == "":
		return errors.Join(ErrInvalidSource, errors.New("empty url"))
	case s.License == "":
		return errors.Join(ErrInvalidSource, errors.New("empty license: use \"unspecified\" when the catalog declares none"))
	case s.Format == "":
		return errors.Join(ErrInvalidSource, errors.New("empty format: the registry selects the adapter"))
	case s.Automation.Pollable() && s.Interval <= 0:
		return errors.Join(ErrInvalidSource, errors.New("enabled source needs a positive interval"))
	case s.Automation.Pollable() && s.Access == AccessUndocumentedBackend:
		return errors.Join(ErrInvalidSource, errors.New("undocumented backends are never pollable"))
	}
	return nil
}

// Option returns an adapter setting, or the fallback when it is unset.
func (s Source) Option(key, fallback string) string {
	if v, ok := s.Options[key]; ok && v != "" {
		return v
	}
	return fallback
}
