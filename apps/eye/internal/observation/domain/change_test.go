package domain_test

import (
	"encoding/json"
	"testing"
	"time"

	observation "github.com/FullFran/eye/internal/observation/domain"
)

var now = time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

// subject builds one observation of a thing, as a source would report it.
func subject(title string, severity observation.Severity) observation.Record {
	return observation.Record{
		ID: "src:" + title, Source: "src", Kind: "event", Topic: "events",
		ObservedAt: now, FetchedAt: now,
		Title: title, Severity: severity, Confidence: 1,
		Quality: observation.QualityOfficial,
		Provenance: observation.Provenance{
			Publisher: "IMAE", SourceURL: "https://example.test",
			License: "unspecified", FetchedAt: now, RawHash: "abc",
		},
	}
}

// changeOf reads the change payload back.
func changeOf(t *testing.T, r observation.Record) observation.Change {
	t.Helper()

	var c observation.Change
	if err := json.Unmarshal(r.Payload, &c); err != nil {
		t.Fatalf("change payload: %v", err)
	}
	return c
}

func TestDetectChangesReportsWhatIsNew(t *testing.T) {
	t.Parallel()

	current := []observation.Record{subject("Pablo López", observation.SeverityInfo)}

	changes := observation.DetectChanges(nil, current, now, true)
	if len(changes) != 1 {
		t.Fatalf("changes = %d, want 1", len(changes))
	}

	c := changeOf(t, changes[0])
	if c.Kind != observation.ChangeAppeared {
		t.Errorf("kind = %q, want appeared", c.Kind)
	}
	if changes[0].Kind != "change" {
		t.Errorf("a change must be stored as a change record, got kind %q", changes[0].Kind)
	}
	// A change is an observation like any other, so it must be storable.
	if err := changes[0].Validate(); err != nil {
		t.Errorf("the change record does not validate: %v", err)
	}
	// It must be traceable to the observation it was derived from.
	if c.RecordID != current[0].ID {
		t.Errorf("RecordID = %q, want %q", c.RecordID, current[0].ID)
	}
}

func TestDetectChangesReportsNothingWhenNothingMoved(t *testing.T) {
	t.Parallel()

	before := []observation.Record{subject("Pablo López", observation.SeverityInfo)}
	after := []observation.Record{subject("Pablo López", observation.SeverityInfo)}

	if changes := observation.DetectChanges(before, after, now, true); len(changes) != 0 {
		t.Errorf("changes = %d on an unchanged poll, want none:\n%+v", len(changes), changes)
	}
}

// Every poll moves ObservedAt and FetchedAt. Reporting those as changes would
// make every tick a wall of noise and hide the real ones.
func TestDetectChangesIgnoresTheTimestampsThatAlwaysMove(t *testing.T) {
	t.Parallel()

	before := []observation.Record{subject("Pablo López", observation.SeverityInfo)}

	later := subject("Pablo López", observation.SeverityInfo)
	later.ObservedAt = now.Add(5 * time.Minute)
	later.FetchedAt = now.Add(5 * time.Minute)

	changes := observation.DetectChanges(before, []observation.Record{later}, now, true)
	if len(changes) != 0 {
		t.Errorf("a re-poll produced %d changes:\n%+v", len(changes), changes)
	}
}

