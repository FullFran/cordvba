// Package domain models the source registry: which public feeds eye is allowed
// to read, how often, and on what legal footing.
package domain

import (
	"errors"
	"strings"
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
	// AccessUndocumentedPersonal is an undocumented endpoint the operator has
	// decided to read for their own personal use.
	//
	// It exists because "undocumented" and "forbidden" are not the same
	// thing: an endpoint a public site calls to render public information is
	// a reasonable thing for a person to read for themselves. What it is not
	// is a contract. It can change without notice, it can be rate-limited,
	// and nobody else running eye can reproduce a result that depends on it.
	//
	// So it is gated twice over. The registry must say so AND the operator
	// must opt in at runtime, it is never redistributed by eye serve, and it
	// is labelled as what it is everywhere it appears.
	AccessUndocumentedPersonal Access = "undocumented_personal"
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

// Personal reports whether a source is an operator's own undocumented one.
//
// These carry two consequences everywhere they are used: they need a runtime
// opt-in beyond the registry, and eye must never redistribute what they return.
func (a Access) Personal() bool { return a == AccessUndocumentedPersonal }

// Redistributable reports whether eye may serve this source's records to
// somebody else.
//
// Undocumented personal sources are not. Neither is anything eye reads under
// terms that permit personal use only — the registry records those as their own
// licence, and this is the code path that keeps `eye serve` honest about it.
func (s Source) Redistributable() bool { return !s.Access.Personal() }

// SamplesKind reports whether a record kind from this source is a sample of a
// moving signal rather than a statement worth diffing.
func (s Source) SamplesKind(kind string) bool {
	for _, k := range s.SampledKinds {
		if strings.EqualFold(k, kind) {
			return true
		}
	}
	return false
}

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

	// SampledKinds names the record kinds this source publishes as SAMPLES
	// of a continuously moving signal, rather than as statements about
	// persistent things.
	//
	// A position is a sample: it is different every time by definition, so
	// diffing two of them reports the reading back as news. A news article,
	// an incident, a delay or a timetable is a statement, and a statement
	// changing is exactly what is worth knowing about.
	//
	// It is per kind rather than per source because one feed does both:
	// RENFE's real-time feed publishes train positions, which are samples,
	// alongside trip delays, which are not. Marking the whole source would
	// throw away the half worth watching.
	//
	// Empty means everything this source publishes is watched. A missed
	// change is worse than a noisy one, so the default is to watch.
	SampledKinds []string `json:"sampled_kinds,omitempty"`

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
	case s.Access.Personal() && s.Notes == "":
		// An undocumented source has to say what it is and why it is here.
		// A registry entry nobody can explain is one nobody can review.
		return errors.Join(ErrInvalidSource, errors.New("an undocumented_personal source must carry notes explaining what it is"))
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
