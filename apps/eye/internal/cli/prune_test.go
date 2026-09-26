package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	domain "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	store "github.com/FullFran/cordvba/apps/eye/internal/observation/infrastructure"
)

// testRecord builds a valid record carrying the given raw hash and expiry,
// for tests that only care about retention and the raw cache — not about
// what a real provider would normalize.
func testRecord(id, rawHash string, expiresAt *time.Time) domain.Record {
	now := time.Now().UTC()
	return domain.Record{
		ID: id, Source: "test", Kind: "test_kind", Topic: "test",
		ObservedAt: now, FetchedAt: now,
		Title:      id,
		Quality:    domain.QualityOfficial,
		Confidence: 1,
		ExpiresAt:  expiresAt,
		Provenance: domain.Provenance{
			Publisher: "Test Authority", SourceURL: "https://example.org", License: "unspecified",
			FetchedAt: now, RawHash: rawHash,
		},
	}
}

// ageRawCachePayload backdates a cached payload's mtime, the same way
// RawCache's own tests do, so a test can put it outside the grace window
// without waiting for real time to pass.
func ageRawCachePayload(t *testing.T, cache *store.RawCache, hash string, at time.Time) {
	t.Helper()
	path := filepath.Join(cache.Root(), hash[0:2], hash[2:4], hash)
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatalf("age raw cache payload %s: %v", hash, err)
	}
}

// minimalRegistry writes a registry these tests never poll, so newRuntime
// resolves a known, deterministic file instead of falling through to
// whatever sources.yaml the operator running the test happens to have under
// their own XDG config directory.
func minimalRegistry(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "sources.yaml")
	body := []byte(`
sources:
  - id: unused
    authority: Test Authority
    topic: press
    url: https://example.org/rss
    format: rss
    license: unspecified
    access: documented_api
    automation: disabled
`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write registry: %v", err)
	}
	return path
}

// This exercises the full seam #74 adds: the daemon's prune enforces record
// retention first, then removes only the raw payloads no surviving record
// references, and only once they are older than the configured grace period.
func TestRuntimePruneRemovesOnlyUnreferencedAgedPayloads(t *testing.T) {
	t.Parallel()

	rt, err := newRuntime(runtimeOptions{registry: minimalRegistry(t), dataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("newRuntime() = %v", err)
	}
	defer func() { _ = rt.Close() }()
	rt.cfg.RawCacheGrace = time.Hour

	ctx := context.Background()
	past := time.Now().Add(-2 * time.Hour)
	expired := time.Now().Add(-time.Minute)
	future := time.Now().Add(48 * time.Hour)

	sharedHash, err := rt.cache.Put([]byte("payload referenced by two records"))
	if err != nil {
		t.Fatalf("Put() = %v", err)
	}
	orphanHash, err := rt.cache.Put([]byte("payload nobody references any more"))
	if err != nil {
		t.Fatalf("Put() = %v", err)
	}
	freshOrphanHash, err := rt.cache.Put([]byte("fetched just now, not normalized yet"))
	if err != nil {
		t.Fatalf("Put() = %v", err)
	}

	// Age every payload but the fresh one past the grace window. The fresh
	// one keeps "now" as its mtime, so it stays inside the grace window
	// regardless of what references it.
	ageRawCachePayload(t, rt.cache, sharedHash, past)
	ageRawCachePayload(t, rt.cache, orphanHash, past)

	records := []domain.Record{
		// The shared hash is referenced by one expired and one live record:
		// it must survive, because the live one still points at it.
		testRecord("shared-expired", sharedHash, &expired),
		testRecord("shared-live", sharedHash, &future),
		// Nothing survives to reference the orphan hash.
		testRecord("orphan-expired", orphanHash, &expired),
	}
	if _, err := rt.store.Append(ctx, records); err != nil {
		t.Fatalf("Append() = %v", err)
	}

	pr, err := rt.prune(ctx)
	if err != nil {
		t.Fatalf("prune() = %v", err)
	}

	if pr.Records != 2 {
		t.Errorf("Records pruned = %d, want 2 (shared-expired, orphan-expired)", pr.Records)
	}
	if pr.Payloads != 1 {
		t.Errorf("Payloads pruned = %d, want 1 (only the orphan)", pr.Payloads)
	}

	if !rt.cache.Has(sharedHash) {
		t.Error("a payload a surviving record still references was removed")
	}
	if rt.cache.Has(orphanHash) {
		t.Error("an aged, unreferenced payload survived the prune")
	}
	if !rt.cache.Has(freshOrphanHash) {
		t.Error("a payload inside the grace window was removed despite having no reference yet")
	}
}

// A historical source's records never expire, so the payload behind one must
// never become eligible for the raw cache prune either.
func TestRuntimePruneKeepsPayloadsBehindHistoricalRecords(t *testing.T) {
	t.Parallel()

	rt, err := newRuntime(runtimeOptions{registry: minimalRegistry(t), dataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("newRuntime() = %v", err)
	}
	defer func() { _ = rt.Close() }()
	rt.cfg.RawCacheGrace = time.Hour

	ctx := context.Background()
	past := time.Now().Add(-2 * time.Hour)

	hash, err := rt.cache.Put([]byte("an environmental reading, kept as history"))
	if err != nil {
		t.Fatalf("Put() = %v", err)
	}
	ageRawCachePayload(t, rt.cache, hash, past)

	// A historical record: no ExpiresAt, exactly what the collector produces
	// for a source marked retention: historical (see #73).
	if _, err := rt.store.Append(ctx, []domain.Record{testRecord("historical-1", hash, nil)}); err != nil {
		t.Fatalf("Append() = %v", err)
	}

	pr, err := rt.prune(ctx)
	if err != nil {
		t.Fatalf("prune() = %v", err)
	}
	if pr.Records != 0 {
		t.Errorf("Records pruned = %d, want 0 — a historical record must never expire", pr.Records)
	}
	if pr.Payloads != 0 {
		t.Errorf("Payloads pruned = %d, want 0", pr.Payloads)
	}
	if !rt.cache.Has(hash) {
		t.Error("the payload behind a historical record was removed")
	}
}

func TestFormatBytes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		n    int64
		want string
	}{
		{name: "bytes", n: 512, want: "512 B"},
		{name: "kibibytes", n: 2048, want: "2.0 KiB"},
		{name: "mebibytes", n: 27_500_000, want: "26.2 MiB"},
		{name: "zero", n: 0, want: "0 B"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := formatBytes(tc.n); got != tc.want {
				t.Errorf("formatBytes(%d) = %q, want %q", tc.n, got, tc.want)
			}
		})
	}
}