func TestDetectChangesReportsFieldsThatMoved(t *testing.T) {
	t.Parallel()

	before := subject("Pablo López", observation.SeverityInfo)
	start := now.Add(9 * time.Hour)
	before.ValidFrom = &start

	after := subject("Pablo López", observation.SeverityModerate)
	moved := start.Add(30 * time.Minute)
	after.ValidFrom = &moved
	after.Description = "Retrasado"

	changes := observation.DetectChanges(
		[]observation.Record{before}, []observation.Record{after}, now, true)
	if len(changes) != 1 {
		t.Fatalf("changes = %d, want 1", len(changes))
	}

	c := changeOf(t, changes[0])
	if c.Kind != observation.ChangeUpdated {
		t.Errorf("kind = %q, want updated", c.Kind)
	}

	moved3 := map[string]observation.FieldChange{}
	for _, f := range c.Fields {
		moved3[f.Field] = f
	}
	for _, field := range []string{"valid_from", "severity", "description"} {
		if _, ok := moved3[field]; !ok {
			t.Errorf("%s changed and was not reported; got %+v", field, c.Fields)
		}
	}
	// The old value has to survive, or the change says nothing useful.
	if f := moved3["valid_from"]; f.From == "" || f.From == f.To {
		t.Errorf("valid_from change = %+v, want both sides", f)
	}
}

// This is the hard case, and the one that makes naive versions lie. A record
// that stops arriving from a source that is FAILING says nothing at all.
func TestDetectChangesDoesNotCallASilentSourceAnEnding(t *testing.T) {
	t.Parallel()

	before := []observation.Record{subject("Pablo López", observation.SeverityInfo)}

	changes := observation.DetectChanges(before, nil, now, false)
	if len(changes) != 0 {
		t.Errorf("a failing source produced %d endings:\n%+v", len(changes), changes)
	}
}

func TestDetectChangesReportsADisappearanceFromAHealthySource(t *testing.T) {
	t.Parallel()

	before := []observation.Record{subject("Pablo López", observation.SeverityInfo)}

	changes := observation.DetectChanges(before, nil, now, true)
	if len(changes) != 1 {
		t.Fatalf("changes = %d, want 1", len(changes))
	}
	if c := changeOf(t, changes[0]); c.Kind != observation.ChangeDisappeared {
		t.Errorf("kind = %q, want disappeared", c.Kind)
	}
}

// A record eye cannot identify cannot be paired, so it cannot be diffed. It
// must not be reported as appearing on every single poll.
func TestDetectChangesSkipsUnidentifiableRecords(t *testing.T) {
	t.Parallel()

	blank := subject("   ", observation.SeverityInfo)

	if changes := observation.DetectChanges(nil, []observation.Record{blank}, now, true); len(changes) != 0 {
		t.Errorf("an unidentifiable record produced %d changes", len(changes))
	}
}

func TestDetectChangesHandlesSeveralThingsAtOnce(t *testing.T) {
	t.Parallel()

	before := []observation.Record{
		subject("Concierto A", observation.SeverityInfo),
		subject("Concierto B", observation.SeverityInfo),
	}
	after := []observation.Record{
		subject("Concierto A", observation.SeverityHigh), // changed
		subject("Concierto C", observation.SeverityInfo), // new; B is gone
	}

	counts := map[string]int{}
	for _, ch := range observation.DetectChanges(before, after, now, true) {
		counts[changeOf(t, ch).Kind]++
	}

	want := map[string]int{
		observation.ChangeUpdated:     1,
		observation.ChangeAppeared:    1,
		observation.ChangeDisappeared: 1,
	}
	for kind, n := range want {
		if counts[kind] != n {
			t.Errorf("%s = %d, want %d (got %v)", kind, counts[kind], n, counts)
		}
	}
}

// Two changes to the same thing on the same tick must not collide in the
// store, and the same tick re-run must not duplicate them.
func TestDetectChangesGivesEachChangeAStableID(t *testing.T) {
	t.Parallel()

	before := []observation.Record{subject("Concierto A", observation.SeverityInfo)}
	after := []observation.Record{subject("Concierto A", observation.SeverityHigh)}

	first := observation.DetectChanges(before, after, now, true)
	again := observation.DetectChanges(before, after, now, true)

	if len(first) != 1 || len(again) != 1 {
		t.Fatalf("changes = %d and %d, want 1 each", len(first), len(again))
	}
	if first[0].ID != again[0].ID {
		t.Errorf("the same change got two IDs:\n%s\n%s", first[0].ID, again[0].ID)
	}
}
