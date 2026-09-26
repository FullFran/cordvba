# ADR-0003: Store observations in SQLite, without CGO

## Status

Accepted

## Context

`eye` needs durable local storage for records, entities, source health and the
raw response cache. The research proposed SQLite for the MVP and PostgreSQL +
PostGIS once serious spatial queries arrive. That progression is right, but the
SQLite driver choice has a consequence the research did not spell out:
`mattn/go-sqlite3` requires CGO, which means a C toolchain on every machine that
builds `eye` and the loss of trivially cross-compiled static binaries.

For a project whose whole deployment story is "copy the binary", that is not a
detail.

## Decision

Use SQLite in WAL mode as the default store, through a **pure-Go, CGO-free**
driver (`modernc.org/sqlite`). `CGO_ENABLED=0` is set in the Makefile and in CI,
so a build that reintroduces a C dependency fails loudly.

The store lives at `$XDG_DATA_HOME/eye/eye.db`, defaulting to
`~/.local/share/eye/eye.db`.

Storage sits behind a port in `observation/domain`, so PostgreSQL + PostGIS can
be added later as a second adapter rather than a migration of the whole system.

## Consequences

**Positive:**
- `GOOS=linux GOARCH=arm64 go build` works from any machine. eye runs on a
  Raspberry Pi without a cross-compilation toolchain.
- No database to install, run or back up. The store is one file.
- WAL mode lets `eye status` read while `eye daemon` writes.

**Negative:**
- The pure-Go driver is slower than the C one under heavy concurrent write
  load. eye's write volume is bounded by public publication cadences, not by us.
- No PostGIS. Spatial queries are bounding box plus haversine until the volume
  justifies a second adapter — which is enough for a city-scale viewport.

## Alternatives considered

| Alternative | Why it was rejected |
|---|---|
| `mattn/go-sqlite3` | Requires CGO; kills static cross-compilation, which is the deployment story. |
| PostgreSQL + PostGIS from the start | A mandatory service contradicts ADR-0001. It stays as the growth path, behind the same port. |
| Files on disk (JSONL) | No indexes, no concurrent reader/writer, no time-range queries. Fine for the raw cache, not for the store. |
