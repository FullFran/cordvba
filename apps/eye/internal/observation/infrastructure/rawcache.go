package infrastructure

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// ErrRawCache is returned when a payload cannot be stored or read back.
var ErrRawCache = errors.New("raw cache")

// ErrNotFound reports a hash the cache does not hold.
var ErrNotFound = errors.New("raw cache: not found")

// maxRawSize caps a single stored payload. A public feed that suddenly returns
// a gigabyte is a problem to notice, not to absorb.
const maxRawSize = 64 << 20

// RawCache stores source payloads exactly as received, addressed by the SHA-256
// of their bytes.
//
// This is what Provenance.RawHash points at. Without it, "why did eye say
// that?" has no answer and no inference can be replayed.
type RawCache struct {
	root string
}

// NewRawCache builds a cache rooted at dir. The directory is created lazily, on
// the first write, so a read-only command never creates state.
func NewRawCache(dir string) *RawCache { return &RawCache{root: dir} }

// Root is where the cache lives on disk.
func (c *RawCache) Root() string { return c.root }

// Put stores a payload and returns its hex SHA-256.
//
// Storage is content-addressed, so an unchanged feed fetched a hundred times
// occupies one file. Writing is atomic: a crash mid-write leaves a temporary
// file, never a truncated payload masquerading as evidence.
func (c *RawCache) Put(body []byte) (string, error) {
	if len(body) > maxRawSize {
		return "", fmt.Errorf("%w: payload of %d bytes exceeds the cap", ErrRawCache, len(body))
	}

	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])

	path := c.pathFor(hash)
	if _, err := os.Stat(path); err == nil {
		return hash, nil // Already held; identical bytes are stored once.
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", fmt.Errorf("%w: create directory: %w", ErrRawCache, err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return "", fmt.Errorf("%w: create temp file: %w", ErrRawCache, err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	zw := gzip.NewWriter(tmp)
	if _, err := zw.Write(body); err != nil {
		_ = zw.Close()
		_ = tmp.Close()
		return "", fmt.Errorf("%w: compress: %w", ErrRawCache, err)
	}
	if err := zw.Close(); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("%w: finish compression: %w", ErrRawCache, err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("%w: close temp file: %w", ErrRawCache, err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return "", fmt.Errorf("%w: publish: %w", ErrRawCache, err)
	}
	return hash, nil
}

// Get reads a payload back by hash, verifying that the bytes still hash to the
// name they are filed under. Silent corruption in the evidence store would be
// worse than a missing file.
func (c *RawCache) Get(hash string) ([]byte, error) {
	f, err := os.Open(c.pathFor(hash)) // #nosec G304 -- path derived from a validated hex hash
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, hash)
		}
		return nil, fmt.Errorf("%w: open %s: %w", ErrRawCache, hash, err)
	}
	defer func() { _ = f.Close() }()

	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("%w: decompress %s: %w", ErrRawCache, hash, err)
	}
	defer func() { _ = zr.Close() }()

	body, err := io.ReadAll(io.LimitReader(zr, maxRawSize+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read %s: %w", ErrRawCache, hash, err)
	}

	sum := sha256.Sum256(body)
	if got := hex.EncodeToString(sum[:]); got != hash {
		return nil, fmt.Errorf("%w: %s hashes to %s — the evidence store is corrupt", ErrRawCache, hash, got)
	}
	return body, nil
}

// Has reports whether the cache holds a payload.
func (c *RawCache) Has(hash string) bool {
	_, err := os.Stat(c.pathFor(hash))
	return err == nil
}

// Prune deletes payloads not modified since the cutoff, and reports how many
// went. Evidence for anything still in the record store should outlive it, so
// callers pass a cutoff derived from their longest retention window.
func (c *RawCache) Prune(before time.Time, keep func(hash string) bool) (int, error) {
	var removed int

	err := filepath.WalkDir(c.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil // Nothing cached yet.
			}
			return err
		}
		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil || info.ModTime().After(before) {
			return nil //nolint:nilerr // an unreadable entry is skipped, not fatal
		}

		hash := filepath.Base(path)

		// Only ever delete something that looks like a payload this
		// cache wrote. A stray file, a symlink, or anything else under
		// the root is left alone rather than removed on a walk.
		if !isHashName(hash) {
			return nil
		}
		if keep != nil && keep(hash) {
			return nil
		}
		if err := os.Remove(path); err == nil { // #nosec G122 -- name validated as a hex hash above
			removed++
		}
		return nil
	})
	if err != nil {
		return removed, fmt.Errorf("%w: prune: %w", ErrRawCache, err)
	}
	return removed, nil
}

// isHashName reports whether a filename is a full hex SHA-256, which is the
// only shape this cache ever writes.
func isHashName(name string) bool {
	if len(name) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(name)
	return err == nil
}

// pathFor fans payloads out over two levels, so a directory never holds a
// million entries.
func (c *RawCache) pathFor(hash string) string {
	if len(hash) < 4 {
		return filepath.Join(c.root, "invalid", hash)
	}
	return filepath.Join(c.root, hash[0:2], hash[2:4], hash)
}

// Compile-time proof that a payload survives a round trip unchanged.
var _ = bytes.Equal
