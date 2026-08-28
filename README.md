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

> **A personal project.** eye reads public sources for its owner's own use, and
> several of those sources are licensed on exactly that basis — DGT's camera
> images permit reproduction only "para uso personal y privado", and OpenSky
> distinguishes personal from commercial use. Reusing eye for anything
> commercial means re-reading [the registry](./configs/sources.yaml) source by
> source first. See [data ethics](./docs/legal/data-ethics.md).

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
| `eye cameras` | The camera inventory — 1948 DGT plus 32 municipal, positions and metadata |
| `eye camera` | Show one camera's current frame, with the age of the image |
| `eye bus` | Live arrival estimates at a Córdoba stop, searched by name |
| `eye watch` | A live board of the city, refreshing on screen |
| `eye serve` | HTTP API, so other programs read the same records |
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

### Cameras

```bash
eye camera "A-4 cordoba"          # draw it in the terminal
eye camera 421 --open             # system image viewer
eye camera 421 --window           # a fresh Ghostty window, outside tmux
eye camera --near 37.88,-4.78     # nearest camera to a point
```

```
A-4 km 399.1 · CÓRDOBA
37.8900, -4.7449

IMAGE AGE   11m (captured 12:59:01)
SOURCE      Direccion General de Trafico · free-of-charge-nap-terms
IMAGE       https://etraffic.dgt.es/camarasEtraffic/421.jpg

Held in memory only. eye does not store camera images.
```

**The age is not decoration.** DGT cameras refresh every two to three minutes,
and some stop refreshing entirely — one sampled camera had not updated in 53
days. A frame older than half an hour is labelled `STALE`, loudly, because
showing it as the current state of a road would be the exact lie this project
exists not to tell.

There is no video: all 1948 DGT cameras publish a still JPEG and nothing else.
See [docs/legal/data-ethics.md](./docs/legal/data-ethics.md) for the reuse terms,
which are narrower for the images than for the metadata.

### Buses

```bash
eye bus tendillas            # find the stop by name, then its next buses
eye bus --stop 116           # by the number printed on the pole
eye bus                      # the stops the registry watches
```

```
DUE     LINE  ROUTE                               STOP                        OCCUPANCY
8 min   6     LEVANTE - TEJARES - B.GUADALQUIVIR  Ronda Tejares (Cruz Conde)  Ocupación Baja
11 min  2     FáTIMA - TEJARES - C. SANITARIA     Ronda Tejares (Cruz Conde)  Ocupación Baja
24 min  6     LEVANTE - TEJARES - B.GUADALQUIVIR  Ronda Tejares (Cruz Conde)  —

6 arrivals · operator estimate, read 0s ago
```

**These are predictions, not measurements**, and they are stored as
`quality=preliminary` so nothing downstream can mistake them for a timetable or
for a bus that has been seen.

AUCORSA publishes no documentation for this endpoint, so it is registered as an
undocumented personal source: the registry alone cannot switch it on, the
machine has to set `EYE_ALLOW_PERSONAL_SOURCES=1`, and its records never leave
that machine through `eye serve`. See
[ADR-0008](./docs/adr/0008-undocumented-personal-sources.md).

Search by name rather than guessing numbers. The operator's web pages carry a
second, unrelated identifier, and configuring that one returns no arrivals and
no error at all.

### The live board

```bash
eye watch                    # full screen, refreshing
eye watch --every 30s
eye watch --offline          # read beside a running eye daemon
```

```
E Y E   CÓRDOBA · 28 Aug · 13:53 · tick 7

── LATEST ────────────────────────────────────────────
 ▸ 30s   adsb.lol           GWOWO
 ▸ 5m    Diario Cordoba     Sorprendido al volante en Baena…
 ▸ 15m   El Dia de Cordoba  Manuel Gavira visita la sede…
 ▸ 18m   Cordopolis         Iván Ania: "Siento la ilusión…"
 ▸ 30m   Universidad        XI Congreso de la CUEMYC
 ▸ 30m   IGN                terremoto 28/08/2026 2:01:23

── SOURCES ───────────────────────────────────────────
  ● 12 answered   · 0 failed   ◐ 16 held
```

`▸` marks what arrived since you last looked.

**No single source is allowed to take the board.** ADS-B produces a record
every few seconds and is always the newest thing in the store, so a plain
newest-first feed is a list of aircraft callsigns with the city pushed off the
bottom. Every source gets a guaranteed share first; leftover rows are filled in
time order, because on a quiet night more aircraft beats blank space.

### Feeding other programs

eye is a gateway: providers are written once, and anything that speaks HTTP,
JSON or SQL reads the same normalized records with the same provenance
attached.

```bash
eye serve                      # http://127.0.0.1:8787
eye serve --addr 0.0.0.0:8787 --public
```

```
GET /v1/records   topic, source, kind, since, until, bbox, near, radius_km, text, limit
GET /v1/entities  kind, source, topic, near, radius_km, text, limit
GET /v1/sources   the registry — including what is held, and why
GET /health
```

```bash
curl 'localhost:8787/v1/records?topic=press&since=2h&text=feria'
curl 'localhost:8787/v1/entities?kind=camera&near=37.8882,-4.7794&radius_km=15'
```

`since` takes an RFC 3339 timestamp or a duration like `2h`, because a script
has one and a person has the other.

Three integration paths, in order of how much you want to be tied to eye:

| | |
|---|---|
| **HTTP** | `eye serve`, above |
| **JSON** | every command takes `--json`; pipe it anywhere |
| **SQL** | the store is a plain SQLite file at `~/.local/share/eye/eye.db` — open it read-only and query it |

Three things the API will not do. It **binds to loopback unless you pass
`--public`**. It **never serves camera images**. And it **never serves records
from a source marked `undocumented_personal`** — see
[ADR-0008](./docs/adr/0008-undocumented-personal-sources.md), which lets you
read undocumented endpoints for yourself while keeping eye from redistributing
what it was not entitled to.

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
