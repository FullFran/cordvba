package domain

import (
	"errors"
	"time"
)

// ErrMissingProvenance is returned when a record or entity reaches the store
// without the evidence needed to audit it. eye treats provenance as part of
// the data, never as decoration.
var ErrMissingProvenance = errors.New("provenance: publisher, source URL and license are all required")

// Provenance records where an observation came from and under what terms it
// may be reused. Every Record and Entity carries one.
//
// RawHash points back at the byte-for-byte original response kept in the raw
// cache, so any alert eye emits can be replayed from its evidence.
type Provenance struct {
	// Publisher is the authority that published the data, not the software
	// that fetched it: "DGT", "AEMET", "Ayuntamiento de Cordoba".
	Publisher string `json:"publisher"`

	// SourceURL is the public, documented URL the data came from.
	SourceURL string `json:"source_url"`

	// License is the reuse license as declared by the publisher. When a
	// catalog leaves it blank, store it as "unspecified" rather than
	// guessing: an unknown license is a fact worth keeping.
	License string `json:"license"`

	// FetchedAt is when eye retrieved the bytes.
	FetchedAt time.Time `json:"fetched_at"`

	// RawHash is the hex SHA-256 of the exact raw payload this observation
	// was normalized from.
	RawHash string `json:"raw_hash,omitempty"`
}

// LicenseUnspecified is the placeholder for sources that publish data without
// declaring reuse terms. It is a warning, not a permission.
const LicenseUnspecified = "unspecified"

// Validate reports whether the provenance is complete enough to be stored.
func (p Provenance) Validate() error {
	if p.Publisher == "" || p.SourceURL == "" || p.License == "" {
		return ErrMissingProvenance
	}
	return nil
}
