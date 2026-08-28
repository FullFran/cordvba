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
[![Dependencies](https://img.shields.io/badge/direct%20deps-budgeted-533483)](./docs/adr/0004-dependency-budget.md)
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
eye status / query / events / radar   →  answer from the local store
eye daemon                            →  run the scheduler
eye serve                             →  REST + SSE, opt-in
```

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

Requirements: Go 1.25+. That is the whole list.

```bash
git clone https://github.com/FullFran/eye.git
cd eye

make build
./bin/eye help
```

```bash
make ci-local   # fmt + vet + lint + test + build
make test       # go test -race -cover ./...
make help       # every target
```

## Sources

25 sources declared in [`configs/sources.yaml`](./configs/sources.yaml), across
transport, air, weather, fire, hydrology, air quality, events and civic
documents — DGT, AEMET, NASA FIRMS, CKAN Córdoba, RENFE, AUCORSA, SAIH
Guadalquivir, MITECO, IGN, INFOCA, adsb.lol, UCO, Agenda Única, IMAE, BOE, BOP,
PLACSP and more.

Each one declares its authority, licence, access kind and automation gate.
Sources whose reuse terms are unresolved are held, on purpose, and shown as
held. See [docs/legal/data-ethics.md](./docs/legal/data-ethics.md).

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
