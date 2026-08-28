# Roadmap

Nine epics. Each one is a GitHub milestone; each issue inside it is meant to be
a single reviewable pull request.

The ordering is a dependency order, not a schedule. Effort figures are
engineering estimates for one experienced Go developer — not calendar dates and
not commitments.

## Dependency graph

```mermaid
flowchart LR
    E1["E1 · Core<br/>domain · registry<br/>scheduler · store"]

    E2["E2 · Road &amp; city<br/>DGT · CKAN"]
    E3["E3 · Environment<br/>AEMET · FIRMS<br/>SAIH · ICA"]
    E4["E4 · Movement<br/>ADS-B · GTFS-RT"]
    E5["E5 · Events<br/>agendas · dedupe<br/>change tracking"]

    E6["E6 · Fusion<br/>rule engine<br/>correlation"]
    E7["E7 · Interface<br/>status · radar<br/>TUI · serve"]
    E8["E8 · Civic<br/>BOE · BOP · PLACSP"]
    E9["E9 · Hardening<br/>retention · releases"]

    E1 --> E2
    E1 --> E3
    E1 --> E4
    E1 --> E5

    E2 --> E6
    E3 --> E6
    E4 --> E6
    E5 --> E6

    E2 --> E7
    E3 --> E7
    E6 --> E7

    E1 --> E8
    E8 --> E6

    E6 --> E9
    E7 --> E9

    style E1 fill:#e94560,stroke:#1a1a2e,color:#fff
    style E2 fill:#0984e3,stroke:#74b9ff,color:#fff
    style E3 fill:#0984e3,stroke:#74b9ff,color:#fff
    style E4 fill:#0984e3,stroke:#74b9ff,color:#fff
    style E5 fill:#0984e3,stroke:#74b9ff,color:#fff
    style E6 fill:#533483,stroke:#1a1a2e,color:#fff
    style E7 fill:#00b894,stroke:#55efc4,color:#0b2b22
    style E8 fill:#0f4c75,stroke:#1b262c,color:#bbe1fa
    style E9 fill:#2d3436,stroke:#636e72,color:#eaeaea
```

`E1` is the only hard blocker. Once the core exists, the four source epics are
independent of each other and can be picked up in any order — or in parallel by
different people.

## Epics

### E1 · Core — the spine

Everything else plugs into this. Nothing here talks to the network.

- Source registry loader and validation from `configs/sources.yaml`
- Scheduler: jitter, exponential backoff, circuit breaker, `Retry-After`,
  ETag / `If-Modified-Since`, per-host concurrency limits
- Source health tracking and staleness
- SQLite store (WAL, CGO-free) behind the domain port
- Raw cache: gzipped payloads addressed by SHA-256
- Structured logging with `log/slog`
- `eye status` rendering source health, including held sources

*Estimate: 28–40 h*

### E2 · Road and city

- Generic DATEX II 3.7 reader, driven by the published XSD
- DGT incident provider
- DGT camera **inventory** provider — positions and metadata only
- Generic CKAN provider, reusable for any municipality
- Córdoba CKAN camera and mobility assets

*Estimate: 24–36 h*

### E3 · Environment

- AEMET provider, handling the two-step `datos` URL protocol
- NASA FIRMS provider (thermal anomalies)
- MITECO ICA provider (air quality)
- INFOCA daily risk raster via WMS
- SAIH Guadalquivir adapter — **only** once its reuse terms are resolved;
  readings are recorded as `quality: preliminary`
- IGN seismic catalogue

*Estimate: 28–42 h*

### E4 · Movement

- adsb.lol provider with a configurable viewport
- OpenSky provider with OAuth2 — held on licence review for non-personal use
- Generic GTFS parser (static)
- Generic GTFS-RT parser (protobuf)
- RENFE provider: TripUpdates + VehiclePositions
- AUCORSA static GTFS
- Retention policy: `ExpiresAt` on movement records, aggregates for the long term

*Estimate: 28–42 h*

### E5 · Events — what Córdoba is about to do

The second axis of the project. Not a calendar: a tracker.

- `Event` domain with a status machine — `scheduled`, `confirmed`, `changed`,
  `postponed`, `cancelled`, `sold_out`, `finished`
- Fingerprint deduplication: normalised title + venue + start time truncated to
  30 minutes, plus fuzzy matching for the rest
- Observation model: one canonical event, many source observations, confidence
  rising with independent agreement
- Change detection producing `EVENT_CHANGE` diffs
- Providers: UCO (ICS — the easiest and the one to build first), Agenda Única,
  Turismo de Córdoba, IMAE
- `eye events`, `eye event <query>`, `eye events --new|--changed|--cancelled`

*Estimate: 30–45 h*

### E6 · Fusion

- Spatio-temporal windowing over the record store
- Rule engine reading `configs/rules.yaml`
- The four seed rules: wildfire, flood watch, transport disruption, smoke impact
- Confidence scoring from independent source agreement
- Provenance graph: every inference resolves to the raw payloads behind it
- Every output labelled `quality: inferred`, with its disclaimer

*Estimate: 30–45 h*

### E7 · Interface

- `eye status` — the city in one screen
- `eye query --bbox --since --topic`
- `eye radar` — the next 72 hours
- `eye tonight`, `eye whatsup`
- Bubble Tea TUI with source health, timeline and filters
- Image preview through Kitty Graphics / chafa, in a suspended terminal rather
  than inside the render loop
- `eye serve` — REST + SSE, opt-in

*Estimate: 24–36 h*

### E8 · Civic intelligence

The unglamorous half, and where the early signals live. A large event leaves a
public trace — a contract, an authorisation, a road closure — months before it
is announced.

- BOE OpenData API and RSS
- BOP Córdoba
- PLACSP public procurement
- Diputación de Córdoba CKAN
- Full-text search over documents
- Weak-signal detection: a `POSSIBLE EVENT` with a confidence score and its
  signals listed, never an assertion that something will happen

*Estimate: 24–36 h*

### E9 · Hardening

- Retention enforcement and compaction into aggregates
- Metrics: latency, error rate, freshness per source
- Recorded fixtures in `testdata/` so no test touches the network
- Cross-compiled release binaries via release-please
- Privacy and licence documentation kept in step with the registry

*Estimate: 32–48 h*

## Order of magnitude

Roughly **220–320 hours** in total. The variance sits in three places: whether
SAIH and INFOCA ever offer a stable contract, how exhaustively DATEX II gets
parsed, and how far event deduplication has to go before it stops producing
six copies of the same concert.

## The version that matters

The goal is not `eye cameras`. It is `eye status` answering, from evidence:

```
CÓRDOBA · 28 AUG 2026 21:38

City state: NORMAL

Signals:
  DGT       2 active road incidents < 50 km
  FIRMS     no correlated fire signal < 50 km
  INFOCA    fire risk HIGH in western sector
  AEMET     no severe warning
  SAIH      river levels stable        (preliminary)
  ICA       air quality GOOD
  EVENTS    7 today · 2 with mobility impact

Correlations:
  No multi-source critical incident detected.

Sources:
  18 healthy
   1 stale
   3 held: reuse terms unresolved

Press [Enter] for evidence.
```

Every line of that traces back to a public source, and the last line is the
whole point.
