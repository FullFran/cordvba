-- eye storage schema.
--
-- Records are append-only: there is no UPDATE path for an observation, and
-- INSERT OR IGNORE is what makes a re-poll of an unchanged feed a no-op.
--
-- Times are stored as Unix microseconds. SQLite has no date type, and an
-- integer sorts, ranges and indexes correctly without a parser in the middle.

PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;

CREATE TABLE IF NOT EXISTS records (
    id          TEXT PRIMARY KEY,
    source      TEXT    NOT NULL,
    kind        TEXT    NOT NULL,
    topic       TEXT    NOT NULL,

    observed_at INTEGER NOT NULL,
    fetched_at  INTEGER NOT NULL,
    valid_from  INTEGER,
    valid_until INTEGER,
    expires_at  INTEGER,

    lat         REAL,
    lon         REAL,
    geometry    TEXT,

    title       TEXT    NOT NULL,
    description TEXT    NOT NULL DEFAULT '',

    severity    INTEGER NOT NULL,
    confidence  REAL    NOT NULL,
    quality     TEXT    NOT NULL,

    entity_id   TEXT,
    local_key   TEXT,
    dedupe_key  TEXT,
    payload     TEXT,

    -- Provenance is columns, not a blob: it has to be queryable to be useful.
    publisher   TEXT    NOT NULL,
    source_url  TEXT    NOT NULL,
    license     TEXT    NOT NULL,
    raw_hash    TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS records_observed_at  ON records (observed_at DESC);
CREATE INDEX IF NOT EXISTS records_topic_time   ON records (topic, observed_at DESC);
CREATE INDEX IF NOT EXISTS records_source_time  ON records (source, observed_at DESC);
CREATE INDEX IF NOT EXISTS records_expires_at   ON records (expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS records_bbox         ON records (lat, lon) WHERE lat IS NOT NULL;
CREATE INDEX IF NOT EXISTS records_dedupe       ON records (dedupe_key) WHERE dedupe_key IS NOT NULL;

CREATE TABLE IF NOT EXISTS entities (
    id          TEXT PRIMARY KEY,
    source      TEXT    NOT NULL,
    kind        TEXT    NOT NULL,
    topic       TEXT    NOT NULL,

    title       TEXT    NOT NULL,
    description TEXT    NOT NULL DEFAULT '',

    lat         REAL,
    lon         REAL,
    geometry    TEXT,

    -- first_seen is eye's own observation window, not the lifetime of the
    -- real-world thing. An upsert refreshes last_seen and keeps first_seen.
    first_seen  INTEGER NOT NULL,
    last_seen   INTEGER NOT NULL,

    payload     TEXT,

    publisher   TEXT    NOT NULL,
    source_url  TEXT    NOT NULL,
    license     TEXT    NOT NULL,
    raw_hash    TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS entities_kind      ON entities (kind);
CREATE INDEX IF NOT EXISTS entities_topic     ON entities (topic);
CREATE INDEX IF NOT EXISTS entities_bbox      ON entities (lat, lon) WHERE lat IS NOT NULL;

-- Per-source polling state, so a restart does not re-download everything the
-- publisher already told us has not changed.
CREATE TABLE IF NOT EXISTS source_state (
    source_id          TEXT PRIMARY KEY,
    etag               TEXT NOT NULL DEFAULT '',
    last_modified      TEXT NOT NULL DEFAULT '',
    last_attempt       INTEGER,
    last_success       INTEGER,
    consecutive_errors INTEGER NOT NULL DEFAULT 0,
    last_error         TEXT NOT NULL DEFAULT '',
    records            INTEGER NOT NULL DEFAULT 0
);

-- What each source reported on its last successful poll. Changes are measured
-- against this rather than against the append-only history, which grows without
-- bound and mixes every poll together.
--
-- Derived state: dropping this table costs the next tick's changes and nothing
-- else. The observations themselves live in `records`.
CREATE TABLE IF NOT EXISTS snapshots (
    source     TEXT    NOT NULL,
    identity   TEXT    NOT NULL,
    record     TEXT    NOT NULL,
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (source, identity)
);

-- A source with no rows here is not the same as a source eye has never seen:
-- on a first sighting nothing may be claimed to have appeared, so the fact of
-- having looked is recorded separately from what was found.
CREATE TABLE IF NOT EXISTS snapshot_taken (
    source     TEXT    PRIMARY KEY,
    updated_at INTEGER NOT NULL
);
