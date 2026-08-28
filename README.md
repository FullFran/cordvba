<div align="center">

<h1>eye</h1>
<p><strong>A live, source-verifiable model of Córdoba</strong></p>

<p>
Traffic, buses, trains, aircraft, fire, weather, rivers, air quality, public
documents and city events — assembled from public sources only, correlated in
space and time, and shipped as one Go binary with no mandatory services.
</p>

<br/>

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![CGO](https://img.shields.io/badge/CGO-disabled-2da44e)](./docs/adr/0003-sqlite-without-cgo.md)
[![SQLite](https://img.shields.io/badge/SQLite-WAL-003B57?logo=sqlite&logoColor=white)](https://sqlite.org)
[![Dependencies](https://img.shields.io/badge/direct%20deps-2-533483)](./docs/adr/0004-dependency-budget.md)
[![golangci-lint](https://img.shields.io/badge/golangci--lint-clean-yellow)](https://golangci-lint.run)

<br/>

[Idea](#the-idea) · [Boundary](#the-boundary) · [Architecture](#architecture) · [Quick start](#quick-start) · [Sources](#sources) · [Docs](#documentation)

</div>

---

## The idea

Not "a program for looking at cameras". A small **geo-temporal data fusion
centre** for one city, where a road incident, a thermal anomaly, a river level,
a delayed train and a Thursday concert are all just different kinds of
observation.

That gives `eye` three temporal dimensions:

```
                       EYE

            PAST        NOW        FUTURE
             │           │           │
          history     sensors      events
         incidents    traffic       works
          changes     flights      alerts
             │           │           │
             └───────────┼───────────┘
                         ▼
                     CÓRDOBA
```

And it makes the interesting question answerable:

```bash
eye status                    # what is the city doing right now
eye radar                     # the next 72 hours
eye events --weekend
eye event "pablo lopez"
eye query --bbox cordoba --since 30m --topic transport,fire
```

Every figure printed traces back to the public source it came from, with its
age and its licence. An inference is always labelled as an inference.

## The boundary

`eye` observes **public systems and phenomena**. It does not build profiles of
people.

| | |
|---|---|
| Camera positions and metadata, as an inventory | **Yes** |
| Documented public APIs — DATEX II, GTFS, CKAN, AEMET, FIRMS | **Yes** |
| Correlating fire, wind, hydrology, traffic and events | **Yes** |
| Face recognition, licence plates, per-person tracking | **Never** |
| Probing for undocumented RTSP, bypassing access control | **Never** |
| Archiving urban video | **Never** |
| Presenting an eye inference as an official 112 or INFOCA notice | **Never** |

This is enforced by mechanism, not by promise: camera media is RAM-only with a
TTL and has no persistence path in the code; movement data expires in 24–72
hours; every inference carries `quality: inferred` and a disclaimer. See
[ADR-0007](./docs/adr/0007-ethical-boundary.md).

## Architecture

One binary, several modes. HTTP is one adapter among several, never the centre.

```
eye status / query / news / events    →  answer from the local store
eye daemon                            →  run the scheduler continuously
eye serve                             →  REST + SSE, opt-in (not yet built)
```

The scheduler applies jitter so twenty sources never fire on the same second,
exponential backoff when one fails, and a circuit breaker that stops calling a
source that is consistently down. Per-host concurrency is capped, because
several Córdoba feeds share a publisher and the courtesy belongs to the host.

```
public source → registry gate → adapter → raw cache (sha256)
                                       → normalize → Record / Entity
                                       → store → rule engine → inference
```

The registry gate comes **before** the adapter: a source that is not explicitly
`automation: enabled` is never fetched, and its held state is visible in
`eye status` rather than silently missing.

Full diagrams in [docs/architecture/overview.md](./docs/architecture/overview.md).

## Quick start

Requirements: Go 1.25+. That is the whole list — no database, no Docker, no
services. The source registry is compiled into the binary, so it works the
moment it finishes building.

```bash
git clone https://github.com/FullFran/eye.git
cd eye && make build

./bin/eye status
```

```
CÓRDOBA · 28 Aug 2026 03:52

  air        1 observation     newest 2s
  city       50 observations   newest 8d
  civic      137 observations  newest 27h
  events     20 observations   newest 0s
  press      218 observations  newest 44m
  transport  1 observation     newest 0s
  inventory  32 assets mapped

Sources
  8 answered
  16 held: reuse terms unresolved or no documented interface
  5 awaiting an adapter in this build

Polled in 505ms · registry: (embedded)
```

### What works today

| Command | What it does |
|---|---|
| `eye status` | Polls every live source and reports the state of the city |
| `eye news` | The Córdoba press: Diario Córdoba, Cordópolis, El Día de Córdoba |
| `eye events` | What is scheduled, from the UCO events feed |
| `eye civic` | BOE publications and municipal open-data catalog changes |
| `eye sky` | Aircraft currently over the city, live |
| `eye cameras` | The 32 municipal traffic cameras — positions only |
| `eye query` | `--topic --since --text --source --limit`, across everything |
| `eye sources` | The registry: what eye may read, what it can read, and when each last answered |
| `eye daemon` | Polls continuously and persists everything — the watching mode |

Every command takes `--json`, and the JSON keeps the full provenance: publisher,
licence, source URL, and both timestamps so the source latency stays visible.

```bash
eye news --since 6h --json | jq '.[] | {title, publisher, source_latency_seconds}'
eye query --topic press,events --text patio
eye sky --json | jq '.[].payload.callsign'
```

### Persistence

Everything eye collects is stored in SQLite at `~/.local/share/eye/eye.db`, and
every payload is kept byte-for-byte in a content-addressed cache alongside it.
That is what makes `Provenance.RawHash` mean something: any answer can be
replayed from the exact bytes it came from.

```bash
eye daemon                      # poll continuously, persist, enforce retention
eye daemon --once               # one pass — what a systemd timer would call
eye news --offline              # answer from the store, zero network
```

Records are append-only and keyed by a stable id, so a second pass over
unchanged feeds stores nothing:

```
$ eye daemon --once
8 sources polled · 387 records new · 0 expired records pruned

$ eye daemon --once
8 sources polled · 0 records new · 0 expired records pruned
```

Aircraft positions carry an expiry and are deleted when it passes. Retention is
a `DELETE` that runs, not a paragraph in a document.

### Development

```bash
make ci-local   # fmt + vet + lint + test + build — must pass before any PR
make test       # go test -race -cover ./...
make help       # every target
```

## Sources

29 sources declared in [`configs/sources.yaml`](./configs/sources.yaml), across
transport, air, weather, fire, hydrology, air quality, local press, events and
civic documents.

**Live now (8):** Diario Córdoba, Cordópolis, El Día de Córdoba, BOE, UCO
events, the municipal CKAN catalog, the municipal camera inventory, adsb.lol.

**Held (16):** DGT DATEX II, RENFE, AUCORSA, SAIH Guadalquivir, INFOCA, IGN,
Agenda Única, Turismo de Córdoba, IMAE, BOP, PLACSP, OpenSky, e-distribución.
Held means the reuse terms are unresolved, there is no documented machine
interface, or — for the three DGT feeds — the portal answers `403` to automated
clients. That is a fact recorded in the registry, not a bug to work around.

**Awaiting an adapter (5):** AEMET, NASA FIRMS, MITECO ICA, the Diputación CKAN.
Permitted, readable in principle, not yet written.

Those are three different states and `eye sources` reports all three. See
[docs/legal/data-ethics.md](./docs/legal/data-ethics.md) for why a held source
is a normal outcome rather than a failure.

## Documentation

| Document | What it is for |
|---|---|
| [docs/](./docs/README.md) | Documentation index |
| [Architecture overview](./docs/architecture/overview.md) | The shape of the system, with diagrams |
| [Roadmap](./docs/roadmap.md) | Nine epics and their dependencies |
| [Data ethics](./docs/legal/data-ethics.md) | Reuse, retention, the source checklist |
| [ADRs](./docs/adr/README.md) | The seven decisions this is built on |
| [Getting started](./docs/development/getting-started.md) | Build, test, add a provider |
| [AGENTS.md](./AGENTS.md) | Rules for AI agents working in this repo |

---

<div align="center">
<sub>Bootstrapped from the <code>backend-go</code> template of the <a href="https://github.com/hagalink">Hagalink</a> ecosystem — governance kept, HTTP-service runtime replaced.</sub>
</div>
