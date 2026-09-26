package infrastructure_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domain "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	store "github.com/FullFran/cordvba/apps/eye/internal/observation/infrastructure"
)

// openStore creates a store in a temp directory.
func openStore(t *testing.T) *store.SQLiteStore {
	t.Helper()

	s, err := store.OpenSQLite(filepath.Join(t.TempDir(), "eye.db"))
	if err != nil {
		t.Fatalf("OpenSQLite() = %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// cordoba is the reference point for spatial tests.
var cordoba = domain.Point{Lat: 37.8882, Lon: -4.7794}

// fullRecord exercises every column, including the optional ones.
func fullRecord(id string, at time.Time) domain.Record {
	entityID := "cam-1"
	validFrom := at.Add(48 * time.Hour)
	expires := at.Add(72 * time.Hour)

	return domain.Record{
		ID: id, Source: "test", Kind: "news_item", Topic: "press",
		ObservedAt: at, FetchedAt: at.Add(time.Minute),
		ValidFrom: &validFrom, ExpiresAt: &expires,
		Position:    &cordoba,
		Geometry:    json.RawMessage(`{"type":"Point","coordinates":[-4.7794,37.8882]}`),
		Title:       "Velá de la Fuensanta",
		Description: "Plaza de la Fuensanta",
		Severity:    domain.SeverityModerate,
		Confidence:  0.75,
		Quality:     domain.QualityOfficial,
		EntityID:    &entityID,
		DedupeKey:   "dk-" + id,
		Payload:     json.RawMessage(`{"link":"https://example.org/1"}`),
		Provenance: domain.Provenance{
			Publisher: "Diario Córdoba", SourceURL: "https://example.org/1",
			License: "unspecified", FetchedAt: at.Add(time.Minute), RawHash: "abc123",
		},
	}
}

func TestSQLiteRoundTripsEveryField(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := openStore(t)
	at := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)

	want := fullRecord("r1", at)
	if added, err := s.Append(ctx, []domain.Record{want}); err != nil || added != 1 {
		t.Fatalf("Append() = %d, %v", added, err)
	}

	got, err := s.Query(ctx, domain.Filter{})
	if err != nil {
		t.Fatalf("Query() = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("records = %d, want 1", len(got))
	}
	g := got[0]

	checks := []struct {
		field string
		ok    bool
		why   string
	}{
		{field: "ObservedAt", ok: g.ObservedAt.Equal(want.ObservedAt)},
		{field: "FetchedAt", ok: g.FetchedAt.Equal(want.FetchedAt)},
		{field: "Latency", ok: g.Latency() == time.Minute, why: "the two timestamps were collapsed"},
		{field: "ValidFrom", ok: g.ValidFrom != nil && g.ValidFrom.Equal(*want.ValidFrom)},
		{field: "ExpiresAt", ok: g.ExpiresAt != nil, why: "retention would never fire"},
		{field: "Position", ok: g.Position != nil && g.Position.Lat == cordoba.Lat},
		{field: "Geometry", ok: len(g.Geometry) > 0},
		{field: "EntityID", ok: g.EntityID != nil && *g.EntityID == "cam-1"},
		{field: "DedupeKey", ok: g.DedupeKey == want.DedupeKey},
		{field: "Payload", ok: len(g.Payload) > 0},
		{field: "Severity", ok: g.Severity == want.Severity},
		{field: "Confidence", ok: g.Confidence == want.Confidence},
		{field: "Quality", ok: g.Quality == want.Quality},
		{field: "Description", ok: g.Description == want.Description},
		{field: "Provenance.Publisher", ok: g.Provenance.Publisher == want.Provenance.Publisher},
		{field: "Provenance.License", ok: g.Provenance.License == "unspecified"},
		{field: "Provenance.RawHash", ok: g.Provenance.RawHash == "abc123", why: "the evidence chain is broken"},
	}

	for _, c := range checks {
		if !c.ok {
			if c.why != "" {
				t.Errorf("%s did not survive the round trip — %s", c.field, c.why)
				continue
			}
			t.Errorf("%s did not survive the round trip", c.field)
		}
	}

	if err := g.Validate(); err != nil {
		t.Errorf("round-tripped record does not validate: %v", err)
	}
}

func TestSQLiteHandlesNullableFields(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := openStore(t)
	at := time.Now().UTC()

	minimal := domain.Record{
		ID: "min", Source: "test", Kind: "k", Topic: "press",
		ObservedAt: at, FetchedAt: at, Title: "No optionals",
		Quality: domain.QualityInferred, Confidence: 0,
		Provenance: domain.Provenance{
			Publisher: "P", SourceURL: "https://example.org", License: "unspecified", FetchedAt: at,
		},
	}
	if _, err := s.Append(ctx, []domain.Record{minimal}); err != nil {
		t.Fatalf("Append() = %v", err)
	}

	got, _ := s.Query(ctx, domain.Filter{})
	if got[0].Position != nil || got[0].ValidFrom != nil || got[0].ExpiresAt != nil || got[0].EntityID != nil {
		t.Errorf("absent optionals came back non-nil: %+v", got[0])
	}
}

// Records are append-only. Re-polling an unchanged feed must be a no-op.
func TestSQLiteAppendIsIdempotent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := openStore(t)
	at := time.Now().UTC()

	batch := []domain.Record{fullRecord("a", at), fullRecord("b", at)}

	if added, _ := s.Append(ctx, batch); added != 2 {
		t.Fatalf("first Append() = %d, want 2", added)
	}
	if added, _ := s.Append(ctx, batch); added != 0 {
		t.Fatalf("second Append() = %d, want 0", added)
	}

	records, _, err := s.Counts(ctx)
	if err != nil {
		t.Fatalf("Counts() = %v", err)
	}
	if records != 2 {
		t.Errorf("store holds %d records, want 2", records)
	}
}

func TestSQLiteSurvivesReopen(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "eye.db")
	at := time.Now().UTC()

	first, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite() = %v", err)
	}
	if _, err := first.Append(ctx, []domain.Record{fullRecord("persisted", at)}); err != nil {
		t.Fatalf("Append() = %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}

	// The whole point of the slice: what one run collected, the next run
	// can still answer from.
	second, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatalf("reopen = %v", err)
	}
	defer func() { _ = second.Close() }()

	got, _ := second.Query(ctx, domain.Filter{})
	if len(got) != 1 || got[0].ID != "persisted" {
		t.Errorf("after reopen the store holds %v", got)
	}
}

func TestSQLiteQueryFilters(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := openStore(t)
	now := time.Now().UTC()

	press := fullRecord("press", now)
	old := fullRecord("old", now.Add(-48*time.Hour))
	civic := fullRecord("civic", now)
	civic.Topic = "civic"
	civic.Title = "Anuncio de licitación"
	civic.Severity = domain.SeverityHigh
	far := fullRecord("far", now)
	far.Position = &domain.Point{Lat: 40.4168, Lon: -3.7038} // Madrid

	if _, err := s.Append(ctx, []domain.Record{press, old, civic, far}); err != nil {
		t.Fatalf("Append() = %v", err)
	}

	since := now.Add(-time.Hour)
	cases := []struct {
		name   string
		filter domain.Filter
		want   []string
	}{
		{name: "topic", filter: domain.Filter{Topics: []string{"civic"}}, want: []string{"civic"}},
		{name: "topic is case-insensitive", filter: domain.Filter{Topics: []string{"CIVIC"}}, want: []string{"civic"}},
		{name: "since", filter: domain.Filter{Topics: []string{"press"}, Since: &since}, want: []string{"press", "far"}},
		{name: "min severity", filter: domain.Filter{MinSeverity: domain.SeverityHigh}, want: []string{"civic"}},
		{name: "text", filter: domain.Filter{Text: "licitación"}, want: []string{"civic"}},
		{name: "text is case-insensitive", filter: domain.Filter{Text: "LICITACIÓN"}, want: []string{"civic"}},
		{name: "source", filter: domain.Filter{Sources: []string{"nope"}}, want: nil},
		{
			name:   "bbox",
			filter: domain.Filter{BBox: &domain.BBox{West: -5.6, South: 37.1, East: -4.0, North: 38.7}},
			want:   []string{"press", "old", "civic"},
		},
		{
			name:   "radius excludes Madrid",
			filter: domain.Filter{Near: &cordoba, RadiusKm: 20},
			want:   []string{"press", "old", "civic"},
		},
		{name: "limit", filter: domain.Filter{Limit: 2}, want: []string{"press", "civic"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := s.Query(ctx, tc.filter)
			if err != nil {
				t.Fatalf("Query() = %v", err)
			}

			ids := map[string]bool{}
			for _, r := range got {
				ids[r.ID] = true
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %d records %v, want %d %v", len(got), keys(ids), len(tc.want), tc.want)
			}
			for _, want := range tc.want {
				if !ids[want] {
					t.Errorf("missing %q; got %v", want, keys(ids))
				}
			}
		})
	}
}

func TestSQLiteQueryIsNewestFirst(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := openStore(t)
	now := time.Now().UTC()

	_, _ = s.Append(ctx, []domain.Record{
		fullRecord("oldest", now.Add(-3*time.Hour)),
		fullRecord("newest", now),
		fullRecord("middle", now.Add(-time.Hour)),
	})

	got, _ := s.Query(ctx, domain.Filter{})
	if got[0].ID != "newest" || got[2].ID != "oldest" {
		t.Errorf("order = %s, %s, %s", got[0].ID, got[1].ID, got[2].ID)
	}
}

// Retention is a DELETE that runs, not a promise in a document.
func TestSQLitePruneRemovesExpiredRecords(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := openStore(t)
	now := time.Now().UTC()

	expired := fullRecord("expired", now)
	past := now.Add(-time.Hour)
	expired.ExpiresAt = &past

	live := fullRecord("live", now)
	future := now.Add(72 * time.Hour)
	live.ExpiresAt = &future

	permanent := fullRecord("permanent", now)
	permanent.ExpiresAt = nil

	_, _ = s.Append(ctx, []domain.Record{expired, live, permanent})

	pruned, err := s.Prune(ctx, now)
	if err != nil {
		t.Fatalf("Prune() = %v", err)
	}
	if pruned != 1 {
		t.Errorf("pruned = %d, want 1", pruned)
	}

	got, _ := s.Query(ctx, domain.Filter{})
	if len(got) != 2 {
		t.Fatalf("after prune the store holds %d records, want 2", len(got))
	}
	for _, r := range got {
		if r.ID == "expired" {
			t.Error("an expired record survived the prune")
		}
	}
}

// RawHashInUse is the keep() lookup #74 wires into RawCache.Prune: a payload
// is kept as long as any surviving record OR entity still points at its hash.
// #132: retention.historical was, until now, enforced only on the collector
// path — records already stored through the daemon's own path still carry
// the expiry their adapter computed. This is the store-side half of the fix:
// a start-up reconciliation clears it, never sets one, and touches only the
// sources it is told to.
func TestSQLiteClearExpiryForSourcesOnlyClearsNamedSources(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := openStore(t)
	now := time.Now().UTC().Truncate(time.Microsecond)

	staleExpiry := now.Add(30 * 24 * time.Hour)
	historical := fullRecord("historical-1", now)
	historical.Source = "metar-cordoba"
	historical.ExpiresAt = &staleExpiry

	otherExpiry := now.Add(24 * time.Hour)
	untouched := fullRecord("untouched-1", now)
	untouched.Source = "adsb-lol"
	untouched.ExpiresAt = &otherExpiry

	alreadyNil := fullRecord("already-nil", now)
	alreadyNil.Source = "metar-cordoba"
	alreadyNil.ExpiresAt = nil

	if _, err := s.Append(ctx, []domain.Record{historical, untouched, alreadyNil}); err != nil {
		t.Fatalf("Append() = %v", err)
	}

	n, err := s.ClearExpiryForSources(ctx, []string{"metar-cordoba"})
	if err != nil {
		t.Fatalf("ClearExpiryForSources() = %v", err)
	}
	// Only historical-1 had a non-nil expiry to clear; already-nil has
	// nothing to touch, so it must not be counted.
	if n != 1 {
		t.Errorf("cleared = %d, want 1", n)
	}

	got, err := s.Query(ctx, domain.Filter{})
	if err != nil {
		t.Fatalf("Query() = %v", err)
	}
	for _, r := range got {
		switch r.ID {
		case "historical-1":
			if r.ExpiresAt != nil {
				t.Errorf("historical-1 still carries an expiry: %v", r.ExpiresAt)
			}
		case "untouched-1":
			if r.ExpiresAt == nil || !r.ExpiresAt.Equal(otherExpiry) {
				t.Errorf("untouched-1's expiry changed: got %v, want %v", r.ExpiresAt, otherExpiry)
			}
		}
	}
}

func TestSQLiteClearExpiryForSourcesEmptyListIsANoOp(t *testing.T) {
	t.Parallel()

	s := openStore(t)
	n, err := s.ClearExpiryForSources(context.Background(), nil)
	if err != nil {
		t.Fatalf("ClearExpiryForSources() = %v", err)
	}
	if n != 0 {
		t.Errorf("cleared = %d, want 0", n)
	}
}

func TestSQLiteRawHashInUse(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := openStore(t)
	now := time.Now().UTC()

	recordOnly := fullRecord("record-only", now)
	recordOnly.Provenance.RawHash = "hash-record-only"
	unreferenced := fullRecord("unreferenced", now)
	unreferenced.Provenance.RawHash = "hash-nobody-points-at"

	if _, err := s.Append(ctx, []domain.Record{recordOnly, unreferenced}); err != nil {
		t.Fatalf("Append() = %v", err)
	}

	entityOnly := domain.Entity{
		ID: "cam-1", Source: "test", Kind: "camera", Topic: "transport",
		Title: "Puente", FirstSeen: now, LastSeen: now,
		Provenance: domain.Provenance{
			Publisher: "P", SourceURL: "https://example.org", License: "unspecified",
			FetchedAt: now, RawHash: "hash-entity-only",
		},
	}
	if _, err := s.Upsert(ctx, []domain.Entity{entityOnly}); err != nil {
		t.Fatalf("Upsert() = %v", err)
	}

	cases := []struct {
		name string
		hash string
		want bool
	}{
		{name: "referenced by a record", hash: "hash-record-only", want: true},
		{name: "referenced by an entity", hash: "hash-entity-only", want: true},
		{name: "referenced by nothing", hash: "hash-nobody-points-at-x", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := s.RawHashInUse(ctx, tc.hash)
			if err != nil {
				t.Fatalf("RawHashInUse(%q) = %v", tc.hash, err)
			}
			if got != tc.want {
				t.Errorf("RawHashInUse(%q) = %v, want %v", tc.hash, got, tc.want)
			}
		})
	}
}

