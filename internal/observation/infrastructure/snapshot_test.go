package infrastructure_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	domain "github.com/FullFran/eye/internal/observation/domain"
	store "github.com/FullFran/eye/internal/observation/infrastructure"
)

var snapshotTime = time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

// snapshotRecord builds a storable observation.
func snapshotRecord(title string, severity domain.Severity) domain.Record {
	return domain.Record{
		ID: "feed:" + title, Source: "feed", Kind: "event", Topic: "events",
		ObservedAt: snapshotTime, FetchedAt: snapshotTime,
		Title: title, Severity: severity, Confidence: 1,
		Quality: domain.QualityOfficial,
		Provenance: domain.Provenance{
			Publisher: "IMAE", SourceURL: "https://example.test",
			License: "unspecified", FetchedAt: snapshotTime, RawHash: "abc",
		},
	}
}

// Never asked and found nothing are different answers, and change detection
// depends on telling them apart.
func TestSnapshotDistinguishesNeverLookedFromFoundNothing(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := context.Background()

	if _, known, err := s.Snapshot(ctx, "feed"); err != nil || known {
		t.Errorf("Snapshot() known = %v, err = %v; want false, nil", known, err)
	}

	if err := s.SaveSnapshot(ctx, "feed", nil); err != nil {
		t.Fatalf("SaveSnapshot() = %v", err)
	}

	records, known, err := s.Snapshot(ctx, "feed")
	if err != nil {
		t.Fatalf("Snapshot() = %v", err)
	}
	if !known {
		t.Error("a source eye has looked at reads as never looked at")
	}
	if len(records) != 0 {
		t.Errorf("records = %d, want none", len(records))
	}
}

