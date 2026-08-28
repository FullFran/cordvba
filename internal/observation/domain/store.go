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
func (f Filter) matchText(fields ...string) bool {
	if f.Text == "" {
		return true
	}
	needle := strings.ToLower(f.Text)
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), needle) {
			return true
		}
	}
	return false
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
