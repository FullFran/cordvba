package domain

import (
	"context"
	"sort"
	"strings"
	"time"
)

// RecordStore persists observations. Records are append-only: eye does not
// rewrite history in place.
type RecordStore interface {
	// Append stores records, ignoring ones already present by ID, and
	// returns how many were new.
	Append(ctx context.Context, records []Record) (int, error)
	// Query returns records matching a filter, newest first.
	Query(ctx context.Context, f Filter) ([]Record, error)
}

// EntityStore persists inventory.
type EntityStore interface {
	// Upsert stores entities, refreshing LastSeen on ones already known.
	Upsert(ctx context.Context, entities []Entity) (int, error)
	// Entities returns inventory matching a filter.
	Entities(ctx context.Context, f Filter) ([]Entity, error)
}

// SnapshotStore holds what each source reported on its last SUCCESSFUL poll.
//
// Changes are measured against this rather than against the append-only
// history, which grows without bound and mixes every poll together. The
// snapshot is derived state: losing it costs the next tick's changes and
// nothing else.
type SnapshotStore interface {
	// Snapshot returns what a source last reported. known is false when eye
	// has never held a snapshot for it, which is different from a source
	// that reported nothing: on a first sighting there is nothing to compare
	// against, so nothing may be claimed to have appeared.
	Snapshot(ctx context.Context, source string) (records []Record, known bool, err error)
	// SaveSnapshot replaces a source's snapshot with what it just reported.
	SaveSnapshot(ctx context.Context, source string, records []Record) error
}

// Filter selects a slice of what eye knows, in space, time and subject.
type Filter struct {
	Topics  []string
	Sources []string
	Kinds   []string

	// Since and Until bound ObservedAt. Filtering on FetchedAt would ask
	// "when did we look", which is rarely the question.
	Since *time.Time
	Until *time.Time

	// BBox restricts to a bounding box; Near and RadiusKm to a circle.
	BBox     *BBox
	Near     *Point
	RadiusKm float64

	MinSeverity Severity

	// Text matches the title and description, case-insensitively.
	Text string

	// Limit caps the result. Zero means no cap.
	Limit int
}

// MatchRecord reports whether a record satisfies the filter.
func (f Filter) MatchRecord(r Record) bool {
	switch {
	case !matchAny(f.Topics, r.Topic):
		return false
	case !matchAny(f.Sources, r.Source):
		return false
	case !matchAny(f.Kinds, r.Kind):
		return false
	case f.Since != nil && r.ObservedAt.Before(*f.Since):
		return false
	case f.Until != nil && r.ObservedAt.After(*f.Until):
		return false
	case r.Severity < f.MinSeverity:
		return false
	case !f.matchText(r.Title, r.Description):
		return false
	}
	return f.matchPosition(r.Position)
}

// MatchEntity reports whether an entity satisfies the filter.
func (f Filter) MatchEntity(e Entity) bool {
	switch {
	case !matchAny(f.Topics, e.Topic):
		return false
	case !matchAny(f.Sources, e.Source):
		return false
	case !matchAny(f.Kinds, e.Kind):
		return false
	case !f.matchText(e.Title, e.Description):
		return false
	}
	return f.matchPosition(e.Position)
}

// matchPosition applies the spatial part of the filter. A record with no
// position passes only when no spatial filter was requested: eye does not
// invent coordinates to satisfy a query.
func (f Filter) matchPosition(p *Point) bool {
	if f.BBox == nil && f.Near == nil {
		return true
	}
	if p == nil {
		return false
	}
	if f.BBox != nil && !f.BBox.Contains(*p) {
		return false
	}
	if f.Near != nil && f.RadiusKm > 0 && f.Near.DistanceKm(*p) > f.RadiusKm {
		return false
	}
	return true
}

// matchText applies the free-text part of the filter.
//
// Two behaviours worth stating, both learned from the data rather than assumed:
//
//   - Accents are folded. eye reads Spanish sources and nobody types "CÓRDOBA"
//     with the accent when searching. Without folding, the camera on the A-4 is
//     unfindable by the name of the city it sits outside.
//   - Every word must appear, in any order, across the fields together. A
//     search for "A-4 Córdoba" should find "A-4 km 399.1 · CÓRDOBA", which a
//     contiguous substring match never would.
func (f Filter) matchText(fields ...string) bool {
	if f.Text == "" {
		return true
	}

	haystack := Fold(strings.Join(fields, " "))
	for _, word := range strings.Fields(Fold(f.Text)) {
		if !strings.Contains(haystack, word) {
			return false
		}
	}
	return true
}

// accentFolding maps the accented characters Spanish sources actually emit onto
// their unaccented forms. It is a table rather than a Unicode normalisation
// dependency: the alphabet is small, known, and does not change.
var accentFolding = map[rune]rune{
	'á': 'a', 'à': 'a', 'ä': 'a', 'â': 'a', 'ã': 'a', 'å': 'a',
	'é': 'e', 'è': 'e', 'ë': 'e', 'ê': 'e',
	'í': 'i', 'ì': 'i', 'ï': 'i', 'î': 'i',
	'ó': 'o', 'ò': 'o', 'ö': 'o', 'ô': 'o', 'õ': 'o',
	'ú': 'u', 'ù': 'u', 'ü': 'u', 'û': 'u',
	'ñ': 'n', 'ç': 'c', 'ý': 'y',
}

// Fold lowercases a string and strips the accents Spanish text carries, so a
// search for "cordoba" finds "CÓRDOBA".
//
// Ñ folds to N deliberately. It is a distinct letter in Spanish, but somebody
// searching for "espana" should still find "España", and no eye query
// distinguishes the two.
func Fold(s string) string {
	var sb strings.Builder
	sb.Grow(len(s))

	for _, r := range strings.ToLower(s) {
		if folded, ok := accentFolding[r]; ok {
			sb.WriteRune(folded)
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// matchAny reports whether value is in allowed, treating an empty list as
// "no restriction".
func matchAny(allowed []string, value string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, a := range allowed {
		if strings.EqualFold(a, value) {
			return true
		}
	}
	return false
}

// SortRecordsNewestFirst orders records by observation time, most recent first.
func SortRecordsNewestFirst(records []Record) {
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].ObservedAt.After(records[j].ObservedAt)
	})
}

// ApplyLimit truncates a slice to the filter's limit.
func ApplyLimit[T any](items []T, limit int) []T {
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}
