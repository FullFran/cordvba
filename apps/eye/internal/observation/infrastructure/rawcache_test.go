package infrastructure_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	store "github.com/FullFran/cordvba/apps/eye/internal/observation/infrastructure"
)

func TestRawCacheRoundTrip(t *testing.T) {
	t.Parallel()

	c := store.NewRawCache(t.TempDir())
	body := []byte(`<?xml version="1.0" encoding="ISO-8859-1"?><rss><channel><title>BOE</title></channel></rss>`)

	hash, err := c.Put(body)
	if err != nil {
		t.Fatalf("Put() = %v", err)
	}

	sum := sha256.Sum256(body)
	if want := hex.EncodeToString(sum[:]); hash != want {
		t.Errorf("hash = %q, want the SHA-256 of the payload %q", hash, want)
	}
	if !c.Has(hash) {
		t.Error("Has() = false right after Put()")
	}

	got, err := c.Get(hash)
	if err != nil {
		t.Fatalf("Get() = %v", err)
	}
	// Byte-for-byte: the payload is evidence, not a display string.
	if !bytes.Equal(got, body) {
		t.Errorf("round trip changed the payload:\n got %q\nwant %q", got, body)
	}
}

func TestRawCacheStoresIdenticalPayloadsOnce(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c := store.NewRawCache(dir)
	body := []byte("an unchanged feed, fetched twice")

	h1, err := c.Put(body)
	if err != nil {
		t.Fatalf("first Put() = %v", err)
	}
	h2, err := c.Put(body)
	if err != nil {
		t.Fatalf("second Put() = %v", err)
	}
	if h1 != h2 {
		t.Fatalf("hashes differ: %q vs %q", h1, h2)
	}

	if files := countFiles(t, dir); files != 1 {
		t.Errorf("cache holds %d files, want 1 — content addressing is not deduplicating", files)
	}
}

func TestRawCacheGetMissing(t *testing.T) {
	t.Parallel()

	c := store.NewRawCache(t.TempDir())
	_, err := c.Get("0000000000000000000000000000000000000000000000000000000000000000")

	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Get() = %v, want ErrNotFound", err)
	}
	if c.Has("0000") {
		t.Error("Has() = true for a hash never stored")
	}
}

// Evidence that has been silently corrupted is worse than evidence that is
// missing, because it still looks like proof.
func TestRawCacheDetectsCorruption(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c := store.NewRawCache(dir)

	hash, err := c.Put([]byte("original evidence"))
	if err != nil {
		t.Fatalf("Put() = %v", err)
	}

	// Overwrite the stored payload with different, validly gzipped content.
	other := store.NewRawCache(t.TempDir())
	otherHash, _ := other.Put([]byte("tampered evidence"))
	tampered, err := os.ReadFile(filepath.Join(other.Root(), otherHash[0:2], otherHash[2:4], otherHash)) // #nosec G304 -- path built from a hash this test just produced
	if err != nil {
		t.Fatalf("read tampered payload: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, hash[0:2], hash[2:4], hash), tampered, 0o600); err != nil { // #nosec G703 -- path built from a hash this test just produced
		t.Fatalf("write tampered payload: %v", err)
	}

	if _, err := c.Get(hash); err == nil {
		t.Fatal("Get() returned tampered bytes without complaining")
	}
}

func TestRawCachePrune(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	c := store.NewRawCache(dir)

	keepHash, _ := c.Put([]byte("still referenced by a record"))
	dropHash, _ := c.Put([]byte("no record points at this any more"))
	dropPath := filepath.Join(dir, dropHash[0:2], dropHash[2:4], dropHash)
	dropInfo, err := os.Stat(dropPath)
	if err != nil {
		t.Fatalf("stat the payload before pruning it: %v", err)
	}

	// Age both entries past the cutoff.
	past := time.Now().Add(-48 * time.Hour)
	for _, h := range []string{keepHash, dropHash} {
		path := filepath.Join(dir, h[0:2], h[2:4], h)
		if err := os.Chtimes(path, past, past); err != nil {
			t.Fatalf("chtimes: %v", err)
		}
	}

	removed, freedBytes, err := c.Prune(time.Now().Add(-time.Hour), func(hash string) bool {
		return hash == keepHash
	})
	if err != nil {
		t.Fatalf("Prune() = %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}
	if freedBytes != dropInfo.Size() {
		t.Errorf("freedBytes = %d, want the removed payload's own size %d", freedBytes, dropInfo.Size())
	}
	if !c.Has(keepHash) {
		t.Error("a payload the keep function protected was deleted")
	}
	if c.Has(dropHash) {
		t.Error("an unreferenced payload survived the prune")
	}
}

func TestRawCachePruneKeepsRecentPayloads(t *testing.T) {
	t.Parallel()

	c := store.NewRawCache(t.TempDir())
	hash, _ := c.Put([]byte("fetched just now"))

	removed, freedBytes, err := c.Prune(time.Now().Add(-time.Hour), nil)
	if err != nil {
		t.Fatalf("Prune() = %v", err)
	}
	if removed != 0 {
		t.Errorf("removed = %d, want 0", removed)
	}
	if freedBytes != 0 {
		t.Errorf("freedBytes = %d, want 0", freedBytes)
	}
	if !c.Has(hash) {
		t.Error("a payload newer than the cutoff was deleted")
	}
}

func TestRawCachePruneOnAnEmptyCache(t *testing.T) {
	t.Parallel()

	c := store.NewRawCache(filepath.Join(t.TempDir(), "never-written"))
	if _, _, err := c.Prune(time.Now(), nil); err != nil {
		t.Errorf("Prune() on an empty cache = %v, want nil", err)
	}
}

// Size is what the daemon logs as "the raw cache now holds" after a prune
// cycle: files removed and bytes freed tell an operator what just happened,
// and the remaining size tells them where they stand.
func TestRawCacheSize(t *testing.T) {
	t.Parallel()

	c := store.NewRawCache(t.TempDir())
	if _, err := c.Put([]byte("one payload")); err != nil {
		t.Fatalf("Put() = %v", err)
	}
	if _, err := c.Put([]byte("a second, different payload")); err != nil {
		t.Fatalf("Put() = %v", err)
	}

	files, bytes, err := c.Size()
	if err != nil {
		t.Fatalf("Size() = %v", err)
	}
	if files != 2 {
		t.Errorf("files = %d, want 2", files)
	}
	if bytes <= 0 {
		t.Errorf("bytes = %d, want > 0", bytes)
	}
}

func TestRawCacheSizeOnAnEmptyCache(t *testing.T) {
	t.Parallel()

	c := store.NewRawCache(filepath.Join(t.TempDir(), "never-written"))
	files, bytes, err := c.Size()
	if err != nil {
		t.Fatalf("Size() = %v", err)
	}
	if files != 0 || bytes != 0 {
		t.Errorf("Size() = %d files, %d bytes, want 0, 0", files, bytes)
	}
}

// countFiles counts regular files under dir.
func countFiles(t *testing.T, dir string) int {
	t.Helper()

	var n int
	err := filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			n++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	return n
}
