package infrastructure

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	domain "github.com/FullFran/eye/internal/observation/domain"
)

// Snapshot returns what a source last reported.
//
// known distinguishes "this source reported nothing" from "eye has never
// looked". On a first sighting there is nothing to compare against, so nothing
// may be claimed to have appeared — otherwise a fresh install would announce
// every record in Cordoba as breaking news.
func (s *SQLiteStore) Snapshot(ctx context.Context, source string) ([]domain.Record, bool, error) {
	var taken int64
	err := s.db.QueryRowContext(ctx,
		`SELECT updated_at FROM snapshot_taken WHERE source = ?`, source).Scan(&taken)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, false, nil
	case err != nil:
		return nil, false, fmt.Errorf("%w: snapshot state: %w", ErrStore, err)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT record FROM snapshots WHERE source = ? ORDER BY identity`, source)
	if err != nil {
		return nil, false, fmt.Errorf("%w: snapshot: %w", ErrStore, err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Record
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return nil, false, fmt.Errorf("%w: snapshot row: %w", ErrStore, err)
		}
		var r domain.Record
		if err := json.Unmarshal([]byte(body), &r); err != nil {
			// A row eye can no longer read is a row eye must not compare
			// against: reporting a change derived from it would be
			// reporting a difference from something unknown.
			continue
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("%w: snapshot rows: %w", ErrStore, err)
	}
	return out, true, nil
}

// SaveSnapshot replaces a source's snapshot with what it just reported.
//
// Replace, not merge: the snapshot is what the source says now, and a record
// left behind from an earlier poll would be reported as disappearing over and
// over. The whole replacement is one transaction, so a crash mid-write cannot
// leave a half-source that would produce fictional changes on the next tick.
func (s *SQLiteStore) SaveSnapshot(ctx context.Context, source string, records []domain.Record) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%w: begin: %w", ErrStore, err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM snapshots WHERE source = ?`, source); err != nil {
		return fmt.Errorf("%w: clear snapshot: %w", ErrStore, err)
	}

	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR REPLACE INTO snapshots (source, identity, record, updated_at) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("%w: prepare snapshot: %w", ErrStore, err)
	}
	defer func() { _ = stmt.Close() }()

	now := time.Now().UTC().Unix()
	for _, r := range records {
		identity := r.Identity()
		if identity == "" {
			// A record eye cannot identify cannot be paired on the next
			// poll, so keeping it would only take space.
			continue
		}
		body, err := json.Marshal(r)
		if err != nil {
			continue
		}
		if _, err := stmt.ExecContext(ctx, source, identity, string(body), now); err != nil {
			return fmt.Errorf("%w: write snapshot: %w", ErrStore, err)
		}
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT OR REPLACE INTO snapshot_taken (source, updated_at) VALUES (?, ?)`,
		source, now); err != nil {
		return fmt.Errorf("%w: mark snapshot: %w", ErrStore, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%w: commit snapshot: %w", ErrStore, err)
	}
	return nil
}

var _ domain.SnapshotStore = (*SQLiteStore)(nil)