func TestSnapshotRoundTripsARecord(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := context.Background()

	want := snapshotRecord("Concierto A", domain.SeverityHigh)
	if err := s.SaveSnapshot(ctx, "feed", []domain.Record{want}); err != nil {
		t.Fatalf("SaveSnapshot() = %v", err)
	}

	records, _, err := s.Snapshot(ctx, "feed")
	if err != nil {
		t.Fatalf("Snapshot() = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}

	got := records[0]
	// The fields a diff is computed from have to survive storage, or the
	// next poll reports changes that did not happen.
	if got.ID != want.ID || got.Title != want.Title || got.Severity != want.Severity {
		t.Errorf("record came back as %+v", got)
	}
	if got.Fingerprint() != want.Fingerprint() {
		t.Errorf("identity did not survive:\n%s\n%s", got.Fingerprint(), want.Fingerprint())
	}
}

// The snapshot is what the source says now. A record left over from an earlier
// poll would be reported as disappearing on every tick from then on.
func TestSaveSnapshotReplacesRatherThanAccumulates(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := context.Background()

	first := []domain.Record{
		snapshotRecord("Concierto A", domain.SeverityInfo),
		snapshotRecord("Concierto B", domain.SeverityInfo),
	}
	if err := s.SaveSnapshot(ctx, "feed", first); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSnapshot(ctx, "feed", first[:1]); err != nil {
		t.Fatal(err)
	}

	records, _, err := s.Snapshot(ctx, "feed")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Errorf("records = %d, want the snapshot replaced, not merged", len(records))
	}
}

func TestSnapshotsAreKeptPerSource(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := context.Background()

	if err := s.SaveSnapshot(ctx, "feed-a", []domain.Record{snapshotRecord("A", domain.SeverityInfo)}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSnapshot(ctx, "feed-b", []domain.Record{snapshotRecord("B", domain.SeverityInfo)}); err != nil {
		t.Fatal(err)
	}

	a, _, err := s.Snapshot(ctx, "feed-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 1 || a[0].Title != "A" {
		t.Errorf("feed-a snapshot = %+v", a)
	}
	if _, known, _ := s.Snapshot(ctx, "feed-c"); known {
		t.Error("an untouched source reads as known")
	}
}

// A record eye cannot identify cannot be paired on the next poll, so storing
// it would only take space.
func TestSaveSnapshotSkipsUnidentifiableRecords(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := context.Background()

	blank := snapshotRecord("   ", domain.SeverityInfo)
	if err := s.SaveSnapshot(ctx, "feed", []domain.Record{blank}); err != nil {
		t.Fatal(err)
	}

	records, known, err := s.Snapshot(ctx, "feed")
	if err != nil || !known {
		t.Fatalf("Snapshot() known = %v, err = %v", known, err)
	}
	if len(records) != 0 {
		t.Errorf("records = %d, want the unidentifiable one skipped", len(records))
	}
}

// The snapshot has to outlive the process, or every restart would report the
// whole city as new.
func TestSnapshotSurvivesReopening(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "eye.db")
	ctx := context.Background()

	first, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite() = %v", err)
	}
	if err := first.SaveSnapshot(ctx, "feed", []domain.Record{snapshotRecord("A", domain.SeverityInfo)}); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatalf("reopen = %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	records, known, err := second.Snapshot(ctx, "feed")
	if err != nil || !known || len(records) != 1 {
		t.Errorf("after reopening: records = %d, known = %v, err = %v", len(records), known, err)
	}
}

// Changes are derived and numerous. ADR-0009 requires retention to cover them
// from the start rather than as a later rescue, so the prune that clears
// expired observations has to clear them too.
func TestPruneClearsExpiredChanges(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	ctx := context.Background()

	previous := []domain.Record{snapshotRecord("Concierto A", domain.SeverityInfo)}
	current := []domain.Record{snapshotRecord("Concierto A", domain.SeverityHigh)}

	changes := domain.DetectChanges(previous, current, snapshotTime, true)
	if len(changes) != 1 {
		t.Fatalf("changes = %d, want 1", len(changes))
	}
	if _, err := s.Append(ctx, changes); err != nil {
		t.Fatal(err)
	}

	// Still inside its window.
	if pruned, err := s.Prune(ctx, snapshotTime.Add(time.Hour)); err != nil || pruned != 0 {
		t.Errorf("Prune() = %d, %v; want the change kept", pruned, err)
	}

	// Well past it.
	pruned, err := s.Prune(ctx, snapshotTime.Add(365*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if pruned != 1 {
		t.Errorf("Prune() = %d, want the expired change removed", pruned)
	}

	left, err := s.Query(ctx, domain.Filter{Kinds: []string{domain.ChangeKindRecord}})
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Errorf("%d expired changes survived the prune", len(left))
	}
}

// A store written by an older eye must keep working. The column arrived after
// the first release, and a migration that fails leaves somebody with a store
// they cannot open.
func TestOpeningAStoreWrittenBeforeLocalKey(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "eye.db")
	ctx := context.Background()

	// Build the store, then drop the column back out to imitate the old one.
	first, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite() = %v", err)
	}
	if _, err := first.Append(ctx, []domain.Record{snapshotRecord("Concierto A", domain.SeverityInfo)}); err != nil {
		t.Fatal(err)
	}
	if err := first.DropColumnForTest(ctx, "records", "local_key"); err != nil {
		t.Fatalf("imitate the old schema: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatalf("reopening a pre-migration store = %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	// The rows that were already there survive, and new ones carry the key.
	withKey := snapshotRecord("Concierto B", domain.SeverityInfo)
	withKey.LocalKey = "concierto-b"
	if _, err := second.Append(ctx, []domain.Record{withKey}); err != nil {
		t.Fatalf("Append() after migration = %v", err)
	}

	records, err := second.Query(ctx, domain.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %d, want the old row and the new one", len(records))
	}
	for _, r := range records {
		if r.Title == "Concierto B" && r.LocalKey != "concierto-b" {
			t.Errorf("LocalKey = %q, want it stored", r.LocalKey)
		}
	}
}