// The keep() lookup runs once per cached payload, every prune cycle — on the
// operator's own store, 1296 files in one afternoon. A per-hash SCAN of the
// whole records (or entities) table for each one would make the prune loop's
// cost grow with the product of both table sizes. This test is the executable
// record of that decision: it pins the query plan to an index SEARCH, so a
// future change that drops the index fails loudly here instead of quietly
// reintroducing the O(payloads × rows) scan.
func TestSQLiteRawHashLookupUsesAnIndexNotATableScan(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := openStore(t)

	plan, err := s.ExplainRawHashLookup(ctx, "any-hash")
	if err != nil {
		t.Fatalf("ExplainRawHashLookup() = %v", err)
	}
	for _, table := range []string{"records", "entities"} {
		if strings.Contains(plan, "SCAN "+table) {
			t.Errorf("plan scans %s instead of searching an index:\n%s", table, plan)
		}
	}
	if !strings.Contains(plan, "records_raw_hash") {
		t.Errorf("plan does not use the records_raw_hash index:\n%s", plan)
	}
	if !strings.Contains(plan, "entities_raw_hash") {
		t.Errorf("plan does not use the entities_raw_hash index:\n%s", plan)
	}
}

func TestSQLiteEntitiesUpsertKeepsFirstSeen(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := openStore(t)

	first := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	later := first.Add(200 * 24 * time.Hour)

	entity := func(seen time.Time, title string) domain.Entity {
		return domain.Entity{
			ID: "cam-1", Source: "test", Kind: "camera", Topic: "transport",
			Title: title, Position: &cordoba, FirstSeen: seen, LastSeen: seen,
			Provenance: domain.Provenance{
				Publisher: "Ayuntamiento", SourceURL: "https://example.org",
				License: "unspecified", FetchedAt: seen,
			},
		}
	}

	if added, _ := s.Upsert(ctx, []domain.Entity{entity(first, "Puente Romano")}); added != 1 {
		t.Fatal("first Upsert() should report a new entity")
	}
	if added, _ := s.Upsert(ctx, []domain.Entity{entity(later, "Puente Romano (renamed)")}); added != 0 {
		t.Fatal("second Upsert() should report no new entity")
	}

	got, err := s.Entities(ctx, domain.Filter{})
	if err != nil {
		t.Fatalf("Entities() = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("entities = %d, want 1", len(got))
	}
	if !got[0].FirstSeen.Equal(first) {
		t.Errorf("FirstSeen = %v, want the original %v", got[0].FirstSeen, first)
	}
	if !got[0].LastSeen.Equal(later) {
		t.Errorf("LastSeen = %v, want the refreshed %v", got[0].LastSeen, later)
	}
	if got[0].Title != "Puente Romano (renamed)" {
		t.Errorf("title was not refreshed: %q", got[0].Title)
	}
}

func TestSQLiteSourceState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := openStore(t)
	now := time.Now().UTC().Truncate(time.Microsecond)

	if err := s.SaveState(ctx, store.SourceState{
		SourceID: "boe", ETag: `"v1"`, LastModified: "Wed, 27 Aug 2026 10:00:00 GMT",
		LastAttempt: now, LastSuccess: now, Records: 137,
	}); err != nil {
		t.Fatalf("SaveState() = %v", err)
	}

	// A later failure must not erase the last known success: "when did this
	// source last work" is exactly what staleness needs.
	if err := s.SaveState(ctx, store.SourceState{
		SourceID: "boe", ETag: `"v1"`, LastAttempt: now.Add(time.Hour),
		ConsecutiveErrors: 2, LastError: "503 from upstream", Records: 137,
	}); err != nil {
		t.Fatalf("second SaveState() = %v", err)
	}

	states, err := s.States(ctx)
	if err != nil {
		t.Fatalf("States() = %v", err)
	}

	got := states["boe"]
	if got.ETag != `"v1"` {
		t.Errorf("etag = %q", got.ETag)
	}
	if !got.LastSuccess.Equal(now) {
		t.Errorf("LastSuccess = %v, want the earlier success %v to be retained", got.LastSuccess, now)
	}
	if got.ConsecutiveErrors != 2 || got.LastError == "" {
		t.Errorf("failure not recorded: %+v", got)
	}
	if h := got.Health(); h.SourceID != "boe" || h.ConsecutiveErrors != 2 {
		t.Errorf("Health() = %+v", h)
	}
}

