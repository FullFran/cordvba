# ADR-0002: Keep the hexagonal per-domain layout

## Status

Accepted

## Context

This repository is bootstrapped from the Hagalink `backend-go` template, whose
convention is `internal/<domain>/{domain,application,infrastructure}`. That
template targets an HTTP service with chi and pgx; `eye` is a CLI with an
embedded store. The question is whether the layering still earns its keep when
there is no HTTP server at the centre.

It does, and for a sharper reason than in a web service: `eye` will grow
roughly twenty source adapters, each speaking a different protocol — DATEX II,
GTFS-RT, CKAN, ICS, WMS, RSS. Without a hard boundary, the shape of whichever
API was integrated first leaks into everything downstream.

## Decision

Keep the three-layer split per domain. `eye`'s domains are `observation`,
`source`, `provider` and `event`.

Layer rules:

| Layer | May import | Must NOT import |
|---|---|---|
| `domain` | stdlib only | anything else in the project, any driver, `net/http` |
| `application` | `domain` packages | `infrastructure`, `net/http`, storage drivers |
| `infrastructure` | `application`, `domain`, drivers | (no cycles) |

Source adapters live in `internal/provider/infrastructure/<source>/` and are the
only packages that know a wire format exists.

## Consequences

**Positive:**
- A source can be added, changed or dropped without touching the core.
- The domain is testable with no network and no database.
- Terminal, HTTP and file output are peers; none of them is privileged.

**Negative:**
- More packages than a flat CLI needs on day one.
- Some ceremony for genuinely trivial adapters.

## Alternatives considered

| Alternative | Why it was rejected |
|---|---|
| Flat `internal/` packages | Twenty wire formats with no boundary is how the first API's shape ends up in the query layer. |
| Layering by technical concern (`models/`, `handlers/`) | Says nothing about what the program is for; the directory tree should scream "Córdoba observations", not "MVC". |
