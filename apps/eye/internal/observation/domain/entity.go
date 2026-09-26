package domain

import (
	"encoding/json"
	"errors"
	"time"
)

// ErrInvalidEntity is returned when an entity breaks a domain invariant.
var ErrInvalidEntity = errors.New("entity: invalid")

// Entity is something that persists across observations: a traffic camera, a
// reservoir gauge, a bus route, a venue.
//
// Keeping Entity apart from Record is the difference between "an aircraft" and
// "the position of an aircraft at 12:01:03". Collapsing the two produces a
// table that is neither an inventory nor a time series.
type Entity struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Kind   string `json:"kind"`
	Topic  string `json:"topic"`

	Title       string `json:"title"`
	Description string `json:"description,omitempty"`

	Position *Point          `json:"position,omitempty"`
	Geometry json.RawMessage `json:"geometry,omitempty"`

	// FirstSeen and LastSeen track eye's own observation window for the
	// entity, not the lifetime of the real-world thing.
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`

	Payload json.RawMessage `json:"payload,omitempty"`

	Provenance Provenance `json:"provenance"`
}

// Validate enforces the invariants every entity must satisfy before storage.
func (e Entity) Validate() error {
	switch {
	case e.ID == "":
		return errors.Join(ErrInvalidEntity, errors.New("empty id"))
	case e.Source == "":
		return errors.Join(ErrInvalidEntity, errors.New("empty source"))
	case e.Kind == "":
		return errors.Join(ErrInvalidEntity, errors.New("empty kind"))
	case e.Topic == "":
		return errors.Join(ErrInvalidEntity, errors.New("empty topic"))
	case e.Position != nil && !e.Position.Valid():
		return errors.Join(ErrInvalidEntity, errors.New("position outside WGS84"))
	case !e.LastSeen.IsZero() && e.LastSeen.Before(e.FirstSeen):
		return errors.Join(ErrInvalidEntity, errors.New("last_seen before first_seen"))
	}
	return e.Provenance.Validate()
}