func TestSQLiteEmptyBatches(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := openStore(t)

	if added, err := s.Append(ctx, nil); err != nil || added != 0 {
		t.Errorf("Append(nil) = %d, %v", added, err)
	}
	if added, err := s.Upsert(ctx, nil); err != nil || added != 0 {
		t.Errorf("Upsert(nil) = %d, %v", added, err)
	}
}

func TestOpenSQLiteCreatesParentDirectory(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", "deeper", "eye.db")
	s, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatalf("OpenSQLite() = %v", err)
	}
	defer func() { _ = s.Close() }()

	if s.Path() != path {
		t.Errorf("Path() = %q, want %q", s.Path(), path)
	}
}

// keys renders a set for error messages.
func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// The store must fold accents too. SQLite's LIKE does not, which is why the
// text filter is applied in Go rather than pushed into SQL.
func TestSQLiteTextSearchFoldsAccents(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := openStore(t)
	now := time.Now().UTC()

	rec := fullRecord("cam", now)
	rec.Title = "A-4 km 399.1 · CÓRDOBA"
	if _, err := s.Append(ctx, []domain.Record{rec}); err != nil {
		t.Fatalf("Append() = %v", err)
	}

	for _, text := range []string{"cordoba", "CÓRDOBA", "Cordoba", "a-4 cordoba"} {
		got, err := s.Query(ctx, domain.Filter{Text: text})
		if err != nil {
			t.Fatalf("Query(%q) = %v", text, err)
		}
		if len(got) != 1 {
			t.Errorf("Query(%q) returned %d records, want 1", text, len(got))
		}
	}

	got, _ := s.Query(ctx, domain.Filter{Text: "sevilla"})
	if len(got) != 0 {
		t.Errorf("Query(\"sevilla\") returned %d records, want 0", len(got))
	}
}

