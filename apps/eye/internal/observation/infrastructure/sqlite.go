package infrastructure

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	domain "github.com/FullFran/cordvba/apps/eye/internal/observation/domain"
	provider "github.com/FullFran/cordvba/apps/eye/internal/provider/domain"

	// Pure-Go SQLite. No CGO, so eye still cross-compiles to a static
	// binary — see ADR-0003. Swapping this for mattn/go-sqlite3 would
	// silently reintroduce a C toolchain requirement.
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

// ErrStore is returned when the store cannot serve a request.
var ErrStore = errors.New("store")

// SQLiteStore is the durable adapter for records, entities and source state.
type SQLiteStore struct {
	db   *sql.DB
	path string
}

// OpenSQLite opens (and creates, if needed) the store at path.
func OpenSQLite(path string) (*SQLiteStore, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("%w: create %s: %w", ErrStore, dir, err)
		}
	}

	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("%w: open %s: %w", ErrStore, path, err)
	}

	// One writer, several readers. WAL allows `eye status` to read while
	// `eye daemon` writes, but SQLite still serialises writes.
	db.SetMaxOpenConns(1)

	if _, err := db.ExecContext(context.Background(), schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("%w: apply schema: %w", ErrStore, err)
	}

	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &SQLiteStore{db: db, path: path}, nil
}

// Close releases the database handle.
func (s *SQLiteStore) Close() error { return s.db.Close() }

// Path is where the store lives on disk.
func (s *SQLiteStore) Path() string { return s.path }

