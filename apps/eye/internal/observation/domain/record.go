package domain

import (
	"encoding/json"
	"errors"
	"time"
)

// ErrInvalidRecord is returned when a record breaks a domain invariant.
var ErrInvalidRecord = errors.New("record: invalid")

// Severity is a coarse, cross-source impact scale. It is deliberately small so
// that a DGT incident, a FIRMS thermal anomaly and a river level can be ranked
// in the same list without pretending they are the same kind of thing.
type Severity int

// The severity scale.
const (
	SeverityNone     Severity = 0
	SeverityInfo     Severity = 1
	SeverityLow      Severity = 2
	SeverityModerate Severity = 3
	SeverityHigh     Severity = 4
	SeverityCritical Severity = 5
)

// Valid reports whether the severity is inside the 0..5 scale.
func (s Severity) Valid() bool { return s >= SeverityNone && s <= SeverityCritical }

// Quality is the publisher's own claim about the data, preserved verbatim
// through normalization. SAIH, for example, states that its automatic station
// readings are not cross-checked; that warning must survive all the way to the
// UI instead of dissolving into a number.
type Quality string

// The quality levels eye distinguishes.
const (
	// QualityOfficial is a validated statement by the competent authority.
	QualityOfficial Quality = "official"
	// QualityValidated has been reviewed by the publisher but is not an
	// official declaration.
	QualityValidated Quality = "validated"
	// QualityPreliminary is automatic, unreviewed sensor output.
	QualityPreliminary Quality = "preliminary"
	// QualityInferred was produced by eye's own fusion engine and is never
	// an official notice.
	QualityInferred Quality = "inferred"
)

// Valid reports whether the quality is one of the known levels.
func (q Quality) Valid() bool {
	switch q {
	case QualityOfficial, QualityValidated, QualityPreliminary, QualityInferred:
		return true
	default:
		return false
	}
}

// Record is a single observation: something a source said was true at a given
// moment. Records are append-only; eye never edits history in place.
//
// ObservedAt and FetchedAt are kept apart on purpose. Their difference is the
// source latency, and without it eye would be selling thirteen-minute-old data
// as "live".
type Record struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Kind   string `json:"kind"`
	Topic  string `json:"topic"`

	// ObservedAt is when the phenomenon happened or was measured, as
	// reported by the source.
	ObservedAt time.Time `json:"observed_at"`
	// FetchedAt is when eye retrieved it.
	FetchedAt time.Time `json:"fetched_at"`

	// ValidFrom and ValidUntil bound the period the statement applies to,
	// for sources that publish forecasts or scheduled restrictions.
	ValidFrom  *time.Time `json:"valid_from,omitempty"`
	ValidUntil *time.Time `json:"valid_until,omitempty"`

	// Position is the point location, when the source provides one.
	Position *Point `json:"position,omitempty"`
	// Geometry is optional GeoJSON for anything a point cannot express:
	// road segments, fire perimeters, affected areas.
	Geometry json.RawMessage `json:"geometry,omitempty"`

	Title       string `json:"title"`
	Description string `json:"description,omitempty"`

	Severity Severity `json:"severity"`
	// Confidence is eye's own confidence in the observation, never the
	// source's internal score. A FIRMS "confidence" field is product
	// metadata and belongs in Payload, not here.
	Confidence float32 `json:"confidence"`
	Quality    Quality `json:"quality"`

	// EntityID links the observation to the persistent thing it describes,
	// when one has been resolved.
	EntityID *string `json:"entity_id,omitempty"`

	// DedupeKey is the stable fingerprint used to recognize the same
	// observation arriving from several catalogs.
	DedupeKey string `json:"dedupe_key,omitempty"`
	// LocalKey is the source's OWN stable identifier for the thing this
	// observation is about: an aircraft's hex, a stop and a line, a
	// dataset's name. It is what lets eye recognise the same thing arriving
	// from one source twice, and it must exclude everything that changes —
	// a position, a delay, a status. When a source publishes one it beats
	// the computed fingerprint, which has to guess from content.
	LocalKey string `json:"local_key,omitempty"`
	// ExpiresAt drives retention. Movement data (ADS-B, GTFS-RT) is short
	// lived by design.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`

	// Payload is the source-specific remainder, kept as received.
	Payload json.RawMessage `json:"payload,omitempty"`

	Provenance Provenance `json:"provenance"`
}

// Latency reports how stale the observation already was when eye fetched it.
// A negative value means the source dated it in the future, which is a data
// problem worth surfacing rather than clamping away.
func (r Record) Latency() time.Duration {
	return r.FetchedAt.Sub(r.ObservedAt)
}

// Validate enforces the invariants every record must satisfy before storage.
func (r Record) Validate() error {
	switch {
	case r.ID == "":
		return errors.Join(ErrInvalidRecord, errors.New("empty id"))
	case r.Source == "":
		return errors.Join(ErrInvalidRecord, errors.New("empty source"))
	case r.Topic == "":
		return errors.Join(ErrInvalidRecord, errors.New("empty topic"))
	case r.ObservedAt.IsZero():
		return errors.Join(ErrInvalidRecord, errors.New("zero observed_at"))
	case r.FetchedAt.IsZero():
		return errors.Join(ErrInvalidRecord, errors.New("zero fetched_at"))
	case !r.Severity.Valid():
		return errors.Join(ErrInvalidRecord, errors.New("severity out of 0..5 range"))
	case r.Confidence < 0 || r.Confidence > 1:
		return errors.Join(ErrInvalidRecord, errors.New("confidence out of 0..1 range"))
	case !r.Quality.Valid():
		return errors.Join(ErrInvalidRecord, errors.New("unknown quality"))
	case r.Position != nil && !r.Position.Valid():
		return errors.Join(ErrInvalidRecord, errors.New("position outside WGS84"))
	}
	return r.Provenance.Validate()
}