// A text filter must not lose rows to an early SQL LIMIT applied before the
// Go-side match runs.
func TestSQLiteTextSearchAppliesLimitAfterFiltering(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := openStore(t)
	now := time.Now().UTC()

	batch := make([]domain.Record, 0, 30)
	for i := range 30 {
		r := fullRecord(fmt.Sprintf("noise-%d", i), now.Add(-time.Duration(i)*time.Minute))
		r.Title = "unrelated road"
		batch = append(batch, r)
	}
	target := fullRecord("target", now.Add(-time.Hour))
	target.Title = "A-4 km 399.1 · CÓRDOBA"
	batch = append(batch, target)

	if _, err := s.Append(ctx, batch); err != nil {
		t.Fatalf("Append() = %v", err)
	}

	got, err := s.Query(ctx, domain.Filter{Text: "cordoba", Limit: 5})
	if err != nil {
		t.Fatalf("Query() = %v", err)
	}
	if len(got) != 1 || got[0].ID != "target" {
		t.Errorf("got %d records %v, want just the matching one", len(got), got)
	}
}

// A total computed from a page of records is not a total. The aggregate is
// pushed into SQL so a summary of a large store stays a summary and does not
// become a full scan through the API.
func TestSQLiteAggregate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := openStore(t)
	at := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)

	first := fullRecord("r1", at.Add(-48*time.Hour))
	second := fullRecord("r2", at)
	second.Topic = "traffic"
	second.Kind = "incident"
	second.Source = "dgt-incidents"
	third := fullRecord("r3", at.Add(-time.Hour))
	third.Topic = "traffic"

	if _, err := s.Append(ctx, []domain.Record{first, second, third}); err != nil {
		t.Fatalf("Append() = %v", err)
	}
	if _, err := s.Upsert(ctx, []domain.Entity{{
		ID: "e1", Source: "test", Kind: "camera", Topic: "traffic",
		Title: "A camera", FirstSeen: at, LastSeen: at,
		Provenance: domain.Provenance{
			Publisher: "Test", SourceURL: "https://example.org", License: "CC-BY-4.0", FetchedAt: at,
		},
	}}); err != nil {
		t.Fatalf("Upsert() = %v", err)
	}

	got, err := s.Aggregate(ctx)
	if err != nil {
		t.Fatalf("Aggregate() = %v", err)
	}

	if got.Records != 3 || got.Entities != 1 {
		t.Errorf("counts = %d records, %d entities", got.Records, got.Entities)
	}
	if got.ByTopic["press"] != 1 || got.ByTopic["traffic"] != 2 {
		t.Errorf("by topic = %v", got.ByTopic)
	}
	if got.BySource["test"] != 2 || got.BySource["dgt-incidents"] != 1 {
		t.Errorf("by source = %v", got.BySource)
	}
	if got.ByKind["news_item"] != 2 || got.ByKind["incident"] != 1 {
		t.Errorf("by kind = %v", got.ByKind)
	}
	if !got.Oldest.Equal(at.Add(-48 * time.Hour)) {
		t.Errorf("oldest = %v", got.Oldest)
	}
	if !got.Newest.Equal(at) {
		t.Errorf("newest = %v", got.Newest)
	}
}

// An empty store has no oldest observation, and a zero timestamp reported as
// one is a lie with a date on it.
func TestSQLiteAggregateOnAnEmptyStore(t *testing.T) {
	t.Parallel()

	got, err := openStore(t).Aggregate(context.Background())
	if err != nil {
		t.Fatalf("Aggregate() = %v", err)
	}

	if got.Records != 0 || len(got.ByTopic) != 0 {
		t.Errorf("aggregate = %+v", got)
	}
	if !got.Oldest.IsZero() || !got.Newest.IsZero() {
		t.Errorf("span = %v .. %v, want both absent", got.Oldest, got.Newest)
	}
}