// Append stores records and returns how many were new. Existing ids are
// ignored rather than overwritten: eye does not rewrite history in place.
func (s *SQLiteStore) Append(ctx context.Context, records []domain.Record) (int, error) {
	if len(records) == 0 {
		return 0, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("%w: begin: %w", ErrStore, err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO records (
			id, source, kind, topic,
			observed_at, fetched_at, valid_from, valid_until, expires_at,
			lat, lon, geometry,
			title, description,
			severity, confidence, quality,
			entity_id, local_key, dedupe_key, payload,
			publisher, source_url, license, raw_hash
		) VALUES (?,?,?,?, ?,?,?,?,?, ?,?,?, ?,?, ?,?,?, ?,?,?,?, ?,?,?,?)`)
	if err != nil {
		return 0, fmt.Errorf("%w: prepare: %w", ErrStore, err)
	}
	defer func() { _ = stmt.Close() }()

	var added int
	for _, r := range records {
		lat, lon := nullableCoords(r.Position)

		res, err := stmt.ExecContext(ctx,
			r.ID, r.Source, r.Kind, r.Topic,
			micros(r.ObservedAt), micros(r.FetchedAt),
			nullableMicros(r.ValidFrom), nullableMicros(r.ValidUntil), nullableMicros(r.ExpiresAt),
			lat, lon, nullableJSON(r.Geometry),
			r.Title, r.Description,
			int(r.Severity), r.Confidence, string(r.Quality),
			nullableString(r.EntityID), nullEmpty(r.LocalKey), nullEmpty(r.DedupeKey), nullableJSON(r.Payload),
			r.Provenance.Publisher, r.Provenance.SourceURL, r.Provenance.License, r.Provenance.RawHash,
		)
		if err != nil {
			return added, fmt.Errorf("%w: insert %s: %w", ErrStore, r.ID, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			added++
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("%w: commit: %w", ErrStore, err)
	}
	return added, nil
}

// Query returns matching records, newest first.
//
// The bounding box is applied in SQL and the radius is refined in Go. Without
// PostGIS that split is the honest one: an index can narrow a box, but a
// great-circle distance cannot be indexed without extension support.
func (s *SQLiteStore) Query(ctx context.Context, f domain.Filter) ([]domain.Record, error) {
	where, args := recordPredicates(f)

	// #nosec G202 -- clause built from package literals and placeholders; the only interpolated value is an int limit
	query := `SELECT
			id, source, kind, topic,
			observed_at, fetched_at, valid_from, valid_until, expires_at,
			lat, lon, geometry,
			title, description,
			severity, confidence, quality,
			entity_id, local_key, dedupe_key, payload,
			publisher, source_url, license, raw_hash
		FROM records` + where + ` ORDER BY observed_at DESC`

	// The limit is applied after the radius refinement, so a circular query
	// cannot come back short because the box happened to hold more.
	if f.Limit > 0 && f.Near == nil && f.Text == "" {
		query += fmt.Sprintf(" LIMIT %d", f.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%w: query: %w", ErrStore, err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Record
	for rows.Next() {
		r, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		// Text and radius are both refined here rather than in SQL.
		if (f.Near != nil || f.Text != "") && !f.MatchRecord(r) {
			continue
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: scan: %w", ErrStore, err)
	}

	return domain.ApplyLimit(out, f.Limit), nil
}

// Upsert stores entities, refreshing last_seen and keeping the original
// first_seen for ones already known.
func (s *SQLiteStore) Upsert(ctx context.Context, entities []domain.Entity) (int, error) {
	if len(entities) == 0 {
		return 0, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("%w: begin: %w", ErrStore, err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO entities (
			id, source, kind, topic, title, description,
			lat, lon, geometry, first_seen, last_seen, payload,
			publisher, source_url, license, raw_hash
		) VALUES (?,?,?,?,?,?, ?,?,?,?,?,?, ?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			title       = excluded.title,
			description = excluded.description,
			lat         = excluded.lat,
			lon         = excluded.lon,
			geometry    = excluded.geometry,
			last_seen   = excluded.last_seen,
			payload     = excluded.payload,
			license     = excluded.license,
			raw_hash    = excluded.raw_hash`)
	if err != nil {
		return 0, fmt.Errorf("%w: prepare: %w", ErrStore, err)
	}
	defer func() { _ = stmt.Close() }()

	var added int
	for _, e := range entities {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM entities WHERE id = ?`, e.ID).Scan(&exists); err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return added, fmt.Errorf("%w: lookup %s: %w", ErrStore, e.ID, err)
			}
			added++
		}

		lat, lon := nullableCoords(e.Position)
		if _, err := stmt.ExecContext(ctx,
			e.ID, e.Source, e.Kind, e.Topic, e.Title, e.Description,
			lat, lon, nullableJSON(e.Geometry), micros(e.FirstSeen), micros(e.LastSeen), nullableJSON(e.Payload),
			e.Provenance.Publisher, e.Provenance.SourceURL, e.Provenance.License, e.Provenance.RawHash,
		); err != nil {
			return added, fmt.Errorf("%w: upsert %s: %w", ErrStore, e.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("%w: commit: %w", ErrStore, err)
	}
	return added, nil
}

// Entities returns matching inventory.
func (s *SQLiteStore) Entities(ctx context.Context, f domain.Filter) ([]domain.Entity, error) {
	where, args := entityPredicates(f)

	// #nosec G202 -- clause built from package literals and placeholders; the only interpolated value is an int limit
	query := `SELECT
			id, source, kind, topic, title, description,
			lat, lon, geometry, first_seen, last_seen, payload,
			publisher, source_url, license, raw_hash
		FROM entities` + where + ` ORDER BY title`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%w: query entities: %w", ErrStore, err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Entity
	for rows.Next() {
		e, err := scanEntity(rows)
		if err != nil {
			return nil, err
		}
		if (f.Near != nil || f.Text != "") && !f.MatchEntity(e) {
			continue
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: scan entities: %w", ErrStore, err)
	}

	return domain.ApplyLimit(out, f.Limit), nil
}

// Prune deletes records whose retention window has closed.
//
// This is the mechanism behind ADR-0007: aircraft and vehicle positions expire
// because a DELETE runs, not because a document says they should.
func (s *SQLiteStore) Prune(ctx context.Context, now time.Time) (int, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM records WHERE expires_at IS NOT NULL AND expires_at <= ?`, micros(now))
	if err != nil {
		return 0, fmt.Errorf("%w: prune: %w", ErrStore, err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// Counts reports how much the store holds.
func (s *SQLiteStore) Counts(ctx context.Context) (records, entities int, err error) {
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM records`).Scan(&records); err != nil {
		return 0, 0, fmt.Errorf("%w: count records: %w", ErrStore, err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM entities`).Scan(&entities); err != nil {
		return 0, 0, fmt.Errorf("%w: count entities: %w", ErrStore, err)
	}
	return records, entities, nil
}

// Aggregate summarises everything the store holds.
//
// The grouping runs in SQL rather than over a page of rows read back through a
// filter: a total computed from the first five thousand records is not a
// total, and a dashboard built on one is confidently wrong.
type Aggregate struct {
	Records  int
	Entities int
	ByTopic  map[string]int
	BySource map[string]int
	ByKind   map[string]int
	// Oldest and Newest bound the observation times held. Both are zero on
	// an empty store, which is different from an epoch timestamp.
	Oldest time.Time
	Newest time.Time
}

// Aggregate returns the summary.
func (s *SQLiteStore) Aggregate(ctx context.Context) (Aggregate, error) {
	records, entities, err := s.Counts(ctx)
	if err != nil {
		return Aggregate{}, err
	}

	agg := Aggregate{Records: records, Entities: entities}
	for _, group := range []struct {
		column string
		into   *map[string]int
	}{
		{column: "topic", into: &agg.ByTopic},
		{column: "source", into: &agg.BySource},
		{column: "kind", into: &agg.ByKind},
	} {
		counts, err := s.countBy(ctx, group.column)
		if err != nil {
			return Aggregate{}, err
		}
		*group.into = counts
	}

	var oldest, newest sql.NullInt64
	if err := s.db.QueryRowContext(ctx,
		`SELECT MIN(observed_at), MAX(observed_at) FROM records`).Scan(&oldest, &newest); err != nil {
		return Aggregate{}, fmt.Errorf("%w: aggregate span: %w", ErrStore, err)
	}
	agg.Oldest = timeFromNull(oldest)
	agg.Newest = timeFromNull(newest)

	return agg, nil
}

// countBy groups the records by one column.
//
// The column is never request input: it comes from the fixed list above, which
// is what keeps a GROUP BY that cannot use a placeholder from being an
// injection vector.
func (s *SQLiteStore) countBy(ctx context.Context, column string) (map[string]int, error) {
	// #nosec G202 -- column comes from a package-local literal list, never from a request
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+column+`, COUNT(*) FROM records GROUP BY `+column)
	if err != nil {
		return nil, fmt.Errorf("%w: count by %s: %w", ErrStore, column, err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]int{}
	for rows.Next() {
		var (
			value string
			count int
		)
		if err := rows.Scan(&value, &count); err != nil {
			return nil, fmt.Errorf("%w: scan count by %s: %w", ErrStore, column, err)
		}
		out[value] = count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: count by %s: %w", ErrStore, column, err)
	}
	return out, nil
}

// SaveState persists a source's polling state, so a restart does not
// re-download what the publisher already said has not changed.
func (s *SQLiteStore) SaveState(ctx context.Context, st SourceState) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO source_state (
			source_id, etag, last_modified, last_attempt, last_success,
			consecutive_errors, last_error, records
		) VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(source_id) DO UPDATE SET
			etag               = excluded.etag,
			last_modified      = excluded.last_modified,
			last_attempt       = excluded.last_attempt,
			last_success       = COALESCE(excluded.last_success, source_state.last_success),
			consecutive_errors = excluded.consecutive_errors,
			last_error         = excluded.last_error,
			records            = excluded.records`,
		st.SourceID, st.ETag, st.LastModified,
		nullableMicrosValue(st.LastAttempt), nullableMicrosValue(st.LastSuccess),
		st.ConsecutiveErrors, st.LastError, st.Records)
	if err != nil {
		return fmt.Errorf("%w: save state %s: %w", ErrStore, st.SourceID, err)
	}
	return nil
}

// States returns the persisted polling state for every known source.
func (s *SQLiteStore) States(ctx context.Context) (map[string]SourceState, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT source_id, etag, last_modified, last_attempt, last_success,
		       consecutive_errors, last_error, records
		FROM source_state`)
	if err != nil {
		return nil, fmt.Errorf("%w: load states: %w", ErrStore, err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]SourceState{}
	for rows.Next() {
		var (
			st               SourceState
			attempt, success sql.NullInt64
		)
		if err := rows.Scan(&st.SourceID, &st.ETag, &st.LastModified,
			&attempt, &success, &st.ConsecutiveErrors, &st.LastError, &st.Records); err != nil {
			return nil, fmt.Errorf("%w: scan state: %w", ErrStore, err)
		}
		st.LastAttempt = timeFromNull(attempt)
		st.LastSuccess = timeFromNull(success)
		out[st.SourceID] = st
	}
	return out, rows.Err()
}

// SourceState is what eye remembers about polling one source between runs.
type SourceState struct {
	SourceID          string
	ETag              string
	LastModified      string
	LastAttempt       time.Time
	LastSuccess       time.Time
	ConsecutiveErrors int
	LastError         string
	Records           int
}

// Health projects the persisted state onto the provider health type.
func (s SourceState) Health() provider.Health {
	return provider.Health{
		SourceID:          s.SourceID,
		LastAttempt:       s.LastAttempt,
		LastSuccess:       s.LastSuccess,
		ConsecutiveErrors: s.ConsecutiveErrors,
		LastError:         s.LastError,
		Records:           s.Records,
	}
}

var (
	_ domain.RecordStore = (*SQLiteStore)(nil)
	_ domain.EntityStore = (*SQLiteStore)(nil)
)

// nullEmpty maps an empty string onto SQL NULL.
func nullEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// nullableString maps a nil pointer onto SQL NULL.
func nullableString(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// nullableJSON maps empty raw JSON onto SQL NULL.
func nullableJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return string(raw)
}

// nullableCoords splits an optional point into two nullable columns.
func nullableCoords(p *domain.Point) (lat, lon any) {
	if p == nil {
		return nil, nil
	}
	return p.Lat, p.Lon
}

// micros converts a time to Unix microseconds.
func micros(t time.Time) int64 { return t.UTC().UnixMicro() }

// nullableMicros converts an optional time to a nullable column value.
func nullableMicros(t *time.Time) any {
	if t == nil {
		return nil
	}
	return micros(*t)
}

// nullableMicrosValue maps a zero time onto SQL NULL.
func nullableMicrosValue(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return micros(t)
}

// timeFromMicros converts stored microseconds back to a UTC time.
func timeFromMicros(us int64) time.Time { return time.UnixMicro(us).UTC() }

// timeFromNull converts a nullable column back to a time, zero when absent.
func timeFromNull(v sql.NullInt64) time.Time {
	if !v.Valid {
		return time.Time{}
	}
	return timeFromMicros(v.Int64)
}

// quoteIdentifierList renders a placeholder list for an IN clause.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// addedColumns are columns that arrived after the first release. The schema
// creates tables with IF NOT EXISTS, which does nothing for a table that
// already exists, so a store created by an older eye needs them added.
//
// Additive only, and each one nullable. eye never rewrites history in place,
// and a migration that could destroy a column is a migration that eventually
// will.
var addedColumns = []struct{ table, column, definition string }{
	{"records", "local_key", "TEXT"},
}

// migrate brings an existing store up to the current schema.
func migrate(db *sql.DB) error {
	ctx := context.Background()

	for _, c := range addedColumns {
		has, err := hasColumn(ctx, db, c.table, c.column)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", c.table, c.column, c.definition)
		if _, err := db.ExecContext(ctx, stmt); err != nil { // #nosec G202 -- table, column and type are package literals, never input
			return fmt.Errorf("%w: add %s.%s: %w", ErrStore, c.table, c.column, err)
		}
	}
	return nil
}

// hasColumn reports whether a table already carries a column.
func hasColumn(ctx context.Context, db *sql.DB, table, column string) (bool, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table)) // #nosec G202 -- table name is a package literal
	if err != nil {
		return false, fmt.Errorf("%w: inspect %s: %w", ErrStore, table, err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			cid          int
			name, ctype  string
			notNull, pk  int
			defaultValue sql.NullString
		)
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &defaultValue, &pk); err != nil {
			return false, fmt.Errorf("%w: inspect %s: %w", ErrStore, table, err)
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// DropColumnForTest removes a column so a test can imitate a store written by
// an older eye. It exists only for that: nothing in eye drops columns, because
// a migration that can destroy one eventually will.
func (s *SQLiteStore) DropColumnForTest(ctx context.Context, table, column string) error {
	stmt := fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", table, column) // #nosec G202 -- test-only helper over package literals
	if _, err := s.db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("%w: drop %s.%s: %w", ErrStore, table, column, err)
	}
	return nil
}
