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
| `eye changes` | What appeared, moved or stopped being published since eye last looked |
| `eye watch` | A live board of the city, refreshing on screen |
| `eye tui` | A full-screen cockpit: dashboard, braille map, feed, sources, transit |
| `eye serve` | HTTP API and the web console, so other programs and people read the same records |
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

### What changed

Every other command answers "what is there". This one answers "what is
different", which is the question you actually have when you already looked an
hour ago.

```bash
eye changes                  # everything that moved in the last day
eye changes --since 2h
eye changes --new            # or --changed, or --gone
eye changes --topic transport
```

```
NEW
  28 Aug 22:00  press      El Puente Romano cierra al tráfico       Diario Córdoba

CHANGED
  28 Aug 22:00  press      Incendio en la Ribera                    Diario Córdoba
                             description: Un incendio junto al río → Extinguido

GONE
  28 Aug 21:59  press      Corte de agua en Ciudad Jardín           Diario Córdoba

3 changes · 1 new · 1 changed · 1 gone
```

**GONE means a working source stopped publishing it.** A record that stops
arriving may mean the thing ended, or that the feed broke, or that our poll
failed — and a tool that cannot tell those apart will eventually announce that
your train was cancelled because a server was down. eye already records the
last success and the staleness of every source, so a disappearance is reported
only when the source that used to publish it answered and left it out. A
failing or stale source produces silence here, never an ending.

The first run has nothing to compare against and says so, rather than
announcing every record in Córdoba as breaking news.

Identity is built only from what does not change — kind, title, roughly where,
roughly when — because a field inside the identity is a field whose change can
never be detected. The reasoning, and what it costs, is in
[ADR-0009](./docs/adr/0009-identity-before-fusion.md).

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

### The cockpit

`eye watch` is a board you leave running. `eye tui` is the one you drive.

```bash
eye tui                      # tab or 1-5 to switch, ? for keys, q to leave
eye tui --view map
eye tui --interval 10s
eye tui --once > frame.txt   # one frame, no keyboard — for a pipe or a log
```

Five screens over the same store: **DASHBOARD** (counts, per-source health, a
records-per-hour histogram), **MAP**, **FEED** (newest first, `/` filters as you
type, Enter opens the full record with its provenance), **SOURCES** (the whole
registry, `r` polls the selected one), and **TRANSIT** (stop search, live
arrivals, train departures).

The map is drawn in braille. Each terminal cell holds a 2×4 dot grid, so an
ordinary 100×30 terminal is a 200×120 pixel canvas — enough to see the shape of
the city, over SSH, with no image protocol and no dependency. Arrows pan, `+`
and `-` zoom, `f` fits to the data, and the status line reads out the cursor's
coordinates and the nearest feature.

Raw mode is entered through `stty` on `/dev/tty` and restored on every exit
path, signals included. On a terminal that has neither, the cockpit says so and
falls back to a refresh loop instead of failing.

### The web console

`eye serve` ships a web application compiled into the binary. There is nothing
to build, nothing to deploy beside it, and no way for the two to drift apart.

```bash
eye serve                    # console on http://127.0.0.1:8787/
eye serve --no-ui            # the JSON API alone
```

Five sections, the same five questions: a dashboard of counts and source health,
a dark map of everything with a position, a transit board with bus stop search,
live arrivals and train departures, a query explorer over every API filter with
CSV and GeoJSON download, and the registry with the reasoning each entry carries.

Clicking a feature on the map opens the full record **including its provenance** —
publisher, licence, source URL, and both timestamps with the latency between
them. That is the point of the whole project, so it is one click away rather
than buried.

The basemap is OpenStreetMap's own tiles, inverted in CSS rather than fetched
dark: no API key, no watermark, and attribution on the map where its licence
requires it. Leaflet loads from a pinned CDN with subresource integrity. If the
CDN is blocked the map says so and every other section keeps working.

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

53 sources declared in [`configs/sources.yaml`](./configs/sources.yaml), across
transport, air, weather, fire, hydrology, air quality, local press, events and
civic documents. `eye sources` reports which of them are live, which are held,
and which are permitted but have no adapter yet — three different states, and
none of them is a bug.

**Live:** the Córdoba press, BOE, PLACSP, UCO events, IMAE, the municipal CKAN
catalog and eleven of its map layers (parking, zona azul, bike racks, EV
chargers, taxi ranks, loading bays, cameras, buses), DGT incidents, VMS and
cameras, RENFE timetables and live delays for both Cercanías and AV/LD/MD, the
Córdoba transport consortium's timetable and service notices, AUCORSA lines and
stops, OpenStreetMap bus stop positions, IGN seismic, adsb.lol. AUCORSA live
arrivals is live only on a machine that opted into personal sources.

**Held:** SAIH Guadalquivir, INFOCA, REDIAM, Agenda Única, Turismo de Córdoba,
BOP, OpenSky, e-distribución and the four municipal IDE layers. Held means the
reuse terms are unresolved or there is no documented machine interface.

**Needs a free key:** AEMET (warnings and observations) and NASA FIRMS. Set
`AEMET_API_KEY` and `FIRMS_MAP_KEY` and they start working; without them the
adapters say so by name rather than polling empty.

An audit on 2026-09-03 re-probed every entry against its real endpoint, and
three things it found are worth repeating here, because each had been recorded
as fact and each was wrong:

- **DGT was never blocked.** The 403 that held its incident and VMS feeds for
  months applies to the CKAN discovery path, not to the published DATEX II
  files, which serve 200 to any client with or without a User-Agent.
- **RENFE does publish real time for AV/LD/MD** — the network that actually
  serves Córdoba. Two CC-BY-4.0 feeds, refreshed every minute, that the
  registry had recorded as non-existent.
- **AUCORSA does publish a GTFS feed**, through the national access point. It
  is login-gated and its service calendar expired in March 2026, which is a
  different problem from the one previously written down. See
[docs/legal/data-ethics.md](./docs/legal/data-ethics.md) for why a held source
is a normal outcome rather than a failure.

A source may also declare `sampled_kinds`: the record kinds it publishes as
samples of a moving signal rather than as statements. A train position is
different every time by definition, so diffing two of them reports the reading
back as news; a delay on the same feed is a statement, and a delay growing is
exactly what you want to be told. It is per kind because one feed does both.

## Documentation

| Document | What it is for |
|---|---|
| [docs/](./docs/README.md) | Documentation index |
| [Architecture overview](./docs/architecture/overview.md) | The shape of the system, with diagrams |
| [Roadmap](./docs/roadmap.md) | Nine epics and their dependencies |
| [Data ethics](./docs/legal/data-ethics.md) | Reuse, retention, the source checklist |
| [ADRs](./docs/adr/README.md) | The ten decisions this is built on |
| [Deployment](./docs/deployment.md) | Running the API privately: token, Docker, systemd |
| [Getting started](./docs/development/getting-started.md) | Build, test, add a provider |
| [AGENTS.md](./AGENTS.md) | Rules for AI agents working in this repo |

---

<div align="center">
<sub>Bootstrapped from the <code>backend-go</code> template of the <a href="https://github.com/hagalink">Hagalink</a> ecosystem — governance kept, HTTP-service runtime replaced.</sub>
</div>
