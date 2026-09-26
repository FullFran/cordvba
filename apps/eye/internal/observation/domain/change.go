package domain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"
)

// A change is itself an observation — see
// docs/adr/0009-identity-before-fusion.md. It is emitted as a Record of kind
// `change`, so it lands in the timeline, is queryable by topic and time,
// expires under the same retention, and carries the provenance of the thing it
// describes. No second timeline, no new storage concept.

// ChangeKind values.
const (
	// ChangeAppeared is a thing a source had not reported before.
	ChangeAppeared = "appeared"
	// ChangeUpdated is a watched field moving.
	ChangeUpdated = "updated"
	// ChangeDisappeared is a thing a HEALTHY source stopped reporting.
	ChangeDisappeared = "disappeared"
)

// ChangeKindRecord is the record kind every change is stored under.
const ChangeKindRecord = "change"

// changeTTL is how long changes are kept. They are derived, they are numerous,
// and the observations they were derived from remain. ADR-0009 requires
// retention to cover them from the start rather than as a later rescue.
const changeTTL = 30 * 24 * time.Hour

// FieldChange is one watched field that moved.
type FieldChange struct {
	Field string `json:"field"`
	From  string `json:"from"`
	To    string `json:"to"`
}

// Change is the payload of a change record.
type Change struct {
	Kind string `json:"kind"`
	// RecordID is the observation this change was derived from — the current
	// one, or for a disappearance the last one seen.
	RecordID string `json:"record_id"`
	// Identity is the key the two observations were paired on, so a reader
	// can see why eye considered them the same thing.
	Identity string        `json:"identity"`
	Fields   []FieldChange `json:"fields,omitempty"`
}

// DetectChanges compares what a source reports now against what it reported
// before, and emits one change record per difference.
//
// sourceHealthy is the whole difficulty of this feature. A record that stops
// arriving may mean the thing ended, or that the feed broke, or that our poll
// failed. Disappearances are reported ONLY when the source's latest poll
// actually succeeded; a failing or stale source produces silence, never an
// ending. eye already persists that health, so this is settled by data rather
// than by a guess.
//
// at is the time the comparison was made, which is when the change was
// observed — the change did not happen when the underlying thing did.
func DetectChanges(previous, current []Record, at time.Time, sourceHealthy bool) []Record {
	before, after := byIdentity(previous), byIdentity(current)

	var changes []Record

	// Identities are walked in order so a tick produces the same output
	// twice, which matters for both tests and stored IDs.
	for _, id := range sortedKeys(after) {
		now := after[id]
		then, seen := before[id]
		if !seen {
			changes = append(changes, changeRecord(now, Change{
				Kind: ChangeAppeared, RecordID: now.ID, Identity: id,
			}, at))
			continue
		}
		if fields := Diff(then, now); len(fields) > 0 {
			changes = append(changes, changeRecord(now, Change{
				Kind: ChangeUpdated, RecordID: now.ID, Identity: id, Fields: fields,
			}, at))
		}
	}

	if !sourceHealthy {
		return changes
	}
	for _, id := range sortedKeys(before) {
		if _, still := after[id]; still {
			continue
		}
		changes = append(changes, changeRecord(before[id], Change{
			Kind: ChangeDisappeared, RecordID: before[id].ID, Identity: id,
		}, at))
	}
	return changes
}

// byIdentity groups observations by what they describe.
//
// A record eye cannot identify is dropped rather than given a placeholder
// identity: an unidentifiable record would otherwise appear to arrive new on
// every single poll, or worse, pair with an unrelated one.
func byIdentity(records []Record) map[string]Record {
	out := make(map[string]Record, len(records))
	for _, r := range records {
		if id := r.Identity(); id != "" {
			out[id] = r
		}
	}
	return out
}

// sortedKeys returns map keys in a stable order.
func sortedKeys(m map[string]Record) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// watchedFields are the fields a change is worth reporting for.
//
// ObservedAt and FetchedAt are deliberately absent: every poll moves them, so
// reporting them would bury the real changes under a wall of noise. Payload is
// absent too — a source reshuffling its own JSON is not news.
var watchedFields = []struct {
	name string
	of   func(Record) string
}{
	{"title", func(r Record) string { return r.Title }},
	{"description", func(r Record) string { return r.Description }},
	{"severity", func(r Record) string { return strconv.Itoa(int(r.Severity)) }},
	{"quality", func(r Record) string { return string(r.Quality) }},
	{"valid_from", func(r Record) string { return formatTime(r.ValidFrom) }},
	{"valid_until", func(r Record) string { return formatTime(r.ValidUntil) }},
	{"position", func(r Record) string { return r.coarsePosition() }},
}

// Diff reports the watched fields that differ between two observations of the
// same thing.
func Diff(previous, current Record) []FieldChange {
	var out []FieldChange
	for _, f := range watchedFields {
		from, to := f.of(previous), f.of(current)
		if from != to {
			out = append(out, FieldChange{Field: f.name, From: from, To: to})
		}
	}
	return out
}

// changeRecord wraps a change as an observation.
//
// It inherits the subject's topic, severity and provenance: a change about a
// DGT incident is still published by the DGT under the DGT's licence, and
// saying otherwise would break the property that every stored fact can be
// traced to a public source.
func changeRecord(subject Record, c Change, at time.Time) Record {
	payload, _ := json.Marshal(c)
	expires := at.Add(changeTTL)

	return Record{
		// The ID is derived from what changed rather than from when it was
		// noticed, so re-running one tick cannot duplicate it.
		ID:     fmt.Sprintf("change:%s:%s:%s", subject.Source, c.Kind, changeDigest(c)),
		Source: subject.Source,
		Kind:   ChangeKindRecord,
		Topic:  subject.Topic,
		// The change was observed now. The thing it describes has its own
		// timestamps, and conflating them would misdate the change.
		ObservedAt: at,
		FetchedAt:  at,
		Position:   subject.Position,
		Title:      subject.Title,
		Severity:   subject.Severity,
		Confidence: 1,
		// Derived from observations eye holds, not published by anyone.
		Quality:    QualityInferred,
		DedupeKey:  c.Identity,
		ExpiresAt:  &expires,
		Payload:    payload,
		Provenance: subject.Provenance,
	}
}

// changeDigest is a stable identifier for one change.
func changeDigest(c Change) string {
	parts := c.Identity
	for _, f := range c.Fields {
		parts += "|" + f.Field + "=" + f.To
	}
	return HashHex(parts)
}

// formatTime renders an optional timestamp, with absence distinguishable from
// any real value.
func formatTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
